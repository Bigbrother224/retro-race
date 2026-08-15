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

// TestJoustOwners pins the Joust gate: when active, ownership of both ports
// swaps every joustEvery frames; when inactive it stays fixed (player 0 -> port
// 0, player 1 -> port 1) and a degenerate cadence never divides by zero.
func TestJoustOwners(t *testing.T) {
	const every = 60

	// Inactive: routing never changes regardless of the frame.
	for f := 0; f < 2*every; f++ {
		if o := joustOwners(f, every, false); o != [2]int{0, 1} {
			t.Fatalf("inactive frame %d owner = %v, want [0 1]", f, o)
		}
	}

	// Active: first cadence window player 0 drives port 0, second swaps.
	if o := joustOwners(0, every, true); o != [2]int{0, 1} {
		t.Fatalf("first window owner = %v, want [0 1]", o)
	}
	if o := joustOwners(59, every, true); o != [2]int{0, 1} {
		t.Fatalf("end of first window owner = %v, want [0 1]", o)
	}
	if o := joustOwners(60, every, true); o != [2]int{1, 0} {
		t.Fatalf("second window owner = %v, want [1 0]", o)
	}
	if o := joustOwners(119, every, true); o != [2]int{1, 0} {
		t.Fatalf("end of second window owner = %v, want [1 0]", o)
	}
	if o := joustOwners(120, every, true); o != [2]int{0, 1} {
		t.Fatalf("third window owner = %v, want [0 1]", o)
	}

	// Degenerate cadence must not panic nor swap.
	if o := joustOwners(100, 0, true); o != [2]int{0, 1} {
		t.Fatalf("zero cadence owner = %v, want [0 1]", o)
	}
}

// TestPlayerPlan pins the player-assignment decision. The key case for local
// testing without two controllers: a keyboard + one gamepad is two players
// (keyboard -> player 0, gamepad -> player 1), so Joust works with a single
// physical controller on the machine.
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
