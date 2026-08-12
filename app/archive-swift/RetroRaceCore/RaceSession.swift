import CRetroRace
import Foundation

/// Runs the player's core and composites the ghost (from shared memory) into
/// an RGBA framebuffer, without any UI. The App renders `frame` and the CLI
/// can drive it headlessly.
public final class RaceSession: @unchecked Sendable {
    public struct GhostState {
        public var x = 0
        public var y = 0
        public var visible = false
        public var tile: [UInt8] = []

        public init() {}
        public init(x: Int, y: Int, visible: Bool, tile: [UInt8]) {
            self.x = x
            self.y = y
            self.visible = visible
            self.tile = tile
        }
    }

    /// One point of the ghost trail, in game pixels, with its age in steps.
    public struct TrailPoint {
        public let x: Int
        public let y: Int
        public let age: Int   // 0 = newest

        public init(x: Int, y: Int, age: Int) {
            self.x = x
            self.y = y
            self.age = age
        }
    }

    /// How many trail points the session keeps (drives the trail length).
    public var trailLength: Int
    private var trail: [TrailPoint] = []

    public private(set) var width = 0
    public private(set) var height = 0
    private var rgb: [UInt8] = []

    public private(set) var ghost = GhostState()

    private var shm = rr_shm()
    private var shmAttached = false
    private var lastFrame: UInt32 = UInt32.max
    private let shmName: String

    private let sink = OverlaySink()
    private var coreLoaded = false

    /// Live input provider; the UI mutates it (keyboard/gamepad), the core
    /// reads it every frame.
    public let input = PlayerInput()

    public init(shmName: String = "retro_race_ghost", trailLength: Int = 30) {
        self.shmName = shmName
        self.trailLength = trailLength
    }

    /// Loads the ROM and starts the player core. Call from a background thread.
    public func start(rom: String) throws {
        let romData = loadROM(rom)
        loadCore(rom: rom)

        let handle = Unmanaged.passUnretained(sink).toOpaque()
        let videoCallback: @convention(c) (rr_frame, UnsafeMutableRawPointer?) -> Void = { frame, user in
            guard let user else { return }
            Unmanaged<OverlaySink>.fromOpaque(user).takeUnretainedValue().onFrame(frame)
        }
        rr_set_video(rr_video(on_frame: videoCallback, user: handle))
        attachPlayerInput(input)

        guard rr_load_game((romData as NSData).bytes, romData.count) == 0 else {
            throw RaceError.loadFailed
        }
        coreLoaded = true
        width = sink.width > 0 ? sink.width : 256
        height = sink.height > 0 ? sink.height : 240
    }

    /// Advances one frame and pulls the latest ghost state from shared memory.
    public func step() {
        guard coreLoaded else { return }
        rr_run()

        if sink.width > 0 {
            width = sink.width
            height = sink.height
            rgb = sink.rgba
        }

        if !shmAttached {
            shmAttached = rr_shm_open_named(shmName, false, &shm) == 0
        }
        if shmAttached {
            var slot = rr_shm_slot()
            if rr_shm_take(&shm, &lastFrame, &slot) {
                let tileCount = Int(RR_SHM_TILE_W) * Int(RR_SHM_TILE_H) * 4
                var tile = [UInt8](repeating: 0, count: tileCount)
                tile.withUnsafeMutableBytes { buf in
                    rr_shm_slot_tile_copy(&slot, buf.baseAddress?.assumingMemoryBound(to: UInt8.self), UInt32(tileCount))
                }
                ghost = GhostState(x: Int(slot.pos_x), y: Int(slot.pos_y),
                                   visible: true, tile: tile)
                recordTrail()
            }
        }
    }

    /// Samples the ghost position into the trail and ages existing points.
    private func recordTrail() {
        trail = trail.map { TrailPoint(x: $0.x, y: $0.y, age: $0.age + 1) }
            .filter { $0.age < trailLength }
        trail.append(TrailPoint(x: ghost.x, y: ghost.y, age: 0))
    }

    /// Ghost trail points, oldest first, in game pixels.
    public var ghostTrail: [TrailPoint] { trail }

    /// The latest rendered player frame (RGBA).
    public var frame: [UInt8] { rgb }

    /// The latest ghost tile in the player's coordinate space (game pixels).
    public var ghostState: GhostState { ghost }

    public func stop() {
        if coreLoaded {
            rr_unload_game()
            rr_unload()
            coreLoaded = false
        }
        if shmAttached {
            rr_shm_close(&shm, false)
            shmAttached = false
        }
    }

    public enum RaceError: Error {
        case loadFailed
    }
}