import Foundation

/// `list [--dir PATH]` — scans a ROM folder and prints the local library
/// (name, extension, size, hashes). Headless mirror of the launcher library.
func runList(_ args: [String]) {
    var dir = "/Users/mac/retro-race/app/Roms"
    var i = 0
    while i < args.count {
        if args[i] == "--dir" {
            dir = args[safe: i + 1] ?? dir
            i += 2
        } else {
            i += 1
        }
    }

    let entries = LocalLibrary.scan(URL(fileURLWithPath: dir))
    print("\(entries.count) game(s) in \(dir)")
    for g in entries {
        print("  \(g.displayName) [\(g.extensions)] \(g.size)B md5=\(g.md5) sha256=\(g.sha256)")
    }
    if entries.isEmpty {
        print("(aucun jeu supporté trouvé)")
    }
}