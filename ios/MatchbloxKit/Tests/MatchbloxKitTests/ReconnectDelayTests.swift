import XCTest
import MatchbloxKit

final class ReconnectDelayTests: XCTestCase {
    func testALongSessionReconnectsAtOnce() {
        var delay = ReconnectDelay()
        XCTAssertEqual(delay.next(after: .seconds(60)), .zero)
        XCTAssertEqual(delay.next(after: ReconnectDelay.steady), .zero)
    }

    func testShortSessionsBackOffToACap() {
        var delay = ReconnectDelay()
        XCTAssertEqual(delay.next(after: .milliseconds(300)), .seconds(1))
        XCTAssertEqual(delay.next(after: .seconds(1)), .seconds(2))
        XCTAssertEqual(delay.next(after: .zero), .seconds(5))
        XCTAssertEqual(delay.next(after: .zero), .seconds(5))
    }

    func testALongSessionResetsTheBackOff() {
        var delay = ReconnectDelay()
        _ = delay.next(after: .zero)
        _ = delay.next(after: .zero)
        XCTAssertEqual(delay.next(after: .seconds(30)), .zero)
        XCTAssertEqual(delay.next(after: .zero), .seconds(1))
    }
}
