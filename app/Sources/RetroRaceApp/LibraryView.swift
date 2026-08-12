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

/// Simulated local Race: player core + silent ghost (shared memory).
struct RaceView: View {
    let game: GameEntry
    @ObservedObject var model: LibraryViewModel
    @State private var frame: [UInt8] = []
    @State private var width = 256
    @State private var height = 240
    @State private var ghostX = 0
    @State private var ghostY = 0
    @State private var ghostTile: [UInt8] = []
    @State private var ghostVisible = false
    @State private var status = "Démarrage…"
    @State private var errorText: String?

    private let session = RaceSession()
    @State private var timer: Timer?

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

            GameCanvasView(frame: frame, width: width, height: height,
                           ghostX: ghostX, ghostY: ghostY,
                           ghostTile: ghostTile, ghostVisible: ghostVisible)
                .aspectRatio(CGFloat(width) / CGFloat(height), contentMode: .fit)
                .background(Color.black)

            Text("Ghost : \(ghostVisible ? "visible" : "en attente du processus ghost…")")
                .font(.caption)
                .foregroundStyle(.secondary)
                .padding(6)
        }
        .onAppear { start() }
        .onDisappear { timer?.invalidate(); session.stop() }
    }

    private func start() {
        status = "Démarrage…"
        let profile = model.calibrationProfile(for: game)
        if let profile {
            status = "Profil calibration chargé (offset 0x\(String(format: "%04x", profile.positionCandidate?.offset ?? 0)))"
        }

        // ghost-live child publishes position + sprite over shm
        let exe = executablePath()
        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: exe)
        proc.arguments = ["ghost-live", "--shm", "retro_race_ghost"]
        proc.standardOutput = Pipe()
        proc.standardError = Pipe()
        do {
            try proc.run()
        } catch {
            errorText = "Impossible de lancer ghost-live : \(error.localizedDescription)"
            return
        }

        DispatchQueue.global(qos: .userInitiated).async {
            do {
                try session.start(rom: game.path)
                DispatchQueue.main.async {
                    width = session.width
                    height = session.height
                    status = "En course"
                    startTimer()
                }
            } catch {
                DispatchQueue.main.async {
                    errorText = "Échec du chargement : \(error)"
                }
            }
        }
    }

    private func startTimer() {
        timer = Timer.scheduledTimer(withTimeInterval: 1.0 / 60.0, repeats: true) { _ in
            session.step()
            frame = session.frame
            width = session.width
            height = session.height
            let g = session.ghostState
            ghostX = g.x
            ghostY = g.y
            ghostTile = g.tile
            ghostVisible = g.visible
        }
    }

    private func executablePath() -> String {
        // App and CLI live side by side; reuse the CLI binary to spawn ghost-live.
        let base = Bundle.main.bundleURL
        if let exe = Bundle.main.executablePath {
            let dir = (exe as NSString).deletingLastPathComponent
            let cli = (dir as NSString).appendingPathComponent("RetroRaceCLI")
            if FileManager.default.fileExists(atPath: cli) { return cli }
            return base.appendingPathComponent("RetroRaceCLI").path
        }
        return "RetroRaceCLI"
    }
}