import XCTest
@testable import MatchbloxKit

final class KeyBarTests: XCTestCase {
    func testNineKeysInOrder() {
        XCTAssertEqual(KeyBar.keys.map(\.label), ["◂", "↑", "↓", "⏎", "Tab", "⇧Tab", "Esc", "^C", "⌨"])
    }

    func testActions() {
        XCTAssertEqual(KeyBar.keys.map(\.action), [
            .send(HomeKey.bytes),
            .send([0x1b, 0x5b, 0x41]),
            .send([0x1b, 0x5b, 0x42]),
            .send([0x0d]),
            .send([0x09]),
            .send([0x1b, 0x5b, 0x5a]),
            .send([0x1b]),
            .send([0x03]),
            .keyboard,
        ])
    }

    func testArrowsAreWiderAndTaller() {
        for key in KeyBar.keys {
            let arrow = key.label == "↑" || key.label == "↓"
            XCTAssertEqual(key.width, arrow ? 1.5 : 1, key.label)
            XCTAssertGreaterThanOrEqual(key.height, arrow ? 52 : KeyBar.minSide, key.label)
        }
    }

    func testMinSide() {
        XCTAssertGreaterThanOrEqual(KeyBar.minSide, 44)
    }

    func testLabelsAreNoWords() {
        for key in KeyBar.keys {
            XCTAssertLessThanOrEqual(key.label.count, 4, key.label)
        }
    }
}
