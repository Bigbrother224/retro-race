package app

import (
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font/basicfont"
)

// Palette rétro.
var (
	colBG        = color.RGBA{0x0d, 0x0a, 0x12, 0xff} // fond sombre violet
	colPanel     = color.RGBA{0x1c, 0x15, 0x2a, 0xff}
	colPanelHi   = color.RGBA{0x2a, 0x1f, 0x3d, 0xff}
	colText      = color.RGBA{0xf0, 0xe6, 0xff, 0xff}
	colTextDim   = color.RGBA{0x8f, 0x7f, 0xa8, 0xff}
	colAccent    = color.RGBA{0xff, 0x5e, 0x3a, 0xff} // orange arcade
	colAccent2   = color.RGBA{0xff, 0xd5, 0x3a, 0xff} // jaune
	colScanline  = color.RGBA{0x00, 0x00, 0x00, 0x35}
	colVignette  = color.RGBA{0x00, 0x00, 0x00, 0x60}
	colNES       = color.RGBA{0xc2, 0x3b, 0x2c, 0xff}
	colSNES      = color.RGBA{0x8a, 0x4f, 0xce, 0xff}
	colGB        = color.RGBA{0x4f, 0x8f, 0x5f, 0xff}
	colGenesis   = color.RGBA{0x2c, 0x2c, 0x3c, 0xff}
)

// drawScanlines draws horizontal CRT scanlines over the whole screen.
func drawScanlines(screen *ebiten.Image) {
	img := ebiten.NewImage(960, 1)
	img.Fill(colScanline)
	for y := 0; y < 720; y += 3 {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(0, float64(y))
		screen.DrawImage(img, op)
	}
}

// drawVignette darkens the edges (CRT feel).
func drawVignette(screen *ebiten.Image) {
	img := ebiten.NewImage(960, 720)
	img.Fill(colVignette)
	op := &ebiten.DrawImageOptions{}
	screen.DrawImage(img, op)
}

// drawText draws text with the built-in pixel face, scaled.
func drawText(screen *ebiten.Image, s string, x, y int, scale int, col color.Color) {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		// Basic font face: 7x13. Draw once into a small image then scale with
		// nearest filter for a chunky pixel look.
		w := len(line) * 7
		h := 13
		img := ebiten.NewImage(w, h)
		text.Draw(img, line, basicfont.Face7x13, 0, 11, col)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(float64(scale), float64(scale))
		op.GeoM.Translate(float64(x), float64(y)+float64(i*h*scale))
		op.Filter = ebiten.FilterNearest
		screen.DrawImage(img, op)
	}
}

// drawPanel draws a rounded-ish retro panel (simple: filled rect + border).
func drawPanel(screen *ebiten.Image, x, y, w, h int, fill, border color.Color) {
	img := ebiten.NewImage(w, h)
	img.Fill(fill)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(img, op)

	// Border: top+bottom 3px bars (retro brackets).
	bar := ebiten.NewImage(w, 3)
	bar.Fill(border)
	opTop := &ebiten.DrawImageOptions{}
	opTop.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(bar, opTop)
	opBot := &ebiten.DrawImageOptions{}
	opBot.GeoM.Translate(float64(x), float64(y+h-3))
	screen.DrawImage(bar, opBot)
}

// fillRect fills a rectangle with a color.
func fillRect(screen *ebiten.Image, x, y, w, h int, col color.Color) {
	img := ebiten.NewImage(w, h)
	img.Fill(col)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(img, op)
}
