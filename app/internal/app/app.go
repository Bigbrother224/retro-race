package app

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"retrorace/internal/engine"
	"retrorace/internal/library"
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

	errMsg string
}

func Run() error {
	consoles := library.New().Scan(romsDir)
	a := &App{consoles: consoles, state: stateTitle, boxarts: map[boxartKey]*ebiten.Image{}}
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
	return nil
}

// ---- playing ----

func (a *App) updatePlaying() {
	if a.emu == nil {
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.stopGame()
		return
	}
	a.updateGameInput()
	a.emu.Step()
}

// updateGameInput maps keyboard keys to logical buttons (SNES-style).
func (a *App) updateGameInput() {
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
		a.emu.SetButton(btn, inpututil.IsKeyJustPressed(key) || ebiten.IsKeyPressed(key))
	}
	a.updateGamepadInput()
}

func (a *App) stopGame() {
	if a.emu != nil {
		a.emu.Stop()
		a.emu = nil
	}
	a.running = false
	a.state = stateGame
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

	// HUD: game name + hint.
	con := a.consoles[a.selCon]
	g := con.Games[a.selGame]
	drawText(screen, fmt.Sprintf("%s — %s", con.Name, g.Name), 20, 16, 1, colTextDim)
	drawText(screen, "Échap pour revenir au menu", 20, 700, 1, colTextDim)
	drawScanlines(screen)
}
