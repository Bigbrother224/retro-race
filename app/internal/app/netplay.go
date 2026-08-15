package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image/color"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"retrorace/internal/engine"
	"retrorace/internal/netplay"
	"retrorace/internal/rollback"
	"retrorace/internal/shm"
)

// errNetQuit is returned from Update to stop the shared-game window cleanly.
var errNetQuit = errors.New("netplay quit")

// NetplayConfig describes a shared-game session: one machine hosts (player 1),
// the other joins (player 2). Both run the same ROM + core locally; only input
// packets cross the network.
type NetplayConfig struct {
	Host  string // direct: listen address when hosting, e.g. ":9330"
	Join  string // direct: host:port to join, e.g. "192.168.1.10:9330"
	Relay string // relay: server address, e.g. "relay.retrorace.app:9330" (no NAT)
	Code  string // relay: room code shared with the friend
	ROM   string // path to the ROM (both sides have it locally)
	Core  string // path to the core dylib (both sides have it locally)
	Game  string // display name for the HUD
}

// fileSHA256 hashes a file's bytes, used to prove both sides run the same ROM
// and core during the netplay handshake (no content is ever transferred).
func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// RunNetplay starts the shared-game window. It hosts or joins, then runs the
// same game deterministically on both machines, exchanging only controller
// inputs per frame.
func RunNetplay(cfg NetplayConfig) error {
	romID, err := fileSHA256(cfg.ROM)
	if err != nil {
		return fmt.Errorf("rom: %w", err)
	}
	coreID, err := fileSHA256(cfg.Core)
	if err != nil {
		return fmt.Errorf("core: %w", err)
	}
	gameID := romID + coreID

	var s *netplay.Session
	switch {
	case cfg.Relay != "":
		// Relay mode: both players connect out (no NAT); role is decided by
		// the relay (first member = host). No game handshake until the friend
		// joins the room.
		s, err = netplay.Relay(cfg.Relay, cfg.Code, gameID)
		if err != nil {
			return fmt.Errorf("netplay relay: %w", err)
		}
	case cfg.Host != "":
		ln, err := net.Listen("tcp", cfg.Host)
		if err != nil {
			return fmt.Errorf("listen %s: %w", cfg.Host, err)
		}
		s, err = netplay.Host(ln, gameID)
		if err != nil {
			return fmt.Errorf("netplay host: %w", err)
		}
	default:
		s, err = netplay.Join(cfg.Join, gameID)
		if err != nil {
			return fmt.Errorf("netplay join: %w", err)
		}
	}
	return runNetplayGame(s, cfg, 0, "", nil)
}

// runNetplayGame runs the shared-game window for an already-established
// session, stepping the local core with both players' inputs each frame.
// latency, when > 0, simulates network round-trip on the session (debug).
// shmPath/cons, when non-nil, come from a fake opponent whose framebuffer is
// shown in a right-side PiP panel.
func runNetplayGame(s *netplay.Session, cfg NetplayConfig, latency time.Duration, shmPath string, cons *shm.Consumer) error {
	core := engine.NewCore()
	if err := core.Start(cfg.ROM, cfg.Core); err != nil {
		return err
	}
	role := "join"
	if s.Role() == netplay.RoleHost {
		role = "host"
	}
	log.Printf("netplay: game=%s role=%s players=%d", cfg.Game, role, core.ControllerPlayers())
	if latency > 0 {
		s.SetLatency(latency)
	}

	a := &NetplayApp{session: s, emu: core, game: cfg.Game, players: core.ControllerPlayers(), shmPath: shmPath, rivalCons: cons}
	// Rollback/prediction only pays off under real (or simulated) latency: it
	// re-simulates with corrected inputs. On a 0-RTT local test it just adds a
	// per-frame state save that costs fps, so we enable it only with latency.
	if latency > 0 {
		if _, err := core.Save(); err == nil {
			a.rb = rollback.New(rollbackCore{core}, int(netPredictMax)+2)
			s.SetPredict(true, netPredictMax)
		}
		s.SetAutoDelay(true, time.Second/60)
	} else if shmPath != "" {
		// Local fake-opponent test: RTT≈0, so use the minimum input delay for a
		// responsive, crisp feel (no need to adapt to network latency).
		s.SetDelay(1)
	} else {
		s.SetAutoDelay(true, time.Second/60)
	}
	ebiten.SetWindowSize(960, 720)
	ebiten.SetWindowTitle("Retro Race — partie partagée")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if err := ebiten.RunGame(a); err != nil && !errors.Is(err, errNetQuit) {
		return err
	}
	return nil
}

