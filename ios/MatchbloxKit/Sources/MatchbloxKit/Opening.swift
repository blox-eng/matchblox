import Foundation

public enum Route: Equatable, Sendable {
    case scan
    case pair(PairDraft)
    case console(Host)
    case hosts
    case reach(Host, ConnectFailure)
}

public enum Opening {
    /// One host opens straight into its console; several hosts ask which one.
    public static func first(hosts: [Host]) -> Route {
        switch hosts.count {
        case 0: .scan
        case 1: .console(hosts[0])
        default: .hosts
        }
    }

    /// The texts for a host that was paired before. Pairing.message keeps the texts for the pairing itself.
    public static func message(for failure: ConnectFailure, host: String) -> String {
        switch failure {
        case .refused:
            "This phone's key no longer opens \(host). Run matchblox pair on \(host), then scan the new code."
        case .closed:
            "The connection to \(host) closed. Tap Try again, or scan again."
        case .unreachable, .hostKeyChanged:
            Pairing.message(for: failure, host: host)
        }
    }
}
