// Package rival implements a headless second player: a separate process that
// runs its own libretro core on the same ROM, replays a recorded run (real,
// deterministic inputs — the product's "Beat this Ghost" async mechanic), and
// publishes its framebuffer, progress and state over shared memory.
//
// The libretro C shim is single-instance per process, so the rival MUST run in
// its own process. That is the product's "one process per player" architecture.
package rival

import (
	"fmt"
	"log"
	"os"
	"time"

	"retrorace/internal/arbiter"
	"retrorace/internal/engine"
	"retrorace/internal/replay"
	"retrorace/internal/shm"
)

// Config configures a headless rival run.
type Config struct {
	ROM     string // path to the ROM file
	Core    string // path to the core dylib
	ShmPath string // shared-memory region path
	RunPath string // optional recorded run (JSON); empty = no-input run

	// ExpectedFrames is the estimated segment length in frames, used for the
	// progress gauge when no recorded run defines a duration.
	ExpectedFrames int
	// FPS is the target publish cadence (the game's frame rate).
	FPS int
}

// Run loads the core and ROM, replays the run (if any) and publishes the
// rival's live framebuffer and race state to shared memory until the run
// finishes or the segment ends. It returns the rival's finish frame, or an
// error.
func Run(cfg Config) (int, error) {
	if cfg.FPS <= 0 {
		cfg.FPS = 60
	}

	core := engine.NewCore()
	if err := core.Start(cfg.ROM, cfg.Core); err != nil {
		return 0, fmt.Errorf("rival: core start: %w", err)
	}
	defer core.Stop()

	// Deterministic playback of a recorded run (real inputs), or a no-input
	// run when none is provided.
	var player *replay.Player
	var run *replay.Run
	if cfg.RunPath != "" {
		data, err := os.ReadFile(cfg.RunPath)
		if err != nil {
			return 0, fmt.Errorf("rival: read run: %w", err)
		}
		run, err = replay.UnmarshalRun(data)
		if err != nil {
			return 0, fmt.Errorf("rival: parse run: %w", err)
		}
		player = replay.NewPlayer(run, replayCore{core})
	}

	prod, err := shm.OpenProducer(cfg.ShmPath)
	if err != nil {
		return 0, fmt.Errorf("rival: shm producer: %w", err)
	}
	defer prod.Close()

	arb := arbiter.New(arbiter.DefaultConfig())
	arb.Reset()

	// Warm up a few frames so width/height/framebuffer are known before we
	// start publishing, mirroring the player's launch.
	for i := 0; i < 6; i++ {
		core.Step()
	}

	var frame int
	finished := 0
	interval := time.Second / time.Duration(cfg.FPS)

	for {
		start := time.Now()

		// Replay applies recorded inputs then steps; a no-input run just steps.
		if player != nil {
			if !player.Step() {
				break // run finished
			}
		} else {
			core.Step()
		}
		frame++

		fb := core.Frame()
		if fb == nil {
			continue
		}

		state := uint32(shm.StateRacing)
		total := runFrames(run, cfg, frame)
		progress := uint32(float64(frame) / float64(total) * 1000)
		if progress > 1000 {
			progress = 1000
		}

		// Real finish: the rival's own arbiter detects a persistent screen
		// change on ITS framebuffer (end screen / game over).
		if finished == 0 && arb.Update(fb) == arbiter.SegmentEnd {
			finished = frame
			state = shm.StateDone
			progress = 1000
			prod.Write(fb, core.Width(), core.Height(), state, progress, uint32(cfg.FPS), uint32(frame))
			break
		}

		prod.Write(fb, core.Width(), core.Height(), state, progress, uint32(cfg.FPS), 0)

		// Publish at the game's cadence.
		if d := time.Since(start); d < interval {
			time.Sleep(interval - d)
		}
	}

	// Signal done even if we broke out of the loop without a segment end.
	if finished == 0 {
		if fb := core.Frame(); fb != nil {
			prod.SetState(shm.StateDone, 1000, uint32(frame))
		}
		finished = frame
	}
	log.Printf("rival: finished at frame %d", finished)
	return finished, nil
}

// runFrames returns the run's recorded duration, or the expected segment
// length (so a no-input run reports progress growing toward the segment end).
func runFrames(run *replay.Run, cfg Config, frame int) int {
	if run != nil && run.Duration() > 0 {
		return run.Duration()
	}
	if cfg.ExpectedFrames > 0 {
		return cfg.ExpectedFrames
	}
	return frame
}

// replayCore adapts an engine.Emulator to replay.Core.
type replayCore struct{ e engine.Emulator }

func (c replayCore) SetButton(b int, pressed bool) { c.e.SetButton(engine.JoyButton(b), pressed) }
func (c replayCore) Step()                         { c.e.Step() }
