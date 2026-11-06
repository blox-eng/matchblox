import Foundation

/// A vertical swipe as mouse wheel steps, for a terminal whose program reads the mouse (tmux,
/// the console). A step scrolls `linesPerStep` lines (tmux copy mode scrolls 5, the console 1),
/// so the lines keep pace with the finger: one step for each `linesPerStep` cells of travel.
/// As in iOS, a finger that moves down shows older lines (wheel up), and a finger that moves
/// up shows newer lines.
public struct WheelScroll: Sendable {
    public enum Step: Sendable, Equatable {
        case up
        case down

        /// The mouse button, as SwiftTerm's `encodeButton` numbers the wheel.
        public var button: Int { self == .up ? 4 : 5 }
    }

    /// A flick carries on for at most this long after the finger lifts. The coast is a
    /// distance, so it scrolls the same lines whatever `linesPerStep` is.
    public static let coastTime: TimeInterval = 0.5
    /// The lines a step scrolls in a tmux pane: tmux's copy-mode wheel binding
    /// (`send -X -N 5 scroll-up`).
    public static let tmuxLines = 5
    /// UIScrollView's normal deceleration (0.998 of the speed each millisecond), per second.
    private static let decay = -1000 * log(0.998)

    public let cellHeight: CGFloat
    public let linesPerStep: Int
    private var travel: CGFloat = 0

    public init(cellHeight: CGFloat, linesPerStep: Int = 1) {
        self.cellHeight = cellHeight
        self.linesPerStep = linesPerStep
    }

    /// The finger, or the coast, moved `dy` points down (up when negative).
    public mutating func move(_ dy: CGFloat) -> [Step] {
        let stepHeight = cellHeight * CGFloat(linesPerStep)
        guard stepHeight > 0 else { return [] }
        travel += dy
        let steps = (travel / stepHeight).rounded(.towardZero)
        travel -= steps * stepHeight
        return Array(repeating: steps > 0 ? Step.up : Step.down, count: Int(abs(steps)))
    }

    /// How far a flick that left the finger at `velocity` points a second has carried after
    /// `elapsed` seconds.
    public static func coast(velocity: CGFloat, after elapsed: TimeInterval) -> CGFloat {
        let t = min(max(elapsed, 0), coastTime)
        return velocity * CGFloat((1 - exp(-decay * t)) / decay)
    }
}
