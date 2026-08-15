// Package netplay provides delay-based, deterministic netcode for a shared game
// instance.
//
// Two machines run the same ROM + core from the same start state and exchange
// their controller inputs every frame. Each side renders a fixed number of
// frames behind "now" (the input delay), so the peer's input for the frame
// being rendered has already arrived — network round-trip time no longer costs
// one frame per frame. Both sides apply the same merged input set (controller 1
// = host, controller 2 = guest) and step their core once per rendered frame,
// keeping the shared game state identical on both machines. Only tiny input
// packets cross the network; each machine renders locally.
//
// This is delay-based netcode (the "rollback-lite" that makes internet play
// smooth); full predictive rollback with re-simulation is a larger, separate
// layer. It is valid for games that natively support a second controller; the
// game is never modified.
package netplay

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"retrorace/internal/relay"
)

// Role identifies whether this machine is the game host (player 1) or the
// joining guest (player 2).
type Role int

const (
	// RoleHost is the machine that accepted the connection and drives
	// controller port 1.
	RoleHost Role = iota
	// RoleGuest is the machine that joined and drives controller port 2.
	RoleGuest
)

// ButtonCount is the number of logical buttons per player (the SNES set).
const ButtonCount = 12

// DefaultInputDelay is the initial number of frames each side renders behind
// "now". It absorbs one network round-trip plus jitter; raise it for high-latency
// links. A larger delay is smoother but more latent.
const DefaultInputDelay = 3

// Session is a delay-based netplay link between two machines running the same
// game.
type Session struct {
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer

	role      Role
	localPort int

	wmu            sync.Mutex // serializes writes to the connection
	mu             sync.Mutex
	sendFrame      uint32                       // next frame to send (this machine's input stream)
	renderFrame    uint32                       // next frame to render/step
	delay          uint32                       // input delay: render lags send by this many frames
	recv           map[uint32]uint16            // peer inputs keyed by the frame they belong to
	my             map[uint32][ButtonCount]bool // this machine's inputs by frame
	peerHash       uint32                       // peer frame at which its latest state hash was taken
	peerHashV      uint64                       // peer's latest reported state hash
	peerHashOk     bool                         // a peer state hash has been received
	rtt            time.Duration                // latest measured round-trip time
	rttSet         bool
	autoDelay      bool              // adjust delay automatically from measured RTT
	frameTime      time.Duration     // one emulated frame, for delay tuning
	predict        bool              // predict peer input to reduce latency (rollback)
	predictMax     uint32            // max frames to predict ahead before stalling
	latency        time.Duration     // debug hook: delay applying peer inputs (simulate RTT)
	lastPeer       uint16            // last KNOWN real peer input, used for prediction
	rendered       map[uint32]uint16 // peer input actually rendered per frame
	predictedAhead uint32            // consecutive frames rendered from a prediction
	corrections    []correction      // frames whose real input differed from rendered
	done           bool
	err            error
	closed         chan struct{}
	closeOnce      sync.Once

	stats Stats
}

// Stats exposes runtime telemetry for debugging.
type Stats struct {
	Sent     uint64 // input frames sent to the peer
	Rendered uint64 // frames rendered locally
	Stalled  uint64 // times a frame was ready to render but blocked on the peer
	Buffered int    // peer inputs currently buffered
}

// Host accepts a single client on ln and performs the game handshake. gameID
// identifies the exact ROM+core so both sides verify they run the same content.
// The host drives controller port 1.
// NewSession performs the game handshake over an already-connected full-duplex
// connection and returns a ready Session. The connection can be a direct TCP
// link or a relayed link; netplay only needs a byte stream. role decides the
// controller port: host drives port 1, guest drives port 2.
func NewSession(conn net.Conn, role Role, gameID string) (*Session, error) {
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetNoDelay(true) // input packets must not be held by Nagle
	}
	localPort := 1
	if role == RoleGuest {
		localPort = 2
	}
	s := &Session{conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn), role: role, localPort: localPort, recv: make(map[uint32]uint16), my: make(map[uint32][ButtonCount]bool), rendered: make(map[uint32]uint16), delay: DefaultInputDelay, closed: make(chan struct{})}
	var err error
	if role == RoleHost {
		err = s.handshakeHost(gameID)
	} else {
		err = s.handshakeGuest(gameID)
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	// The handshake's sendLine sets a short write deadline; clear it so a slow
	// peer cannot error the session later (a blocked write is a stall, not a
	// failure).
	conn.SetWriteDeadline(time.Time{})
	s.start()
	return s, nil
}

