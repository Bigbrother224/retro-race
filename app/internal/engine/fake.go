package engine

import "fmt"

// FakeCore is a Go-only Emulator for tests and demo mode. It renders a
// deterministic animated racing scene (a scrolling track with a moving car)
// instead of a real game, so the UI and race logic can be exercised without a
// core dylib or a ROM. The scene is recognisable as a game so the rival's
// picture-in-picture window reads as "the opponent playing", not a test tile.
type FakeCore struct {
	width, height int
	frame         int
	rgba          []byte
	buttons       [2][16]bool
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
	roadW := w / 3
	x0 := w / 2
	// Car sways gently around the centre of the road.
	carX := x0 + (sway(f.frame*5)/128)*(roadW/2)
	carY := h/2 + (sway(f.frame*7+40)/128)*(h/6)
	for y := 0; y < h; y++ {
		// Dashed centre line scrolls downward with the frame.
		dash := ((y-f.frame*2)/14)%2 == 0
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			r, g, b := groundPixel(x, y, x0, roadW, dash)
			// Car: yellow body, dark outline/wheels.
			dx, dy := x-carX, y-carY
			if abs(dx) <= 7 && abs(dy) <= 7 {
				if abs(dx) <= 4 && abs(dy) <= 4 {
					r, g, b = 255, 213, 58 // body
				} else {
					r, g, b = 40, 40, 48 // wheels/edge
				}
			}
			f.rgba[i+0] = byte(r)
			f.rgba[i+1] = byte(g)
			f.rgba[i+2] = byte(b)
			f.rgba[i+3] = 255
		}
	}
	return f.rgba
}

// sway returns a coarse sine in [-128,127].
func sway(v int) int {
	tab := [8]int{0, 45, 90, 127, 90, 45, 0, -45}
	idx := (v%8 + 8) % 8
	return tab[idx]
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// groundPixel returns grass on the sides, asphalt in the road band, with the
// dashed centre line already accounted for.
func groundPixel(x, y, x0, roadW int, dash bool) (int, int, int) {
	if x < x0-roadW/2 || x > x0+roadW/2 {
		// Grass.
		if (x/16)%2 == 0 {
			return 34, 96, 48
		}
		return 28, 82, 40
	}
	// Asphalt with a dashed centre line down the middle.
	if abs(x-x0) <= 3 {
		if dash {
			return 230, 230, 230
		}
		return 52, 54, 62
	}
	if (y/8)%2 == 0 {
		return 44, 46, 54
	}
	return 52, 54, 62
}

// Width implements Emulator.
func (f *FakeCore) Width() int { return f.width }

// Height implements Emulator.
func (f *FakeCore) Height() int { return f.height }

// SetButton implements Emulator: writes the button state on port 0.
func (f *FakeCore) SetButton(button JoyButton, pressed bool) {
	f.SetButtonPort(0, button, pressed)
}

// SetButtonPort implements Emulator: writes the button state for a port.
func (f *FakeCore) SetButtonPort(port int, button JoyButton, pressed bool) {
	if port < 0 || port >= len(f.buttons) || int(button) >= len(f.buttons[port]) {
		return
	}
	f.buttons[port][int(button)] = pressed
}

// Reset implements Emulator: restarts the animation from frame 0.
func (f *FakeCore) Reset() { f.frame = 0 }

// Stop implements Emulator.
func (f *FakeCore) Stop() {}
