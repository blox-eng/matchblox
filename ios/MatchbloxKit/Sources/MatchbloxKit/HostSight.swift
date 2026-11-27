import Foundation

/// How a host row looks on Pocket home (no background probe in v1).
public enum HostRowStatus: Equatable, Sendable {
    case live
    case lastSeen(at: Date)
    case notReachable(since: Date)
    /// No last-seen yet; keep the address only (checking spinner unused in v1).
    case unknown
}

/// Per-host last-seen / last-fail times for home rows.
public struct HostSight {
    private let defaults: UserDefaults
    private static let seenPrefix = "pocket.home.seen."
    private static let failPrefix = "pocket.home.fail."

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    public mutating func markSeen(_ fingerprint: String, at date: Date = Date()) {
        defaults.set(date.timeIntervalSince1970, forKey: Self.seenPrefix + fingerprint)
        defaults.removeObject(forKey: Self.failPrefix + fingerprint)
    }

    public mutating func markFailed(_ fingerprint: String, at date: Date = Date()) {
        defaults.set(date.timeIntervalSince1970, forKey: Self.failPrefix + fingerprint)
    }

    public func status(fingerprint: String, liveFingerprint: String?) -> HostRowStatus {
        if liveFingerprint == fingerprint { return .live }
        let fail = date(for: Self.failPrefix + fingerprint)
        let seen = date(for: Self.seenPrefix + fingerprint)
        if let fail, seen.map({ $0 < fail }) ?? true {
            return .notReachable(since: fail)
        }
        if let seen { return .lastSeen(at: seen) }
        return .unknown
    }

    private func date(for key: String) -> Date? {
        guard defaults.object(forKey: key) != nil else { return nil }
        return Date(timeIntervalSince1970: defaults.double(forKey: key))
    }
}
