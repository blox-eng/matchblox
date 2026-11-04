import SwiftUI
import UIKit

/// The install line and the two ways to use it from a phone, which cannot run it: the sheet and the
/// demo's ended screen show the same copy.
enum Install {
    static let command = "curl -fsSL https://matchblox.sh | sh"
    static let startLink = "https://docs.matchblox.com/start/"
}

/// The line in a box, wrapped at any size and never cut off, with Copy inline.
struct InstallLine: View {
    @State private var copied = false

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(Install.command)
                .font(.body)
                .textSelection(.enabled)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityIdentifier("invite-command")
            Button(action: copy) {
                Text(copied ? "Copied" : "Copy")
                    .frame(minWidth: 44, minHeight: 44)
            }
            .buttonStyle(.bordered)
            .controlSize(.small)
        }
        .padding(16)
        .background(Theme.wash, in: RoundedRectangle(cornerRadius: 12))
        .task(id: copied) {
            guard copied else { return }
            try? await Task.sleep(for: .seconds(2))
            copied = false
        }
    }

    private func copy() {
        UIPasteboard.general.string = Install.command
        copied = true
        AccessibilityNotification.Announcement("Copied").post()
    }
}

/// The share sheet with the line and the get-started link: the way from a phone to a computer.
struct SendToComputerButton: View {
    var body: some View {
        ShareLink(item: "\(Install.command)\n\(Install.startLink)") {
            Text("Send to my computer")
                .frame(maxWidth: .infinity, minHeight: 44)
        }
        .buttonStyle(.borderedProminent)
        .foregroundStyle(Theme.bg)
    }
}

/// What the demo ends in: the person has no host and a phone cannot run the install, so the way
/// on is to send the line to a computer, or to pair a host from here.
struct InviteSheet: View {
    let onPair: () -> Void
    let onKeepLooking: () -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                Text("Your agents wait the same way.")
                    .font(.title2.bold())
                    .accessibilityAddTraits(.isHeader)
                InstallLine()
                VStack(alignment: .leading, spacing: 12) {
                    SendToComputerButton()
                    Button(action: onPair) {
                        Text("Pair a host")
                            .frame(maxWidth: .infinity, minHeight: 44)
                    }
                    .buttonStyle(.bordered)
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
    }
}
