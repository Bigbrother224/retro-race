package app

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"retrorace/internal/engine"
	"retrorace/internal/library"
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

// App is the Ebitengine game: the arcade menu (navigation + selection) and the
// coordinator that hands play to a gameSession. All in-game/race/netplay state
// lives in gameSession (R4 split); App keeps only menu + navigation.
type App struct {
	consoles []library.Console
	state    state
	selCon   int
	selGame  int
	frame    int
	carousel carousel

	// Profile: player name + chosen mode (solo play or race).
	username  string
	gameMode  int // 0 = solo, 1 = race, 2 = share
	profileAt int // frame when the profile screen was entered

	// Boxart cache (loaded once per game).
	boxarts map[boxartKey]*ebiten.Image

	// Active game session (solo, race or netplay shared game). nil in the menu.
	game *gameSession

	errMsg string
}

func Run() error {
	// Point the libretro core at the user-data directories before any core
	// loads: saves persist across runs instead of landing in /tmp.
	engine.SetSystemDir(savesDir)
	engine.SetSaveDir(savesDir)

	consoles := library.New().Scan(romsDir)
	a := &App{consoles: consoles, state: stateTitle, boxarts: map[boxartKey]*ebiten.Image{}}
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
		a.game.updateNetLobby()
	case statePlaying:
		a.game.updatePlaying()
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
		a.game.drawNetLobby(screen)
	case statePlaying:
		a.game.drawPlaying(screen)
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
// detected and read.
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
		for ax := range 8 {
			if ax >= n {
				break
			}
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
		a.game = newGameSession(a, con, g)
		if a.gameMode == modeShare {
			a.game.enterNetLobby()
			return
		}
		if err := a.game.load(); err != nil {
			a.errMsg = err.Error()
			a.game = nil
			return
		}
		a.state = statePlaying
		if a.gameMode == modeRace {
			a.game.startRace()
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
