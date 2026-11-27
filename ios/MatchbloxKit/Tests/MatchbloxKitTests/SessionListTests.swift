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
          {"n": 7, "name": "hearth-api"}
        ]
        """.utf8)
        XCTAssertEqual(SessionList.decode(list), [
            SessionRow(n: 3, name: "flint-auth", word: "asks"),
            SessionRow(n: 7, name: "hearth-api", word: ""),
        ])
        XCTAssertEqual(SessionList.decode(Data("[]".utf8)), [])
        XCTAssertNil(SessionList.decode(Data(#"{"n": 1}"#.utf8)))
    }
}
