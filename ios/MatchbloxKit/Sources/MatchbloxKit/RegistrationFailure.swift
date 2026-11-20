import Foundation

/// Why the last registration with a host failed, and when. The Island screen shows it under
/// the host, with its fix, until a registration with that host succeeds.
public struct RegistrationFailure: Codable, Equatable, Sendable {
    public enum Kind: String, Codable, Sendable {
        case noDeviceToken, noNotifyKey, noGrant, noAppAttest, relay, relayReply
        case unreachable, hostKeyChanged, refused, other
    }

    public let kind: Kind
    /// The words of the error: the grant's kind, the relay's answer, or any other error's text.
    public let detail: String
    public let at: Date

    public init(_ kind: Kind, detail: String = "", at: Date) {
        self.kind = kind
        self.detail = detail
        self.at = at
    }

    /// A failure to reach or open the host, as `ConnectFailure` sorts it.
    public static func connect(_ error: Error, at: Date) -> RegistrationFailure {
        switch ConnectFailure.classify(error) {
        case .unreachable: RegistrationFailure(.unreachable, at: at)
        case .hostKeyChanged: RegistrationFailure(.hostKeyChanged, at: at)
        case .refused: RegistrationFailure(.refused, at: at)
        case .closed(let why): RegistrationFailure(.other, detail: why, at: at)
        }
    }

    /// One line: what failed, then its fix.
    public var line: String {
        let retry = "open matchblox again to retry"
        let said = detail.isEmpty ? "an error with no text" : detail
        let parts: (what: String, fix: String) = switch kind {
        case .noDeviceToken: ("no APNs device token", "allow notifications for matchblox in Settings")
        case .noNotifyKey: ("the notification key could not be kept", "unlock the phone, then \(retry)")
        case .noGrant: ("the relay gave no grant for \(detail)", retry)
        case .noAppAttest: ("App Attest is not available on this device", "the buzz and the island need it")
        case .relay: (said, retry)
        case .relayReply: ("the relay's reply is not what this app reads", "update matchblox")
        case .unreachable: ("the host is not reachable", "\(retry) where the phone reaches the host")
        case .hostKeyChanged: ("the host has a new host key", "pair the host again")
        case .refused: ("the host refused the phone's key", "pair the host again")
        case .other: (said, retry)
        }
        return "Not registered: \(parts.what) — \(parts.fix)"
    }
}
