import CoreVideo
import Foundation

/// Drives the emulation loop at the display refresh rate (vsync) instead of
/// an imprecise Timer — the standard way libretro frontends pace frames.
final class DisplayLinkDriver: @unchecked Sendable {
    private var link: CVDisplayLink?
    var onFrame: (() -> Void)?

    init() {}

    func start() {
        CVDisplayLinkCreateWithActiveCGDisplays(&link)
        guard let link else { return }
        CVDisplayLinkSetOutputHandler(link) { [weak self] _, _, _, _, _ in
            DispatchQueue.main.async {
                self?.onFrame?()
            }
            return kCVReturnSuccess
        }
        CVDisplayLinkStart(link)
    }

    func stop() {
        if let link {
            CVDisplayLinkStop(link)
        }
    }
}