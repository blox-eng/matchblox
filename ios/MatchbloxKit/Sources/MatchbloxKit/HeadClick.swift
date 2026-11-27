import Foundation

/// What a lift from the head's middle means. Enter can answer an agent, so only a short, still
/// touch on a head at rest is Enter: a touch that stops a coast, comes right after a roll, or
/// rolls itself sends nothing. A still touch held `hold` seconds opens the sessions.
public struct HeadClick: Sendable {
    public enum Lift: Equatable, Sendable {
        case enter, nothing, hold
    }

    /// The head must not have rolled this long before the touch.
    public static let still: TimeInterval = 0.3
    /// The most a tap may travel, in points.
    public static let slop: CGFloat = 6
    /// The longest a tap may last.
    public static let press: TimeInterval = 0.35
    /// A still touch this long starts the hold, while the finger is still down.
    public static let hold: TimeInterval = 0.6

    private var start: TimeInterval = 0
    private var coasting = false
    private var rolledBefore = false
    private var travel: CGFloat = 0

    public init() {}

    /// `rolled`: the head rolled before the touch or during it, before the hold began. Moves in the
    /// hold step through the sessions; they do not undo it.
    public static func lift(after held: TimeInterval, rolled: Bool, coasting: Bool) -> Lift {
        guard !rolled, !coasting else { return .nothing }
        if holdDue(after: held) { return .hold }
        return held < press ? .enter : .nothing
    }

    public static func holdDue(after held: TimeInterval) -> Bool {
        held >= hold
    }

    public mutating func began(at time: TimeInterval, coasting: Bool, lastRoll: TimeInterval?) {
        start = time
        self.coasting = coasting
        rolledBefore = lastRoll.map { time - $0 < Self.still } ?? false
        travel = 0
    }

    public mutating func moved(by distance: CGFloat) {
        travel += abs(distance)
    }

    public func isClick(endedAt time: TimeInterval) -> Bool {
        Self.lift(after: time - start, rolled: rolledBefore || travel >= Self.slop, coasting: coasting) == .enter
    }
}
