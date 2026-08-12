import Foundation

/// `check-render --rom PATH [--frames N]`
///
/// Renders N frames and prints framebuffer size + color diversity — a quick
/// sanity check that the pixel format conversion is correct (not a green/
/// black screen, not uniform colors).
func runCheckRender(_ args: [String]) {
    var rom = romPath
    var frames = 60
    var i = 0
    while i < args.count {
        switch args[i] {
        case "--rom": rom = args[safe: i + 1] ?? rom; i += 2
        case "--frames": frames = Int(args[safe: i + 1] ?? "60") ?? 60; i += 2
        default: i += 1
        }
    }

    let session = RaceSession()
    do {
        try session.start(rom: rom)
    } catch {
        print("[check-render] start failed: \(error)")
        exit(1)
    }
    let layout = FramebufferLayout.current
    print("[check-render] pixel format \(layout.format) (0=1555, 1=XRGB8888, 2=RGB565), bpp \(layout.bytesPerPixel)")
    for _ in 0..<frames { session.step() }

    let f = session.frame
    var colors = Set<UInt32>()
    var nonBlack = 0
    for px in stride(from: 0, to: f.count, by: 4) {
        let r = Int(f[px]), g = Int(f[px + 1]), b = Int(f[px + 2])
        colors.insert(UInt32(r << 16 | g << 8 | b))
        if r + g + b > 30 { nonBlack += 1 }
    }
    print("[check-render] \(session.width)x\(session.height), distinct colors \(colors.count), non-black \(nonBlack)/\(f.count / 4)")

    // A valid frame shows a meaningful fraction of non-black pixels with a
    // reasonable palette (SNES intro screens are dark, so keep the bar low).
    let ok = colors.count > 8 && nonBlack > (f.count / 4) / 20
    print(ok ? "[check-render] PASS (couleurs correctes)" : "[check-render] FAIL (rendu douteux)")
    session.stop()
    exit(ok ? 0 : 1)
}