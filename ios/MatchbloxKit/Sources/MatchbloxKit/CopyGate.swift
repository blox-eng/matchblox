import Foundation

/// Decides whether a copy from the host (OSC 52) may reach the phone's clipboard. Any program
/// in a pane can send one, so only a copy the person asked for passes: one that comes within
/// `window` of a key that can ask for a copy (the belt's `c copy reply`, or a typed key). One
/// copy disarms the gate.
public struct CopyGate: Sendable {
    public enum Verdict: Equatable, Sendable {
        case copy(String)
        /// Asked for, but over `maxBytes`: nothing is copied.
        case tooLarge
        /// Nobody asked: nothing is copied.
        case unasked
    }

    public static let window: TimeInterval = 3
    public static let maxBytes = 1_000_000

    private var askedAt: Date?

    public init() {}

    /// The person sent a key that can ask for a copy.
    public mutating func asked(at now: Date) {
        askedAt = now
    }

    public mutating func take(_ content: Data, at now: Date) -> Verdict {
        guard let askedAt, now >= askedAt, now.timeIntervalSince(askedAt) <= Self.window else {
            self.askedAt = nil
            return .unasked
        }
        self.askedAt = nil
        guard content.count <= Self.maxBytes else { return .tooLarge }
        return .copy(String(decoding: content, as: UTF8.self))
    }
}
