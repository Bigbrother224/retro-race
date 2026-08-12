package app

import (
	"image/color"
	_ "image/png" // register PNG decoder for boxart files
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// boxartDir is where user-provided boxart images live, keyed by console id
// then game name (e.g. Boxart/nes/Alter Ego.png). It is optional: when an
// image is absent, a generated placeholder tile is shown instead.
const boxartDir = "/Users/mac/retro-race/app/Boxart"

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

// placeholderBoxart draws a procedural pixel-art tile for a game: a
// console-tinted background, a console-colored frame, a small motif block and
// the game name. Used when no local boxart image exists.
func placeholderBoxart(consoleID, name string) *ebiten.Image {
	const w, h = 160, 220
	img := ebiten.NewImage(w, h)
	c := consoleColor(consoleID)

	// Dark console-tinted background.
	img.Fill(color.RGBA{0x18, 0x14, 0x20, 0xff})

	// Accent frame (console color).
	fillRect(img, 6, 6, w-12, 6, c)    // top
	fillRect(img, 6, h-12, w-12, 6, c) // bottom
	fillRect(img, 6, 6, 6, h-12, c)    // left
	fillRect(img, w-12, 6, 6, h-12, c) // right

	// Motif block.
	fillRect(img, w/2-24, 64, 48, 48, c)

	// Game name, wrapped.
	drawText(img, wrapLines(name, 14), 10, 132, 1, colText)

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
