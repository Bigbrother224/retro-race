package app

import (
	"bytes"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Press Start 2P is loaded once and reused everywhere. Glyphs are cached by
// text/v2's LRU cache, so drawing is allocation-free after warm-up.

var (
	fontOnce sync.Once
	fontSrc  *text.GoTextFaceSource
	fontErr  error
)

func loadFont() *text.GoTextFaceSource {
	fontOnce.Do(func() {
		b, err := os.ReadFile(filepath.Join(fontsDir, "PressStart2P-Regular.ttf"))
		if err != nil {
			fontErr = err
			return
		}
		fontSrc, fontErr = text.NewGoTextFaceSource(bytes.NewReader(b))
	})
	return fontSrc
}

var faceCache sync.Map // float64 -> *text.GoTextFace

// psFace returns a cached Press Start 2P face at the given pixel size.
func psFace(size float64) *text.GoTextFace {
	if v, ok := faceCache.Load(size); ok {
		return v.(*text.GoTextFace)
	}
	src := loadFont()
	if src == nil {
		return nil
	}
	f := &text.GoTextFace{Source: src, Size: size}
	faceCache.Store(size, f)
	return f
}

// ascii maps the few non-ASCII glyphs the app used (French accents, arrows)
// to ASCII equivalents, so the ASCII-only font never renders tofu boxes.
func ascii(s string) string {
	has := false
	for _, r := range s {
		if r >= 128 {
			has = true
			break
		}
	}
	if !has {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 'É', 'é', 'È', 'è':
			b.WriteRune('E')
		case 'À', 'à', 'Â', 'â':
			b.WriteRune('A')
		case 'Ç', 'ç':
			b.WriteRune('C')
		case '—', '–', '−':
			b.WriteRune('-')
		case '→':
			b.WriteString("->")
		case '←':
			b.WriteString("<-")
		case '↑':
			b.WriteString("^")
		case '↓':
			b.WriteString("v")
		case '▶', '►':
			b.WriteString(">")
		case '◀', '◄':
			b.WriteString("<")
		case '·':
			b.WriteRune('.')
		default:
			if r < 128 {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// psText draws s with Press Start 2P at size, top-left at (x, y).
func psText(screen *ebiten.Image, s string, x, y, size float64, col color.Color) {
	face := psFace(size)
	if face == nil {
		return
	}
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(col)
	text.Draw(screen, ascii(s), face, op)
}

// psTextC draws s centered horizontally on cx, top at y.
func psTextC(screen *ebiten.Image, s string, cx, y, size float64, col color.Color) {
	w, _ := psMeasure(s, size)
	psText(screen, s, cx-w/2, y, size, col)
}

// psTextR draws s right-aligned ending at x, top at y.
func psTextR(screen *ebiten.Image, s string, x, y, size float64, col color.Color) {
	w, _ := psMeasure(s, size)
	psText(screen, s, x-w, y, size, col)
}

// psMeasure returns the rendered width and height of s at the given size.
func psMeasure(s string, size float64) (float64, float64) {
	face := psFace(size)
	if face == nil {
		return 0, 0
	}
	return text.Measure(ascii(s), face, size*1.3)
}

func psWidth(s string, size float64) float64 {
	w, _ := psMeasure(s, size)
	return w
}

// drawText is the legacy helper used by app.go's playing HUD. It renders with
// Press Start 2P, scaling by the old integer scale (1 -> 16px).
func drawText(screen *ebiten.Image, s string, x, y int, scale int, col color.Color) {
	psText(screen, s, float64(x), float64(y), float64(scale)*16, col)
}
