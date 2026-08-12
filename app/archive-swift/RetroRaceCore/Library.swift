import CommonCrypto
import Foundation

// MARK: - Local library: detected games

/// A locally-owned game file, identified by hash — the ROM never leaves the
/// machine (roadmap: local library + hash import).
public struct GameEntry: Identifiable, Hashable {
    public let id: String          // sha256
    public let name: String        // filename without extension
    public let path: String
    public let md5: String
    public let sha256: String
    public let size: Int
    public let extensions: String  // e.g. "nes"

    /// Best-effort display name: strip the extension and common archive noise.
    public var displayName: String {
        name
            .replacingOccurrences(of: "&amp;", with: "&")
            .replacingOccurrences(of: "_", with: " ")
    }

    public init(id: String, name: String, path: String, md5: String,
                sha256: String, size: Int, extensions: String) {
        self.id = id
        self.name = name
        self.path = path
        self.md5 = md5
        self.sha256 = sha256
        self.size = size
        self.extensions = extensions
    }
}

/// Scans a folder for supported ROMs and hashes each file (local-only).
public struct LocalLibrary {
    /// Extensions mapped to the core dylib path, relative to the cores dir.
    public static let supportedExtensions = [
        "nes": "libretro-fceumm/fceumm_libretro.dylib",
        "fds": "libretro-fceumm/fceumm_libretro.dylib",
        "snes": "snes9x_libretro.dylib",
        "sfc": "snes9x_libretro.dylib",
        "smc": "snes9x_libretro.dylib",
        "gb": "gambatte_libretro.dylib",
        "gbc": "gambatte_libretro.dylib",
        "gen": "genesis_plus_gx_libretro.dylib",
        "md": "genesis_plus_gx_libretro.dylib",
    ]

    public static func scan(_ directory: URL) -> [GameEntry] {
        let fm = FileManager.default
        guard let files = try? fm.contentsOfDirectory(at: directory,
                                                      includingPropertiesForKeys: [.fileSizeKey],
                                                      options: [.skipsHiddenFiles]) else {
            return []
        }

        return files
            .filter { !$0.hasDirectoryPath }
            .compactMap { url -> GameEntry? in
                let ext = url.pathExtension.lowercased()
                guard supportedExtensions[ext] != nil else { return nil }
                guard let data = try? Data(contentsOf: url) else { return nil }

                let md5 = md5Hex(data)
                let sha256 = sha256HexData(data)
                let size = (try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? data.count
                return GameEntry(id: sha256,
                                 name: url.deletingPathExtension().lastPathComponent,
                                 path: url.path,
                                 md5: md5,
                                 sha256: sha256,
                                 size: size,
                                 extensions: ext)
            }
            .sorted { $0.displayName.localizedCaseInsensitiveCompare($1.displayName) == .orderedAscending }
    }
}

func md5Hex(_ data: Data) -> String {
    var digest = [UInt8](repeating: 0, count: Int(CC_MD5_DIGEST_LENGTH))
    data.withUnsafeBytes { buf in
        _ = CC_MD5(buf.baseAddress, CC_LONG(data.count), &digest)
    }
    return digest.map { String(format: "%02x", $0) }.joined()
}

func sha256HexData(_ data: Data) -> String {
    var digest = [UInt8](repeating: 0, count: Int(CC_SHA256_DIGEST_LENGTH))
    data.withUnsafeBytes { buf in
        _ = CC_SHA256(buf.baseAddress, CC_LONG(data.count), &digest)
    }
    return digest.map { String(format: "%02x", $0) }.joined()
}