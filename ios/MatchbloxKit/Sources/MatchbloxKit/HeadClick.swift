import Foundation

/// Whether a touch on the match head is a tap that sends Enter. Enter can answer an agent, so
/// only a short, still touch on a head at rest counts: a touch that stops a coast, comes right
/// after a roll, or rolls itself sends nothing.
public struct HeadClick: Sendable {
    /// The head must not have rolled this long before the touch.
    public static let still: TimeInterval = 0.3
    /// The most a tap may travel, in points.
    public static let slop: CGFloat = 6
    /// The longest a tap may last.
    public static let press: TimeInterval = 0.35

    private var start: TimeInterval = 0
    private var wasStill = false
    private var travel: CGFloat = 0

    public init() {}

    public mutating func began(at time: TimeInterval, coasting: Bool, lastRoll: TimeInterval?) {
        start = time
        wasStill = !coasting && lastRoll.map { time - $0 >= Self.still } ?? true
        travel = 0
    }

    public mutating func moved(by distance: CGFloat) {
        travel += abs(distance)
    }

    public func isClick(endedAt time: TimeInterval) -> Bool {
        wasStill && travel < Self.slop && time - start < Self.press
    }
}
