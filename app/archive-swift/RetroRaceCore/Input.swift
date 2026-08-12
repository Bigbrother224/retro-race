import CRetroRace
import Foundation

// MARK: - Live player input (keyboard + gamepad, same callbacks as scripts)

public enum Joypad: UInt32 {
    case b = 0, y = 1, select = 2, start = 3
    case up = 4, down = 5, left = 6, right = 7
    case a = 8, x = 9, l = 10, r = 11
    case l2 = 12, r2 = 13, l3 = 14, r3 = 15
}

/// Real-time input provider. The UI layer (keyboard events, gamepad) mutates
/// `pressed`; the libretro callbacks read it each frame. Thread-safe.
public final class PlayerInput: @unchecked Sendable {
    private var pressed: Set<Joypad> = []
    private let lock = NSLock()

    public init() {}

    /// Current union of keyboard + gamepad buttons.
    public var buttons: Set<Joypad> {
        lock.lock()
        defer { lock.unlock() }
        return pressed
    }

    public func set(_ button: Joypad, pressed: Bool) {
        lock.lock()
        if pressed {
            self.pressed.insert(button)
        } else {
            self.pressed.remove(button)
        }
        lock.unlock()
    }

    /// Releases every button (e.g. window loses focus).
    public func releaseAll() {
        lock.lock()
        pressed.removeAll()
        lock.unlock()
    }
}

private let liveInputPollCallback: @convention(c) (UnsafeMutableRawPointer?) -> Void = { _ in }
private let liveInputStateCallback: @convention(c) (UInt32, UInt32, UInt32, UInt32, UnsafeMutableRawPointer?) -> Int16 = {
    _, _, _, id, user in
    guard let user else { return 0 }
    let input = Unmanaged<PlayerInput>.fromOpaque(user).takeUnretainedValue()
    return input.buttons.contains(Joypad(rawValue: id) ?? .b) ? 1 : 0
}

/// Strong references kept alive while the core holds raw pointers to them.
final class InputStore: @unchecked Sendable {
    private var inputs: [AnyObject] = []
    private let lock = NSLock()

    func keep(_ input: AnyObject) {
        lock.lock()
        inputs.append(input)
        lock.unlock()
    }
}

private let inputStore = InputStore()

/// Attaches a live player input to the core. Replaces any scripted input.
public func attachPlayerInput(_ input: PlayerInput) {
    inputStore.keep(input)
    let handle = Unmanaged.passUnretained(input).toOpaque()
    rr_set_input(rr_input(poll: liveInputPollCallback, state: liveInputStateCallback, user: handle))
}

// MARK: - Scripted input (deterministic schedules for tests/calibration)

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

private let inputPollCallback: @convention(c) (UnsafeMutableRawPointer?) -> Void = { user in
    guard let user else { return }
    Unmanaged<ScriptedInput>.fromOpaque(user).takeUnretainedValue().frame += 1
}

private let inputStateCallback: @convention(c) (UInt32, UInt32, UInt32, UInt32, UnsafeMutableRawPointer?) -> Int16 = {
    port, device, index, id, user in
    guard let user else { return 0 }
    let script = Unmanaged<ScriptedInput>.fromOpaque(user).takeUnretainedValue()
    return script.buttons(at: script.frame).contains(Joypad(rawValue: id) ?? .b) ? 1 : 0
}

func attachInput(_ script: ScriptedInput) {
    inputStore.keep(script)
    let handle = Unmanaged.passUnretained(script).toOpaque()
    rr_set_input(rr_input(poll: inputPollCallback, state: inputStateCallback, user: handle))
}