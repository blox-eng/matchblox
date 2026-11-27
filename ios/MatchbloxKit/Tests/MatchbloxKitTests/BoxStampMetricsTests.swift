import XCTest
@testable import MatchbloxKit

final class BoxStampMetricsTests: XCTestCase {
    func testBoxStampMinHitIs44() {
        XCTAssertEqual(BoxStampMetrics.minHit, 44)
    }
}
