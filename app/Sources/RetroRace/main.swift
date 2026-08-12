import CRetroRace
import Foundation

// MARK: - Paths and ROM

let corePath = envOr("RETRO_RACE_CORE", default: "/Users/mac/retro-race/cores/libretro-fceumm/fceumm_libretro.dylib")
let romPath = envOr("RETRO_RACE_ROM", default: "/Users/mac/retro-race/app/Roms/Alter_Ego.nes")

func envOr(_ key: String, default: String) -> String {
    ProcessInfo.processInfo.environment[key] ?? `default`
}

func loadROM(_ path: String) -> Data {
    let url = URL(fileURLWithPath: path)
    do {
        return try Data(contentsOf: url)
    } catch {
        print("FATAL: cannot read ROM at \(path): \(error)")
        exit(1)
    }
}

func loadCore() {
    let rc = rr_load(corePath)
    guard rc == 0 else {
        print("FATAL: rr_load failed (\(rc))")
        exit(1)
    }
    // Populates g_need_fullpath inside the shim so rr_load_game passes a path
    // or a buffer depending on what the core requires.
    var infoBuf = [CChar](repeating: 0, count: 256)
    _ = rr_system_info(&infoBuf, infoBuf.count)
}

func printCoreInfo() {
    var infoBuf = [CChar](repeating: 0, count: 256)
    _ = rr_system_info(&infoBuf, infoBuf.count)
    print("core info: \(String(cString: infoBuf))")
    print("api version: \(rr_api_version())")
}

// MARK: - Save state helpers

func serializeState() -> Data? {
    let size = rr_serialize_size()
    var data = [UInt8](repeating: 0, count: size)
    let ok = data.withUnsafeMutableBytes { buf in
        rr_serialize(buf.baseAddress, buf.count) == 0
    }
    return ok ? Data(data) : nil
}

func unserializeState(_ data: Data) -> Bool {
    data.withUnsafeBytes { buf in
        rr_unserialize(buf.baseAddress, buf.count) == 0
    }
}

// MARK: - Entry point

let args = CommandLine.arguments

let mode = args.count > 1 ? args[1] : "spike"

switch mode {
case "spike":
    runSpike()
case "determinism":
    runDeterminism(Array(args.dropFirst(2)))
case "calibrate":
    runCalibrate(Array(args.dropFirst(2)))
case "calibrate-run":
    runCalibrateRun(Array(args.dropFirst(2)))
case "ghost":
    runGhost(Array(args.dropFirst(2)))
case "ghost-live":
    runGhostLive(Array(args.dropFirst(2)))
case "shm-peek":
    runShmPeek(Array(args.dropFirst(2)))
case "overlay":
    runOverlay()
default:
    print("""
    Usage:
      RetroRace spike                 determinism + framebuffer check
      RetroRace determinism [--frames N] [--interval N]
      RetroRace calibrate [--frames N]  spawn 2 children, diff their save states
      RetroRace calibrate-run --out PATH [--hold RIGHT --from 120] [--frames N]
      RetroRace ghost --out PATH        silent ghost: run frames, dump save state
      RetroRace ghost-live --shm NAME   publish ghost position+sprite over shared memory
      RetroRace shm-peek --shm NAME     consumer: print frames from shared memory (headless check)
      RetroRace overlay                 player window + transparent click-through ghost overlay
    """)
    exit(1)
}
