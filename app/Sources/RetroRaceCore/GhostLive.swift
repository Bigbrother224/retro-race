import CRetroRace
import Foundation

/// A frame producer: reads the ghost's framebuffer, crops a sprite tile around
/// the calibrated position, and publishes position + tile over shared memory.
final class GhostTileExtractor: @unchecked Sendable {
    var width = 0
    var height = 0
    var pitch = 0
    var rgba: [UInt8] = []

    func onFrame(_ frame: rr_frame) {
        guard let fb = frame.framebuffer else { return }
        width = Int(frame.width)
        height = Int(frame.height)
        pitch = Int(frame.pitch)
        let rowBytes = width * 4
        rgba = [UInt8](repeating: 0, count: rowBytes * height)
        let src = fb.assumingMemoryBound(to: UInt8.self)
        rgba.withUnsafeMutableBufferPointer { dst in
            for y in 0..<height {
                let s = src + y * pitch
                let d = dst.baseAddress! + y * rowBytes
                for x in 0..<width {
                    let b = s[x * 4 + 0]
                    let g = s[x * 4 + 1]
                    let r = s[x * 4 + 2]
                    d[x * 4 + 0] = r
                    d[x * 4 + 1] = g
                    d[x * 4 + 2] = b
                    d[x * 4 + 3] = 255
                }
            }
        }
    }

    /// Crops a RR_SHM_TILE_W x RR_SHM_TILE_H RGBA tile centred on (x, y),
    /// in game-pixel coordinates.
    func tile(at x: Int, y: Int) -> [UInt8] {
        let t = Int(RR_SHM_TILE_W)
        let half = t / 2
        var out = [UInt8](repeating: 0, count: t * t * 4)
        guard width > 0, height > 0 else { return out }

        let x0 = max(0, x - half)
        let y0 = max(0, y - half)
        for tileY in 0..<t {
            let sy = y0 + tileY
            guard sy < height else { continue }
            for tileX in 0..<t {
                let sx = x0 + tileX
                guard sx < width else { continue }
                let si = (sy * width + sx) * 4
                let di = (tileY * t + tileX) * 4
                out[di + 0] = rgba[si + 0]
                out[di + 1] = rgba[si + 1]
                out[di + 2] = rgba[si + 2]
                out[di + 3] = rgba[si + 3]
            }
        }
        return out
    }
}

func makeSlot() -> rr_shm_slot {
    var s = rr_shm_slot()
    s.magic = RR_SHM_MAGIC
    s.version = RR_SHM_VERSION
    s.frame = 0
    s.state = UInt32(RR_SHM_STATE_IDLE)
    s.game_fps = 60
    return s
}

/// `ghost-live --shm NAME [--frames N]`
///
/// Runs the core with rendering enabled, extracts the calibrated position and
/// a sprite tile around it each frame, and publishes them over shared memory.
/// The player process maps the same NAME and consumes via rr_shm_take.
func runGhostLive(_ args: [String]) {
    var shmName = "retro_race_ghost"
    var frames = -1  // unlimited
    var calibrationPath: String? = nil

    var i = 0
    while i < args.count {
        switch args[i] {
        case "--shm": shmName = args[safe: i + 1] ?? shmName; i += 2
        case "--frames": frames = Int(args[safe: i + 1] ?? "-1") ?? -1; i += 2
        case "--calibration": calibrationPath = args[safe: i + 1]; i += 2
        default: i += 1
        }
    }

    let rom = loadROM(romPath)
    loadCore()

    let extractor = GhostTileExtractor()
    let handle = Unmanaged.passUnretained(extractor).toOpaque()
    let videoCallback: @convention(c) (rr_frame, UnsafeMutableRawPointer?) -> Void = { frame, user in
        guard let user else { return }
        Unmanaged<GhostTileExtractor>.fromOpaque(user).takeUnretainedValue().onFrame(frame)
    }
    rr_set_video(rr_video(on_frame: videoCallback, user: handle))
    // Same input scheme as the committed calibration (RIGHT hold from 120).
    attachInput(ScriptedInput([(120, [.right])]))

    guard rr_load_game((rom as NSData).bytes, rom.count) == 0 else {
        print("[ghost-live] load failed")
        exit(1)
    }

    let stateSize = rr_serialize_size()

    // Calibration offset from the versioned JSON profile when provided,
    // else fall back to the committed Alter Ego profile (0x0072).
    var calOffset = 0x0072
    if let calibrationPath {
        let url = URL(fileURLWithPath: calibrationPath)
        if let data = try? Data(contentsOf: url),
           let profile = try? JSONDecoder().decode(CalibrationProfile.self, from: data),
           let cand = profile.positionCandidate {
            calOffset = cand.offset
            print("[ghost-live] calibration \(url.lastPathComponent) -> position offset 0x\(String(format: "%04x", calOffset))")
        } else {
            print("[ghost-live] WARN: cannot read calibration at \(calibrationPath), using fallback 0x\(String(format: "%04x", calOffset))")
        }
    }

    func positionBytes() -> (x: Int, y: Int) {
        var data = [UInt8](repeating: 0, count: stateSize)
        let ok = data.withUnsafeMutableBytes { rr_serialize($0.baseAddress, $0.count) == 0 }
        guard ok, calOffset + 1 < data.count else { return (0, 0) }
        return (Int(data[calOffset]), Int(data[calOffset + 1]))
    }

    var shm = rr_shm()
    guard rr_shm_open_named(shmName, true, &shm) == 0 else {
        print("[ghost-live] cannot open shm \(shmName)")
        exit(1)
    }
    defer { rr_shm_close(&shm, true) }

    var slot = makeSlot()
    var frame = 0
    let start = CFAbsoluteTimeGetCurrent()

    while frames < 0 || frame < frames {
        rr_run()
        frame += 1

        let pos = positionBytes()
        let tile = extractor.tile(at: pos.x, y: pos.y)
        slot.pos_x = UInt32(pos.x)
        slot.pos_y = UInt32(pos.y)
        slot.state = UInt32(RR_SHM_STATE_RACING)
        tile.withUnsafeBytes { buf in
            rr_shm_slot_set_tile(&slot, buf.baseAddress?.assumingMemoryBound(to: UInt8.self), UInt32(tile.count))
        }
        slot.frame = UInt32(frame)
        rr_shm_publish(&shm, &slot)
    }

    print("[ghost-live] published \(frame) frames in \(String(format: "%.2f", CFAbsoluteTimeGetCurrent() - start))s")
    rr_unload_game()
    rr_unload()
}