// Host accepts a single client on ln and performs the game handshake. gameID
// identifies the exact ROM+core so both sides verify they run the same content.
// The host drives controller port 1.
func Host(ln net.Listener, gameID string) (*Session, error) {
	conn, err := ln.Accept()
	if err != nil {
		return nil, err
	}
	return NewSession(conn, RoleHost, gameID)
}

// Join connects to a host at addr and performs the game handshake. gameID must
// match the host's game. The guest drives controller port 2.
func Join(addr, gameID string) (*Session, error) {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, err
	}
	return NewSession(conn, RoleGuest, gameID)
}

// Relay joins a relay room by code and performs the game handshake over the
// relayed connection. Neither player configures NAT: both connect out to the
// relay, which pipes their inputs. The first member of the room is the host.
func Relay(relayAddr, code, gameID string) (*Session, error) {
	conn, role, err := relay.Dial(relayAddr, code)
	if err != nil {
		return nil, err
	}
	nr := RoleHost
	if role == relay.RoleGuest {
		nr = RoleGuest
	}
	return NewSession(conn, nr, gameID)
}

// SetDelay changes the input delay (frames rendered behind "now"). Raise it for
// high-latency links, lower it for responsive local play.
func (s *Session) SetDelay(d uint32) {
	s.mu.Lock()
	s.delay = d
	s.mu.Unlock()
}

// InputDelay returns the current input delay in frames.
func (s *Session) InputDelay() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.delay
}

// start launches the background reader and the RTT ping loop.
func (s *Session) start() {
	go s.readLoop()
	go s.pingLoop()
}

// pingLoop measures the round-trip time every few seconds and, when auto-delay
// is enabled, retunes the input delay so it covers the current RTT (plus one
// frame of margin) — low on a good link, higher on a laggy one.
func (s *Session) pingLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.sendPing()
			if s.autoDelay {
				s.mu.Lock()
				rtt, set, ft := s.rtt, s.rttSet, s.frameTime
				s.mu.Unlock()
				if set && ft > 0 {
					d := uint32(rtt/ft) + 1
					if d < 2 {
						d = 2
					}
					if d > 8 {
						d = 8
					}
					s.SetDelay(d)
				}
			}
		case <-s.closed:
			return
		}
	}
}

// sendPing writes a ping carrying a timestamp (a pong echoes it back).
func (s *Session) sendPing() {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	rec := [recPingLen]byte{recTypePing}
	binary.BigEndian.PutUint64(rec[1:], uint64(time.Now().UnixNano()))
	s.wmu.Lock()
	s.w.Write(rec[:])
	s.w.Flush()
	s.wmu.Unlock()
}

// sendPong echoes a ping's timestamp back so the peer can measure RTT.
func (s *Session) sendPong(ts []byte) {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	rec := [recPingLen]byte{recTypePong}
	copy(rec[1:], ts)
	s.wmu.Lock()
	s.w.Write(rec[:])
	s.w.Flush()
	s.wmu.Unlock()
}

// RTT returns the latest measured round-trip time (0 until the first ping).
func (s *Session) RTT() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rtt
}

// SetAutoDelay turns automatic input-delay tuning on or off. frameTime is the
// duration of one emulated frame (e.g. 16.67 ms at 60 fps).
func (s *Session) SetAutoDelay(enabled bool, frameTime time.Duration) {
	s.mu.Lock()
	s.autoDelay = enabled
	if frameTime > 0 {
		s.frameTime = frameTime
	}
	s.mu.Unlock()
}

