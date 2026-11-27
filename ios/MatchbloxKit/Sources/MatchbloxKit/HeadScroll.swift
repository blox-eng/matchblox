import Foundation

/// The head's walk through the output. A roll of the edge scrolls it notch by notch, faster the
/// longer it goes; a push past the first option reads it a line a push. No step is a key.
public struct HeadScroll: Sendable {
    /// `up` is anticlockwise, or a push `↑`: toward older lines.
    public enum Dir: Sendable {
        case up, down
    }

    public enum Bump: Equatable, Sendable {
        case top, bottom, nothing
    }

    public enum Step: Equatable, Sendable {
        /// Positive: older lines.
        case scroll(Int)
        case home
        case bump(Bump)
    }

    public enum Tap: Equatable, Sendable {
        case enter
        /// Up in the output: the tap comes back down and sends nothing.
        case home
    }

    public private(set) var linesUp = 0
    private var notches = 0
    private var lastNotch: TimeInterval?

    public init() {}

    /// One notch of a roll. `route` is where a notch of output would go: for `up`, asked for older lines.
    public mutating func notch(_ dir: Dir, route: HeadRoute, at time: TimeInterval) -> Step {
        if let bump = bump(dir, route: route) { return bump }
        if let lastNotch, time - lastNotch >= MatchHead.pause { notches = 0 }
        lastNotch = time
        notches += 1
        return move(dir, lines: MatchHead.pace(notchesSinceCross: notches), route: route)
    }

    /// One push: a line, at any pace.
    public mutating func push(_ dir: Dir, route: HeadRoute) -> Step {
        bump(dir, route: route) ?? move(dir, lines: 1, route: route)
    }

    private func bump(_ dir: Dir, route: HeadRoute) -> Step? {
        switch (dir, route) {
        case (.up, .end): return Step.bump(.top)
        case (.up, .nothing): return Step.bump(.nothing)
        case (.down, _) where linesUp == 0: return Step.bump(.bottom)
        default: return nil
        }
    }

    private mutating func move(_ dir: Dir, lines: Int, route: HeadRoute) -> Step {
        if dir == .up {
            linesUp += lines
            return .scroll(lines)
        }
        if route == .end || lines >= linesUp {
            linesUp = 0
            return .home
        }
        linesUp -= lines
        return .scroll(-lines)
    }

    /// A tap on the middle: up in the output it comes home, else it is Enter.
    public mutating func tap() -> Tap {
        guard linesUp > 0 else { return .enter }
        linesUp = 0
        return .home
    }

    /// The roll ended: the next notch starts the pace again.
    public mutating func lift() {
        notches = 0
        lastNotch = nil
    }
}
