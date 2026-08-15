package netplay

import (
	"net"
	"testing"
	"time"
)

// startPair launches a Host and a Join on loopback and returns both sessions.
func startPair(t *testing.T, gameID string) (*Session, *Session) {
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

// waitRender polls RenderNext until it yields a frame (or fails).
func waitRender(t *testing.T, s *Session, what string) ([ButtonCount]bool, [ButtonCount]bool) {
	t.Helper()
	for i := 0; i < 1000; i++ {
		my, peer, advanced, ok := s.RenderNext()
		if !ok {
			t.Fatalf("%s: session ended: %v", what, s.Err())
		}
		if advanced {
			return my, peer
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s: no frame rendered within timeout", what)
	return [ButtonCount]bool{}, [ButtonCount]bool{}
}

func TestLockstepExchange(t *testing.T) {
	host, guest := startPair(t, "g1")
	defer host.Close()
	defer guest.Close()
	host.SetDelay(0)
	guest.SetDelay(0)

	hostInputs := [][ButtonCount]bool{
		{true, false, false, false, false, false, false, false, false, false, false, false}, // Up
		{false, false, false, false, false, false, false, false, true, false, false, false}, // A
		{false, false, false, true, false, false, false, false, false, false, false, false}, // Left
	}
	guestInputs := [][ButtonCount]bool{
		{false, false, false, true, false, false, false, false, false, false, false, false}, // Left
		{false, false, false, false, false, false, true, false, false, false, false, false}, // Right
		{false, true, false, false, false, false, false, false, false, false, false, false}, // Y
	}

	for _, in := range hostInputs {
		if !host.Send(in) {
			t.Fatalf("host Send: %v", host.Err())
		}
	}
	for _, in := range guestInputs {
		if !guest.Send(in) {
			t.Fatalf("guest Send: %v", guest.Err())
		}
	}

	for f := 0; f < len(hostInputs); f++ {
		hmy, hpeer := waitRender(t, host, "host")
		gmy, gpeer := waitRender(t, guest, "guest")
		if hmy != hostInputs[f] {
			t.Fatalf("frame %d host my = %v, want host input %v", f, hmy, hostInputs[f])
		}
		if hpeer != guestInputs[f] {
			t.Fatalf("frame %d host peer = %v, want guest input %v", f, hpeer, guestInputs[f])
		}
		if gmy != guestInputs[f] {
			t.Fatalf("frame %d guest my = %v, want guest input %v", f, gmy, guestInputs[f])
		}
		if gpeer != hostInputs[f] {
			t.Fatalf("frame %d guest peer = %v, want host input %v", f, gpeer, hostInputs[f])
		}
	}
}

// TestInputDelayGating verifies the input-delay warm-up: no frame is rendered
// until enough frames have been sent, and rendering never runs ahead of
// sendFrame - 1 - delay.
func TestInputDelayGating(t *testing.T) {
	host, guest := startPair(t, "g1")
	defer host.Close()
	defer guest.Close()
	const d = uint32(2)
	host.SetDelay(d)
	guest.SetDelay(d)

	in := [ButtonCount]bool{true, false, false, false, false, false, false, false, false, false, false, false}

	// Send 2 frames (< delay+1=3): nothing should render yet.
	for i := 0; i < 2; i++ {
		host.Send(in)
		guest.Send(in)
	}
	for i := 0; i < 20; i++ {
		if _, _, advanced, ok := host.RenderNext(); !ok || advanced {
			t.Fatalf("host rendered before warm-up completed (advanced=%v)", advanced)
		}
	}

	// Send one more (3 total): frame 0 becomes renderable (send-1-delay = 0).
	// The peer input may take a scheduling tick to arrive, so poll.
	host.Send(in)
	guest.Send(in)
	advanced := false
	var ok bool
	for i := 0; i < 100 && !advanced; i++ {
		_, _, advanced, ok = host.RenderNext()
		if !ok {
			t.Fatalf("host session ended: %v", host.Err())
		}
		if !advanced {
			time.Sleep(time.Millisecond)
		}
	}
	if !advanced {
		t.Fatal("host did not render frame 0 after warm-up")
	}
	// Frame 1 requires sendFrame >= 4; we've sent only 3, so it stalls.
	if _, _, advanced, ok = host.RenderNext(); !ok || advanced {
		t.Fatal("host rendered ahead of the input-delay window")
	}
}

// TestStallOnMissingPeer verifies rendering stalls (does not advance) when the
// peer has not supplied the input for the frame being rendered.
func TestStallOnMissingPeer(t *testing.T) {
	host, guest := startPair(t, "g1")
	defer host.Close()
	defer guest.Close()
	host.SetDelay(0)
	guest.SetDelay(0)

	in := [ButtonCount]bool{true, false, false, false, false, false, false, false, false, false, false, false}

	// Host sends its input for frame 0 but the guest never does: host must not
	// render frame 0 (its peer input is missing).
	host.Send(in)
	host.Send(in)
	for i := 0; i < 50; i++ {
		if _, _, advanced, ok := host.RenderNext(); !ok || advanced {
			t.Fatalf("host rendered without peer input (advanced=%v)", advanced)
		}
		time.Sleep(time.Millisecond)
	}

	// Now the guest supplies its input; host renders and the guest, which sent
	// nothing, reports it has no frame ready.
	guest.Send(in)
	var advanced bool
	for i := 0; i < 100 && !advanced; i++ {
		_, _, advanced, _ = host.RenderNext()
		time.Sleep(time.Millisecond)
	}
	if !advanced {
		t.Fatal("host did not render after the peer supplied its input")
	}
}

// TestStateHashExchange verifies a state-hash record travels across the link
// and is surfaced through PeerHash (used for divergence detection).
func TestStateHashExchange(t *testing.T) {
	host, guest := startPair(t, "g1")
	defer host.Close()
	defer guest.Close()

	const want = uint64(0xdeadbeefcafef00d)
	var idle [ButtonCount]bool
	if !host.Send(idle) {
		t.Fatalf("Send: %v", host.Err())
	}
	if !host.SendStateHash(want) {
		t.Fatalf("SendStateHash: %v", host.Err())
	}

	// The guest's readLoop delivers it asynchronously; poll for it.
	var (
		frame uint32
		hash  uint64
		ok    bool
	)
	for i := 0; i < 100; i++ {
		frame, hash, ok = guest.PeerHash()
		if ok {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !ok {
		t.Fatal("guest never received a peer state hash")
	}
	if hash != want {
		t.Fatalf("guest peer hash = %x, want %x", hash, want)
	}
	if frame == 0 {
		t.Fatalf("guest peer hash frame = 0, expected a nonzero send frame")
	}
}

// TestRTTMeasured verifies the ping/pong loop measures a round-trip time (used
// to auto-tune the input delay).
func TestRTTMeasured(t *testing.T) {
	host, guest := startPair(t, "g1")
	defer host.Close()
	defer guest.Close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if host.RTT() > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("RTT not measured within 3s (ping loop did not deliver a pong)")
}

// TestPredictionAndCorrection verifies the rollback hook: when the peer's input
// for a frame has not arrived, the session predicts it (holding the last known
// input) instead of stalling, up to predictMax frames; and when the real input
// later differs, it reports a correction at the affected frame.
func TestPredictionAndCorrection(t *testing.T) {
	host, guest := startPair(t, "g1")
	defer host.Close()
	defer guest.Close()
	host.SetDelay(0)
	guest.SetDelay(0)
	host.SetPredict(true, 2)
	guest.SetPredict(true, 2)

	a := [ButtonCount]bool{true, false, false, false, false, false, false, false, false, false, false, false}
	for i := 0; i < 4; i++ {
		if !host.Send(a) {
			t.Fatalf("host Send: %v", host.Err())
		}
	}
	// Guest sends its frame 0 now (so host renders frame 0 with a REAL input),
	// but delays frame 1 so the host predicts it.
	b := [ButtonCount]bool{false, true, false, false, false, false, false, false, false, false, false, false}
	if !guest.Send(b) {
		t.Fatalf("guest Send: %v", guest.Err())
	}

	// Frame 0 renders with the real guest input (frame 0 is never predicted).
	peer0, ok := advance(t, host)
	if !ok || peer0 != b {
		t.Fatalf("frame 0 should use the real guest input (ok=%v peer=%v)", ok, peer0)
	}
	// Frames 1 and 2 are PREDICTED: the predictor holds the last known peer
	// input (b), since the guest's frame 1 has not arrived yet.
	peer1, ok := advance(t, host)
	if !ok || peer1 != b {
		t.Fatalf("frame 1 should be predicted as the last known input (ok=%v peer=%v)", ok, peer1)
	}
	if peer2, ok := advance(t, host); !ok || peer2 != b {
		t.Fatalf("frame 2 should be predicted as the last known input (ok=%v peer=%v)", ok, peer2)
	}

	// The guest's real frame 1 is DIFFERENT (button 2) from the prediction:
	// a correction must be reported for frame 1.
	c := [ButtonCount]bool{false, false, true, false, false, false, false, false, false, false, false, false}
	if !guest.Send(c) {
		t.Fatalf("guest Send frame 1: %v", guest.Err())
	}
	var (
		frame    uint32
		realPeer uint16
		okC      bool
	)
	for i := 0; i < 100; i++ {
		frame, realPeer, okC = host.TakeCorrection()
		if okC {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !okC {
		t.Fatal("no correction reported after a differing real input arrived")
	}
	if frame != 1 {
		t.Fatalf("correction frame = %d, want 1 (the first predicted frame)", frame)
	}
	if realPeer != 0b100 {
		t.Fatalf("correction realPeer = %b, want button 2 (0b100)", realPeer)
	}
}

// advance renders the next frame on s, waiting briefly if the peer input has
// not arrived yet. It returns the peer input used and whether a frame rendered.
func advance(t *testing.T, s *Session) ([ButtonCount]bool, bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, peer, adv, ok := s.RenderNext()
		if !ok {
			t.Fatalf("session ended: %v", s.Err())
		}
		if adv {
			return peer, true
		}
		time.Sleep(time.Millisecond)
	}
	return [ButtonCount]bool{}, false
}

// TestLockstepRoles verifies each side drives the expected controller port.
func TestLockstepRoles(t *testing.T) {
	host, guest := startPair(t, "g1")
	defer host.Close()
	defer guest.Close()

	if host.Role() != RoleHost || guest.Role() != RoleGuest {
		t.Fatalf("roles: host=%v guest=%v", host.Role(), guest.Role())
	}
	if host.LocalPort() != 1 || host.RemotePort() != 2 {
		t.Fatalf("host ports: local=%d remote=%d, want 1/2", host.LocalPort(), host.RemotePort())
	}
	if guest.LocalPort() != 2 || guest.RemotePort() != 1 {
		t.Fatalf("guest ports: local=%d remote=%d, want 2/1", guest.LocalPort(), guest.RemotePort())
	}
}

// TestGameMismatchRejected verifies the handshake refuses a peer running a
// different game (ROM/core hash).
func TestGameMismatchRejected(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		_, err := Host(ln, "host-game")
		errCh <- err
	}()

	if _, err := Join(addr, "other-game"); err == nil {
		t.Fatal("Join with a mismatched game id should fail")
	}
	if err := <-errCh; err == nil {
		t.Fatal("Host should fail when the client game does not match")
	}
}
