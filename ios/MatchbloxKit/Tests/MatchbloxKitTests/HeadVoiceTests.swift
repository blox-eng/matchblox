import XCTest
@testable import MatchbloxKit

/// VoiceOver's swipe on the head pages, and says where the page went.
final class HeadVoiceTests: XCTestCase {
    func testPagedLines() {
        XCTAssertEqual(HeadVoice.paged(.wheel, older: true), "a page up")
        XCTAssertEqual(HeadVoice.paged(.scrollback, older: false), "a page down")
        XCTAssertEqual(HeadVoice.paged(.end, older: true), "top of the scrollback")
        XCTAssertEqual(HeadVoice.paged(.end, older: false), "bottom of the output")
        XCTAssertEqual(HeadVoice.paged(.nothing, older: true), "nothing scrolls here")
    }
}
