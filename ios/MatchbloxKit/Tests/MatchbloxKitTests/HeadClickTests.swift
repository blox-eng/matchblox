import XCTest
@testable import MatchbloxKit

/// A tap on the head sends Enter, which can answer an agent: only a tap on a still head counts.
final class HeadClickTests: XCTestCase {
    func testTouchThatStopsACoastIsNoClick() {
        var click = HeadClick()
        click.began(at: 10, coasting: true, lastRoll: 9.9)
        XCTAssertFalse(click.isClick(endedAt: 10.1))
    }

    func testShortLiftAfterARollIsNoClick() {
        var click = HeadClick()
        click.began(at: 10, coasting: false, lastRoll: 9.95)
        XCTAssertFalse(click.isClick(endedAt: 10.08))
    }

    func testRollDuringTheTouchIsNoClick() {
        var click = HeadClick()
        click.began(at: 10, coasting: false, lastRoll: nil)
        click.moved(by: 4)
        click.moved(by: -3)
        XCTAssertFalse(click.isClick(endedAt: 10.1), "7 pt of travel either way is a roll")
    }

    func testLongPressIsNoClick() {
        var click = HeadClick()
        click.began(at: 10, coasting: false, lastRoll: nil)
        XCTAssertFalse(click.isClick(endedAt: 10.5))
    }

    func testStillTapIsAClick() {
        var click = HeadClick()
        click.began(at: 10, coasting: false, lastRoll: 9.0)
        click.moved(by: 2)
        XCTAssertTrue(click.isClick(endedAt: 10.12))
    }

    func testMiddleLift() {
        XCTAssertEqual(HeadClick.lift(after: 0.2, rolled: false, coasting: false), .enter)
        XCTAssertEqual(HeadClick.lift(after: 0.4, rolled: false, coasting: false), .nothing)
        XCTAssertEqual(HeadClick.lift(after: 0.2, rolled: true, coasting: false), .nothing)
        XCTAssertEqual(HeadClick.lift(after: 0.2, rolled: false, coasting: true), .nothing)
    }

    func testHoldDue() {
        XCTAssertFalse(HeadClick.holdDue(after: 0.59))
        XCTAssertTrue(HeadClick.holdDue(after: 0.6), "the hold starts during the press, not on the lift")
    }

    func testLiftAfterHold() {
        XCTAssertEqual(HeadClick.lift(after: 0.6, rolled: false, coasting: false), .hold)
        XCTAssertEqual(HeadClick.lift(after: 2, rolled: false, coasting: false), .hold)
    }
}
