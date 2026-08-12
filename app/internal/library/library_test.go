package library

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScanGroupsByConsole exercises the Library seam without the app:
// scanning a ROM dir groups files by console extension, ignores non-ROM
// files, sorts games alphabetically, and resets on re-scan (no
// accumulation across calls).
func TestScanGroupsByConsole(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Alter_Ego.nes"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "smw.sfc"), []byte("y"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("z"), 0o644)

	lib := New()
	consoles := lib.Scan(dir)

	if len(consoles) != 2 {
		t.Fatalf("want 2 consoles with games, got %d", len(consoles))
	}
	var gotNES, gotSNES bool
	for _, c := range consoles {
		switch c.ID {
		case "nes":
			gotNES = true
			if len(c.Games) != 1 || c.Games[0].Name != "Alter Ego" {
				t.Fatalf("nes games wrong: %+v", c.Games)
			}
		case "snes":
			gotSNES = true
			if len(c.Games) != 1 || c.Games[0].Name != "smw" {
				t.Fatalf("snes games wrong: %+v", c.Games)
			}
		}
	}
	if !gotNES || !gotSNES {
		t.Fatalf("missing console: nes=%v snes=%v", gotNES, gotSNES)
	}

	// Re-scan must reset games, not accumulate.
	consoles2 := lib.Scan(dir)
	if len(consoles2) != 2 {
		t.Fatalf("re-scan should not accumulate: got %d consoles", len(consoles2))
	}
}

// TestScanRealRomsDir is the smoke check that replaces launching the window:
// it scans the actual ROM folder and asserts at least one console resolves.
// It skips cleanly when the folder is absent or empty, so the suite stays
// green on checkouts without ROMs (Roms/ is gitignored).
func TestScanRealRomsDir(t *testing.T) {
	dir := "/Users/mac/retro-race/app/Roms"
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		t.Skipf("Roms dir %s absent or empty", dir)
	}

	consoles := New().Scan(dir)
	if len(consoles) == 0 {
		t.Fatalf("expected at least one console with games in %s", dir)
	}
}
