// Package aiagent drives a game with an AI that plays by watching the screen,
// the way a human does: each decision it sends the current framebuffer to a
// vision model and receives the controller buttons to press. Two agents with
// different strategies ("win" plays well, "lose" deliberately plays badly) can
// be run to simulate two games with a real winner and loser.
package aiagent

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"retrorace/internal/engine"
)

// Buttons is the controller state an agent decides to press.
type Buttons struct {
	Up, Down, Left, Right bool
	A, B, X, Y            bool
	Start, Select, L, R   bool
}

// Visioner decides what to press given a game screen. imgPNG is a small
// downsampled PNG of the current frame.
type Visioner interface {
	Decide(imgPNG []byte, strategy string) (Buttons, error)
}

// Config drives one agent run.
type Config struct {
	ROM, Core       string
	Strategy        string // "win" or "lose"
	Vision          Visioner
	DecisionEvery   int // frames between AI decisions (held in between)
	MaxFrames       int
	OutDir          string // optional: save screenshots here
	ScreenshotEvery int
}

// Result reports what one agent did.
type Result struct {
	Frames      int
	StateHash   uint64
	Screenshots int
	LastErr     error // last vision-decision error, if any (non-fatal)
}

// Run plays one game for MaxFrames: every DecisionEvery frames it asks the
// vision model what to press and holds that input between decisions.
func Run(cfg Config) (Result, error) {
	core := engine.NewCore()
	if err := core.Start(cfg.ROM, cfg.Core); err != nil {
		return Result{}, fmt.Errorf("core: %w", err)
	}
	defer core.Stop()

	if cfg.DecisionEvery <= 0 {
		cfg.DecisionEvery = 1
	}
	if cfg.MaxFrames <= 0 {
		cfg.MaxFrames = 600
	}
	if cfg.OutDir != "" {
		_ = os.MkdirAll(cfg.OutDir, 0o755)
	}

	var held Buttons
	var lastErr error
	screenshots := 0
	// Press Start at frame 0 so the game (Contra) begins.
	held.Start = true

	for frame := 0; frame < cfg.MaxFrames; frame++ {
		if frame > 2 {
			held.Start = false
		}
		if cfg.DecisionEvery > 0 && frame%cfg.DecisionEvery == 0 {
			fb := core.Frame()
			img := downsample(fb, core.Width(), core.Height(), 160, 144)
			b, err := cfg.Vision.Decide(img, cfg.Strategy)
			if err != nil {
				lastErr = err
			} else {
				held = b
			}
		}
		apply(core, held)
		core.Step()

		if cfg.OutDir != "" && cfg.ScreenshotEvery > 0 && frame%cfg.ScreenshotEvery == 0 {
			saveShot(filepath.Join(cfg.OutDir, fmt.Sprintf("frame-%05d.png", frame)), core)
			screenshots++
		}
	}

	return Result{Frames: cfg.MaxFrames, StateHash: core.StateHash(), Screenshots: screenshots, LastErr: lastErr}, nil
}

// apply writes a Buttons state to controller port 0 (the agent's player).
func apply(core *engine.Core, b Buttons) {
	sets := []struct {
		btn engine.JoyButton
		on  bool
	}{
		{engine.BtnUp, b.Up}, {engine.BtnDown, b.Down},
		{engine.BtnLeft, b.Left}, {engine.BtnRight, b.Right},
		{engine.BtnA, b.A}, {engine.BtnB, b.B},
		{engine.BtnX, b.X}, {engine.BtnY, b.Y},
		{engine.BtnStart, b.Start}, {engine.BtnSelect, b.Select},
		{engine.BtnL, b.L}, {engine.BtnR, b.R},
	}
	for _, s := range sets {
		core.SetButtonPort(0, s.btn, s.on)
	}
}

