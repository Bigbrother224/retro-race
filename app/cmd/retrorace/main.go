package main

import (
	"flag"
	"log"

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
	flag.Parse()

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

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
