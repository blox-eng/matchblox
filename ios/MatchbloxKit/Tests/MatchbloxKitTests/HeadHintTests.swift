import XCTest
@testable import MatchbloxKit

/// The handle row always says what the head does now, and why a roll stopped.
final class HeadHintTests: XCTestCase {
    func testRestLine() {
        var marks = HeadMarks()
        XCTAssertEqual(HeadHint.rest(hold: marks[.hold]).line, "roll: the output · hold: sessions · BOX sets them")
        try? marks.set("Esc", at: .hold)
        XCTAssertEqual(HeadHint.rest(hold: marks[.hold]).line, "roll: the output · hold: cancel · BOX sets them")
    }

    func testUpLine() {
        XCTAssertEqual(HeadHint.up(12).line, "12 lines up · push ↓ or tap to come back")
        XCTAssertEqual(HeadHint.up(1).line, "1 line up · push ↓ or tap to come back")
    }

    func testOptionLine() {
        XCTAssertEqual(HeadHint.option(n: 2, of: 3, text: "Yes, and open a pull request").line, "2 of 3 · Yes, and open a pull request")
    }

    func testBumpLines() {
        XCTAssertEqual(HeadHint.bump(.top).line, "top of the scrollback · roll the other way to come down")
        XCTAssertEqual(HeadHint.bump(.nothing).line, "nothing scrolls here · this program has no scrollback")
        XCTAssertEqual(HeadHint.bump(.bottom).line, "bottom of the output · roll the other way to read up")
    }
}
