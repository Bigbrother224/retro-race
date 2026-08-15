package app

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"retrorace/internal/arbiter"
	"retrorace/internal/engine"
	"retrorace/internal/library"
	"retrorace/internal/netplay"
	"retrorace/internal/relay"
	"retrorace/internal/replay"
	"retrorace/internal/rollback"
	"retrorace/internal/shm"
)

// gameSession owns everything about playing a game: the emulator, the race,
// the real rival process, deterministic replay, input routing and the netplay
// shared-game session. App keeps the menu/navigation and hands the selected
// game to a session when play begins. This is the R4 split that de-gods App.
type gameSession struct {
	app *App            // back-ref for navigation, errors, profile name, frame counter
	con library.Console // the selected console
	g   library.Game    // the selected game

	// Emulator.
	emu     engine.Emulator
	paused  bool
	img     *ebiten.Image
	arbiter *arbiter.Arbiter

	// Local race: real rival process, PiP, gauge, replay.
	race       *race
	pipImg     *ebiten.Image
	replayImgs [2]*ebiten.Image

	// Real rival: a headless process running its own core, publishing its
	// framebuffer + state over shared memory (the product's two-process race).
	rivalProc   *os.Process
	rivalCmd    *exec.Cmd
	rivalCons   *shm.Consumer
	rivalShm    string
	rivalFinish int // -1 until the rival's real finish arrives

	// Replay: deterministic playback of a recorded run.
	replayPlayer *replay.Player
	replayMsg    string // export confirmation or error shown in dramatic ending

	// Input gate (2 players): player 0 -> port 0, player 1 -> port 1.
	players [2][12]bool
	// keyboardActive is latched once a mapped key is pressed during a race. It
	// tells the gate whether the keyboard is a live player (so keyboard +
	// one gamepad reads as two players). Reset at each race start.
	keyboardActive bool

	// Netplay shared-game session (menu lobby + live play).
	netSess        *netplay.Session
	netRole        int // 0 = not chosen, 1 = host, 2 = guest
	netCode        string
	netErr         string
	netConnCh      chan connectResult
	netConnecting  bool
	netPlayers     int  // core controller-port capability (-1 unknown)
	netKnown       bool // whether netPlayers comes from the known-game table
	netRendered    int  // frames rendered, for the divergence check
	netLastCheck   int
	netRb          *rollback.Session // predictive rollback re-simulation state
	netCorrections int               // rollback corrections applied (for HUD/debug)
}

func newGameSession(app *App, con library.Console, g library.Game) *gameSession {
	return &gameSession{
		app:            app,
		con:            con,
		g:              g,
		arbiter:        arbiter.New(arbiter.DefaultConfig()),
		rivalFinish:    -1,
		replayImgs:     [2]*ebiten.Image{},
		netPlayers:     -1,
		keyboardActive: false,
	}
}

// load starts the selected game on a fresh core. It is the solo/race entry.
func (s *gameSession) load() error {
	corePath := filepath.Join(coresDir, s.con.Core)
	log.Printf("launching %s on %s (%s)", s.g.Name, s.con.Name, corePath)
	core := engine.NewCore()
	if err := core.Start(s.g.Path, corePath); err != nil {
		return err
	}
	s.emu = core
	s.arbiter.Reset()
	s.race = nil // solo play by default; a race is started explicitly
	s.replayPlayer = nil
	return nil
}

// ---- input ----

// updatePlayerInputs collects raw input per logical player, independent of
// which emulator port they will drive. Keyboard is player 0; gamepads are
// assigned in connection order (first -> player 0, second -> player 1). This
// is the only place physical devices are turned into player button states, so
// a future remote source (relayed input packets) plugs in here as one more
// player.
func (s *gameSession) updatePlayerInputs() {
	s.players = [2][12]bool{}

	// Keyboard -> player 0. Latch keyboard activity once any mapped key is
	// seen so a keyboard + gamepad combo reads as two players.
	keyUsed := false
	for key, btn := range keyboardButtons {
		if inpututil.IsKeyJustPressed(key) || ebiten.IsKeyPressed(key) {
			s.players[0][btn] = true
			keyUsed = true
		}
	}
	if keyUsed {
		s.keyboardActive = true
	}

	gps := s.standardGamepads()
	if len(gps) == 0 {
		return // keyboard only
	}

	_, start := playerPlan(s.keyboardActive, len(gps))
	p := start
	for _, id := range gps {
		if p >= len(s.players) {
			break
		}
		s.readGamepad(id, p)
		p++
	}
}

