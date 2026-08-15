package app

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"retrorace/internal/replay"
)

// Local race state machine and rendering. The opponent plays the same segment
// the player is playing (the product's parallel-race concept), so its view and
// replay are the player's own game a few frames behind — no second emulator
// instance is required.

// RaceConfig tunes the local race.
type RaceConfig struct {
	ExpectedFrames int // gauge scale: estimated segment length in frames
	ReplaySeconds  int // how many seconds of replay to keep per side
	ReplayStride   int // store every Nth frame in the replay ring
	SlowmoSeconds  int // slow-motion duration before the replay
}

// DefaultRaceConfig returns sensible defaults for a ~1-minute segment.
func DefaultRaceConfig() RaceConfig {
	return RaceConfig{
		ExpectedFrames: 60 * 60, // ~60 s
		ReplaySeconds:  5,
		ReplayStride:   4, // ~15 fps replay
		SlowmoSeconds:  1,
	}
}

// frameRing stores the last max sampled frames (each a copy), preallocated so
// the draw loop does not allocate per frame. Every stride-th Add is kept.
type frameRing struct {
	frames [][]byte
	stride int
	max    int
	count  int
	head   int
	n      int
}

func newFrameRing(frameSize, max, stride int) *frameRing {
	r := &frameRing{stride: stride, max: max}
	r.frames = make([][]byte, max)
	for i := range r.frames {
		r.frames[i] = make([]byte, frameSize)
	}
	return r
}

// Add stores a copy of frame when the sampling interval says so.
func (r *frameRing) Add(frame []byte) {
	if r.stride <= 1 || r.n%r.stride == 0 {
		copy(r.frames[r.head], frame)
		r.head = (r.head + 1) % r.max
		if r.count < r.max {
			r.count++
		}
	}
	r.n++
}

// Len returns how many frames are stored.
func (r *frameRing) Len() int { return r.count }

// Frame returns the i-th stored frame, oldest first (i in [0, Len())).
func (r *frameRing) Frame(i int) []byte {
	if r.count == 0 || i < 0 || i >= r.count {
		return nil
	}
	return r.frames[(r.head-r.count+i+r.max)%r.max]
}

// size returns the per-frame buffer size, or 0 before the first Add.
func (r *frameRing) size() int {
	if r.count == 0 {
		return 0
	}
	return len(r.frames[0])
}

type raceState int

const (
	racePlaying raceState = iota
	raceSlowmo
	raceReplay
	raceDone
)

// race holds the pure local-race state machine: two progress trackers, the
// replay ring buffers, the winner, and the playing→slowmo→replay→done flow.
type race struct {
	cfg RaceConfig

	state raceState
	frame int

	playerFinish int // -1 until finished
	oppFinish    int // -1 until the rival really finishes
	winner       int // 0 none, 1 player, 2 opponent

	// oppProgressOverride is the rival's real gauge progress (0..1) published
	// over shared memory; -1 until the first real value arrives.
	oppProgressOverride float64

	playerFrameSize int // set on the first player frame
	oppFrameSize    int // set when the opponent instance is created
	replayPlayer    *frameRing
	replayOpp       *frameRing

	slowStart   int
	replayStart int
	replayTick  int
	rec         *replay.Recorder // input recorder (lazy, created on first RecordInput)
}

func newRace(cfg RaceConfig) *race {
	return &race{cfg: cfg, state: racePlaying, playerFinish: -1, oppFinish: -1, oppProgressOverride: -1}
}

// ensureRings lazily sizes both replay rings once both frame sizes are known.
func (r *race) ensureRings() {
	if r.replayPlayer != nil || r.playerFrameSize <= 0 || r.oppFrameSize <= 0 {
		return
	}
	maxFrames := r.cfg.ReplaySeconds*60/r.cfg.ReplayStride + 1
	r.replayPlayer = newFrameRing(r.playerFrameSize, maxFrames, r.cfg.ReplayStride)
	r.replayOpp = newFrameRing(r.oppFrameSize, maxFrames, r.cfg.ReplayStride)
}

// AddPlayerFrame records the player's frame into the replay ring.
func (r *race) AddPlayerFrame(frame []byte) {
	if r.playerFrameSize == 0 && len(frame) > 0 {
		r.playerFrameSize = len(frame)
	}
	r.ensureRings()
	if r.replayPlayer != nil && len(frame) == r.playerFrameSize {
		r.replayPlayer.Add(frame)
	}
}

