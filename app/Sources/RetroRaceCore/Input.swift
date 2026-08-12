import CRetroRace
import Foundation

// MARK: - Input script (deterministic button schedule)

enum Joypad: UInt32 {
    case b = 0, y = 1, select = 2, start = 3
    case up = 4, down = 5, left = 6, right = 7
    case a = 8, x = 9, l = 10, r = 11
    case l2 = 12, r2 = 13, l3 = 14, r3 = 15
}

/// A scripted controller: holds a set of buttons starting at a given frame.
final class ScriptedInput {
    var frame = 0
    private var holds: [(startFrame: Int, buttons: Set<Joypad>)] = []

    init(_ script: [(Int, Set<Joypad>)]) {
        holds = script.map { (startFrame: $0.0, buttons: $0.1) }
            .sorted(by: { $0.startFrame < $1.startFrame })
    }

    func buttons(at f: Int) -> Set<Joypad> {
        var result: Set<Joypad> = []
        for (startFrame, buttons) in holds where startFrame <= f {
            result.formUnion(buttons)
        }
        return result
    }
}

let inputPollCallback: @convention(c) (UnsafeMutableRawPointer?) -> Void = { user in
    guard let user else { return }
    Unmanaged<ScriptedInput>.fromOpaque(user).takeUnretainedValue().frame += 1
}

let inputStateCallback: @convention(c) (UInt32, UInt32, UInt32, UInt32, UnsafeMutableRawPointer?) -> Int16 = {
    port, device, index, id, user in
    guard let user else { return 0 }
    let script = Unmanaged<ScriptedInput>.fromOpaque(user).takeUnretainedValue()
    return script.buttons(at: script.frame).contains(Joypad(rawValue: id) ?? .b) ? 1 : 0
}

/// Strong references kept alive while the core holds raw pointers to them.
final class ScriptStore: @unchecked Sendable {
    private var scripts: [ScriptedInput] = []
    private let lock = NSLock()

    func keep(_ script: ScriptedInput) {
        lock.lock()
        scripts.append(script)
        lock.unlock()
    }
}

private let scriptStore = ScriptStore()

func attachInput(_ script: ScriptedInput) {
    scriptStore.keep(script)
    let handle = Unmanaged.passUnretained(script).toOpaque()
    rr_set_input(rr_input(poll: inputPollCallback, state: inputStateCallback, user: handle))
}