// standardGamepads returns every connected gamepad.
func (s *gameSession) standardGamepads() []ebiten.GamepadID {
	return ebiten.AppendGamepadIDs(nil)
}

// readGamepad reads one gamepad's pressed buttons into player p's state.
func (s *gameSession) readGamepad(id ebiten.GamepadID, p int) {
	readGamepadButtons(id, &s.players[p])
}

// applyGate routes the two logical players' button states to the emulator
// controller ports (player 0 -> port 0, player 1 -> port 1). Local two-gamepad
// play and remote relayed inputs both land here, so the routing is identical
// whether the players are side by side or across the world.
func (s *gameSession) applyGate() {
	if s.emu == nil {
		return
	}
	for port := 0; port < 2; port++ {
		st := s.players[port]
		for b := 0; b < 12; b++ {
			s.emu.SetButtonPort(port, engine.JoyButton(b), st[b])
		}
	}
}

// updateGameInput collects raw input from keyboard + gamepads into per-player
// states, routes them through the gate to the emulator controller ports, and
// returns the effective port-0 state (what actually drove the player's game)
// for the input recorder.
func (s *gameSession) updateGameInput() [12]bool {
	s.updatePlayerInputs()
	s.applyGate()
	return s.players[0]
}

// ---- race ----

// startRace launches a real rival: a headless process running its own core on
// the same ROM, publishing its framebuffer and race state over shared memory.
// This is the product's "one process per player" architecture — the rival is a
// genuine second emulator instance, not a delayed copy of the player's game.
func (s *gameSession) startRace() {
	s.race = newRace(DefaultRaceConfig())
	s.keyboardActive = false
	s.pipImg = nil
	s.replayImgs = [2]*ebiten.Image{}
	s.rivalFinish = -1

	if s.emu == nil {
		return
	}

	// Spawn the headless rival with the same ROM/core.
	self, err := os.Executable()
	if err != nil {
		log.Printf("rival: cannot find executable: %v", err)
		return
	}
	s.rivalShm = fmt.Sprintf("/tmp/retro_race_rival_%d.shm", os.Getpid())
	os.Remove(s.rivalShm)

	run := s.findRunFor()
	args := []string{"--rival", "--rom", s.g.Path, "--core", filepath.Join(coresDir, s.con.Core),
		"--shm", s.rivalShm, "--expected", fmt.Sprintf("%d", DefaultRaceConfig().ExpectedFrames)}
	if run != "" {
		args = append(args, "--run", run)
	}
	cmd := exec.Command(self, args...)
	cmd.Env = os.Environ()
	if stderr, err := cmd.StderrPipe(); err == nil {
		go func() {
			buf := make([]byte, 4096)
			for {
				n, _ := stderr.Read(buf)
				if n == 0 {
					return
				}
				log.Printf("rival(stderr): %s", string(buf[:n]))
			}
		}()
	}
	if err := cmd.Start(); err != nil {
		log.Printf("rival: spawn failed: %v", err)
		s.rivalShm = ""
		return
	}
	s.rivalCmd = cmd
	s.rivalProc = cmd.Process

	// Wait for the rival to create the region, then map it (the SNES core can
	// take a couple of seconds to load).
	for range 200 {
		if cons, err := shm.OpenConsumer(s.rivalShm); err == nil {
			s.rivalCons = cons
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if s.rivalCons == nil {
		log.Printf("rival: consumer open failed")
	}
}

// findRunFor returns the most recent recorded run for the game, if any.
func (s *gameSession) findRunFor() string {
	dir := replaysDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	prefix := sanitizeFilename(s.g.Name) + "-"
	best := ""
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if e.Name() > best {
			best = e.Name()
		}
	}
	if best == "" {
		return ""
	}
	return filepath.Join(dir, best)
}

// ---- playing ----

func (s *gameSession) updatePlaying() {
	if s.emu == nil {
		return
	}
	// Shared game over the network: exchange inputs with the peer.
	if s.netSess != nil {
		s.updateNetplayPlaying()
		return
	}
	// Deterministic replay playback: step the recorded run, no race logic.
	if s.replayPlayer != nil {
		if !s.replayPlayer.Step() {
			s.stopGame()
		}
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		s.stopGame()
		return
	}

	// Pause / resume (P). While paused the emulator is frozen.
	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		s.paused = !s.paused
	}
	if s.paused {
		// Reset while paused restarts the game from its initial state.
		if inpututil.IsKeyJustPressed(ebiten.KeyR) {
			s.emu.Reset()
			s.arbiter.Reset()
		}
		return
	}

	// Solo play: no race, no arbiter, just play (ESC to return to the menu).
	if s.race == nil {
		s.emu.Step()
		s.race = nil
		return
	}

	switch s.race.State() {
	case racePlaying:
		state := s.updateGameInput()
		s.race.RecordInput(state[:])
		s.emu.Step()
		if f := s.emu.Frame(); f != nil {
			s.race.AddPlayerFrame(f)
			// Real rival: read its live framebuffer + state from shared memory.
			s.consumeRival()
			if s.arbiter.Update(f) == arbiter.SegmentEnd {
				s.race.PlayerFinished()
			}
		}
	case raceSlowmo:
		// Slow motion: step the game at a reduced cadence.
		if s.race.Frame()%4 == 0 {
			s.emu.Step()
		}
	case raceReplay:
		// Dramatic ending: R to replay, E to export.
		if inpututil.IsKeyJustPressed(ebiten.KeyR) {
			s.startReplay()
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyE) {
			s.exportReplay()
		}
	case raceDone:
		// Persist this run so the next race replays it as the rival ("Beat
		// this Ghost").
		s.saveRun()
		s.stopGame()
		return
	}
	s.race.Tick()
}

