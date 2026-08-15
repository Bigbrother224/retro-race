package app

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"retrorace/internal/engine"
)

// TestGamepadButtonToSNES pins the positional gamepad → SNES mapping: the
// standard layout is shared by PlayStation (✕ △ □ ○) and Xbox (A B X Y)
// controllers, so each standard button must land on the correct logical SNES
// button with no gaps or duplicates.
func TestGamepadButtonToSNES(t *testing.T) {
	want := map[ebiten.StandardGamepadButton]engine.JoyButton{
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
		ebiten.StandardGamepadButtonFrontTopLeft:  engine.BtnL,
		ebiten.StandardGamepadButtonFrontTopRight: engine.BtnR,
	}

	if len(snesGamepadButtons) != len(want) {
		t.Fatalf("mapping has %d entries, want %d", len(snesGamepadButtons), len(want))
	}
	for gb, sn := range want {
		got, ok := gamepadButtonToSNES(gb)
		if !ok {
			t.Errorf("no mapping for standard button %v", gb)
			continue
		}
		if got != sn {
			t.Errorf("standard button %v -> %v, want %v", gb, got, sn)
		}
	}
}

// TestPlayerPlan pins the player-assignment decision. The key case for local
// testing without two controllers: a keyboard + one gamepad is two players
// (keyboard -> player 0, gamepad -> player 1), so a second gamepad is not
// required to exercise two-player input routing.
func TestPlayerPlan(t *testing.T) {
	cases := []struct {
		kb    bool
		gp    int
		two   bool
		start int
	}{
		{false, 0, false, 0}, // nothing
		{true, 0, false, 0},  // keyboard only
		{false, 1, false, 0}, // solo gamepad -> player 0, not two
		{false, 2, true, 0},  // two gamepads, no keyboard
		{true, 1, true, 1},   // keyboard + one gamepad = two players
		{true, 2, true, 1},   // keyboard + gamepads
	}
	for _, c := range cases {
		two, start := playerPlan(c.kb, c.gp)
		if two != c.two || start != c.start {
			t.Errorf("playerPlan(kb=%v, gp=%d) = (two=%v, start=%d), want (two=%v, start=%d)",
				c.kb, c.gp, two, start, c.two, c.start)
		}
	}
}
