import CoreGraphics
import Foundation

/// Efficient framebuffer → CGImage bridge. Keeps one bitmap context and
/// reuses it across frames, avoiding a full RGBA copy + CGImage allocation
/// every frame (the main cause of jank in the first UI).
public final class FrameRenderer {
    private var context: CGContext?
    private var contextSize = CGSize.zero

    public init() {}

    /// Returns a CGImage drawn from `rgba` (tightly packed RGBA8), reusing an
    /// internal bitmap context. The image is valid until the next call.
    public func image(from rgba: [UInt8], width: Int, height: Int) -> CGImage? {
        guard !rgba.isEmpty else { return nil }
        let size = CGSize(width: width, height: height)
        if context == nil || contextSize != size {
            var bytes = [UInt8](repeating: 0, count: width * height * 4)
            guard let ctx = bytes.withUnsafeMutableBytes({ raw in
                CGContext(data: raw.baseAddress,
                          width: width, height: height,
                          bitsPerComponent: 8, bytesPerRow: width * 4,
                          space: CGColorSpaceCreateDeviceRGB(),
                          bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)
            }) else { return nil }
            context = ctx
            contextSize = size
        }
        guard let context else { return nil }

        if let dest = context.data {
            memcpy(dest, rgba, width * height * 4)
        }
        return context.makeImage()
    }
}