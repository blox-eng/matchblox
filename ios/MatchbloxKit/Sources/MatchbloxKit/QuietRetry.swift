import Foundation

/// What the console does when a connect fails. On a return the last screen stays: iOS brings
/// the network back after Pocket is active, so the first connect can fail at once, and the
/// console tries again behind the screen. After `window` it says so on that screen. A changed
/// host key or a refused key needs the person, and a first open has no screen to keep: those
/// leave for the Reach screen at once.
public struct QuietRetry: Equatable, Sendable {
    public enum Next: Equatable, Sendable {
        case retry(after: Duration)
        case say
        case leave
    }

    public static let window = Duration.seconds(20)
    private static let waits: [Duration] = [.seconds(1), .seconds(2), .seconds(4)]

    private let keptScreen: Bool
    private var tries = 0
    private var firstFailure: ContinuousClock.Instant?

    public init(keptScreen: Bool) {
        self.keptScreen = keptScreen
    }

    public mutating func next(after failure: ConnectFailure, at now: ContinuousClock.Instant) -> Next {
        switch failure {
        case .hostKeyChanged, .refused: return .leave
        case .unreachable, .closed: break
        }
        guard keptScreen else { return .leave }
        let first = firstFailure ?? now
        firstFailure = first
        guard now - first < Self.window else { return .say }
        let wait = Self.waits[min(tries, Self.waits.count - 1)]
        tries += 1
        return .retry(after: wait)
    }
}

/// Keys typed while the console comes back. They go to the host once it is there; a flood
/// past `limit` is not held.
public struct HeldKeys: Equatable, Sendable {
    public static let limit = 4096
    private var keys = Data()

    public init() {}

    public mutating func hold(_ data: Data) {
        guard keys.count + data.count <= Self.limit else { return }
        keys.append(data)
    }

    public mutating func take() -> Data? {
        guard !keys.isEmpty else { return nil }
        defer { keys = Data() }
        return keys
    }

    public mutating func drop() {
        keys = Data()
    }
}
