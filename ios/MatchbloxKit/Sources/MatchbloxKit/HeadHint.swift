import Foundation

/// What the handle row says: what the head does now, or why a roll stopped and which way to go.
public enum HeadHint: Equatable, Sendable {
    /// `hold` is the key id the owner set on the hold.
    case rest(hold: String)
    /// The agent's mark, or in the chat Pocket's highlight, is on option `n` of `count`, after a push.
    case option(n: Int, of: Int, text: String)
    case up(Int)
    case bump(HeadScroll.Bump)

    public var line: String {
        switch self {
        case .rest(let hold):
            "roll: the output · hold: \(KeyFace.word(of: hold) ?? hold) · BOX sets them"
        case .option(let n, let count, let text):
            "\(n) of \(count) · \(text)"
        case .up(let lines):
            "\(lines) \(lines == 1 ? "line" : "lines") up · push ↓ or tap to come back"
        case .bump(.top):
            "top of the scrollback · roll the other way to come down"
        case .bump(.bottom):
            "bottom of the output · roll the other way to read up"
        case .bump(.nothing):
            "nothing scrolls here · this program has no scrollback"
        }
    }
}