// SetLatency adds an artificial delay before peer inputs are applied. It is a
// debug/testing hook to exercise rollback under simulated round-trip time.
func (s *Session) SetLatency(d time.Duration) {
	s.mu.Lock()
	s.latency = d
	s.mu.Unlock()
}

type correction struct {
	frame    uint32
	realPeer uint16
}

// SetPredict enables predictive rollback: render closer to "now" by predicting
// the peer's input when it is late, correcting via re-simulation when the real
// input differs. predictMax bounds how many frames ahead prediction may run
// before stalling (and must fit the rollback window). Disable to use the
// delay-based path.
func (s *Session) SetPredict(enabled bool, predictMax uint32) {
	s.mu.Lock()
	s.predict = enabled
	if predictMax < 1 {
		predictMax = 1
	}
	s.predictMax = predictMax
	s.mu.Unlock()
}

// TakeCorrection returns the next frame whose real peer input differed from
// what was rendered (a prediction was wrong), with the real input. The caller
// must re-simulate from that frame using the corrected peer input.
func (s *Session) TakeCorrection() (frame uint32, realPeer uint16, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.corrections) == 0 {
		return 0, 0, false
	}
	c := s.corrections[0]
	s.corrections = s.corrections[1:]
	return c.frame, c.realPeer, true
}

func (s *Session) sendLine(line string) error {
	if _, err := s.w.WriteString(line + "\n"); err != nil {
		return err
	}
	return s.w.Flush()
}

func (s *Session) handshakeHost(gameID string) error {
	line, err := s.r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("handshake read: %w", err)
	}
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) != 2 || parts[0] != "GAME" {
		s.sendLine("ERR bad-handshake")
		return fmt.Errorf("bad handshake from client: %q", line)
	}
	if parts[1] != gameID {
		s.sendLine("ERR game-mismatch")
		return fmt.Errorf("game mismatch: client runs a different ROM/core")
	}
	if err := s.sendLine("OK"); err != nil {
		return err
	}
	return s.sendLine("START")
}

func (s *Session) handshakeGuest(gameID string) error {
	if err := s.sendLine("GAME " + gameID); err != nil {
		return err
	}
	line, err := s.r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("handshake read: %w", err)
	}
	line = strings.TrimSpace(line)
	if line != "OK" {
		if strings.HasPrefix(line, "ERR") {
			return fmt.Errorf("host rejected: %s", line)
		}
		return fmt.Errorf("unexpected host reply: %q", line)
	}
	line, err = s.r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("handshake read: %w", err)
	}
	if strings.TrimSpace(line) != "START" {
		return fmt.Errorf("unexpected host message: %q", line)
	}
	return nil
}

const (
	recInputLen = 7  // [1B type][4B frame][2B button mask]
	recCheckLen = 13 // [1B type][4B frame][8B state hash]
	recPingLen  = 9  // [1B type][8B timestamp] — ping or pong
)

const (
	recTypeInput = 0
	recTypeCheck = 1
	recTypePing  = 2
	recTypePong  = 3
)

func encodeMask(state [ButtonCount]bool) uint16 {
	var m uint16
	for i := 0; i < ButtonCount; i++ {
		if state[i] {
			m |= 1 << i
		}
	}
	return m
}

func decodeMask(m uint16) [ButtonCount]bool {
	var st [ButtonCount]bool
	for i := 0; i < ButtonCount; i++ {
		if m&(1<<i) != 0 {
			st[i] = true
		}
	}
	return st
}

