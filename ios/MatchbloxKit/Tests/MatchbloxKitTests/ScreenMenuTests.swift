import XCTest
@testable import MatchbloxKit

/// The menu at the foot of the visible screen, read on the phone with the host's rules.
final class ScreenMenuTests: XCTestCase {
    private let question = [
        "● One choice is yours before I wire the rotation.",
        "",
        "Should a refresh also rotate the token?",
        "",
        "❯ 1. Rotate it",
        "     the old one stops at once",
        "  2. Rotate, keep the old one a minute",
        "  3. Keep the token",
        "",
        "Enter to select · ↑/↓ to navigate · Esc to cancel",
        "",
    ]

    func testMarkedFirstOfThree() {
        let menu = ScreenMenu.read(lines: question)
        XCTAssertEqual(menu?.chosen, 1)
        XCTAssertEqual(menu?.count, 3)
        XCTAssertEqual(menu?.text, "Rotate it the old one stops at once", "a wrapped line joins its option")
        XCTAssertEqual(menu?.options[1], "Rotate, keep the old one a minute")
    }

    func testTheMarkMoves() {
        var lines = question
        lines[4] = "  1. Rotate it"
        lines[6] = "❯ 2. Rotate, keep the old one a minute"
        XCTAssertEqual(ScreenMenu.read(lines: lines)?.chosen, 2)
        lines[6] = "› 2. Rotate, keep the old one a minute"
        XCTAssertEqual(ScreenMenu.read(lines: lines)?.chosen, 2)
    }

    /// Surface: an agent's tools. A numbered list in a reply has other text under it: it is no
    /// menu, so `↑` there is a plain arrow and never reads up.
    func testAListWithTextUnderItIsNoMenu() {
        let reply = [
            "● Two ways to do it:",
            "❯ 1. Rotate it",
            "  2. Keep the token",
            "Tell me which one you want.",
            "",
            "╭──────────────────╮",
            "│ >                │",
            "╰──────────────────╯",
            "  ? for shortcuts",
        ]
        XCTAssertNil(ScreenMenu.read(lines: reply))
        XCTAssertNil(ScreenMenu.read(lines: Array(reply.prefix(4)) + ["And one more line."]),
                     "two lines of text under the list")
        XCTAssertNil(ScreenMenu.read(lines: Array(reply.prefix(3)) + ["╭────╮", "│ >  │", "╰────╯"]),
                     "the agent's empty input under the list")
    }

    func testAMarkInsideAnOptionsTextIsNotTheMark() {
        var lines = question
        lines[7] = "  3. Keep the token: press ❯ 9. to skip"
        let menu = ScreenMenu.read(lines: lines)
        XCTAssertEqual(menu?.chosen, 1)
        XCTAssertEqual(menu?.count, 3)
        XCTAssertEqual(menu?.options[2], "Keep the token: press ❯ 9. to skip")
    }

    func testNoMarkIsNoMenu() {
        var lines = question
        lines[4] = "  1. Rotate it"
        XCTAssertNil(ScreenMenu.read(lines: lines))
    }

    func testOptionsOutOfOrderAreNoMenu() {
        var lines = question
        lines[7] = "  4. Keep the token"
        XCTAssertNil(ScreenMenu.read(lines: lines))
    }

    /// A box round the menu, rules between its options, and one status line closing the screen
    /// (a jumped tmux pane's, or the demo's key line) still read.
    func testBoxRulesAndAStatusLine() {
        let boxed = [
            "╭──────────────────────────────────╮",
            "│ Do you want to run go test?      │",
            "│ ❯ 1. Yes                         │",
            "│   2. No, and tell Claude         │",
            "╰──────────────────────────────────╯",
            "   Esc to cancel",
        ]
        XCTAssertEqual(ScreenMenu.read(lines: boxed)?.count, 2)
        var lines = question
        lines.insert("────────────────────", at: 7)
        lines += ["", " type 2 to answer · ctrl+a m back"]
        let menu = ScreenMenu.read(lines: lines)
        XCTAssertEqual(menu?.chosen, 1)
        XCTAssertEqual(menu?.count, 3)
        XCTAssertNil(ScreenMenu.read(lines: lines + ["[0] 0:claude*"]), "two lines under the menu are text, not a status line")
    }
}
