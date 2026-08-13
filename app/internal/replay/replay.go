// Package replay records and replays a deterministic Run of player inputs.
//
// A Run stores only the player's button changes (input events with frame
// timestamps), never frames, so it can be replayed deterministically through a
// core. This is the foundation for async challenges and Verified Run replay:
// the same inputs over the same ROM/core produce the same output.
package replay

import "encoding/json"

// InputEvent is a single button state change at a frame.
type InputEvent struct {
	Frame   int  `json:"frame"`
	Button  int  `json:"button"`
	Pressed bool `json:"pressed"`
}

// Run is a recorded segment: game identity plus the ordered input events.
// Only button changes are stored; the button state between changes persists.
type Run struct {
	Game    string       `json:"game"`
	Console string       `json:"console"`
	Core    string       `json:"core"`
	ROMHash string       `json:"rom_hash"`
	ROMPath string       `json:"rom_path,omitempty"`
	Width   int          `json:"width"`
	Height  int          `json:"height"`
	Events  []InputEvent `json:"events"`
}

// Duration returns the run's frame count (the last event frame, or 0).
func (r *Run) Duration() int {
	if len(r.Events) == 0 {
		return 0
	}
	return r.Events[len(r.Events)-1].Frame
}

// MarshalRun serializes a Run to indented JSON.
func MarshalRun(r *Run) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// UnmarshalRun parses a Run from JSON.
func UnmarshalRun(data []byte) (*Run, error) {
	var r Run
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Recorder diffs each frame's button state and keeps only the changes, so a
// full run stays compact.
type Recorder struct {
	prev   []bool
	events []InputEvent
	frame  int
}

// NewRecorder creates a Recorder for buttonCount buttons.
func NewRecorder(buttonCount int) *Recorder {
	return &Recorder{prev: make([]bool, buttonCount)}
}

// Record captures one frame's button state. len(state) must be buttonCount.
func (r *Recorder) Record(state []bool) {
	for i, p := range state {
		if i < len(r.prev) && p != r.prev[i] {
			r.events = append(r.events, InputEvent{Frame: r.frame, Button: i, Pressed: p})
		}
	}
	copy(r.prev, state)
	r.frame++
}

// Events returns the recorded input events in frame order.
func (r *Recorder) Events() []InputEvent { return r.events }

// Duration returns the number of frames recorded.
func (r *Recorder) Duration() int { return r.frame }

// Core is what a Player needs to replay against: set a button and step once.
type Core interface {
	SetButton(button int, pressed bool)
	Step()
}

// Player replays a Run deterministically: each frame it applies the events
// whose timestamp matches, then steps the core once.
type Player struct {
	run   *Run
	core  Core
	frame int
	idx   int
}

// NewPlayer creates a Player for a run against the given core. It starts at
// frame 0, matching the Recorder, so the first event (Frame 0) is applied on
// the first Step.
func NewPlayer(run *Run, core Core) *Player {
	return &Player{run: run, core: core, frame: 0}
}

// Step advances the replay by one frame. It returns false once the run has
// finished (the core has run for the recorded duration).
func (p *Player) Step() bool {
	for p.idx < len(p.run.Events) && p.run.Events[p.idx].Frame == p.frame {
		e := p.run.Events[p.idx]
		p.core.SetButton(e.Button, e.Pressed)
		p.idx++
	}
	p.core.Step()
	p.frame++
	return p.frame <= p.run.Duration()
}
