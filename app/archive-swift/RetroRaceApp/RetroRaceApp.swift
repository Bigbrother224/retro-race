import AppKit
import SwiftUI

@main
struct RetroRaceApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var appDelegate

    var body: some Scene {
        WindowGroup {
            LibraryView()
                .frame(minWidth: 640, minHeight: 400)
        }
    }
}

/// Forces the bare SwiftPM executable to behave as a regular GUI app so the
/// window actually appears and comes to the front.
final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.regular)
        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }
}