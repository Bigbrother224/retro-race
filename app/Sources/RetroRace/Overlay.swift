import AppKit
import CRetroRace
import Foundation

// MARK: - Phase 0 overlay spike

/// Copies the libretro framebuffer (XRGB8888) into a tightly packed RGBA buffer.
final class OverlaySink: @unchecked Sendable {
    var width = 0
    var height = 0
    var pitch = 0
    var rgba: [UInt8] = []

    func onFrame(_ frame: rr_frame) {
        guard let fb = frame.framebuffer else { return }
        width = Int(frame.width)
        height = Int(frame.height)
        pitch = Int(frame.pitch)
        let rowBytes = width * 4
        rgba = [UInt8](repeating: 0, count: rowBytes * height)
        let src = fb.assumingMemoryBound(to: UInt8.self)
        rgba.withUnsafeMutableBufferPointer { dst in
            for y in 0..<height {
                let s = src + y * pitch
                let d = dst.baseAddress! + y * rowBytes
                for x in 0..<width {
                    let b = s[x * 4 + 0]
                    let g = s[x * 4 + 1]
                    let r = s[x * 4 + 2]
                    d[x * 4 + 0] = r
                    d[x * 4 + 1] = g
                    d[x * 4 + 2] = b
                    d[x * 4 + 3] = 255
                }
            }
        }
    }
}

final class GameFrameView: NSView {
    private var image: CGImage?
    private var clickCount = 0

    func setImage(_ img: CGImage) {
        image = img
        needsDisplay = true
    }

    override var acceptsFirstResponder: Bool { true }

    override func mouseDown(with event: NSEvent) {
        clickCount += 1
        window?.title = "Retro Race — game window (click-through check: \(clickCount) clicks)"
        super.mouseDown(with: event)
    }

    override func draw(_ dirtyRect: NSRect) {
        guard let image else {
            NSColor.black.setFill()
            dirtyRect.fill()
            return
        }
        let scale = min(bounds.width / CGFloat(image.width),
                        bounds.height / CGFloat(image.height))
        let w = CGFloat(image.width) * scale
        let h = CGFloat(image.height) * scale
        let rect = NSRect(x: (bounds.width - w) / 2, y: (bounds.height - h) / 2, width: w, height: h)
        if let ctx = NSGraphicsContext.current?.cgContext {
            ctx.interpolationQuality = .none
            ctx.draw(image, in: rect)
        }
    }
}

final class GhostOverlayView: NSView {
    /// Ghost tile center in *game pixels* (calibrated position, see runOverlay).
    var ghostX: CGFloat = 0
    var ghostY: CGFloat = 0
    var ghostVisible = false

    override func draw(_ dirtyRect: NSRect) {
        NSColor.clear.setFill()
        dirtyRect.fill()
        guard ghostVisible else { return }

        let tile: CGFloat = 16
        let scale = bounds.width / 256
        let w = tile * scale
        let h = tile * scale
        let rect = NSRect(x: ghostX * scale - w / 2,
                          y: bounds.height - ghostY * scale - h / 2,
                          width: w, height: h)

        let path = NSBezierPath(roundedRect: rect, xRadius: 4, yRadius: 4)
        NSColor(calibratedRed: 0.6, green: 0.2, blue: 0.9, alpha: 0.55).setFill()
        path.fill()
        NSColor(calibratedRed: 0.9, green: 0.6, blue: 1.0, alpha: 0.9).setStroke()
        path.lineWidth = 1.5
        path.stroke()
    }
}

