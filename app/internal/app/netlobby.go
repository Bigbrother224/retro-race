package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"retrorace/internal/engine"
	"retrorace/internal/library"
	"retrorace/internal/netplay"
	"retrorace/internal/relay"
	"retrorace/internal/rollback"
)

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

// enterNetLobby opens the shared-game lobby for the selected game. The host is
// the default role and a fresh room code is generated.
func (a *App) enterNetLobby(con library.Console, g library.Game) {
	a.netSess = nil
	a.netRole = 1 // host by default
	a.netCode = relay.GenerateCode()
	a.netErr = ""
	a.netConnCh = nil
	a.netConnecting = false
	a.netPlayers = -1
	a.netRendered = 0
	a.netLastCheck = 0
	a.state = stateNetLobby
}

// updateNetLobby handles role choice, room-code entry (guest), and starting the
// (asynchronous) relay connection.
func (a *App) updateNetLobby() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.state = stateGame
		return
	}

	if a.netConnecting {
		select {
		case res := <-a.netConnCh:
			a.netConnecting = false
			if res.err != nil {
				a.netErr = res.err.Error()
			} else {
				a.netSess = res.sess
				a.startNetplayGame()
			}
		default:
		}
		return
	}

	// Toggle host/guest.
	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) || inpututil.IsKeyJustPressed(ebiten.KeyRight) {
		a.netRole = 3 - a.netRole
		if a.netRole == 1 {
			a.netCode = relay.GenerateCode() // a new room gets a fresh code
		} else {
			a.netCode = ""
		}
	}

	// Guest types the code the host gave them.
	if a.netRole == 2 {
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(a.netCode) > 0 {
			r := []rune(a.netCode)
			a.netCode = string(r[:len(r)-1])
		}
		for _, ch := range ebiten.InputChars() {
			ch = unicode.ToUpper(ch)
			if (ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9') && len(a.netCode) < 12 {
				a.netCode += string(ch)
			}
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		if len(a.netCode) < 4 {
			a.netErr = "Entre le code de la partie (4+ caracteres)"
			return
		}
		a.connectNetplay()
	}
}

// connectNetplay opens the relay connection asynchronously (the host waits for
// the guest to join the room, which can take a while).
func (a *App) connectNetplay() {
	con := a.consoles[a.selCon]
	g := con.Games[a.selGame]
	corePath := filepath.Join(coresDir, con.Core)
	gameID, err := gameIDFor(g.Path, corePath)
	if err != nil {
		a.netErr = err.Error()
		return
	}
	a.netErr = ""
	a.netConnecting = true
	a.netConnCh = make(chan connectResult, 1)
	go func() {
		sess, err := netplay.Relay(RelayAddr, a.netCode, gameID)
		a.netConnCh <- connectResult{sess, err}
	}()
}

type connectResult struct {
	sess *netplay.Session
	err  error
}

// startNetplayGame launches the local core once both players are connected and
// enters the shared-game play state.
func (a *App) startNetplayGame() {
	con := a.consoles[a.selCon]
	g := con.Games[a.selGame]
	corePath := filepath.Join(coresDir, con.Core)
	core := engine.NewCore()
	if err := core.Start(g.Path, corePath); err != nil {
		a.netErr = err.Error()
		if a.netSess != nil {
			a.netSess.Close()
			a.netSess = nil
		}
		return
	}
	a.emu = core
	a.netSess.SetAutoDelay(true, time.Second/60)
	a.netRb = nil
	if _, err := core.Save(); err == nil {
		// Only enable rollback if the core truly supports save states; a core
		// that does not would fail every correction. The window must hold the
		// frame before the oldest correctable frame (predictMax+2).
		a.netRb = rollback.New(rollbackCore{core}, int(netPredictMax)+2)
		a.netSess.SetPredict(true, netPredictMax)
	}
	a.netPlayers = core.ControllerPlayers()
	a.netKnown = false
	if n, ok := knownPlayerCount(g.Name); ok {
		a.netPlayers = n
		a.netKnown = true
	}
	a.running = true
	a.paused = false
	a.state = statePlaying
	a.arbiter.Reset()
}

