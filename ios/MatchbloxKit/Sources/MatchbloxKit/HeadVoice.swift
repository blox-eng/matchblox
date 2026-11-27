import Foundation

/// What VoiceOver's swipe up or down on the head says. A swipe pages: it moves no option, so the
/// double-tap, Enter, answers the option the agent's own mark is on.
public enum HeadVoice {
    /// What VoiceOver says after a page went `route`.
    public static func paged(_ route: HeadRoute, older: Bool) -> String {
        switch route {
        case .wheel, .scrollback: older ? "a page up" : "a page down"
        case .end: older ? "top of the scrollback" : "bottom of the output"
        case .nothing: "nothing scrolls here"
        }
    }
}
