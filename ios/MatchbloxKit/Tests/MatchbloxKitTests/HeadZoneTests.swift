import XCTest
@testable import MatchbloxKit

/// The middle is the stick and the edge the wheel, so where a finger lands decides what a touch does.
final class HeadZoneTests: XCTestCase {
    func testZones() {
        XCTAssertEqual(HeadZone.at(point: CGPoint(x: 0, y: 20), radius: 66), .middle)
        XCTAssertEqual(HeadZone.at(point: CGPoint(x: 0, y: -50), radius: 66), .edge(.top))
        XCTAssertEqual(HeadZone.at(point: CGPoint(x: -50, y: 5), radius: 66), .edge(.left))
        XCTAssertEqual(HeadZone.at(point: CGPoint(x: 50, y: 0), radius: 66), .edge(.right))
        XCTAssertEqual(HeadZone.at(point: CGPoint(x: 3, y: 50), radius: 66), .edge(.bottom))
    }
}
