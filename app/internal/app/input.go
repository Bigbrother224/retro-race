package app

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"retrorace/internal/engine"
)

// snesGamepadButtons maps each standard-layout gamepad button to a logical
// SNES button. The standard layout is positional (W3C remapping), so a single
// mapping covers PlayStation (✕ △ □ ○) and Xbox (A B X Y) controllers without
// detecting the model: RightBottom is the bottom face button (✕/A), RightTop
// the top one (△/Y), etc.
var snesGamepadButtons = map[ebiten.StandardGamepadButton]engine.JoyButton{
	ebiten.StandardGamepadButtonRightBottom: engine.BtnB, // ✕ / A
	ebiten.StandardGamepadButtonRightRight:  engine.BtnA, // ○ / B
	ebiten.StandardGamepadButtonRightLeft:   engine.BtnY, // □ / X
	ebiten.StandardGamepadButtonRightTop:    engine.BtnX, // △ / Y

	ebiten.StandardGamepadButtonLeftTop:    engine.BtnUp,
	ebiten.StandardGamepadButtonLeftBottom: engine.BtnDown,
	ebiten.StandardGamepadButtonLeftLeft:   engine.BtnLeft,
	ebiten.StandardGamepadButtonLeftRight:  engine.BtnRight,

	ebiten.StandardGamepadButtonCenterLeft:    engine.BtnSelect,
	ebiten.StandardGamepadButtonCenterRight:   engine.BtnStart,
	ebiten.StandardGamepadButtonFrontTopLeft:  engine.BtnL, // L1 / LB
	ebiten.StandardGamepadButtonFrontTopRight: engine.BtnR, // R1 / RB
}

// gamepadButtonToSNES returns the logical SNES button for a standard gamepad
// button. It is pure so it can be unit-tested without a controller.
func gamepadButtonToSNES(b ebiten.StandardGamepadButton) (engine.JoyButton, bool) {
	btn, ok := snesGamepadButtons[b]
	return btn, ok
}

// keyboardButtons maps keyboard keys to logical SNES buttons (player 0's
// default device).
var keyboardButtons = map[ebiten.Key]engine.JoyButton{
	ebiten.KeyArrowUp:    engine.BtnUp,
	ebiten.KeyArrowDown:  engine.BtnDown,
	ebiten.KeyArrowLeft:  engine.BtnLeft,
	ebiten.KeyArrowRight: engine.BtnRight,
	ebiten.KeyZ:          engine.BtnA,
	ebiten.KeyX:          engine.BtnB,
	ebiten.KeyA:          engine.BtnY,
	ebiten.KeyS:          engine.BtnX,
	ebiten.KeyEnter:      engine.BtnStart,
	ebiten.KeyShift:      engine.BtnSelect,
	ebiten.KeyQ:          engine.BtnL,
	ebiten.KeyE:          engine.BtnR,
}

// playerPlan decides how to assign gamepads to logical players given the
// keyboard activity and the number of standard gamepads. It is pure so it can
// be unit-tested without a display or controllers.
//
//   - keyboard active + >=1 gamepad: two players (keyboard -> player 0, first
//     gamepad -> player 1).
//   - keyboard inactive: gamepad-only play; the first gamepad is player 0, and
//     a second gamepad makes it two players.
func playerPlan(keyboardActive bool, gamepadCount int) (twoPlayers bool, gamepadStart int) {
	if gamepadCount == 0 {
		return false, 0
	}
	if keyboardActive {
		return true, 1
	}
	return gamepadCount >= 2, 0
}

// rawButtonToSNES maps a non-standard gamepad's raw button index to a logical
// SNES button using the common HID ordering (D-pad 0-3, face 4-7, shoulders
// 8-9, select/start 10-11). Best-effort for pads ebiten does not recognise as
// standard; the controller test screen exposes the raw indices to tune it.
var rawButtonToSNES = []engine.JoyButton{
	engine.BtnUp, engine.BtnDown, engine.BtnLeft, engine.BtnRight,
	engine.BtnB, engine.BtnA, engine.BtnY, engine.BtnX,
	engine.BtnL, engine.BtnR,
	engine.BtnSelect, engine.BtnStart,
}

// readGamepadButtons reads one gamepad into st: the positional standard layout
// when available, otherwise a raw-button fallback. This is the single place a
// physical gamepad becomes button states, shared by local play and netplay.
func readGamepadButtons(id ebiten.GamepadID, st *[12]bool) {
	if ebiten.IsStandardGamepadLayoutAvailable(id) {
		for gbtn, btn := range snesGamepadButtons {
			if inpututil.IsStandardGamepadButtonJustPressed(id, gbtn) ||
				ebiten.IsStandardGamepadButtonPressed(id, gbtn) {
				st[btn] = true
			}
		}
		return
	}
	n := ebiten.GamepadButtonNum(id)
	for i := 0; i < n && i < len(rawButtonToSNES); i++ {
		if ebiten.IsGamepadButtonPressed(id, ebiten.GamepadButton(i)) {
			st[rawButtonToSNES[i]] = true
		}
	}
}
