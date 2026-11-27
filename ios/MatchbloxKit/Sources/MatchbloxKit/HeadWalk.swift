import Foundation

/// One roll of the head, notch by notch. At a menu a notch moves Pocket's own highlight; past the
/// first option the roll crosses into the output and scrolls it. A roll that left the menu never
/// comes back into it, so it never moves the agent's choice; a new roll does. No step is a key:
/// a menu is answered only by a pick the host checks.
public struct HeadWalk: Sendable {
    /// `up` is anticlockwise.
    public enum Dir: Sendable {
        case up, down
    }

    public enum Bump: Equatable, Sendable {
        case lastOption, top, bottom, nothing
    }

    public enum Step: Equatable, Sendable {
        case highlight(Int)
        /// No menu reply yet: the notch is dropped.
        case wait
        case cross
        /// Positive: older lines.
        case scroll(Int)
        case home
        case bump(Bump)
    }

    public enum Tap: Equatable, Sendable {
        case pick(n: Int, text: String, sign: String)
        case enter
        case home
    }

    private enum Sight: Sendable {
        case asking
        case none
        case menu(MenuSight)
    }

    private var sight = Sight.asking
    /// The option Pocket highlights, by its number.
    public private(set) var highlight = 0
    public private(set) var linesUp = 0
    private var notchesSinceCross = 0
    private var lastNotch: TimeInterval?
    /// This roll scrolled or crossed: it stays in the output.
    private var inOutput = false

    public init() {}

    /// The menu the session shows now, nil when none. The same menu again keeps the highlight.
    public mutating func menu(_ menu: MenuSight?) {
        guard let menu else { sight = .none; return }
        if case .menu(let seen) = sight, seen.sign == menu.sign {} else { highlight = menu.chosen }
        sight = .menu(menu)
    }

    /// `route` is where a notch of output would go: for `up`, asked for older lines.
    public mutating func notch(_ dir: Dir, route: HeadRoute, at time: TimeInterval) -> Step {
        switch sight {
        case .asking:
            return .wait
        case .menu(let menu) where linesUp == 0 && !inOutput:
            switch dir {
            case .up where highlight > 1:
                highlight -= 1
                return .highlight(highlight)
            case .up:
                inOutput = true
                notchesSinceCross = 0
                lastNotch = nil
                return .cross
            case .down where highlight < menu.options.count:
                highlight += 1
                return .highlight(highlight)
            case .down:
                return .bump(.lastOption)
            }
        default:
            return scroll(dir, route: route, at: time)
        }
    }

    private mutating func scroll(_ dir: Dir, route: HeadRoute, at time: TimeInterval) -> Step {
        inOutput = true
        switch (dir, route) {
        case (.up, .end): return .bump(.top)
        case (.up, .nothing): return .bump(.nothing)
        case (.down, _) where linesUp == 0: return .bump(.bottom)
        default: break
        }
        if let lastNotch, time - lastNotch >= MatchHead.pause { notchesSinceCross = 0 }
        lastNotch = time
        notchesSinceCross += 1
        let lines = MatchHead.pace(notchesSinceCross: notchesSinceCross)
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

    /// A tap on the middle: up in the output it comes home; at a menu it picks the highlight;
    /// else it is Enter.
    public mutating func tap() -> Tap {
        if linesUp > 0 {
            linesUp = 0
            return .home
        }
        guard case .menu(let menu) = sight, let option = menu.options.first(where: { $0.n == highlight }) else {
            return .enter
        }
        return .pick(n: option.n, text: option.text, sign: menu.sign)
    }

    /// The finger left the head: the next notch starts a new roll.
    public mutating func lift() {
        inOutput = false
        notchesSinceCross = 0
        lastNotch = nil
    }
}
