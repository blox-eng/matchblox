import XCTest
@testable import MatchbloxKit

final class TerminalLinkTests: XCTestCase {
    /// The URL under the first cell of `word` in `row`.
    private func url(_ row: String, at word: String) -> String? {
        let cells = Array(row)
        let col = cells.indices.first { String(cells[$0...]).hasPrefix(word) }!
        return TerminalLink.url(in: cells, at: col)
    }

    func testAURLInASentence() {
        let row = "see https://kindle-web.example/pull/42 for it"
        XCTAssertEqual(url(row, at: "https"), "https://kindle-web.example/pull/42")
        XCTAssertEqual(url(row, at: "kindle"), "https://kindle-web.example/pull/42")
        XCTAssertEqual(url(row, at: "42"), "https://kindle-web.example/pull/42")
        XCTAssertNil(url(row, at: "see"))
        XCTAssertNil(url(row, at: "for"))
    }

    func testTrailingPunctuationIsTheSentences() {
        XCTAssertEqual(url("Open http://kindle-web.local:8080/.", at: "kindle"), "http://kindle-web.local:8080/")
        XCTAssertEqual(url("at https://kindle-web.example/a, then", at: "kindle"), "https://kindle-web.example/a")
        XCTAssertEqual(url("'https://kindle-web.example/a';", at: "kindle"), "https://kindle-web.example/a")
        XCTAssertNil(url("see https://kindle-web/a.", at: "."))
    }

    func testABracketStaysOnlyWithItsPair() {
        XCTAssertEqual(url("(see https://kindle-web.example/a)", at: "kindle"), "https://kindle-web.example/a")
        XCTAssertEqual(url("[docs](https://kindle-web.example/docs).", at: "kindle"), "https://kindle-web.example/docs")
        XCTAssertEqual(url("https://kindle-web.example/wiki/Ink_(film)", at: "kindle"), "https://kindle-web.example/wiki/Ink_(film)")
        XCTAssertNil(url("(https://kindle-web.example/a)", at: "("))
    }

    func testABoxLineOrANulEndsTheURL() {
        XCTAssertEqual(url("│ https://kindle-web.example/a│", at: "kindle"), "https://kindle-web.example/a")
        XCTAssertEqual(url("https://kindle-web.example/a\0\0", at: "kindle"), "https://kindle-web.example/a")
        XCTAssertEqual(url("url=https://kindle-web.example/a", at: "kindle"), "https://kindle-web.example/a")
    }

    func testOnlyWebURLsWithAHost() {
        XCTAssertNil(url("ftp://kindle-web.example/a", at: "kindle"))
        XCTAssertNil(url("see https:// here", at: "https"))
        XCTAssertNil(url("kindle-web.example/a", at: "kindle"))
        XCTAssertEqual(url("HTTPS://kindle-web.example", at: "kindle"), "HTTPS://kindle-web.example")
    }

    func testAColumnOffTheRowIsNoURL() {
        let cells = Array("https://kindle-web.example")
        XCTAssertNil(TerminalLink.url(in: cells, at: -1))
        XCTAssertNil(TerminalLink.url(in: cells, at: cells.count))
        XCTAssertNil(TerminalLink.url(in: [], at: 0))
    }

    func testOnlyAWebLinkOpens() {
        XCTAssertTrue(TerminalLink.opens("https://kindle-web.example"))
        XCTAssertTrue(TerminalLink.opens("HTTP://kindle-web.local"))
        XCTAssertFalse(TerminalLink.opens("file:///etc/hosts"))
        XCTAssertFalse(TerminalLink.opens("sms:1234"))
        XCTAssertFalse(TerminalLink.opens("javascript:alert(1)"))
        XCTAssertFalse(TerminalLink.opens("not a link"))
    }

    /// An agent draws a long URL over several rows, each after a line break and an indent.
    private let wrapped = [
        "   https://accounts.kindle-web.example/auth?redirect_uri=https%3A%2F",
        "     %2Fsdk.kindle-web.example%2Fcode&scope=openid+https%3A%2F%2Fwww",
        "     .kindle-web.example%2Fauth&state=Bkxx",
        "",
        "  Then paste the code here.",
    ].map { Array($0.padding(toLength: 70, withPad: " ", startingAt: 0)) }
    private let whole = "https://accounts.kindle-web.example/auth?redirect_uri=https%3A%2F%2Fsdk.kindle-web.example%2Fcode&scope=openid+https%3A%2F%2Fwww.kindle-web.example%2Fauth&state=Bkxx"

    func testATapOnAnyRowOfAWrappedURLGivesItWhole() {
        XCTAssertEqual(TerminalLink.url(in: wrapped, row: 0, col: 5), whole)
        XCTAssertEqual(TerminalLink.url(in: wrapped, row: 1, col: 10), whole)
        XCTAssertEqual(TerminalLink.url(in: wrapped, row: 2, col: 8), whole)
        XCTAssertNil(TerminalLink.url(in: wrapped, row: 1, col: 2), "the indent is no part of it")
        XCTAssertNil(TerminalLink.url(in: wrapped, row: 4, col: 4))
    }

    func testAURLThatEndsShortOfTheEdgeEndsThere() {
        let rows = ["see https://kindle-web.example/a", "  next"].map { Array($0.padding(toLength: 70, withPad: " ", startingAt: 0)) }
        XCTAssertEqual(TerminalLink.url(in: rows, row: 0, col: 10), "https://kindle-web.example/a")
        XCTAssertNil(TerminalLink.url(in: rows, row: 1, col: 3))
    }

    func testAWordAfterTheIndentGoesOnOnlyAlone() {
        let rows = ["  https://kindle-web.example/aaaa", "  bbbb and more"].map { Array($0) }
        XCTAssertEqual(TerminalLink.url(in: rows, row: 0, col: 5), "https://kindle-web.example/aaaa")
        let next = ["  https://kindle-web.example/aaaa", "  https://kindle-web.example/b"].map { Array($0) }
        XCTAssertEqual(TerminalLink.url(in: next, row: 0, col: 5), "https://kindle-web.example/aaaa")
        XCTAssertEqual(TerminalLink.url(in: next, row: 1, col: 5), "https://kindle-web.example/b")
    }

    func testASelectionGivesItsURLWhole() {
        let text = "   https://accounts.kindle-web.example/auth?redirect_uri=https%3A%2F\n     %2Fsdk.kindle-web.example%2Fcode&scope=openid+https%3A%2F%2Fwww\n     .kindle-web.example%2Fauth&state=Bkxx\n\n  Then paste the code here."
        XCTAssertEqual(TerminalLink.url(inSelection: text), whole)
    }

    func testASelectionDropsTheSidebar() {
        let text = """
         mbux      busy │   https://kindle-web.example/a%2F
         mbcopy    idle │     %2Fb&c=d
                        │
        """
        XCTAssertEqual(TerminalLink.url(inSelection: text), "https://kindle-web.example/a%2F%2Fb&c=d")
    }

    func testASelectionKeepsTheWordsAfterItsURLOut() {
        XCTAssertEqual(TerminalLink.url(inSelection: "open https://kindle-web.example/a.\nthen sign in"), "https://kindle-web.example/a")
        XCTAssertEqual(TerminalLink.url(inSelection: "https://kindle-web.example/a then\n  more"), "https://kindle-web.example/a")
        XCTAssertEqual(TerminalLink.url(inSelection: "https://kindle-web.example/a\nhttps://kindle-web.example/b"), "https://kindle-web.example/a")
        XCTAssertNil(TerminalLink.url(inSelection: "no link here"))
        XCTAssertNil(TerminalLink.url(inSelection: ""))
    }
}
