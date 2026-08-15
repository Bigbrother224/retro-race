#!/usr/bin/env bash
# Builds a distributable Retro Race.app bundle for macOS (Apple Silicon).
#
# The bundled app resolves its cores from the bundle's Resources/cores and its
# user content (ROMs, boxart, replays, profile) from
# ~/Library/Application Support/RetroRace. The relay server binary is bundled
# too, so either player can host it.
#
# Usage: scripts/build-app.sh [OUTPUT_DIR]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/dist/RetroRace.app}"
GO=go

echo ">> Preparing clean output"
rm -rf "$OUT"

echo ">> Building retrorace + relay"
cd "$ROOT/app"
mkdir -p "$OUT/Contents/MacOS"
$GO build -o "$OUT/Contents/MacOS/retrorace" ./cmd/retrorace
$GO build -o "$OUT/Contents/MacOS/relay" ./cmd/relay

echo ">> Assembling bundle at $OUT"
mkdir -p "$OUT/Contents/Resources/cores/libretro-fceumm"
mkdir -p "$OUT/Contents/Resources/fonts"
mkdir -p "$OUT/Contents/Resources/consoles"

# Bundle the cores as the library registry expects them. Copy only the dylibs
# (libretro-fceumm is a git clone — never copy its .git).
cp "$ROOT/cores/libretro-fceumm/fceumm_libretro.dylib" "$OUT/Contents/Resources/cores/libretro-fceumm/"
cp "$ROOT/cores/snes9x_libretro.dylib" "$OUT/Contents/Resources/cores/"

# Bundle the fonts and console photos (resolved at runtime via paths.go).
cp -R "$ROOT/app/Assets/fonts/." "$OUT/Contents/Resources/fonts/"
cp -R "$ROOT/app/Assets/consoles/." "$OUT/Contents/Resources/consoles/"

cat > "$OUT/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>Retro Race</string>
	<key>CFBundleDisplayName</key>
	<string>Retro Race</string>
	<key>CFBundleIdentifier</key>
	<string>com.retrorace.app</string>
	<key>CFBundleExecutable</key>
	<string>retrorace</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>0.1.0</string>
	<key>CFBundleVersion</key>
	<string>1</string>
	<key>LSMinimumSystemVersion</key>
	<string>13.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSPrincipalClass</key>
	<string>NSApplication</string>
	<key>LSApplicationCategoryType</key>
	<string>public.app-category.games</string>
</dict>
</plist>
PLIST

# Gatekeeper: ad-hoc sign so the app launches on the friend's Mac without
# triggering a hard block. Notarization is a separate, later step.
codesign --force --deep --sign - "$OUT" 2>/dev/null || true

echo ">> Done: $OUT"
du -sh "$OUT"
