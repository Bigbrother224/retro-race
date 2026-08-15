// Command simulate runs a real, end-to-end two-player netplay session with
// scripted "bot" players, using the actual relay, netcode and rollback — one
// process per player (the libretro shim is single-instance per process).
//
// Default mode (no --side) is the DRIVER: it starts an in-process relay,
// spawns the host and guest sides as separate processes, runs them, and
// compares their final game-state hashes to prove both machines stay in exact
// sync. Each side also saves framebuffer screenshots so you can see the game.
//
// Usage:
//
//	go build -o /tmp/sim ./cmd/simulate
//	/tmp/sim --rom <rom> --core <core> --frames 900 [--out /tmp/retrorace-sim]
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"retrorace/internal/engine"
	"retrorace/internal/netplay"
	"retrorace/internal/relay"
	"retrorace/internal/rollback"
)

var (
	rom     = flag.String("rom", "", "ROM path (required)")
	core    = flag.String("core", "", "core dylib path (required)")
	frames  = flag.Int("frames", 900, "frames to simulate")
	out     = flag.String("out", "/tmp/retrorace-sim", "directory for screenshots")
	every   = flag.Int("every", 90, "save a screenshot every N frames")
	side    = flag.String("side", "", "run one side (host|guest); empty = driver")
	relayAd = flag.String("relay", "", "relay address (side mode)")
	code    = flag.String("code", "", "room code (side mode)")
	latMS   = flag.Int("latency", 0, "artificial RTT in ms per side (exercise rollback)")
)

func main() {
	flag.Parse()
	if *rom == "" || *core == "" {
		log.Fatalf("--rom and --core are required (the ROM is user-owned content; point the tool at it explicitly)")
	}
	if *side == "" {
		driver()
		return
	}
	if err := runSide(*side); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL side %s: %v\n", *side, err)
		os.Exit(1)
	}
}

// driver starts a relay and two side processes, then compares their states.
func driver() {
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatalf("out: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("relay listen: %v", err)
	}
	go func() { _ = relay.Serve(ln) }()
	code := relay.GenerateCode()
	addr := ln.Addr().String()
	self, _ := os.Executable()

	fmt.Printf("E2E sim: %s, %d frames, relay %s, code %s\n", *rom, *frames, addr, code)

	states := map[string]string{}
	cors := map[string]string{}
	type sideRun struct {
		name    string
		cmd     *exec.Cmd
		out     *bytes.Buffer
		started time.Time
	}
	cmds := []*sideRun{}
	for _, s := range []string{"host", "guest"} {
		args := []string{"--side", s, "--rom", *rom, "--core", *core,
			"--frames", fmt.Sprint(*frames), "--every", fmt.Sprint(*every),
			"--relay", addr, "--code", code, "--out", *out,
			"--latency", fmt.Sprint(*latMS)}
		cmd := exec.Command(self, args...)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			log.Fatalf("side %s start: %v", s, err)
		}
		cmds = append(cmds, &sideRun{s, cmd, &buf, time.Now()})
	}
	for _, r := range cmds {
		if err := r.cmd.Wait(); err != nil {
			log.Fatalf("side %s failed: %v", r.name, err)
		}
		state, corr, framesN := parseSide(r.out.String())
		states[r.name] = state
		cors[r.name] = corr
		fmt.Printf("[%s] done in %s, %d frames, rollback corrections=%s\n", r.name, time.Since(r.started).Round(time.Millisecond), framesN, corr)
	}

	fmt.Println("--- sync proof ---")
	fmt.Printf("host  state = %s\n", states["host"])
	fmt.Printf("guest state = %s\n", states["guest"])
	sync := states["host"] != "" && states["host"] == states["guest"]
	fmt.Printf("IDENTICAL = %v\n", sync)
	if !sync {
		os.Exit(1)
	}
}

// parseSide extracts STATE/CORR/FRAMES lines from a side's stdout.
func parseSide(stdout string) (state, corr string, framesN int) {
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "STATE ") {
			state = strings.TrimSpace(strings.TrimPrefix(line, "STATE "))
		}
		if strings.HasPrefix(line, "CORR ") {
			corr = strings.TrimSpace(strings.TrimPrefix(line, "CORR "))
		}
		if strings.HasPrefix(line, "FRAMES ") {
			fmt.Sscanf(strings.TrimPrefix(line, "FRAMES "), "%d", &framesN)
		}
	}
	return
}

