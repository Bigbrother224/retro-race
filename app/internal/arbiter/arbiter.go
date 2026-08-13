// Package arbiter implements the Race Arbiter: a component that detects a
// segment end by a large persistent change of the emulator framebuffer.
//
// It is a pure state machine over tightly packed RGBA8 frames (4 bytes per
// pixel), independent of any core and of the framebuffer dimensions, so it is
// unit-testable without a game or a display.
package arbiter

// Event is a detection outcome returned by Arbiter.Update.
type Event int

const (
	// None means nothing was detected.
	None Event = iota
	// SegmentEnd means a segment end (end screen / Game Over) was detected.
	SegmentEnd
)

// Config tunes the transition→settle state machine.
type Config struct {
	// ChangeFrac is the fraction of changed pixels that counts as a big
	// frame transition (e.g. an end screen appearing).
	ChangeFrac float64
	// SettleFrac is the fraction below which the screen is considered
	// settled (static), i.e. the transition has finished.
	SettleFrac float64
	// SettleFrames is how many consecutive settled frames must hold before
	// SegmentEnd fires (persistence of the change).
	SettleFrames int
}

// DefaultConfig returns sensible defaults, tunable per game if needed.
func DefaultConfig() Config {
	return Config{
		ChangeFrac:   0.35,
		SettleFrac:   0.02,
		SettleFrames: 15,
	}
}

type state int

const (
	statePlaying state = iota
	stateTransitioning
	stateSettling
	stateEnded
)

// Arbiter detects a segment end via a big frame transition followed by the
// screen settling for K frames. Continuous gameplay (scroll, animation) never
// settles, so it does not fire.
type Arbiter struct {
	cfg       Config
	state     state
	prev      []byte
	settleCnt int
}

// New returns an Arbiter with the given config.
func New(cfg Config) *Arbiter {
	return &Arbiter{cfg: cfg, state: statePlaying}
}

// Reset clears the machine for a new segment or game. It also releases the
// reference to the previous frame.
func (a *Arbiter) Reset() {
	a.state = statePlaying
	a.prev = nil
	a.settleCnt = 0
}

// Update feeds one RGBA8 frame and returns SegmentEnd exactly once when a
// segment end is detected, then None until Reset.
func (a *Arbiter) Update(frame []byte) Event {
	if a.state == stateEnded {
		return None
	}

	delta := 0.0
	if a.prev != nil && len(frame) == len(a.prev) && len(frame) > 0 {
		delta = changedFraction(frame, a.prev)
	}
	// Copy: the emulator reuses its framebuffer across frames.
	a.prev = append(a.prev[:0], frame...)

	switch a.state {
	case statePlaying:
		if delta >= a.cfg.ChangeFrac {
			a.state = stateTransitioning
		}

	case stateTransitioning:
		switch {
		case delta >= a.cfg.ChangeFrac:
			// Still changing.
		case delta <= a.cfg.SettleFrac:
			a.state = stateSettling
			a.settleCnt = 1
		default:
			// Medium activity: active gameplay resumes.
			a.state = statePlaying
		}

	case stateSettling:
		switch {
		case delta <= a.cfg.SettleFrac:
			a.settleCnt++
			if a.settleCnt >= a.cfg.SettleFrames {
				a.state = stateEnded
				return SegmentEnd
			}
		case delta >= a.cfg.ChangeFrac:
			a.state = stateTransitioning
			a.settleCnt = 0
		default:
			// Medium activity interrupts the settle.
			a.state = statePlaying
			a.settleCnt = 0
		}
	}
	return None
}

// changedFraction returns the fraction of pixels whose RGB differs beyond a
// small tolerance between two tightly packed RGBA8 frames.
func changedFraction(a, b []byte) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	const tol = 8 // per-channel tolerance against noise
	n := len(a) / 4
	changed := 0
	for i := 0; i < n; i++ {
		j := i * 4
		if absDiff(a[j], b[j]) > tol || absDiff(a[j+1], b[j+1]) > tol || absDiff(a[j+2], b[j+2]) > tol {
			changed++
		}
	}
	return float64(changed) / float64(n)
}

func absDiff(a, b byte) int {
	d := int(a) - int(b)
	if d < 0 {
		return -d
	}
	return d
}
