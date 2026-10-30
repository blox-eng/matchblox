import Foundation

/// A vertical swipe as mouse wheel steps, for a terminal whose program reads the mouse (tmux,
/// the console). One step for each cell height the finger travels. As in iOS, a finger that
/// moves down shows older lines (wheel up), and a finger that moves up shows newer lines.
public struct WheelScroll: Sendable {
    public enum Step: Sendable, Equatable {
        case up
        case down

        /// The mouse button flags: buttons 4 and 5, as SwiftTerm's `encodeButton` gives them.
        public var flags: Int { self == .up ? 64 : 65 }
    }

    /// A flick carries on for at most this long after the finger lifts.
    public static let coastTime: TimeInterval = 0.5
    /// UIScrollView's normal deceleration (0.998 of the speed each millisecond), per second.
    private static let decay = -1000 * log(0.998)

    public let cellHeight: CGFloat
    private var travel: CGFloat = 0

    public init(cellHeight: CGFloat) {
        self.cellHeight = cellHeight
    }

    /// The finger, or the coast, moved `dy` points down (up when negative).
    public mutating func move(_ dy: CGFloat) -> [Step] {
        guard cellHeight > 0 else { return [] }
        travel += dy
        let cells = (travel / cellHeight).rounded(.towardZero)
        travel -= cells * cellHeight
        return Array(repeating: cells > 0 ? Step.up : Step.down, count: Int(abs(cells)))
    }

    /// How far a flick that left the finger at `velocity` points a second has carried after
    /// `elapsed` seconds.
    public static func coast(velocity: CGFloat, after elapsed: TimeInterval) -> CGFloat {
        let t = min(max(elapsed, 0), coastTime)
        return velocity * CGFloat((1 - exp(-decay * t)) / decay)
    }
}
