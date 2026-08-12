package app

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Pixel-art console illustrations, drawn in code. Each console is drawn into
// its own image at a fixed size, then scaled/blended by the carousel.

const artW, artH = 320, 220

var (
	colGreyDark  = color.RGBA{0x3a, 0x3a, 0x42, 0xff}
	colGreyMid   = color.RGBA{0x4c, 0x4c, 0x56, 0xff}
	colGreyLight = color.RGBA{0x6b, 0x6b, 0x78, 0xff}
	colBlack     = color.RGBA{0x14, 0x14, 0x18, 0xff}
	colScreen    = color.RGBA{0x9a, 0xd1, 0x7e, 0xff}
	colScreenGB  = color.RGBA{0x9b, 0xbc, 0x5a, 0xff}
	colRedAcc    = color.RGBA{0xd8, 0x3a, 0x2c, 0xff}
	colVioletAcc = color.RGBA{0x8a, 0x4f, 0xce, 0xff}
	colGenBlue   = color.RGBA{0x2a, 0x7a, 0xd4, 0xff}
	colGenRed    = color.RGBA{0xe0, 0x3a, 0x2c, 0xff}
	colGBGrey    = color.RGBA{0x8f, 0x8f, 0x9a, 0xff}
)

func newArt() *ebiten.Image {
	return ebiten.NewImage(artW, artH)
}

// drawNESArt: classic gray box, red accents, dark top.
func drawNESArt() *ebiten.Image {
	img := newArt()
	// Body
	fill(img, 30, 40, 260, 150, colGreyMid)
	// Top ridge
	fill(img, 30, 40, 260, 14, colGreyDark)
	// Cartridge slot
	fill(img, 110, 54, 100, 18, colBlack)
	// Front plate
	fill(img, 50, 86, 220, 92, colGreyLight)
	// Power + reset buttons
	fill(img, 70, 100, 34, 20, colRedAcc)
	fill(img, 70, 130, 34, 20, colGreyDark)
	// Controller ports
	fill(img, 210, 96, 44, 12, colBlack)
	fill(img, 210, 118, 44, 12, colBlack)
	// Label stripe
	fill(img, 120, 160, 100, 10, colGreyDark)
	return img
}

// drawSNESArt: purple/gray two-tone, rounded-ish, eject flap.
func drawSNESArt() *ebiten.Image {
	img := newArt()
	// Body
	fill(img, 20, 60, 280, 130, colVioletAcc)
	fill(img, 20, 60, 280, 50, color.RGBA{0x6a, 0x3a, 0xa8, 0xff})
	// Cartridge slot
	fill(img, 40, 70, 240, 26, colBlack)
	// Eject buttons
	fill(img, 60, 108, 26, 14, colGreyLight)
	fill(img, 234, 108, 26, 14, colGreyLight)
	// Controller ports
	fill(img, 90, 140, 60, 12, colBlack)
	fill(img, 170, 140, 60, 12, colBlack)
	// Brand strip
	fill(img, 110, 164, 100, 12, color.RGBA{0x4a, 0x2a, 0x7e, 0xff})
	return img
}

// drawGBArt: classic Game Boy — grey, green screen, d-pad, A/B.
func drawGBArt() *ebiten.Image {
	img := newArt()
	// Body
	fill(img, 60, 20, 200, 190, colGBGrey)
	// Screen bezel
	fill(img, 90, 36, 140, 104, colGreyDark)
	// Screen
	fill(img, 100, 46, 120, 84, colScreenGB)
	// Screen glare
	fillRect(img, 104, 50, 60, 10, color.RGBA{0xd0, 0xe8, 0xa0, 0x60})
	// D-pad
	fill(img, 96, 160, 26, 44, colBlack)
	fill(img, 83, 173, 52, 18, colBlack)
	// A/B buttons
	fill(img, 168, 168, 20, 20, color.RGBA{0x7a, 0x1a, 0x4e, 0xff})
	fill(img, 196, 182, 20, 20, color.RGBA{0x7a, 0x1a, 0x4e, 0xff})
	// Start/Select
	fill(img, 128, 192, 16, 6, colBlack)
	fill(img, 150, 192, 16, 6, colBlack)
	// Speaker lines
	for i := 0; i < 4; i++ {
		fill(img, 212+i*12, 160, 6, 44, colGreyDark)
	}
	return img
}

// drawGenesisArt: black slab, red/gold stripe, blue buttons.
func drawGenesisArt() *ebiten.Image {
	img := newArt()
	// Body
	fill(img, 10, 70, 300, 110, colBlack)
	// Cartridge slot
	fill(img, 30, 40, 160, 34, colGreyDark)
	// Stripe
	fill(img, 30, 96, 260, 16, colGenRed)
	fill(img, 30, 112, 260, 8, color.RGBA{0xf0, 0xc0, 0x3a, 0xff})
	// Buttons
	fill(img, 210, 84, 16, 16, colGenBlue)
	fill(img, 236, 84, 16, 16, colGenBlue)
	// Power LED
	fill(img, 190, 60, 8, 8, color.RGBA{0xff, 0x40, 0x40, 0xff})
	return img
}

// consoleArt builds the art image for a console id.
func consoleArt(id string) *ebiten.Image {
	switch id {
	case "nes":
		return drawNESArt()
	case "snes":
		return drawSNESArt()
	case "gb":
		return drawGBArt()
	case "genesis":
		return drawGenesisArt()
	}
	return drawNESArt()
}

// fill fills a rect inside an art image (topleft coords).
func fill(img *ebiten.Image, x, y, w, h int, c color.Color) {
	fillRect(img, x, y, w, h, c)
}
