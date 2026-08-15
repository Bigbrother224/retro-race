package library

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Console is one supported system with its games.
type Console struct {
	ID    string // "nes", "snes", ...
	Name  string // "NES / Famicom", ...
	Core  string // relative dylib path under the cores dir
	Exts  []string
	Games []Game
	Emoji string
}

// Game is a locally-owned ROM identified by hashes.
type Game struct {
	Name    string // display name (no extension)
	Path    string
	SHA256  string
	Size    int64
	Ext     string
	Console string // console id
}

// Library is the console registry for a run, populated from a ROM directory.
type Library struct {
	Consoles []Console
}

// New returns a Library with the supported console registry. Core paths are
// relative to the cores directory.
func New() *Library {
	return &Library{Consoles: []Console{
		{ID: "nes", Name: "NES / Famicom", Core: "libretro-fceumm/fceumm_libretro.dylib", Exts: []string{"nes", "fds"}, Emoji: "🕹️"},
		{ID: "snes", Name: "Super Nintendo / SNES", Core: "snes9x_libretro.dylib", Exts: []string{"sfc", "smc", "snes"}, Emoji: "🎮"},
		{ID: "gb", Name: "Game Boy / Color", Core: "gambatte_libretro.dylib", Exts: []string{"gb", "gbc"}, Emoji: "📟"},
		{ID: "genesis", Name: "Genesis / Mega Drive", Core: "genesis_plus_gx_libretro.dylib", Exts: []string{"gen", "md"}, Emoji: "⚡"},
	}}
}

// Scan scans a ROM directory and returns the consoles that have matching
// games, sorted games-first (alphabetical). It mutates only l, never any
// package-level state.
func (l *Library) Scan(romsDir string) []Console {
	entries, err := os.ReadDir(romsDir)
	if err != nil {
		return nil
	}

	extToConsole := map[string]*Console{}
	for i := range l.Consoles {
		for _, e := range l.Consoles[i].Exts {
			extToConsole[e] = &l.Consoles[i]
		}
	}

	// Reset games.
	for i := range l.Consoles {
		l.Consoles[i].Games = nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(entry.Name()), "."))
		console, ok := extToConsole[ext]
		if !ok {
			continue
		}
		path := filepath.Join(romsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		sha := sha256.Sum256(data)
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		console.Games = append(console.Games, Game{
			Name:    strings.ReplaceAll(name, "_", " "),
			Path:    path,
			SHA256:  hex.EncodeToString(sha[:]),
			Size:    int64(len(data)),
			Ext:     ext,
			Console: console.ID,
		})
	}

	// Sort consoles with games first, games alphabetically.
	nonEmpty := make([]Console, 0, len(l.Consoles))
	for i := range l.Consoles {
		if len(l.Consoles[i].Games) > 0 {
			sort.Slice(l.Consoles[i].Games, func(a, b int) bool {
				return l.Consoles[i].Games[a].Name < l.Consoles[i].Games[b].Name
			})
			nonEmpty = append(nonEmpty, l.Consoles[i])
		}
	}
	return nonEmpty
}
