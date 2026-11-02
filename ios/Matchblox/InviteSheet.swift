import SwiftUI
import UIKit

/// What the demo ends in: a phone cannot run the install, so the invite lets the person pass the
/// line on, or pair a real host from here.
struct InviteSheet: View {
    static let command = "curl -fsSL https://matchblox.sh | sh"
    static let startLink = "https://docs.matchblox.com/start/"

    let onPair: () -> Void
    let onKeepLooking: () -> Void

    @State private var copied = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                Text("Your agents wait the same way.")
                    .font(.title2.bold())
                    .accessibilityAddTraits(.isHeader)
                // Wraps at any size and is never cut off: it is a command to get right.
                Text(Self.command)
                    .font(.body)
                    .textSelection(.enabled)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(16)
                    .background(Theme.wash, in: RoundedRectangle(cornerRadius: 12))
                    .accessibilityIdentifier("invite-command")
                VStack(alignment: .leading, spacing: 12) {
                    Button(action: copy) {
                        Text(copied ? "Copied" : "Copy")
                            .frame(maxWidth: .infinity, minHeight: 44)
                    }
                    .buttonStyle(.bordered)
                    ShareLink(item: "\(Self.command)\n\(Self.startLink)") {
                        Text("Send to my computer")
                            .frame(maxWidth: .infinity, minHeight: 44)
                    }
                    .buttonStyle(.bordered)
                    Button(action: onPair) {
                        Text("Pair a host")
                            .frame(maxWidth: .infinity, minHeight: 44)
                    }
                    .buttonStyle(.borderedProminent)
                    .foregroundStyle(Theme.bg)
                    Button(action: onKeepLooking) {
                        Text("Keep looking")
                            .frame(maxWidth: .infinity, minHeight: 44)
                    }
                    .foregroundStyle(Theme.muted)
                }
            }
            .padding(24)
        }
        .scrollBounceBehavior(.basedOnSize)
        .foregroundStyle(Theme.fg)
        .fontDesign(.monospaced)
        .tint(Theme.accent)
        .presentationBackground(Theme.bg)
        .presentationDetents([.medium, .large])
        .task(id: copied) {
            guard copied else { return }
            try? await Task.sleep(for: .seconds(2))
            copied = false
        }
    }

    private func copy() {
        UIPasteboard.general.string = Self.command
        copied = true
        AccessibilityNotification.Announcement("Copied").post()
    }
}
