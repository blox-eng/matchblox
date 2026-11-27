import Foundation

/// The match head. On the dock the finger turns it round its edge: a turn moves in notches of
/// `notch` radians; the more notches since the crossing into the output, the more lines a notch
/// moves (`pace`). Angles are `atan2(dy, dx)` in UIKit points, so a growing angle is clockwise.
/// On the belt the thumb rolls it: a roll moves in notches of `rollNotch` points; a faster roll
/// moves more lines a notch, and a roll down is positive and shows older lines.
public struct MatchHead: Sendable {
    public struct Roll: Equatable, Sendable {
        /// Positive: clockwise, or a roll down.
        public let notches: Int
        /// `WheelScroll`'s sense: positive shows older lines, so an anticlockwise turn or a roll
        /// down is positive.
        public let lines: Int
        /// The head struck on this roll: play the rigid tap once.
        public let strike: Bool
    }

    public static let notch: CGFloat = .pi / 12
    /// A roll with no notch this long starts its pace again at 1.
    public static let pause: TimeInterval = 0.35
    /// The most lines a notch moves; the head strikes at it.
    static let maxLines = 20
    /// The head goes out below this heat.
    static let outHeat: CGFloat = 0.5
    /// The share of heat left after one 60 Hz frame with no roll.
    static let coolPerFrame: CGFloat = 0.94

    static let rollNotch: CGFloat = 10
    /// The roll's speed (pt/s) at which heat is 1.
    static let white: CGFloat = 2600
    /// A roll strikes the head at this heat.
    static let strikeHeat: CGFloat = 0.92
    /// Up to this speed (pt/s) a roll's notch moves one line.
    static let slow: CGFloat = 250
    /// From this speed a roll's notch moves `rollMaxLines`.
    static let fast: CGFloat = 2400
    static let rollMaxLines = 10

    /// 0 cold to 1 white hot: a turn's lines a notch ÷ `maxLines`, a roll's speed ÷ `white`.
    public private(set) var heat: CGFloat = 0
    /// The head is alight.
    public private(set) var struck = false

    /// Taps closer than this blur into a buzz on the Taptic Engine.
    public static let tapGap: TimeInterval = 1.0 / 30

    private var angle: CGFloat?
    private var travel: CGFloat = 0
    private var notchesSinceCross = 0
    private var lastNotch: TimeInterval?
    private var lastTap: TimeInterval?
    private var rollTravel: CGFloat = 0

    public init() {}

    /// Whether a notch at `time` (seconds) taps: never two taps within `tapGap`.
    public mutating func tap(at time: TimeInterval) -> Bool {
        if let lastTap, time - lastTap < Self.tapGap { return false }
        lastTap = time
        return true
    }

    /// The finger is at `angle` radians round the head at `time`. The first angle of a roll only
    /// sets where it starts.
    public mutating func turn(angle: CGFloat, at time: TimeInterval) -> Roll {
        defer { self.angle = angle }
        guard let last = self.angle else { return Roll(notches: 0, lines: 0, strike: false) }
        travel += (angle - last).remainder(dividingBy: 2 * .pi)
        let notches = Int((travel / Self.notch).rounded(.towardZero))
        travel -= CGFloat(notches) * Self.notch
        guard notches != 0 else { return Roll(notches: 0, lines: 0, strike: false) }
        if let lastNotch, time - lastNotch >= Self.pause { notchesSinceCross = 0 }
        lastNotch = time
        var lines = 0
        var each = 1
        for _ in 0..<abs(notches) {
            notchesSinceCross += 1
            each = Self.pace(notchesSinceCross: notchesSinceCross)
            lines += each
        }
        heat = CGFloat(each) / CGFloat(Self.maxLines)
        let strike = !struck && each == Self.maxLines
        settle(strike: strike)
        return Roll(notches: notches, lines: notches < 0 ? lines : -lines, strike: strike)
    }

    /// The thumb rolled the head `dy` points down (up when negative) at `speed` points a second.
    public mutating func roll(dy: CGFloat, speed: CGFloat) -> Roll {
        rollTravel += dy
        let notches = (rollTravel / Self.rollNotch).rounded(.towardZero)
        rollTravel -= notches * Self.rollNotch
        let count = Int(notches)
        heat = min(abs(speed) / Self.white, 1)
        let strike = !struck && heat >= Self.strikeHeat
        settle(strike: strike)
        return Roll(notches: count, lines: count * Self.lines(atSpeed: speed), strike: strike)
    }

    /// The roll crossed out of the options into the output: its pace, heat and strike count from here.
    public mutating func cross() {
        notchesSinceCross = 0
    }

    /// The finger left the head: the next turn starts a new roll.
    public mutating func lift() {
        angle = nil
        travel = 0
        notchesSinceCross = 0
        lastNotch = nil
    }

    /// A frame of `frame` seconds passed with no roll: the head cools, and goes out.
    public mutating func cool(frame: TimeInterval) {
        heat *= pow(Self.coolPerFrame, CGFloat(frame * 60))
        settle(strike: false)
    }

    private mutating func settle(strike: Bool) {
        if strike { struck = true } else if heat < Self.outHeat { struck = false }
    }

    /// Lines a notch moves, by the notches since the crossing (the first is 1): 1 to 10, 3 to 24,
    /// 8 to 48, then `maxLines`.
    public static func pace(notchesSinceCross n: Int) -> Int {
        switch n {
        case ...10: 1
        case ...24: 3
        case ...48: 8
        default: maxLines
        }
    }

    /// Lines a roll's notch moves: 1 up to `slow`, rising on a curve to `rollMaxLines` at `fast`.
    public static func lines(atSpeed speed: CGFloat) -> Int {
        let t = min(max((abs(speed) - slow) / (fast - slow), 0), 1)
        return 1 + Int((CGFloat(rollMaxLines - 1) * t * t).rounded())
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
