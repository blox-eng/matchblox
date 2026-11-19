import XCTest
@testable import MatchbloxKit

final class TextSizeTests: XCTestCase {
    private func defaults() -> UserDefaults {
        let name = "TextSizeTests.\(UUID().uuidString)"
        let d = UserDefaults(suiteName: name)!
        addTeardownBlock { d.removePersistentDomain(forName: name) }
        return d
    }

    func testASizeStaysInItsRange() {
        XCTAssertEqual(TextSize(3).points, 9)
        XCTAssertEqual(TextSize(40).points, 24)
        XCTAssertEqual(TextSize(15).points, 15)
    }

    func testStepsStopAtEachEnd() {
        XCTAssertEqual(TextSize(9).smaller, TextSize(9))
        XCTAssertEqual(TextSize(24).bigger, TextSize(24))
        XCTAssertEqual(TextSize(13).bigger.points, 14)
        XCTAssertEqual(TextSize(13).smaller.points, 12)
        XCTAssertTrue(TextSize(9).isSmallest)
        XCTAssertTrue(TextSize(24).isBiggest)
        XCTAssertFalse(TextSize.standard.isSmallest || TextSize.standard.isBiggest)
    }

    func testAPinchStepsInWholePointsAndClamps() {
        let start = TextSize(13)
        XCTAssertEqual(start.pinched(1), start)
        XCTAssertEqual(start.pinched(1.03), start)
        XCTAssertEqual(start.pinched(1.5).points, 20)
        XCTAssertEqual(start.pinched(0.8).points, 10)
        XCTAssertEqual(start.pinched(4).points, 24)
        XCTAssertEqual(start.pinched(0.1).points, 9)
    }

    func testABadScaleKeepsTheSize() {
        XCTAssertEqual(TextSize(13).pinched(0), TextSize(13))
        XCTAssertEqual(TextSize(13).pinched(.nan), TextSize(13))
        XCTAssertEqual(TextSize(13).pinched(.infinity), TextSize(13))
    }

    func testTheSizeIsKept() {
        let store = defaults()
        XCTAssertEqual(TextSize.load(from: store), TextSize.standard)
        TextSize(17).save(to: store)
        XCTAssertEqual(TextSize.load(from: store).points, 17)
        store.set(99, forKey: TextSize.storeKey)
        XCTAssertEqual(TextSize.load(from: store).points, 24)
        store.set("big", forKey: TextSize.storeKey)
        XCTAssertEqual(TextSize.load(from: store), TextSize.standard)
    }
}
