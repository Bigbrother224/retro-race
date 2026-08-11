import CRetroRace
import Foundation

// MARK: - Silent ghost process

/// Runs the core with the video callback set to NULL (renders nothing),
/// advances `frames`, and dumps the save state to --out.
/// This is the building block for the Ghost: a second process that replays an
/// opponent's inputs deterministically without paying for rendering.
func runGhost(_ args: [String]) {
    var outPath: String? = nil
    var frames = 1200
    var holdFrom: Int? = nil

    var i = 0
    while i < args.count {
        switch args[i] {
        case "--out": outPath = args[safe: i + 1]; i += 2
        case "--frames": frames = Int(args[safe: i + 1] ?? "1200") ?? 1200; i += 2
        case "--hold-from": holdFrom = Int(args[safe: i + 1] ?? "0") ?? 0; i += 2
        default: i += 1
        }
    }

    let rom = loadROM(romPath)
    loadCore()

    let start = CFAbsoluteTimeGetCurrent()

    // Video callback NULL -> the core skips rendering. This is the "silent
    // ghost" mode: deterministic logic only, minimal CPU cost.
    rr_set_video(rr_video(on_frame: nil, user: nil))
    if let hf = holdFrom {
        attachInput(ScriptedInput([(hf, [.right])]))
    } else {
        attachInput(ScriptedInput([]))
    }

    guard rr_load_game((rom as NSData).bytes, rom.count) == 0 else {
        print("[ghost] load failed")
        exit(1)
    }

    for _ in 0..<frames {
        rr_run()
    }

    let elapsed = CFAbsoluteTimeGetCurrent() - start
    let effectiveFps = Double(frames) / elapsed
    print("[ghost] ran \(frames) frames in \(String(format: "%.3f", elapsed))s -> \(String(format: "%.0f", effectiveFps)) fps (silent, video=NULL)")

    if let outPath {
        guard let state = serializeState() else {
            print("[ghost] serialize failed")
            exit(1)
        }
        try? state.write(to: URL(fileURLWithPath: outPath))
        print("[ghost] state -> \(outPath) (\(state.count) bytes)")
    }

    rr_unload_game()
    rr_unload()
}
