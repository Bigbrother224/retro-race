package engine

// JoyButton is a logical controller button — the UI layer maps keyboard and
// gamepad to these; the emulator never knows about physical input devices.
type JoyButton int

const (
	BtnUp JoyButton = iota
	BtnDown
	BtnLeft
	BtnRight
	BtnA
	BtnB
	BtnX
	BtnY
	BtnStart
	BtnSelect
	BtnL
	BtnR
)

// Emulator is the seam between the game UI and the emulation backend.
// The interface is small (pull model): the UI steps the emulator once per
// frame and reads the resulting RGBA framebuffer.
//
// Implementations:
//   - Core:     the real libretro core (cgo).
//   - FakeCore: deterministic Go-only frames (tests, demo mode).
type Emulator interface {
	// Start loads a core and a ROM. Must be called before Step.
	Start(romPath, corePath string) error
	// Step advances the emulation by one frame.
	Step()
	// Frame returns the latest framebuffer as tightly packed RGBA8. The slice
	// is reused across frames — valid until the next Step call.
	Frame() []byte
	// Width/Height of the current framebuffer (0 until the first frame).
	Width() int
	Height() int
	// SetButton updates a logical button state (pressed or released).
	SetButton(button JoyButton, pressed bool)
	// Stop releases the game and the core.
	Stop()
}
