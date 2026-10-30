import XCTest
@testable import MatchbloxKit

final class KeyBarTests: XCTestCase {
    func testSevenKeysInOrder() {
        XCTAssertEqual(KeyBar.keys.map(\.label), ["↑", "↓", "⏎", "Tab", "⇧Tab", "Esc", "^C"])
    }

    func testBytes() {
        XCTAssertEqual(KeyBar.keys.map(\.bytes), [
            [0x1b, 0x5b, 0x41],
            [0x1b, 0x5b, 0x42],
            [0x0d],
            [0x09],
            [0x1b, 0x5b, 0x5a],
            [0x1b],
            [0x03],
        ])
    }

    func testMinSide() {
        XCTAssertGreaterThanOrEqual(KeyBar.minSide, 44)
    }
}
