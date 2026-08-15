package netplay

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"retrorace/internal/rollback"
)

// This is the integration test the app's shared-game path relies on: two
// netplay sessions (host + guest over real TCP) each drive a deterministic
// core through the rollback layer exactly like gameSession.updateNetplayPlaying
// does — Send local input, RenderNext, rollback.Commit. Both sides are driven
// and the assertion is that their cores converge to an IDENTICAL final state,
// proving the whole session + rollback + core stack is wired correctly.
// (Corrections under injected latency are exercised end-to-end by cmd/simulate;
// this test proves the lockstep wiring deterministically and fast.)

// fakeCore is a deterministic emulator seam: its state is a pure accumulator of
// the applied button history, so the same inputs always yield the same state.
// Save/Restore round-trip the state (rollback re-simulation).
type fakeCore struct {
	buttons [2][12]bool
	state   uint64
}

func (f *fakeCore) SetButton(port, id int, pressed bool) { f.buttons[port][id] = pressed }
func (f *fakeCore) Step() {
	var b uint64
	for port := 0; port < 2; port++ {
		for id := 0; id < 12; id++ {
			if f.buttons[port][id] {
				b |= 1 << uint(port*12+id)
			}
		}
	}
	f.state = f.state*31 + b + 7
}
func (f *fakeCore) Save() ([]byte, error) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, f.state)
	return buf, nil
}
func (f *fakeCore) Restore(b []byte) error {
	f.state = binary.LittleEndian.Uint64(b)
	return nil
}

// toMask packs a player's button state into the P1/P2 bitmask rollback expects.
func toMask(b [ButtonCount]bool) uint16 {
	var m uint16
	for i := range b {
		if b[i] {
			m |= 1 << i
		}
	}
	return m
}

// packInputs builds the rollback.Input for a frame from both players' states.
func packInputs(p1, p2 [ButtonCount]bool) rollback.Input {
	return rollback.Input{P1: toMask(p1), P2: toMask(p2)}
}

// integratePair launches a Host + Join on loopback, like the app's lobby does.
func integratePair(t *testing.T, gameID string) (*Session, *Session) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	hostCh := make(chan *Session, 1)
	errCh := make(chan error, 1)
	go func() {
		s, err := Host(ln, gameID)
		if err != nil {
			errCh <- err
			return
		}
		hostCh <- s
	}()
	guest, err := Join(addr, gameID)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	var host *Session
	select {
	case host = <-hostCh:
	case err := <-errCh:
		t.Fatalf("host: %v", err)
	}
	return host, guest
}

