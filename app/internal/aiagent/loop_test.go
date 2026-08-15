package aiagent

import (
	"os"
	"path/filepath"
	"testing"
)

// mockVision returns a fixed, plausible Contra input so we can prove the game
// loop advances without needing a real vision model.
type mockVision struct{ f int }

func (m *mockVision) Decide(_ []byte, strategy string) (Buttons, error) {
	m.f++
	var b Buttons
	b.Right = true  // advance
	b.A = true      // shoot
	if strategy == "lose" && m.f%3 == 0 {
		b.Left = true // deliberately move toward danger
	}
	return b, nil
}

func TestLoopAdvances(t *testing.T) {
	rom, core := findGameFiles()
	if rom == "" || core == "" {
		t.Skip("no repo ROM/core found on this checkout; skipping real-game loop test")
	}
	dir := t.TempDir()
	res, err := Run(Config{
		ROM:             rom,
		Core:            core,
		Strategy:        "win",
		Vision:          &mockVision{},
		DecisionEvery:   30,
		MaxFrames:       600,
		OutDir:          dir,
		ScreenshotEvery: 90,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Frames != 600 {
		t.Fatalf("frames = %d, want 600", res.Frames)
	}
	if res.StateHash == 0 {
		t.Fatal("state hash is 0 (game did not advance)")
	}
	if res.Screenshots < 2 {
		t.Fatalf("screenshots = %d, want >= 2", res.Screenshots)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) == 0 {
		t.Fatal("no screenshots written")
	}
}

func TestParseButtons(t *testing.T) {
	b, err := parseButtons(`{"up":true,"down":false,"left":false,"right":true,"a":true,"b":false,"x":false,"y":false,"start":false,"select":false,"l":false,"r":false}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !b.Up || !b.Right || !b.A {
		t.Fatalf("want up+right+a, got %+v", b)
	}
	if b.Down || b.Left || b.B {
		t.Fatalf("unexpected buttons: %+v", b)
	}
	// tolerate code fences
	b, err = parseButtons("```json\n{\"right\":true}\n```")
	if err != nil {
		t.Fatalf("parse fenced: %v", err)
	}
	if !b.Right {
		t.Fatal("fenced json not parsed")
	}
}

// findGameFiles walks up from the test working directory to the repo root
// (a `cores` dir next to an `app` dir) and returns the default NES ROM + core
// for the real-game loop test, or ("","") when not present on this checkout.
func findGameFiles() (rom, core string) {
	dir, err := os.Getwd()
	if err != nil {
		return "", ""
	}
	for {
		if isDir(filepath.Join(dir, "cores")) && isDir(filepath.Join(dir, "app")) {
			rom = filepath.Join(dir, "app", "Roms", "Contra (USA).nes")
			core = filepath.Join(dir, "cores", "libretro-fceumm", "fceumm_libretro.dylib")
			if fileExists(rom) && fileExists(core) {
				return rom, core
			}
			return "", ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
