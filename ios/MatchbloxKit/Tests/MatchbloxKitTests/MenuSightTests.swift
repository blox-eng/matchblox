import XCTest
@testable import MatchbloxKit

final class MenuSightTests: XCTestCase {
    private func json(options: [String], chosen: Int, sign: String? = "0123456789abcdef") -> Data {
        let rows = options.enumerated().map { ["n": $0.offset + 1, "text": $0.element] as [String: Any] }
        var body: [String: Any] = ["options": rows, "chosen": chosen]
        if let sign { body["sign"] = sign }
        return try! JSONSerialization.data(withJSONObject: body)
    }

    func testMenuSight() {
        let three = ["Rotate it", "Rotate, keep the old one a minute", "Keep the token"]
        XCTAssertEqual(
            MenuSight.decode(json(options: three, chosen: 2)),
            MenuSight(options: three.enumerated().map { MenuSight.Option(n: $0.offset + 1, text: $0.element) },
                      chosen: 2, sign: "0123456789abcdef")
        )
    }

    func testMenuHostile() {
        let three = ["a", "b", "c"]
        XCTAssertNil(MenuSight.decode(json(options: three, chosen: 0)))
        XCTAssertNil(MenuSight.decode(json(options: three, chosen: 4)))
        XCTAssertNil(MenuSight.decode(json(options: (1...12).map { "option \($0)" }, chosen: 1)))
        XCTAssertNil(MenuSight.decode(json(options: three, chosen: 1, sign: nil)))
        XCTAssertNil(MenuSight.decode(json(options: three, chosen: 1, sign: "")))
        XCTAssertNil(MenuSight.decode(json(options: three, chosen: 1, sign: "'; rm -rf / '")))
        XCTAssertNil(MenuSight.decode(json(options: ["a", String(repeating: "x", count: 501)], chosen: 1)))
        XCTAssertNil(MenuSight.decode(json(options: ["a", ""], chosen: 1)))
        XCTAssertNil(MenuSight.decode(json(options: [], chosen: 0)))
        XCTAssertNil(MenuSight.decode(Data(#"{"options":[{"n":2,"text":"b"},{"n":1,"text":"a"}],"chosen":1,"sign":"ab"}"#.utf8)))
        XCTAssertNil(MenuSight.decode(Data("not json".utf8)))
        XCTAssertNotNil(MenuSight.decode(json(options: ["a", String(repeating: "x", count: 500)], chosen: 1)))
    }
}
