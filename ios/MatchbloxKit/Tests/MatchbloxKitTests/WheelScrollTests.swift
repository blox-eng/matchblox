import XCTest
@testable import MatchbloxKit

final class WheelScrollTests: XCTestCase {
    func testOneStepForEachCellOfTravel() {
        var wheel = WheelScroll(cellHeight: 16)
        XCTAssertEqual(wheel.move(15), [])
        XCTAssertEqual(wheel.move(1), [.up])
        XCTAssertEqual(wheel.move(48), [.up, .up, .up])
    }

    func testFingerUpIsWheelDown() {
        var wheel = WheelScroll(cellHeight: 16)
        XCTAssertEqual(wheel.move(-33), [.down, .down])
    }

    func testRemainderCarriesAndTurns() {
        var wheel = WheelScroll(cellHeight: 16)
        XCTAssertEqual(wheel.move(10), [])
        XCTAssertEqual(wheel.move(-12), [])
        XCTAssertEqual(wheel.move(-14), [.down])
    }

    func testNoCellHeightSendsNothing() {
        var wheel = WheelScroll(cellHeight: 0)
        XCTAssertEqual(wheel.move(100), [])
    }

    func testWheelFlags() {
        XCTAssertEqual(WheelScroll.Step.up.flags, 64)
        XCTAssertEqual(WheelScroll.Step.down.flags, 65)
    }

    func testCoastFollowsTheFlick() {
        XCTAssertEqual(WheelScroll.coast(velocity: 1000, after: 0), 0)
        let early = WheelScroll.coast(velocity: 1000, after: 0.1)
        let late = WheelScroll.coast(velocity: 1000, after: 0.4)
        XCTAssertGreaterThan(early, 0)
        XCTAssertGreaterThan(late, early)
        XCTAssertLessThan(late, 1000 * 0.4)
        XCTAssertEqual(WheelScroll.coast(velocity: -1000, after: 0.4), -late, accuracy: 0.001)
    }

    func testCoastSlowsDown() {
        let first = WheelScroll.coast(velocity: 1000, after: 0.1)
        let second = WheelScroll.coast(velocity: 1000, after: 0.2) - first
        XCTAssertLessThan(second, first)
    }

    func testCoastStopsAtHalfASecond() {
        XCTAssertEqual(WheelScroll.coastTime, 0.5)
        let end = WheelScroll.coast(velocity: 2000, after: 0.5)
        XCTAssertEqual(WheelScroll.coast(velocity: 2000, after: 3), end)
    }
}
