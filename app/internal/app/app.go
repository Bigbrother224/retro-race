package app

import (
	"fmt"
	"image/color"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"retrorace/internal/arbiter"
	"retrorace/internal/engine"
	"retrorace/internal/library"
	"retrorace/internal/netplay"
	"retrorace/internal/replay"
	"retrorace/internal/rollback"
	"retrorace/internal/shm"
)

// RelayAddr is the input relay server both players connect to (no NAT). It is
// overridable via --relay; the default is a local dev relay.
var RelayAddr = "127.0.0.1:9330"

type state int

const (
	stateTitle state = iota
	stateProfile
	stateConsole
	stateGame
	stateNetLobby
	statePlaying
	stateControllerTest
)

// App is the Ebitengine game: arcade menu + game view.
type App struct {
	consoles []library.Console
	state    state
	selCon   int
	selGame  int
	frame    int
	carousel carousel

	// Game state
	emu     engine.Emulator
	running bool
	paused  bool
	img     *ebiten.Image

	// Profile: player name + chosen mode (solo play or race).
	username  string
	gameMode  int // 0 = solo, 1 = race
	profileAt int // frame when the profile screen was entered

	// Boxart cache (loaded once per game).
	boxarts map[boxartKey]*ebiten.Image

	// Race Arbiter detects a segment end by screen change.
	arbiter *arbiter.Arbiter

	// Local race state (item 5): real rival process, PiP, gauge, replay.
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

	// Replay (item 6): deterministic playback of a recorded run.
	replayPlayer *replay.Player
	replayMsg    string // export confirmation or error shown in dramatic ending

	// Input gate (2 players): routes each player's button state to an emulator
	// controller port. It is the single place that decides who drives which
	// port, so local (two gamepads) and remote (relayed inputs) share the same
	// path. In Joust mode port ownership swaps every joustEvery frames, so two
	// humans fight for control of the same character.
	players    [2][12]bool
	joust      bool
	joustEvery int
	// keyboardActive is latched once a mapped key is pressed during a race. It
	// tells the gate whether the keyboard is a live player (so keyboard +
	// one gamepad reads as two players). Reset at each race start.
	keyboardActive bool

	// Netplay shared-game mode (menu): lobby state + active session.
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

	errMsg string
}

func Run() error {
	consoles := library.New().Scan(romsDir)
	a := &App{consoles: consoles, state: stateTitle, boxarts: map[boxartKey]*ebiten.Image{}, arbiter: arbiter.New(arbiter.DefaultConfig())}
	a.username = loadProfile()
	ebiten.SetWindowSize(960, 720)
	ebiten.SetWindowTitle("Retro Race")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if err := ebiten.RunGame(a); err != nil {
		return err
	}
	return nil
}

func (a *App) Update() error {
	a.frame++
	switch a.state {
	case stateTitle:
		a.updateTitle()
	case stateProfile:
		a.updateProfile()
	case stateConsole:
		a.updateConsole()
	case stateGame:
		a.updateGameSelect()
	case stateNetLobby:
		a.updateNetLobby()
	case statePlaying:
		a.updatePlaying()
	case stateControllerTest:
		a.updateControllerTest()
	}
	return nil
}

func (a *App) Draw(screen *ebiten.Image) {
	switch a.state {
	case stateTitle:
		a.drawTitleScreen(screen, a.frame)
	case stateProfile:
		a.drawProfileScreen(screen)
	case stateConsole:
		a.drawConsoleScreen(screen)
	case stateGame:
		a.drawGameScreen(screen)
	case stateNetLobby:
		a.drawNetLobby(screen)
	case statePlaying:
		a.drawPlaying(screen)
	case stateControllerTest:
		a.drawControllerTest(screen)
	}
}

func (a *App) Layout(outsideW, outsideH int) (int, int) {
	return 960, 720
}

// ---- title ----

