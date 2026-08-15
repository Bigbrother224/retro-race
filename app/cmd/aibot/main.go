// Command aibot plays a game with AI agents that watch the screen like a human
// and decide controller inputs via a vision model. Two strategies ("win" and
// "lose") can be run to simulate two games with a real winner and loser.
//
// Default mode (no --side) is the DRIVER: it spawns a "win" agent and a "lose"
// agent as separate processes (the libretro shim is single-instance per
// process), runs both, and reports each one's progression so you can see who
// won and who lost.
//
// The vision brain is an OpenAI-compatible API (xAI Grok, OpenAI, ...). Set:
//
//	export AIBAI_API_KEY=sk-...
//	export AIBAI_MODEL=grok-2-vision
//	export AIBAI_BASE_URL=https://api.x.ai/v1
//
// Usage:
//
//	go build -o /tmp/aibot ./cmd/aibot
//	/tmp/aibot --rom <rom> --core <core> --out /tmp/aibots
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"retrorace/internal/aiagent"
)

var (
	rom     = flag.String("rom", "/Users/mac/retro-race/app/Roms/Contra (USA).nes", "ROM path")
	core    = flag.String("core", "/Users/mac/retro-race/cores/libretro-fceumm/fceumm_libretro.dylib", "core dylib")
	side    = flag.String("side", "", "run one agent (win|lose); empty = driver")
	frames  = flag.Int("frames", 1200, "frames per agent game")
	every   = flag.Int("every", 30, "frames between AI decisions (held in between)")
	out     = flag.String("out", "/tmp/aibots", "directory for screenshots")
	snaps   = flag.Int("snapshot", 90, "save a screenshot every N frames")
	model   = flag.String("model", "", "vision model (default AIBAI_MODEL or grok-2-vision)")
	baseURL = flag.String("base", "", "vision API base (default AIBAI_BASE_URL or https://api.x.ai/v1)")
)

func vision() aiagent.Visioner {
	key := os.Getenv("AIBAI_API_KEY")
	return &aiagent.OpenAICompat{
		APIKey:  key,
		BaseURL: firstNonEmpty(*baseURL, os.Getenv("AIBAI_BASE_URL"), "https://api.x.ai/v1"),
		Model:   firstNonEmpty(*model, os.Getenv("AIBAI_MODEL"), "grok-2-vision"),
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func main() {
	flag.Parse()
	if os.Getenv("AIBAI_API_KEY") == "" {
		log.Fatal("set AIBAI_API_KEY (vision model key)")
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

func runSide(side string) error {
	start := time.Now()
	res, err := aiagent.Run(aiagent.Config{
		ROM:             *rom,
		Core:            *core,
		Strategy:        side,
		Vision:          vision(),
		DecisionEvery:   *every,
		MaxFrames:       *frames,
		OutDir:          fmt.Sprintf("%s/%s", *out, side),
		ScreenshotEvery: *snaps,
	})
	if err != nil {
		return err
	}
	line := fmt.Sprintf("RESULT side=%s frames=%d hash=%x screenshots=%d elapsed=%s", side, res.Frames, res.StateHash, res.Screenshots, time.Since(start).Round(time.Millisecond))
	if res.LastErr != nil {
		line += fmt.Sprintf(" lastDecisionErr=%v", res.LastErr)
	}
	fmt.Println(line)
	return nil
}

// driver spawns win and lose agents and reports the comparison.
func driver() {
	self, _ := os.Executable()
	_ = os.MkdirAll(*out, 0o755)
	type r struct {
		name string
		cmd  *exec.Cmd
		buf  bytes.Buffer
	}
	sides := []*r{}
	for _, s := range []string{"win", "lose"} {
		cmd := exec.Command(self, "--side", s, "--rom", *rom, "--core", *core,
			"--frames", fmt.Sprint(*frames), "--every", fmt.Sprint(*every),
			"--out", *out, "--snapshot", fmt.Sprint(*snaps),
			"--model", *model, "--base", *baseURL)
		cmd.Env = os.Environ()
		b := &r{name: s, cmd: cmd}
		cmd.Stdout = &b.buf
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			log.Fatalf("side %s start: %v", s, err)
		}
		sides = append(sides, b)
	}
	fmt.Printf("AI bot sim: 2 agents (win vs lose), %d frames each, vision model=%s\n", *frames, vision().(*aiagent.OpenAICompat).Model)
	for _, s := range sides {
		if err := s.cmd.Wait(); err != nil {
			log.Printf("side %s failed: %v", s.name, err)
		}
		for _, l := range strings.Split(s.buf.String(), "\n") {
			if strings.HasPrefix(l, "RESULT") {
				fmt.Println(l)
			}
		}
	}
	fmt.Printf("Screenshots in %s/{win,lose}/ — compare them to see who won.\n", *out)
}