// AddOppFrame records the opponent's frame into its replay ring. The rival
// plays the same segment as the player, so its frames are the same size.
func (r *race) AddOppFrame(frame []byte) {
	if r.oppFrameSize == 0 && len(frame) > 0 {
		r.oppFrameSize = len(frame)
	}
	r.ensureRings()
	if r.replayOpp != nil && len(frame) == r.oppFrameSize {
		r.replayOpp.Add(frame)
	}
}

// PlayerFinished records the player's segment end (from the Race Arbiter). It
// is a no-op outside the playing phase or if already recorded. The winner is
// whoever finished first; a tie goes to the player.
func (r *race) PlayerFinished() {
	if r.state != racePlaying || r.playerFinish >= 0 {
		return
	}
	r.playerFinish = r.frame
	r.winner = 1
	if r.oppFinish >= 0 && r.oppFinish < r.playerFinish {
		r.winner = 2
	}
	r.slowStart = r.frame
	r.state = raceSlowmo
}

// OppFinished records the rival's real finish (from its own arbiter via shared
// memory). It is a no-op if the player already finished (the race is over).
func (r *race) OppFinished() {
	if r.state != racePlaying {
		return
	}
	if r.oppFinish < 0 {
		r.oppFinish = r.frame
	}
	if r.playerFinish >= 0 {
		return // race already settled by the player
	}
	r.winner = 2
	r.slowStart = r.frame
	r.state = raceSlowmo
}

// SetOppProgress overrides the opponent's gauge progress with the real value
// published by the rival process.
func (r *race) SetOppProgress(p float64) {
	r.oppProgressOverride = p
}

// PlayerProgress returns the player's gauge progress in [0,1]. Progress is a
// design choice documented in the product: elapsed time over the expected
// segment length (the real rival publishes its own progress over shared
// memory). The winner is settled by whichever side's arbiter detects the
// segment end first.
func (r *race) PlayerProgress() float64 {
	if r.playerFinish >= 0 {
		return 1
	}
	return math.Min(1, float64(r.frame)/float64(r.cfg.ExpectedFrames))
}

// OppProgress is the opponent's gauge progress in [0,1]. It uses the real
// value published by the rival process when available.
func (r *race) OppProgress() float64 {
	if r.oppFinish >= 0 {
		return 1
	}
	if r.oppProgressOverride >= 0 {
		return math.Min(1, r.oppProgressOverride)
	}
	return math.Min(1, float64(r.frame)/float64(r.cfg.ExpectedFrames))
}

// Tick advances the race state machine one update frame. Callers should step
// the emulators according to the returned state.
func (r *race) Tick() raceState {
	r.frame++
	switch r.state {
	case racePlaying:
		// The rival's real finish is delivered via OppFinished (shared memory);
		// no scripted timer here.
	case raceSlowmo:
		if r.frame >= r.slowStart+r.cfg.SlowmoSeconds*60 {
			r.state = raceReplay
			r.replayStart = r.frame
			r.replayTick = 0
		}
	case raceReplay:
		r.replayTick++
		if total := r.replayLen() * r.cfg.ReplayStride; r.replayTick >= total {
			r.state = raceDone
		}
	}
	return r.state
}

func (r *race) replayLen() int {
	if r.replayPlayer == nil {
		return 0
	}
	return r.replayPlayer.Len()
}

// State returns the current race phase.
func (r *race) State() raceState { return r.state }

// Frame returns the current race frame counter.
func (r *race) Frame() int { return r.frame }

// Winner returns 1 (player) or 2 (opponent); 0 before a finish.
func (r *race) Winner() int { return r.winner }

// WinnerFinishFrames returns the winner's finish time in frames.
func (r *race) WinnerFinishFrames() int {
	if r.winner == 2 {
		return r.oppFinish
	}
	return r.playerFinish
}

// ReplayIndex returns the current replay frame index (0-based), clamped.
func (r *race) ReplayIndex() int {
	if r.replayLen() == 0 {
		return 0
	}
	i := r.replayTick / r.cfg.ReplayStride
	if i >= r.replayLen() {
		i = r.replayLen() - 1
	}
	return i
}

