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

    func testHeatFollowsSpeed() {
        var head = MatchHead()
        _ = head.roll(dy: 1, speed: 1300)
        XCTAssertEqual(head.heat, 0.5, accuracy: 0.001)
        _ = head.roll(dy: 1, speed: 0)
        XCTAssertEqual(head.heat, 0)
        _ = head.roll(dy: 1, speed: -5200)
        XCTAssertEqual(head.heat, 1)
    }

    func testStrikesOnce() {
        var head = MatchHead()
        XCTAssertTrue(head.roll(dy: 1, speed: 2400).strike)
        XCTAssertTrue(head.struck)
        XCTAssertFalse(head.roll(dy: 1, speed: 2500).strike, "one rigid tap per strike")
    }

    func testCoolsWithinASecond() {
        var head = MatchHead()
        _ = head.roll(dy: 1, speed: 2600)
        for _ in 0..<60 { head.cool(frame: 1.0 / 60) }
        XCTAssertLessThan(head.heat, 0.05)
        XCTAssertFalse(head.struck)
    }

    /// Every notch the thumb feels moves the pane: a notch is at least one wheel step, even
    /// where a step is 5 lines (tmux), and nothing carries over to the next notch.
    func testEveryNotchSendsAStep() {
        XCTAssertEqual(WheelBurst.steps(lines: 1, linesPerStep: 5), 1)
        XCTAssertEqual(WheelBurst.steps(lines: 6, linesPerStep: 5), 2)
        XCTAssertEqual(WheelBurst.steps(lines: 10, linesPerStep: 5), 2)
        XCTAssertEqual(WheelBurst.steps(lines: -1, linesPerStep: 5), -1)
        XCTAssertEqual(WheelBurst.steps(lines: 4, linesPerStep: 1), 4)
        XCTAssertEqual(WheelBurst.steps(lines: 0, linesPerStep: 5), 0)
    }

    func testRollReportsLines() {
        var head = MatchHead()
        let roll = head.roll(dy: -30, speed: 2400)
        XCTAssertEqual(roll.notches, -3)
        XCTAssertEqual(roll.lines, -30)
    }
}
