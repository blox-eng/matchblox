import XCTest
@testable import MatchbloxKit

final class BoardTouchTests: XCTestCase {
    /// The list and the chat have no pages: a sideways swipe that does not start at the edge
    /// sends nothing, in either direction, however far or fast.
    func testSidewaysSwipeTurnsNoPage() {
        for (dx, velocity) in [(80.0, 0.0), (-80.0, 0.0), (20.0, 900.0), (-20.0, -900.0), (300.0, 2000.0)] {
            XCTAssertNil(BoardTouch.sideways(fromEdge: false, dx: CGFloat(dx), velocity: CGFloat(velocity)), "dx \(dx), velocity \(velocity)")
        }
    }

    /// The edge swipe sends `◂`, the belt's way back to the list, once it runs far or fast enough.
    func testEdgeSwipeGoesBack() {
        let back = KeyBar.bytes(for: "◂")
        XCTAssertEqual(back, HomeKey.bytes)
        XCTAssertEqual(BoardTouch.sideways(fromEdge: true, dx: 80, velocity: 0), back)
        XCTAssertEqual(BoardTouch.sideways(fromEdge: true, dx: 20, velocity: 900), back)
        XCTAssertNil(BoardTouch.sideways(fromEdge: true, dx: 40, velocity: 200))
        XCTAssertNil(BoardTouch.sideways(fromEdge: true, dx: -80, velocity: -900))
    }

    func testALongPressIsARightClickAtItsCell() {
        XCTAssertEqual(BoardTouch.rightClick(col: 0, row: 0), Array("\u{1b}[<2;1;1M\u{1b}[<2;1;1m".utf8))
        XCTAssertEqual(BoardTouch.rightClick(col: 11, row: 4), Array("\u{1b}[<2;12;5M\u{1b}[<2;12;5m".utf8))
    }

    func testTheLongPressIsTheConsoles() {
        XCTAssertEqual(BoardTouch.longPress, 0.48)
    }
}