// Send queues the local controller input for the current send frame and reads
// any already-arrived peer inputs into the buffer. It never blocks on the peer.
// Returns false once the session has ended.
func (s *Session) Send(local [ButtonCount]bool) bool {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return false
	}
	f := s.sendFrame
	s.sendFrame++
	s.mu.Unlock()

	rec := make([]byte, recInputLen)
	rec[0] = recTypeInput
	binary.BigEndian.PutUint32(rec[1:5], f)
	binary.BigEndian.PutUint16(rec[5:7], encodeMask(local))
	s.wmu.Lock()
	_, werr := s.w.Write(rec)
	if werr == nil {
		werr = s.w.Flush()
	}
	s.wmu.Unlock()
	if werr != nil {
		s.fail(werr)
		return false
	}
	s.mu.Lock()
	s.my[f] = local
	// If the peer renders slower than we send (refresh-rate drift), keep the
	// history bounded instead of growing without limit.
	if len(s.my) > 512 {
		for k := range s.my {
			if k < s.renderFrame {
				delete(s.my, k)
			}
		}
	}
	s.stats.Sent++
	s.mu.Unlock()
	return true
}

// SendStateHash reports this machine's game-state hash (taken at the given
// frame) to the peer so both sides can detect divergence. Returns false once
// the session has ended.
func (s *Session) SendStateHash(hash uint64) bool {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return false
	}
	f := s.sendFrame
	s.mu.Unlock()

	rec := make([]byte, recCheckLen)
	rec[0] = recTypeCheck
	binary.BigEndian.PutUint32(rec[1:5], f)
	binary.BigEndian.PutUint64(rec[5:13], hash)
	s.wmu.Lock()
	_, werr := s.w.Write(rec)
	if werr == nil {
		werr = s.w.Flush()
	}
	s.wmu.Unlock()
	if werr != nil {
		s.fail(werr)
		return false
	}
	return true
}

// RenderNext returns this machine's and the peer's inputs for the next render
// frame when both are available and within the input-delay window, so the
// caller can apply the merged inputs and step its core.
//
//	local     – this machine's input for the frame to render (sent `delay`
//	            frames ago).
//	peer      – the peer's input for the same frame.
//	advanced  – true when a new frame is ready (caller must step its core once).
//	ok        – false when the session has ended (check Err).
//
// When advanced is false, the caller should not step; it simply waits (a brief
// stall while the peer's input is in flight, or the input-delay warm-up).
func (s *Session) RenderNext() (local, peer [ButtonCount]bool, advanced, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return [ButtonCount]bool{}, [ButtonCount]bool{}, false, false
	}
	// Prediction renders one frame behind "now" (instead of the full delay).
	effDelay := s.delay
	if s.predict && effDelay > 1 {
		effDelay = 1
	}
	// Not enough frames sent yet to start rendering (input-delay warm-up).
	if s.sendFrame < effDelay+1 {
		s.stats.Stalled++
		return [ButtonCount]bool{}, [ButtonCount]bool{}, false, true
	}
	// Do not render frames newer than (sent - 1 - delay): the peer needs that
	// many frames of real time to have sent its matching input.
	if s.renderFrame > s.sendFrame-effDelay-1 {
		s.stats.Stalled++
		return [ButtonCount]bool{}, [ButtonCount]bool{}, false, true
	}
	lm, haveMy := s.my[s.renderFrame]
	if !haveMy {
		s.stats.Stalled++
		return [ButtonCount]bool{}, [ButtonCount]bool{}, false, true
	}
	pm, havePeer := s.recv[s.renderFrame]
	if !havePeer {
		// No prediction for frame 0 (there is no prior state to roll back to)
		// or past the bound: wait for the peer's input instead.
		if !s.predict || s.predictedAhead >= s.predictMax || s.renderFrame < 1 {
			s.stats.Stalled++
			return [ButtonCount]bool{}, [ButtonCount]bool{}, false, true
		}
		// Predict the peer input (hold the last known one) to reduce latency.
		pm = s.lastPeer
		s.predictedAhead++
	} else {
		s.predictedAhead = 0
	}
	s.rendered[s.renderFrame] = pm
	delete(s.my, s.renderFrame)
	delete(s.recv, s.renderFrame)
	if len(s.rendered) > 512 {
		for k := range s.rendered {
			if k < s.renderFrame {
				delete(s.rendered, k)
			}
		}
	}
	s.renderFrame++
	s.stats.Rendered++
	return lm, decodeMask(pm), true, true
}

