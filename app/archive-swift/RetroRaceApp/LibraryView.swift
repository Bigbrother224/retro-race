import SwiftUI
import RetroRaceCore

struct LibraryView: View {
    @StateObject private var model = LibraryViewModel()

    var body: some View {
        Group {
            if let running = model.runningGame {
                RaceView(game: running, model: model)
            } else {
                libraryList
            }
        }
        .onAppear { model.rescan() }
    }

    private var libraryList: some View {
        VStack(spacing: 0) {
            header
            Divider()
            if model.games.isEmpty {
                emptyState
            } else {
                gameList
            }
        }
    }

    private var header: some View {
        HStack {
            Image(systemName: "gamecontroller.fill")
                .foregroundStyle(.tint)
            Text("Retro Race")
                .font(.headline)
            Spacer()
            if model.isScanning {
                ProgressView().controlSize(.small)
            }
            Button("Scanner", action: { model.rescan() })
        }
        .padding(12)
    }

    private var emptyState: some View {
        VStack(spacing: 10) {
            Spacer()
            Image(systemName: "opticaldisc")
                .font(.system(size: 44))
                .foregroundStyle(.secondary)
            Text("Aucun jeu détecté")
                .font(.title3)
            Text("Place des ROMs (NES, SNES, GB/GBC, Genesis) dans \(model.romsDirectory.path)")
                .font(.caption)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .padding(.horizontal, 40)
            Button("Scanner le dossier") { model.rescan() }
                .padding(.top, 4)
            Spacer()
        }
    }

    private var gameList: some View {
        List(model.games) { game in
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text(game.displayName)
                        .font(.body)
                    Text("\(game.extensions.uppercased()) · \(game.size) octets")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Button("Lancer") { model.startRace(game) }
            }
            .padding(.vertical, 2)
        }
    }
}

/// Player screen: runs the game locally (no ghost).
struct RaceView: View {
    let game: GameEntry
    @ObservedObject var model: LibraryViewModel
    @State private var status = "Démarrage…"
    @State private var errorText: String?

    private let session = RaceSession()
    private let renderer = FrameRenderer()
    @State private var displayLink: DisplayLinkDriver?
    @State private var gamepad: GamepadManager?
    @State private var aspect: CGFloat = 256.0 / 240.0
    @StateObject private var canvasBridge = CanvasBridge()

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Button("< Retour") { model.stopRace() }
                Spacer()
                Text(game.displayName)
                    .font(.headline)
                Spacer()
                if let errorText {
                    Text(errorText)
                        .font(.caption)
                        .foregroundStyle(.red)
                } else {
                    Text(status)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            .padding(10)

            GameCanvasView(input: session.input, bridge: canvasBridge)
                .aspectRatio(aspect, contentMode: .fit)
                .background(Color.black)

            Text("Flèches: direction · Z/X: A/B · A/S: Y/X · Entrée: Start · Maj: Select · Q/E: L/R")
                .font(.caption2)
                .foregroundStyle(.secondary)
                .padding(4)
        }
        .onAppear { start() }
        .onDisappear { displayLink?.stop(); gamepad?.stop(); session.stop() }
    }

    private func start() {
        status = "Démarrage…"
        let profile = model.calibrationProfile(for: game)
        if let profile {
            status = "Profil calibration chargé (offset 0x\(String(format: "%04x", profile.positionCandidate?.offset ?? 0)))"
        }

        let pad = GamepadManager(input: session.input)
        pad.start()
        gamepad = pad

        DispatchQueue.global(qos: .userInitiated).async {
            do {
                try session.start(rom: game.path)
                DispatchQueue.main.async {
                    aspect = CGFloat(session.width) / CGFloat(session.height)
                    status = "En jeu"
                    startDisplayLink()
                }
            } catch {
                DispatchQueue.main.async {
                    errorText = "Échec du chargement : \(error)"
                }
            }
        }
    }

    private func startDisplayLink() {
        let link = DisplayLinkDriver()
        let session = self.session
        let renderer = self.renderer
        let bridge = self.canvasBridge
        link.onFrame = {
            session.step()
            if let img = renderer.image(from: session.frame,
                                        width: session.width,
                                        height: session.height) {
                bridge.canvas?.setImage(img)
            }
        }
        link.start()
        displayLink = link
    }
}