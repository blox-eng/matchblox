import XCTest
@testable import MatchbloxKit

/// VoiceOver's swipe on the head: one option at a menu, never past either end; a page with none.
final class HeadVoiceTests: XCTestCase {
    private let sign = "0123456789abcdef"

    private func menu(chosen: Int) -> MenuSight {
        let texts = ["Yes", "Rotate, keep", "No"]
        return MenuSight(options: texts.enumerated().map { MenuSight.Option(n: $0.offset + 1, text: $0.element) }, chosen: chosen, sign: sign)
    }

    private func ready(_ sight: MenuSight?) -> HeadWalk {
        var walk = HeadWalk()
        walk.menu(sight)
        return walk
    }

    func testIncrementMovesDownOneOption() {
        let sight = menu(chosen: 1)
        var walk = ready(sight)
        let step = HeadVoice.swipe(&walk, older: false, menu: sight)
        XCTAssertEqual(step, .option(n: 2, of: 3, text: "Rotate, keep"))
        XCTAssertEqual(step.line, "2 of 3 · Rotate, keep")
    }

    func testDecrementMovesUpOneOption() {
        let sight = menu(chosen: 3)
        var walk = ready(sight)
        XCTAssertEqual(HeadVoice.swipe(&walk, older: true, menu: sight), .option(n: 2, of: 3, text: "Rotate, keep"))
    }

    /// A roll crosses into the output past the first option; a swipe stops there, so the
    /// double-tap after it still answers.
    func testStopsAtTheFirstOption() {
        let sight = menu(chosen: 1)
        var walk = ready(sight)
        XCTAssertEqual(HeadVoice.swipe(&walk, older: true, menu: sight), .first)
        XCTAssertEqual(HeadVoice.swipe(&walk, older: true, menu: sight), .first)
        XCTAssertEqual(walk.tap(), .pick(n: 1, text: "Yes", sign: sign))
    }

    func testStopsAtTheLastOption() {
        let sight = menu(chosen: 3)
        var walk = ready(sight)
        XCTAssertEqual(HeadVoice.swipe(&walk, older: false, menu: sight), .last)
        XCTAssertEqual(walk.tap(), .pick(n: 3, text: "No", sign: sign))
    }

    /// The double-tap after a swipe picks the option the swipe named, through the checked pick.
    func testTapPicksTheSwipedOption() {
        let sight = menu(chosen: 1)
        var walk = ready(sight)
        _ = HeadVoice.swipe(&walk, older: false, menu: sight)
        XCTAssertEqual(walk.tap(), .pick(n: 2, text: "Rotate, keep", sign: sign))
    }

    /// A swipe is a roll of its own: one after a roll that crossed still walks the options.
    func testSwipeAfterACrossWalksTheOptions() {
        let sight = menu(chosen: 1)
        var walk = ready(sight)
        XCTAssertEqual(walk.notch(.up, route: .wheel, at: 0), .cross)
        XCTAssertEqual(HeadVoice.swipe(&walk, older: false, menu: sight), .option(n: 2, of: 3, text: "Rotate, keep"))
    }

    func testNoMenuPages() {
        var walk = ready(nil)
        XCTAssertEqual(HeadVoice.swipe(&walk, older: false, menu: nil), .page(older: false))
        XCTAssertEqual(HeadVoice.swipe(&walk, older: true, menu: nil), .page(older: true))
        XCTAssertEqual(walk.linesUp, 0)
    }

    /// Up in the output a menu is out of sight: a swipe pages.
    func testUpInTheOutputPages() {
        let sight = menu(chosen: 1)
        var walk = ready(sight)
        _ = walk.notch(.up, route: .wheel, at: 0)
        _ = walk.notch(.up, route: .wheel, at: 0.01)
        XCTAssertGreaterThan(walk.linesUp, 0)
        XCTAssertEqual(HeadVoice.swipe(&walk, older: false, menu: sight), .page(older: false))
    }

    func testPagedLines() {
        XCTAssertEqual(HeadVoice.paged(.wheel, older: true), "a page up")
        XCTAssertEqual(HeadVoice.paged(.scrollback, older: false), "a page down")
        XCTAssertEqual(HeadVoice.paged(.end, older: true), "top of the scrollback")
        XCTAssertEqual(HeadVoice.paged(.end, older: false), "bottom of the output")
        XCTAssertEqual(HeadVoice.paged(.nothing, older: true), "nothing scrolls here")
        XCTAssertNil(HeadVoice.Step.page(older: true).line)
    }
}
