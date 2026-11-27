import XCTest
@testable import MatchbloxKit

/// The handle row always says what a roll does now, and why a roll stopped.
final class HeadHintTests: XCTestCase {
    func testRestLine() {
        var marks = HeadMarks()
        XCTAssertEqual(HeadHint.rest(hold: marks[.hold]).line, "roll: options, then the output · hold: sessions · BOX sets them")
        try? marks.set("Esc", at: .hold)
        XCTAssertEqual(HeadHint.rest(hold: marks[.hold]).line, "roll: options, then the output · hold: cancel · BOX sets them")
    }

    func testUpLine() {
        XCTAssertEqual(HeadHint.up(12).line, "12 lines up · tap the head to come back")
        XCTAssertEqual(HeadHint.up(1).line, "1 line up · tap the head to come back")
    }

    func testOptionLine() {
        XCTAssertEqual(HeadHint.option(n: 2, of: 3, text: "Yes, and open a pull request").line, "2 of 3 · Yes, and open a pull request")
    }

    func testStaleLine() {
        XCTAssertEqual(HeadHint.questionChanged.line, "the question changed · roll again")
        let stale = HostReply(output: Data("the question changed\n".utf8), ok: false, status: 3)
        XCTAssertEqual(HeadHint.afterPick(stale), .questionChanged)
    }

    func testPickReplies() {
        XCTAssertNil(HeadHint.afterPick(HostReply(output: Data(), ok: true, status: 0)))
        XCTAssertEqual(HeadHint.afterPick(HostReply(output: Data(), ok: false, status: 1)), .notSent)
        XCTAssertEqual(HeadHint.afterPick(HostReply(output: Data(), ok: false, status: nil)), .notSent)
        XCTAssertEqual(HeadHint.notSent.line, "no word from the host · look, then tap again")
    }

    func testBumpLines() {
        XCTAssertEqual(HeadHint.bump(.top).line, "top of the scrollback · roll the other way to come down")
        XCTAssertEqual(HeadHint.bump(.lastOption).line, "last option · roll back, or tap to answer")
        XCTAssertEqual(HeadHint.bump(.nothing).line, "nothing scrolls here · this program has no scrollback")
        XCTAssertEqual(HeadHint.bump(.bottom).line, "bottom of the output · roll the other way to read up")
    }

    func testWaitAndCrossLines() {
        XCTAssertEqual(HeadHint.waiting.line, "reading the screen · roll again")
        XCTAssertEqual(HeadHint.crossed.line, "past the first option · roll on to read the output")
    }
}
