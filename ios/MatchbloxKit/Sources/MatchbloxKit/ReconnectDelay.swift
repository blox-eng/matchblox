import Foundation

/// The wait before the console opens again. A session that ends soon after it opened waits longer each time, so a host that drops every session is not called in a hot loop.
public struct ReconnectDelay: Equatable, Sendable {
    public static let steady = Duration.seconds(5)
    private static let steps: [Duration] = [.seconds(1), .seconds(2), .seconds(5)]

    private var shortSessions = 0

    public init() {}

    public mutating func next(after lifetime: Duration) -> Duration {
        guard lifetime < Self.steady else {
            shortSessions = 0
            return .zero
        }
        defer { shortSessions += 1 }
        return Self.steps[min(shortSessions, Self.steps.count - 1)]
    }
}
