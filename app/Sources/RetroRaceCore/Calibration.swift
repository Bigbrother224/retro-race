import CRetroRace
import CommonCrypto
import Foundation

// MARK: - Calibration

/// Runs two child processes (one per input script), each dumping a save state
/// at the same frame, then diffs the two states to locate the player's bytes.
func runCalibrate(_ args: [String]) {
    var frames = 600
    var jsonOut: URL? = nil
    var i = 0
    while i < args.count {
        switch args[i] {
        case "--frames": frames = Int(args[safe: i + 1] ?? "600") ?? frames; i += 2
        case "--json-out": jsonOut = URL(fileURLWithPath: args[safe: i + 1] ?? ""); i += 2
        default: i += 1
        }
    }

    print("== Calibration: find player position by state diff ==")
    print("two instances, identical ROM/core/start, different inputs, \(frames) frames each")

    let tmp = FileManager.default.temporaryDirectory
    let stateA = tmp.appendingPathComponent("cal_a_\(Int.random(in: 0..<1_000_000)).bin")
    let stateB = tmp.appendingPathComponent("cal_b_\(Int.random(in: 0..<1_000_000)).bin")
    defer {
        try? FileManager.default.removeItem(at: stateA)
        try? FileManager.default.removeItem(at: stateB)
    }

    let exe = CommandLine.arguments[0]

    // Instance A holds RIGHT from frame 120 (moves right).
    // Instance B holds LEFT from frame 120 (moves left / stands against wall).
    guard runChild(exe, state: stateA, hold: .right, from: 120, frames: frames),
          runChild(exe, state: stateB, hold: .left, from: 120, frames: frames) else {
        print("FAIL: calibration children failed")
        exit(1)
    }

    guard let dataA = try? Data(contentsOf: stateA),
          let dataB = try? Data(contentsOf: stateB) else {
        print("FAIL: cannot read child states")
        exit(1)
    }

    print("state A: \(dataA.count) bytes, state B: \(dataB.count) bytes")

    var diffRanges: [(start: Int, end: Int)] = []
    var current: (start: Int, end: Int)? = nil
    for i in 0..<min(dataA.count, dataB.count) where dataA[i] != dataB[i] {
        if var c = current, i == c.end {
            c.end = i + 1
            current = c
        } else {
            if let c = current { diffRanges.append(c) }
            current = (start: i, end: i + 1)
        }
    }
    if let c = current { diffRanges.append(c) }

    var total = 0
    for r in diffRanges { total += r.end - r.start }

    print("\ndiff bytes: \(total)/\(min(dataA.count, dataB.count))")
    for r in diffRanges.prefix(40) {
        let len = r.end - r.start
        let aBytes = dataA[r.start..<r.end].map { String(format: "%02x", $0) }.joined(separator: " ")
        let bBytes = dataB[r.start..<r.end].map { String(format: "%02x", $0) }.joined(separator: " ")
        print(String(format: "  [0x%04x, +%d)  A: %@  B: %@", r.start, len, aBytes, bBytes))
    }
    if diffRanges.count > 40 {
        print("  ... and \(diffRanges.count - 40) more ranges")
    }

    // Heuristic: the largest moving byte range is very likely the player's
    // position (Alter Ego is a puzzle platformer, the hero moves around).
    var positionCandidate: (offset: Int, span: Int, a: Int, b: Int, spread: Int)? = nil
    if let largest = diffRanges.max(by: { ($0.end - $0.start) < ($1.end - $1.start) }) {
        let aVal = Int(dataA[largest.start])
        let bVal = Int(dataB[largest.start])
        let spread = abs(aVal - bVal)
        positionCandidate = (largest.start, largest.end - largest.start, aVal, bVal, spread)
        print(String(format: "player position candidate: offset 0x%04x, span %d bytes, A=%d B=%d (spread %d)",
                     largest.start, largest.end - largest.start, aVal, bVal, spread))
    }

    if let jsonOut {
        writeCalibrationJSON(to: jsonOut, frames: frames, diffRanges: diffRanges,
                             dataA: dataA, dataB: dataB, position: positionCandidate)
    }
}