// consumeRival reads the rival's latest published frame + state from shared
// memory: its framebuffer drives the PiP, and its real progress/finish drive
// the race.
func (s *gameSession) consumeRival() {
	if s.rivalCons == nil {
		return
	}
	snap := s.rivalCons.Take()
	if snap == nil {
		return
	}

	// PiP: the rival's real framebuffer.
	w, h := snap.Width, snap.Height
	if len(snap.Frame) > 0 && w > 0 && h > 0 && w*h*4 == len(snap.Frame) && s.race != nil {
		if s.pipImg == nil || s.pipImg.Bounds().Dx() != w || s.pipImg.Bounds().Dy() != h {
			s.pipImg = ebiten.NewImage(w, h)
		}
		s.pipImg.WritePixels(snap.Frame)
		s.race.AddOppFrame(snap.Frame)
	}

	// Real finish: the rival's arbiter detected a segment end on ITS screen.
	if snap.State == shm.StateDone && s.rivalFinish < 0 {
		s.rivalFinish = int(snap.FinishFrame)
		s.race.OppFinished()
	}

	// Real progress feeds the gauge.
	s.race.SetOppProgress(float64(snap.Progress) / 1000.0)
}

func (s *gameSession) stopGame() {
	if s.emu != nil {
		s.emu.Stop()
		s.emu = nil
	}
	s.stopRival()
	s.paused = false
	s.app.state = stateGame
	s.arbiter.Reset()
	s.race = nil
	s.pipImg = nil
	s.replayImgs = [2]*ebiten.Image{}
	s.replayPlayer = nil
	s.replayMsg = ""
}

// stopRival terminates the headless rival process and releases the shm region.
func (s *gameSession) stopRival() {
	if s.rivalCons != nil {
		s.rivalCons.Close()
		s.rivalCons = nil
	}
	if s.rivalProc != nil {
		s.rivalProc.Kill()
		s.rivalProc.Wait()
		s.rivalProc = nil
	}
	s.rivalCmd = nil
	if s.rivalShm != "" {
		os.Remove(s.rivalShm)
		s.rivalShm = ""
	}
	s.rivalFinish = -1
}

// replayCore adapts an engine.Emulator to the replay.Core interface.
type replayCore struct{ e engine.Emulator }

func (c replayCore) SetButton(b int, pressed bool) { c.e.SetButton(engine.JoyButton(b), pressed) }
func (c replayCore) Step()                         { c.e.Step() }

// buildRun assembles a replay.Run from the current race and game metadata.
func (s *gameSession) buildRun() *replay.Run {
	return &replay.Run{
		Game:    s.g.Name,
		Console: s.con.ID,
		Core:    s.con.Core,
		ROMHash: s.g.SHA256,
		ROMPath: s.g.Path,
		Width:   s.emu.Width(),
		Height:  s.emu.Height(),
		Frames:  s.race.InputDuration(),
		Events:  s.race.RunEvents(),
	}
}

