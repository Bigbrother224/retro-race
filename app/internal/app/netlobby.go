package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"retrorace/internal/engine"
	"retrorace/internal/netplay"
	"retrorace/internal/rollback"
)

// netplayCheckEvery is how often (frames) the shared-game session hashes and
// compares game state with the peer (~1 s at 60 fps).
const netplayCheckEvery = 60

// stateHasher is implemented by emulators that can hash their game state for
// netplay divergence detection (the real libretro Core; FakeCore does not).
type stateHasher interface {
	StateHash() uint64
}

// fileSHA256 hashes a file's bytes, used to prove both sides run the same ROM
// and core during the netplay handshake (no content is ever transferred).
func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// knownPlayers maps normalized game names to their ACTUAL 2-player capability.
// This is a small, user-extensible Game Profile table: it answers "is this ROM
// really 2P?" for known games — something the core's controller-info cannot
// (that reports hardware capacity, not the ROM's real second character).
// Unknown games fall back to the hardware report / "unknown".
var knownPlayers = map[string]int{
	"supermariobros":  2,
	"contra":          2,
	"alterego":        1,
	"supermarioworld": 1,
}

// normalizeName reduces a game name to a stable key (lowercase alphanumeric).
func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// knownPlayerCount returns the real 2-player capability for a known game, and
// whether the game is known at all.
func knownPlayerCount(gameName string) (int, bool) {
	n, ok := knownPlayers[normalizeName(gameName)]
	return n, ok
}

// netPredictMax bounds how many frames ahead the peer input is predicted before
// stalling; the rollback window is predictMax+1 so a late real input is always
// correctable within it.
const netPredictMax = 2

// mask16 packs a two-player button state into two bitmasks for rollback.
func mask16(p1, p2 [netplay.ButtonCount]bool) rollback.Input {
	var in rollback.Input
	for i := range p1 {
		if p1[i] {
			in.P1 |= 1 << i
		}
		if p2[i] {
			in.P2 |= 1 << i
		}
	}
	return in
}

// rollbackCore adapts the real libretro core to the rollback.Core interface
// (which wants SetButton(port,id,pressed)); *engine.Core uses a typed button.
// Without this adapter the rollback was silently disabled (the type assertion
// on *engine.Core never matched).
type rollbackCore struct{ e *engine.Core }

func (r rollbackCore) SetButton(port, id int, pressed bool) {
	r.e.SetButtonPort(port, engine.JoyButton(id), pressed)
}
func (r rollbackCore) Step()                  { r.e.Step() }
func (r rollbackCore) Save() ([]byte, error)  { return r.e.Save() }
func (r rollbackCore) Restore(b []byte) error { return r.e.Restore(b) }

// connectResult carries the outcome of an asynchronous relay connection.
type connectResult struct {
	sess *netplay.Session
	err  error
}

// netReadLocalButtons reads this machine's controller state (keyboard + all
// standard gamepads) as the input this player contributes to the shared game.
func netReadLocalButtons() [12]bool {
	var st [12]bool
	for key, btn := range keyboardButtons {
		if inpututil.IsKeyJustPressed(key) || ebiten.IsKeyPressed(key) {
			st[btn] = true
		}
	}
	for _, id := range ebiten.AppendGamepadIDs(nil) {
		readGamepadButtons(id, &st)
	}
	return st
}

// gameIDFor hashes the ROM and core bytes to prove both players run the same
// content during the netplay handshake (nothing is transferred).
func gameIDFor(romPath, corePath string) (string, error) {
	romID, err := fileSHA256(romPath)
	if err != nil {
		return "", err
	}
	coreID, err := fileSHA256(corePath)
	if err != nil {
		return "", err
	}
	return romID + coreID, nil
}
