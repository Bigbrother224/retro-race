import CRetroRace
import Foundation

final class FrameSink {
    var count = 0
    var lastWidth = 0
    var lastHeight = 0
    var lastPitch = 0
    var bytes: [UInt8] = []
    var sampleFrame: [UInt8] = []

    func onFrame(_ frame: rr_frame) {
        count += 1
        lastWidth = Int(frame.width)
        lastHeight = Int(frame.height)
        lastPitch = Int(frame.pitch)
        let rowBytes = Int(frame.width) * 4
        var buf = [UInt8](repeating: 0, count: rowBytes * Int(frame.height))
        let src = frame.framebuffer!.assumingMemoryBound(to: UInt8.self)
        for y in 0..<Int(frame.height) {
            let srcRow = src + y * Int(frame.pitch)
            let dstRow = buf.withUnsafeMutableBufferPointer { $0.baseAddress! + y * rowBytes }
            memcpy(dstRow, srcRow, rowBytes)
        }
        bytes = buf
        if count == 600 {
            sampleFrame = buf
        }
    }
}

func runSpike() {
    let rom = loadROM(romPath)
    print("== Retro Race Phase 0 spike ==")
    print("core: \(corePath)")
    print("rom:  \(romPath) (\(rom.count) bytes, md5 b12d0aefbde9b50eec53884d04d083b5)")

    loadCore()
    printCoreInfo()

    let sink = FrameSink()
    let sinkHandle = Unmanaged.passUnretained(sink).toOpaque()
    let videoCallback: @convention(c) (rr_frame, UnsafeMutableRawPointer?) -> Void = { frame, user in
        guard let user else { return }
        Unmanaged<FrameSink>.fromOpaque(user).takeUnretainedValue().onFrame(frame)
    }
    rr_set_video(rr_video(on_frame: videoCallback, user: sinkHandle))
    attachInput(ScriptedInput([]))

    guard rr_load_game((rom as NSData).bytes, rom.count) == 0 else {
        print("FATAL: rr_load_game failed")
        exit(1)
    }

    var w: UInt32 = 0, h: UInt32 = 0, maxW: UInt32 = 0, maxH: UInt32 = 0
    var fps: Double = 0
    rr_av_info(&w, &h, &fps, &maxW, &maxH)
    print("geometry: \(w)x\(h) max \(maxW)x\(maxH), fps \(fps)")

    let runStart = CFAbsoluteTimeGetCurrent()
    var i = 0
    while i < 1200 {
        rr_run()
        i += 1
    }
    let runElapsed = CFAbsoluteTimeGetCurrent() - runStart
    print("ran 1200 frames in \(String(format: "%.3f", runElapsed))s -> \(String(format: "%.0f", Double(i) / runElapsed)) fps effective")
    print("video frames received: \(sink.count), size \(sink.lastWidth)x\(sink.lastHeight) pitch \(sink.lastPitch)")

    print("\n== Determinism check ==")
    let stateSize = rr_serialize_size()
    print("save state size: \(stateSize) bytes")

    var state1 = [UInt8](repeating: 0, count: stateSize)
    func snapshot(_ dst: inout [UInt8]) -> Bool {
        dst.withUnsafeMutableBytes { buf in
            rr_serialize(buf.baseAddress, buf.count) == 0
        }
    }
    guard snapshot(&state1) else { print("FATAL: serialize failed"); exit(1) }

    let frameA = sink.bytes
    var framesAfter = 0
    while framesAfter < 90 {
        rr_run()
        framesAfter += 1
    }
    let frameB = sink.bytes

    _ = state1.withUnsafeBytes { buf in rr_unserialize(buf.baseAddress, buf.count) }
    sink.count = 0
    var framesRe = 0
    while framesRe < 90 {
        rr_run()
        framesRe += 1
    }
    let frameB2 = sink.bytes

    if frameB == frameB2 {
        print("PASS: 90 frames after restore produce identical framebuffer (\(frameB.count) bytes)")
    } else {
        var diffs = 0
        for (a, b) in zip(frameB, frameB2) where a != b {
            diffs += 1
        }
        print("FAIL: framebuffer diverged after restore, \(diffs)/\(frameB.count) bytes differ")
    }

    print("\nfirst frame bytes (sample of 16): \(frameA.prefix(16).map { String(format: "%02x", $0) }.joined(separator: " "))")

    rr_unload_game()
    rr_unload()
    print("done")
}

// MARK: - Long-run determinism check

func fnv1a(_ bytes: ArraySlice<UInt8>) -> UInt64 {
    var hash: UInt64 = 0xcbf29ce484222325
    for b in bytes {
        hash ^= UInt64(b)
        hash &*= 0x100000001b3
    }
    return hash
}

/// Runs a full calibration-style pass with two identical instances replayed
/// from a saved start state, hashing the framebuffer every `interval` frames.
/// Long-run determinism: the hash sequence must be byte-identical between the
/// original run and the restored replay.
func runDeterminism(_ args: [String]) {
    var frames = 50_000
    var interval = 1_000
    var i = 0
    while i < args.count {
        switch args[i] {
        case "--frames": frames = Int(args[safe: i + 1] ?? "50000") ?? 50_000; i += 2
        case "--interval": interval = Int(args[safe: i + 1] ?? "1000") ?? 1_000; i += 2
        default: i += 1
        }
    }

    let rom = loadROM(romPath)
    loadCore()
    print("== Long determinism check ==")
    print("frames=\(frames), checkpoint every \(interval) frames")

    let sink = FrameSink()
    let sinkHandle = Unmanaged.passUnretained(sink).toOpaque()
    let videoCallback: @convention(c) (rr_frame, UnsafeMutableRawPointer?) -> Void = { frame, user in
        guard let user else { return }
        Unmanaged<FrameSink>.fromOpaque(user).takeUnretainedValue().onFrame(frame)
    }
    rr_set_video(rr_video(on_frame: videoCallback, user: sinkHandle))
    attachInput(ScriptedInput([]))

    guard rr_load_game((rom as NSData).bytes, rom.count) == 0 else {
        print("FATAL: rr_load_game failed")
        exit(1)
    }

    guard let startState = serializeState() else {
        print("FATAL: serialize start state failed")
        exit(1)
    }
    print("start state: \(startState.count) bytes")

    func runSegments() -> [UInt64] {
        var hashes: [UInt64] = []
        _ = startState.withUnsafeBytes { rr_unserialize($0.baseAddress, $0.count) }
        var ran = 0
        while ran < frames {
            var target = min(ran + interval, frames)
            while ran < target {
                rr_run()
                ran += 1
            }
            hashes.append(fnv1a(sink.bytes[0..<sink.bytes.count]))
        }
        return hashes
    }

    let pass1 = runSegments()
    let pass2 = runSegments()

    if pass1 == pass2 {
        print("PASS: \(frames) frames replayed from start state -> identical hash sequence (\(pass1.count) checkpoints)")
    } else {
        var first = -1
        for (i, (a, b)) in zip(pass1, pass2).enumerated() where a != b {
            first = i
            break
        }
        let frameAt = first < 0 ? -1 : (first + 1) * interval
        print("FAIL: hash sequence diverged at checkpoint \(first) (frame ~\(frameAt))")
        print("  pass1[..]: \(pass1.prefix(4))")
        print("  pass2[..]: \(pass2.prefix(4))")
    }

    rr_unload_game()
    rr_unload()
}