// startReplay stops the current core, starts a fresh one with the recorded
// game, and begins deterministic playback.
func (s *gameSession) startReplay() {
	run := s.buildRun()
	if len(run.Events) == 0 {
		s.replayMsg = "Rien à rejouer (aucun input enregistré)"
		return
	}
	if s.emu != nil {
		s.emu.Stop()
	}
	core := engine.NewCore()
	if err := core.Start(run.ROMPath, run.Core); err != nil {
		s.app.errMsg = "replay: " + err.Error()
		s.emu = nil
		s.stopGame()
		return
	}
	s.emu = core
	s.replayPlayer = replay.NewPlayer(run, replayCore{core})
	s.race = nil
	s.pipImg = nil
	s.replayImgs = [2]*ebiten.Image{}
	s.arbiter.Reset()
}

// saveRun writes the recorded run to app/Replays/ so a later race can replay
// it as the rival ("Beat this Ghost"). Returns the written filename or "".
func (s *gameSession) saveRun() string {
	run := s.buildRun()
	if len(run.Events) == 0 {
		return ""
	}
	data, err := replay.MarshalRun(run)
	if err != nil {
		return ""
	}
	dir := replaysDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	name := sanitizeFilename(run.Game) + "-" + fmt.Sprintf("%d", time.Now().Unix()) + ".json"
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return ""
	}
	return name
}

// exportReplay serialises the recorded run to a JSON file in app/Replays/.
func (s *gameSession) exportReplay() {
	if name := s.saveRun(); name != "" {
		s.replayMsg = "Replay exporté : " + name
	} else {
		s.replayMsg = "Rien à exporter (aucun input enregistré)"
	}
}

// ---- netplay lobby + shared-game session ----

// enterNetLobby opens the shared-game lobby for the selected game. The host is
// the default role and a fresh room code is generated.
func (s *gameSession) enterNetLobby() {
	s.netSess = nil
	s.netRole = 1 // host by default
	s.netCode = relay.GenerateCode()
	s.netErr = ""
	s.netConnCh = nil
	s.netConnecting = false
	s.netPlayers = -1
	s.netRendered = 0
	s.netLastCheck = 0
	s.app.state = stateNetLobby
}

// updateNetLobby handles role choice, room-code entry (guest), and starting the
// (asynchronous) relay connection.
func (s *gameSession) updateNetLobby() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		s.app.state = stateGame
		return
	}

	if s.netConnecting {
		select {
		case res := <-s.netConnCh:
			s.netConnecting = false
			if res.err != nil {
				s.netErr = res.err.Error()
			} else {
				s.netSess = res.sess
				s.startNetplayGame()
			}
		default:
		}
		return
	}

	// Toggle host/guest.
	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) || inpututil.IsKeyJustPressed(ebiten.KeyRight) {
		s.netRole = 3 - s.netRole
		if s.netRole == 1 {
			s.netCode = relay.GenerateCode() // a new room gets a fresh code
		} else {
			s.netCode = ""
		}
	}

	// Guest types the code the host gave them.
	if s.netRole == 2 {
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(s.netCode) > 0 {
			r := []rune(s.netCode)
			s.netCode = string(r[:len(r)-1])
		}
		for _, ch := range ebiten.InputChars() {
			ch = unicode.ToUpper(ch)
			if (ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9') && len(s.netCode) < 12 {
				s.netCode += string(ch)
			}
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		if len(s.netCode) < 4 {
			s.netErr = "Entre le code de la partie (4+ caracteres)"
			return
		}
		s.connectNetplay()
	}
}

// connectNetplay opens the relay connection asynchronously (the host waits for
// the guest to join the room, which can take a while).
func (s *gameSession) connectNetplay() {
	corePath := filepath.Join(coresDir, s.con.Core)
	gameID, err := gameIDFor(s.g.Path, corePath)
	if err != nil {
		s.netErr = err.Error()
		return
	}
	s.netErr = ""
	s.netConnecting = true
	s.netConnCh = make(chan connectResult, 1)
	go func() {
		sess, err := netplay.Relay(RelayAddr, s.netCode, gameID)
		s.netConnCh <- connectResult{sess, err}
	}()
}

// startNetplayGame launches the local core once both players are connected and
// enters the shared-game play state.
func (s *gameSession) startNetplayGame() {
	corePath := filepath.Join(coresDir, s.con.Core)
	core := engine.NewCore()
	if err := core.Start(s.g.Path, corePath); err != nil {
		s.netErr = err.Error()
		if s.netSess != nil {
			s.netSess.Close()
			s.netSess = nil
		}
		return
	}
	s.emu = core
	s.netSess.SetAutoDelay(true, time.Second/60)
	s.netRb = nil
	if _, err := core.Save(); err == nil {
		// Only enable rollback if the core truly supports save states; a core
		// that does not would fail every correction. The window must hold the
		// frame before the oldest correctable frame (predictMax+2).
		s.netRb = rollback.New(rollbackCore{core}, int(netPredictMax)+2)
		s.netSess.SetPredict(true, netPredictMax)
	}
	s.netPlayers = core.ControllerPlayers()
	s.netKnown = false
	if n, ok := knownPlayerCount(s.g.Name); ok {
		s.netPlayers = n
		s.netKnown = true
	}
	s.paused = false
	s.app.state = statePlaying
	s.arbiter.Reset()
}

