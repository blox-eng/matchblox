import Foundation

/// Pocket home words: rows, VoiceOver, remove question, footer, Live Activity line.
public enum HomeCopy {
    public static let voiceOverBox = "Home, the box"

    public static func voiceOverBack(label: String) -> String {
        "Back to \(label)"
    }

    public static func hostRowSpoken(
        label: String,
        address: String,
        status: HostRowStatus,
        now: Date = Date()
    ) -> String {
        switch status {
        case .live:
            return "\(label), \(address), live"
        case .notReachable(let since):
            return "\(label), \(address), not reachable since \(clock(since))"
        case .lastSeen(let at):
            return "\(label), \(address), \(relative(from: at, to: now))"
        case .unknown:
            return "\(label), \(address)"
        }
    }

    public static func removeQuestion(label: String, isLive: Bool) -> String {
        var q = "remove \(label)? the phone forgets its key; matchblox pair brings it back."
        if isLive { q += " its console closes." }
        return q
    }

    public static func showDemoInFooter(hosts: [Host]) -> Bool {
        !hosts.contains(where: \.isDemo)
    }

    /// One line for the Live Activity row: host · show · buzz.
    public static func liveActivityRow(prefs: IslandPrefs, hosts: [Host]) -> String {
        let byFP = Dictionary(uniqueKeysWithValues: hosts.map { ($0.fingerprint, $0) })
        let hostWord: String
        if let fp = prefs.follow, let host = byFP[fp] {
            hostWord = host.label
        } else if prefs.follow == nil, prefs.choice == nil, let fp = prefs.lastOpened, let host = byFP[fp] {
            hostWord = host.label
        } else {
            hostWord = "none"
        }
        let showWord = prefs.show.rawValue
        let buzzWord: String
        if let fp = prefs.follow {
            buzzWord = prefs.buzzOn(fp) ? "buzz on" : "buzz off"
        } else {
            let anyOn = hosts.contains { prefs.buzzOn($0.fingerprint) }
            buzzWord = anyOn ? "buzz on" : "buzz off"
        }
        if hostWord == "none" {
            return "\(showWord)  \(buzzWord)"
        }
        return "\(hostWord) · \(showWord)  \(buzzWord)"
    }

    public static func relative(from then: Date, to now: Date) -> String {
        let seconds = max(0, Int(now.timeIntervalSince(then)))
        if seconds < 60 { return "just now" }
        let minutes = seconds / 60
        if minutes < 60 { return "\(minutes)m ago" }
        let hours = minutes / 60
        if hours < 48 { return "\(hours)h ago" }
        let days = hours / 24
        return "\(days)d ago"
    }

    public static func clock(_ date: Date) -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_GB")
        f.dateFormat = "HH:mm"
        return f.string(from: date)
    }
}
