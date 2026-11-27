import XCTest
@testable import MatchbloxKit

final class SessionListTests: XCTestCase {
    func testSessionRows() {
        let long = String(repeating: "x", count: 201)
        let list = Data("""
        [
          {"n": 3, "name": "flint-auth", "pane": "%2", "word": "asks", "status": "idle"},
          {"name": "wick-mobile", "pane": "%7", "word": "idle"},
          {"n": "4", "name": "typed", "word": "idle"},
          {"n": 5, "name": "\(long)", "word": "busy"},
          {"n": 6, "name": "", "word": "busy"},
          {"n": 0, "name": "zero", "word": "busy"},
          {"n": 7, "name": "hearth-api"},
          {"n": 8, "name": "odd-pane", "pane": "%2;rm", "word": "idle"},
          {"n": 9, "name": "named-pane", "pane": "%3@matchblox", "word": "idle"},
          {"n": 10, "name": "typed-pane", "pane": 2, "word": "idle"}
        ]
        """.utf8)
        XCTAssertEqual(SessionList.decode(list), [
            SessionRow(n: 3, name: "flint-auth", word: "asks", pane: "%2"),
            SessionRow(n: 7, name: "hearth-api", word: ""),
            SessionRow(n: 8, name: "odd-pane", word: "idle"),
            SessionRow(n: 9, name: "named-pane", word: "idle"),
            SessionRow(n: 10, name: "typed-pane", word: "idle"),
        ])
        XCTAssertEqual(SessionList.decode(Data("[]".utf8)), [])
        XCTAssertNil(SessionList.decode(Data(#"{"n": 1}"#.utf8)))
    }
}
