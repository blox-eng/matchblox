#if canImport(CoreGraphics)
import CoreGraphics
#endif
import Foundation
import XCTest
@testable import MatchbloxKit

final class HeadSummonTests: XCTestCase {
    private let screen = CGRect(x: 0, y: 0, width: 400, height: 800)

    func testSitsUnderTheFinger() {
        XCTAssertEqual(HeadSummon.center(finger: CGPoint(x: 200, y: 300), bounds: screen), CGPoint(x: 200, y: 300))
    }

    func testStaysInsideTheEdges() {
        XCTAssertEqual(HeadSummon.center(finger: CGPoint(x: 5, y: 5), bounds: screen), CGPoint(x: 28, y: 28))
    }

    func testStaysInsideTheSafeArea() {
        let safe = CGRect(x: 0, y: 47, width: 390, height: 763)
        XCTAssertEqual(HeadSummon.center(finger: CGPoint(x: 200, y: 40), bounds: safe).y, 75)
    }

    func testStaysAboveTheKeyboard() {
        let center = HeadSummon.center(finger: CGPoint(x: 200, y: 700), bounds: screen, keyboardTop: 600)
        XCTAssertEqual(center.y, 572)
    }

    /// A tap meant for ↓ must never land on the head and send Enter.
    func testShiftsClearOfTheJumpButton() {
        let jump = CGRect(x: 340, y: 740, width: 44, height: 44)
        let center = HeadSummon.center(finger: CGPoint(x: 360, y: 760), bounds: screen, avoid: [jump])
        XCTAssertFalse(HeadSummon.frame(center: center).intersects(jump))
        XCTAssertTrue(screen.contains(HeadSummon.frame(center: center)))
    }

    func testWaitsThreeSecondsAfterTheLastTouch() {
        XCTAssertFalse(HeadSummon.isGone(lastTouch: 10, now: 12.9))
        XCTAssertTrue(HeadSummon.isGone(lastTouch: 10, now: 13.0))
    }
}
