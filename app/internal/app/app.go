package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"retrorace/internal/arbiter"
	"retrorace/internal/engine"
	"retrorace/internal/library"
	"retrorace/internal/replay"
)

const (
	coresDir = "/Users/mac/retro-race/cores"
	romsDir  = "/Users/mac/retro-race/app/Roms"
)

type state int

const (
	stateTitle state = iota
	stateConsole
	stateGame
	statePlaying
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
	img     *ebiten.Image

	// Boxart cache (loaded once per game).
	boxarts map[boxartKey]*ebiten.Image

	// Race Arbiter detects a segment end by screen change.
	arbiter *arbiter.Arbiter

	// Local race state (item 5): simulated opponent, PiP, gauge, replay.
	race       *race
	emu2       engine.Emulator
	pipImg     *ebiten.Image
	pipTick    int
	replayImgs [2]*ebiten.Image

	// Replay (item 6): deterministic playback of a recorded run.
	replayPlayer *replay.Player
	replayMsg    string // export confirmation or error shown in dramatic ending

	errMsg string
}

func Run() error {
	consoles := library.New().Scan(romsDir)
	a := &App{consoles: consoles, state: stateTitle, boxarts: map[boxartKey]*ebiten.Image{}, arbiter: arbiter.New(arbiter.DefaultConfig())}
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
	case stateConsole:
		a.updateConsole()
	case stateGame:
		a.updateGameSelect()
	case statePlaying:
		a.updatePlaying()
	}
	return nil
}

func (a *App) Draw(screen *ebiten.Image) {
	switch a.state {
	case stateTitle:
		a.drawTitleScreen(screen, a.frame)
	case stateConsole:
		a.drawConsoleScreen(screen)
	case stateGame:
		a.drawGameScreen(screen)
	case statePlaying:
		a.drawPlaying(screen)
	}
}

func (a *App) Layout(outsideW, outsideH int) (int, int) {
	return 960, 720
}

// ---- title ----

func (a *App) updateTitle() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) ||
		inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		if len(a.consoles) == 0 {
			a.errMsg = "Aucun jeu détecté dans " + romsDir
			a.state = stateConsole
			return
		}
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
		if err := a.launch(con, g); err != nil {
			a.errMsg = err.Error()
			return
		}
		a.state = statePlaying
		a.running = true
	}
}

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
	a.startRace()
	return nil
}

// startRace initializes the local race: the simulated opponent instance and
// the pure race state machine. The opponent is a FakeCore (Go-only, so it is
// safe to run alongside the real core; the C shim is single-instance).
func (a *App) startRace() {
	a.race = newRace(DefaultRaceConfig())
	a.emu2 = engine.NewFakeCore(256, 224)
	_ = a.emu2.Start("", "")
	a.race.SetOppFrameSize(a.emu2.Width() * a.emu2.Height() * 4)
	a.pipImg = nil
	a.pipTick = 0
	a.replayImgs = [2]*ebiten.Image{}
}

// ---- playing ----