func (a *App) updateTitle() {
	if inpututil.IsKeyJustPressed(ebiten.KeyT) {
		a.state = stateControllerTest
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) ||
		inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		if len(a.consoles) == 0 {
			a.errMsg = "Aucun jeu détecté dans " + romsDir
			a.state = stateConsole
			return
		}
		a.state = stateProfile
		a.profileAt = a.frame
	}
}

// ---- controller test ----

func (a *App) updateControllerTest() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.state = stateTitle
	}
}

// controllerButtons lists the standard gamepad buttons with friendly labels so
// the controller test shows exactly what each button does.
var controllerButtons = []struct {
	btn   ebiten.StandardGamepadButton
	label string
}{
	{ebiten.StandardGamepadButtonLeftTop, "Haut"},
	{ebiten.StandardGamepadButtonLeftBottom, "Bas"},
	{ebiten.StandardGamepadButtonLeftLeft, "Gauche"},
	{ebiten.StandardGamepadButtonLeftRight, "Droite"},
	{ebiten.StandardGamepadButtonRightBottom, "B (bas/✕/A)"},
	{ebiten.StandardGamepadButtonRightRight, "A (dr/○/B)"},
	{ebiten.StandardGamepadButtonRightLeft, "Y (gau/□/X)"},
	{ebiten.StandardGamepadButtonRightTop, "X (haut/△/Y)"},
	{ebiten.StandardGamepadButtonCenterLeft, "Select"},
	{ebiten.StandardGamepadButtonCenterRight, "Start"},
	{ebiten.StandardGamepadButtonFrontTopLeft, "L1"},
	{ebiten.StandardGamepadButtonFrontTopRight, "R1"},
}

// drawControllerTest lists every detected gamepad and shows its buttons
// lighting up in real time — the visible proof that a controller is actually
// detected and read (Phase 1 step 1).
func (a *App) drawControllerTest(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0x0d, 0x0a, 0x12, 0xff})
	psTextC(screen, "TEST MANETTES", 480, 20, 16, colText)
	psTextC(screen, "ESC  RETOUR", 480, 40, 9, colTextDim)

	ids := ebiten.AppendGamepadIDs(nil)
	if len(ids) == 0 {
		psTextC(screen, "Aucune manette détectée. Branche-la puis appuie sur un bouton.", 480, 130, 12, colAccent2)
		psTextC(screen, "Clavier : fleches, Z=A, X=B, Entree=Start, Maj=Select, Q=L, E=R", 480, 156, 9, colTextDim)
		return
	}

	y := 72.0
	for gi, id := range ids {
		name := ebiten.GamepadName(id)
		sdl := ebiten.GamepadSDLID(id)
		layout := "BRUT"
		if ebiten.IsStandardGamepadLayoutAvailable(id) {
			layout = "STANDARD"
		}
		col := colPlayer
		if layout == "BRUT" {
			col = colAccent2
		}
		psText(screen, fmt.Sprintf("Manette %d : %s  [%s]  %s", gi+1, name, layout, sdl), 40, y, 11, col)

		// Buttons in 3 columns x 4 rows.
		bx, by := 40.0, y+26.0
		for i, b := range controllerButtons {
			on := ebiten.IsStandardGamepadButtonPressed(id, b.btn)
			c := colTextDim
			state := "OFF"
			if on {
				c = colText
				state = "ON "
			}
			colX := bx + float64(i%3)*260
			rowY := by + float64(i/3)*20
			psText(screen, fmt.Sprintf("%-18s %s", b.label, state), colX, rowY, 9, c)
		}

		// Analog axes.
		n := ebiten.GamepadAxisNum(id)
		var sb strings.Builder
		for ax := 0; ax < n && ax < 8; ax++ {
			v := ebiten.GamepadAxisValue(id, ebiten.GamepadAxisType(ax))
			sb.WriteString(fmt.Sprintf("axe%d=%.2f  ", ax, v))
		}
		if sb.Len() > 0 {
			psText(screen, sb.String(), 40, by+84, 9, colTextDim)
		}
		if !ebiten.IsStandardGamepadLayoutAvailable(id) {
			// Diagnostic: show which raw button indices are pressed so a
			// non-standard pad's mapping can be verified/tuned.
			var rw strings.Builder
			rw.WriteString("Bruts: ")
			n := ebiten.GamepadButtonNum(id)
			any := false
			for i := 0; i < n && i < 16; i++ {
				if ebiten.IsGamepadButtonPressed(id, ebiten.GamepadButton(i)) {
					fmt.Fprintf(&rw, "b%d ", i)
					any = true
				}
			}
			if !any {
				rw.WriteString("(aucun)")
			}
			psText(screen, rw.String(), 40, by+98, 9, colAccent2)
		}
		y = by + 116
	}
}

