import CRetroRace
import Foundation

// MARK: - Framebuffer conversion (all libretro pixel formats)

/// Converts a core framebuffer row into tightly-packed RGBA8. Handles the
/// three libretro pixel formats (0RGB1555, XRGB8888, RGB565). `pitch` is the
/// bytes-per-row the core reported; `bytesPerPixel` must match the format.
func convertFramebuffer(src: UnsafePointer<UInt8>, width: Int, height: Int,
                        pitch: Int, bytesPerPixel: Int, format: Int) -> [UInt8] {
    let rowBytes = width * 4
    var out = [UInt8](repeating: 0, count: rowBytes * height)

    out.withUnsafeMutableBufferPointer { dst in
        for y in 0..<height {
            let s = src + y * pitch
            let d = dst.baseAddress! + y * rowBytes
            for x in 0..<width {
                switch bytesPerPixel {
                case 4:
                    // XRGB8888: memory order B,G,R,X (little-endian 0x00RRGGBB)
                    let b = s[x * 4 + 0]
                    let g = s[x * 4 + 1]
                    let r = s[x * 4 + 2]
                    d[x * 4 + 0] = r
                    d[x * 4 + 1] = g
                    d[x * 4 + 2] = b
                    d[x * 4 + 3] = 255
                case 2:
                    // RGB565: 16-bit little-endian rrrrrggggggbbbbb
                    let raw = UInt16(s[x * 2 + 0]) | (UInt16(s[x * 2 + 1]) << 8)
                    let r5 = (raw >> 11) & 0x1F
                    let g6 = (raw >> 5) & 0x3F
                    let b5 = raw & 0x1F
                    d[x * 4 + 0] = UInt8((r5 << 3) | (r5 >> 2))
                    d[x * 4 + 1] = UInt8((g6 << 2) | (g6 >> 4))
                    d[x * 4 + 2] = UInt8((b5 << 3) | (b5 >> 2))
                    d[x * 4 + 3] = 255
                default:
                    break
                }
            }
        }
    }
    _ = format
    return out
}

/// Current framebuffer layout as reported by the shim.
struct FramebufferLayout {
    let format: Int        // rr_pixel_format(): 0=0RGB1555, 1=XRGB8888, 2=RGB565
    let bytesPerPixel: Int

    static var current: FramebufferLayout {
        let fmt = Int(rr_pixel_format())
        let bpp = (fmt == 2) ? 2 : 4   // RGB565 = 2, 0RGB1555/XRGB8888 = 4
        return FramebufferLayout(format: fmt, bytesPerPixel: bpp)
    }
}