package app

import (
	"image/color"
	"math"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font/basicfont"
)

// Palette rétro.
var (
	colBG       = color.RGBA{0x0d, 0x0a, 0x12, 0xff} // fond sombre violet
	colPanel    = color.RGBA{0x1c, 0x15, 0x2a, 0xff}
	colPanelHi  = color.RGBA{0x2a, 0x1f, 0x3d, 0xff}
	colText     = color.RGBA{0xf0, 0xe6, 0xff, 0xff}
	colTextDim  = color.RGBA{0x8f, 0x7f, 0xa8, 0xff}
	colAccent   = color.RGBA{0xff, 0x5e, 0x3a, 0xff} // orange arcade
	colAccent2  = color.RGBA{0xff, 0xd5, 0x3a, 0xff} // jaune
	colScanline = color.RGBA{0x00, 0x00, 0x00, 0x2e}
	colVignette = color.RGBA{0x00, 0x00, 0x00, 0x50}
	colNES      = color.RGBA{0xc2, 0x3b, 0x2c, 0xff}
	colSNES     = color.RGBA{0x8a, 0x4f, 0xce, 0xff}
	colGB       = color.RGBA{0x4f, 0x8f, 0x5f, 0xff}
	colGenesis  = color.RGBA{0x2c, 0x2c, 0x3c, 0xff}
	colGlow     = color.RGBA{0x3a, 0x2a, 0x60, 0xff}
)

// ---------------------------------------------------------------------------
// Allocation-free primitives. Every draw helper caches its backing images so
// the draw loop never allocates after warm-up.

// whitePixel is a 1x1 opaque white image tinted via ColorScale to draw any
// solid rectangle with zero per-frame allocation.
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

// fillRect fills an integer rectangle (thin wrapper over fillRectF).
func fillRect(screen *ebiten.Image, x, y, w, h int, col color.Color) {
	fillRectF(screen, float64(x), float64(y), float64(w), float64(h), col)
}

// panelImage draws a retro panel (fill + border bars) into an image, once.
// It is cached per (w, h, fill, border) so drawing a panel is allocation-free.
var panelCache sync.Map // key panelKey -> *ebiten.Image

type panelKey struct {
	w, h, fg, bg uint32
}

// drawPanel draws a rounded-ish retro panel (filled rect + border bars).
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
	// Top + bottom border bars.
	fillRect(img, 0, 0, w, 3, border)
	fillRect(img, 0, h-3, w, 3, border)
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
// Text, cached per (string, color) so rendering is allocation-free.

type textKey struct {
	s       string
	r, g, b uint8
}

var textCache sync.Map // textKey -> *ebiten.Image

// textImage renders s (newline-aware) with the built-in pixel face at native
// size into a cached image. The image is shared across all draw scales.
func textImage(s string, col color.Color) *ebiten.Image {
	r, g, b, _ := col.RGBA()
	key := textKey{s, uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
	if v, ok := textCache.Load(key); ok {
		return v.(*ebiten.Image)
	}
	lines := strings.Split(s, "\n")
	width := 0
	for _, ln := range lines {
		if w := len(ln) * 7; w > width {
			width = w
		}
	}
	if width == 0 {
		width = 7
	}
	height := len(lines) * 13
	img := ebiten.NewImage(width, height)
	for i, ln := range lines {
		text.Draw(img, ln, basicfont.Face7x13, 0, i*13+11, col)
	}
	textCache.Store(key, img)
	return img
}

// drawText draws s scaled with nearest filtering for a chunky pixel look.
func drawText(screen *ebiten.Image, s string, x, y int, scale int, col color.Color) {
	img := textImage(s, col)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(scale), float64(scale))
	op.GeoM.Translate(float64(x), float64(y))
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(img, op)
}

// textWidth returns the on-screen width (in pixels, unscaled) of s.
func textWidth(s string) int {
	w := 0
	for _, ln := range strings.Split(s, "\n") {
		if l := len(ln) * 7; l > w {
			w = l
		}
	}
	return w
}

// ---------------------------------------------------------------------------
// CRT overlays, precomputed once.

