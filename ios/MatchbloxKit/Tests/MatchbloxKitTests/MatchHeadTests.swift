import XCTest
@testable import MatchbloxKit

final class MatchHeadTests: XCTestCase {
    /// Turns the head one notch a call, 0.01 s apart, anticlockwise when `notches` is negative,
    /// through the wrap at ±π.
    private func spin(_ head: inout MatchHead, notches: Int) -> [MatchHead.Roll] {
        _ = head.turn(angle: 0, at: 0)
        return (1...abs(notches)).map { i in head.turn(angle: angle(notches: i * notches.signum()), at: Double(i) * 0.01) }
    }

    /// A shade over `notches` notches from 0, wrapped to ±π.
    private func angle(notches: Int) -> CGFloat {
        (CGFloat(notches) * MatchHead.notch * 1.0001).remainder(dividingBy: 2 * .pi)
    }

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

    func testHeatFollowsSpeed() {
        var head = MatchHead()
        _ = head.roll(dy: 1, speed: 1300)
        XCTAssertEqual(head.heat, 0.5, accuracy: 0.001)
        _ = head.roll(dy: 1, speed: 0)
        XCTAssertEqual(head.heat, 0)
        _ = head.roll(dy: 1, speed: -5200)
        XCTAssertEqual(head.heat, 1)
    }

    func testRollStrikesOnce() {
        var head = MatchHead()
        XCTAssertTrue(head.roll(dy: 1, speed: 2400).strike)
        XCTAssertTrue(head.struck)
        XCTAssertFalse(head.roll(dy: 1, speed: 2500).strike, "one rigid tap per strike")
    }

    func testFifteenDegreesIsOneNotch() {
        var head = MatchHead()
        _ = head.turn(angle: 0, at: 0)
        XCTAssertEqual(head.turn(angle: 0.27, at: 0.1).notches, 1)
        XCTAssertEqual(head.turn(angle: -0.01, at: 0.2).notches, -1)
    }

    func testAngleWrapsAtPi() {
        var head = MatchHead()
        _ = head.turn(angle: 3.1, at: 0)
        XCTAssertEqual(head.turn(angle: -3.1, at: 0.1).notches, 0, "0.08 rad clockwise, not a full turn back")
        XCTAssertEqual(head.turn(angle: -2.9, at: 0.2).notches, 1)
    }

    func testPaceSteps() {
        XCTAssertEqual([1, 10, 11, 24, 25, 48, 49, 200].map { MatchHead.pace(notchesSinceCross: $0) }, [1, 1, 3, 3, 8, 8, 20, 20])
        var head = MatchHead()
        XCTAssertEqual(spin(&head, notches: -49).map(\.lines), Array(repeating: 1, count: 10) + Array(repeating: 3, count: 14) + Array(repeating: 8, count: 24) + [20])
    }

    func testPauseResetsPace() {
        var head = MatchHead()
        _ = spin(&head, notches: -30)
        XCTAssertEqual(head.turn(angle: angle(notches: -31), at: 0.3 + 0.36).lines, 1)
    }

    func testStrikesOnceARoll() {
        var head = MatchHead()
        let rolls = spin(&head, notches: -60)
        XCTAssertEqual(rolls.indices.filter { rolls[$0].strike }, [48], "the 49th notch strikes, once")
        XCTAssertTrue(head.struck)
    }

    func testHeatFollowsPace() {
        let heat = [1, 11, 25, 49].map { n -> CGFloat in
            var head = MatchHead()
            _ = spin(&head, notches: n)
            return head.heat
        }
        zip(heat, [0.05, 0.15, 0.4, 1]).forEach { XCTAssertEqual($0, $1, accuracy: 0.001) }
    }

    func testTapRateCap() {
        var head = MatchHead()
        XCTAssertEqual([0, 0.010, 0.020, 0.034].map { head.tap(at: $0) }, [true, false, false, true])
    }

    func testEveryNotchTapsWhenSlow() {
        var head = MatchHead()
        XCTAssertEqual([0, 0.1, 0.2, 0.3].map { head.tap(at: $0) }, [true, true, true, true])
    }

    func testCoolsWithinASecond() {
        var head = MatchHead()
        _ = spin(&head, notches: 49)
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

    func testAnticlockwiseShowsOlderLines() {
        var head = MatchHead()
        _ = head.turn(angle: 0, at: 0)
        let up = head.turn(angle: -0.8, at: 0.1)
        XCTAssertEqual(up.notches, -3)
        XCTAssertEqual(up.lines, 3)
        XCTAssertEqual(head.turn(angle: 0.1, at: 0.2).lines, -3)
    }
}
