import XCTest
@testable import MatchbloxKit

/// The middle answers and the rim's marks send keys, so where a finger lands decides what a lift means.
final class HeadZoneTests: XCTestCase {
    func testZones() {
        XCTAssertEqual(HeadZone.at(point: CGPoint(x: 0, y: 20), radius: 66), .middle)
        XCTAssertEqual(HeadZone.at(point: CGPoint(x: 0, y: -50), radius: 66), .edge(.top))
        XCTAssertEqual(HeadZone.at(point: CGPoint(x: -50, y: 5), radius: 66), .edge(.left))
        XCTAssertEqual(HeadZone.at(point: CGPoint(x: 50, y: 0), radius: 66), .edge(.right))
        XCTAssertEqual(HeadZone.at(point: CGPoint(x: 3, y: 50), radius: 66), .edge(.bottom))
    }

    func testPressUnderTenPoints() {
        XCTAssertTrue(HeadZone.isPress(moved: 9.9))
        XCTAssertFalse(HeadZone.isPress(moved: 10), "a touch that moves 10 pt brushes the edge: no key")
    }
}
