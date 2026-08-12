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
	// Move offset toward 0 exponentially (smooth deceleration).
	c.offset += (0 - c.offset) * 0.18
	if math.Abs(c.offset) < 0.001 {
		c.offset = 0
	}
}

// snapTo sets the target and resets the offset (instant for a big jump).
func (c *carousel) snapTo(target int) {
	c.target = target
	c.offset = 0
}

// drawConsoleScreen draws the console carousel with console art.
func (a *App) drawConsoleScreen(screen *ebiten.Image) {
	a.carousel.step()
	fillRect(screen, 0, 0, 960, 720, colBG)

	// Header
	drawText(screen, "SELECT CONSOLE", 330, 36, 3, colAccent2)

	// Background glow behind center.
	centerX := 480.0
	centerY := 330.0

	// Draw all consoles from left to right, farthest first.
	n := len(a.consoles)
	for i := 0; i < n; i++ {
		// Position relative to target, with the animated offset.
		rel := float64(i - a.carousel.target)
		pos := rel*420 + a.carousel.offset
		if pos < -660 || pos > 660 {
			continue
		}
		con := a.consoles[i]
		art := consoleArt(con.ID)

		// Scale: center big, sides smaller.
		dist := math.Abs(pos) / 420.0
		scale := 1.0 - 0.35*math.Min(dist, 1.0)
		alpha := 1.0 - 0.55*math.Min(dist, 1.0)

		w := float64(artW) * scale
		h := float64(artH) * scale

		// Draw a shadowed pedestal under the console.
		pedW := w * 0.9
		fillRectF(screen, centerX+pos-pedW/2, centerY+h*0.92, pedW, 14, color.RGBA{0, 0, 0, 90})

		// Console art, dimmed when far.
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(centerX+pos-w/2, centerY-h/2)
		op.Filter = ebiten.FilterNearest
		op.ColorScale.ScaleAlpha(float32(alpha))
		screen.DrawImage(art, op)

		// Name under the console.
		drawText(screen, con.Name, int(centerX+pos)-len(con.Name)*7/2, int(centerY+h/2+26), 1, colText)
	}

	// Selection frame: brackets around the center console.
	a.drawSelectionFrame(screen, centerX, centerY)

	// Footer hints.
	drawPanel(screen, 40, 620, 880, 52, colPanel, colAccent)
	drawText(screen, "← →  slider      Entrée  sélectionner      Échap  titre", 220, 634, 2, colText)

	drawScanlines(screen)
}

// drawSelectionFrame draws retro corner brackets around the selected console.
func (a *App) drawSelectionFrame(screen *ebiten.Image, cx, cy float64) {
	w, h := float64(artW)*1.06, float64(artH)*1.12
	x, y := cx-w/2, cy-h/2
	bracket := 26.0
	thick := 6.0
	c := colAccent2

	// Four corner brackets.
	fillRectF(screen, x, y, bracket, thick, c)
	fillRectF(screen, x, y, thick, bracket, c)
	fillRectF(screen, x+w-bracket, y, bracket, thick, c)
	fillRectF(screen, x+w-thick, y, thick, bracket, c)
	fillRectF(screen, x, y+h-thick, bracket, thick, c)
	fillRectF(screen, x, y+h-bracket, thick, bracket, c)
	fillRectF(screen, x+w-bracket, y+h-thick, bracket, thick, c)
	fillRectF(screen, x+w-thick, y+h-bracket, thick, bracket, c)
}

// fillRectF fills a float rectangle.
func fillRectF(screen *ebiten.Image, x, y, w, h float64, c color.Color) {
	img := ebiten.NewImage(int(math.Ceil(w)), int(math.Ceil(h)))
	img.Fill(c)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(x, y)
	screen.DrawImage(img, op)
}
