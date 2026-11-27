import Foundation

/// What VoiceOver's swipe up or down on the head does. At a menu it moves Pocket's highlight one
/// option and stops at the first and the last: unlike a roll it never crosses into the output, so
/// the double-tap after it answers the option it named. With no menu, or up in the output, it pages.
public enum HeadVoice {
    public enum Step: Equatable, Sendable {
        case option(n: Int, of: Int, text: String)
        case first
        case last
        /// A screen of lines; `older`: toward older lines.
        case page(older: Bool)

        /// What VoiceOver says after the swipe; a page says `paged`.
        public var line: String? {
            switch self {
            case .option(let n, let count, let text): HeadHint.option(n: n, of: count, text: text).line
            case .first: "first option · double-tap to answer"
            case .last: "last option · double-tap to answer"
            case .page: nil
            }
        }
    }

    /// `older` is VoiceOver's decrement, a swipe down: back up the options, or toward older lines.
    /// `menu` is the menu the head read last, nil when none.
    public static func swipe(_ walk: inout HeadWalk, older: Bool, menu: MenuSight?) -> Step {
        guard let menu, walk.linesUp == 0 else { return .page(older: older) }
        if older, walk.highlight <= 1 { return .first }
        walk.menu(menu)
        walk.lift()
        switch walk.notch(older ? .up : .down, route: .nothing, at: 0) {
        case .highlight(let n):
            return .option(n: n, of: menu.options.count, text: menu.options.first { $0.n == n }?.text ?? "")
        default:
            // At a menu, a notch that moves no highlight is the bump past the last option.
            return .last
        }
    }

    /// What VoiceOver says after a page went `route`.
    public static func paged(_ route: HeadRoute, older: Bool) -> String {
        switch route {
        case .wheel, .scrollback: older ? "a page up" : "a page down"
        case .end: older ? "top of the scrollback" : "bottom of the output"
        case .nothing: "nothing scrolls here"
        }
    }
}