// NetplayApp is an Ebitengine game that runs a shared instance with a remote
// peer: each frame it exchanges the local controller input, applies both
// players' inputs to the two controller ports and steps the core once. Both
// machines render the same game locally.
type NetplayApp struct {
	session *netplay.Session
	emu     engine.Emulator
	game    string
	players int // controller ports the core exposes (-1 unknown, 1 solo)

	img    *ebiten.Image
	ended  bool
	errMsg string

	// Divergence check: every checkEvery rendered frames both machines hash
	// their game state and compare, so a random game with a non-deterministic
	// core cannot drift apart silently.
	rendered  int
	lastCheck int

	rb          *rollback.Session // predictive rollback re-simulation state
	corrections int               // rollback corrections applied

	// Fake-opponent PiP: the opponent's live framebuffer (shared memory).
	shmPath   string
	rivalCons *shm.Consumer
	pipImg    *ebiten.Image

	// Visible proof of a real second player: the peer's latest input, and
	// whether the last divergence check confirmed both machines in sync.
	peerInput [netplay.ButtonCount]bool
	syncOK    bool
}

// stateHasher is implemented by emulators that can hash their game state for
// netplay divergence detection (the real libretro Core; FakeCore does not).
type stateHasher interface {
	StateHash() uint64
}

const netplayCheckEvery = 60 // ~1 s at 60 fps

// Update implements ebiten.Game.
func (a *NetplayApp) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.closeOpponent()
		a.session.Close()
		return errNetQuit
	}
	if a.ended {
		return nil
	}
	a.consumeOpponent()

	local := netReadLocalButtons()
	if !a.session.Send(local) {
		a.setEnded()
		return nil
	}

	// Render as many frames as are ready (usually one per Send; more right
	// after a stall). Each frame applies the merged inputs (controller 1 =
	// host, controller 2 = guest) identically on both machines.
	rendered := 0
	for {
		my, peer, advanced, ok := a.session.RenderNext()
		if !ok {
			a.setEnded()
			return nil
		}
		if !advanced {
			break
		}
		var p1, p2 [netplay.ButtonCount]bool
		if a.session.LocalPort() == 1 {
			p1, p2 = my, peer
		} else {
			p1, p2 = peer, my
		}
		a.peerInput = peer // the peer's real input for this frame (proof of a 2nd player)
		if a.rb != nil {
			// rollback.Commit applies inputs, steps once and records the state.
			a.rb.Commit(mask16(p1, p2))
		} else {
			for b := 0; b < netplay.ButtonCount; b++ {
				a.emu.SetButtonPort(0, engine.JoyButton(b), p1[b])
				a.emu.SetButtonPort(1, engine.JoyButton(b), p2[b])
			}
			a.emu.Step()
		}
		a.rendered++
		if a.rb != nil {
			if !a.drainCorrections() {
				return nil
			}
		}
		if a.checkDivergence() {
			return nil
		}
		rendered++
		if rendered >= 4 {
			break
		}
	}
	return nil
}

// consumeOpponent reads the fake opponent's latest published frame (shared
// memory) into the PiP image.
func (a *NetplayApp) consumeOpponent() {
	if a.rivalCons == nil {
		return
	}
	snap := a.rivalCons.Take()
	if snap == nil || len(snap.Frame) == 0 || snap.Width <= 0 || snap.Height <= 0 {
		return
	}
	if a.pipImg == nil || a.pipImg.Bounds().Dx() != snap.Width || a.pipImg.Bounds().Dy() != snap.Height {
		a.pipImg = ebiten.NewImage(snap.Width, snap.Height)
	}
	a.pipImg.WritePixels(snap.Frame)
}

// closeOpponent releases the PiP shared memory and removes the region file.
func (a *NetplayApp) closeOpponent() {
	if a.rivalCons != nil {
		a.rivalCons.Close()
		a.rivalCons = nil
	}
	if a.shmPath != "" {
		os.Remove(a.shmPath)
		a.shmPath = ""
	}
}

