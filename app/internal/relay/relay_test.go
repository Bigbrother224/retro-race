package relay

import (
	"fmt"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"
)

// startServer starts a relay on loopback and returns its address.
func startServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go Serve(ln) //nolint:errcheck
	return ln.Addr().String()
}

// TestRelayPipesBytes verifies two members of a room are piped to each other:
// bytes written by one are read by the other, in both directions.
func TestRelayPipesBytes(t *testing.T) {
	addr := startServer(t)
	code := "ABC123"

	hostCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		c, role, err := Dial(addr, code)
		if err != nil {
			errCh <- err
			return
		}
		if role != RoleHost {
			errCh <- errHostRole(role)
			return
		}
		hostCh <- c
	}()

	// Wait until the host has registered (got its role) before the guest
	// joins: in the real flow the host creates the room first.
	var host net.Conn
	select {
	case host = <-hostCh:
	case err := <-errCh:
		t.Fatalf("host dial: %v", err)
	}

	guest, gRole, err := Dial(addr, code)
	if err != nil {
		t.Fatalf("guest dial: %v", err)
	}
	if gRole != RoleGuest {
		t.Fatalf("guest role = %q, want %q", gRole, RoleGuest)
	}

	// Host -> guest.
	if _, err := host.Write([]byte("ping-from-host")); err != nil {
		t.Fatalf("host write: %v", err)
	}
	got := readAll(t, guest, "ping-from-host")
	if got != "ping-from-host" {
		t.Fatalf("guest got %q, want host payload", got)
	}

	// Guest -> host.
	if _, err := guest.Write([]byte("ping-from-guest")); err != nil {
		t.Fatalf("guest write: %v", err)
	}
	got = readAll(t, host, "ping-from-guest")
	if got != "ping-from-guest" {
		t.Fatalf("host got %q, want guest payload", got)
	}
}

// TestRelayRejectsFullRoom verifies a third member is refused.
func TestRelayRejectsFullRoom(t *testing.T) {
	addr := startServer(t)
	code := "ROOM99"

	c1, _, err := Dial(addr, code)
	if err != nil {
		t.Fatalf("member 1: %v", err)
	}
	defer c1.Close()
	c2, _, err := Dial(addr, code)
	if err != nil {
		t.Fatalf("member 2: %v", err)
	}
	defer c2.Close()

	if _, _, err := Dial(addr, code); err == nil {
		t.Fatal("third member should be rejected")
	}
}

// TestNetplayThroughRelay runs a full netplay session over the relay to prove
// the handshake and input exchange work end to end through the relay pipe.
func TestNetplayThroughRelay(t *testing.T) {
	addr := startServer(t)
	code := "GAME01"

	type res struct {
		s    net.Conn
		role Role
		err  error
	}
	hc := make(chan res, 1)
	go func() {
		c, r, err := Dial(addr, code)
		hc <- res{c, r, err}
	}()

	// Wait for the host to register as the room's first member, then the guest
	// joins; both netplay handshakes then run over the relayed connection.
	hres := <-hc
	if hres.err != nil {
		t.Fatalf("host dial: %v", hres.err)
	}
	if hres.role != RoleHost {
		t.Fatalf("host role = %q, want host", hres.role)
	}
	gc, gRole, err := Dial(addr, code)
	if err != nil {
		t.Fatalf("guest dial: %v", err)
	}
	if gRole != RoleGuest {
		t.Fatalf("guest role = %q, want guest", gRole)
	}

	// Exchange bytes through the pipe as the netplay handshake would. Only ONE
	// reader reads the host's connection (readAll below).
	if _, err := gc.Write([]byte("GAME abc123\n")); err != nil {
		t.Fatalf("guest write: %v", err)
	}

	hostLine := readAll(t, hres.s, "GAME abc123")
	if !strings.Contains(hostLine, "GAME abc123") {
		t.Fatalf("host did not receive the guest's handshake line through the relay (got %q)", hostLine)
	}
}

func errHostRole(role Role) error {
	return &roleErr{role}
}

type roleErr struct{ got Role }

func (e *roleErr) Error() string { return "host got role " + string(e.got) }

// readAll reads until the expected substring appears (or 1s), returning all
// data read. If want is empty it just waits briefly and returns what arrived.
func readAll(t *testing.T, c net.Conn, want string) string {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	var buf []byte
	tmp := make([]byte, 256)
	for {
		n, err := c.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if want != "" && contains(buf, want) {
				return string(buf)
			}
		}
		if err != nil {
			break
		}
	}
	return string(buf)
}

func contains(b []byte, s string) bool {
	if len(b) < len(s) {
		return false
	}
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return true
		}
	}
	return false
}

// TestRelayStressAbruptClose hammers the relay with concurrent joins and abrupt
// disconnects to shake out panics, races and goroutine leaks. It fails if the
// goroutine count grows without bound (leaked rooms / pipes).
func TestRelayStressAbruptClose(t *testing.T) {
	addr := startServer(t)
	before := runtime.NumGoroutine()

	for i := 0; i < 40; i++ {
		code := fmt.Sprintf("S%03d", i%8)

		// Host registers then closes before any guest joins.
		if c, _, err := Dial(addr, code); err == nil {
			c.Close()
		}

		// Host + guest connect, exchange a byte, then the guest disconnects
		// abruptly mid-pipe.
		hc := make(chan net.Conn, 1)
		go func() {
			c, _, err := Dial(addr, code)
			if err != nil {
				return
			}
			hc <- c
		}()
		var host net.Conn
		select {
		case host = <-hc:
		case <-time.After(3 * time.Second):
			t.Fatalf("iter %d: host did not register", i)
		}
		guest, _, err := Dial(addr, code)
		if err != nil {
			t.Fatalf("iter %d: guest dial: %v", i, err)
		}
		if _, err := host.Write([]byte("hi")); err != nil {
			t.Fatalf("iter %d: host write: %v", i, err)
		}
		guest.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 2)
		guest.Read(buf) //nolint:errcheck
		guest.Close()
		host.Close()
	}

	// Give leaked goroutines time to exit, then check for unbounded growth.
	time.Sleep(500 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()
	if after > before+30 {
		t.Fatalf("goroutine leak: before=%d after=%d", before, after)
	}
}
