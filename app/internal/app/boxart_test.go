package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBoxartPath pins the local boxart resolution seam: an image present
// under Boxart/<consoleID>/<gameName>.png resolves to that path, a missing
// image falls back to a placeholder, and the console scoping is respected.
func TestBoxartPath(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "nes"), 0o755)
	os.WriteFile(filepath.Join(dir, "nes", "Alter Ego.png"), []byte("x"), 0o644)

	p, ok := boxartPath(dir, "nes", "Alter Ego")
	if !ok || filepath.Base(p) != "Alter Ego.png" {
		t.Fatalf("existing boxart not resolved: path=%q ok=%v", p, ok)
	}

	if _, ok := boxartPath(dir, "nes", "Missing Game"); ok {
		t.Fatal("missing image should fall back to placeholder")
	}

	if _, ok := boxartPath(dir, "snes", "Alter Ego"); ok {
		t.Fatal("image in another console must not be reused")
	}
}

// TestWrapLines guards the placeholder text wrapping.
func TestWrapLines(t *testing.T) {
	got := wrapLines("Super Mario World", 14)
	want := "Super Mario Wo\nrld"
	if got != want {
		t.Fatalf("wrapLines: got %q, want %q", got, want)
	}
	if wrapLines("Short", 14) != "Short" {
		t.Fatal("short string should not wrap")
	}
}
