/// The app's mark before a key that may jump, so the console knows which screen asked
/// (`keycmd.MarkKey`). It goes only to a console that announced it reads marks: any other
/// would show it as typed text.
public enum InputMark {
    public static let bytes = Array("\u{1b}_matchblox;mark\u{1b}\\".utf8)

    private static let esc: UInt8 = 0x1b
    private static let bracket = UInt8(ascii: "[")
    private static let sgr = UInt8(ascii: "<")
    private static let press = UInt8(ascii: "M")
    private static let release = UInt8(ascii: "m")
    private static let semicolon = UInt8(ascii: ";")
    private static let leftButton = Array("0".utf8)
    /// The classic report's button byte for a left press: 32 plus button 0.
    private static let classicLeft: UInt8 = 32

    /// True for a Return, or a press of the left button as SGR (`ESC [ < 0 ; x ; y M`) or classic
    /// (`ESC [ M cb x y`) reports it.
    public static func marks(_ data: ArraySlice<UInt8>) -> Bool {
        data.contains(0x0d) || pressesLeft(Array(data))
    }

    private static func pressesLeft(_ bytes: [UInt8]) -> Bool {
        var at = 0
        while at + 2 < bytes.count {
            guard bytes[at] == esc, bytes[at + 1] == bracket else { at += 1; continue }
            if bytes[at + 2] == sgr {
                let report = bytes[(at + 3)...]
                guard let end = report.firstIndex(where: { $0 == press || $0 == release }) else { return false }
                if bytes[end] == press, Array(report.prefix(while: { $0 != semicolon })) == leftButton {
                    return true
                }
                at = end + 1
            } else if bytes[at + 2] == press, at + 3 < bytes.count {
                if bytes[at + 3] == classicLeft { return true }
                at += 6
            } else {
                at += 1
            }
        }
        return false
    }
}

/// The agent of the pane this app jumped into. The console and the desk share one tmux, so a
/// pane signal may answer another screen's jump: only one that comes within `window` of this
/// app's own mark counts, once for each mark. The agent goes once the terminal is back on the
/// console for `settle`: a redraw split across reads may hide a pane's way back for a moment.
public struct PaneAgent: Sendable {
    public static let window: Duration = .seconds(1)
    public static let settle: Duration = .milliseconds(300)

    private var markedAt: ContinuousClock.Instant?
    private var inPane = false
    private var awaySince: ContinuousClock.Instant?
    public private(set) var agent: String?

    public init() {}

    /// The way back has gone from the screen, and the agent goes if it stays away for `settle`.
    public var leaving: Bool { awaySince != nil }

    public mutating func marked(at now: ContinuousClock.Instant) {
        markedAt = now
    }

    public mutating func signal(_ s: ConsoleSignal, at now: ContinuousClock.Instant) {
        guard s.pane != nil, let mark = markedAt else { return }
        markedAt = nil
        guard now - mark <= Self.window else { return }
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
