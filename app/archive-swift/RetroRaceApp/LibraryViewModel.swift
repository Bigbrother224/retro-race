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

/// Shares the live GameCanvas instance with the parent view.
final class CanvasBridge: ObservableObject {
    weak var canvas: GameCanvas?
}

/// Renders the game framebuffer as a layer-backed view. The parent pushes
/// CGImages directly (no per-frame draw / no SwiftUI copies).
struct GameCanvasView: NSViewRepresentable {
    var input: PlayerInput?
    var bridge: CanvasBridge?

    func makeNSView(context: Context) -> GameCanvas {
        let canvas = GameCanvas()
        canvas.input = input
        bridge?.canvas = canvas
        return canvas
    }

    func updateNSView(_ nsView: GameCanvas, context: Context) {
        nsView.input = input
        bridge?.canvas = nsView
    }
}

final class GameCanvas: NSView {
    var input: PlayerInput?

    override init(frame frameRect: NSRect) {
        super.init(frame: frameRect)
        wantsLayer = true
        layer?.contentsGravity = .resizeAspect
    }

    required init?(coder: NSCoder) {
        super.init(coder: coder)
        wantsLayer = true
        layer?.contentsGravity = .resizeAspect
    }

    override var acceptsFirstResponder: Bool { true }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        window?.makeFirstResponder(self)
    }

    func setImage(_ image: CGImage) {
        layer?.contents = image
    }

    override func keyDown(with event: NSEvent) {
        if let button = keyMapping[event.keyCode] {
            input?.set(button, pressed: true)
        }
    }

    override func keyUp(with event: NSEvent) {
        if let button = keyMapping[event.keyCode] {
            input?.set(button, pressed: false)
        }
    }

    override func flagsChanged(with event: NSEvent) {
        input?.set(.select, pressed: event.modifierFlags.contains(.shift))
    }
}

/// SNES-style keyboard mapping (immutable lookup table, safe to share).
nonisolated(unsafe) private let keyMapping: [UInt16: Joypad] = [
    126: .up,     // arrow up
    125: .down,   // arrow down
    123: .left,   // arrow left
    124: .right,  // arrow right
    6:   .a,      // Z -> A
    7:   .b,      // X -> B
    0:   .y,      // A -> Y
    1:   .x,      // S -> X
    36:  .start,  // Return -> Start
    56:  .select, // Shift -> Select
    12:  .l,      // Q -> L
    14:  .r,      // E -> R
]
