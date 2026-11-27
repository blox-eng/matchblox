import Foundation

/// What the handle row says: what a roll does now, or why it stopped and which way to go.
public enum HeadHint: Equatable, Sendable {
    /// `hold` is the key id the owner set on the hold.
    case rest(hold: String)
    /// No menu reply yet: the notch was dropped.
    case waiting
    case option(n: Int, of: Int, text: String)
    case crossed
    case up(Int)
    case bump(HeadWalk.Bump)
    case questionChanged
    /// The pick failed for another reason, or the host never answered.
    case notSent
    /// The menu read failed, so the tap sent nothing.
    case unreadable

    /// `matchblox choose` exits with this when the menu no longer reads as it did.
    static let questionChangedStatus = 3

    public var line: String {
        switch self {
        case .rest(let hold):
            "roll: options, then the output · hold: \(KeyFace.word(of: hold) ?? hold) · BOX sets them"
        case .waiting:
            "reading the screen · roll again"
        case .option(let n, let count, let text):
            "\(n) of \(count) · \(text)"
        case .crossed:
            "past the first option · roll on to read the output"
        case .up(let lines):
            "\(lines) \(lines == 1 ? "line" : "lines") up · tap the head to come back"
        case .bump(.top):
            "top of the scrollback · roll the other way to come down"
        case .bump(.bottom):
            "bottom of the output · roll the other way to read up"
        case .bump(.lastOption):
            "last option · roll back, or tap to answer"
        case .bump(.nothing):
            "nothing scrolls here · this program has no scrollback"
        case .questionChanged:
            "the question changed · roll again"
        case .notSent:
            "no word from the host · look, then tap again"
        case .unreadable:
            "could not read the screen · tap again"
        }
    }

    /// The row after `matchblox choose` replied; nil when the host typed the pick.
    public static func afterPick(_ reply: HostReply) -> HeadHint? {
        if reply.ok { return nil }
        return reply.status == questionChangedStatus ? .questionChanged : .notSent
    }
}
