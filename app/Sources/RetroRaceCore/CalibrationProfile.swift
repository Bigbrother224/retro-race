import Foundation

// MARK: - Versioned calibration profile (Game Profile seed)

/// The machine-readable calibration artifact produced by `calibrate --json-out`
/// and consumed at launch time. Replaces the hardcoded 0x0072 offset.
public struct CalibrationProfile: Codable {
    public let schema: String
    public let schemaVersion: Int
    public let game: String
    public let core: String
    public let romHashMD5: String
    public let romHashSHA256: String?
    public let method: String
    public let frames: Int
    public let stateSize: Int
    public let positionCandidate: PositionCandidate?

    public enum CodingKeys: String, CodingKey {
        case schema
        case schemaVersion = "schema_version"
        case game, core
        case romHashMD5 = "rom_hash_md5"
        case romHashSHA256 = "rom_hash_sha256"
        case method, frames
        case stateSize = "state_size"
        case positionCandidate = "position_candidate"
    }

    public struct PositionCandidate: Codable {
        public let offset: Int
        public let span: Int
        public let spread: Int
        public let note: String?
    }

    /// Looks for a calibration profile in `directory`, matching `entry` by
    /// MD5 (preferred) then SHA-256. Returns nil when none matches.
    public static func find(in directory: URL, for entry: GameEntry) -> CalibrationProfile? {
        let fm = FileManager.default
        guard let files = try? fm.contentsOfDirectory(at: directory,
                                                      includingPropertiesForKeys: nil,
                                                      options: [.skipsHiddenFiles]) else {
            return nil
        }
        for file in files where file.pathExtension.lowercased() == "json" {
            guard let data = try? Data(contentsOf: file),
                  let profile = try? JSONDecoder().decode(CalibrationProfile.self, from: data) else {
                continue
            }
            if profile.romHashMD5 == entry.md5 { return profile }
            if let p = profile.romHashSHA256, p == entry.sha256 { return profile }
        }
        return nil
    }
}