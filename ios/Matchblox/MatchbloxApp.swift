import MatchbloxKit
import SwiftUI

@main
struct MatchbloxApp: App {
    private let store = HostStore.standard
    @State private var route = Opening.first(hosts: HostStore.standard.all())
    @State private var scanProblem: String?

    var body: some Scene {
        WindowGroup {
            ZStack {
                Theme.bg.ignoresSafeArea()
                switch route {
                case .scan:
                    ScanView(
                        problem: scanProblem,
                        onCode: open,
                        onCancel: store.all().isEmpty ? nil : { go(Opening.first(hosts: store.all())) }
                    )
                case .pair(let draft):
                    PairView(
                        draft: draft,
                        store: store,
                        onPaired: { go(.console($0)) },
                        onScanAgain: { go(.scan) }
                    )
                case .console(let host):
                    ConsoleView(
                        host: host,
                        onFailure: { go(.reach(host, $0)) },
                        onScanAgain: { go(.scan) }
                    )
                case .hosts:
                    HostsView(
                        store: store,
                        onOpen: { go(.console($0)) },
                        onPair: { go(.scan) },
                        onNoHosts: { go(.scan) }
                    )
                case .reach(let host, let failure):
                    ReachView(
                        host: host,
                        failure: failure,
                        store: store,
                        onRetry: { go(.console($0)) },
                        onScanAgain: { go(.scan) }
                    )
                }
            }
            .foregroundStyle(Theme.fg)
            .fontDesign(.monospaced)
            .tint(Theme.accent)
            .preferredColorScheme(.dark)
            .onOpenURL(perform: open)
        }
    }

    private func go(_ next: Route) {
        scanProblem = nil
        route = next
    }

    private func open(_ url: URL) {
        do {
            let payload = try PairPayload(url: url)
            go(.pair(PairDraft(payload: payload, phoneName: Pairing.cleanName(UIDevice.current.name))))
        } catch {
            scanProblem = Pairing.message(for: error)
            route = .scan
        }
    }
}