// stopNetplay ends a shared game and returns to the game list.
func (s *gameSession) stopNetplay() {
	if s.netSess != nil {
		s.netSess.Close()
		s.netSess = nil
	}
	if s.emu != nil {
		s.emu.Stop()
		s.emu = nil
	}
	s.netRb = nil
	s.app.state = stateGame
}

// updateNetplayPlaying drives the shared-game loop: exchange inputs, apply the
// merged ports, step the core, and watch for divergence.
func (s *gameSession) updateNetplayPlaying() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		s.stopNetplay()
		return
	}
	local := netReadLocalButtons()
	if !s.netSess.Send(local) {
		s.netErr = "Connexion perdue : " + s.netSess.Err().Error()
		s.stopNetplay()
		return
	}
	rendered := 0
	for {
		my, peer, advanced, ok := s.netSess.RenderNext()
		if !ok {
			s.netErr = "Connexion perdue : " + s.netSess.Err().Error()
			s.stopNetplay()
			return
		}
		if !advanced {
			break
		}
		var p1, p2 [netplay.ButtonCount]bool
		if s.netSess.LocalPort() == 1 {
			p1, p2 = my, peer
		} else {
			p1, p2 = peer, my
		}
		if s.netRb != nil {
			// rollback.Commit applies the inputs, steps the core once and
			// records the state. The app must NOT step separately here or the
			// core runs at 2x speed.
			s.netRb.Commit(mask16(p1, p2))
		} else {
			for b := 0; b < netplay.ButtonCount; b++ {
				s.emu.SetButtonPort(0, engine.JoyButton(b), p1[b])
				s.emu.SetButtonPort(1, engine.JoyButton(b), p2[b])
			}
			s.emu.Step()
		}
		s.netRendered++
		rendered++
		if rendered >= 4 {
			break
		}
	}
	// Apply any predictions that turned out wrong (rollback re-simulation).
	for s.netRb != nil {
		frame, realPeer, ok := s.netSess.TakeCorrection()
		if !ok {
			break
		}
		if !s.applyCorrection(int(frame), realPeer, s.netSess.LocalPort()) {
			return
		}
	}
	if s.netCheckDivergence() {
		s.stopNetplay()
	}
}

// applyCorrection re-simulates from the frame where the peer's real input
// differed from what was predicted. Returns false if the rollback fails (the
// session is stopped with an error).
func (s *gameSession) applyCorrection(frame int, realPeer uint16, localPort int) bool {
	last := s.netRb.Last()
	var ins []rollback.Input
	for f := frame; f <= last; f++ {
		in, ok := s.netRb.Input(f)
		if !ok {
			s.netErr = "rollback: frame hors fenetre"
			s.stopNetplay()
			return false
		}
		if f == frame {
			// realPeer is the peer's input; put it in the peer's slot (P1 when
			// we are player 2, P2 when we are player 1).
			if localPort == 1 {
				in.P2 = realPeer
			} else {
				in.P1 = realPeer
			}
		}
		ins = append(ins, in)
	}
	if err := s.netRb.Correct(frame, ins); err != nil {
		s.netErr = "rollback: " + err.Error()
		s.stopNetplay()
		return false
	}
	s.netCorrections++
	return true
}

// netCheckDivergence periodically hashes this machine's game state and compares
// with the peer, ending the session on divergence.
func (s *gameSession) netCheckDivergence() bool {
	if s.netRendered < s.netLastCheck+netplayCheckEvery {
		return false
	}
	h, ok := s.emu.(stateHasher)
	if !ok {
		return false
	}
	myHash := h.StateHash()
	if myHash == 0 {
		return false
	}
	s.netLastCheck = s.netRendered
	s.netSess.SendStateHash(myHash)

	myFrame := s.netSess.RenderFrame()
	if pf, ph, have := s.netSess.PeerHash(); have {
		if pf >= myFrame && pf-myFrame <= s.netSess.InputDelay()+8 {
			if ph != myHash {
				s.netErr = "DIVERGENCE : les deux machines ne sont plus synchronisees"
				return true
			}
		}
	}
	return false
}

