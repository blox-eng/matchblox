import XCTest
@testable import MatchbloxKit

/// A roll of the head's edge walks the output: lines up, home, a bump at each end, and a pace
/// that speeds up. A push walks it a line at a time. A tap up in the output comes home.
final class HeadScrollTests: XCTestCase {
    /// `count` notches 0.01 s apart, from `start`.
    private func roll(_ scroll: inout HeadScroll, _ dir: HeadScroll.Dir, _ count: Int,
                      route: HeadRoute = .wheel, start: TimeInterval = 0) -> [HeadScroll.Step] {
        (1...count).map { scroll.notch(dir, route: route, at: start + Double($0) * 0.01) }
    }

    func testRollScrollsAtOnce() {
        var scroll = HeadScroll()
        XCTAssertEqual(roll(&scroll, .up, 2), [.scroll(1), .scroll(1)])
        XCTAssertEqual(scroll.linesUp, 2)
    }

    func testDownStopsAtBottom() {
        var scroll = HeadScroll()
        XCTAssertEqual(roll(&scroll, .up, 2), [.scroll(1), .scroll(1)])
        scroll.lift()
        XCTAssertEqual(roll(&scroll, .down, 3), [.scroll(-1), .home, .bump(.bottom)])
        XCTAssertEqual(scroll.linesUp, 0)
    }

    func testTopOfScrollbackBumps() {
        var scroll = HeadScroll()
        XCTAssertEqual(roll(&scroll, .up, 1, route: .end), [.bump(.top)])
        XCTAssertEqual(roll(&scroll, .up, 1, route: .nothing), [.bump(.nothing)])
        XCTAssertEqual(scroll.linesUp, 0)
    }

    func testRollPaces() {
        var scroll = HeadScroll()
        let up = roll(&scroll, .up, 17)
        XCTAssertEqual(up.last, .scroll(3))
        XCTAssertEqual(scroll.linesUp, 31)
        scroll.lift()
        let again = roll(&scroll, .up, 11)
        XCTAssertEqual(again.first, .scroll(1), "a new roll paces from its first notch")
        XCTAssertEqual(again.last, .scroll(3))
        XCTAssertEqual(roll(&scroll, .down, 1, start: 0.5), [.scroll(-1)], "a pause starts the pace again")
    }

    /// Past the first option a push `↑` reads up a line a push, with no pace, and `↓` comes back.
    func testPushMovesOneLine() {
        var scroll = HeadScroll()
        let up = (1...12).map { _ in scroll.push(.up, route: .wheel) }
        XCTAssertEqual(up, [HeadScroll.Step](repeating: .scroll(1), count: 12), "a held push never speeds up")
        XCTAssertEqual(scroll.linesUp, 12)
        XCTAssertEqual(scroll.push(.down, route: .wheel), .scroll(-1))
        XCTAssertEqual(scroll.linesUp, 11)
        XCTAssertEqual(scroll.push(.up, route: .end), .bump(.top))
        XCTAssertEqual(scroll.linesUp, 11)
        for _ in 1..<11 { _ = scroll.push(.down, route: .wheel) }
        XCTAssertEqual(scroll.push(.down, route: .wheel), .home)
        XCTAssertEqual(scroll.linesUp, 0)
        XCTAssertEqual(scroll.push(.down, route: .wheel), .bump(.bottom))
    }

    func testTapComesHomeElseEnter() {
        var scroll = HeadScroll()
        XCTAssertEqual(scroll.tap(), .enter)
        _ = roll(&scroll, .up, 3)
        XCTAssertEqual(scroll.tap(), .home)
        XCTAssertEqual(scroll.linesUp, 0)
        XCTAssertEqual(scroll.tap(), .enter)
        _ = scroll.push(.up, route: .wheel)
        XCTAssertEqual(scroll.tap(), .home, "a tap up in the output sends nothing")
    }

    /// The view came back down another way (a typed send, a belt key, another pane): the walk is
    /// down too, so the next tap is Enter and the next push `↓` bumps the bottom. A terminal that
    /// knows it is at its live bottom (`.end` toward newer lines) says so; one that scrolls up or
    /// cannot tell keeps the walk.
    func testResetAfterOutsideReturn() {
        var scroll = HeadScroll()
        _ = roll(&scroll, .up, 12)
        XCTAssertFalse(scroll.check(newer: .wheel), "a pane still up kept no walk")
        XCTAssertFalse(scroll.check(newer: .scrollback))
        XCTAssertGreaterThan(scroll.linesUp, 0)
        XCTAssertTrue(scroll.check(newer: .end), "the live bottom did not end the walk")
        XCTAssertEqual(scroll.linesUp, 0)
        XCTAssertEqual(scroll.tap(), .enter)
        XCTAssertFalse(scroll.check(newer: .end), "a walk already down came down again")

        _ = scroll.push(.up, route: .wheel)
        scroll.cameDown()
        XCTAssertEqual(scroll.linesUp, 0)
        XCTAssertEqual(scroll.push(.down, route: .wheel), .bump(.bottom))
        XCTAssertEqual(roll(&scroll, .up, 1), [.scroll(1)], "the pace did not start again")
    }

    /// A finger's swipe on the terminal moves the view as the head does (positive: older): during
    /// a walk its lines count, never below 0, and back at 0 the walk ends. With no walk up a swipe
    /// starts none, so the next tap is still Enter.
    func testPanCountsTowardTheWalk() {
        var scroll = HeadScroll()
        XCTAssertFalse(scroll.panned(3), "a swipe with no walk ended one")
        XCTAssertEqual(scroll.linesUp, 0, "a swipe with no walk started one")
        XCTAssertEqual(scroll.tap(), .enter)

        _ = roll(&scroll, .up, 4)
        XCTAssertEqual(scroll.linesUp, 4)
        XCTAssertFalse(scroll.panned(2))
        XCTAssertEqual(scroll.linesUp, 6, "a swipe up during a walk did not count")
        XCTAssertFalse(scroll.panned(-5))
        XCTAssertEqual(scroll.linesUp, 1)
        XCTAssertTrue(scroll.panned(-3), "a swipe back down past the bottom did not end the walk")
        XCTAssertEqual(scroll.linesUp, 0, "the count went below 0")
        XCTAssertEqual(scroll.tap(), .enter, "after a swipe back down the tap was not Enter")
        XCTAssertEqual(scroll.push(.down, route: .wheel), .bump(.bottom))
        XCTAssertEqual(roll(&scroll, .up, 1), [.scroll(1)], "the pace did not start again")
    }
}
