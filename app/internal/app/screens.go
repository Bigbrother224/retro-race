package app

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// consoleColor returns the arcade color for a console.
func consoleColor(id string) color.Color {
	switch id {
	case "nes":
		return colNES
	case "snes":
		return colSNES
	case "gb":
		return colGB
	case "genesis":
		return colGenesis
	}
	return colAccent
}

// drawFooter draws the small navigation hint at the bottom of menu screens.
func drawFooter(screen *ebiten.Image, hint string) {
	fillRect(screen, 200, 656, 560, 2, colBorder)
	psTextC(screen, hint, 480, 666, 11, colTextDim)
}

// drawTitleScreen shows the SNES-Mini-style boot screen with blinking
// PRESS START.
func (a *App) drawTitleScreen(screen *ebiten.Image, frameCount int) {
	drawBG(screen)

	// Logo.
	psTextC(screen, "RETRO RACE", 480, 240, 62, colText)
	lw := psWidth("RETRO RACE", 62)
	fillRect(screen, int(480-lw/2)+20, 240+62+16, int(lw)-40, 4, colAccent2)

	// Blinking press start (stable slot: bright / dim).
	press := color.RGBA{0x33, 0x33, 0x3d, 0xff}
	if (frameCount/30)%2 == 0 {
		press = colText
	}
	psTextC(screen, "PRESS START", 480, 420, 24, press)
	psTextC(screen, "ENTER  /  START", 480, 468, 12, colTextDim)
}

// drawGameScreen shows the SNES-Mini-style horizontal boxart carousel for the
// selected console. The focused game is centered and enlarged, its name shown
// below the tile.
func (a *App) drawGameScreen(screen *ebiten.Image) {
	drawBG(screen)
	con := a.consoles[a.selCon]
	cc := consoleColor(con.ID)
	n := len(con.Games)

	// Header.
	psText(screen, con.Name, 40, 26, 14, cc)
	psTextR(screen, "ESC  BACK", 920, 28, 12, colTextDim)

	if n == 0 {
		psTextC(screen, "NO GAMES", 480, 356, 22, colTextDim)
		return
	}

	a.carousel.syncTo(a.selGame, 240)

	const slotW, slotH = 210.0, 300.0
	const spacing = 250.0
	cy := 280.0

	for i := range n {
		rel := float64(i - a.selGame)
		pos := rel*spacing + a.carousel.offset
		if pos < -900 || pos > 900 {
			continue
		}
		g := con.Games[i]
		img := a.boxartFor(con.ID, g.Name)
		d := math.Abs(rel)
		scale := 1.0 - 0.28*math.Min(d, 1.0)
		alpha := 1.0 - 0.45*math.Min(d, 1.0)

		iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
		s := math.Min(slotW/iw, slotH/ih) * scale
		w, h := iw*s, ih*s
		cx := 480.0 + pos
		depth := 18.0 * scale

		drawBox3D(screen, img, cx, cy, w, h, depth, alpha)
	}

	// Selected game name + meta below the center tile.
	sel := con.Games[a.selGame]
	cx := 480.0 + a.carousel.offset
	fillRect(screen, 480-260, 466, 520, 2, colBorder)
	psTextC(screen, sel.Name, cx, 478, 20, colText)
	psTextC(screen, fmt.Sprintf("FORMAT %s    %d KB", strings.ToUpper(sel.Ext), sel.Size/1024),
		cx, 478+26+14, 10, colTextDim)

	drawFooter(screen, "UP / DOWN  CHOOSE      ENTER  PLAY      ESC  BACK")
}