func (a *App) updatePlaying() {
	if a.emu == nil {
		return
	}
	// Deterministic replay playback: step the recorded run, no race logic.
	if a.replayPlayer != nil {
		if !a.replayPlayer.Step() {
			a.stopGame()
		}
		return
	}
	if a.race == nil {
		a.startRace()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.stopGame()
		return
	}

	switch a.race.State() {
	case racePlaying:
		state := a.updateGameInput()
		a.race.RecordInput(state[:])
		a.emu.Step()
		if f := a.emu.Frame(); f != nil {
			a.race.AddPlayerFrame(f)
			if a.arbiter.Update(f) == arbiter.SegmentEnd {
				a.race.PlayerFinished()
			}
		}
		a.stepOpponent()
		if f := a.emu2.Frame(); f != nil {
			a.race.AddOppFrame(f)
			a.updatePiP(f)
		}
	case raceSlowmo:
		// Slow motion: step both instances at a reduced cadence.
		if a.race.Frame()%4 == 0 {
			a.emu.Step()
			a.stepOpponent()
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
		a.stopGame()
		return
	}
	a.race.Tick()
}

// updatePiP uploads the opponent framebuffer to the PiP image at low
// frequency (cheap live window).
func (a *App) updatePiP(frame []byte) {
	w, h := a.emu2.Width(), a.emu2.Height()
	if w == 0 || h == 0 {
		return
	}
	if a.pipImg == nil || a.pipImg.Bounds().Dx() != w || a.pipImg.Bounds().Dy() != h {
		a.pipImg = ebiten.NewImage(w, h)
	}
	a.pipTick++
	if a.pipTick%6 != 0 {
		return
	}
	a.pipImg.WritePixels(frame)
}
// updateGameInput maps keyboard keys to logical buttons (SNES-style) and
// returns the full button state for the input recorder.
func (a *App) updateGameInput() [12]bool {
	var state [12]bool
	keys := map[ebiten.Key]engine.JoyButton{
		ebiten.KeyArrowUp:    engine.BtnUp,
		ebiten.KeyArrowDown:  engine.BtnDown,
		ebiten.KeyArrowLeft:  engine.BtnLeft,
		ebiten.KeyArrowRight: engine.BtnRight,
		ebiten.KeyZ:          engine.BtnA,
		ebiten.KeyX:          engine.BtnB,
		ebiten.KeyA:          engine.BtnY,
		ebiten.KeyS:          engine.BtnX,
		ebiten.KeyEnter:      engine.BtnStart,
		ebiten.KeyShift:      engine.BtnSelect,
		ebiten.KeyQ:          engine.BtnL,
		ebiten.KeyE:          engine.BtnR,
	}
	for key, btn := range keys {
		pressed := inpututil.IsKeyJustPressed(key) || ebiten.IsKeyPressed(key)
		state[btn] = pressed
		a.emu.SetButton(btn, pressed)
	}
	a.updateGamepadInputInto(&state)
	return state
}

func (a *App) stopGame() {
	if a.emu != nil {
		a.emu.Stop()
		a.emu = nil
	}
	if a.emu2 != nil {
		a.emu2.Stop()
		a.emu2 = nil
	}
	a.running = false
	a.state = stateGame
	a.arbiter.Reset()
	a.race = nil
	a.pipImg = nil
	a.replayImgs = [2]*ebiten.Image{}
	a.replayPlayer = nil
	a.replayMsg = ""
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
func (a *App) exportReplay() {
	run := a.buildRun()
	if len(run.Events) == 0 {
		a.replayMsg = "Rien à exporter (aucun input enregistré)"
		return
	}
	data, err := replay.MarshalRun(run)
	if err != nil {
		a.replayMsg = "export: " + err.Error()
		return
	}
	dir := "/Users/mac/retro-race/app/Replays"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.replayMsg = "export: " + err.Error()
		return
	}
	name := sanitizeFilename(run.Game) + "-" + fmt.Sprintf("%d", time.Now().Unix()) + ".json"
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		a.replayMsg = "export: " + err.Error()
		return
	}
	a.replayMsg = "Replay exporté : " + name
}

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

	// Scale to fit, preserve aspect, add scanlines.
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	scale := min(float64(sw)/float64(w), float64(sh)/float64(h))
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(a.img, op)

	// Deterministic replay: game image + label, no race overlays.
	if a.replayPlayer != nil {
		drawText(screen, "REPLAY — Échap pour quitter", 20, 16, 1, colAccent2)
		drawScanlines(screen)
		return
	}

	// Local race overlays (clean, non-intrusive over the game).
	if a.race != nil {
		switch a.race.State() {
		case racePlaying, raceSlowmo:
			drawRaceGauge(screen, a.race)
			drawPiP(screen, a.pipImg)
			if a.race.State() == raceSlowmo {
				drawPanel(screen, 330, 300, 300, 40, colPanelHi, colAccent)
				drawText(screen, "FIN DE SEGMENT — RALENTI", 356, 310, 1, colAccent2)
			}
		case raceReplay:
			a.drawDramaticEnding(screen, a.race)
		}
	}

	// Quiet exit hint, bottom-left, small.
	psText(screen, "ESC  MENU", 20, 700, 9, colTextDim)

	drawScanlines(screen)
}
