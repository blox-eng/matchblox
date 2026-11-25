import XCTest
import MatchbloxKit

final class UnlockGraceTests: XCTestCase {
    private let start = ContinuousClock.now

    func testTheWindowIsFiveMinutes() {
        XCTAssertEqual(UnlockGrace.window, .seconds(300))
    }

    func testAnAppThatNeverLeftHolds() {
        var grace = UnlockGrace()
        XCTAssertTrue(grace.returned(at: start))
    }

    func testAShortTripHolds() {
        var grace = UnlockGrace()
        grace.backgrounded(at: start)
        XCTAssertTrue(grace.returned(at: start + .seconds(1)))
    }

    func testAtTheWindowItHolds() {
        var grace = UnlockGrace()
        grace.backgrounded(at: start)
        XCTAssertTrue(grace.returned(at: start + .seconds(300)))
    }

    func testAfterTheWindowItAsks() {
        var grace = UnlockGrace()
        grace.backgrounded(at: start)
        XCTAssertFalse(grace.returned(at: start + .seconds(301)))
    }

    func testEachTripCountsFromItsOwnStart() {
        var grace = UnlockGrace()
        grace.backgrounded(at: start)
        XCTAssertTrue(grace.returned(at: start + .seconds(200)))
        grace.backgrounded(at: start + .seconds(210))
        XCTAssertTrue(grace.returned(at: start + .seconds(410)))
    }
}