// TestSessionRollbackCoreIntegration proves the full shared-game stack keeps
// both machines in exact sync: two sessions over real TCP, each stepping a
// deterministic core through rollback, converge to an identical final state.
// runIntegration wires two sessions + two rollback sessions + two deterministic
// cores (the app's exact updateNetplayPlaying path), drives both sides for
// `frames` frames, and returns each side's final state and correction count.
func runIntegration(t *testing.T) (hs, gs uint64, hcorr, gcorr int) {
	t.Helper()
	const frames = 200
	const predictMax = 2

	host, guest := integratePair(t, "integ")
	defer host.Close()
	defer guest.Close()

	hostCore, guestCore := &fakeCore{}, &fakeCore{}
	hostRb := rollback.New(hostCore, predictMax+2)
	guestRb := rollback.New(guestCore, predictMax+2)
	host.SetDelay(0)
	guest.SetDelay(0)
	host.SetPredict(true, predictMax)
	guest.SetPredict(true, predictMax)

	// Deterministic per-player input scripts, distinct so each side's merged
	// input set is real and both cores must apply the same (host, guest) pair.
	hostIn := func(f int) [ButtonCount]bool {
		var b [ButtonCount]bool
		if f%2 == 0 {
			b[0] = true
		}
		if f%3 == 0 {
			b[2] = true
		}
		return b
	}
	guestIn := func(f int) [ButtonCount]bool {
		var b [ButtonCount]bool
		if f%2 == 1 {
			b[1] = true
		}
		if f%4 == 0 {
			b[3] = true
		}
		return b
	}

	// Send all inputs up front, like the lockstep tests do, so neither side
	// blocks on a missing input once rendering starts.
	for f := 0; f < frames; f++ {
		if !host.Send(hostIn(f)) {
			t.Fatalf("host send frame %d: %v", f, host.Err())
		}
	}
	for f := 0; f < frames; f++ {
		if !guest.Send(guestIn(f)) {
			t.Fatalf("guest send frame %d: %v", f, guest.Err())
		}
	}

	// renderSide renders frames and commits the side's own core through
	// rollback, draining any corrections — the exact gameSession loop. Host is
	// port 1 (p1=my,p2=peer), guest is port 2 (p1=peer,p2=my).
	renderSide := func(s *Session, rb *rollback.Session, core *fakeCore) (uint64, int, error) {
		corr := 0
		for f := 0; f < frames; f++ {
			my, peer, err := waitFrame(s)
			if err != nil {
				return core.state, corr, err
			}
			var p1, p2 [ButtonCount]bool
			if s.LocalPort() == 1 {
				p1, p2 = my, peer
			} else {
				p1, p2 = peer, my
			}
			rb.Commit(packInputs(p1, p2))
			for {
				frame, realPeer, ok := s.TakeCorrection()
				if !ok {
					break
				}
				if err := correctFrame(rb, int(frame), realPeer, s.LocalPort()); err != nil {
					return core.state, corr, err
				}
				corr++
			}
		}
		return core.state, corr, nil
	}

	type res struct {
		state uint64
		corr  int
		err   error
	}
	hc, gc := make(chan res, 1), make(chan res, 1)
	go func() { st, c, e := renderSide(host, hostRb, hostCore); hc <- res{st, c, e} }()
	go func() { st, c, e := renderSide(guest, guestRb, guestCore); gc <- res{st, c, e} }()

	hr, gr := <-hc, <-gc
	if hr.err != nil {
		t.Fatalf("host render: %v", hr.err)
	}
	if gr.err != nil {
		t.Fatalf("guest render: %v", gr.err)
	}
	return hr.state, gr.state, hr.corr, gr.corr
}

// TestSessionRollbackCoreIntegration proves the full shared-game stack keeps
// both machines in exact sync: two sessions over real TCP, each stepping a
// deterministic core through rollback, converge to an identical final state.
//
// Corrections under injected latency are intentionally NOT asserted here: a
// two-sided concurrent loop races on real network timing, so whether a
// correction fires (and thus the convergence path) is timing-dependent and
// would make this test flaky. The correction path is covered deterministically
// by TestPredictionAndCorrection and rollback_test, and end-to-end by simulate.
func TestSessionRollbackCoreIntegration(t *testing.T) {
	hs, gs, hcorr, gcorr := runIntegration(t)
	if hs != gs {
		t.Fatalf("DIVERGENCE: host=%d guest=%d — machines out of sync", hs, gs)
	}
	t.Logf("lockstep in sync (host corr=%d guest corr=%d): state=%d", hcorr, gcorr, hs)
}

// correctFrame re-simulates the frames from the divergence point with the real
// (corrected) peer input, mirroring gameSession.applyCorrection. It returns an
// error instead of failing, so it is safe to call from a spawned goroutine.
func correctFrame(rb *rollback.Session, frame int, realPeer uint16, localPort int) error {
	last := rb.Last()
	ins := make([]rollback.Input, 0, last-frame+1)
	for f := frame; f <= last; f++ {
		in, ok := rb.Input(f)
		if !ok {
			return errNoFrame
		}
		if f == frame {
			if localPort == 1 {
				in.P2 = realPeer
			} else {
				in.P1 = realPeer
			}
		}
		ins = append(ins, in)
	}
	return rb.Correct(frame, ins)
}

// waitFrame polls RenderNext until a frame renders (or a deadline passes). It
// returns an error instead of failing, so it is safe to call from a spawned
// goroutine.
func waitFrame(s *Session) ([ButtonCount]bool, [ButtonCount]bool, error) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		my, peer, advanced, ok := s.RenderNext()
		if !ok {
			return [ButtonCount]bool{}, [ButtonCount]bool{}, s.Err()
		}
		if advanced {
			return my, peer, nil
		}
		time.Sleep(time.Millisecond)
	}
	return [ButtonCount]bool{}, [ButtonCount]bool{}, errNoFrame
}

var errNoFrame = &timeoutError{}

type timeoutError struct{}

func (*timeoutError) Error() string { return "no frame rendered within deadline" }
