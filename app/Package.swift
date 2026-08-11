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
        .executableTarget(
            name: "RetroRace",
            dependencies: ["CRetroRace"],
            path: "Sources/RetroRace"
        )
    ]
)
