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

    /// The console's chat with flint-auth's question: the agent's screen, then the chat's own
    /// line, its rule and its key line. The keys hint wraps at the phone's width.
    private let chat = [
        " ◂ flint-auth ✻ asks · #205 ci     ctx 41%",
        " ● The refresh flow works and the tests",
        "   pass. One choice is yours before I",
        "   wire the rotation.",
        "",
        " ───────────────────────────────────────────",
        "  ☐ Refresh",
        "",
        " Should a refresh also rotate the token?",
        "",
        " ❯ 1. Rotate it",
        "      the old one stops at once",
        "   2. Rotate, keep the old one a minute",
        "      no request fails mid-flight",
        "   3. Keep the token",
        "      it lives until it expires",
        "   4. Type something.",
        " ───────────────────────────────────────────",
        "   5. Chat about this",
        "",
        " Enter to select · ↑/↓ to navigate · Esc to",
        " cancel",
        " ›                     ⏎ send to flint-auth",
        "────────────────────────────────────────────",
        " writing to flint-auth · ◂ list · ⏎ send",
    ]

    /// In the chat the menu sits over the chat's own line: read as a pane, the chat's line is
    /// text under a list, so the chat has its own read. Each option is the number it shows.
    func testTheChatsQuestion() {
        XCTAssertNil(ScreenMenu.read(lines: chat), "the chat's line under the options reads as text")
        let menu = ScreenMenu.read(chat: chat)
        XCTAssertEqual(menu?.count, 5)
        XCTAssertEqual(menu?.chosen, 1)
        XCTAssertEqual(menu?.options[1], "Rotate, keep the old one a minute no request fails mid-flight", "option 2 is the one numbered 2.")
        XCTAssertEqual(menu?.options[4], "Chat about this")
        var typed = chat
        typed[22] = " › ok"
        XCTAssertEqual(ScreenMenu.read(chat: typed)?.count, 5, "text in the chat's line is no part of the menu")
    }

    /// Surface: an agent's tools. A chat with no question, or a screen with no chat line, reads
    /// no menu, so a push there is the chat's own arrow.
    func testAChatWithNoQuestionIsNoMenu() {
        let reply = [
            " ◂ spark-docs · done",
            " ● Got it. I am working on that.",
            "",
            " ›⏎ send to spark-docs",
            "────────────────────────────────────────────",
            " writing to spark-docs · ◂ list · ⏎ send",
        ]
        XCTAssertNil(ScreenMenu.read(chat: reply))
        XCTAssertNil(ScreenMenu.read(chat: Array(chat[..<22])), "no chat line: no chat to read")
    }
}
