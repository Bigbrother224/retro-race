import SwiftUI

@main
struct RetroRaceApp: App {
    var body: some Scene {
        WindowGroup {
            LibraryView()
                .frame(minWidth: 640, minHeight: 400)
        }
    }
}