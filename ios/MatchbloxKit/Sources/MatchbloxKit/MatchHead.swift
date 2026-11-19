import Foundation

/// The match head on the belt: a ball the thumb rolls to scroll the terminal. A roll moves in
/// notches of `notch` points; a faster roll moves more lines a notch. Notches have
/// `WheelScroll`'s sense: a roll down is positive and shows older lines.
public struct MatchHead: Sendable {
    public struct Roll: Equatable, Sendable {
        public let notches: Int
        public let lines: Int
    }

    public static let notch: CGFloat = 10
    /// Up to this speed (pt/s) a notch moves one line.
    static let slow: CGFloat = 250
    /// From this speed a notch moves `maxLines`.
    static let fast: CGFloat = 2400
    static let maxLines = 10

    /// Taps closer than this blur into a buzz on the Taptic Engine.
    static let tapGap: TimeInterval = 1.0 / 30

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
        return Roll(notches: count, lines: count * Self.lines(atSpeed: speed))
    }

    /// Lines a notch moves: 1 up to `slow`, rising on a curve to `maxLines` at `fast`.
    public static func lines(atSpeed speed: CGFloat) -> Int {
        let t = min(max((abs(speed) - slow) / (fast - slow), 0), 1)
        return 1 + Int((CGFloat(maxLines - 1) * t * t).rounded())
    }
}
