import XCTest
import MatchbloxKit

final class ReconnectDelayTests: XCTestCase {
    func testALongSessionWithOutputReconnectsAtOnce() {
        var delay = ReconnectDelay()
        XCTAssertEqual(delay.next(after: .seconds(60), sawOutput: true), .zero)
        XCTAssertEqual(delay.next(after: ReconnectDelay.steady, sawOutput: true), .zero)
    }

    func testThreeShortSessionsInARowStop() {
        var delay = ReconnectDelay()
        XCTAssertEqual(delay.next(after: .milliseconds(300), sawOutput: true), .seconds(1))
        XCTAssertEqual(delay.next(after: .seconds(1), sawOutput: true), .seconds(2))
        XCTAssertNil(delay.next(after: .zero, sawOutput: true))
    }

    func testASessionWithNoOutputIsShort() {
        var delay = ReconnectDelay()
        XCTAssertEqual(delay.next(after: .seconds(60), sawOutput: false), .seconds(1))
        XCTAssertEqual(delay.next(after: .seconds(60), sawOutput: false), .seconds(2))
        XCTAssertNil(delay.next(after: .seconds(60), sawOutput: false))
    }

    func testALongSessionResetsTheCount() {
        var delay = ReconnectDelay()
        _ = delay.next(after: .zero, sawOutput: false)
        _ = delay.next(after: .zero, sawOutput: false)
        XCTAssertEqual(delay.next(after: .seconds(30), sawOutput: true), .zero)
        XCTAssertEqual(delay.next(after: .zero, sawOutput: false), .seconds(1))
    }

    func testStopsAtOnceMessage() {
        XCTAssertEqual(
            Opening.stopsAtOnceMessage(host: "ws-1"),
            "matchblox console on ws-1 stops at once. Run matchblox console on the host to see why, then tap Try again."
        )
    }
}
