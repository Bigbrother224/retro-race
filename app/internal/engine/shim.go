package engine

/*
#cgo CFLAGS: -I${SRCDIR}/../../Sources/CRetroRace/include/CRetroRace
#cgo LDFLAGS: -ldl
#include <stdlib.h>
#include <libretro_shim.h>
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"
)

// Core is the real libretro backend, satisfying the Emulator seam.
// The implementation owns pixel formats, pitch, framebuffer capture and
// buffer reuse — callers only ever see RGBA.
type Core struct {
	loaded  bool
	gameSet bool

	width  int
	height int
	format int

	// Reused buffers: one for the raw capture, one for the RGBA output.
	raw     []byte
	rgba    []byte
}

// NewCore creates a Core implementing Emulator.
func NewCore() *Core {
	return &Core{}
}

// Start implements Emulator: loads the core dylib and the ROM.
func (c *Core) Start(romPath, corePath string) error {
	cpath := C.CString(corePath)
	defer C.free(unsafe.Pointer(cpath))
	if rc := C.rr_load(cpath); rc != 0 {
		return fmt.Errorf("rr_load(%s) failed rc=%d", corePath, rc)
	}
	c.loaded = true

	// Query system info so the C side knows whether this core needs a real
	// file path (e.g. FCEUmm) before we load the game.
	var info [256]C.char
	C.rr_system_info(&info[0], C.size_t(len(info)))

	data, err := os.ReadFile(romPath)
	if err != nil {
		return err
	}
	if rc := C.rr_load_game(unsafe.Pointer(&data[0]), C.size_t(len(data))); rc != 0 {
		return fmt.Errorf("rr_load_game(%s) failed rc=%d", romPath, rc)
	}
	c.gameSet = true
	return nil
}

// Step implements Emulator.
func (c *Core) Step() {
	C.rr_run()
}

// Reset implements Emulator: restarts the loaded game from its initial state.
func (c *Core) Reset() {
	C.rr_reset()
}

// Frame implements Emulator: RGBA, reused buffer (valid until next Step).
func (c *Core) Frame() []byte {
	var w, h C.int
	// Capture into the reused raw buffer (bpp from the C-side format).
	bpp := c.bytesPerPixel()
	if cap(c.raw) < 1024*1024*4 {
		c.raw = make([]byte, 1024*1024*4)
	}
	if rc := C.rr_snapshot(unsafe.Pointer(&c.raw[0]), &w, &h); rc != 0 {
		return nil
	}
	c.width, c.height = int(w), int(h)
	c.format = int(C.rr_pixel_format())

	total := c.width * c.height * bpp
	raw := c.raw[:total]

	// Convert into the reused RGBA buffer.
	need := c.width * c.height * 4
	if cap(c.rgba) < need {
		c.rgba = make([]byte, need)
	}
	c.rgba = c.rgba[:need]
	ToRGBAInto(raw, bpp, c.format, c.rgba)
	return c.rgba
}

// Width implements Emulator.
func (c *Core) Width() int { return c.width }

// Height implements Emulator.
func (c *Core) Height() int { return c.height }

// SetButton implements Emulator: writes the live button state in C.
func (c *Core) SetButton(button JoyButton, pressed bool) {
	if !c.loaded {
		return
	}
	p := 0
	if pressed {
		p = 1
	}
	C.rr_set_button(C.uint(joypadID(button)), C.int(p))
}

func (c *Core) bytesPerPixel() int {
	if c.format == int(PixelFormatRGB565) {
		return 2
	}
	return 4
}

// Stop implements Emulator.
func (c *Core) Stop() {
	if c.gameSet {
		C.rr_unload_game()
		c.gameSet = false
	}
	if c.loaded {
		C.rr_unload()
		c.loaded = false
	}
	C.rr_clear_buttons()
}

// joypadID maps a logical button to a RETRO_DEVICE_ID_JOYPAD_* id.
func joypadID(b JoyButton) uint32 {
	switch b {
	case BtnB:
		return 0
	case BtnY:
		return 1
	case BtnSelect:
		return 2
	case BtnStart:
		return 3
	case BtnUp:
		return 4
	case BtnDown:
		return 5
	case BtnLeft:
		return 6
	case BtnRight:
		return 7
	case BtnA:
		return 8
	case BtnX:
		return 9
	case BtnL:
		return 10
	case BtnR:
		return 11
	}
	return 0
}
