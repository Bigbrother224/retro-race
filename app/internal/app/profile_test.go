package app

import (
	"path/filepath"
	"testing"
)

// TestProfileRoundTrip verifies the username survives a save/load cycle, and
// that a missing profile yields an empty username.
func TestProfileRoundTrip(t *testing.T) {
	orig := profilePath
	profilePath = filepath.Join(t.TempDir(), "profile.json")
	t.Cleanup(func() { profilePath = orig })

	if got := loadProfile(); got != "" {
		t.Fatalf("empty profile should load as empty, got %q", got)
	}

	saveProfile("MAX")
	if got := loadProfile(); got != "MAX" {
		t.Fatalf("loaded username = %q, want MAX", got)
	}
}

// TestProfileMissingFileIsEmpty guards that a nonexistent profile is not an
// error and yields an empty username.
func TestProfileMissingFileIsEmpty(t *testing.T) {
	orig := profilePath
	profilePath = filepath.Join(t.TempDir(), "does-not-exist.json")
	t.Cleanup(func() { profilePath = orig })

	if got := loadProfile(); got != "" {
		t.Fatalf("missing profile should load as empty, got %q", got)
	}
}
