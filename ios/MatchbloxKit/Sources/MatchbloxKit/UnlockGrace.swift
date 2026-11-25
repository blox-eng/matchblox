/// How long one Face ID unlock holds after Pocket leaves the screen: a tap on the island
/// inside the window opens the console with no second prompt (owner, 2026-11-25).
public struct UnlockGrace: Equatable, Sendable {
    /// Apple's longest reuse of a biometric unlock (`LATouchIDAuthenticationMaximumAllowableReuseDuration`).
    public static let window: Duration = .seconds(300)

    private var leftAt: ContinuousClock.Instant?

    public init() {}

    public mutating func backgrounded(at instant: ContinuousClock.Instant) {
        leftAt = instant
    }

    /// True when the unlock still holds; false when Face ID must ask again.
    public mutating func returned(at instant: ContinuousClock.Instant) -> Bool {
        defer { leftAt = nil }
        guard let leftAt else { return true }
        return instant - leftAt <= Self.window
    }
}
