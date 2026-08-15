package app

import (
	"image/color"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// Palette rétro sobre (style menu mini-console).
var (
	colBG       = color.RGBA{0x0d, 0x0a, 0x12, 0xff}
	colPanel    = color.RGBA{0x1a, 0x1a, 0x22, 0xff}
	colPanelHi  = color.RGBA{0x28, 0x28, 0x34, 0xff}
	colText     = color.RGBA{0xe8, 0xe6, 0xea, 0xff}
	colTextDim  = color.RGBA{0x88, 0x84, 0x94, 0xff}
	colAccent   = color.RGBA{0xff, 0x5e, 0x3a, 0xff} // orange arcade
	colAccent2  = color.RGBA{0xff, 0xd5, 0x3a, 0xff} // jaune
	colScanline = color.RGBA{0x00, 0x00, 0x00, 0x2e}
	colVignette = color.RGBA{0x00, 0x00, 0x00, 0x50}
	colNES      = color.RGBA{0xc2, 0x3b, 0x2c, 0xff}
	colSNES     = color.RGBA{0x8a, 0x4f, 0xce, 0xff}
	colGB       = color.RGBA{0x4f, 0x8f, 0x5f, 0xff}
	colGenesis  = color.RGBA{0x2c, 0x2c, 0x3c, 0xff}
	colBorder   = color.RGBA{0x2c, 0x2c, 0x38, 0xff}
)

// ---------------------------------------------------------------------------
// Allocation-free primitives.

// whitePixel is a 1x1 opaque white image tinted via ColorScale.
var whitePixel = func() *ebiten.Image {
	img := ebiten.NewImage(1, 1)
	img.Fill(color.White)
	return img
}()

// fillRectF fills a float rectangle using the shared tinted pixel.
func fillRectF(screen *ebiten.Image, x, y, w, h float64, c color.Color) {
	if w <= 0 || h <= 0 {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(w, h)
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(c)
	screen.DrawImage(whitePixel, op)
}

// fillRect fills an integer rectangle.
func fillRect(screen *ebiten.Image, x, y, w, h int, col color.Color) {
	fillRectF(screen, float64(x), float64(y), float64(w), float64(h), col)
}

var panelCache sync.Map // panelKey -> *ebiten.Image

type panelKey struct {
	w, h, fg, bg uint32
}

// drawPanel draws a flat panel (fill + border bars), cached per (w,h,fill,border).
func drawPanel(screen *ebiten.Image, x, y, w, h int, fill, border color.Color) {
	key := panelKey{uint32(w), uint32(h), rgbaU32(fill), rgbaU32(border)}
	if v, ok := panelCache.Load(key); ok {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(x), float64(y))
		screen.DrawImage(v.(*ebiten.Image), op)
		return
	}
	img := ebiten.NewImage(w, h)
	img.Fill(fill)
	fillRect(img, 0, 0, w, 2, border)
	fillRect(img, 0, h-2, w, 2, border)
	panelCache.Store(key, img)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(img, op)
}

func rgbaU32(c color.Color) uint32 {
	r, g, b, a := c.RGBA()
	return r<<24 | g<<16 | b<<8 | a
}

// ---------------------------------------------------------------------------
// Background: a subtle dark vertical gradient (clean mini-console look).

var (
	bgOnce sync.Once
	bgImg  *ebiten.Image
)

func buildBG() {
	bgImg = ebiten.NewImage(960, 720)
	top := color.RGBA{0x16, 0x16, 0x1e, 0xff}
	bot := color.RGBA{0x0a, 0x0a, 0x10, 0xff}
	for y := range 720 {
		t := float64(y) / 719
		fillRect(bgImg, 0, y, 960, 1, lerp(top, bot, t))
	}
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		255,
	}
}

// drawBG fills the screen with the cached dark gradient.
func drawBG(screen *ebiten.Image) {
	bgOnce.Do(buildBG)
	op := &ebiten.DrawImageOptions{}
	screen.DrawImage(bgImg, op)
}

// ---------------------------------------------------------------------------
// CRT overlays (kept for the playing screen, which app.go renders).

var (
	scanlinesOnce sync.Once
	scanlinesImg  *ebiten.Image
)

func buildScanlines() {
	scanlinesImg = ebiten.NewImage(960, 720)
	for y := range 720 {
		if y%3 != 0 {
			continue
		}
		fillRect(scanlinesImg, 0, y, 960, 1, colScanline)
	}
}

// drawScanlines draws horizontal CRT scanlines over the whole screen.
func drawScanlines(screen *ebiten.Image) {
	scanlinesOnce.Do(buildScanlines)
	op := &ebiten.DrawImageOptions{}
	screen.DrawImage(scanlinesImg, op)
}
