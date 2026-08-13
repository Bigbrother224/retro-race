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

// updateGamepadInputInto reads every connected gamepad through the standard
// positional layout, routes it to the Emulator seam, and fills the button
// state array for the input recorder.
func (a *App) updateGamepadInputInto(state *[12]bool) {
	for _, id := range ebiten.AppendGamepadIDs(nil) {
		if !ebiten.IsStandardGamepadLayoutAvailable(id) {
			continue // unknown layout: no reliable positional mapping
		}
		for gbtn, btn := range snesGamepadButtons {
			pressed := inpututil.IsStandardGamepadButtonJustPressed(id, gbtn) ||
				ebiten.IsStandardGamepadButtonPressed(id, gbtn)
			state[btn] = pressed
			a.emu.SetButton(btn, pressed)
		}
	}
}
