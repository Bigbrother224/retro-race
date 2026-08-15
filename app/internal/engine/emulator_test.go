package engine

import "testing"

// The FakeCore adapter proves the Emulator seam: the UI can be exercised
// without a core dylib or a ROM.

func TestFakeCoreRendersFrames(t *testing.T) {
	f := NewFakeCore(320, 240)
	if err := f.Start("", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer f.Stop()

	first := f.Frame()
	if first == nil {
		t.Fatal("no frame before Step")
	}
	if f.Width() != 320 || f.Height() != 240 {
		t.Fatalf("size = %dx%d, want 320x240", f.Width(), f.Height())
	}
	if len(first) != 320*240*4 {
		t.Fatalf("frame len = %d, want %d", len(first), 320*240*4)
	}
	// Copy before stepping: Frame() reuses the same buffer, so the slice
	// content changes in place.
	snapshot := make([]byte, len(first))
	copy(snapshot, first)

	f.Step()
	second := f.Frame()
	if &second[0] != &first[0] {
		t.Fatal("buffer not reused: Frame() should return the same backing array")
	}
	if string(snapshot) == string(second) {
		t.Fatal("animation did not advance: frames identical after Step")
	}
}

func TestFakeCoreButtons(t *testing.T) {
	f := NewFakeCore(64, 64)
	_ = f.Start("", "")
	defer f.Stop()

	f.SetButton(BtnA, true)
	if !f.buttons[0][BtnA] {
		t.Fatal("BtnA not registered as pressed")
	}
	f.SetButton(BtnA, false)
	if f.buttons[0][BtnA] {
		t.Fatal("BtnA still pressed after release")
	}
}

// The gate writes player inputs per port; ports must stay independent so a
// native 2-player game reads each player's buttons without bleed.
func TestFakeCorePerPortButtons(t *testing.T) {
	f := NewFakeCore(64, 64)
	_ = f.Start("", "")
	defer f.Stop()

	f.SetButtonPort(1, BtnB, true)
	if !f.buttons[1][BtnB] {
		t.Fatal("port 1 BtnB not registered")
	}
	if f.buttons[0][BtnB] {
		t.Fatal("port 1 input leaked onto port 0")
	}
	f.SetButtonPort(1, BtnB, false)
	if f.buttons[1][BtnB] {
		t.Fatal("port 1 BtnB still pressed after release")
	}
}

// Determinism: the same number of steps always produces the same frame.
func TestFakeCoreDeterministic(t *testing.T) {
	a := NewFakeCore(64, 64)
	b := NewFakeCore(64, 64)
	_ = a.Start("", "")
	_ = b.Start("", "")
	defer a.Stop()
	defer b.Stop()

	for i := 0; i < 30; i++ {
		a.Step()
		b.Step()
	}
	if string(a.Frame()) != string(b.Frame()) {
		t.Fatal("two identical fakes diverged after 30 steps")
	}
}
