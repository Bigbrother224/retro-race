package app

import (
	"image/color"
	"math"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// Carousel state: target index + animated offset for smooth sliding.
type carousel struct {
	target   int
	offset   float64 // 0 = perfectly centered on target
	lastStep float64
}

// step eases the offset toward 0 (smooth deceleration).
func (c *carousel) step() {
	c.offset += (0 - c.offset) * 0.16
	if math.Abs(c.offset) < 0.001 {
		c.offset = 0
	}
}

// syncTo animates the carousel toward a new target with a slide-in.
func (c *carousel) syncTo(target int, spacing float64) {
	if c.target != target {
		dir := 1.0
		if target < c.target {
			dir = -1.0
		}
		c.target = target
		c.offset = spacing * dir
	}
	c.step()
}

func (c *carousel) snapTo(target int) {
	c.target = target
	c.offset = 0
}

// ---------------------------------------------------------------------------
// Real console photos (Assets/consoles), loaded once and cached.

var (
	photoOnce sync.Once
	photoMap  map[string]*ebiten.Image
)

func loadPhotos() {
	photoMap = map[string]*ebiten.Image{}
	srcs := []struct{ id, file string }{
		{"nes", "Assets/consoles/nes.png"},
		{"snes", "Assets/consoles/snes.png"},
		{"gb", "Assets/consoles/gb.jpg"},
		{"genesis", "Assets/consoles/genesis.jpg"},
	}
	for _, s := range srcs {
		if img, _, err := ebitenutil.NewImageFromFile(s.file); err == nil {
			photoMap[s.id] = img
		} else {
			photoMap[s.id] = consolePhotoFallback(s.id)
		}
	}
}

// consolePhoto returns the real photo for a console id (cached), or a clean
// fallback tile if the asset is missing.
func consolePhoto(id string) *ebiten.Image {
	photoOnce.Do(loadPhotos)
	if img, ok := photoMap[id]; ok {
		return img
	}
	return consolePhotoFallback(id)
}

// consolePhotoFallback draws a clean console-colored tile when no photo exists.
func consolePhotoFallback(id string) *ebiten.Image {
	const w, h = 480, 300
	img := ebiten.NewImage(w, h)
	img.Fill(color.RGBA{0x16, 0x16, 0x1e, 0xff})
	c := consoleColor(id)
	fillRect(img, 0, 0, w, 6, c)
	fillRect(img, 0, h-6, w, 6, c)
	psTextC(img, id, float64(w)/2, float64(h)/2-14, 22, colText)
	return img
}

// ---------------------------------------------------------------------------
// Console selection screen: horizontal carousel of real console photos.

func (a *App) drawConsoleScreen(screen *ebiten.Image) {
	drawBG(screen)
	psTextC(screen, "CHOOSE CONSOLE", 480, 34, 13, colTextDim)
	fillRect(screen, 400, 54, 160, 2, colBorder)

	n := len(a.consoles)
	if n == 0 {
		psTextC(screen, "NO GAMES FOUND", 480, 356, 22, colTextDim)
		drawFooter(screen, "ESC  BACK", "", "ADD ROMS TO /Roms")
		return
	}

	a.carousel.step()

	const slotW, slotH = 340.0, 300.0
	const spacing = 400.0
	cy := 292.0

	for i := range n {
		rel := float64(i - a.carousel.target)
		pos := rel*spacing + a.carousel.offset
		if pos < -900 || pos > 900 {
			continue
		}
		con := a.consoles[i]
		img := consolePhoto(con.ID)
		d := math.Abs(rel)
		scale := 1.0 - 0.3*math.Min(d, 1.0)
		alpha := float32(1.0 - 0.45*math.Min(d, 1.0))

		iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
		s := math.Min(slotW/iw, slotH/ih) * scale
		w, h := iw*s, ih*s
		cx := 480.0 + pos

		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(s, s)
		op.GeoM.Translate(cx-w/2, cy-h/2)
		op.Filter = ebiten.FilterLinear
		op.ColorScale.ScaleAlpha(alpha)
		screen.DrawImage(img, op)
	}

	// Selected console name below the center tile.
	sel := a.consoles[a.carousel.target]
	cc := consoleColor(sel.ID)
	cx := 480.0 + a.carousel.offset
	psTextC(screen, sel.Name, cx, 486, 20, cc)

	drawFooter(screen, "ESC  BACK", "ENTER  OK", "LEFT / RIGHT  CHOOSE")
}