// RecordInput captures one frame's button state into the input recorder. It is
// a no-op outside the playing phase. The recorder is created lazily on the
// first call.
func (r *race) RecordInput(state []bool) {
	if r.state != racePlaying {
		return
	}
	if r.rec == nil {
		r.rec = replay.NewRecorder(len(state))
	}
	r.rec.Record(state)
}

// RunEvents returns the recorded input events, or nil if nothing was recorded.
func (r *race) RunEvents() []replay.InputEvent {
	if r.rec == nil {
		return nil
	}
	return r.rec.Events()
}

// InputDuration returns the number of frames recorded, or 0.
func (r *race) InputDuration() int {
	if r.rec == nil {
		return 0
	}
	return r.rec.Duration()
}

// sanitizeFilename replaces path-unsafe characters with underscores.
func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '/', '\\', ':', ' ':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "replay"
	}
	return b.String()
}

// ---- drawing (Ebitengine) ----

var (
	colPlayer   = color.RGBA{0xff, 0xd5, 0x3a, 0xff} // yellow (you)
	colOpponent = color.RGBA{0x2a, 0x7a, 0xd4, 0xff} // blue (rival)
)

// drawRacePanel draws the right-side race dashboard: a header with both
// players, the rival's live screen (large and labelled), and a clear progress
// gauge. It lives entirely in the reserved right column, so the game is never
// covered.
func (a *App) drawRacePanel(screen *ebiten.Image, r *race) {
	x := 960 - racePanelW
	w := racePanelW

	// Panel background so it reads as its own surface, not floating text.
	fillRect(screen, x, 0, w, 720, color.RGBA{0x0a, 0x0a, 0x10, 0xff})
	fillRect(screen, x, 0, 2, 720, color.RGBA{0xff, 0xff, 0xff, 0x0d})

	// Header: the race.
	psTextC(screen, "COURSE", float64(x)+float64(w)/2, 20, 12, colTextDim)
	fillRect(screen, x+16, 44, w-32, 2, colBorder)

	// Rival's live screen — the wow: you can see where the opponent is.
	a.drawRivalScreen(screen, float64(x), float64(w))

	// Progress gauge: clear and readable (two labelled bars).
	drawRaceGauge(screen, r, a.playerName(), float64(x), float64(w))

	// Joust gate status: only meaningful when two players are connected.
	if a.twoPlayers() {
		label := "JOUST : OFF   (J)"
		if a.joust {
			label = "JOUST : ON    (J)"
		}
		psTextC(screen, label, float64(x)+float64(w)/2, 646, 9, colTextDim)
	}

	// Exit hint at the bottom of the panel.
	psTextC(screen, "ESC  MENU", float64(x)+float64(w)/2, 700, 9, colTextDim)
}

// drawRivalScreen renders the opponent's live framebuffer as a large framed
// window inside the race panel.
func (a *App) drawRivalScreen(screen *ebiten.Image, px, pw float64) {
	if a.pipImg == nil {
		return
	}
	const viewW, viewH = 204, 152
	// Fit the rival frame into the view, letterboxed.
	iw, ih := float64(a.pipImg.Bounds().Dx()), float64(a.pipImg.Bounds().Dy())
	if iw == 0 || ih == 0 {
		return
	}
	scale := math.Min(viewW/iw, viewH/ih)
	dw, dh := iw*scale, ih*scale
	bx := px + (pw-viewW)/2
	by := 76.0

	// Frame + label above.
	fillRect(screen, int(bx)-2, int(by)-2, int(viewW)+4, int(viewH)+4, colOpponent)
	psTextC(screen, "RIVAL", bx+viewW/2, by-22, 10, colOpponent)

	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterNearest
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(bx+(viewW-dw)/2, by+(viewH-dh)/2)
	screen.DrawImage(a.pipImg, op)
}

