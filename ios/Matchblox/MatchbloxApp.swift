import MatchbloxKit
import SwiftUI

@main
struct MatchbloxApp: App {
    private enum Route {
        case scan(problem: String?)
        case pair(PairDraft)
        case console(Host)
    }

    @State private var route = Route.scan(problem: nil)

    var body: some Scene {
        WindowGroup {
            ZStack {
                Theme.bg.ignoresSafeArea()
                switch route {
                case .scan(let problem):
                    ScanView(problem: problem, onCode: open)
                case .pair(let draft):
                    PairView(
                        draft: draft,
                        store: .standard,
                        onPaired: { route = .console($0) },
                        onScanAgain: { route = .scan(problem: nil) }
                    )
                case .console(let host):
                    ConsolePlaceholder(host: host, onScanAgain: { route = .scan(problem: nil) })
                }
            }
            .foregroundStyle(Theme.fg)
            .fontDesign(.monospaced)
            .tint(Theme.accent)
            .preferredColorScheme(.dark)
            .onOpenURL(perform: open)
        }
    }

    private func open(_ url: URL) {
        do {
            let payload = try PairPayload(url: url)
            route = .pair(PairDraft(payload: payload, phoneName: Pairing.cleanName(UIDevice.current.name)))
        } catch {
            route = .scan(problem: Pairing.message(for: error))
        }
    }
}

/// Stands in for the console window until it is built.
private struct ConsolePlaceholder: View {
    let host: Host
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
