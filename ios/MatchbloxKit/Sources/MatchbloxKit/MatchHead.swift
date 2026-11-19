import Foundation

/// The match head on the belt: a ball the thumb rolls to scroll the terminal. A roll moves in
/// notches of `notch` points; a faster roll moves more lines a notch. Notches have
/// `WheelScroll`'s sense: a roll down is positive and shows older lines.
public struct MatchHead: Sendable {
    public struct Roll: Equatable, Sendable {
        public let notches: Int
        public let lines: Int
        /// The head struck on this roll: play the rigid tap once.
        public let strike: Bool
    }

    /// The speed (pt/s) at which heat is 1.
    static let white: CGFloat = 2600
    /// The head strikes at this heat, and goes out below `outHeat`.
    static let strikeHeat: CGFloat = 0.92
    static let outHeat: CGFloat = 0.5
    /// The share of heat left after one 60 Hz frame with no roll.
    static let coolPerFrame: CGFloat = 0.94

    /// 0 cold to 1 white hot: how fast the head turns now.
    public private(set) var heat: CGFloat = 0
    /// The head is alight.
    public private(set) var struck = false

    public static let notch: CGFloat = 10
    /// Up to this speed (pt/s) a notch moves one line.
    static let slow: CGFloat = 250
    /// From this speed a notch moves `maxLines`.
    static let fast: CGFloat = 2400
    static let maxLines = 10

    /// Taps closer than this blur into a buzz on the Taptic Engine.
    public static let tapGap: TimeInterval = 1.0 / 30

    private var travel: CGFloat = 0
    private var lastTap: TimeInterval?

    public init() {}

    /// Whether a notch at `time` (seconds) taps: never two taps within `tapGap`.
    public mutating func tap(at time: TimeInterval) -> Bool {
        if let lastTap, time - lastTap < Self.tapGap { return false }
        lastTap = time
        return true
    }

    /// The thumb rolled the head `dy` points down (up when negative) at `speed` points a second.
    public mutating func roll(dy: CGFloat, speed: CGFloat) -> Roll {
        travel += dy
        let notches = (travel / Self.notch).rounded(.towardZero)
        travel -= notches * Self.notch
        let count = Int(notches)
        heat = min(abs(speed) / Self.white, 1)
        let strike = !struck && heat >= Self.strikeHeat
        settle(strike: strike)
        return Roll(notches: count, lines: count * Self.lines(atSpeed: speed), strike: strike)
    }

    /// A frame of `frame` seconds passed with no roll: the head cools, and goes out.
    public mutating func cool(frame: TimeInterval) {
        heat *= pow(Self.coolPerFrame, CGFloat(frame * 60))
        settle(strike: false)
    }

    private mutating func settle(strike: Bool) {
        if strike { struck = true } else if heat < Self.outHeat { struck = false }
    }

    /// Lines a notch moves: 1 up to `slow`, rising on a curve to `maxLines` at `fast`.
    public static func lines(atSpeed speed: CGFloat) -> Int {
        let t = min(max((abs(speed) - slow) / (fast - slow), 0), 1)
        return 1 + Int((CGFloat(maxLines - 1) * t * t).rounded())
    }
}

/// The head's lines as mouse wheel steps. A step scrolls `linesPerStep` lines (tmux scrolls 5).
public enum WheelBurst {
    /// The steps for one roll's `lines` (positive: older, as `WheelScroll.Step.up`), rounded away
    /// from zero: every notch the thumb feels moves the pane at least one step.
    public static func steps(lines: Int, linesPerStep: Int) -> Int {
        let per = max(linesPerStep, 1)
        let count = (abs(lines) + per - 1) / per
        return lines < 0 ? -count : count
    }
}
