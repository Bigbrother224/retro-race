import CRetroRace
import Foundation

/// `shm-peek --shm NAME [--ticks N] [--expect-motion]`
///
/// Attachment side used for headless verification: maps the shm region the
/// ghost producer writes and prints the published frames. With
/// `--expect-motion` it fails (exit 1) unless the ghost position moves, which
/// is what the player UI relies on.
func runShmPeek(_ args: [String]) {
    var shmName = "retro_race_ghost"
    var ticks = 120
    var expectMotion = false

    var i = 0
    while i < args.count {
        switch args[i] {
        case "--shm": shmName = args[safe: i + 1] ?? shmName; i += 2
        case "--ticks": ticks = Int(args[safe: i + 1] ?? "120") ?? 120; i += 2
        case "--expect-motion": expectMotion = true; i += 1
        default: i += 1
        }
    }

    var shm = rr_shm()
    guard rr_shm_open_named(shmName, false, &shm) == 0 else {
        print("[shm-peek] cannot open shm \(shmName)")
        exit(1)
    }
    defer { rr_shm_close(&shm, false) }

    var slot = rr_shm_slot()
    var lastFrame: UInt32 = UInt32.max
    var samples: [(frame: UInt32, x: UInt32, y: UInt32)] = []
    var nonZeroTileHashes = Set<String>()
    let start = CFAbsoluteTimeGetCurrent()

    for _ in 0..<ticks {
        if rr_shm_take(&shm, &lastFrame, &slot) {
            samples.append((slot.frame, slot.pos_x, slot.pos_y))
            let sum = rr_shm_slot_tile_sum(&slot)
            if sum > 0 { nonZeroTileHashes.insert("\(sum % 256)") }
        }
        usleep(16_000)  // ~60Hz polling, mirrors the player loop
    }
    let elapsed = CFAbsoluteTimeGetCurrent() - start

    print("[shm-peek] \(samples.count) frames in \(String(format: "%.2f", elapsed))s (magic=\(String(format: "%08x", UInt32(slot.magic))))")
    for s in samples.prefix(5) {
        print("  frame=\(s.frame) pos=(\(s.x),\(s.y))")
    }
    if samples.count > 5 {
        print("  ... ")
    }
    print("  non-zero tile sum diversity: \(nonZeroTileHashes.count) distinct buckets")

    if samples.isEmpty {
        print("[shm-peek] FAIL: no frames published")
        exit(1)
    }

    // A ghost that never moves is useless to the player.
    let xs = Set(samples.map { $0.x })
    let ys = Set(samples.map { $0.y })
    let moved = (xs.count > 1) || (ys.count > 1)
    if expectMotion && !moved {
        print("[shm-peek] FAIL: --expect-motion, but position never changed x=\(xs) y=\(ys)")
        exit(1)
    }
    if nonZeroTileHashes.isEmpty {
        print("[shm-peek] FAIL: tile is all zeros (extraction not working)")
        exit(1)
    }

    print("[shm-peek] PASS")
}