/// Writes the calibration result as a versioned Game Profile JSON — the
/// machine-readable artifact Phase 1 consumes.
func writeCalibrationJSON(to url: URL, frames: Int,
                          diffRanges: [(start: Int, end: Int)],
                          dataA: Data, dataB: Data,
                          position: (offset: Int, span: Int, a: Int, b: Int, spread: Int)?) {
    let ranges = diffRanges
        .sorted { $0.start < $1.start }
        .map { r in
            let a = dataA[r.start..<r.end].map { String(format: "%02x", $0) }.joined()
            let b = dataB[r.start..<r.end].map { String(format: "%02x", $0) }.joined()
            return ["offset": r.start, "span": r.end - r.start, "A": a, "B": b]
        }

    var profile: [String: Any] = [
        "schema": "retro-race/game-profile/calibration",
        "schema_version": 1,
        "game": "alter-ego",
        "core": "fceumm",
        "rom_hash_md5": "b12d0aefbde9b50eec53884d04d083b5",
        "rom_hash_sha256": sha256Hex(URL(fileURLWithPath: romPath)),
        "method": "save-state-diff",
        "input_scheme": ["A: RIGHT from frame 120", "B: LEFT from frame 120"],
        "frames": frames,
        "state_size": dataA.count,
        "diff_offsets": ranges,
        "generated_at": ISO8601DateFormatter().string(from: Date()),
    ]
    if let position {
        profile["position_candidate"] = [
            "offset": position.offset,
            "span": position.span,
            "spread": position.spread,
            "note": "largest diff range; heuristic, verify per game",
        ]
    }

    guard let d = try? JSONSerialization.data(withJSONObject: profile, options: [.prettyPrinted, .sortedKeys]),
          let str = String(data: d, encoding: .utf8) else {
        print("FAIL: cannot serialize calibration JSON")
        exit(1)
    }
    do {
        try str.write(to: url, atomically: true, encoding: .utf8)
        print("wrote calibration profile: \(url.path)")
    } catch {
        print("FAIL: cannot write calibration JSON: \(error)")
        exit(1)
    }
}

func sha256Hex(_ url: URL) -> String {
    guard let data = try? Data(contentsOf: url) else { return "" }
    var digest = [UInt8](repeating: 0, count: Int(CC_SHA256_DIGEST_LENGTH))
    data.withUnsafeBytes { buf in
        _ = CC_SHA256(buf.baseAddress, CC_LONG(data.count), &digest)
    }
    return digest.map { String(format: "%02x", $0) }.joined()
}

/// Spawns a child `calibrate-run` that runs the core with a scripted input and
/// dumps the save state to `statePath`.
func runChild(_ exe: String, state: URL, hold: Joypad, from: Int, frames: Int) -> Bool {
    let proc = Process()
    proc.executableURL = URL(fileURLWithPath: exe)
    proc.arguments = [
        "calibrate-run",
        "--out", state.path,
        "--hold", "\(hold.rawValue)",
        "--from", "\(from)",
        "--frames", "\(frames)",
    ]
    let pipe = Pipe()
    proc.standardOutput = pipe
    proc.standardError = pipe
    do {
        try proc.run()
        proc.waitUntilExit()
    } catch {
        print("child spawn failed: \(error)")
        return false
    }
    let data = pipe.fileHandleForReading.readDataToEndOfFile()
    if let out = String(data: data, encoding: .utf8) {
        // Echo child output for debugging.
        for line in out.split(separator: "\n") where line.contains("calibrate-run") {
            print("  [child] \(line)")
        }
    }
    return proc.terminationStatus == 0 && FileManager.default.fileExists(atPath: state.path)
}

/// Single calibration child: loads the core, applies a scripted hold, runs N
/// frames, writes the save state to --out.
func runCalibrateRun(_ args: [String]) {
    var outPath: String? = nil
    var holdButton: UInt32 = 0
    var holdFrom = 0
    var frames = 600

    var i = 0
    while i < args.count {
        switch args[i] {
        case "--out": outPath = args[safe: i + 1]; i += 2
        case "--hold": holdButton = UInt32(args[safe: i + 1] ?? "0") ?? 0; i += 2
        case "--from": holdFrom = Int(args[safe: i + 1] ?? "0") ?? 0; i += 2
        case "--frames": frames = Int(args[safe: i + 1] ?? "600") ?? 600; i += 2
        default: i += 1
        }
    }

    guard let outPath else {
        print("[calibrate-run] missing --out")
        exit(1)
    }

    let rom = loadROM(romPath)
    loadCore()

    // Silent ghost: discard video (NULL sink). Deterministic replay needs no render.
    rr_set_video(rr_video(on_frame: nil, user: nil))
    attachInput(ScriptedInput([(holdFrom, [Joypad(rawValue: holdButton) ?? .right])]))

    let loadRC = rr_load_game((rom as NSData).bytes, rom.count)
    guard loadRC == 0 else {
        print("[calibrate-run] load failed (rc=\(loadRC))")
        exit(1)
    }

    for _ in 0..<frames {
        rr_run()
    }

    guard let state = serializeState() else {
        print("[calibrate-run] serialize failed")
        exit(1)
    }
    try? state.write(to: URL(fileURLWithPath: outPath))
    print("[calibrate-run] hold=\(holdButton) from=\(holdFrom) frames=\(frames) state=\(state.count)B -> \(outPath)")

    rr_unload_game()
    rr_unload()
}

extension Array {
    subscript(safe idx: Int) -> Element? {
        indices.contains(idx) ? self[idx] : nil
    }
}
