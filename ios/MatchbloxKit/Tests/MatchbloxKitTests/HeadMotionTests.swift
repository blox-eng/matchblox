import XCTest
@testable import MatchbloxKit

/// The head moves as the mockup does: these are the mockup's numbers, and the side-by-side check
/// on the phone is the test of feel.
final class HeadMotionTests: XCTestCase {
    func testMotionIsTheMockups() {
        XCTAssertEqual(HeadMotion.marksFade, 0.18)
        XCTAssertEqual(HeadMotion.marksFrom, 0.86)
        XCTAssertEqual(HeadMotion.marksStay, 0.9)
        XCTAssertEqual(HeadMotion.marksCurve, HeadMotion.Curve(x1: 0.25, y1: 0.1, x2: 0.25, y2: 1), "CSS's ease")

        XCTAssertEqual(HeadMotion.springBack, 0.24)
        XCTAssertEqual(HeadMotion.springCurve, HeadMotion.Curve(x1: 0.34, y1: 1.56, x2: 0.64, y2: 1))
        XCTAssertGreaterThan(HeadMotion.springCurve.y1, 1, "the spring overshoots")
        XCTAssertEqual(HeadMotion.coastKeeps, 0.93)

        XCTAssertEqual(HeadMotion.beltForward, 0.25)
        XCTAssertEqual(HeadMotion.beltBackAfter, 1.4)
        XCTAssertEqual(HeadMotion.keyboard, 0.25)
        XCTAssertEqual(HeadMotion.drawer, 0.38)
        XCTAssertEqual(HeadMotion.drawerCurve, HeadMotion.Curve(x1: 0.2, y1: 0.8, x2: 0.2, y2: 1))
        XCTAssertEqual(HeadMotion.cardStays, 0.6)

        let full = HeadMotion.rule(reduceMotion: false)
        XCTAssertEqual(full, HeadMotion.Rule(fade: 0.18, fromScale: 0.86, springs: true))
        let reduced = HeadMotion.rule(reduceMotion: true)
        XCTAssertEqual(reduced.fade, 0.18, "Reduce Motion keeps the fades")
        XCTAssertEqual(reduced.fromScale, 1, "Reduce Motion drops the scales")
        XCTAssertFalse(reduced.springs, "Reduce Motion drops the spring")
    }
}
