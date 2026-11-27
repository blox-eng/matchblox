import XCTest
@testable import MatchbloxKit

/// VoiceOver pushes the head as a thumb does: a swipe is the push `↑` or `↓`, a double-tap is the
/// tap, and the marks and the hold are its actions.
final class HeadVoiceTests: XCTestCase {
    /// The swipe reads no screen: in a pane, in the chat, at a question or up in the output it is
    /// the same push, and the push decides what that screen does with it.
    func testSwipeUpPushesUpSwipeDownPushesDown() {
        XCTAssertEqual(HeadVoice.does(.swipeUp), .push("↑"))
        XCTAssertEqual(HeadVoice.does(.swipeDown), .push("↓"))
    }

    func testDoubleTapIsTheTap() {
        XCTAssertEqual(HeadVoice.does(.doubleTap), .tap)
    }

    func testHintSaysPushAndEnter() {
        XCTAssertEqual(HeadVoice.hint, "Swipe up or down to push. Double-tap for Enter.")
    }

    func testActionsAreTheFourMarksAndSessions() {
        XCTAssertEqual(HeadVoice.actions(for: HeadMarks()), [
            HeadVoice.Action(name: "❯ up, top mark", id: "↑"),
            HeadVoice.Action(name: "cursor, left mark", id: "←"),
            HeadVoice.Action(name: "cursor, right mark", id: "→"),
            HeadVoice.Action(name: "❯ down, bottom mark", id: "↓"),
            HeadVoice.Action(name: "sessions", id: "⇄"),
        ])
    }

    /// A `·` mark sends nothing and is no action; another key on the hold is named for the hold.
    func testActionsFollowHome() throws {
        var marks = HeadMarks()
        try marks.set("Esc", at: .left)
        try marks.set("·", at: .right)
        try marks.set("Tab", at: .hold)
        XCTAssertEqual(HeadVoice.actions(for: marks).map(\.name), [
            "❯ up, top mark", "cancel, left mark", "❯ down, bottom mark", "tab, hold",
        ])
        try marks.set("·", at: .hold)
        XCTAssertEqual(HeadVoice.actions(for: marks).map(\.name), [
            "❯ up, top mark", "cancel, left mark", "❯ down, bottom mark",
        ])
    }

    /// After a swipe VoiceOver says the line the push gave the handle row, else the key it pushed.
    func testSpokenAfterASwipe() {
        XCTAssertEqual(HeadVoice.spoken(key: "↓", said: "2 of 3 · No"), "2 of 3 · No")
        XCTAssertEqual(HeadVoice.spoken(key: "↓", said: nil), "❯ down")
        XCTAssertEqual(HeadVoice.spoken(key: "↑", said: nil), "❯ up")
    }
}
