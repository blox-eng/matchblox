import MatchbloxKit
import SwiftUI

/// Stands in for the console window until it is built.
/// The console opens its own SSHSession and reports a failed connect through `onFailure`; the router shows the screen that fixes it.
struct ConsoleView: View {
    let host: Host
    let onFailure: (ConnectFailure) -> Void
    let onScanAgain: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Paired with \(host.label).")
                .font(.title3)
            Text("\(host.user)@\(host.address)")
                .foregroundStyle(Theme.muted)
            Button("Pair another host", action: onScanAgain)
                .foregroundStyle(Theme.muted)
            Spacer()
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(24)
    }
}
