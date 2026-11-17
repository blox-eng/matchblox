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
}
