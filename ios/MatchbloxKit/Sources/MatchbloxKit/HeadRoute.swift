/// Where a notch of the match head goes. A program that reads the mouse takes wheel steps; else
/// SwiftTerm's own scrollback moves; else nothing scrolls. `end` is a known end of the lines:
/// the head stops and bumps. A tmux pane's top is never known, its live bottom is.
public enum HeadRoute: Equatable, Sendable {
    case wheel
    case scrollback
    case end
    case nothing

    /// `position` is SwiftTerm's scroll position, 0 at the top and 1 at the bottom.
    /// `scrollBackSteps` counts the wheel steps a tmux pane is up from its bottom.
    public static func route(readsMouse: Bool, inTmuxPane: Bool, canScroll: Bool,
                             position: Double, scrollBackSteps: Int, older: Bool) -> HeadRoute {
        if readsMouse {
            return inTmuxPane && !older && scrollBackSteps == 0 ? .end : .wheel
        }
        guard canScroll else { return .nothing }
        if older ? position <= 0 : position >= 1 { return .end }
        return .scrollback
    }
}
