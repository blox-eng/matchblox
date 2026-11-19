import XCTest
@testable import MatchbloxKit

final class ScrollBackTests: XCTestCase {
    func testAtTheBottomFirst() {
        XCTAssertFalse(ScrollBack().isUp)
    }

    func testStepsUpLessStepsDown() {
        var back = ScrollBack()
        back.wheeled([.up, .up, .up])
        back.wheeled([.down])
        XCTAssertEqual(back.steps, 2)
        XCTAssertTrue(back.isUp)
        back.wheeled([.down, .down])
        XCTAssertFalse(back.isUp)
    }

    func testAStepDownAtTheBottomCountsNothing() {
        var back = ScrollBack()
        back.wheeled([.down, .down])
        back.wheeled([.up])
        XCTAssertEqual(back.steps, 1)
    }

    func testATypedKeyEndsTheCount() {
        var back = ScrollBack()
        back.wheeled([.up, .up])
        back.typed()
        XCTAssertFalse(back.isUp)
        XCTAssertEqual(back.jump(), [])
    }

    func testTheJumpSendsEachStepBack() {
        var back = ScrollBack()
        back.wheeled([.up, .up, .down, .up])
        XCTAssertEqual(back.jump(), [.down, .down])
        XCTAssertFalse(back.isUp)
        XCTAssertEqual(back.jump(), [])
    }
}