// drainCorrections applies any peer-input predictions that turned out wrong,
// re-simulating the affected frames via rollback. Returns false if the session
// must stop.
func (a *NetplayApp) drainCorrections() bool {
	for a.rb != nil {
		frame, realPeer, ok := a.session.TakeCorrection()
		if !ok {
			return true
		}
		localPort := a.session.LocalPort()
		last := a.rb.Last()
		var ins []rollback.Input
		for f := int(frame); f <= last; f++ {
			in, ok := a.rb.Input(f)
			if !ok {
				a.setEnded()
				a.errMsg = "rollback: frame hors fenetre"
				return false
			}
			if f == int(frame) {
				// realPeer is the peer's input; put it in the peer's slot (P1
				// when we are player 2, P2 when we are player 1).
				if localPort == 1 {
					in.P2 = realPeer
				} else {
					in.P1 = realPeer
				}
			}
			ins = append(ins, in)
		}
		if err := a.rb.Correct(int(frame), ins); err != nil {
			a.setEnded()
			a.errMsg = "rollback: " + err.Error()
			return false
		}
		a.corrections++
	}
	return true
}

// setEnded marks the session over and records a human-readable error.
func (a *NetplayApp) setEnded() {
	a.ended = true
	if a.session != nil && a.session.Err() != nil {
		a.errMsg = "Connexion perdue : " + a.session.Err().Error()
	} else {
		a.errMsg = "Connexion perdue"
	}
}

// checkDivergence periodically hashes this machine's game state, sends it to
// the peer and compares with the peer's latest hash. It returns true (ending
// the session with an error) when the states differ, meaning the two machines
// are no longer running the same game (a non-deterministic core, for example).
// It is a no-op until enough frames pass and a peer hash has been received.
func (a *NetplayApp) checkDivergence() bool {
	if a.rendered < a.lastCheck+netplayCheckEvery {
		return false
	}
	h, ok := a.emu.(stateHasher)
	if !ok {
		return false
	}
	myHash := h.StateHash()
	if myHash == 0 {
		return false // core does not support save states; cannot verify
	}
	a.lastCheck = a.rendered
	a.session.SendStateHash(myHash)

	myFrame := a.session.RenderFrame()
	if pf, ph, have := a.session.PeerHash(); have {
		// Only compare when the peer's hash is for a frame near ours (within
		// the input-delay window plus slack); otherwise the frames genuinely
		// differ and so do the hashes.
		if pf >= myFrame && pf-myFrame <= a.session.InputDelay()+8 {
			if ph != myHash {
				a.ended = true
				a.errMsg = "DIVERGENCE : les deux machines ne sont plus synchronisees"
				return true
			}
			a.syncOK = true // peer hash matched → both machines running the same game
		}
	}
	return false
}

// Draw implements ebiten.Game.
func (a *NetplayApp) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0x0d, 0x0a, 0x12, 0xff})

	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	// Fake-opponent mode reserves a right column for the adversary's PiP.
	gameW := sw
	showPiP := a.rivalCons != nil
	if showPiP {
		gameW = sw - racePanelW
	}

	frame := a.emu.Frame()
	w, h := a.emu.Width(), a.emu.Height()
	if frame != nil && w > 0 && h > 0 {
		if a.img == nil || a.img.Bounds().Dx() != w || a.img.Bounds().Dy() != h {
			a.img = ebiten.NewImage(w, h)
		}
		a.img.WritePixels(frame)
		scale := min(float64(gameW)/float64(w), float64(sh)/float64(h))
		dw, dh := float64(w)*scale, float64(h)*scale
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate((float64(gameW)-dw)/2, (float64(sh)-dh)/2)
		op.Filter = ebiten.FilterNearest
		screen.DrawImage(a.img, op)
	} else {
		ebitenutil.DebugPrint(screen, "chargement en cours...")
	}

	role := "JOUEUR 1  (HOTE)"
	cc := colPlayer
	if a.session.LocalPort() == 2 {
		role = "JOUEUR 2  (INVITE)"
		cc = colOpponent
	}

	if showPiP {
		a.drawOpponentPanel(screen, role, cc)
	} else {
		// Friend/real shared game: full-screen game + a compact HUD.
		psText(screen, role, 16, 14, 12, cc)
		if a.game != "" {
			psText(screen, a.game, 16, 40, 9, colTextDim)
		}
		switch {
		case a.players >= 2:
			psText(screen, fmt.Sprintf("%d manettes (materiel) — le 2e perso agit ?", a.players), 16, 62, 9, colTextDim)
		case a.players == 1:
			psText(screen, "Jeu solo : pas de 2e personnage", 16, 62, 9, colAccent2)
		case a.players == 0:
			psText(screen, "Aucun controller expose par le core", 16, 62, 9, colTextDim)
		default:
			psText(screen, "Compat 2 joueurs : inconnue", 16, 62, 9, colTextDim)
		}
		if st := a.session.Stats(); st.Rendered > 0 {
			psText(screen, fmt.Sprintf("frames %d   buffer %d   stalls %d   delay %d",
				st.Rendered, st.Buffered, st.Stalled, a.session.InputDelay()), 16, 86, 9, colTextDim)
		}
		psText(screen, "ESC  QUITTER", 16, float64(sh)-24, 9, colTextDim)
	}

	if a.ended {
		fillRect(screen, 0, 0, sw, sh, color.RGBA{0, 0, 0, 0x80})
		msg := a.errMsg
		if msg == "" {
			msg = "Partie terminée"
		}
		psTextC(screen, msg, 480, 356, 20, colText)
	}
}

