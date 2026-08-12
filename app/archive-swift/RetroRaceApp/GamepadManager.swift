import Foundation
import GameController
import RetroRaceCore

/// Bridges a connected gamepad (GCController) into a PlayerInput. Polls the
/// extended gamepad profile and updates the shared input each frame.
@MainActor
final class GamepadManager: NSObject {
    private let input: PlayerInput
    private var pollTimer: Timer?

    init(input: PlayerInput) {
        self.input = input
        super.init()
    }

    func start() {
        NotificationCenter.default.addObserver(
            self, selector: #selector(controllerConnected(_:)),
            name: .GCControllerDidConnect, object: nil)
        NotificationCenter.default.addObserver(
            self, selector: #selector(controllerDisconnected(_:)),
            name: .GCControllerDidDisconnect, object: nil)

        // Adopt already-connected controllers.
        for controller in GCController.controllers() {
            controller.playerIndex = .index1
        }

        pollTimer = Timer.scheduledTimer(withTimeInterval: 1.0 / 60.0, repeats: true) { [weak self] _ in
            self?.poll()
        }
    }

    func stop() {
        pollTimer?.invalidate()
        pollTimer = nil
    }

    private func poll() {
        guard let gamepad = GCController.controllers().first?.extendedGamepad else { return }

        input.set(.up, pressed: gamepad.dpad.up.isPressed || gamepad.leftThumbstick.up.value > 0.5)
        input.set(.down, pressed: gamepad.dpad.down.isPressed || gamepad.leftThumbstick.down.value > 0.5)
        input.set(.left, pressed: gamepad.dpad.left.isPressed || gamepad.leftThumbstick.left.value > 0.5)
        input.set(.right, pressed: gamepad.dpad.right.isPressed || gamepad.leftThumbstick.right.value > 0.5)
        input.set(.b, pressed: gamepad.buttonB.isPressed)
        input.set(.a, pressed: gamepad.buttonA.isPressed)
        input.set(.x, pressed: gamepad.buttonX.isPressed)
        input.set(.y, pressed: gamepad.buttonY.isPressed)
        input.set(.start, pressed: gamepad.buttonMenu.isPressed)
        input.set(.select, pressed: gamepad.buttonOptions?.isPressed ?? false)
        input.set(.l, pressed: gamepad.leftShoulder.isPressed)
        input.set(.r, pressed: gamepad.rightShoulder.isPressed)
    }

    @objc private func controllerConnected(_ note: Notification) {
        if let controller = note.object as? GCController {
            controller.playerIndex = .index1
        }
    }

    @objc private func controllerDisconnected(_ note: Notification) {
        input.releaseAll()
    }
}