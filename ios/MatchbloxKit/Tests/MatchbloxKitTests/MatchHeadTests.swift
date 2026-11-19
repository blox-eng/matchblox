import XCTest
@testable import MatchbloxKit

final class MatchHeadTests: XCTestCase {
    func testTenPointsIsOneNotch() {
        var head = MatchHead()
        XCTAssertEqual(head.roll(dy: 25, speed: 100).notches, 2)
        XCTAssertEqual(head.roll(dy: 5, speed: 100).notches, 1, "the 5 pt left over carries")
    }

    func testUpRollIsUpNotches() {
        var head = MatchHead()
        XCTAssertEqual(head.roll(dy: -20, speed: 100).notches, -2)
    }

    func testLinesPerNotch() {
        XCTAssertEqual(MatchHead.lines(atSpeed: 0), 1)
        XCTAssertEqual(MatchHead.lines(atSpeed: 250), 1)
        XCTAssertEqual(MatchHead.lines(atSpeed: 2400), 10)
        XCTAssertEqual(MatchHead.lines(atSpeed: -5000), 10)
        var last = 1
        for speed in stride(from: 250, through: 2400, by: 50) {
            let lines = MatchHead.lines(atSpeed: CGFloat(speed))
            XCTAssertGreaterThanOrEqual(lines, last)
            last = lines
        }
    }

    func testTapRateCap() {
        var head = MatchHead()
        XCTAssertEqual([0, 0.010, 0.020, 0.034].map { head.tap(at: $0) }, [true, false, false, true])
    }

    func testEveryNotchTapsWhenSlow() {
        var head = MatchHead()
        XCTAssertEqual([0, 0.1, 0.2, 0.3].map { head.tap(at: $0) }, [true, true, true, true])
    }

    func testRollReportsLines() {
        var head = MatchHead()
        let roll = head.roll(dy: -30, speed: 2400)
        XCTAssertEqual(roll.notches, -3)
        XCTAssertEqual(roll.lines, -30)
    }
}
