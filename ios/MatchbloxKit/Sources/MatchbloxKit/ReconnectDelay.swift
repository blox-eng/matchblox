import Foundation

/// The wait before the console opens again. A short session (it ended soon after it opened, or it showed no output) waits longer each time; after three in a row the console stops, so a host that ends every session is not called in a loop.
public struct ReconnectDelay: Equatable, Sendable {
    public static let steady = Duration.seconds(5)
    private static let waits: [Duration] = [.seconds(1), .seconds(2)]

    private var shortSessions = 0

    public init() {}

    /// Returns nil when another try will not help.
    public mutating func next(after lifetime: Duration, sawOutput: Bool) -> Duration? {
        guard lifetime < Self.steady || !sawOutput else {
            shortSessions = 0
            return .zero
        }
        shortSessions += 1
        guard shortSessions <= Self.waits.count else { return nil }
        return Self.waits[shortSessions - 1]
    }
}
