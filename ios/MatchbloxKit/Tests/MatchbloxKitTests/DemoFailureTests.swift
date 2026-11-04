import XCTest
@testable import MatchbloxKit

final class DemoFailureTests: XCTestCase {
    func testNoSpaceLeftSaysToFreeSpace() {
        let raw = "demo: write /var/mobile/tmp/x/tmux-panes.txt: no space left on device"
        XCTAssertEqual(DemoFailure.sentence(for: raw), "The phone is out of space. Free some space, then tap Try again.")
    }

    func testAnOldRunStillClosingSaysToWait() {
        XCTAssertEqual(DemoFailure.sentence(for: "demo: already running"), "The demo is still closing. Tap Try again in a moment.")
    }

    func testAnUnknownErrorSaysNothingOfItsText() {
        let sentence = DemoFailure.sentence(for: "open /private/var/x: permission denied")
        XCTAssertEqual(sentence, "The demo could not run. Tap Try again.")
        XCTAssertEqual(DemoFailure.sentence(for: nil), sentence)
    }
}