// Stats returns a copy of the session's runtime telemetry.
func (s *Session) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.stats
	st.Buffered = len(s.recv)
	return st
}

// Role returns this machine's role.
func (s *Session) Role() Role { return s.role }

// LocalPort returns the controller port this machine drives (1 = host,
// 2 = guest).
func (s *Session) LocalPort() int { return s.localPort }

// RemotePort returns the controller port the peer drives.
func (s *Session) RemotePort() int { return 3 - s.localPort }

// RenderFrame returns the number of the frame currently being rendered.
func (s *Session) RenderFrame() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.renderFrame
}

// PeerHash returns the peer's latest reported state hash and the frame at
// which it was taken. The boolean reports whether any peer hash has arrived.
func (s *Session) PeerHash() (frame uint32, hash uint64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peerHash, s.peerHashV, s.peerHashOk
}

// Err returns the fatal error that ended the session, if any.
func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Done reports whether the session has ended.
func (s *Session) Done() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done
}

// readLoop reads peer records until the connection ends or fails. Input
// records are buffered by frame; state-hash records update the latest peer
// check value.
func (s *Session) readLoop() {
	for {
		var t [1]byte
		if _, err := io.ReadFull(s.r, t[:]); err != nil {
			s.fail(err)
			return
		}
		switch t[0] {
		case recTypeInput:
			var pr [6]byte
			if _, err := io.ReadFull(s.r, pr[:]); err != nil {
				s.fail(err)
				return
			}
			pf := binary.BigEndian.Uint32(pr[0:4])
			mask := binary.BigEndian.Uint16(pr[4:6])
			s.mu.Lock()
			lat := s.latency
			s.mu.Unlock()
			store := func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				if s.done {
					return
				}
				s.recv[pf] = mask
				s.lastPeer = mask
				if used, wasRendered := s.rendered[pf]; wasRendered && used != mask {
					s.corrections = append(s.corrections, correction{pf, mask})
				}
			}
			if lat > 0 {
				// Realistic network latency: the packet "arrives" late, but the
				// socket is drained immediately (no buffer back-up).
				time.AfterFunc(lat, store)
			} else {
				store()
			}
		case recTypeCheck:
			var pr [12]byte
			if _, err := io.ReadFull(s.r, pr[:]); err != nil {
				s.fail(err)
				return
			}
			pf := binary.BigEndian.Uint32(pr[0:4])
			h := binary.BigEndian.Uint64(pr[4:12])
			s.mu.Lock()
			if !s.done {
				s.peerHash = pf
				s.peerHashV = h
				s.peerHashOk = true
			}
			s.mu.Unlock()
		case recTypePing:
			var pr [8]byte
			if _, err := io.ReadFull(s.r, pr[:]); err != nil {
				s.fail(err)
				return
			}
			s.sendPong(pr[:]) // echo the timestamp back to measure RTT
		case recTypePong:
			var pr [8]byte
			if _, err := io.ReadFull(s.r, pr[:]); err != nil {
				s.fail(err)
				return
			}
			ts := binary.BigEndian.Uint64(pr[:])
			s.mu.Lock()
			s.rtt = time.Duration(uint64(time.Now().UnixNano()) - ts)
			s.rttSet = true
			s.mu.Unlock()
		default:
			s.fail(fmt.Errorf("netplay: unknown record type %d", t[0]))
			return
		}
	}
}

func (s *Session) fail(err error) {
	s.mu.Lock()
	if !s.done {
		s.done = true
		s.err = err
	}
	s.mu.Unlock()
	s.Close()
}

// Close ends the session and stops its background goroutines.
func (s *Session) Close() {
	s.closeOnce.Do(func() { close(s.closed) })
	if s.conn != nil {
		s.conn.Close()
	}
}
