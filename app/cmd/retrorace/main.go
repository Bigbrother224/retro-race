package main

import (
	"flag"
	"log"
	"time"

	"retrorace/internal/app"
	"retrorace/internal/rival"
)

func main() {
	// Headless rival mode: `retrorace --rival --rom X --core Y --shm Z`.
	// Runs a second emulator process that publishes to shared memory.
	rivalMode := flag.Bool("rival", false, "run as a headless rival process")
	rivalROM := flag.String("rom", "", "ROM path for the rival")
	rivalCore := flag.String("core", "", "core dylib path for the rival")
	rivalShm := flag.String("shm", "", "shared-memory region path for the rival")
	rivalRun := flag.String("run", "", "recorded run (JSON) for the rival to replay")
	rivalExpected := flag.Int("expected", 3600, "estimated segment length in frames")

	// Netplay shared-game mode: `--host :PORT` hosts, `--join HOST:PORT` joins.
	netHost := flag.String("host", "", "netplay: listen address when hosting (e.g. :9330)")
	netJoin := flag.String("join", "", "netplay: host:port to join when not hosting")
	netROM := flag.String("netrom", "/Users/mac/retro-race/app/Roms/Contra (USA).nes", "netplay: ROM path (both sides have it locally)")
	netCore := flag.String("netcore", "/Users/mac/retro-race/cores/libretro-fceumm/fceumm_libretro.dylib", "netplay: core dylib path")
	netGame := flag.String("netgame", "", "netplay: game name for the HUD")
	netRelay := flag.String("relay", "", "netplay: input relay server (default local dev relay)")
	netCode := flag.String("code", "", "netplay: room code (with --relay)")
	fakeOpp := flag.Bool("fakeopp", false, "play vs a local fake-opponent bot (player 1)")
	netBot := flag.Bool("netbot", false, "run as the headless fake-opponent bot (player 2)")
	netJoinAddr := flag.String("netjoin", "", "netbot: host address to join")
	netGameID := flag.String("gameid", "", "netbot: game id (ROM+core hash) to join")
	fakeLat := flag.Int("fake-latency", 0, "fake-opponent: artificial RTT in ms on the session")
	flag.Parse()

	// Relay address for the shared-game lobby (defaults to the local dev relay).
	if *netRelay != "" {
		app.RelayAddr = *netRelay
	}

	if *rivalMode {
		if *rivalROM == "" || *rivalCore == "" || *rivalShm == "" {
			log.Fatal("--rival requires --rom, --core and --shm")
		}
		if _, err := rival.Run(rival.Config{
			ROM:            *rivalROM,
			Core:           *rivalCore,
			ShmPath:        *rivalShm,
			RunPath:        *rivalRun,
			ExpectedFrames: *rivalExpected,
		}); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *netBot {
		if *netROM == "" || *netCore == "" || *netJoinAddr == "" || *netGameID == "" {
			log.Fatal("--netbot requires --netrom, --netcore, --netjoin and --gameid")
		}
		if err := app.RunNetBot(app.NetBotConfig{
			ROM:     *netROM,
			Core:    *netCore,
			Join:    *netJoinAddr,
			GameID:  *netGameID,
			ShmPath: *rivalShm,
			Latency: time.Duration(*fakeLat) * time.Millisecond,
		}); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *fakeOpp {
		if *netROM == "" || *netCore == "" {
			log.Fatal("--fakeopp requires --netrom and --netcore")
		}
		if err := app.RunFakeOpponent(app.NetplayConfig{
			ROM:  *netROM,
			Core: *netCore,
			Game: *netGame,
		}, time.Duration(*fakeLat)*time.Millisecond); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *netHost != "" || *netJoin != "" || *netRelay != "" {
		if *netROM == "" || *netCore == "" {
			log.Fatal("--host/--join/--relay require --netrom and --netcore")
		}
		if *netRelay != "" && *netCode == "" {
			log.Fatal("--relay requires --code")
		}
		if err := app.RunNetplay(app.NetplayConfig{
			Host:  *netHost,
			Join:  *netJoin,
			Relay: *netRelay,
			Code:  *netCode,
			ROM:   *netROM,
			Core:  *netCore,
			Game:  *netGame,
		}); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