// stopNetplay ends a shared game and returns to the game list.
func (a *App) stopNetplay() {
	if a.netSess != nil {
		a.netSess.Close()
		a.netSess = nil
	}
	if a.emu != nil {
		a.emu.Stop()
		a.emu = nil
	}
	a.netRb = nil
	a.running = false
	a.state = stateGame
}

// updateNetplayPlaying drives the shared-game loop: exchange inputs, apply the
// merged ports, step the core, and watch for divergence.
func (a *App) updateNetplayPlaying() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.stopNetplay()
		return
	}
	local := netReadLocalButtons()
	if !a.netSess.Send(local) {
		a.netErr = "Connexion perdue : " + a.netSess.Err().Error()
		a.stopNetplay()
		return
	}
	rendered := 0
	for {
		my, peer, advanced, ok := a.netSess.RenderNext()
		if !ok {
			a.netErr = "Connexion perdue : " + a.netSess.Err().Error()
			a.stopNetplay()
			return
		}
		if !advanced {
			break
		}
		var p1, p2 [netplay.ButtonCount]bool
		if a.netSess.LocalPort() == 1 {
			p1, p2 = my, peer
		} else {
			p1, p2 = peer, my
		}
		if a.netRb != nil {
			// rollback.Commit applies the inputs, steps the core once and
			// records the state. The app must NOT step separately here or the
			// core runs at 2x speed.
			a.netRb.Commit(mask16(p1, p2))
		} else {
			for b := 0; b < netplay.ButtonCount; b++ {
				a.emu.SetButtonPort(0, engine.JoyButton(b), p1[b])
				a.emu.SetButtonPort(1, engine.JoyButton(b), p2[b])
			}
			a.emu.Step()
		}
		a.netRendered++
		rendered++
		if rendered >= 4 {
			break
		}
	}
	// Apply any predictions that turned out wrong (rollback re-simulation).
	for a.netRb != nil {
		frame, realPeer, ok := a.netSess.TakeCorrection()
		if !ok {
			break
		}
		if !a.applyCorrection(int(frame), realPeer, a.netSess.LocalPort()) {
			return
		}
	}
	if a.netCheckDivergence() {
		a.stopNetplay()
	}
}

// applyCorrection re-simulates from the frame where the peer's real input
// differed from what was predicted. Returns false if the rollback fails (the
// session is stopped with an error).
func (a *App) applyCorrection(frame int, realPeer uint16, localPort int) bool {
	last := a.netRb.Last()
	var ins []rollback.Input
	for f := frame; f <= last; f++ {
		in, ok := a.netRb.Input(f)
		if !ok {
			a.netErr = "rollback: frame hors fenetre"
			a.stopNetplay()
			return false
		}
		if f == frame {
			// realPeer is the peer's input; put it in the peer's slot (P1 when
			// we are player 2, P2 when we are player 1).
			if localPort == 1 {
				in.P2 = realPeer
			} else {
				in.P1 = realPeer
			}
		}
		ins = append(ins, in)
	}
	if err := a.netRb.Correct(frame, ins); err != nil {
		a.netErr = "rollback: " + err.Error()
		a.stopNetplay()
		return false
	}
	a.netCorrections++
	return true
}

// netCheckDivergence periodically hashes this machine's game state and compares
// with the peer, ending the session on divergence. Mirrors NetplayApp.
func (a *App) netCheckDivergence() bool {
	if a.netRendered < a.netLastCheck+netplayCheckEvery {
		return false
	}
	h, ok := a.emu.(stateHasher)
	if !ok {
		return false
	}
	myHash := h.StateHash()
	if myHash == 0 {
		return false
	}
	a.netLastCheck = a.netRendered
	a.netSess.SendStateHash(myHash)

	myFrame := a.netSess.RenderFrame()
	if pf, ph, have := a.netSess.PeerHash(); have {
		if pf >= myFrame && pf-myFrame <= a.netSess.InputDelay()+8 {
			if ph != myHash {
				a.netErr = "DIVERGENCE : les deux machines ne sont plus synchronisees"
				return true
			}
		}
	}
	return false
}

