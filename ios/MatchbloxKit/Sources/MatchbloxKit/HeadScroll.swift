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

    /// The view came back to the live bottom another way: a typed send, a belt key, another pane.
    public mutating func cameDown() {
        linesUp = 0
        lift()
    }

    /// A finger's swipe moved the view `lines` lines (positive: older). It counts toward a walk
    /// that is up, never below 0, and starts none; true when it brought the walk down.
    public mutating func panned(_ lines: Int) -> Bool {
        guard linesUp > 0 else { return false }
        linesUp = max(linesUp + lines, 0)
        guard linesUp == 0 else { return false }
        cameDown()
        return true
    }

    /// `route` is where a notch toward newer lines would go now. At the live bottom (`.end`) the
    /// view is down, whatever brought it there; true when that ended a walk that was up.
    public mutating func check(newer route: HeadRoute) -> Bool {
        guard linesUp > 0, route == .end else { return false }
        cameDown()
        return true
    }

    /// The roll ended: the next notch starts the pace again.
    public mutating func lift() {
        notches = 0
        lastNotch = nil
    }
}
