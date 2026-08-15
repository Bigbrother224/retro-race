// Package rollback implements the re-simulation core of predictive netcode.
//
// Delay-based netcode renders a fixed number of frames behind "now" so the
// peer's input has arrived; that adds latency. Rollback instead renders as
// close to "now" as possible by PREDICTING the peer's input (hold the last
// known one). When the peer's real input arrives and differs from what was
// predicted, the diverged frames are re-simulated: the emulator restores a
// saved state before the divergence and replays the affected frames with the
// correct inputs. Because 8/16-bit emulators are deterministic, the replay
// reproduces exactly what would have happened had the real input been known,
// so both machines stay in sync with far lower latency.
//
// The correction window is bounded (the prediction depth), so save states and
// re-simulation stay small and fast.
package rollback

import (
	"errors"
	"fmt"
)

// Input is the two-player button state for one frame (bitmask per port).
type Input struct {
	P1, P2 uint16
}

// Core is the emulation seam rollback needs: step, set buttons, and save /
// restore the full game state. The real libretro core implements this via its
// serialize support.
type Core interface {
	Step()
	SetButton(port, id int, pressed bool)
	Save() ([]byte, error)
	Restore([]byte) error
}

// Frame records one rendered frame so it can be re-simulated after a
// correction.
type Frame struct {
	Num   int
	State []byte // core state after this frame was stepped with Input
	In    Input
}

// Session keeps a ring of the most recent frames and can roll back and replay
// a corrected window. The core is always left at the current (last committed)
// frame.
type Session struct {
	core   Core
	window int
	ring   []Frame
	last   int // highest committed frame number
	count  int // number of ring slots filled
}

// New creates a rollback Session with a ring of window recent frames. window
// must be >= 1.
func New(core Core, window int) *Session {
	if window < 1 {
		window = 1
	}
	return &Session{core: core, window: window, ring: make([]Frame, window), last: -1}
}

func applyInput(core Core, in Input) {
	for i := 0; i < 12; i++ {
		core.SetButton(0, i, in.P1&(1<<i) != 0)
		core.SetButton(1, i, in.P2&(1<<i) != 0)
	}
}

// Commit applies the input for the next frame, steps the core, and records the
// resulting state. It is called once per rendered frame.
func (s *Session) Commit(in Input) {
	applyInput(s.core, in)
	s.core.Step()
	state, _ := s.core.Save()
	s.last++
	slot := s.last % s.window
	s.ring[slot] = Frame{Num: s.last, State: state, In: in}
	if s.count < s.window {
		s.count++
	}
}

// Last returns the highest committed frame number.
func (s *Session) Last() int { return s.last }

// Input returns the input recorded for a frame that is still in the window,
// used to reconstruct the corrected sequence when a prediction is fixed.
func (s *Session) Input(frame int) (Input, bool) {
	slot, ok := s.slot(frame)
	if !ok {
		return Input{}, false
	}
	return s.ring[slot].In, true
}

// slot returns the ring slot holding the given frame, and whether it is still
// in the window.
func (s *Session) slot(frame int) (int, bool) {
	if frame > s.last || frame < 0 {
		return 0, false
	}
	if s.last-frame >= s.window {
		return 0, false // older than the ring
	}
	slot := frame % s.window
	if s.ring[slot].Num != frame {
		return 0, false
	}
	return slot, true
}

// Correct re-simulates from fromFrame (the first frame whose input changed).
// ins holds the corrected inputs for frames fromFrame..Last (len must equal
// Last-fromFrame+1). The core is left at the corrected state for Last.
func (s *Session) Correct(fromFrame int, ins []Input) error {
	if fromFrame <= 0 {
		return errors.New("rollback: cannot correct before frame 0")
	}
	expected := s.last - fromFrame + 1
	if len(ins) != expected {
		return fmt.Errorf("rollback: need %d corrected inputs, got %d", expected, len(ins))
	}
	// Restore the state before the diverged frame (the state after fromFrame-1).
	prevSlot, ok := s.slot(fromFrame - 1)
	if !ok {
		return fmt.Errorf("rollback: state before frame %d no longer in window", fromFrame)
	}
	if err := s.core.Restore(s.ring[prevSlot].State); err != nil {
		return err
	}
	// Replay fromFrame..Last with the corrected inputs, recording new states.
	for i, in := range ins {
		applyInput(s.core, in)
		s.core.Step()
		state, _ := s.core.Save()
		slot := (fromFrame + i) % s.window
		s.ring[slot] = Frame{Num: fromFrame + i, State: state, In: in}
	}
	return nil
}

// Predict returns the peer input to use when the real one has not arrived yet.
// The simplest safe predictor holds the last known peer input (buttons are
// usually held briefly), so a correction only happens when the peer actually
// changed input.
func Predict(lastKnown uint16) uint16 { return lastKnown }
