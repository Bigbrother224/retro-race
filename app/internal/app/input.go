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

// updatePlayerInputs collects raw input per logical player, independent of
// which emulator port they will drive. Keyboard is player 0; gamepads are
// assigned in connection order (first -> player 0, second -> player 1). This
// is the only place physical devices are turned into player button states, so
// a future remote source (relayed input packets) plugs in here as one more
// player.
func (a *App) updatePlayerInputs() {
	a.players = [2][12]bool{}

	// Keyboard -> player 0. Latch keyboard activity once any mapped key is
	// seen so a keyboard + gamepad combo reads as two players.
	keyUsed := false
	for key, btn := range keyboardButtons {
		if inpututil.IsKeyJustPressed(key) || ebiten.IsKeyPressed(key) {
			a.players[0][btn] = true
			keyUsed = true
		}
	}
	if keyUsed {
		a.keyboardActive = true
	}

	gps := a.standardGamepads()
	if len(gps) == 0 {
		return // keyboard only
	}

	// Decide player assignment. When the keyboard is a live player, the first
	// gamepad becomes player 1 (so keyboard + one gamepad is a valid 2-player
	// local setup). With no keyboard activity, a gamepad-only player keeps
	// control of port 0.
	_, start := playerPlan(a.keyboardActive, len(gps))
	p := start
	for _, id := range gps {
		if p >= len(a.players) {
			break
		}
		a.readGamepad(id, p)
		p++
	}
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

// standardGamepads returns every connected gamepad. readGamepad handles both
// standard and raw layouts, so no pad is dropped from player assignment.
func (a *App) standardGamepads() []ebiten.GamepadID {
	return ebiten.AppendGamepadIDs(nil)
}

// readGamepad reads one gamepad's pressed buttons into player p's state.
func (a *App) readGamepad(id ebiten.GamepadID, p int) {
	readGamepadButtons(id, &a.players[p])
}

// twoPlayers reports whether two distinct human input sources are present:
// keyboard + at least one gamepad, or two gamepads. Joust only makes sense when
// two humans can fight for control; with a single player it would just lock the
// game out.
func (a *App) twoPlayers() bool {
	two, _ := playerPlan(a.keyboardActive, len(a.standardGamepads()))
	return two
}

// joustOwners returns which logical player drives each emulator port for a
// given race frame. It is pure so it can be unit-tested without a display or
// controllers. When active, ownership of both ports swaps every joustEvery
// frames, so two humans alternate controlling the same character.
func joustOwners(frame, joustEvery int, active bool) [2]int {
	owner := [2]int{0, 1}
	if active && joustEvery > 0 {
		if (frame/joustEvery)%2 == 1 {
			owner[0], owner[1] = owner[1], owner[0]
		}
	}
	return owner
}

// portOwnerForFrame returns which logical player drives each emulator port this
// frame: [port0] = player, [port1] = player. It is where the Joust gate lives.
func (a *App) portOwnerForFrame() [2]int {
	if a.race == nil {
		return [2]int{0, 1}
	}
	return joustOwners(a.race.Frame(), a.joustEvery, a.joust && a.twoPlayers())
}

// applyGate routes the two logical players' button states to the emulator
// controller ports through the gate. Local two-gamepad play and remote relayed
// inputs both land here, so the routing is identical whether the players are
// side by side or across the world.
func (a *App) applyGate() {
	if a.emu == nil {
		return
	}
	owner := a.portOwnerForFrame()
	for port := 0; port < len(owner); port++ {
		st := a.players[owner[port]]
		for b := 0; b < 12; b++ {
			a.emu.SetButtonPort(port, engine.JoyButton(b), st[b])
		}
	}
}
