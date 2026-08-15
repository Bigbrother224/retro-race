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
	} else {
		coresDir = "/Users/mac/retro-race/cores"
		fontsDir = "/Users/mac/retro-race/app/Assets/fonts"
		consolesDir = "/Users/mac/retro-race/app/Assets/consoles"
		romsDir = "/Users/mac/retro-race/app/Roms"
		boxartDir = "/Users/mac/retro-race/app/Boxart"
		replaysDir = "/Users/mac/retro-race/app/Replays"
		profilesDir = "/Users/mac/retro-race/app/Profiles"
		profilePath = "/Users/mac/retro-race/app/profile.json"
	}
	override("RETRO_RACE_CORES", &coresDir)
	override("RETRO_RACE_ROMS", &romsDir)
	override("RETRO_RACE_BOXART", &boxartDir)
	override("RETRO_RACE_REPLAYS", &replaysDir)
	override("RETRO_RACE_PROFILES", &profilesDir)
	override("RETRO_RACE_PROFILE", &profilePath)
	override("RETRO_RACE_FONTS", &fontsDir)
	override("RETRO_RACE_CONSOLES", &consolesDir)
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

func override(key string, dst *string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}
