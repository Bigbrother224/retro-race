package engine

import "fmt"

// FakeCore is a Go-only Emulator for tests and demo mode. It renders a
// deterministic animated pattern (a moving bar + gradient) instead of a real
// game, so the UI and race logic can be exercised without a core dylib or a
// ROM.
type FakeCore struct {
	width, height int
	frame         int
	rgba          []byte
	buttons       [16]bool
}

// NewFakeCore creates a FakeCore with the given screen size.
func NewFakeCore(width, height int) *FakeCore {
	return &FakeCore{width: width, height: height}
}

// Start implements Emulator.
func (f *FakeCore) Start(romPath, corePath string) error {
	if f.width <= 0 || f.height <= 0 {
		return fmt.Errorf("fake core needs a positive size")
	}
	f.rgba = make([]byte, f.width*f.height*4)
	return nil
}

// Step implements Emulator: advances the animation.
func (f *FakeCore) Step() {
	f.frame++
}

// Frame implements Emulator: renders the current frame into a reused buffer.
func (f *FakeCore) Frame() []byte {
	if f.rgba == nil {
		return nil
	}
	w, h := f.width, f.height
	barX := (f.frame * 3) % w
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			// Gradient background (blue-ish, darkening with depth).
			r := byte(20 + (x*40)/w)
			g := byte(30 + (y*50)/h)
			b := byte(80 + (x*60)/w)
			// Moving bar in race color.
			if x >= barX && x < barX+40 {
				r, g, b = 255, 94, 58
			}
			f.rgba[i+0] = r
			f.rgba[i+1] = g
			f.rgba[i+2] = b
			f.rgba[i+3] = 255
		}
	}
	return f.rgba
}

// Width implements Emulator.
func (f *FakeCore) Width() int { return f.width }

// Height implements Emulator.
func (f *FakeCore) Height() int { return f.height }

// SetButton implements Emulator.
func (f *FakeCore) SetButton(button JoyButton, pressed bool) {
	f.buttons[int(button)] = pressed
}

// Stop implements Emulator.
func (f *FakeCore) Stop() {}
