import AppKit
import Foundation
import RetroRaceCore
import SwiftUI

/// The library + launcher screen. Scans a ROM folder, lists detected games,
/// and starts a simulated local Race (player core + silent ghost via shm).
final class LibraryViewModel: ObservableObject {
    @Published var games: [GameEntry] = []
    @Published var isScanning = false
    @Published var errorMessage: String?
    @Published var runningGame: GameEntry?

    /// Default scan location: app/Roms (bundled homebrew), overridable.
    var romsDirectory: URL {
        let env = ProcessInfo.processInfo.environment["RETRO_RACE_ROMS"]
        if let env, !env.isEmpty {
            return URL(fileURLWithPath: env)
        }
        return URL(fileURLWithPath: "/Users/mac/retro-race/app/Roms")
    }

    var profilesDirectory: URL {
        URL(fileURLWithPath: "/Users/mac/retro-race/app/Profiles")
    }

    func rescan() {
        isScanning = true
        errorMessage = nil
        games = LocalLibrary.scan(romsDirectory)
        isScanning = false
    }

    /// Returns the calibration profile for a game, if one exists locally.
    func calibrationProfile(for game: GameEntry) -> CalibrationProfile? {
        CalibrationProfile.find(in: profilesDirectory, for: game)
    }

    func startRace(_ game: GameEntry) {
        errorMessage = nil
        // Calibration may be absent: RaceSession then uses the core defaults.
        runningGame = game
    }

    func stopRace() {
        runningGame = nil
    }
}

/// Renders an RGBA buffer + a tinted ghost tile at position, no window
/// management (draws into the SwiftUI view).
struct GameCanvasView: NSViewRepresentable {
    var frame: [UInt8]
    var width: Int
    var height: Int
    var ghostX: Int
    var ghostY: Int
    var ghostTile: [UInt8]
    var ghostVisible: Bool

    func makeNSView(context: Context) -> GameCanvas {
        GameCanvas()
    }

    func updateNSView(_ nsView: GameCanvas, context: Context) {
        nsView.pixelBuffer = frame
        nsView.width = width
        nsView.height = height
        nsView.ghostX = ghostX
        nsView.ghostY = ghostY
        nsView.ghostTile = ghostTile
        nsView.ghostVisible = ghostVisible
        nsView.needsDisplay = true
    }
}

final class GameCanvas: NSView {
    var pixelBuffer: [UInt8] = []
    var width = 256
    var height = 240
    var ghostX = 0
    var ghostY = 0
    var ghostTile: [UInt8] = []
    var ghostVisible = false

    override func draw(_ dirtyRect: NSRect) {
        guard let ctx = NSGraphicsContext.current?.cgContext else { return }
        ctx.setFillColor(NSColor.black.cgColor)
        ctx.fill(bounds)

        if !pixelBuffer.isEmpty, let img = makeCGImage(rgba: pixelBuffer, width: width, height: height) {
            let scale = min(bounds.width / CGFloat(img.width),
                            bounds.height / CGFloat(img.height))
            let w = CGFloat(img.width) * scale
            let h = CGFloat(img.height) * scale
            let rect = NSRect(x: (bounds.width - w) / 2, y: (bounds.height - h) / 2, width: w, height: h)
            ctx.interpolationQuality = .none
            ctx.draw(img, in: rect)

            if ghostVisible, !ghostTile.isEmpty,
               let ghostImg = tintGhostTile() {
                let tilePx = 32
                let tw = CGFloat(tilePx) * scale
                let th = CGFloat(tilePx) * scale
                let ghostRect = NSRect(x: CGFloat(ghostX) * scale - tw / 2,
                                       y: bounds.height - CGFloat(ghostY) * scale - th / 2,
                                       width: tw, height: th)
                ctx.setAlpha(0.6)
                ctx.draw(ghostImg, in: ghostRect)
                ctx.setAlpha(1.0)
            }
        }
    }

    private func tintGhostTile() -> CGImage? {
        guard !ghostTile.isEmpty else { return nil }
        let n = ghostTile.count
        var tinted = [UInt8](repeating: 0, count: n)
        for i in stride(from: 0, to: n, by: 4) {
            guard ghostTile[i + 3] > 8 else { continue }
            let lum = (Int(ghostTile[i]) * 3 + Int(ghostTile[i + 1]) * 4 + Int(ghostTile[i + 2]) * 1) / 8
            tinted[i + 0] = UInt8(min(255, lum * 6 / 10 + 40))
            tinted[i + 1] = UInt8(min(255, lum * 2 / 10))
            tinted[i + 2] = UInt8(min(255, lum * 9 / 10 + 30))
            tinted[i + 3] = 255
        }
        return makeCGImage(rgba: tinted, width: 32, height: 32)
    }
}