// drawNetLobby renders the shared-game lobby.
func (s *gameSession) drawNetLobby(screen *ebiten.Image) {
	drawBG(screen)
	psTextC(screen, "PARTIE PARTAGEE", 480, 60, 20, colAccent2)
	psTextC(screen, s.g.Name, 480, 96, 12, colTextDim)

	// Role cards.
	roles := []string{"HOTE  (cree)", "INVITE  (rejoint)"}
	for i, label := range roles {
		x := 240 + i*240
		sel := s.netRole == i+1
		fill, border := colPanel, colBorder
		if sel {
			fill, border = colPanelHi, colAccent2
		}
		drawPanel(screen, x, 170, 200, 70, fill, border)
		tc := colTextDim
		if sel {
			tc = colAccent2
		}
		psTextC(screen, label, float64(x)+100, 200, 13, tc)
	}

	// Code display / entry.
	psTextC(screen, "CODE DE LA PARTIE", 480, 300, 12, colTextDim)
	code := s.netCode
	if (s.app.frame/30)%2 == 0 && s.netRole == 2 && !s.netConnecting {
		code += "|"
	}
	psTextC(screen, code, 480, 336, 30, colText)
	fillRect(screen, 320, 366, 320, 2, colBorder)

	if s.netConnecting {
		msg := "En attente du partenaire... (code " + s.netCode + ")"
		if s.netRole == 2 {
			msg = "Connexion au code " + s.netCode + "..."
		}
		psTextC(screen, msg, 480, 430, 12, colAccent2)
		psTextC(screen, "Relais : "+RelayAddr, 480, 456, 9, colTextDim)
	} else {
		hint := "← / →  ROLE   ·   ENTER  CONNECT"
		if s.netRole == 2 {
			hint = "TAPE LE CODE   ·   ENTER  CONNECT"
		}
		psTextC(screen, hint, 480, 430, 10, colTextDim)
		psTextC(screen, "Relais : "+RelayAddr, 480, 456, 9, colTextDim)
	}

	if s.netErr != "" {
		psTextC(screen, strings.ToUpper(s.netErr), 480, 500, 10, colAccent)
	}

	drawFooter(screen, "ESC  BACK", "", "")
}

// drawNetplayHUD renders the shared-game overlay during play: role, room code,
// multiplayer capability and network telemetry.
func (s *gameSession) drawNetplayHUD(screen *ebiten.Image) {
	role := "JOUEUR 1  (HOTE)"
	cc := colPlayer
	code := s.netCode
	if s.netSess.LocalPort() == 2 {
		role = "JOUEUR 2  (INVITE)"
		cc = colOpponent
	}
	psText(screen, role, 16, 14, 12, cc)
	if code != "" {
		psText(screen, "CODE "+code, 16, 40, 9, colTextDim)
	}
	switch {
	case s.netKnown && s.netPlayers >= 2:
		psText(screen, "2P confirme (jeu connu)", 16, 62, 9, colTextDim)
	case s.netKnown && s.netPlayers == 1:
		psText(screen, "Jeu solo : pas de 2e personnage", 16, 62, 9, colAccent2)
	case s.netPlayers >= 2:
		psText(screen, fmt.Sprintf("%d manettes (materiel) — le 2e perso agit ?", s.netPlayers), 16, 62, 9, colTextDim)
	case s.netPlayers == 1:
		psText(screen, "Jeu solo (probable) : pas de 2e personnage", 16, 62, 9, colAccent2)
	default:
		psText(screen, "Compat 2 joueurs : inconnue", 16, 62, 9, colTextDim)
	}
	if st := s.netSess.Stats(); st.Rendered > 0 {
		rb := ""
		if s.netRb != nil {
			rb = fmt.Sprintf("   rollback %d", s.netCorrections)
		}
		psText(screen, fmt.Sprintf("frames %d   buffer %d   stalls %d   delay %d   ping %d ms%s",
			st.Rendered, st.Buffered, st.Stalled, s.netSess.InputDelay(), s.netSess.RTT().Milliseconds(), rb), 16, 86, 9, colTextDim)
	}
	psText(screen, "ESC  QUITTER", 16, float64(screen.Bounds().Dy())-24, 9, colTextDim)
}

// ---- drawing (race + playing) ----

// racePanelW is the width reserved for the race dashboard on the right of the
// game. The game keeps its native aspect and full pixel quality in the space
// left over; nothing is ever drawn on top of it.
const racePanelW = 232

