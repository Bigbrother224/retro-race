package replay

import "testing"

func TestRecorderOnlyRecordsChanges(t *testing.T) {
	r := NewRecorder(4)
	r.Record([]bool{true, false, false, false}) // frame 0: button 0 pressed
	r.Record([]bool{true, false, false, false}) // frame 1: no change -> nothing
	r.Record([]bool{true, true, false, false})  // frame 2: button 1 pressed
	r.Record([]bool{false, true, false, false}) // frame 3: button 0 released

	ev := r.Events()
	if len(ev) != 3 {
		t.Fatalf("events=%d want 3", len(ev))
	}
	want := []InputEvent{
		{Frame: 0, Button: 0, Pressed: true},
		{Frame: 2, Button: 1, Pressed: true},
		{Frame: 3, Button: 0, Pressed: false},
	}
	for i, w := range want {
		if ev[i] != w {
			t.Fatalf("ev[%d]=%+v want %+v", i, ev[i], w)
		}
	}
	if r.Duration() != 4 {
		t.Fatalf("Duration=%d want 4", r.Duration())
	}
}

func TestRunRoundTrip(t *testing.T) {
	run := &Run{
		Game: "Alter Ego", Console: "nes", Core: "fceumm", ROMHash: "abc",
		Width: 256, Height: 240,
		Events: []InputEvent{
			{Frame: 0, Button: 2, Pressed: true},
			{Frame: 7, Button: 2, Pressed: false},
		},
	}
	data, err := MarshalRun(run)
	if err != nil {
		t.Fatalf("MarshalRun: %v", err)
	}
	got, err := UnmarshalRun(data)
	if err != nil {
		t.Fatalf("UnmarshalRun: %v", err)
	}
	if got.Game != run.Game || got.Console != run.Console || got.Core != run.Core ||
		got.ROMHash != run.ROMHash || got.Width != run.Width || got.Height != run.Height {
		t.Fatalf("round-trip metadata mismatch: %+v", got)
	}
	if len(got.Events) != 2 || got.Events[0] != run.Events[0] || got.Events[1] != run.Events[1] {
		t.Fatalf("round-trip events mismatch: %+v", got.Events)
	}
}

// fakeCore records the button state after each Step for determinism checks.
type fakeCore struct {
	steps []fakeState
	cur   [12]bool
}

type fakeState [12]bool

func (f *fakeCore) SetButton(b int, pressed bool) { f.cur[b] = pressed }
func (f *fakeCore) Step()                         { f.steps = append(f.steps, f.cur) }

func TestPlayerAppliesInputsDeterministically(t *testing.T) {
	run := &Run{Events: []InputEvent{
		{Frame: 0, Button: 0, Pressed: true},  // first press at frame 0
		{Frame: 4, Button: 0, Pressed: false}, // release at frame 4
		{Frame: 4, Button: 3, Pressed: true},  // another button same frame
	}}
	fc := &fakeCore{}
	p := NewPlayer(run, fc)

	for p.Step() {
		// Run until Step reports the run is finished.
	}
	// Frames 0..4 = 5 core steps (Duration is the last event frame, 4).
	if len(fc.steps) != 5 {
		t.Fatalf("core steps recorded %d, want 5", len(fc.steps))
	}
	// Frame 0 event must be applied on the first step.
	if !fc.steps[0][0] {
		t.Fatal("button 0 (Frame 0) must be down after the first step")
	}
	// Button 0 held through frames 1..3, released at frame 4.
	for i := 1; i < 4; i++ {
		if !fc.steps[i][0] {
			t.Fatalf("button 0 released on frame %d", i)
		}
	}
	if fc.steps[4][0] {
		t.Fatal("button 0 should be up after frame 4")
	}
	if !fc.steps[4][3] {
		t.Fatal("button 3 should be down after frame 4")
	}
}

func TestPlayerEmptyRunEndsImmediately(t *testing.T) {
	fc := &fakeCore{}
	p := NewPlayer(&Run{}, fc)
	if p.Step() {
		t.Fatal("empty run must finish immediately")
	}
	if len(fc.steps) != 1 {
		t.Fatalf("empty run stepped %d times, want 1 (start frame only)", len(fc.steps))
	}
}

// TestPlayerHeldButtonRunsFullDuration guards the real replay contract: a
// player holding a button (e.g. running right) records ONE event but hundreds
// of frames. The replay must run for the stored Frames, not just until the
// last input change — otherwise a rival replaying the run "finishes" after a
// single frame.
func TestPlayerHeldButtonRunsFullDuration(t *testing.T) {
	run := &Run{
		Frames: 300, // real recorded duration
		Events: []InputEvent{{Frame: 0, Button: 7, Pressed: true}},
	}
	fc := &fakeCore{}
	p := NewPlayer(run, fc)

	steps := 0
	for p.Step() {
		steps++
	}
	if steps != run.Frames {
		t.Fatalf("replay ran %d steps, want %d (full recorded duration)", steps, run.Frames)
	}
	// The held button stays down for every frame.
	for i := 0; i < len(fc.steps); i++ {
		if !fc.steps[i][7] {
			t.Fatalf("held button 7 released on replay frame %d", i)
		}
	}
}

// TestRunDurationPrefersStoredFrames guards serialization round-trip of the
// explicit duration.
func TestRunDurationPrefersStoredFrames(t *testing.T) {
	run := &Run{Frames: 300, Events: []InputEvent{{Frame: 0, Button: 7, Pressed: true}}}
	data, err := MarshalRun(run)
	if err != nil {
		t.Fatalf("MarshalRun: %v", err)
	}
	got, err := UnmarshalRun(data)
	if err != nil {
		t.Fatalf("UnmarshalRun: %v", err)
	}
	if got.Duration() != 300 {
		t.Fatalf("Duration=%d want 300 (stored Frames, not last event frame)", got.Duration())
	}
}
