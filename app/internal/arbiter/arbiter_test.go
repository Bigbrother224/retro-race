package arbiter

import "testing"

// solidFrame returns a tightly packed RGBA8 frame of n pixels all set to the
// given per-channel value.
func solidFrame(ch byte, n int) []byte {
	f := make([]byte, n*4)
	for i := range f {
		f[i] = ch
	}
	return f
}

func testConfig() Config {
	return Config{ChangeFrac: 0.30, SettleFrac: 0.05, SettleFrames: 5}
}

// TestStableGameplayNeverFires: identical frames (no big change) never fire.
func TestStableGameplayNeverFires(t *testing.T) {
	a := New(testConfig())
	f := solidFrame(0x00, 100)
	for i := 0; i < 60; i++ {
		if a.Update(f) != None {
			t.Fatalf("frame %d: stable gameplay must not fire", i)
		}
	}
}

// TestBigChangeThenSettleFiresOnce: a big frame transition followed by a
// settled screen is a segment end, fired exactly once, then latched.
func TestBigChangeThenSettleFiresOnce(t *testing.T) {
	a := New(testConfig())
	black := solidFrame(0x00, 100)
	white := solidFrame(0xff, 100)

	// Warm up on the pre-change screen.
	for i := 0; i < 3; i++ {
		if a.Update(black) != None {
			t.Fatal("warm-up must not fire")
		}
	}
	fired := 0
	// Big transition (black -> white) then settle over enough frames.
	for i := 0; i < 8; i++ {
		if a.Update(white) == SegmentEnd {
			fired++
		}
	}
	if fired != 1 {
		t.Fatalf("expected exactly one SegmentEnd, got %d", fired)
	}
	// Latched: further changes must not re-fire.
	for i := 0; i < 6; i++ {
		if a.Update(black) != None {
			t.Fatal("arbiter must stay latched after firing")
		}
	}
}

// TestContinuousChangeNeverFires: a screen that keeps changing (scroll,
// animation) never settles and must not fire.
func TestContinuousChangeNeverFires(t *testing.T) {
	a := New(testConfig())
	black := solidFrame(0x00, 100)
	white := solidFrame(0xff, 100)
	for i := 0; i < 40; i++ {
		var f []byte
		if i%2 == 0 {
			f = black
		} else {
			f = white
		}
		if a.Update(f) != None {
			t.Fatalf("frame %d: continuous change must not fire", i)
		}
	}
}

// TestResetAllowsRefire: after a segment end, Reset clears the latch so a new
// segment can be detected.
func TestResetAllowsRefire(t *testing.T) {
	a := New(testConfig())
	black := solidFrame(0x00, 100)
	white := solidFrame(0xff, 100)

	detect := func() bool {
		for i := 0; i < 3; i++ {
			a.Update(black)
		}
		for i := 0; i < 8; i++ {
			if a.Update(white) == SegmentEnd {
				return true
			}
		}
		return false
	}

	if !detect() {
		t.Fatal("first segment not detected")
	}
	a.Reset()
	if !detect() {
		t.Fatal("after Reset the second segment must be detected")
	}
}

// TestFrameSizeChangeIsSafe: a framebuffer whose size changes (new game)
// must not panic or fire spuriously.
func TestFrameSizeChangeIsSafe(t *testing.T) {
	a := New(testConfig())
	a.Update(solidFrame(0x00, 100))
	if ev := a.Update(solidFrame(0xff, 200)); ev != None {
		t.Fatalf("size change must not fire, got %v", ev)
	}
}
