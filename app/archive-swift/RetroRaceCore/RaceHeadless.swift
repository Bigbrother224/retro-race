import Foundation

/// `race-headless [--frames N] [--shm NAME] [--rom PATH]`
///
/// Headless check of the RaceSession used by the launcher: starts the player
/// core + a ghost-live child (shared memory), runs N frames, and verifies the
/// framebuffer is non-empty and the ghost position arrives.
func runRaceHeadless(_ args: [String]) {
    var frames = 300
    var shmName = "retro_race_test"
    var rom = romPath
    var i = 0
    while i < args.count {
        switch args[i] {
        case "--frames": frames = Int(args[safe: i + 1] ?? "300") ?? 300; i += 2
        case "--shm": shmName = args[safe: i + 1] ?? shmName; i += 2
        case "--rom": rom = args[safe: i + 1] ?? rom; i += 2
        default: i += 1
        }
    }

    // ghost-live child
    let exe = CommandLine.arguments[0]
    let proc = Process()
    proc.executableURL = URL(fileURLWithPath: exe)
    proc.arguments = ["ghost-live", "--shm", shmName]
    proc.standardOutput = Pipe()
    proc.standardError = Pipe()
    do {
        try proc.run()
    } catch {
        print("[race-headless] cannot spawn ghost-live: \(error)")
        exit(1)
    }

    let session = RaceSession(shmName: shmName)
    do {
        try session.start(rom: rom)
    } catch {
        print("[race-headless] start failed: \(error)")
        proc.terminate()
        exit(1)
    }

    var framesWithPixels = 0
    var ghostSeen = false
    var lastGhost: (x: Int, y: Int) = (-1, -1)
    var ghostMoved = false
    var trailSeen = false

    for _ in 0..<frames {
        session.step()
        if !session.frame.isEmpty { framesWithPixels += 1 }
        let g = session.ghostState
        if g.visible {
            ghostSeen = true
            if lastGhost != (g.x, g.y) && lastGhost != (-1, -1) {
                ghostMoved = true
            }
            lastGhost = (g.x, g.y)
        }
        if !session.ghostTrail.isEmpty { trailSeen = true }
    }

    print("[race-headless] \(frames) frames, \(framesWithPixels) with pixels (\(session.width)x\(session.height))")
    print("[race-headless] ghost seen=\(ghostSeen) moved=\(ghostMoved) trail=\(trailSeen)(\(session.ghostTrail.count)pts) last=(\(lastGhost.0),\(lastGhost.1))")

    let ok = framesWithPixels > 0 && ghostSeen
    print(ok ? "[race-headless] PASS" : "[race-headless] FAIL")
    session.stop()
    proc.terminate()

    exit(ok ? 0 : 1)
}