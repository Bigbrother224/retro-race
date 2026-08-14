package app

import "testing"

func TestFrameRingSampling(t *testing.T) {
	r := newFrameRing(4, 8, 3) // frameSize 4 bytes, max 8, keep every 3rd
	for i := 0; i < 20; i++ {
		r.Add([]byte{byte(i), byte(i), byte(i), byte(i)})
	}
	// Kept frames: 0,3,6,9,12,15,18 → 7
	if r.Len() != 7 {
		t.Fatalf("Len=%d want 7", r.Len())
	}
	for i, w := range []int{0, 3, 6, 9, 12, 15, 18} {
		if got := int(r.Frame(i)[0]); got != w {
			t.Fatalf("Frame(%d)=%d want %d", i, got, w)
		}
	}
}

func TestFrameRingWrapKeepsNewest(t *testing.T) {
	r := newFrameRing(4, 3, 1) // keep every frame, max 3
	for i := 0; i < 5; i++ {
		r.Add([]byte{byte(i), 0, 0, 0})
	}
	if r.Len() != 3 {
		t.Fatalf("Len=%d want 3", r.Len())
	}
	for i, w := range []int{2, 3, 4} {
		if got := int(r.Frame(i)[0]); got != w {
			t.Fatalf("Frame(%d)=%d want %d", i, got, w)
		}
	}
}

func TestRacePlayerWinsWhenFinishingFirst(t *testing.T) {
	r := newRace(DefaultRaceConfig())
	r.AddOppFrame([]byte{9, 9, 9, 9})

	for i := 0; i < 10; i++ {
		r.Tick()
	}
	r.PlayerFinished() // player finishes at frame 10
	if r.State() != raceSlowmo {
		t.Fatalf("state=%v want slowmo", r.State())
	}
	if r.Winner() != 1 {
		t.Fatalf("winner=%d want player (1)", r.Winner())
	}
}

func TestRaceOpponentWinsWhenRealFinishArrives(t *testing.T) {
	r := newRace(DefaultRaceConfig())
	r.AddOppFrame([]byte{9, 9, 9, 9})

	// The rival really finishes (via its own arbiter, delivered over shared
	// memory) before the player reaches the end.
	r.OppFinished()
	if r.State() != raceSlowmo {
		t.Fatalf("state=%v want slowmo after real rival finish", r.State())
	}
	if r.Winner() != 2 {
		t.Fatalf("winner=%d want opponent (2)", r.Winner())
	}
	// A later player finish must not overturn the rival's win.
	r.PlayerFinished()
	if r.Winner() != 2 {
		t.Fatalf("winner=%d want opponent (2) after late player finish", r.Winner())
	}
}

func TestRacePlayerFinishedLatches(t *testing.T) {
	r := newRace(DefaultRaceConfig())
	r.AddOppFrame([]byte{9, 9, 9, 9})

	r.PlayerFinished()
	if r.State() != raceSlowmo {
		t.Fatalf("state=%v want slowmo", r.State())
	}
	r.PlayerFinished() // second call must be a no-op
	if r.State() != raceSlowmo {
		t.Fatal("second PlayerFinished changed the state")
	}
}

func TestRaceFullTransition(t *testing.T) {
	cfg := DefaultRaceConfig()
	cfg.SlowmoSeconds = 1 // 60 slowmo ticks
	r := newRace(cfg)
	pf := []byte{1, 1, 1, 1}
	of := []byte{2, 2, 2, 2}

	// Populate the replay rings during the playing phase.
	for i := 0; i < 30; i++ {
		r.Tick()
		r.AddPlayerFrame(pf)
		r.AddOppFrame(of)
	}
	if r.replayPlayer.Len() == 0 || r.replayOpp.Len() == 0 {
		t.Fatal("replay rings not populated")
	}

	r.PlayerFinished()
	if r.State() != raceSlowmo {
		t.Fatalf("state=%v want slowmo", r.State())
	}
	for r.State() == raceSlowmo {
		r.Tick()
	}
	if r.State() != raceReplay {
		t.Fatalf("state=%v want replay", r.State())
	}
	replayLen := r.replayPlayer.Len()
	// Replaying each stored frame for stride ticks must eventually finish.
	for r.State() == raceReplay {
		r.Tick()
	}
	if r.State() != raceDone {
		t.Fatalf("state=%v want done", r.State())
	}
	if r.ReplayIndex() != replayLen-1 {
		t.Fatalf("ReplayIndex=%d want %d (clamped to last)", r.ReplayIndex(), replayLen-1)
	}
}

func TestRaceProgress(t *testing.T) {
	cfg := DefaultRaceConfig()
	cfg.ExpectedFrames = 100
	r := newRace(cfg)
	r.AddOppFrame([]byte{9, 9, 9, 9})

	for i := 0; i < 50; i++ {
		r.Tick()
	}
	p := r.PlayerProgress()
	if p < 0.49 || p > 0.51 {
		t.Fatalf("PlayerProgress=%v want ~0.5", p)
	}
	if r.OppProgress() != p {
		t.Fatalf("OppProgress=%v != PlayerProgress=%v (simultaneous start)", r.OppProgress(), p)
	}
}

func TestRaceWinnerTime(t *testing.T) {
	r := newRace(DefaultRaceConfig())
	r.AddOppFrame([]byte{9, 9, 9, 9})

	for i := 0; i < 10; i++ {
		r.Tick()
	}
	// Rival finishes at the current frame; the player finishes later.
	r.OppFinished()
	rivalFrame := r.frame
	for i := 0; i < 20; i++ {
		r.Tick()
	}
	r.PlayerFinished()
	if r.Winner() != 2 || r.WinnerFinishFrames() != rivalFrame {
		t.Fatalf("winner=%d time=%d want opponent(2) at %d", r.Winner(), r.WinnerFinishFrames(), rivalFrame)
	}
}
