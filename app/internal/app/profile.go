package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// profilePath is where the player's profile (currently just the username) is
// persisted between launches. It is a var so tests can redirect it.
var profilePath = "/Users/mac/retro-race/app/profile.json"

// profile is the persisted player preference.
type profile struct {
	Username string `json:"username"`
}

// loadProfile reads the persisted username, or "" if none has been saved.
func loadProfile() string {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return ""
	}
	var p profile
	if err := json.Unmarshal(data, &p); err != nil {
		return ""
	}
	return p.Username
}

// saveProfile writes the username so it survives restarts. It never fails
// loudly: persistence is best-effort (a read-only home should not block play).
func saveProfile(username string) {
	if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(profile{Username: username}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(profilePath, data, 0o644)
}
