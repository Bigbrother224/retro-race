import AppKit
import CRetroRace
import Foundation

// MARK: - Phase 1: local Ghost lab (launcher + shared memory)

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

/// Draws the ghost sprite (from shared memory) tinted purple, on the
/// transparent click-through overlay.
final class GhostOverlayView: NSView {
    var ghostX: CGFloat = 0
    var ghostY: CGFloat = 0
    var ghostVisible = false
    private var tileRGBA: [UInt8] = []
    private let tilePx = Int(RR_SHM_TILE_W)

    func setGhost(x: Int, y: Int, tile: [UInt8]) {
        ghostX = CGFloat(x)
        ghostY = CGFloat(y)
        tileRGBA = tile
        ghostVisible = !tile.isEmpty
        needsDisplay = true
    }

    override func draw(_ dirtyRect: NSRect) {
        NSColor.clear.setFill()
        dirtyRect.fill()
        guard ghostVisible, !tileRGBA.isEmpty else { return }

        let scale = bounds.width / 256
        let w = CGFloat(tilePx) * scale
        let h = CGFloat(tilePx) * scale
        let rect = NSRect(x: ghostX * scale - w / 2,
                          y: bounds.height - ghostY * scale - h / 2,
                          width: w, height: h)

        guard let tinted = makeTintedImage(rgba: tileRGBA, w: tilePx, h: tilePx) else {
            return
        }
        if let ctx = NSGraphicsContext.current?.cgContext {
            ctx.interpolationQuality = .none
            ctx.saveGState()
            // race color: purple ghost, 60% opacity
            ctx.setAlpha(0.6)
            ctx.draw(tinted, in: rect)
            ctx.restoreGState()
        }
    }

    /// Recolors non-transparent pixels toward the player's race color (purple).
    private func makeTintedImage(rgba: [UInt8], w: Int, h: Int) -> CGImage? {
        guard !rgba.isEmpty else { return nil }
        var tinted = [UInt8](repeating: 0, count: rgba.count)
        for i in stride(from: 0, to: rgba.count, by: 4) {
            let a = rgba[i + 3]
            guard a > 8 else { continue }
            let lum = (Int(rgba[i]) * 3 + Int(rgba[i + 1]) * 4 + Int(rgba[i + 2]) * 1) / 8
            // purple tint: keep luminance, push hue toward 270° (red 0.6, green 0.2, blue 0.9)
            tinted[i + 0] = UInt8(min(255, lum * 6 / 10 + 40))
            tinted[i + 1] = UInt8(min(255, lum * 2 / 10))
            tinted[i + 2] = UInt8(min(255, lum * 9 / 10 + 30))
            tinted[i + 3] = 255
        }
        return makeCGImage(rgba: tinted, width: w, height: h)
    }
}

/// Launcher: spawns the silent ghost (ghost-live) and shows the player window
/// with the ghost composited from shared memory.
///
/// Player process A runs its own core (your game, your controller). Ghost
/// process B (ghost-live) is spawned as a child, replays inputs, publishes
/// position + sprite tile over shared memory. A transparent click-through
/// window draws the tinted ghost above the player framebuffer.
func runOverlay() {
    setbuf(stdout, nil)

    let shmName = "retro_race_ghost"

    // --- spawn ghost-live child ---
    let exe = CommandLine.arguments[0]
    let ghostProc = Process()
    ghostProc.executableURL = URL(fileURLWithPath: exe)
    ghostProc.arguments = ["ghost-live", "--shm", shmName]
    let ghostOut = Pipe()
    ghostProc.standardOutput = ghostOut
    ghostProc.standardError = ghostOut
    do {
        try ghostProc.run()
    } catch {
        print("[overlay] cannot spawn ghost-live: \(error)")
        exit(1)
    }

    // --- player core (own instance) ---
    let rom = loadROM(romPath)
    loadCore()

    let sink = OverlaySink()
    let sinkHandle = Unmanaged.passUnretained(sink).toOpaque()
    let videoCallback: @convention(c) (rr_frame, UnsafeMutableRawPointer?) -> Void = { frame, user in
        guard let user else { return }
        Unmanaged<OverlaySink>.fromOpaque(user).takeUnretainedValue().onFrame(frame)
    }
    rr_set_video(rr_video(on_frame: videoCallback, user: sinkHandle))
    attachInput(ScriptedInput([]))
    guard rr_load_game((rom as NSData).bytes, rom.count) == 0 else {
        print("FATAL: rr_load_game failed")
        exit(1)
    }

    // --- shared memory (consumer side) ---
    var shm = rr_shm()
    var attached = false
    var lastFrame: UInt32 = UInt32.max

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

    var tick = 0
    let timer = Timer.scheduledTimer(withTimeInterval: 1.0 / 60.0, repeats: true) { _ in
        tick += 1
        rr_run()

        if sink.width > 0, let cg = makeCGImage(rgba: sink.rgba, width: sink.width, height: sink.height) {
            gameView.setImage(cg)
        }

        // attach to shm lazily (ghost-live may still be booting)
        if !attached {
            if rr_shm_open_named(shmName, false, &shm) == 0 {
                attached = true
                print("[overlay] attached to shm \(shmName)")
            }
        }

        if attached {
            var slot = rr_shm_slot()
            if rr_shm_take(&shm, &lastFrame, &slot) {
                let tileCount = Int(RR_SHM_TILE_W) * Int(RR_SHM_TILE_H) * 4
                var tile = [UInt8](repeating: 0, count: tileCount)
                tile.withUnsafeMutableBytes { buf in
                    rr_shm_slot_tile_copy(&slot, buf.baseAddress?.assumingMemoryBound(to: UInt8.self), UInt32(tileCount))
                }
                ghostView.setGhost(x: Int(slot.pos_x), y: Int(slot.pos_y), tile: tile)
            }
        }

        if tick % 120 == 0 {
            print("[overlay] tick=\(tick) attached=\(attached)")
        }
    }
    timer.tolerance = 0.002

    app.activate(ignoringOtherApps: true)
    app.run()
}

/// Builds a CGImage from an RGBA byte buffer already converted by OverlaySink.
public func makeCGImage(rgba: [UInt8], width: Int, height: Int) -> CGImage? {
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