// drawPlaying renders the emulated game in the left part of the window at its
// native aspect (pixel-perfect, never covered) and, during a race, a clean
// dashboard on the right showing the rival's screen and the progress gauge.
func (s *gameSession) drawPlaying(screen *ebiten.Image) {
	if s.app.errMsg != "" {
		ebitenutil.DebugPrint(screen, s.app.errMsg)
		return
	}
	frame := s.emu.Frame()
	w, h := s.emu.Width(), s.emu.Height()
	if frame == nil || w == 0 || h == 0 {
		ebitenutil.DebugPrint(screen, "chargement en cours…")
		return
	}
	if s.img == nil || s.img.Bounds().Dx() != w || s.img.Bounds().Dy() != h {
		s.img = ebiten.NewImage(w, h)
	}
	s.img.WritePixels(frame)

	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	// In a race the game occupies the left area; in replay it uses the whole
	// window.
	gameW := sw
	if s.race != nil && s.replayPlayer == nil {
		gameW = sw - racePanelW
	}
	scale := min(float64(gameW)/float64(w), float64(sh)/float64(h))
	dispW, dispH := float64(w)*scale, float64(h)*scale
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate((float64(gameW)-dispW)/2, (float64(sh)-dispH)/2)
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(s.img, op)

	// Shared game over the network: full-screen game + a small HUD.
	if s.netSess != nil {
		s.drawNetplayHUD(screen)
		if s.netErr != "" {
			fillRect(screen, 0, 0, screen.Bounds().Dx(), screen.Bounds().Dy(), color.RGBA{0, 0, 0, 0x80})
			psTextC(screen, strings.ToUpper(s.netErr), 480, 356, 16, colAccent)
		}
		drawScanlines(screen)
		return
	}

	// Deterministic replay: game image + label, no race overlays.
	if s.replayPlayer != nil {
		drawText(screen, "REPLAY — Échap pour quitter", 20, 16, 1, colAccent2)
		drawScanlines(screen)
		return
	}

	// Race dashboard on the right (the game is never covered).
	if s.race != nil {
		switch s.race.State() {
		case racePlaying, raceSlowmo:
			s.drawRacePanel(screen, s.race)
			if s.race.State() == raceSlowmo {
				// Centered on the game area, not the whole window.
				gcx := gameW / 2
				drawPanel(screen, gcx-150, 300, 300, 40, colPanelHi, colAccent)
				drawText(screen, "FIN DE SEGMENT — RALENTI", gcx-122, 310, 1, colAccent2)
			}
		case raceReplay:
			s.drawDramaticEnding(screen, s.race)
		}
	}

	// Pause overlay: frozen game + controls hint.
	if s.paused {
		fillRect(screen, 0, 0, screen.Bounds().Dx(), screen.Bounds().Dy(), color.RGBA{0, 0, 0, 0x60})
		drawPanel(screen, 300, 260, 360, 90, colPanelHi, colAccent)
		psTextC(screen, "PAUSE", 480, 282, 24, colText)
		psTextC(screen, "R  RESET      P  REPRENDRE      ESC  QUITTER", 480, 322, 11, colTextDim)
	}

	// Quiet exit hint, bottom-left of the game area.
	psText(screen, "ESC  MENU", 16, 700, 9, colTextDim)

	drawScanlines(screen)
}

// drawRacePanel renders the race dashboard on the right of the game.
func (s *gameSession) drawRacePanel(screen *ebiten.Image, r *race) {
	x := 960 - racePanelW
	w := racePanelW

	// Panel background so it reads as its own surface, not floating text.
	fillRect(screen, x, 0, w, 720, color.RGBA{0x0a, 0x0a, 0x10, 0xff})
	fillRect(screen, x, 0, 2, 720, color.RGBA{0xff, 0xff, 0xff, 0x0d})

	// Header: the race.
	psTextC(screen, "COURSE", float64(x)+float64(w)/2, 20, 12, colTextDim)
	fillRect(screen, x+16, 44, w-32, 2, colBorder)

	// Rival's live screen — the wow: you can see where the opponent is.
	s.drawRivalScreen(screen, float64(x), float64(w))

	// Progress gauge: clear and readable (two labelled bars).
	drawRaceGauge(screen, r, s.app.playerName(), float64(x), float64(w))

	// Exit hint at the bottom of the panel.
	psTextC(screen, "ESC  MENU", float64(x)+float64(w)/2, 700, 9, colTextDim)
}

