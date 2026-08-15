package app

import (
	"image"
	"image/color"
	_ "image/png" // register PNG decoder for boxart files
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// boxartDir (see paths.go) is where user-provided boxart images live, keyed by
// console id then game name (e.g. Boxart/nes/Alter Ego.png). It is optional:
// when an image is absent, a generated placeholder tile is shown instead.

// boxartPath returns the local boxart image path for a game under dir, and
// whether it exists. (_, false) means fall back to a generated placeholder.
// Pure and directory-parameterized so it can be unit-tested.
func boxartPath(dir, consoleID, gameName string) (string, bool) {
	p := filepath.Join(dir, consoleID, gameName+".png")
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	return p, true
}

// boxartKey identifies a game's boxart for caching.
type boxartKey struct {
	consoleID string
	gameName  string
}

// boxartFor returns the boxart image for a game: the local file when present,
// otherwise a generated placeholder. Images are cached so each game is loaded
// once, keeping the draw loop allocation-free.
func (a *App) boxartFor(consoleID, gameName string) *ebiten.Image {
	key := boxartKey{consoleID, gameName}
	if img, ok := a.boxarts[key]; ok {
		return img
	}
	var img *ebiten.Image
	if p, ok := boxartPath(boxartDir, consoleID, gameName); ok {
		if loaded, _, err := ebitenutil.NewImageFromFile(p); err == nil {
			img = loaded
		}
	}
	if img == nil {
		img = placeholderBoxart(consoleID, gameName)
	}
	a.boxarts[key] = img
	return img
}

// placeholderBoxart draws a clean procedural tile for a game: a dark panel,
// console-tinted top/bottom bars and the game name centered. Used when no
// local boxart image exists.
func placeholderBoxart(consoleID, name string) *ebiten.Image {
	const w, h = 160, 220
	img := ebiten.NewImage(w, h)
	c := consoleColor(consoleID)

	img.Fill(color.RGBA{0x11, 0x11, 0x17, 0xff})
	fillRect(img, 5, 5, w-10, h-10, color.RGBA{0x18, 0x18, 0x20, 0xff})
	fillRect(img, 5, 5, w-10, 4, c)
	fillRect(img, 5, h-9, w-10, 4, c)

	// Game name centered.
	lines := strings.Split(wrapLines(name, 12), "\n")
	y := float64(h)/2 - float64(len(lines)*16)/2 + 8
	for _, ln := range lines {
		psTextC(img, ln, float64(w)/2, y, 11, colText)
		y += 16
	}

	// Small console tag near the bottom.
	psTextC(img, strings.ToUpper(consoleID), float64(w)/2, float64(h)-26, 9, colTextDim)
	return img
}

// wrapLines chunks s into lines of at most n runes each.
func wrapLines(s string, n int) string {
	runes := []rune(s)
	var lines []string
	for len(runes) > n {
		lines = append(lines, string(runes[:n]))
		runes = runes[n:]
	}
	if len(runes) > 0 {
		lines = append(lines, string(runes))
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// 3D SNES-Mini-style box rendering.
//
// The real SNES Classic menu shows each game as a standing box: the front
// cover plus a darkened right spine and a thin top face, sitting on a soft
// drop shadow. We fake the same look by drawing the cover, then sheared,
// darkened edge strips for the spine and top.

var (
	shadowOnce sync.Once
	shadowImg  *ebiten.Image // soft vertical fade used as the drop shadow
)

// buildShadow renders a small vertical fade (solid at top -> transparent at
// bottom) that we stretch into an ellipse under each box.
func buildShadow() {
	const w, h = 64, 16
	shadowImg = ebiten.NewImage(w, h)
	for y := range h {
		t := float64(y) / float64(h)
		a := uint8(110 * (1 - t) * (1 - t))
		fillRect(shadowImg, 0, y, w, 1, color.RGBA{0, 0, 0, a})
	}
}

// drawBox3D draws the boxart img as a 3D box centered on (cx, cy) with front
// face w x h and box depth d (right spine + top face), plus a drop shadow.
// alpha fades the whole box (0..1). Allocation-free: faces are SubImages
// sheared via GeoM, shadow is cached.
func drawBox3D(screen *ebiten.Image, img *ebiten.Image, cx, cy, w, h, d, alpha float64) {
	fx := cx - w/2
	fy := cy - h/2
	sw := float64(img.Bounds().Dx())
	sh := float64(img.Bounds().Dy())
	a := float32(alpha)

	// Drop shadow: an ellipse under the box.
	shadowOnce.Do(buildShadow)
	{
		op := &ebiten.DrawImageOptions{}
		shw := w*0.95 + d
		shh := 14.0
		op.GeoM.Scale(shw/float64(shadowImg.Bounds().Dx()), shh/float64(shadowImg.Bounds().Dy()))
		op.GeoM.Translate(cx-shw/2, fy+h-2)
		op.ColorScale.ScaleAlpha(a * 0.8)
		screen.DrawImage(shadowImg, op)
	}

	// Top face: a thin horizontal sliver of the cover, sheared and darkened.
	{
		th := d
		strip := img.SubImage(image.Rect(0, 0, img.Bounds().Dx(), int(sh*0.06))).(*ebiten.Image)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(w/float64(strip.Bounds().Dx()), th/float64(strip.Bounds().Dy()))
		op.Filter = ebiten.FilterLinear
		op.ColorScale.Scale(0.5, 0.5, 0.58, a)
		op.GeoM.Skew(0.2, 0)
		op.GeoM.Translate(fx, fy-th)
		screen.DrawImage(strip, op)
	}

	// Right spine: a thin vertical sliver of the cover, sheared and darkened.
	{
		wd := d
		strip := img.SubImage(image.Rect(0, 0, int(sw*0.06), img.Bounds().Dy())).(*ebiten.Image)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(wd/float64(strip.Bounds().Dx()), h/float64(strip.Bounds().Dy()))
		op.Filter = ebiten.FilterLinear
		op.ColorScale.Scale(0.35, 0.35, 0.45, a)
		op.GeoM.Skew(0, 0.2)
		op.GeoM.Translate(fx+w, fy)
		screen.DrawImage(strip, op)
	}

	// Front cover last so its crisp edges sit on top of the spine/top.
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(w/sw, h/sh)
	op.GeoM.Translate(fx, fy)
	op.Filter = ebiten.FilterLinear
	op.ColorScale.ScaleAlpha(a)
	screen.DrawImage(img, op)
}