var (
	scanlinesOnce sync.Once
	scanlinesImg  *ebiten.Image
	vignetteImg   *ebiten.Image
)

func buildOverlays() {
	scanlinesImg = ebiten.NewImage(960, 720)
	for y := 0; y < 720; y += 3 {
		fillRect(scanlinesImg, 0, y, 960, 1, colScanline)
	}
	vignetteImg = ebiten.NewImage(960, 720)
	vignetteImg.Fill(colVignette)
}

// drawScanlines draws horizontal CRT scanlines over the whole screen.
func drawScanlines(screen *ebiten.Image) {
	scanlinesOnce.Do(buildOverlays)
	op := &ebiten.DrawImageOptions{}
	screen.DrawImage(scanlinesImg, op)
}

// drawVignette darkens the edges (CRT feel).
func drawVignette(screen *ebiten.Image) {
	scanlinesOnce.Do(buildOverlays)
	op := &ebiten.DrawImageOptions{}
	screen.DrawImage(vignetteImg, op)
}

// ---------------------------------------------------------------------------
// Glow: a soft pixel spotlight, cached per color.

var glowCache sync.Map // glowKey -> *ebiten.Image

type glowKey struct {
	w, h int
	rgba color.RGBA
}

// glowImage returns a radial-style pixel glow (w x h) tinted col, cached.
func glowImage(w, h int, col color.Color) *ebiten.Image {
	r, g, b, a := col.RGBA()
	rgba := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
	key := glowKey{w, h, rgba}
	if v, ok := glowCache.Load(key); ok {
		return v.(*ebiten.Image)
	}
	img := ebiten.NewImage(w, h)
	// Ellipse spotlight: per row, width follows an ellipse and alpha fades
	// toward the edges. Built once, drawn every frame for free.
	hw, hh := float64(w)/2, float64(h)/2
	for y := range h {
		dy := (float64(y) - hh) / hh // -1..1
		if dy < -1 || dy > 1 {
			continue
		}
		halfW := hw * math.Sqrt(1-dy*dy)
		alpha := uint8(float64(a) * (1 - absF(dy)))
		row := color.RGBA{rgba.R, rgba.G, rgba.B, alpha}
		fillRect(img, int(hw-halfW), y, int(2*halfW)+1, 1, row)
	}
	glowCache.Store(key, img)
	return img
}

// drawGlow draws a cached glow centered at (cx, cy).
func drawGlow(screen *ebiten.Image, cx, cy, w, h int, col color.Color) {
	img := glowImage(w, h, col)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(cx-w/2), float64(cy-h/2))
	screen.DrawImage(img, op)
}

// drawImageCentered draws an image centered on (cx, cy), preserving size.
func drawImageCentered(screen *ebiten.Image, img *ebiten.Image, cx, cy int) {
	b := img.Bounds()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(cx-b.Dx()/2), float64(cy-b.Dy()/2))
	screen.DrawImage(img, op)
}

// drawBrackets draws retro corner brackets around a rect (cx, cy center).
func drawBrackets(screen *ebiten.Image, cx, cy, w, h, pad, thick int, c color.Color) {
	xf := float64(cx - w/2 - pad)
	yf := float64(cy - h/2 - pad)
	wf := float64(w + 2*pad)
	hf := float64(h + 2*pad)
	br := float64(bracketLen(pad))
	fillRectF(screen, xf, yf, br, float64(thick), c)
	fillRectF(screen, xf, yf, float64(thick), br, c)
	fillRectF(screen, xf+wf-br, yf, br, float64(thick), c)
	fillRectF(screen, xf+wf-float64(thick), yf, float64(thick), br, c)
	fillRectF(screen, xf, yf+hf-float64(thick), br, float64(thick), c)
	fillRectF(screen, xf, yf+hf-br, float64(thick), br, c)
	fillRectF(screen, xf+wf-br, yf+hf-float64(thick), br, float64(thick), c)
	fillRectF(screen, xf+wf-float64(thick), yf+hf-br, float64(thick), br, c)
}

func bracketLen(pad int) float64 {
	l := float64(pad) * 0.8
	if l < 18 {
		return 18
	}
	return l
}

func absF(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
