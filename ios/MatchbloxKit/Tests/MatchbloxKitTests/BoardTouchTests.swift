import XCTest
@testable import MatchbloxKit

final class BoardTouchTests: XCTestCase {
    private let left = Array("\u{1b}[D".utf8)
    private let right = Array("\u{1b}[C".utf8)

    func testASwipeToTheRightSendsLeft() {
        XCTAssertEqual(BoardTouch.page(dx: 80, velocity: 0), left)
        XCTAssertEqual(BoardTouch.page(dx: -80, velocity: 0), right)
    }

    func testAFlickTurnsThePageToo() {
        XCTAssertEqual(BoardTouch.page(dx: 20, velocity: 900), left)
        XCTAssertEqual(BoardTouch.page(dx: -20, velocity: -900), right)
        XCTAssertEqual(BoardTouch.page(dx: 0, velocity: -900), right)
    }

    func testAShortSlowSwipeSendsNothing() {
        XCTAssertNil(BoardTouch.page(dx: 40, velocity: 200))
        XCTAssertNil(BoardTouch.page(dx: 0, velocity: 0))
    }

    func testOnlyAMostlySidewaysPanIsAPage() {
        XCTAssertTrue(BoardTouch.isSideways(dx: 300, dy: 100))
        XCTAssertTrue(BoardTouch.isSideways(dx: -300, dy: 299))
        XCTAssertFalse(BoardTouch.isSideways(dx: 100, dy: 300))
        XCTAssertFalse(BoardTouch.isSideways(dx: 200, dy: -200))
        XCTAssertFalse(BoardTouch.isSideways(dx: 0, dy: 0))
    }

    func testTheLeftEdgeStaysBack() {
        XCTAssertTrue(BoardTouch.fromEdge(x: 0))
        XCTAssertTrue(BoardTouch.fromEdge(x: 23))
        XCTAssertFalse(BoardTouch.fromEdge(x: 24))
        XCTAssertFalse(BoardTouch.fromEdge(x: 200))
    }

    func testALongPressIsARightClickAtItsCell() {
        XCTAssertEqual(BoardTouch.rightClick(col: 0, row: 0), Array("\u{1b}[<2;1;1M\u{1b}[<2;1;1m".utf8))
        XCTAssertEqual(BoardTouch.rightClick(col: 11, row: 4), Array("\u{1b}[<2;12;5M\u{1b}[<2;12;5m".utf8))
    }

    func testTheLongPressIsTheConsoles() {
        XCTAssertEqual(BoardTouch.longPress, 0.48)
    }
}
