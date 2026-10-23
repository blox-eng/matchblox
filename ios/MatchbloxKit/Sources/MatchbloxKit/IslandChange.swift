import Foundation

/// What a change of Follow or Buzz did with the hosts concerned (design 0010 §4.3).
public struct IslandChange: Equatable, Sendable {
    /// One line on the Island screen for a host the change did not reach.
    public struct Notice: Hashable, Sendable {
        public var label: String
        public var line: String

        public init(label: String, line: String) { self.label = label; self.line = line }
    }

    /// The hosts the change did not reach, each with its line.
    public var notChanged: [Notice] = []
    /// The hosts the change did not try, as it was cancelled: they stay pending.
    public var untried: [Host] = []

    public init() {}

    /// Notes a host whose registration threw. A cancel (the app went to the background, or a
    /// newer change) leaves the host pending; any other error is a host not changed.
    public mutating func failed(_ host: Host, _ error: Error, cancelled: Bool) {
        if cancelled || error is CancellationError {
            untried.append(host)
        } else {
            notChanged.append(Notice(label: host.label, line: Opening.notChanged(host: host.label, error)))
        }
    }
}

extension IslandChange {
    /// Whether a channel the relay did not delete is forgotten all the same: only when the
    /// relay refused it for good (400, 403). A limit, a fault, an older relay or no network
    /// (nil) keeps it, and the next registration deletes it then.
    public static func forgetsChannel(refusedWith status: Int?) -> Bool {
        status == 400 || status == 403
    }
}

extension Opening {
    /// The line for a host a change did not reach: an unreachable host says so; anything
    /// else (the relay, the network of the phone, a grant) only that the host did not change.
    public static func notChanged(host: String, _ error: Error) -> String {
        if case .unreachable? = error as? ConnectFailure { return notReachable(host: host) }
        return "\(host) is not changed: it changes at the next connect"
    }

    /// The lines for what iOS keeps from the phone: no notification, so no buzz; no Live
    /// Activity, so no island.
    public static func blocked(notificationsDenied: Bool, activitiesEnabled: Bool) -> [String] {
        var lines: [String] = []
        if notificationsDenied {
            lines.append("Notifications are off for matchblox: no buzz comes. Turn them on in Settings.")
        }
        if !activitiesEnabled {
            lines.append("Live Activities are off for matchblox: no island shows. Turn them on in Settings.")
        }
        return lines
    }
}
