import XCTest
@testable import MatchbloxKit

/// Pocket's highlight over the options of the question the console's chat shows.
final class ChatPickTests: XCTestCase {
    private func menu(_ texts: [String], chosen: Int = 1) -> ScreenMenu {
        ScreenMenu(options: texts, chosen: chosen)
    }

    private let three = ["Rotate it", "Rotate, keep", "Keep the token"]

    /// Surface: an agent's tools. A push moves only Pocket's highlight: it starts at the agent's
    /// own mark, stops at the last option, and `↑` at the first reads up the output.
    func testPushMovesTheHighlight() {
        var pick = ChatPick()
        XCTAssertEqual(pick.push(up: false, menu: menu(three)), .on(n: 2, of: 3, text: "Rotate, keep"))
        XCTAssertEqual(pick.push(up: false, menu: menu(three)), .on(n: 3, of: 3, text: "Keep the token"))
        XCTAssertEqual(pick.push(up: false, menu: menu(three)), .on(n: 3, of: 3, text: "Keep the token"), "the last option stops it")
        XCTAssertEqual(pick.push(up: true, menu: menu(three)), .on(n: 2, of: 3, text: "Rotate, keep"))
        XCTAssertEqual(pick.push(up: true, menu: menu(three)), .on(n: 1, of: 3, text: "Rotate it"))
        XCTAssertEqual(pick.push(up: true, menu: menu(three)), .readUp, "↑ at the first option reads up")

        var marked = ChatPick()
        XCTAssertEqual(marked.push(up: false, menu: menu(three, chosen: 2)), .on(n: 3, of: 3, text: "Keep the token"), "it starts at the agent's ❯")
    }

    /// A tap types the highlighted option's number twice: the chat's first digit marks it, the
    /// second sends it. A tap with no push answers the agent's own `❯`.
    func testTapTypesTheNumber() {
        var pick = ChatPick()
        _ = pick.push(up: false, menu: menu(three))
        XCTAssertEqual(pick.tap(menu: menu(three)), Array("22".utf8))
        XCTAssertEqual(pick.tap(menu: menu(three, chosen: 3)), Array("33".utf8), "a tap with no push answers the ❯")
    }

    /// Surface: an agent's tools. With no options on screen a push is the chat's own arrow and a
    /// tap is Enter; a new question starts the highlight again at its `❯`.
    func testNoMenuAndANewQuestion() {
        var pick = ChatPick()
        XCTAssertEqual(pick.push(up: false, menu: nil), .arrow)
        XCTAssertNil(pick.tap(menu: nil))
        _ = pick.push(up: false, menu: menu(three))
        XCTAssertEqual(pick.push(up: false, menu: menu(["Yes", "No"])), .on(n: 2, of: 2, text: "No"), "a new question starts at its ❯")
        _ = pick.push(up: false, menu: nil)
        XCTAssertEqual(pick.tap(menu: menu(["Yes", "No"])), Array("11".utf8), "a screen with no menu forgot the highlight")
    }

    /// The chat answers 1 to 9: the highlight never goes past option 9.
    func testNineAtMost() {
        let twelve = (1...12).map { "option \($0)" }
        var pick = ChatPick()
        for _ in 0..<12 { _ = pick.push(up: false, menu: menu(twelve)) }
        XCTAssertEqual(pick.push(up: false, menu: menu(twelve)), .on(n: 9, of: 12, text: "option 9"))
        XCTAssertEqual(pick.tap(menu: menu(twelve)), Array("99".utf8))
    }
}
