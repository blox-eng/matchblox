import Foundation

public enum Route: Equatable, Sendable {
    case scan
    case demoScan
    case pair(PairDraft)
    case console(Host, open: Int?)
    case hosts
    case island
    case reach(Host, ConnectFailure)
}

public enum Opening {
    /// One host opens straight into its console; several hosts ask which one.
    public static func first(hosts: [Host]) -> Route {
        switch hosts.count {
        case 0: .scan
        case 1: .console(hosts[0], open: nil)
        default: .hosts
        }
    }

    /// The texts for a host that was paired before. Pairing.message keeps the texts for the pairing itself.
    public static func message(for failure: ConnectFailure, host: String) -> String {
        switch failure {
        case .unreachable:
            "Can't reach \(host)."
        case .hostKeyChanged:
            "\(host) has a new host key. Run matchblox pair on \(host) again."
        case .refused:
            "The key of this device no longer opens \(host). Run matchblox pair on \(host), then scan the new code."
        case .closed:
            "The connection to \(host) closed. Tap Try again, or scan again."
        }
    }

    public static func stopsAtOnceMessage(host: String) -> String {
        "matchblox console on \(host) stops at once. Run matchblox console on the host to see why, then tap Try again."
    }

    public static func endedMessage(host: String) -> String {
        "matchblox console on \(host) ended."
    }

    /// The demo has no key on a host, so only the real hosts are named in the sentence on keys.
    public static func removedNotice(_ hosts: [Host]) -> String {
        let real = hosts.filter { !$0.isDemo }.map(\.label)
        if hosts.count == 1 {
            return real.isEmpty
                ? "\(hosts[0].label) is removed."
                : "\(hosts[0].label) is removed. Remove its key on the host with matchblox devices rm."
        }
        let removed = "\(spoken(hosts.map(\.label))) are removed."
        switch real.count {
        case 0: return removed
        case 1: return "\(removed) Remove \(real[0])'s key on the host with matchblox devices rm."
        default: return "\(removed) Remove the keys of \(spoken(real)) on those hosts with matchblox devices rm."
        }
    }

    private static func spoken(_ labels: [String]) -> String {
        guard let last = labels.last, labels.count > 1 else { return labels.first ?? "" }
        return "\(labels.dropLast().joined(separator: ", ")) and \(last)"
    }
}
