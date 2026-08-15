package app

import (
	"os"
	"path/filepath"
	"strings"
)

// Paths are resolved once at startup so the same binary works in development
// and as a distributed .app bundle:
//
//   - Inside a macOS bundle (executable path contains /Contents/MacOS/), the
//     bundled cores live in the app's Resources/cores, while user content
//     (ROMs, boxart, replays, profile) lives in ~/Library/Application
//     Support/RetroRace — writable by the user.
//   - Outside a bundle (development), the repository's local folders are used.
//   - Every path can be overridden with a RETRO_RACE_* environment variable.
var (
	coresDir    string
	romsDir     string
	boxartDir   string
	replaysDir  string
	profilesDir string
	profilePath string
	fontsDir    string
	consolesDir string
	savesDir    string
)

func init() {
	resources := bundleResources()
	if resources != "" {
		coresDir = filepath.Join(resources, "cores")
		fontsDir = filepath.Join(resources, "fonts")
		consolesDir = filepath.Join(resources, "consoles")
		user := userDataDir()
		romsDir = filepath.Join(user, "Roms")
		boxartDir = filepath.Join(user, "Boxart")
		replaysDir = filepath.Join(user, "Replays")
		profilesDir = filepath.Join(user, "Profiles")
		profilePath = filepath.Join(user, "profile.json")
	} else if root := repoRoot(); root != "" {
		// Dev mode inside a cloned repository: resolve against the repo root so
		// the build runs from any clone without baking in an author's home path.
		coresDir = filepath.Join(root, "cores")
		fontsDir = filepath.Join(root, "app", "Assets", "fonts")
		consolesDir = filepath.Join(root, "app", "Assets", "consoles")
		romsDir = filepath.Join(root, "app", "Roms")
		boxartDir = filepath.Join(root, "app", "Boxart")
		replaysDir = filepath.Join(root, "app", "Replays")
		profilesDir = filepath.Join(root, "app", "Profiles")
		profilePath = filepath.Join(root, "app", "profile.json")
	} else {
		// No bundle and no repository layout on disk: fall back to a temp dir so
		// the app still starts (with an empty library). Set RETRO_RACE_* env vars
		// or run from the repository to point at real content.
		tmp := os.TempDir()
		coresDir = filepath.Join(tmp, "RetroRace", "cores")
		fontsDir = filepath.Join(tmp, "RetroRace", "fonts")
		consolesDir = filepath.Join(tmp, "RetroRace", "consoles")
		romsDir = filepath.Join(tmp, "RetroRace", "Roms")
		boxartDir = filepath.Join(tmp, "RetroRace", "Boxart")
		replaysDir = filepath.Join(tmp, "RetroRace", "Replays")
		profilesDir = filepath.Join(tmp, "RetroRace", "Profiles")
		profilePath = filepath.Join(tmp, "RetroRace", "profile.json")
	}
	override("RETRO_RACE_CORES", &coresDir)
	override("RETRO_RACE_ROMS", &romsDir)
	override("RETRO_RACE_BOXART", &boxartDir)
	override("RETRO_RACE_REPLAYS", &replaysDir)
	override("RETRO_RACE_PROFILES", &profilesDir)
	override("RETRO_RACE_PROFILE", &profilePath)
	override("RETRO_RACE_FONTS", &fontsDir)
	override("RETRO_RACE_CONSOLES", &consolesDir)
	override("RETRO_RACE_SAVES", &savesDir)

	// Saves are user data and must persist across runs, so they always live in
	// the user data directory (never /tmp, never the repo).
	savesDir = filepath.Join(userDataDir(), "Saves")
	if err := os.MkdirAll(savesDir, 0o755); err != nil {
		savesDir = os.TempDir()
	}
}

// bundleResources returns the Contents/Resources directory when running inside
// a macOS .app bundle, or "" otherwise.
func bundleResources() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if i := strings.Index(exe, "/Contents/MacOS/"); i >= 0 {
		return exe[:i] + "/Contents/Resources"
	}
	return ""
}

func userDataDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "Library", "Application Support", "RetroRace")
	}
	return filepath.Join(os.TempDir(), "RetroRace")
}

// repoRoot finds the repository root by walking up from the current working
// directory looking for the Retro Race layout (a `cores` dir and an `app` dir
// side by side). This lets a dev build run from any clone without baking an
// author's home path in. Returns "" when not found.
func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if isDir(filepath.Join(dir, "cores")) && isDir(filepath.Join(dir, "app")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func override(key string, dst *string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}