// drawRaceGauge draws the progress comparison as two clear, labelled bars
// (you vs rival) inside the race panel.
func drawRaceGauge(screen *ebiten.Image, r *race, playerName string, px, pw float64) {
	const barW = 196.0
	const barH = 10.0
	x := px + (pw-barW)/2
	gy := 258.0

	// You (playerName).
	psText(screen, playerName, x, gy-16, 9, colPlayer)
	fillRect(screen, int(x), int(gy), int(barW), int(barH), color.RGBA{0xff, 0xff, 0xff, 0x14})
	if p := r.PlayerProgress(); p > 0 {
		fillRect(screen, int(x), int(gy), int(barW*p), int(barH), colPlayer)
	}

	// Rival (blue).
	psText(screen, "RIVAL", x, gy+barH+12, 9, colOpponent)
	fillRect(screen, int(x), int(gy+barH+28), int(barW), int(barH), color.RGBA{0xff, 0xff, 0xff, 0x14})
	if p := r.OppProgress(); p > 0 {
		fillRect(screen, int(x), int(gy+barH+28), int(barW*p), int(barH), colOpponent)
	}

	// Percentage labels on the right of each bar.
	psTextR(screen, fmt.Sprintf("%d%%", int(r.PlayerProgress()*100)), x+barW, gy-16, 9, colPlayer)
	psTextR(screen, fmt.Sprintf("%d%%", int(r.OppProgress()*100)), x+barW, gy+barH+12, 9, colOpponent)
}

// drawDramaticEnding renders the finish as a full-screen result: a clean
// winner banner, then the last seconds of both screens side by side, centered
// and fully visible.
func (a *App) drawDramaticEnding(screen *ebiten.Image, r *race) {
	drawBG(screen)

	player := a.playerName()
	name := player
	cc := colPlayer
	if r.Winner() == 2 {
		name = "RIVAL"
		cc = colOpponent
	}
	t := float64(r.WinnerFinishFrames()) / 60.0

	// Winner banner: centered, deliberate.
	psTextC(screen, name+" GAGNE", 480, 56, 30, cc)
	w := psWidth(name+" GAGNE", 30)
	fillRect(screen, int(480-w/2)-30, 56+30+14, int(w)+60, 2, cc)
	psTextC(screen, fmt.Sprintf("%.2f s", t), 480, 56+30+30, 12, colTextDim)

	// Caption above the screens.
	psTextC(screen, "LES DERNIERES SECONDES", 480, 150, 11, colTextDim)

	// Two replay screens, centered vertically (no cropping).
	a.renderReplaySide(screen, r, r.replayPlayer, 0, player, a.emu.Width(), a.emu.Height())
	a.renderReplaySide(screen, r, r.replayOpp, 1, "RIVAL", a.emu.Width(), a.emu.Height())

	// Actions.
	psTextC(screen, "R  REJOUER      E  EXPORTER      ESC  MENU", 480, 690, 10, colTextDim)
	if a.replayMsg != "" {
		psTextC(screen, a.replayMsg, 480, 672, 9, colAccent2)
	}
}

// renderReplaySide draws one side of the final replay in a clean framed panel.
func (a *App) renderReplaySide(screen *ebiten.Image, r *race, ring *frameRing, side int, label string, w, h int) {
	if ring == nil || w <= 0 || h <= 0 {
		return
	}
	img := a.replayImg(side, w, h)
	if f := ring.Frame(r.ReplayIndex()); f != nil {
		img.WritePixels(f)
	}
	const vw, vh = 430, 330
	const topY = 176.0
	baseX := 40
	if side == 1 {
		baseX = 490
	}
	cc := colPlayer
	if side == 1 {
		cc = colOpponent
	}
	psTextC(screen, label, float64(baseX)+vw/2, topY-24, 11, cc)

	iw, ih := float64(w), float64(h)
	scale := math.Min(float64(vw)/iw, float64(vh)/ih)
	dw, dh := iw*scale, ih*scale
	bx := float64(baseX) + (float64(vw)-dw)/2
	by := topY + (float64(vh)-dh)/2

	// Thin frame.
	fillRect(screen, int(bx)-2, int(by)-2, int(dw)+4, int(dh)+4, color.RGBA{0xff, 0xff, 0xff, 0x18})

	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterNearest
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(bx, by)
	screen.DrawImage(img, op)
}

// replayImg returns (creating on first use) the reused replay image for a side.
func (a *App) replayImg(side, w, h int) *ebiten.Image {
	if a.replayImgs[side] == nil {
		a.replayImgs[side] = ebiten.NewImage(w, h)
		return a.replayImgs[side]
	}
	if a.replayImgs[side].Bounds().Dx() != w || a.replayImgs[side].Bounds().Dy() != h {
		a.replayImgs[side] = ebiten.NewImage(w, h)
	}
	return a.replayImgs[side]
}
