package app

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Carousel state: target index + animated offset for smooth sliding.
type carousel struct {
	target   int
	offset   float64 // 0 = perfectly centered on target
	lastStep float64
}

// step animates the carousel toward its target with easing.
func (c *carousel) step() {
	c.offset += (0 - c.offset) * 0.16
	if math.Abs(c.offset) < 0.001 {
		c.offset = 0
	}
}

// snapTo sets the target and resets the offset (instant for a big jump).
func (c *carousel) snapTo(target int) {
	c.target = target
	c.offset = 0
}

// glowColor returns a translucent version of c for spotlights.
func glowColor(c color.Color) color.Color {
	r, g, b, _ := c.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0x46}
}

// drawConsoleScreen draws the console carousel with console art.
func (a *App) drawConsoleScreen(screen *ebiten.Image) {
	a.carousel.step()
	fillRect(screen, 0, 0, 960, 720, colBG)

	// Header.
	drawGlow(screen, 480, 90, 560, 120, glowColor(colAccent2))
	drawText(screen, "SELECT CONSOLE", 356, 46, 3, colAccent2)
	drawText(screen, "choisis ta machine", 408, 92, 1, colTextDim)

	centerX, centerY := 480.0, 330.0
	n := len(a.consoles)

	// Glow spotlight behind the focused console (drawn under the art).
	if n > 0 {
		cc := consoleColor(a.consoles[a.carousel.target].ID)
		drawGlow(screen, int(centerX), int(centerY), 460, 340, glowColor(cc))
	}

	// Draw all consoles, farthest first, sliding smoothly.
	for i := 0; i < n; i++ {
		rel := float64(i - a.carousel.target)
		pos := rel*420 + a.carousel.offset
		if pos < -700 || pos > 700 {
			continue
		}
		con := a.consoles[i]
		art := consoleArt(con.ID)
		cc := consoleColor(con.ID)

		dist := math.Abs(pos) / 420.0
		scale := 1.0 - 0.32*math.Min(dist, 1.0)
		alpha := 1.0 - 0.55*math.Min(dist, 1.0)

		w := float64(artW) * scale
		h := float64(artH) * scale

		// Shadowed pedestal under the console.
		pedW := w * 0.86
		fillRectF(screen, centerX+pos-pedW/2, centerY+h*0.9, pedW, 14, color.RGBA{0, 0, 0, 96})

		// Console art.
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(centerX+pos-w/2, centerY-h/2)
		op.Filter = ebiten.FilterNearest
		op.ColorScale.ScaleAlpha(float32(alpha))
		screen.DrawImage(art, op)

		// Name plate under the console, console-tinted.
		name := con.Name
		plateW := 40 + textWidth(name)*1
		if plateW > 300 {
			plateW = 300
		}
		px := int(centerX+pos) - plateW/2
		py := int(centerY + h/2 + 18)
		fillRect(screen, px, py, plateW, 26, color.RGBA{0x12, 0x0c, 0x1e, 0xcc})
		fillRect(screen, px, py, plateW, 3, cc)
		drawText(screen, name, px+(plateW-textWidth(name))/2, py+7, 1, cc)
	}

	// Selection brackets around the focused console.
	if n > 0 {
		bw := artW + artW*4/100
		bh := artH + artH*12/100
		drawBrackets(screen, int(centerX), int(centerY-4), bw, bh, 12, 6, colAccent2)
	}

	// Footer hints.
	drawPanel(screen, 40, 636, 880, 44, colPanel, colAccent)
	drawText(screen, "← →  naviguer     Entrée  sélectionner     Échap  titre", 240, 650, 2, colText)

	drawScanlines(screen)
	drawVignette(screen)
}
