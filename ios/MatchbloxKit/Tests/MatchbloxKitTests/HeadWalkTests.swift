import XCTest
@testable import MatchbloxKit

/// A roll walks Pocket's own highlight through a menu, then the output; a tap picks, comes home or is Enter.
final class HeadWalkTests: XCTestCase {
    private let sign = "0123456789abcdef"

    private func menu(chosen: Int) -> MenuSight {
        let texts = ["Yes", "Yes, and open a pull request", "No, and tell Claude what to do differently"]
        return MenuSight(options: texts.enumerated().map { MenuSight.Option(n: $0.offset + 1, text: $0.element) }, chosen: chosen, sign: sign)
    }

    private func ready(_ sight: MenuSight?) -> HeadWalk {
        var walk = HeadWalk()
        walk.menu(sight)
        return walk
    }

    /// `count` notches 0.01 s apart, from `start`.
    private func roll(_ walk: inout HeadWalk, _ dir: HeadWalk.Dir, _ count: Int,
                      route: HeadRoute = .wheel, start: TimeInterval = 0) -> [HeadWalk.Step] {
        (1...count).map { walk.notch(dir, route: route, at: start + Double($0) * 0.01) }
    }

    func testUpThroughMenuThenCross() {
        var walk = ready(menu(chosen: 3))
        XCTAssertEqual(roll(&walk, .up, 3), [.highlight(2), .highlight(1), .cross])
    }

    func testCrossThenScroll() {
        var walk = ready(menu(chosen: 1))
        XCTAssertEqual(roll(&walk, .up, 2), [.cross, .scroll(1)])
        XCTAssertEqual(walk.linesUp, 1)
    }

    func testNoMenuScrollsAtOnce() {
        var walk = ready(nil)
        XCTAssertEqual(roll(&walk, .up, 2), [.scroll(1), .scroll(1)])
    }

    func testDownStopsAtBottom() {
        var walk = ready(menu(chosen: 1))
        XCTAssertEqual(roll(&walk, .up, 3), [.cross, .scroll(1), .scroll(1)])
        walk.lift()
        XCTAssertEqual(roll(&walk, .down, 3), [.scroll(-1), .home, .bump(.bottom)], "no highlight in a roll that came from the output")
        XCTAssertEqual(walk.linesUp, 0)
    }

    func testNewRollMovesHighlight() {
        var walk = ready(menu(chosen: 2))
        XCTAssertEqual(roll(&walk, .up, 3), [.highlight(1), .cross, .scroll(1)])
        XCTAssertEqual(roll(&walk, .down, 2), [.home, .bump(.bottom)])
        walk.lift()
        XCTAssertEqual(roll(&walk, .down, 1), [.highlight(2)])
    }

    func testSameMenuKeepsHighlight() {
        var walk = ready(menu(chosen: 3))
        _ = roll(&walk, .up, 1)
        walk.menu(menu(chosen: 3))
        XCTAssertEqual(walk.highlight, 2, "a fresh read of the same menu keeps Pocket's highlight")
        walk.menu(MenuSight(options: menu(chosen: 3).options, chosen: 3, sign: "fedcba"))
        XCTAssertEqual(walk.highlight, 3, "a new question starts on the agent's mark")
    }

    func testLastOptionBumps() {
        var walk = ready(menu(chosen: 2))
        XCTAssertEqual(roll(&walk, .down, 2), [.highlight(3), .bump(.lastOption)])
    }

    func testTopOfScrollbackBumps() {
        var walk = ready(nil)
        XCTAssertEqual(roll(&walk, .up, 1, route: .end), [.bump(.top)])
        XCTAssertEqual(roll(&walk, .up, 1, route: .nothing), [.bump(.nothing)])
        XCTAssertEqual(walk.linesUp, 0)
    }

    func testRollStartingUpPaces() {
        var walk = ready(nil)
        let up = roll(&walk, .up, 17)
        XCTAssertEqual(up.last, .scroll(3))
        XCTAssertEqual(walk.linesUp, 31)
        walk.lift()
        let again = roll(&walk, .up, 11)
        XCTAssertEqual(again.first, .scroll(1), "a new roll paces from its first notch")
        XCTAssertEqual(again.last, .scroll(3))
        XCTAssertEqual(roll(&walk, .down, 1, start: 0.5), [.scroll(-1)], "a pause starts the pace again")
    }

    func testCrossStartsThePace() {
        var walk = ready(menu(chosen: 3))
        let steps = roll(&walk, .up, 14)
        XCTAssertEqual(Array(steps.prefix(3)), [.highlight(2), .highlight(1), .cross])
        XCTAssertEqual(Array(steps.dropFirst(3)), [HeadWalk.Step](repeating: .scroll(1), count: 10) + [.scroll(3)],
                       "the menu's notches do not count toward the pace")
    }

    func testWaitsForMenu() {
        var walk = HeadWalk()
        XCTAssertEqual(roll(&walk, .up, 2), [.wait, .wait])
        walk.menu(menu(chosen: 3))
        XCTAssertEqual(walk.highlight, 3, "a waited notch is dropped")
        XCTAssertEqual(roll(&walk, .up, 1), [.highlight(2)])
    }

    func testTapPicks() {
        var walk = ready(menu(chosen: 3))
        XCTAssertEqual(roll(&walk, .up, 1), [.highlight(2)])
        XCTAssertEqual(walk.tap(), .pick(n: 2, text: "Yes, and open a pull request", sign: sign))

        var plain = ready(nil)
        XCTAssertEqual(plain.tap(), .enter)
        _ = roll(&plain, .up, 3)
        XCTAssertEqual(plain.tap(), .home)
        XCTAssertEqual(plain.linesUp, 0)
        XCTAssertEqual(plain.tap(), .enter)
    }

    func testFailedReadAfterARollIsNoEnter() {
        var walk = ready(menu(chosen: 1))
        XCTAssertEqual(roll(&walk, .down, 2), [.highlight(2), .highlight(3)])
        walk.lift()
        walk.read(.unreadable)
        XCTAssertEqual(walk.tap(), .unreadable, "a screen Pocket could not read is never Enter")
        walk.read(.menu(menu(chosen: 1)))
        XCTAssertEqual(walk.tap(), .pick(n: 3, text: "No, and tell Claude what to do differently", sign: sign),
                       "the tap again answers the option rolled to, not the agent's mark")
    }

    func testUnreadableScreenSendsNothing() {
        var walk = HeadWalk()
        walk.read(.unreadable)
        XCTAssertEqual(walk.tap(), .unreadable)
        var fresh = HeadWalk()
        XCTAssertEqual(fresh.tap(), .unreadable, "a tap before any read is no Enter either")
        walk.read(.none)
        XCTAssertEqual(walk.tap(), .enter, "only a screen read with no question is Enter")
    }
}
