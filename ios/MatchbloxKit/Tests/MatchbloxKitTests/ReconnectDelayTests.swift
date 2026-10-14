import XCTest
import MatchbloxKit

final class ReconnectDelayTests: XCTestCase {
    func testALongSessionWithOutputEndsAndWaitsForATap() {
        var delay = ReconnectDelay()
        XCTAssertEqual(delay.next(after: .seconds(60), sawOutput: true), .ended)
        XCTAssertEqual(delay.next(after: ReconnectDelay.steady, sawOutput: true), .ended)
    }

    func testThreeShortSessionsInARowStop() {
        var delay = ReconnectDelay()
        XCTAssertEqual(delay.next(after: .milliseconds(300), sawOutput: true), .reconnect(after: .seconds(1)))
        XCTAssertEqual(delay.next(after: .seconds(1), sawOutput: true), .reconnect(after: .seconds(2)))
        XCTAssertEqual(delay.next(after: .zero, sawOutput: true), .stop)
    }

    func testASessionWithNoOutputIsShort() {
        var delay = ReconnectDelay()
        XCTAssertEqual(delay.next(after: .seconds(60), sawOutput: false), .reconnect(after: .seconds(1)))
        XCTAssertEqual(delay.next(after: .seconds(60), sawOutput: false), .reconnect(after: .seconds(2)))
        XCTAssertEqual(delay.next(after: .seconds(60), sawOutput: false), .stop)
    }

    func testALongSessionResetsTheCount() {
        var delay = ReconnectDelay()
        _ = delay.next(after: .zero, sawOutput: false)
        _ = delay.next(after: .zero, sawOutput: false)
        XCTAssertEqual(delay.next(after: .seconds(30), sawOutput: true), .ended)
        XCTAssertEqual(delay.next(after: .zero, sawOutput: false), .reconnect(after: .seconds(1)))
    }

    func testStopsAtOnceMessage() {
        XCTAssertEqual(
            Opening.stopsAtOnceMessage(host: "ws-1"),
            "matchblox console on ws-1 stops at once. Run matchblox console on the host to see why, then tap Try again."
        )
    }

    func testEndedMessage() {
        XCTAssertEqual(Opening.endedMessage(host: "ws-1"), "matchblox console on ws-1 ended.")
    }
}
