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

    /// Only an ok reply with no options is no menu; a failed or rejected reply is a screen Pocket could not read.
    func testReading() {
        let three = ["a", "b", "c"]
        func reply(_ data: Data, ok: Bool = true) -> HostReply { HostReply(output: data, ok: ok, status: ok ? 0 : 1) }
        XCTAssertEqual(MenuSight.reading(reply(Data(#"{"options":[],"chosen":0,"sign":""}"#.utf8))), .none)
        XCTAssertEqual(MenuSight.reading(reply(json(options: three, chosen: 2))), .menu(MenuSight.decode(json(options: three, chosen: 2))!))
        XCTAssertEqual(MenuSight.reading(reply(json(options: three, chosen: 0))), .unreadable, "options with no mark")
        XCTAssertEqual(MenuSight.reading(reply(json(options: (1...12).map { "option \($0)" }, chosen: 1))), .unreadable)
        XCTAssertEqual(MenuSight.reading(reply(Data("not json".utf8))), .unreadable)
        XCTAssertEqual(MenuSight.reading(reply(Data(), ok: false)), .unreadable)
        XCTAssertEqual(MenuSight.reading(HostReply(output: Data(), ok: false)), .unreadable, "no reply")
        XCTAssertEqual(MenuSight.reading(reply(Data(#"{"options":[],"chosen":0,"sign":""}"#.utf8), ok: false)), .unreadable)
    }
}