// drawRivalScreen renders the opponent's live framebuffer as a large framed
// window inside the race panel.
func (s *gameSession) drawRivalScreen(screen *ebiten.Image, px, pw float64) {
	if s.pipImg == nil {
		return
	}
	const viewW, viewH = 204, 152
	// Fit the rival frame into the view, letterboxed.
	iw, ih := float64(s.pipImg.Bounds().Dx()), float64(s.pipImg.Bounds().Dy())
	if iw == 0 || ih == 0 {
		return
	}
	scale := math.Min(viewW/iw, viewH/ih)
	dw, dh := iw*scale, ih*scale
	bx := px + (pw-viewW)/2
	by := 76.0

	// Frame + label above.
	fillRect(screen, int(bx)-2, int(by)-2, int(viewW)+4, int(viewH)+4, colOpponent)
	psTextC(screen, "RIVAL", bx+viewW/2, by-22, 10, colOpponent)

	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterNearest
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(bx+(viewW-dw)/2, by+(viewH-dh)/2)
	screen.DrawImage(s.pipImg, op)
}

// drawDramaticEnding renders the finish as a full-screen result: a clean
// winner banner, then the last seconds of both screens side by side, centered
// and fully visible.
func (s *gameSession) drawDramaticEnding(screen *ebiten.Image, r *race) {
	drawBG(screen)

	player := s.app.playerName()
	name := player
	cc := colPlayer
	if r.Winner() == 2 {
		name = "RIVAL"
		cc = colOpponent
	}
	t := float64(r.WinnerFinishFrames()) / 60.0

	// Winner banner: centered, deliberate.
	psTextC(screen, name+" GAGNE", 480, 56, 30, cc)
	w := psWidth(name+" GAGNE", 30)
	fillRect(screen, int(480-w/2)-30, 56+30+14, int(w)+60, 2, cc)
	psTextC(screen, fmt.Sprintf("%.2f s", t), 480, 56+30+30, 12, colTextDim)

	// Caption above the screens.
	psTextC(screen, "LES DERNIERES SECONDES", 480, 150, 11, colTextDim)

	// Two replay screens, centered vertically (no cropping).
	s.renderReplaySide(screen, r, r.replayPlayer, 0, player, s.emu.Width(), s.emu.Height())
	s.renderReplaySide(screen, r, r.replayOpp, 1, "RIVAL", s.emu.Width(), s.emu.Height())

	// Actions.
	psTextC(screen, "R  REJOUER      E  EXPORTER      ESC  MENU", 480, 690, 10, colTextDim)
	if s.replayMsg != "" {
		psTextC(screen, s.replayMsg, 480, 672, 9, colAccent2)
	}
}

// renderReplaySide draws one side of the final replay in a clean framed panel.
func (s *gameSession) renderReplaySide(screen *ebiten.Image, r *race, ring *frameRing, side int, label string, w, h int) {
	if ring == nil || w <= 0 || h <= 0 {
		return
	}
	img := s.replayImg(side, w, h)
	if f := ring.Frame(r.ReplayIndex()); f != nil {
		img.WritePixels(f)
	}
	const vw, vh = 430, 330
	const topY = 176.0
	baseX := 40
	if side == 1 {
		baseX = 490
	}
	cc := colPlayer
	if side == 1 {
		cc = colOpponent
	}
	psTextC(screen, label, float64(baseX)+vw/2, topY-24, 11, cc)

	iw, ih := float64(w), float64(h)
	scale := math.Min(float64(vw)/iw, float64(vh)/ih)
	dw, dh := iw*scale, ih*scale
	bx := float64(baseX) + (float64(vw)-dw)/2
	by := topY + (float64(vh)-dh)/2

	// Thin frame.
	fillRect(screen, int(bx)-2, int(by)-2, int(dw)+4, int(dh)+4, color.RGBA{0xff, 0xff, 0xff, 0x18})

	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterNearest
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(bx, by)
	screen.DrawImage(img, op)
}

// replayImg returns (creating on first use) the reused replay image for a side.
func (s *gameSession) replayImg(side, w, h int) *ebiten.Image {
	if s.replayImgs[side] == nil {
		s.replayImgs[side] = ebiten.NewImage(w, h)
		return s.replayImgs[side]
	}
	if s.replayImgs[side].Bounds().Dx() != w || s.replayImgs[side].Bounds().Dy() != h {
		s.replayImgs[side] = ebiten.NewImage(w, h)
	}
	return s.replayImgs[side]
}
