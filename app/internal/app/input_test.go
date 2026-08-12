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
