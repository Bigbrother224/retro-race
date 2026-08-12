import Foundation

/// Public entry point used by the headless CLI target. Keeps the Core's
/// internals (spike/calibrate/ghost/shm) hidden while exposing one entry.
public func runRetroRaceCLI(_ args: [String]) {
    let mode = args.count > 1 ? args[1] : "spike"

    switch mode {
    case "spike":
        runSpike()
    case "determinism":
        runDeterminism(Array(args.dropFirst(2)))
    case "calibrate":
        runCalibrate(Array(args.dropFirst(2)))
    case "calibrate-run":
        runCalibrateRun(Array(args.dropFirst(2)))
    case "ghost":
        runGhost(Array(args.dropFirst(2)))
    case "ghost-live":
        runGhostLive(Array(args.dropFirst(2)))
    case "shm-peek":
        runShmPeek(Array(args.dropFirst(2)))
    case "list":
        runList(Array(args.dropFirst(2)))
    case "race-headless":
        runRaceHeadless(Array(args.dropFirst(2)))
    case "overlay":
        runOverlay()
    default:
        print("""
        Usage:
          RetroRaceCLI spike                 determinism + framebuffer check
          RetroRaceCLI determinism [--frames N] [--interval N]
          RetroRaceCLI calibrate [--frames N]  spawn 2 children, diff their save states
          RetroRaceCLI calibrate-run --out PATH [--hold RIGHT --from 120] [--frames N]
          RetroRaceCLI ghost --out PATH        silent ghost: run frames, dump save state
          RetroRaceCLI ghost-live --shm NAME [--calibration PATH]  publish ghost position+sprite over shared memory
          RetroRaceCLI shm-peek --shm NAME     consumer: print frames from shared memory (headless check)
          RetroRaceCLI list [--dir PATH]        scan a folder, list detected games with hashes
          RetroRaceCLI race-headless           launcher RaceSession + ghost-live, headless check
          RetroRaceCLI overlay                 player window + transparent click-through ghost overlay
        """)
        exit(1)
    }
}