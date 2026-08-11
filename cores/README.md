# Cores

Core libretro compiled locally as `.dylib` (not committed, see `.gitignore`).

- `libretro-fceumm/` — NES core (FCEUmm), source clone. Build with:
  `make -j$(sysctl -n hw.ncpu)` → produces `fceumm_libretro.dylib`
