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

    /// A belt key after the match head scrolled a tmux pane up: the jump brings every step back
    /// down first, so tmux leaves copy mode and `q` or `/` reach the agent.
    func testJumpAfterHeadBurst() {
        var back = ScrollBack()
        back.wheeled(Array(repeating: .up, count: 7))
        XCTAssertEqual(back.jump(), Array(repeating: .down, count: 7))
        XCTAssertFalse(back.isUp)
        XCTAssertEqual(back.jump(), [], "a second key sends no steps")
    }
}