// drawNetLobby renders the shared-game lobby.
func (a *App) drawNetLobby(screen *ebiten.Image) {
	drawBG(screen)
	con := a.consoles[a.selCon]
	g := con.Games[a.selGame]

	psTextC(screen, "PARTIE PARTAGEE", 480, 60, 20, colAccent2)
	psTextC(screen, g.Name, 480, 96, 12, colTextDim)

	// Role cards.
	roles := []string{"HOTE  (cree)", "INVITE  (rejoint)"}
	for i, label := range roles {
		x := 240 + i*240
		sel := a.netRole == i+1
		fill, border := colPanel, colBorder
		if sel {
			fill, border = colPanelHi, colAccent2
		}
		drawPanel(screen, x, 170, 200, 70, fill, border)
		tc := colTextDim
		if sel {
			tc = colAccent2
		}
		psTextC(screen, label, float64(x)+100, 200, 13, tc)
	}
	_ = roles

	// Code display / entry.
	psTextC(screen, "CODE DE LA PARTIE", 480, 300, 12, colTextDim)
	code := a.netCode
	if (a.frame/30)%2 == 0 && a.netRole == 2 && !a.netConnecting {
		code += "|"
	}
	psTextC(screen, code, 480, 336, 30, colText)
	fillRect(screen, 320, 366, 320, 2, colBorder)

	if a.netConnecting {
		msg := "En attente du partenaire... (code " + a.netCode + ")"
		if a.netRole == 2 {
			msg = "Connexion au code " + a.netCode + "..."
		}
		psTextC(screen, msg, 480, 430, 12, colAccent2)
		psTextC(screen, "Relais : "+RelayAddr, 480, 456, 9, colTextDim)
	} else {
		hint := "← / →  ROLE   ·   ENTER  CONNECT"
		if a.netRole == 2 {
			hint = "TAPE LE CODE   ·   ENTER  CONNECT"
		}
		psTextC(screen, hint, 480, 430, 10, colTextDim)
		psTextC(screen, "Relais : "+RelayAddr, 480, 456, 9, colTextDim)
	}

	if a.netErr != "" {
		psTextC(screen, strings.ToUpper(a.netErr), 480, 500, 10, colAccent)
	}

	drawFooter(screen, "ESC  BACK", "", "")
}

// drawNetplayHUD renders the shared-game overlay during play: role, room code,
// multiplayer capability and network telemetry.
func (a *App) drawNetplayHUD(screen *ebiten.Image) {
	role := "JOUEUR 1  (HOTE)"
	cc := colPlayer
	code := a.netCode
	if a.netSess.LocalPort() == 2 {
		role = "JOUEUR 2  (INVITE)"
		cc = colOpponent
	}
	psText(screen, role, 16, 14, 12, cc)
	if code != "" {
		psText(screen, "CODE "+code, 16, 40, 9, colTextDim)
	}
	switch {
	case a.netKnown && a.netPlayers >= 2:
		psText(screen, "2P confirme (jeu connu)", 16, 62, 9, colTextDim)
	case a.netKnown && a.netPlayers == 1:
		psText(screen, "Jeu solo : pas de 2e personnage", 16, 62, 9, colAccent2)
	case a.netPlayers >= 2:
		psText(screen, fmt.Sprintf("%d manettes (materiel) — le 2e perso agit ?", a.netPlayers), 16, 62, 9, colTextDim)
	case a.netPlayers == 1:
		psText(screen, "Jeu solo (probable) : pas de 2e personnage", 16, 62, 9, colAccent2)
	default:
		psText(screen, "Compat 2 joueurs : inconnue", 16, 62, 9, colTextDim)
	}
	if st := a.netSess.Stats(); st.Rendered > 0 {
		rb := ""
		if a.netRb != nil {
			rb = fmt.Sprintf("   rollback %d", a.netCorrections)
		}
		psText(screen, fmt.Sprintf("frames %d   buffer %d   stalls %d   delay %d   ping %d ms%s",
			st.Rendered, st.Buffered, st.Stalled, a.netSess.InputDelay(), a.netSess.RTT().Milliseconds(), rb), 16, 86, 9, colTextDim)
	}
	psText(screen, "ESC  QUITTER", 16, float64(screen.Bounds().Dy())-24, 9, colTextDim)
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