/// Runs the player window + transparent click-through ghost overlay.
/// Window position: two aligned windows (parent + child overlay). The overlay
/// is borderless, fully transparent and ignores mouse events, so all input
/// reaches the game window underneath.
func runOverlay() {
    setbuf(stdout, nil)

    let rom = loadROM(romPath)
    loadCore()

    let sink = OverlaySink()
    let sinkHandle = Unmanaged.passUnretained(sink).toOpaque()
    let videoCallback: @convention(c) (rr_frame, UnsafeMutableRawPointer?) -> Void = { frame, user in
        guard let user else { return }
        Unmanaged<OverlaySink>.fromOpaque(user).takeUnretainedValue().onFrame(frame)
    }
    rr_set_video(rr_video(on_frame: videoCallback, user: sinkHandle))
    // Same input scheme as the committed calibration (RIGHT hold from frame
    // 120): proves the ghost bytes move, mirroring the calibration scenario.
    attachInput(ScriptedInput([(120, [.right])]))
    guard rr_load_game((rom as NSData).bytes, rom.count) == 0 else {
        print("FATAL: rr_load_game failed")
        exit(1)
    }

    let stateSize = rr_serialize_size()

    let app = NSApplication.shared
    app.setActivationPolicy(.regular)

    let contentRect = NSRect(x: 0, y: 0, width: 768, height: 720)
    let gameWindow = NSWindow(contentRect: contentRect,
                              styleMask: [.titled, .closable, .miniaturizable, .resizable],
                              backing: .buffered, defer: false)
    gameWindow.title = "Retro Race — Alter Ego (game window)"
    let gameView = GameFrameView(frame: contentRect)
    gameWindow.contentView = gameView
    gameWindow.center()
    gameWindow.makeKeyAndOrderFront(nil)

    let overlayWindow = NSWindow(contentRect: NSRect(origin: .zero, size: contentRect.size),
                                 styleMask: [.borderless],
                                 backing: .buffered, defer: false)
    overlayWindow.isOpaque = false
    overlayWindow.backgroundColor = .clear
    overlayWindow.hasShadow = false
    overlayWindow.ignoresMouseEvents = true
    overlayWindow.level = .floating
    let ghostView = GhostOverlayView(frame: NSRect(origin: .zero, size: contentRect.size))
    overlayWindow.contentView = ghostView
    gameWindow.addChildWindow(overlayWindow, ordered: .above)

    // Calibration (committed profile) located the position bytes at 0x0072.
    // In the spike we read that offset back each tick to feed the ghost — this
    // is the "read a few bytes per frame" idea, minus the shared-memory wiring.
    let offsets = [0x0072, 0x0073]
    func readPositionBytes() -> [UInt8] {
        var data = [UInt8](repeating: 0, count: stateSize)
        let ok = data.withUnsafeMutableBytes { rr_serialize($0.baseAddress, $0.count) == 0 }
        guard ok else { return [] }
        return offsets.map { $0 < data.count ? data[$0] : 0 }
    }

    var tick = 0
    let timer = Timer.scheduledTimer(withTimeInterval: 1.0 / 60.0, repeats: true) { _ in
        tick += 1
        rr_run()

        // player window
        if sink.width > 0, let cg = makeCGImage(rgba: sink.rgba, width: sink.width, height: sink.height) {
            gameView.setImage(cg)
        }

        // ghost overlay: derive a position from the calibrated bytes
        let bytes = readPositionBytes()
        if bytes.count == 2 {
            ghostView.ghostX = CGFloat(bytes[0])
            ghostView.ghostY = CGFloat(bytes[1])
            ghostView.ghostVisible = true
            ghostView.needsDisplay = true
        }

        if tick % 120 == 0 {
            print("[overlay] tick=\(tick) ghost bytes=\(bytes.map { String(format: "%02x", $0) })")
        }
    }
    timer.tolerance = 0.002

    app.activate(ignoringOtherApps: true)
    app.run()
}

/// Builds a CGImage from an RGBA byte buffer already converted by OverlaySink.
func makeCGImage(rgba: [UInt8], width: Int, height: Int) -> CGImage? {
    guard !rgba.isEmpty else { return nil }
    let data = Data(rgba) as CFData
    guard let provider = CGDataProvider(data: data) else { return nil }
    let colorSpace = CGColorSpaceCreateDeviceRGB()
    return CGImage(width: width, height: height,
                   bitsPerComponent: 8, bitsPerPixel: 32,
                   bytesPerRow: width * 4, space: colorSpace,
                   bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.noneSkipLast.rawValue),
                   provider: provider, decode: nil,
                   shouldInterpolate: false, intent: .defaultIntent)
}