// ---- profile ----

const (
	modeSolo  = 0
	modeRace  = 1
	modeShare = 2
)

// updateProfile handles username entry and the solo/race mode choice.
func (a *App) updateProfile() {
	// Ignore Enter for a few frames after entering, so the keypress that left
	// the title screen does not immediately confirm the profile.
	if a.frame-a.profileAt < 4 {
		return
	}
	// Backspace removes the last character of the username.
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(a.username) > 0 {
		r := []rune(a.username)
		a.username = string(r[:len(r)-1])
	}
	// Typed characters append to the username (capped for layout).
	for _, ch := range ebiten.InputChars() {
		if len([]rune(a.username)) < 18 {
			a.username += string(ch)
		}
	}
	// Left/Right cycles the mode (solo / course / partage).
	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		a.gameMode = (a.gameMode + 2) % 3
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyRight) || inpututil.IsKeyJustPressed(ebiten.KeyD) {
		a.gameMode = (a.gameMode + 1) % 3
	}
	// Enter/Space confirms, persists the username, and moves to the launcher.
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		saveProfile(a.username)
		a.state = stateConsole
	}
}

// ---- console selection ----

func (a *App) updateConsole() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.state = stateTitle
		return
	}
	if len(a.consoles) == 0 {
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyRight) || inpututil.IsKeyJustPressed(ebiten.KeyD) {
		a.selCon = (a.selCon + 1) % len(a.consoles)
		a.carousel.target = a.selCon
		a.carousel.offset = -420 // start off-screen for a slide-in
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		a.selCon = (a.selCon - 1 + len(a.consoles)) % len(a.consoles)
		a.carousel.target = a.selCon
		a.carousel.offset = 420
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		a.selGame = 0
		a.state = stateGame
	}
}

// ---- game selection ----

func (a *App) updateGameSelect() {
	con := a.consoles[a.selCon]
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.state = stateConsole
		return
	}
	if len(con.Games) == 0 {
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) || inpututil.IsKeyJustPressed(ebiten.KeyS) {
		a.selGame = (a.selGame + 1) % len(con.Games)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) || inpututil.IsKeyJustPressed(ebiten.KeyW) {
		a.selGame = (a.selGame - 1 + len(con.Games)) % len(con.Games)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g := con.Games[a.selGame]
		if a.gameMode == modeShare {
			a.enterNetLobby(con, g)
			return
		}
		if err := a.launch(con, g); err != nil {
			a.errMsg = err.Error()
			return
		}
		a.state = statePlaying
		a.running = true
		if a.gameMode == modeRace {
			a.startRace()
		}
	}
}

// playerName returns the profile username, or "TOI" if none was entered.
func (a *App) playerName() string {
	if a.username != "" {
		return a.username
	}
	return "TOI"
}

// launch plays the game (solo by default; a race is started explicitly).
func (a *App) launch(con library.Console, g library.Game) error {
	corePath := filepath.Join(coresDir, con.Core)
	log.Printf("launching %s on %s (%s)", g.Name, con.Name, corePath)
	core := engine.NewCore()
	if err := core.Start(g.Path, corePath); err != nil {
		return err
	}
	a.emu = core
	a.running = true
	a.arbiter.Reset()
	a.race = nil // solo play by default; a race is started explicitly
	a.replayPlayer = nil
	return nil
}

