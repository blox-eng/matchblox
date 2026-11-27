import XCTest
import MatchbloxKit

/// A return keeps the last screen: a connect that fails while the phone brings its network back
/// tries again behind it, and only a failure that needs the person leaves the console.
final class QuietRetryTests: XCTestCase {
    private let start = ContinuousClock.now

    func testAKeptScreenTriesAClosedConnectionAgainAfter1Then2Then4Seconds() {
        var quiet = QuietRetry(keptScreen: true)
        XCTAssertEqual(quiet.next(after: .closed("eof"), at: start), .retry(after: .seconds(1)))
        XCTAssertEqual(quiet.next(after: .closed("eof"), at: start + .seconds(1)), .retry(after: .seconds(2)))
        XCTAssertEqual(quiet.next(after: .closed("eof"), at: start + .seconds(3)), .retry(after: .seconds(4)))
        XCTAssertEqual(quiet.next(after: .closed("eof"), at: start + .seconds(7)), .retry(after: .seconds(4)))
    }

    func testAKeptScreenTriesAnUnreachableHostAgain() {
        var quiet = QuietRetry(keptScreen: true)
        XCTAssertEqual(quiet.next(after: .unreachable, at: start), .retry(after: .seconds(1)))
    }

    func testAfter20SecondsTheKeptScreenSaysSo() {
        var quiet = QuietRetry(keptScreen: true)
        _ = quiet.next(after: .unreachable, at: start)
        XCTAssertEqual(quiet.next(after: .unreachable, at: start + .seconds(20)), .say)
    }

    func testAChangedHostKeyOrARefusedKeyLeavesTheConsoleAtOnce() {
        var changed = QuietRetry(keptScreen: true)
        XCTAssertEqual(changed.next(after: .hostKeyChanged, at: start), .leave)
        var refused = QuietRetry(keptScreen: true)
        XCTAssertEqual(refused.next(after: .refused, at: start), .leave)
    }

    func testAFirstOpenWithNoScreenLeavesForTheReachScreen() {
        var quiet = QuietRetry(keptScreen: false)
        XCTAssertEqual(quiet.next(after: .unreachable, at: start), .leave)
    }
}

/// Keys typed while the console comes back go to the host once it is there.
final class HeldKeysTests: XCTestCase {
    func testHeldKeysComeOutOnceInTheOrderTyped() {
        var held = HeldKeys()
        held.hold(Data("ls".utf8))
        held.hold(Data("\r".utf8))
        XCTAssertEqual(held.take(), Data("ls\r".utf8))
        XCTAssertNil(held.take())
    }

    func testPastTheLimitNoMoreKeysAreHeld() {
        var held = HeldKeys()
        held.hold(Data(repeating: 0x61, count: HeldKeys.limit))
        held.hold(Data("x".utf8))
        XCTAssertEqual(held.take()?.count, HeldKeys.limit)
    }

    func testDroppedKeysNeverGo() {
        var held = HeldKeys()
        held.hold(Data("y".utf8))
        held.drop()
        XCTAssertNil(held.take())
    }
}