// joyButtonLabels maps a logical SNES button index to a short label, used to
// show what the adversary is pressing in real time.
var joyButtonLabels = [12]string{
	"Haut", "Bas", "Gauche", "Droite",
	"Tirer(A)", "Sauter(B)", "X", "Y",
	"L", "R", "Select", "Start",
}

// peerInputSummary renders the peer's current input as a short list, e.g.
// "Droite, Tirer(A)" — visible proof that a real second player is acting.
func peerInputSummary(in [netplay.ButtonCount]bool) string {
	var parts []string
	for i := 0; i < len(in) && i < 12; i++ {
		if in[i] {
			parts = append(parts, joyButtonLabels[i])
		}
	}
	if len(parts) == 0 {
		return "(rien)"
	}
	return strings.Join(parts, ", ")
}

// drawOpponentPanel renders the fake opponent's live screen and the netplay
// telemetry in the reserved right column (the racing idiom: your game stays
// untouched on the left).
func (a *NetplayApp) drawOpponentPanel(screen *ebiten.Image, role string, cc color.RGBA) {
	x := 960 - racePanelW
	w := racePanelW

	fillRect(screen, x, 0, w, 720, color.RGBA{0x0a, 0x0a, 0x10, 0xff})
	fillRect(screen, x, 0, 2, 720, color.RGBA{0xff, 0xff, 0xff, 0x0d})

	psTextC(screen, "ADVERSAIRE", float64(x)+float64(w)/2, 20, 12, colTextDim)
	fillRect(screen, x+16, 44, w-32, 2, colBorder)

	// Opponent's live screen, from shared memory.
	const viewW, viewH = 204, 152
	if a.pipImg != nil {
		iw, ih := float64(a.pipImg.Bounds().Dx()), float64(a.pipImg.Bounds().Dy())
		scale := math.Min(viewW/iw, viewH/ih)
		dw, dh := iw*scale, ih*scale
		bx := float64(x) + (float64(w)-viewW)/2
		by := 76.0
		fillRect(screen, int(bx)-2, int(by)-2, int(viewW)+4, int(viewH)+4, colOpponent)
		psTextC(screen, "FAUX JOUEUR  (JOUEUR 2)", bx+viewW/2, by-22, 9, colOpponent)
		op := &ebiten.DrawImageOptions{}
		op.Filter = ebiten.FilterNearest
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(bx+(viewW-dw)/2, by+(viewH-dh)/2)
		screen.DrawImage(a.pipImg, op)
	}

	// You: role line.
	psTextC(screen, role, float64(x)+float64(w)/2, 256, 10, cc)

	// Network telemetry.
	gy := 290.0
	psText(screen, "RESEAU", float64(x)+16, gy, 9, colTextDim)
	if st := a.session.Stats(); st.Rendered > 0 {
		lines := []string{
			fmt.Sprintf("frames  %d", st.Rendered),
			fmt.Sprintf("buffer  %d", st.Buffered),
			fmt.Sprintf("stalls  %d", st.Stalled),
			fmt.Sprintf("delay   %d", a.session.InputDelay()),
			fmt.Sprintf("corr    %d", a.corrections),
		}
		for i, l := range lines {
			psText(screen, l, float64(x)+16, gy+16+float64(i)*16, 9, colTextDim)
		}
	}

	// Visible proof of a real second player: the adversary's current input.
	py := gy + 16 + 5*16 + 4
	psText(screen, "ADVERSAIRE APPUIE", float64(x)+16, py, 9, colTextDim)
	psText(screen, peerInputSummary(a.peerInput), float64(x)+16, py+16, 9, colText)

	// Sync status (real, from the periodic state-hash divergence check).
	sy := py + 40
	syncLabel := "SYNC : en attente..."
	syncCol := colTextDim
	if a.syncOK {
		syncLabel = "SYNC OK"
		syncCol = colPlayer
	}
	psText(screen, syncLabel, float64(x)+16, sy, 10, syncCol)

	// Controls hint so you always know what to press.
	cy := sy + 30
	psText(screen, "COMMANDES", float64(x)+16, cy, 9, colTextDim)
	cmds := []string{
		"FLECHES   bouger",
		"X         sauter (B)",
		"Z         tirer (A)",
		"ENTREE    demarrer",
	}
	for i, c := range cmds {
		psText(screen, c, float64(x)+16, cy+16+float64(i)*16, 9, colText)
	}

	psTextC(screen, "ESC  QUITTER", float64(x)+float64(w)/2, 700, 9, colTextDim)
}