// startRace launches a real rival: a headless process running its own core on
// the same ROM, publishing its framebuffer and race state over shared memory.
// This is the product's "one process per player" architecture — the rival is a
// genuine second emulator instance, not a delayed copy of the player's game.
func (a *App) startRace() {
	a.race = newRace(DefaultRaceConfig())
	a.joust = false
	a.joustEvery = 240 // 4 s at 60 fps
	a.keyboardActive = false
	a.pipImg = nil
	a.replayImgs = [2]*ebiten.Image{}
	a.rivalFinish = -1

	if a.emu == nil {
		return
	}
	con := a.consoles[a.selCon]
	g := con.Games[a.selGame]

	// Spawn the headless rival with the same ROM/core.
	self, err := os.Executable()
	if err != nil {
		log.Printf("rival: cannot find executable: %v", err)
		return
	}
	a.rivalShm = fmt.Sprintf("/tmp/retro_race_rival_%d.shm", os.Getpid())
	os.Remove(a.rivalShm)

	run := a.findRunFor(g)
	args := []string{"--rival", "--rom", g.Path, "--core", filepath.Join(coresDir, con.Core),
		"--shm", a.rivalShm, "--expected", fmt.Sprintf("%d", DefaultRaceConfig().ExpectedFrames)}
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
		a.rivalShm = ""
		return
	}
	a.rivalCmd = cmd
	a.rivalProc = cmd.Process

	// Wait for the rival to create the region, then map it (the SNES core can
	// take a couple of seconds to load).
	for i := 0; i < 200; i++ {
		if cons, err := shm.OpenConsumer(a.rivalShm); err == nil {
			a.rivalCons = cons
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if a.rivalCons == nil {
		log.Printf("rival: consumer open failed")
	}
}

// findRunFor returns the most recent recorded run for the game, if any.
func (a *App) findRunFor(g library.Game) string {
	dir := replaysDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	prefix := sanitizeFilename(g.Name) + "-"
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

func (a *App) updatePlaying() {
	if a.emu == nil {
		return
	}
	// Shared game over the network: exchange inputs with the peer.
	if a.netSess != nil {
		a.updateNetplayPlaying()
		return
	}
	// Deterministic replay playback: step the recorded run, no race logic.
	if a.replayPlayer != nil {
		if !a.replayPlayer.Step() {
			a.stopGame()
		}
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.stopGame()
		return
	}

	// Pause / resume (P). While paused the emulator is frozen.
	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		a.paused = !a.paused
	}
	if a.paused {
		// Reset while paused restarts the game from its initial state.
		if inpututil.IsKeyJustPressed(ebiten.KeyR) {
			a.emu.Reset()
			a.arbiter.Reset()
		}
		return
	}

	// Solo play: no race, no arbiter, just play (ESC to return to the menu).
	if a.race == nil {
		a.emu.Step()
		a.race = nil
		return
	}

	switch a.race.State() {
	case racePlaying:
		// Joust gate: J toggles control-swap between the two players.
		if inpututil.IsKeyJustPressed(ebiten.KeyJ) {
			a.joust = !a.joust
		}
		state := a.updateGameInput()
		a.race.RecordInput(state[:])
		a.emu.Step()
		if f := a.emu.Frame(); f != nil {
			a.race.AddPlayerFrame(f)
			// Real rival: read its live framebuffer + state from shared memory.
			a.consumeRival()
			if a.arbiter.Update(f) == arbiter.SegmentEnd {
				a.race.PlayerFinished()
			}
		}
	case raceSlowmo:
		// Slow motion: step the game at a reduced cadence.
		if a.race.Frame()%4 == 0 {
			a.emu.Step()
		}
	case raceReplay:
		// Dramatic ending: R to replay, E to export.
		if inpututil.IsKeyJustPressed(ebiten.KeyR) {
			a.startReplay()
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyE) {
			a.exportReplay()
		}
	case raceDone:
		// Persist this run so the next race replays it as the rival ("Beat
		// this Ghost").
		a.saveRun()
		a.stopGame()
		return
	}
	a.race.Tick()
}

// consumeRival reads the rival's latest published frame + state from shared
// memory: its framebuffer drives the PiP, and its real progress/finish drive
// the race.
func (a *App) consumeRival() {
	if a.rivalCons == nil {
		return
	}
	snap := a.rivalCons.Take()
	if snap == nil {
		return
	}

	// PiP: the rival's real framebuffer.
	w, h := snap.Width, snap.Height
	if len(snap.Frame) > 0 && w > 0 && h > 0 && w*h*4 == len(snap.Frame) && a.race != nil {
		if a.pipImg == nil || a.pipImg.Bounds().Dx() != w || a.pipImg.Bounds().Dy() != h {
			a.pipImg = ebiten.NewImage(w, h)
		}
		a.pipImg.WritePixels(snap.Frame)
		a.race.AddOppFrame(snap.Frame)
	}

	// Real finish: the rival's arbiter detected a segment end on ITS screen.
	if snap.State == shm.StateDone && a.rivalFinish < 0 {
		a.rivalFinish = int(snap.FinishFrame)
		a.race.OppFinished()
	}

	// Real progress feeds the gauge.
	a.race.SetOppProgress(float64(snap.Progress) / 1000.0)
}

// updateGameInput collects raw input from keyboard + gamepads into per-player
// states, routes them through the gate to the emulator controller ports, and
// returns the effective port-0 state (what actually drove the player's game)
// for the input recorder.
func (a *App) updateGameInput() [12]bool {
	a.updatePlayerInputs()
	a.applyGate()
	return a.players[a.portOwnerForFrame()[0]]
}

func (a *App) stopGame() {
	if a.emu != nil {
		a.emu.Stop()
		a.emu = nil
	}
	a.stopRival()
	a.running = false
	a.paused = false
	a.state = stateGame
	a.arbiter.Reset()
	a.race = nil
	a.pipImg = nil
	a.replayImgs = [2]*ebiten.Image{}
	a.replayPlayer = nil
	a.replayMsg = ""
}

// stopRival terminates the headless rival process and releases the shm region.
func (a *App) stopRival() {
	if a.rivalCons != nil {
		a.rivalCons.Close()
		a.rivalCons = nil
	}
	if a.rivalProc != nil {
		a.rivalProc.Kill()
		a.rivalProc.Wait()
		a.rivalProc = nil
	}
	a.rivalCmd = nil
	if a.rivalShm != "" {
		os.Remove(a.rivalShm)
		a.rivalShm = ""
	}
	a.rivalFinish = -1
}

// replayCore adapts an engine.Emulator to the replay.Core interface.
type replayCore struct{ e engine.Emulator }

func (c replayCore) SetButton(b int, pressed bool) { c.e.SetButton(engine.JoyButton(b), pressed) }
func (c replayCore) Step()                         { c.e.Step() }

// buildRun assembles a replay.Run from the current race and game metadata.
func (a *App) buildRun() *replay.Run {
	con := a.consoles[a.selCon]
	g := con.Games[a.selGame]
	return &replay.Run{
		Game:    g.Name,
		Console: con.ID,
		Core:    con.Core,
		ROMHash: g.SHA256,
		ROMPath: g.Path,
		Width:   a.emu.Width(),
		Height:  a.emu.Height(),
		Frames:  a.race.InputDuration(),
		Events:  a.race.RunEvents(),
	}
}

// startReplay stops the current core, starts a fresh one with the recorded
// game, and begins deterministic playback.
func (a *App) startReplay() {
	run := a.buildRun()
	if len(run.Events) == 0 {
		a.replayMsg = "Rien à rejouer (aucun input enregistré)"
		return
	}
	if a.emu != nil {
		a.emu.Stop()
	}
	core := engine.NewCore()
	if err := core.Start(run.ROMPath, run.Core); err != nil {
		a.errMsg = "replay: " + err.Error()
		a.emu = nil
		a.stopGame()
		return
	}
	a.emu = core
	a.replayPlayer = replay.NewPlayer(run, replayCore{core})
	a.race = nil
	a.pipImg = nil
	a.replayImgs = [2]*ebiten.Image{}
	a.arbiter.Reset()
}

// exportReplay serialises the recorded run to a JSON file in app/Replays/.
// saveRun writes the recorded run to app/Replays/ so a later race can replay
// it as the rival ("Beat this Ghost"). Returns the written filename or "".
func (a *App) saveRun() string {
	run := a.buildRun()
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
func (a *App) exportReplay() {
	if name := a.saveRun(); name != "" {
		a.replayMsg = "Replay exporté : " + name
	} else {
		a.replayMsg = "Rien à exporter (aucun input enregistré)"
	}
}

// racePanelW is the width reserved for the race dashboard on the right of the
// game. The game keeps its native aspect and full pixel quality in the space
// left over; nothing is ever drawn on top of it.
const racePanelW = 232

// drawPlaying renders the emulated game in the left part of the window at its
// native aspect (pixel-perfect, never covered) and, during a race, a clean
// dashboard on the right showing the rival's screen and the progress gauge —
// the racing-game idiom: important info has its own space.
func (a *App) drawPlaying(screen *ebiten.Image) {
	if a.errMsg != "" {
		ebitenutil.DebugPrint(screen, a.errMsg)
		return
	}
	frame := a.emu.Frame()
	w, h := a.emu.Width(), a.emu.Height()
	if frame == nil || w == 0 || h == 0 {
		ebitenutil.DebugPrint(screen, "chargement en cours…")
		return
	}
	if a.img == nil || a.img.Bounds().Dx() != w || a.img.Bounds().Dy() != h {
		a.img = ebiten.NewImage(w, h)
	}
	a.img.WritePixels(frame)

	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	// In a race the game occupies the left area; in replay it uses the whole
	// window.
	gameW := sw
	if a.race != nil && a.replayPlayer == nil {
		gameW = sw - racePanelW
	}
	scale := min(float64(gameW)/float64(w), float64(sh)/float64(h))
	dispW, dispH := float64(w)*scale, float64(h)*scale
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate((float64(gameW)-dispW)/2, (float64(sh)-dispH)/2)
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(a.img, op)

	// Shared game over the network: full-screen game + a small HUD.
	if a.netSess != nil {
		a.drawNetplayHUD(screen)
		if a.netErr != "" {
			fillRect(screen, 0, 0, screen.Bounds().Dx(), screen.Bounds().Dy(), color.RGBA{0, 0, 0, 0x80})
			psTextC(screen, strings.ToUpper(a.netErr), 480, 356, 16, colAccent)
		}
		drawScanlines(screen)
		return
	}

	// Deterministic replay: game image + label, no race overlays.
	if a.replayPlayer != nil {
		drawText(screen, "REPLAY — Échap pour quitter", 20, 16, 1, colAccent2)
		drawScanlines(screen)
		return
	}

	// Race dashboard on the right (the game is never covered).
	if a.race != nil {
		switch a.race.State() {
		case racePlaying, raceSlowmo:
			a.drawRacePanel(screen, a.race)
			if a.race.State() == raceSlowmo {
				// Centered on the game area, not the whole window.
				gcx := gameW / 2
				drawPanel(screen, gcx-150, 300, 300, 40, colPanelHi, colAccent)
				drawText(screen, "FIN DE SEGMENT — RALENTI", gcx-122, 310, 1, colAccent2)
			}
		case raceReplay:
			a.drawDramaticEnding(screen, a.race)
		}
	}

	// Pause overlay: frozen game + controls hint.
	if a.paused {
		fillRect(screen, 0, 0, screen.Bounds().Dx(), screen.Bounds().Dy(), color.RGBA{0, 0, 0, 0x60})
		drawPanel(screen, 300, 260, 360, 90, colPanelHi, colAccent)
		psTextC(screen, "PAUSE", 480, 282, 24, colText)
		psTextC(screen, "R  RESET      P  REPRENDRE      ESC  QUITTER", 480, 322, 11, colTextDim)
	}

	// Quiet exit hint, bottom-left of the game area.
	psText(screen, "ESC  MENU", 16, 700, 9, colTextDim)

	drawScanlines(screen)
}
