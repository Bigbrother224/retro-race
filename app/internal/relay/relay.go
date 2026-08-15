// Package relay implements a lightweight TCP relay so two netplay players can
// connect without any NAT / port-forwarding configuration.
//
// Both players make outbound connections to the relay server and join a room by
// a short code. The relay pipes the bytes between the two members of a room. It
// never interprets the traffic (inputs and handshake flow through untouched),
// never stores a frame, and is not involved in rendering — it is a dumb, light
// byte pipe, consistent with the product's "inputs only, server never renders"
// principle.
//
// The first member of a room is the host (player 1), the second is the guest
// (player 2). A room holds exactly two members.
package relay

import (
	"bufio"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

// Role is the member position in a room.
type Role string

const (
	RoleHost  Role = "host"
	RoleGuest Role = "guest"
)

// room holds the two members of a relay session.
// room holds the two members of a relay session.
type room struct {
	mu       sync.Mutex
	host     net.Conn
	guest    net.Conn
	guestCh  chan struct{} // closed when the guest joins
	gone     chan struct{} // closed when the room is torn down
	goneOnce sync.Once

	hostOnce    sync.Once // start the host->guest forwarder
	guestOnce   sync.Once // start the guest->host forwarder
	cleanupOnce sync.Once
	cleanup     func()
}

// Serve accepts connections on ln and relays each room. It runs until ln
// closes. Rooms are removed when either member disconnects.
func Serve(ln net.Listener) error {
	rooms := make(map[string]*room)
	var roomsMu sync.Mutex
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go handleConn(conn, rooms, &roomsMu)
	}
}

func handleConn(conn net.Conn, rooms map[string]*room, roomsMu *sync.Mutex) {
	// Read the join request: "JOIN <CODE>\n". Bound it so a client that never
	// sends anything cannot leak a connection and goroutine.
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{}) // clear for the forwarded phase
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) != 2 || fields[0] != "JOIN" || !validCode(fields[1]) {
		sendLine(conn, "ERR bad-request")
		conn.Close()
		return
	}
	code := fields[1]

	roomsMu.Lock()
	r := rooms[code]
	if r == nil {
		r = &room{guestCh: make(chan struct{}), gone: make(chan struct{})}
		rooms[code] = r
	}
	roomsMu.Unlock()

	r.mu.Lock()
	var role Role
	switch {
	case r.host == nil:
		r.host = conn
		role = RoleHost
	case r.guest == nil:
		r.guest = conn
		role = RoleGuest
		close(r.guestCh)
	default:
		sendLine(conn, "ERR full")
		conn.Close()
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	if err := sendLine(conn, string(role)); err != nil {
		conn.Close()
		return
	}

	if role == RoleHost {
		// Start forwarding the host immediately (phase 1 buffers until the
		// guest joins). A host that disconnects before a guest arrives is
		// detected by its forwarder, which tears the room down — so a dead
		// host never leaks its room or poisons its code.
		r.startHost(rooms, roomsMu, code)
		select {
		case <-r.guestCh:
		case <-r.gone: // room torn down (host left or error)
			return
		case <-time.After(joinTimeout):
			r.mu.Lock()
			h := r.host
			r.mu.Unlock()
			if h != nil {
				h.Close() // ends the host forwarder, which removes the room
			}
			return
		}
		r.startGuest(rooms, roomsMu, code)
		return
	}

	// Guest: forward both directions once both are connected.
	r.startHost(rooms, roomsMu, code)
	r.startGuest(rooms, roomsMu, code)
}

// startHost begins forwarding the host's bytes to the guest (buffering until
// the guest joins). Idempotent.
func (r *room) startHost(rooms map[string]*room, roomsMu *sync.Mutex, code string) {
	r.hostOnce.Do(func() {
		r.setCleanup(rooms, roomsMu, code)
		go forward(r.host, r.dstGuest, r.cleanup)
	})
}