// Layout implements ebiten.Game.
func (a *NetplayApp) Layout(outsideW, outsideH int) (int, int) {
	return 960, 720
}

// RunFakeOpponent is a local test mode: you play player 1 (host) against a
// scripted "fake player" running as a second process, through the real netplay
// session + rollback stack — the same path a friend uses over Tailscale, minus
// the network. It lets you feel the shared-game experience before the real test.
func RunFakeOpponent(cfg NetplayConfig, latency time.Duration) error {
	romID, err := fileSHA256(cfg.ROM)
	if err != nil {
		return fmt.Errorf("rom: %w", err)
	}
	coreID, err := fileSHA256(cfg.Core)
	if err != nil {
		return fmt.Errorf("core: %w", err)
	}
	gameID := romID + coreID

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	join := ln.Addr().String()

	// The fake opponent is a headless second emulator process (the libretro
	// shim is single-instance per process, so player 2 cannot share ours).
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	latMs := 0
	if latency > 0 {
		latMs = int(latency / time.Millisecond)
	}
	shmPath := fmt.Sprintf("/tmp/retro_race_netbot_%d.shm", os.Getpid())
	os.Remove(shmPath)
	bot := exec.Command(exe, "--netbot",
		"--netrom", cfg.ROM, "--netcore", cfg.Core,
		"--netjoin", join, "--gameid", gameID,
		"--shm", shmPath,
		"--fake-latency", fmt.Sprint(latMs))
	bot.Stdout = os.Stdout
	bot.Stderr = os.Stderr
	if err := bot.Start(); err != nil {
		return fmt.Errorf("fake opponent start: %w", err)
	}
	defer func() {
		if bot.Process != nil {
			_ = bot.Process.Kill()
		}
	}()

	s, err := netplay.Host(ln, gameID)
	if err != nil {
		return fmt.Errorf("netplay host: %w", err)
	}

	// Wait for the bot to create the shm region (the NES core loads in <1 s).
	var cons *shm.Consumer
	for i := 0; i < 200; i++ {
		if c, err := shm.OpenConsumer(shmPath); err == nil {
			cons = c
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if cons == nil {
		log.Printf("fakeopp: shm consumer open failed")
	}
	return runNetplayGame(s, cfg, latency, shmPath, cons)
}

// NetBotConfig describes the headless fake-opponent (player 2) process.
type NetBotConfig struct {
	ROM     string
	Core    string
	Join    string // address of the host to join
	GameID  string // must match the host's ROM+core hash
	ShmPath string // optional: publish the bot's framebuffer here (PiP panel)
	Latency time.Duration
}

// RunNetBot runs a headless fake opponent: it joins the host over a real
// netplay session, drives its own core with a scripted bot, and keeps both in
// sync via rollback until the host quits. It is the "player 2" spawned by
// RunFakeOpponent (and usable standalone against a remote host for testing).
func RunNetBot(cfg NetBotConfig) error {
	c := engine.NewCore()
	if err := c.Start(cfg.ROM, cfg.Core); err != nil {
		return fmt.Errorf("core: %w", err)
	}
	defer c.Stop()

	s, err := netplay.Join(cfg.Join, cfg.GameID)
	if err != nil {
		return fmt.Errorf("netplay join: %w", err)
	}
	defer s.Close()

	// Rollback/prediction only under latency (see runNetplayGame): a 0-RTT bot
	// needs no re-simulation, and skipping it avoids the per-frame state save.
	var rb *rollback.Session
	if cfg.Latency > 0 {
		rb = rollback.New(rollbackCore{c}, int(netPredictMax)+2)
		s.SetPredict(true, netPredictMax)
		s.SetAutoDelay(true, time.Second/60)
		s.SetLatency(cfg.Latency)
	} else if cfg.ShmPath != "" {
		s.SetDelay(1) // local test: minimum input delay for responsiveness
	} else {
		s.SetAutoDelay(true, time.Second/60)
	}

	// Publish the opponent's live framebuffer to shared memory so the host can
	// show it in the PiP panel.
	var prod *shm.Producer
	if cfg.ShmPath != "" {
		os.Remove(cfg.ShmPath)
		if p, err := shm.OpenProducer(cfg.ShmPath); err == nil {
			prod = p
			defer prod.Close()
		} else {
			log.Printf("netbot: shm producer: %v", err)
		}
	}

	role := "guest"
	if s.Role() == netplay.RoleHost {
		role = "host"
	}
	log.Printf("netbot: joined %s game=%s role=%s", cfg.Join, cfg.GameID, role)
	interval := time.Second / 60 // 60 fps, like a real player
	sendCount := 0
	rendered := 0
	for {
		start := time.Now()
		if !s.Send(fakeBotInput(sendCount)) {
			return nil // host quit → clean exit
		}
		sendCount++
		for {
			my, peer, advanced, ok := s.RenderNext()
			if !ok {
				return nil
			}
			if !advanced {
				break
			}
			rendered++
			var p1, p2 [netplay.ButtonCount]bool
			if s.LocalPort() == 1 {
				p1, p2 = my, peer
			} else {
				p1, p2 = peer, my
			}
			if rb != nil {
				rb.Commit(mask16(p1, p2))
				for {
					fr, real, ok := s.TakeCorrection()
					if !ok {
						break
					}
					if err := botApplyCorrection(rb, int(fr), real, s.LocalPort()); err != nil {
						return err
					}
				}
			} else {
				for b := 0; b < netplay.ButtonCount; b++ {
					c.SetButtonPort(0, engine.JoyButton(b), p1[b])
					c.SetButtonPort(1, engine.JoyButton(b), p2[b])
				}
				c.Step()
			}
			if prod != nil {
				if fb := c.Frame(); len(fb) > 0 {
					prod.Write(fb, c.Width(), c.Height(), shm.StateRacing, 0, 60, 0)
				}
			}
			// Send our state hash so the host can verify both machines stay in
			// sync (feeds the SYNC indicator in the panel).
			if rendered%netplayCheckEvery == 0 {
				if sh := c.StateHash(); sh != 0 {
					s.SendStateHash(sh)
				}
			}
		}
		// Pace sends at the game's cadence (measure elapsed, sleep the rest) so
		// the host never waits on us and the game runs at full 60 fps.
		if d := time.Since(start); d < interval {
			time.Sleep(interval - d)
		}
	}
}

// fakeBotInput is a scripted controller-2 pattern for a co-op game like Contra:
// the fake player advances, keeps firing, and jumps periodically — you can tell
// the opponent (blue) apart from your own character (red).
func fakeBotInput(f int) [netplay.ButtonCount]bool {
	var b [netplay.ButtonCount]bool
	b[engine.BtnRight] = true // advance
	b[engine.BtnA] = true     // fire (Contra: A = shoot)
	if f%70 < 12 {
		b[engine.BtnB] = true // jump (Contra: B = jump)
	}
	return b
}

// botApplyCorrection re-simulates from the frame where the peer's real input
// differed from what was predicted, putting realPeer in the peer's controller
// slot (P1 when we are player 2, P2 when we are player 1).
func botApplyCorrection(rb *rollback.Session, frame int, realPeer uint16, localPort int) error {
	last := rb.Last()
	var ins []rollback.Input
	for f := frame; f <= last; f++ {
		in, ok := rb.Input(f)
		if !ok {
			return fmt.Errorf("frame %d out of window", f)
		}
		if f == frame {
			if localPort == 1 {
				in.P2 = realPeer
			} else {
				in.P1 = realPeer
			}
		}
		ins = append(ins, in)
	}
	return rb.Correct(frame, ins)
}