// downsample box-averages an RGBA framebuffer into a small RGBA image and
// returns it as a PNG. Small images keep vision-API calls fast and cheap.
func downsample(fb []byte, w, h, dw, dh int) []byte {
	if w <= 0 || h <= 0 || len(fb) < w*h*4 {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		sy := y * h / dh
		ey := (y + 1) * h / dh
		if ey <= sy {
			ey = sy + 1
		}
		for x := 0; x < dw; x++ {
			sx := x * w / dw
			ex := (x + 1) * w / dw
			if ex <= sx {
				ex = sx + 1
			}
			var r, g, b int
			n := 0
			for yy := sy; yy < ey && yy < h; yy++ {
				for xx := sx; xx < ex && xx < w; xx++ {
					i := (yy*w + xx) * 4
					r += int(fb[i])
					g += int(fb[i+1])
					b += int(fb[i+2])
					n++
				}
			}
			if n > 0 {
				r /= n
				g /= n
				b /= n
			}
			dst.Set(x, y, color.RGBA{uint8(r), uint8(g), uint8(b), 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, dst)
	return buf.Bytes()
}

func saveShot(path string, core *engine.Core) {
	fb := core.Frame()
	w, h := core.Width(), core.Height()
	if w <= 0 || h <= 0 || len(fb) < w*h*4 {
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		o := i * 4
		img.Pix[o] = fb[o]
		img.Pix[o+1] = fb[o+1]
		img.Pix[o+2] = fb[o+2]
		img.Pix[o+3] = 255
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	_ = png.Encode(f, img)
}

// ---- OpenAI-compatible vision driver (xAI Grok, OpenAI, etc.) ----

// OpenAICompat calls any OpenAI-compatible /chat/completions endpoint with an
// image payload. It works with xAI Grok, OpenAI and Anthropic-compatible APIs.
type OpenAICompat struct {
	APIKey  string
	BaseURL string // e.g. https://api.x.ai/v1 (default)
	Model   string // e.g. grok-2-vision
	HTTP    *http.Client
}

// promptFor builds the instruction for a strategy.
func promptFor(strategy string) string {
	base := "You are playing the NES game Contra as player 1 (the red commando). " +
		"This is the current screen. Decide the controller buttons to press RIGHT NOW. " +
		"Reply ONLY with a compact JSON object using exactly these keys: " +
		`{"up":false,"down":false,"left":false,"right":true,"a":true,"b":false,"x":false,"y":false,"start":false,"select":false,"l":false,"r":false}. ` +
		"Never add text, only JSON."
	switch strings.ToLower(strategy) {
	case "lose":
		return base + " Your goal: LOSE on purpose. Move into danger, walk into enemies and bullets, " +
			"fall into pits, never avoid hazards. Play badly and die."
	default: // "win"
		return base + " Your goal: WIN. Play well — advance to the right, shoot enemies, jump over and " +
			"avoid obstacles and bullets, stay alive and progress through the level."
	}
}

// Decide sends the screen to the vision model and parses the JSON buttons.
func (o *OpenAICompat) Decide(imgPNG []byte, strategy string) (Buttons, error) {
	if len(imgPNG) == 0 {
		return Buttons{}, fmt.Errorf("empty image")
	}
	if o.APIKey == "" {
		return Buttons{}, fmt.Errorf("AIBAI_API_KEY not set")
	}
	base := o.BaseURL
	if base == "" {
		base = "https://api.x.ai/v1"
	}
	model := o.Model
	if model == "" {
		model = "grok-2-vision"
	}
	hc := o.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}

	b64 := base64.StdEncoding.EncodeToString(imgPNG)
	payload := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": "You output ONLY valid JSON for a game controller. No commentary."},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": promptFor(strategy)},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + b64}},
			}},
		},
		"response_format": map[string]any{"type": "json_object"},
		"temperature":     0,
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return Buttons{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	resp, err := hc.Do(req)
	if err != nil {
		return Buttons{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return Buttons{}, fmt.Errorf("vision api %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return Buttons{}, fmt.Errorf("parse response: %w", err)
	}
	if len(out.Choices) == 0 {
		return Buttons{}, fmt.Errorf("no choices in response")
	}
	return parseButtons(out.Choices[0].Message.Content)
}

func parseButtons(s string) (Buttons, error) {
	// Strip code fences if present.
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	var b Buttons
	if err := json.Unmarshal([]byte(s), &b); err != nil {
		return Buttons{}, fmt.Errorf("parse buttons %q: %w", s, err)
	}
	return b, nil
}
