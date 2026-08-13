package app

import (
	"image/color"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// Pixel-art console illustrations, drawn in code. Each console is drawn once
// into its own image (cached), then scaled/blended by the carousel.

const artW, artH = 400, 280

var (
	colGreyDark   = color.RGBA{0x2e, 0x2e, 0x36, 0xff}
	colGreyMid    = color.RGBA{0x4c, 0x4c, 0x56, 0xff}
	colGreyLight  = color.RGBA{0x74, 0x74, 0x82, 0xff}
	colBlack      = color.RGBA{0x14, 0x14, 0x18, 0xff}
	colScreen     = color.RGBA{0x9a, 0xd1, 0x7e, 0xff}
	colScreenGB   = color.RGBA{0x9b, 0xbc, 0x5a, 0xff}
	colRedAcc     = color.RGBA{0xe0, 0x3a, 0x2c, 0xff}
	colVioletAcc  = color.RGBA{0x8a, 0x4f, 0xce, 0xff}
	colVioletDark = color.RGBA{0x4a, 0x2a, 0x7e, 0xff}
	colGenBlue    = color.RGBA{0x2a, 0x7a, 0xd4, 0xff}
	colGenRed     = color.RGBA{0xe0, 0x3a, 0x2c, 0xff}
	colGenGold    = color.RGBA{0xf0, 0xc0, 0x3a, 0xff}
	colGBGrey     = color.RGBA{0x9c, 0x9c, 0xa8, 0xff}
	colShadow     = color.RGBA{0x08, 0x06, 0x0c, 0xff}
)

func newArt() *ebiten.Image {
	return ebiten.NewImage(artW, artH)
}

// bevel adds a top highlight and bottom shadow strip for a chunky 3D edge.
func bevel(img *ebiten.Image, x, y, w, h, light, dark int, c color.Color) {
	fill(img, x, y, w, light, lighten(c))
	fill(img, x, y+h-dark, w, dark, darken(c))
}

func lighten(c color.Color) color.Color {
	r, g, b, a := c.RGBA()
	return color.RGBA{clamp8(int(r>>8) + 30), clamp8(int(g>>8) + 30), clamp8(int(b>>8) + 30), uint8(a >> 8)}
}

func darken(c color.Color) color.Color {
	r, g, b, a := c.RGBA()
	return color.RGBA{clamp8(int(r>>8) - 30), clamp8(int(g>>8) - 30), clamp8(int(b>>8) - 30), uint8(a >> 8)}
}

func clamp8(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// drawNESArt: classic gray front-loading box, red accents.
func drawNESArt() *ebiten.Image {
	img := newArt()
	// Body slab with bevel.
	fill(img, 70, 62, 260, 168, colGreyMid)
	bevel(img, 70, 62, 260, 168, 10, 8, colGreyMid)
	// Top-loading ridge.
	fill(img, 70, 62, 260, 22, colGreyDark)
	// Cartridge slot.
	fill(img, 148, 84, 104, 22, colBlack)
	fill(img, 148, 84, 104, 7, colGreyLight)
	// Front plate.
	fill(img, 94, 116, 212, 114, colGreyLight)
	bevel(img, 94, 116, 212, 114, 8, 6, colGreyLight)
	// Power + reset buttons.
	fill(img, 120, 132, 42, 24, colRedAcc)
	fill(img, 120, 132, 42, 5, lighten(colRedAcc))
	fill(img, 120, 166, 42, 24, colGreyDark)
	fill(img, 120, 166, 42, 5, lighten(colGreyDark))
	// Controller ports.
	fill(img, 252, 136, 46, 13, colBlack)
	fill(img, 252, 160, 46, 13, colBlack)
	// Label stripe.
	fill(img, 150, 214, 112, 12, colGreyDark)
	// Shadow under body.
	fill(img, 74, 224, 252, 12, colShadow)
	return img
}

// drawSNESArt: purple/gray two-tone, eject flap, brand strip.
func drawSNESArt() *ebiten.Image {
	img := newArt()
	// Body.
	fill(img, 50, 70, 300, 140, colVioletAcc)
	bevel(img, 50, 70, 300, 140, 10, 8, colVioletAcc)
	// Darker top half.
	fill(img, 50, 70, 300, 54, color.RGBA{0x6a, 0x3a, 0xa8, 0xff})
	// Cartridge slot (open).
	fill(img, 70, 84, 260, 30, colBlack)
	fill(img, 70, 84, 260, 6, colGreyLight)
	// Eject buttons.
	fill(img, 82, 128, 30, 16, colGreyLight)
	fill(img, 288, 128, 30, 16, colGreyLight)
	// Controller ports.
	fill(img, 118, 156, 60, 13, colBlack)
	fill(img, 222, 156, 60, 13, colBlack)
	// Brand strip.
	fill(img, 140, 182, 120, 13, colVioletDark)
	// Bevel shadow bottom.
	fill(img, 54, 204, 292, 10, colShadow)
	return img
}

// drawGBArt: classic Game Boy — grey, green screen, d-pad, A/B.
func drawGBArt() *ebiten.Image {
	img := newArt()
	// Body.
	fill(img, 100, 30, 200, 220, colGBGrey)
	bevel(img, 100, 30, 200, 220, 12, 10, colGBGrey)
	// Screen bezel.
	fill(img, 128, 52, 144, 118, colGreyDark)
	bevel(img, 128, 52, 144, 118, 8, 8, colGreyDark)
	// Screen.
	fill(img, 140, 64, 120, 90, colScreenGB)
	// Screen glare.
	fillRect(img, 146, 70, 62, 12, color.RGBA{0xd0, 0xe8, 0xa0, 0x66})
	// D-pad.
	fill(img, 136, 176, 28, 48, colBlack)
	fill(img, 122, 190, 56, 20, colBlack)
	// A/B buttons.
	fill(img, 212, 184, 22, 22, color.RGBA{0x7a, 0x1a, 0x4e, 0xff})
	fill(img, 244, 200, 22, 22, color.RGBA{0x7a, 0x1a, 0x4e, 0xff})
	// Start/Select.
	fill(img, 174, 210, 18, 6, colBlack)
	fill(img, 198, 210, 18, 6, colBlack)
	// Speaker slots.
	for i := range 4 {
		fill(img, 262+i*12, 178, 6, 48, colGreyDark)
	}
	return img
}

// drawGenesisArt: black slab, red/gold stripe, blue buttons.
func drawGenesisArt() *ebiten.Image {
	img := newArt()
	// Body.
	fill(img, 40, 80, 320, 130, colBlack)
	bevel(img, 40, 80, 320, 130, 10, 8, colBlack)
	// Cartridge slot.
	fill(img, 70, 52, 170, 36, colGreyDark)
	fill(img, 70, 52, 170, 6, colGreyLight)
	// Red/gold stripe.
	fill(img, 60, 110, 280, 18, colGenRed)
	fill(img, 60, 128, 280, 8, colGenGold)
	// Buttons.
	fill(img, 236, 96, 18, 18, colGenBlue)
	fill(img, 266, 96, 18, 18, colGenBlue)
	// Power LED.
	fill(img, 220, 70, 10, 10, color.RGBA{0xff, 0x40, 0x40, 0xff})
	// Shadow under body.
	fill(img, 44, 204, 312, 14, colShadow)
	return img
}

// consoleArt returns the (cached) art image for a console id.
func consoleArt(id string) *ebiten.Image {
	if v, ok := artCache.Load(id); ok {
		return v.(*ebiten.Image)
	}
	var img *ebiten.Image
	switch id {
	case "nes":
		img = drawNESArt()
	case "snes":
		img = drawSNESArt()
	case "gb":
		img = drawGBArt()
	case "genesis":
		img = drawGenesisArt()
	default:
		img = drawNESArt()
	}
	artCache.Store(id, img)
	return img
}

// fill fills a rect inside an art image (topleft coords).
func fill(img *ebiten.Image, x, y, w, h int, c color.Color) {
	fillRect(img, x, y, w, h, c)
}

var artCache sync.Map // string -> *ebiten.Image
