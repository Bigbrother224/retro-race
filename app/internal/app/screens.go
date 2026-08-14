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

// drawFooter draws the bottom navigation hint as three anchored groups:
// left / center / right, small, dim and well-spaced. No heavy divider bar.
func drawFooter(screen *ebiten.Image, left, center, right string) {
	psText(screen, left, 48, 678, 11, colTextDim)
	psTextC(screen, center, 480, 678, 11, colTextDim)
	psTextR(screen, right, 912, 678, 11, colTextDim)
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
// below the tile with clean spacing.
func (a *App) drawGameScreen(screen *ebiten.Image) {
	drawBG(screen)
	con := a.consoles[a.selCon]
	cc := consoleColor(con.ID)
	n := len(con.Games)

	// Quiet console header: small label, thin rule, far from the box.
	psTextC(screen, con.Name, 480, 22, 11, cc)
	w := psWidth(con.Name, 11)
	fillRect(screen, int(480-w/2)-6, 42, int(w)+12, 2, cc)

	if n == 0 {
		psTextC(screen, "NO GAMES", 480, 356, 22, colTextDim)
		return
	}

	a.carousel.syncTo(a.selGame, 240)

	const slotW, slotH = 220.0, 316.0
	const spacing = 256.0
	cy := 300.0

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
		depth := 16.0 * scale

		drawBox3D(screen, img, cx, cy, w, h, depth, alpha)
	}

	// Selected game: name + meta, cleanly spaced below the box.
	sel := con.Games[a.selGame]
	cx := 480.0 + a.carousel.offset
	psTextC(screen, sel.Name, cx, 492, 20, colText)
	psTextC(screen, fmt.Sprintf("%s  ·  %d KB", strings.ToUpper(sel.Ext), sel.Size/1024),
		cx, 492+26+6, 10, colTextDim)

	drawFooter(screen, "ESC  BACK", "ENTER  PLAY", "UP / DOWN  CHOOSE")
}
