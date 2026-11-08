/// The app's mark before a key, so the console knows which screen asked (`keycmd.MarkKey`).
/// Every key the app sends to the console carries it: an unmarked key is taken as the desk's.
/// It goes only to a console that announced it reads marks: any other would show it as typed
/// text.
public enum InputMark {
    public static let bytes = Array("\u{1b}_matchblox;mark\u{1b}\\".utf8)

    /// What the app sends for `data`: behind the mark when `marked`, else as it is.
    public static func sent(_ data: ArraySlice<UInt8>, marked: Bool) -> [UInt8] {
        marked ? bytes + data : Array(data)
    }
}

/// The agent of the pane this app jumped into. The console and the desk share one tmux, so a
/// pane signal may answer another screen's jump: only one that comes within `window` of this
/// app's own mark counts, once for each mark. A session the app opened (a tap on the island or
/// a buzz) counts as a mark for `openWindow`: the console jumps once the phone is attached. The
/// agent goes once the terminal is back on the console for `settle`: a redraw split across
/// reads may hide a pane's way back for a moment.
public struct PaneAgent: Sendable {
    public static let window: Duration = .seconds(1)
    /// The console's wait for the phone's client (`focusWait`, 10 s), and its start.
    public static let openWindow: Duration = .seconds(15)
    public static let settle: Duration = .milliseconds(300)

    /// Until when a pane signal answers this app's own mark.
    private var answersUntil: ContinuousClock.Instant?
    private var inPane = false
    private var awaySince: ContinuousClock.Instant?
    public private(set) var agent: String?

    public init() {}

    /// The way back has gone from the screen, and the agent goes if it stays away for `settle`.
    public var leaving: Bool { awaySince != nil }

    public mutating func marked(at now: ContinuousClock.Instant) {
        answersUntil = max(answersUntil ?? now, now + Self.window)
    }

    /// The app opened one session: the console's jump into its pane is this app's.
    public mutating func opened(at now: ContinuousClock.Instant) {
        answersUntil = max(answersUntil ?? now, now + Self.openWindow)
    }

    public mutating func signal(_ s: ConsoleSignal, at now: ContinuousClock.Instant) {
        guard s.pane != nil, let until = answersUntil else { return }
        answersUntil = nil
        guard now <= until else { return }
        agent = s.agent
        // The jump has not landed yet: the console still shows.
        inPane = false
        awaySince = nil
    }

    /// Whether the terminal shows a pane's way back at `now`. True when the agent just went.
    public mutating func shows(pane: Bool, at now: ContinuousClock.Instant) -> Bool {
        if pane {
            inPane = true
            awaySince = nil
            return false
        }
        guard inPane else { return false }
        let since = awaySince ?? now
        awaySince = since
        guard now - since >= Self.settle else { return false }
        inPane = false
        awaySince = nil
        let had = agent != nil
        left()
        return had
    }

    /// Back on the console: the pane's agent no longer gets the keys.
    public mutating func left() {
        agent = nil
    }
}
