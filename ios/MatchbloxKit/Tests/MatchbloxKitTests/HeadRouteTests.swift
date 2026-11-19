import XCTest
@testable import MatchbloxKit

final class HeadRouteTests: XCTestCase {
    private func route(mouse: Bool = false, tmux: Bool = false, scroll: Bool = false,
                       position: Double = 1, back: Int = 0, older: Bool) -> HeadRoute {
        HeadRoute.route(readsMouse: mouse, inTmuxPane: tmux, canScroll: scroll,
                        position: position, scrollBackSteps: back, older: older)
    }

    func testMouseProgramTakesTheWheel() {
        XCTAssertEqual(route(mouse: true, older: true), .wheel)
        XCTAssertEqual(route(mouse: true, older: false), .wheel)
    }

    func testTmuxBottomIsTheEnd() {
        XCTAssertEqual(route(mouse: true, tmux: true, back: 0, older: false), .end)
        XCTAssertEqual(route(mouse: true, tmux: true, back: 3, older: false), .wheel)
    }

    func testTmuxTopIsNeverKnown() {
        XCTAssertEqual(route(mouse: true, tmux: true, back: 500, older: true), .wheel)
    }

    func testScrollbackEnds() {
        XCTAssertEqual(route(scroll: true, position: 0, older: true), .end)
        XCTAssertEqual(route(scroll: true, position: 1, older: false), .end)
        XCTAssertEqual(route(scroll: true, position: 0.5, older: true), .scrollback)
    }

    func testNothingToScroll() {
        XCTAssertEqual(route(older: true), .nothing)
    }
}
