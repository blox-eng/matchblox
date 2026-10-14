import SwiftUI

@main
struct MatchbloxApp: App {
    var body: some Scene {
        WindowGroup {
            ZStack {
                Color(red: 0x0F / 255, green: 0x0D / 255, blue: 0x0A / 255)
                    .ignoresSafeArea()
                Text("matchblox")
                    .foregroundStyle(.white)
            }
        }
    }
}
