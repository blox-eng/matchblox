import Foundation

/// How the head and what it shows move, as the mockup moves them. Every reveal and hide is a
/// fade with a short scale, never a cut; times are in seconds.
public enum HeadMotion {
    /// A cubic Bézier timing curve through (0, 0), its two control points and (1, 1), as CSS's
    /// `cubic-bezier(x1, y1, x2, y2)` writes it. A `y` above 1 overshoots.
    public struct Curve: Equatable, Sendable {
        public let x1: Double
        public let y1: Double
        public let x2: Double
        public let y2: Double

        public init(x1: Double, y1: Double, x2: Double, y2: Double) {
            self.x1 = x1
            self.y1 = y1
            self.x2 = x2
            self.y2 = y2
        }
    }

    /// The marks and the ring fade in on touch-down, and out after the lift, in this long.
    public static let marksFade: TimeInterval = 0.18
    /// They grow from this scale as they fade in, and shrink back to it as they go.
    public static let marksFrom: Double = 0.86
    /// They start to go this long after the lift.
    public static let marksStay: TimeInterval = 0.9
    /// CSS's `ease`, the mockup's curve for a fade.
    public static let marksCurve = Curve(x1: 0.25, y1: 0.1, x2: 0.25, y2: 1)

    /// The ball springs back to the middle at the lift, past it and back; it follows the thumb
    /// with no delay while pushed.
    public static let springBack: TimeInterval = 0.24
    public static let springCurve = Curve(x1: 0.34, y1: 1.56, x2: 0.64, y2: 1)
    /// A coast keeps this much of its speed each 60 Hz frame.
    public static let coastKeeps: Double = 0.93

    /// The dragged belt comes in front of the head, and the head back, in this long.
    public static let beltForward: TimeInterval = 0.25
    /// The belt goes back behind the head this long after the drag stops.
    public static let beltBackAfter: TimeInterval = 1.4
    /// The keyboard rises and folds in this long: the system's own time, which Pocket keeps.
    public static let keyboard: TimeInterval = 0.25
    /// The matchbox drawer, which holds `BOX` (home), opens and closes in this long.
    public static let drawer: TimeInterval = 0.38
    public static let drawerCurve = Curve(x1: 0.2, y1: 0.8, x2: 0.2, y2: 1)
    /// The step card stays this long after the last fast notch.
    public static let cardStays: TimeInterval = 0.6

    /// How a reveal moves: Reduce Motion keeps the fades and drops the scales and the spring.
    public struct Rule: Equatable, Sendable {
        public let fade: TimeInterval
        /// The scale a reveal starts from and a hide ends at; 1 is none.
        public let fromScale: Double
        /// The ball springs back at the lift; else it is in the middle at once.
        public let springs: Bool

        public init(fade: TimeInterval, fromScale: Double, springs: Bool) {
            self.fade = fade
            self.fromScale = fromScale
            self.springs = springs
        }
    }

    public static func rule(reduceMotion: Bool) -> Rule {
        Rule(fade: marksFade, fromScale: reduceMotion ? 1 : marksFrom, springs: !reduceMotion)
    }
}
