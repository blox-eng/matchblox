import Foundation

/// What the console does after a session ends. A good session (long, with output) ended because the console quit, so the builder taps to open it again. A short session (it ended soon after it opened, or it showed no output) opens again and waits longer each time; after three in a row the console stops, so a host that ends every session is not called in a loop.
public struct ReconnectDelay: Equatable, Sendable {
    public enum Next: Equatable, Sendable {
        case reconnect(after: Duration)
        case ended
        case stop
    }

    public static let steady = Duration.seconds(5)
    private static let waits: [Duration] = [.seconds(1), .seconds(2)]

    private var shortSessions = 0

    public init() {}

    public mutating func next(after lifetime: Duration, sawOutput: Bool) -> Next {
        guard lifetime < Self.steady || !sawOutput else {
            shortSessions = 0
            return .ended
        }
        shortSessions += 1
        guard shortSessions <= Self.waits.count else { return .stop }
        return .reconnect(after: Self.waits[shortSessions - 1])
    }
}
