package shm

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestRoundTrip exercises the full producer/consumer cycle in one process (the
// mmap mechanism is identical across processes): a producer writes frames and
// state, and the consumer reads exactly those complete frames.
func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "race.shm")

	// Producer first (creates the region).
	prod, err := OpenProducer(path)
	if err != nil {
		t.Fatalf("OpenProducer: %v", err)
	}
	defer prod.Close()

	// Consumer maps the same region.
	cons, err := OpenConsumer(path)
	if err != nil {
		t.Fatalf("OpenConsumer: %v", err)
	}
	defer cons.Close()

	// Frame 1: 16x8 RGBA, all red.
	f1 := make([]byte, 16*8*4)
	for i := 0; i < len(f1); i += 4 {
		f1[i], f1[i+1], f1[i+2], f1[i+3] = 255, 0, 0, 255
	}
	prod.Write(f1, 16, 8, StateRacing, 100, 60, 0)

	snap := cons.Take()
	if snap == nil {
		t.Fatal("Take returned nil after first write")
	}
	if snap.Width != 16 || snap.Height != 8 {
		t.Fatalf("dims=%dx%d want 16x8", snap.Width, snap.Height)
	}
	if snap.State != StateRacing || snap.Progress != 100 {
		t.Fatalf("state=%d progress=%d want racing/100", snap.State, snap.Progress)
	}
	if !bytes.Equal(snap.Frame, f1) {
		t.Fatal("frame payload mismatch")
	}

	// No new frame yet.
	if cons.Take() != nil {
		t.Fatal("Take returned a frame before any new publish")
	}

	// Frame 2: all blue.
	f2 := make([]byte, 16*8*4)
	for i := 0; i < len(f2); i += 4 {
		f2[i], f2[i+1], f2[i+2], f2[i+3] = 0, 0, 255, 255
	}
	prod.Write(f2, 16, 8, StateRacing, 500, 60, 0)
	snap = cons.Take()
	if snap == nil {
		t.Fatal("Take returned nil after second write")
	}
	if snap.Progress != 500 || !bytes.Equal(snap.Frame, f2) {
		t.Fatal("second frame mismatch")
	}

	// Done state.
	prod.SetState(StateDone, 1000, 900)
	snap = cons.Take()
	if snap == nil || snap.State != StateDone || snap.FinishFrame != 900 {
		t.Fatalf("done state not delivered: %+v", snap)
	}
}

// TestConsumerBeforeProducer exercises the ordering: a consumer opening before
// the producer exists fails cleanly (the player spawns the rival first).
func TestConsumerBeforeProducer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.shm")
	if _, err := OpenConsumer(path); err == nil {
		t.Fatal("expected error opening consumer before producer")
	}
	os.Remove(path)
}