// startGuest begins forwarding the guest's bytes to the host. Idempotent.
func (r *room) startGuest(rooms map[string]*room, roomsMu *sync.Mutex, code string) {
	r.guestOnce.Do(func() {
		r.setCleanup(rooms, roomsMu, code)
		go forward(r.guest, r.dstHost, r.cleanup)
	})
}

func (r *room) dstGuest() net.Conn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.guest
}

func (r *room) dstHost() net.Conn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.host
}

func (r *room) setCleanup(rooms map[string]*room, roomsMu *sync.Mutex, code string) {
	r.cleanupOnce.Do(func() {
		r.cleanup = func() { cleanupRoom(rooms, roomsMu, code, r) }
	})
}

// forward reads from src and writes to dst() (which may appear later, so src is
// read immediately and buffered in the meantime). On src closure it runs the
// room cleanup, which removes the room and closes both connections.
func forward(src net.Conn, dst func() net.Conn, cleanup func()) {
	defer cleanup()
	buf := make([]byte, 8192)
	var early []byte
	// Phase 1: the destination is not connected yet; buffer source data and
	// detect a source that closed before its peer arrived.
	for dst() == nil {
		n, err := src.Read(buf)
		if n > 0 {
			early = append(early, buf[:n]...)
		}
		if err != nil {
			return // source closed before the destination appeared
		}
	}
	if len(early) > 0 {
		if _, err := dst().Write(early); err != nil {
			return
		}
	}
	// Phase 2: direct copy.
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst().Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// cleanupRoom removes the room and closes both member connections.
func cleanupRoom(rooms map[string]*room, roomsMu *sync.Mutex, code string, r *room) {
	removeRoom(rooms, roomsMu, code, r)
	r.mu.Lock()
	h, g := r.host, r.guest
	r.mu.Unlock()
	if h != nil {
		h.Close()
	}
	if g != nil {
		g.Close()
	}
	// Signal any goroutine waiting on the room (e.g. a host handler waiting for
	// a guest) that it is over, so it can exit instead of leaking. The once
	// makes cleanup idempotent (both forwarders run it).
	r.goneOnce.Do(func() { close(r.gone) })
}

// removeRoom drops a room from the map. Idempotent.
func removeRoom(rooms map[string]*room, roomsMu *sync.Mutex, code string, r *room) {
	roomsMu.Lock()
	if rooms[code] == r {
		delete(rooms, code)
	}
	roomsMu.Unlock()
}

// validCode accepts an alphanumeric room code of 4-12 characters.
func validCode(c string) bool {
	if len(c) < 4 || len(c) > 12 {
		return false
	}
	for _, r := range c {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// joinTimeout bounds how long the relay keeps a host connection open waiting
// for a guest before giving up.
const joinTimeout = 5 * time.Minute

func sendLine(conn net.Conn, line string) error {
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := conn.Write([]byte(line + "\n"))
	// Clear so the deadline never leaks into the forwarded gameplay phase.
	conn.SetWriteDeadline(time.Time{})
	return err
}

// --- client side ---

// Dial connects to the relay at relayAddr and joins the room identified by
// code. It returns a full-duplex connection that is piped to the other member
// of the room, plus this member's role. The caller can use the returned
// connection for the netplay session.
func Dial(relayAddr, code string) (net.Conn, Role, error) {
	conn, err := net.DialTimeout("tcp", relayAddr, 10*time.Second)
	if err != nil {
		return nil, "", fmt.Errorf("relay: %w", err)
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
	}
	if err := sendLine(conn, "JOIN "+code); err != nil {
		conn.Close()
		return nil, "", fmt.Errorf("relay: %w", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, "", fmt.Errorf("relay: %w", err)
	}
	line = strings.TrimSpace(line)
	switch Role(line) {
	case RoleHost:
		return conn, RoleHost, nil
	case RoleGuest:
		return conn, RoleGuest, nil
	default:
		conn.Close()
		return nil, "", errors.New("relay: " + line)
	}
}

// GenerateCode returns a random uppercase room code (6 characters).
func GenerateCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O, 1/I
	b := make([]byte, 6)
	for i := range b {
		b[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return string(b)
}