// runSide runs one player process: load the core, connect via relay, and play
// with a bot through the real netcode + rollback.
func runSide(side string) error {
	c := engine.NewCore()
	if err := c.Start(*rom, *core); err != nil {
		return fmt.Errorf("core: %w", err)
	}
	defer c.Stop()

	gameID := "sim-" + hash(*rom) + "-" + hash(*core)
	sess, err := netplay.Relay(*relayAd, *code, gameID)
	if err != nil {
		return fmt.Errorf("relay: %w", err)
	}
	defer sess.Close()

	rb := rollback.New(rollbackCore{c}, 4)
	sess.SetPredict(true, 2)
	sess.SetAutoDelay(true, time.Second/60)
	if *latMS > 0 {
		sess.SetLatency(time.Duration(*latMS) * time.Millisecond)
	}

	bot := hostBot
	if side == "guest" {
		bot = guestBot
	}
	dir := filepath.Join(*out, side)
	_ = os.MkdirAll(dir, 0o755)

	rendered := 0
	corrections := 0
	sendCount := 0
mainLoop:
	for rendered < *frames {
		local := bot(sendCount)
		sendCount++
		if !sess.Send(local) {
			// If the peer finished and we are (almost) at the target, this is a
			// clean end of the simulation (teardown race), not a real failure.
			if rendered >= *frames-2 {
				break mainLoop
			}
			return fmt.Errorf("send at %d/%d: %w", rendered, *frames, sess.Err())
		}
		for {
			my, peer, advanced, ok := sess.RenderNext()
			if !ok {
				// If the peer finished and we are (almost) at the target, this
				// is a clean end of the simulation, not a real failure.
				if rendered >= *frames-2 {
					break mainLoop
				}
				return fmt.Errorf("render at %d/%d: %w", rendered, *frames, sess.Err())
			}
			if !advanced {
				break
			}
			var p1, p2 [12]bool
			if sess.LocalPort() == 1 {
				p1, p2 = my, peer
			} else {
				p1, p2 = peer, my
			}
			rb.Commit(rollback.Input{P1: pack(p1), P2: pack(p2)})
			for {
				fr, real, ok := sess.TakeCorrection()
				if !ok {
					break
				}
				if err := applyCorrection(rb, int(fr), real, sess.LocalPort()); err != nil {
					return fmt.Errorf("rollback: %w", err)
				}
				corrections++
			}
			rendered++
			if rendered%*every == 0 {
				savePNG(filepath.Join(dir, fmt.Sprintf("frame-%05d.png", rendered)), c)
			}
			if rendered >= *frames {
				break mainLoop
			}
		}
		// Real players run at 60 fps. When simulating latency, pace at frame
		// time (16.6 ms) so the renderer advances faster than the artificially
		// delayed peer inputs, exercising prediction + rollback.
		if *latMS > 0 {
			time.Sleep(time.Second / 60)
		}
	}

	// Rendezvous at the target frame: keep the session open (sending idle and
	// draining, without committing) so the peer can reach the same frame count,
	// then both sides exit together. This makes the sync proof deterministic
	// instead of depending on a teardown race. Only needed when simulating
	// latency (with 0 latency both sides finish together deterministically).
	// Safety cap: ~360 frames (~6 s).
	hold := 0
	for *latMS > 0 && hold < 360 {
		if !sess.Send([12]bool{}) {
			break // peer closed; we've reached the target
		}
		for {
			_, _, adv, ok := sess.RenderNext()
			if !ok {
				hold = 360
				break
			}
			if !adv {
				break
			}
			for {
				if _, _, ok := sess.TakeCorrection(); !ok {
					break
				}
			}
			hold++
		}
		if hold >= 360 {
			break
		}
		time.Sleep(time.Second / 60)
	}

	h := c.StateHash()
	fmt.Printf("STATE %x\n", h)
	fmt.Printf("CORR %d\n", corrections)
	fmt.Printf("FRAMES %d\n", rendered)
	return nil
}

func applyCorrection(rb *rollback.Session, frame int, realPeer uint16, localPort int) error {
	last := rb.Last()
	var ins []rollback.Input
	for f := frame; f <= last; f++ {
		in, ok := rb.Input(f)
		if !ok {
			return fmt.Errorf("frame %d out of window", f)
		}
		if f == frame {
			if localPort == 1 {
				in.P2 = realPeer // peer is player 2
			} else {
				in.P1 = realPeer // peer is player 1
			}
		}
		ins = append(ins, in)
	}
	return rb.Correct(frame, ins)
}

func pack(b [12]bool) uint16 {
	var m uint16
	for i := range b {
		if b[i] {
			m |= 1 << i
		}
	}
	return m
}

// rollbackCore adapts *engine.Core (typed SetButton) to rollback.Core.
type rollbackCore struct{ e *engine.Core }

func (r rollbackCore) SetButton(port, id int, pressed bool) {
	r.e.SetButtonPort(port, engine.JoyButton(id), pressed)
}
func (r rollbackCore) Step()                  { r.e.Step() }
func (r rollbackCore) Save() ([]byte, error)  { return r.e.Save() }
func (r rollbackCore) Restore(b []byte) error { return r.e.Restore(b) }

// hostBot is player 1: presses Start to enter, then moves right and jumps.
func hostBot(f int) [12]bool {
	var b [12]bool
	if f >= 60 && f < 66 {
		b[engine.BtnStart] = true
		return b
	}
	b[engine.BtnRight] = true
	if f%90 < 14 {
		b[engine.BtnA] = true
	}
	return b
}

// guestBot is player 2: sends a distinct controller-2 input pattern.
func guestBot(f int) [12]bool {
	var b [12]bool
	if f%130 < 18 {
		b[engine.BtnLeft] = true
	}
	if f%60 < 8 {
		b[engine.BtnB] = true
	}
	return b
}

func savePNG(path string, c *engine.Core) {
	w, h := c.Width(), c.Height()
	fb := c.Frame()
	if w <= 0 || h <= 0 || len(fb) < w*h*4 {
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			img.Set(x, y, color.RGBA{fb[i], fb[i+1], fb[i+2], 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	_ = png.Encode(f, img)
}

func hash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "?"
	}
	var h uint64 = 14695981039346656037
	for _, b := range data {
		h ^= uint64(b)
		h *= 1099511628211
	}
	return fmt.Sprintf("%x", h)[:8]
}
