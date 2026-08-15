package rollback

import (
	"encoding/binary"
	"testing"
)

// fakeCore is a deterministic emulator seam: a state accumulator that depends
// on the current button states at each step. Save/Restore round-trip the state.
type fakeCore struct {
	buttons [2][12]bool
	state   uint64
}

func (f *fakeCore) SetButton(port, id int, pressed bool) { f.buttons[port][id] = pressed }
func (f *fakeCore) Step() {
	for port := 0; port < 2; port++ {
		for id := 0; id < 12; id++ {
			if f.buttons[port][id] {
				f.state = f.state*1000003 ^ uint64(port*12+id+1)
			}
		}
	}
	f.state = f.state*7 + 1
}
func (f *fakeCore) Save() ([]byte, error) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, f.state)
	return b, nil
}
func (f *fakeCore) Restore(b []byte) error {
	f.state = binary.BigEndian.Uint64(b)
	return nil
}

func mask(buttons ...int) uint16 {
	var m uint16
	for _, b := range buttons {
		m |= 1 << b
	}
	return m
}

// hashState returns the core's current state (via Save) for comparison.
func hashState(c Core) uint64 {
	b, _ := c.Save()
	return binary.BigEndian.Uint64(b)
}

// runFromScratch computes the final state of a fresh core driven by inputs.
func runFromScratch(t *testing.T, ins []Input) uint64 {
	t.Helper()
	c := &fakeCore{}
	s := New(c, 16)
	for _, in := range ins {
		s.Commit(in)
	}
	return hashState(c)
}

// TestDeterministicReplay proves the same inputs over the same core always
// produce the same state — the precondition of rollback.
func TestDeterministicReplay(t *testing.T) {
	ins := []Input{{P1: mask(0, 4), P2: mask(1)}, {P1: mask(2)}, {P1: 0, P2: mask(3, 7)}, {P1: mask(9)}}
	if a, b := runFromScratch(t, ins), runFromScratch(t, ins); a != b {
		t.Fatalf("determinism broken: %x != %x", a, b)
	}
}

// TestCorrectReproducesReference is the heart of rollback: commit a sequence
// where the frame-3 input was predicted wrong, correct it, and verify the
// final state exactly matches a fresh run that used the right input all along.
func TestCorrectReproducesReference(t *testing.T) {
	// True input sequence.
	correct := []Input{
		{P1: mask(0), P2: 0},
		{P1: mask(1), P2: mask(2)},
		{P1: 0, P2: mask(3)},
		{P1: mask(4), P2: mask(5)}, // frame 3: real input X3
		{P1: mask(6), P2: 0},
		{P1: 0, P2: mask(7)},
	}
	reference := runFromScratch(t, correct)

	// Simulate the wrong prediction at frame 3 (Y3 != X3).
	wrong := []Input{correct[0], correct[1], correct[2], {P1: mask(11), P2: 0}, correct[4], correct[5]}

	c := &fakeCore{}
	s := New(c, 16)
	for _, in := range wrong {
		s.Commit(in)
	}
	got := hashState(c)
	if got == reference {
		t.Fatal("test setup: the wrong prediction must actually differ from the reference")
	}

	// Correct frames 3..5 with the real inputs.
	if err := s.Correct(3, correct[3:]); err != nil {
		t.Fatalf("Correct: %v", err)
	}
	if after := hashState(c); after != reference {
		t.Fatalf("corrected state %x != reference %x: re-simulation did not match", after, reference)
	}
}

// TestCorrectOutOfWindow reports an error when the divergence is older than the
// ring (cannot be corrected), rather than corrupting state.
func TestCorrectOutOfWindow(t *testing.T) {
	c := &fakeCore{}
	s := New(c, 2) // tiny window
	for i := 0; i < 5; i++ {
		s.Commit(Input{P1: mask(int(i))})
	}
	if err := s.Correct(0, []Input{{P1: mask(1)}, {P1: mask(2)}, {P1: mask(3)}, {P1: mask(4)}, {P1: mask(5)}}); err == nil {
		t.Fatal("correction before frame 0 should fail")
	}
	if err := s.Correct(1, []Input{{P1: mask(9)}, {P1: mask(4)}, {P1: mask(5)}, {P1: mask(6)}}); err == nil {
		t.Fatal("correction of a frame older than the window should fail")
	}
}
