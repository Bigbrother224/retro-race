// swift-tools-version: 6.0
// The swift-tools-version declares the minimum version of Swift required to build this package.

import PackageDescription

let package = Package(
    name: "RetroRace",
    platforms: [.macOS(.v14)],
    targets: [
        .target(
            name: "CRetroRace",
            path: "Sources/CRetroRace",
            publicHeadersPath: "include"
        ),
        .target(
            name: "RetroRaceCore",
            dependencies: ["CRetroRace"],
            path: "Sources/RetroRaceCore"
        ),
        .executableTarget(
            name: "RetroRaceCLI",
            dependencies: ["RetroRaceCore"],
            path: "Sources/RetroRaceCLI"
        ),
        .executableTarget(
            name: "RetroRaceApp",
            dependencies: ["RetroRaceCore"],
            path: "Sources/RetroRaceApp"
        )
    ]
)