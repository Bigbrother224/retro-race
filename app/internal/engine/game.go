package engine

// Pixel format ids, mirroring the C shim's rr_pixel_format values.
const (
	PixelFormat1555   = 0
	PixelFormatXRGB   = 1
	PixelFormatRGB565 = 2
)

// ToRGBAInto converts a raw core framebuffer into tightly packed RGBA8,
// writing into the caller-provided buffer (reused across frames).
// bpp is the source bytes-per-pixel (2 for RGB565/0RGB1555, 4 for XRGB8888).
func ToRGBAInto(src []byte, bpp int, format int, out []byte) {
	switch {
	case bpp == 2 && format == int(PixelFormatRGB565):
		for i, j := 0, 0; i+1 < len(src) && j+3 < len(out); i, j = i+2, j+4 {
			raw := uint16(src[i]) | uint16(src[i+1])<<8
			r5 := (raw >> 11) & 0x1F
			g6 := (raw >> 5) & 0x3F
			b5 := raw & 0x1F
			out[j+0] = byte((r5 << 3) | (r5 >> 2))
			out[j+1] = byte((g6 << 2) | (g6 >> 4))
			out[j+2] = byte((b5 << 3) | (b5 >> 2))
			out[j+3] = 255
		}
	case bpp == 2: // 0RGB1555
		for i, j := 0, 0; i+1 < len(src) && j+3 < len(out); i, j = i+2, j+4 {
			raw := uint16(src[i]) | uint16(src[i+1])<<8
			r5 := (raw >> 10) & 0x1F
			g5 := (raw >> 5) & 0x1F
			b5 := raw & 0x1F
			out[j+0] = byte((r5 << 3) | (r5 >> 2))
			out[j+1] = byte((g5 << 3) | (g5 >> 2))
			out[j+2] = byte((b5 << 3) | (b5 >> 2))
			out[j+3] = 255
		}
	default: // XRGB8888 (4 bpp)
		for i, j := 0, 0; i+3 < len(src) && j+3 < len(out); i, j = i+4, j+4 {
			b := src[i+0]
			g := src[i+1]
			r := src[i+2]
			out[j+0] = r
			out[j+1] = g
			out[j+2] = b
			out[j+3] = 255
		}
	}
}
