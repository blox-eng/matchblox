import Foundation

/// Pocket's own highlight over the options of the question the console's chat shows. In the chat
/// `↑` `↓` choose a session, so a push moves this highlight instead, and a tap types the
/// highlighted option's number, which the chat takes as its answer.
public struct ChatPick: Sendable {
    public enum Push: Equatable, Sendable {
        /// The highlight is on option `n` of `of`.
        case on(n: Int, of: Int, text: String)
        /// `↑` at the first option: the push reads up the output.
        case readUp
        /// No options on the screen: the push is the chat's own arrow.
        case arrow
    }

    /// The chat answers options 1 to 9.
    static let most = 9

    private var n: Int?
    /// The options the highlight is on; another question starts it again.
    private var options: [String]?

    public init() {}

    public mutating func push(up: Bool, menu: ScreenMenu?) -> Push {
        guard let menu else {
            forget()
            return .arrow
        }
        let at = start(menu)
        if up, at == 1 { return .readUp }
        let next = up ? at - 1 : min(at + 1, min(menu.count, Self.most))
        n = next
        return .on(n: next, of: menu.count, text: menu.options[next - 1])
    }

    /// The tap's keys: the highlighted option's number twice, as the chat's first digit marks an
    /// option and the same digit again sends it. Nil, so Enter, with no options on the screen.
    public mutating func tap(menu: ScreenMenu?) -> [UInt8]? {
        guard let menu else {
            forget()
            return nil
        }
        let digit = UInt8(ascii: "0") + UInt8(start(menu))
        forget()
        return [digit, digit]
    }

    /// The highlighted option, from the agent's own `❯` when the question is new.
    private mutating func start(_ menu: ScreenMenu) -> Int {
        if options != menu.options {
            options = menu.options
            n = nil
        }
        let at = n ?? min(menu.chosen, Self.most)
        n = at
        return at
    }

    private mutating func forget() {
        n = nil
        options = nil
    }
}
