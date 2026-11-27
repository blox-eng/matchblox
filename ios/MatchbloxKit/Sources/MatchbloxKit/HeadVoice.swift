import Foundation

/// The head as VoiceOver works it. A swipe is a push, never a page, and reads no screen: the
/// dock sends it down the path a thumb's push takes, so in the chat it moves Pocket's highlight and
/// in a pane it sends the arrow. A double-tap is the tap.
public enum HeadVoice {
    public enum Gesture: CaseIterable, Sendable {
        /// VoiceOver's increment.
        case swipeUp
        /// VoiceOver's decrement.
        case swipeDown
        case doubleTap
    }

    public enum Does: Equatable, Sendable {
        /// A push toward this mark's key.
        case push(String)
        case tap
    }

    /// A custom action: a mark's key, named for VoiceOver.
    public struct Action: Equatable, Sendable {
        public let name: String
        public let id: String

        public init(name: String, id: String) {
            self.name = name
            self.id = id
        }
    }

    public static let hint = "Swipe up or down to push. Double-tap for Enter."

    private static let sessions = "⇄"
    private static let nothing = "·"

    public static func does(_ gesture: Gesture) -> Does {
        switch gesture {
        case .swipeUp: .push("↑")
        case .swipeDown: .push("↓")
        case .doubleTap: .tap
        }
    }

    /// The four marks and the hold, named from the marks home set: the word, then the place, as
    /// left and right are both `cursor`. A mark that sends nothing (`·`) is no action.
    public static func actions(for marks: HeadMarks) -> [Action] {
        var named: [Action] = []
        for place in [HeadMarks.Place.top, .left, .right, .bottom] {
            let id = marks[place]
            guard id != nothing else { continue }
            named.append(Action(name: "\(KeyFace.word(of: id) ?? id), \(place.rawValue) mark", id: id))
        }
        let hold = marks[.hold]
        if hold == sessions {
            named.append(Action(name: KeyFace.word(of: hold) ?? hold, id: hold))
        } else if hold != nothing {
            named.append(Action(name: "\(KeyFace.word(of: hold) ?? hold), hold", id: hold))
        }
        return named
    }

    /// What VoiceOver says after a swipe pushed `key`: the line the push gave the handle row, else
    /// the key's word.
    public static func spoken(key: String, said: String?) -> String {
        said ?? KeyFace.word(of: key) ?? key
    }
}
