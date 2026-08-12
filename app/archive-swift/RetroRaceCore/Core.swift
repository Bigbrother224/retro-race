import CRetroRace
import Foundation

// MARK: - Paths, ROM and core lifecycle (shared by App and CLI)

let coresDirectory = envOr("RETRO_RACE_CORES", default: "/Users/mac/retro-race/cores")
let corePath = envOr("RETRO_RACE_CORE",
                     default: "/Users/mac/retro-race/cores/libretro-fceumm/fceumm_libretro.dylib")
let romPath = envOr("RETRO_RACE_ROM",
                    default: "/Users/mac/retro-race/app/Roms/Alter_Ego.nes")

func envOr(_ key: String, default: String) -> String {
    ProcessInfo.processInfo.environment[key] ?? `default`
}

/// Chooses the libretro core for a ROM file based on its extension
/// (e.g. .nes -> fceumm, .sfc/.smc -> snes9x). Falls back to the env override.
func corePath(forRom romURL: URL) -> String {
    if let override = ProcessInfo.processInfo.environment["RETRO_RACE_CORE"] {
        return override
    }
    let ext = romURL.pathExtension.lowercased()
    let coreName = LocalLibrary.supportedExtensions[ext]
    guard let coreName else {
        return corePath
    }
    return URL(fileURLWithPath: coresDirectory)
        .appendingPathComponent(coreName).path
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

/// Loads the core matching the given ROM. Falls back to the default core
/// when the ROM path is unknown.
func loadCore(rom: String? = nil) {
    let path: String
    if let rom {
        path = corePath(forRom: URL(fileURLWithPath: rom))
    } else {
        path = corePath
    }
    let rc = rr_load(path)
    guard rc == 0 else {
        print("FATAL: rr_load failed (\(rc)) for \(path)")
        exit(1)
    }
    // Populates g_need_fullpath inside the shim so rr_load_game passes a path
    // or a buffer depending on what the core requires.
    var infoBuf = [CChar](repeating: 0, count: 256)
    _ = rr_system_info(&infoBuf, infoBuf.count)
    print("loaded core: \(String(cString: infoBuf))")
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