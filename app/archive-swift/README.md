# Archive — ancienne implémentation Swift

Cette implémentation (targets SwiftUI/CLI/Core) a été remplacée par Go + Ebitengine
(voir `../README.md`). Conservée pour référence historique.

- `RetroRaceApp` — launcher SwiftUI
- `RetroRaceCLI` — commandes headless (spike, calibrate, ghost, shm-peek…)
- `RetroRaceCore` — moteur : shim, calibration, RaceSession
- `Package.swift` — ancien manifeste SwiftPM

Le shim C (`Sources/CRetroRace`) n'est PAS archivé : il est réutilisé tel quel
via cgo dans l'implémentation Go.
