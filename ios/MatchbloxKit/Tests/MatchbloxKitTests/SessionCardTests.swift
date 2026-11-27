import XCTest
@testable import MatchbloxKit

/// The hold's sessions card: a lift goes only to the row the card shows chosen, and never to the
/// session the console already shows.
final class SessionCardTests: XCTestCase {
    private let radius: CGFloat = 48
    private let rows = [
        SessionRow(n: 1, name: "spark-docs", word: "idle", pane: "%3"),
        SessionRow(n: 2, name: "hearth-api", word: "busy", pane: "%1"),
        SessionRow(n: 5, name: "flint-auth", word: "asks", pane: "%2"),
        SessionRow(n: 6, name: "ember-billing", word: "", pane: "%8"),
    ]

    /// The finger round the head at `degrees` (0 is right of the centre, clockwise grows), `reach`
    /// points out.
    private func at(_ degrees: CGFloat, reach: CGFloat = 40) -> CGPoint {
        let a = degrees * .pi / 180
        return CGPoint(x: reach * cos(a), y: reach * sin(a))
    }

    /// Turns from `from` to `to` degrees in 5° moves, as a finger does.
    private func turn(_ card: inout SessionCard, from: CGFloat, to: CGFloat) {
        let by: CGFloat = to >= from ? 5 : -5
        var d = from
        while (by > 0 && d <= to) || (by < 0 && d >= to) {
            _ = card.moved(to: at(d), radius: radius)
            d += by
        }
    }

    func testTheCurrentSessionIsThePaneThenTheOpenedNumber() {
        XCTAssertEqual(SessionCard(rows: rows, pane: "%2", opened: 1).current, 2, "the pane comes first")
        XCTAssertEqual(SessionCard(rows: rows, pane: "%9", opened: 2).current, 1)
        XCTAssertEqual(SessionCard(rows: rows, pane: nil, opened: 6).current, 3)
        XCTAssertNil(SessionCard(rows: rows, pane: "%9", opened: 9).current)
        XCTAssertNil(SessionCard(rows: rows, pane: nil, opened: nil).current)
    }

    func testTheCardMarksTheCurrentAndTheChosen() {
        let card = SessionCard(rows: rows, pane: "%2", opened: nil)
        XCTAssertEqual(card.text, """
              spark-docs · idle
              hearth-api · busy
            ▸ flint-auth · asks · here
              ember-billing
            """)
    }

    /// Surface: the owner's sessions. A lift on the current session, still or after a turn of
    /// less than a step, or a turn that came back, opens nothing.
    func testLiftOnTheCurrentOpensNothing() {
        var card = SessionCard(rows: rows, pane: "%2", opened: nil)
        XCTAssertNil(card.lift)
        _ = card.moved(to: at(0, reach: 8), radius: radius)
        XCTAssertNil(card.lift, "inside a quarter of the radius the angle steps nothing")
        turn(&card, from: 0, to: 25)
        XCTAssertNil(card.lift, "25° is less than a step")
        turn(&card, from: 25, to: 70)
        XCTAssertEqual(card.lift, 6)
        turn(&card, from: 70, to: 0)
        XCTAssertEqual(card.chosen, 2)
        XCTAssertNil(card.lift, "a turn back onto the current session opens nothing")
    }

    /// Surface: the owner's sessions. The lift opens the number of the row the card shows chosen,
    /// never a neighbour's.
    func testLiftGoesToTheRowShownChosen() {
        var card = SessionCard(rows: rows, pane: "%2", opened: nil)
        turn(&card, from: 0, to: -35)
        XCTAssertEqual(card.chosen, 1)
        XCTAssertTrue(card.text.contains("▸ hearth-api"), card.text)
        XCTAssertEqual(card.lift, 2)
        turn(&card, from: -35, to: -65)
        XCTAssertTrue(card.text.contains("▸ spark-docs"), card.text)
        XCTAssertEqual(card.lift, 1)
    }

    /// A turn past an end is not counted: a step back leaves the end at once.
    func testATurnPastTheEndIsNotCounted() {
        var card = SessionCard(rows: rows, pane: "%2", opened: nil)
        turn(&card, from: 0, to: 300)
        XCTAssertEqual(card.chosen, 3)
        turn(&card, from: 300, to: 265)
        XCTAssertEqual(card.chosen, 2)
    }

    /// The finger that comes back near the centre and out again elsewhere does not jump.
    func testAFingerThroughTheCentreDoesNotJump() {
        var card = SessionCard(rows: rows, pane: "%2", opened: nil)
        _ = card.moved(to: at(0), radius: radius)
        _ = card.moved(to: at(90, reach: 6), radius: radius)
        _ = card.moved(to: at(180), radius: radius)
        XCTAssertEqual(card.chosen, 2)
        XCTAssertNil(card.lift)
    }

    /// With no session marked, nothing is chosen until the first step clockwise chooses the first row.
    func testWithNoneMarkedTheFirstStepChoosesTheFirst() {
        var card = SessionCard(rows: rows, pane: nil, opened: nil)
        XCTAssertNil(card.chosen)
        XCTAssertNil(card.lift)
        turn(&card, from: 0, to: -40)
        XCTAssertNil(card.lift, "anticlockwise from none chooses none")
        turn(&card, from: -40, to: -5)
        XCTAssertEqual(card.lift, 1, "a turn back from none counts from none")
    }

    /// Two rows with one number: a lift on either opens nothing, as the number names no one session.
    func testASharedNumberOpensNothing() {
        let twins = [SessionRow(n: 4, name: "a", word: "", pane: "%1"), SessionRow(n: 4, name: "b", word: "", pane: "%2"),
                     SessionRow(n: 7, name: "c", word: "", pane: "%3")]
        var card = SessionCard(rows: twins, pane: "%3", opened: nil)
        turn(&card, from: 0, to: -35)
        XCTAssertEqual(card.chosen, 1)
        XCTAssertNil(card.lift)
    }

    func testOneSessionOrNoneHasItsOwnLine() {
        for list in [[], [rows[2]]] {
            var card = SessionCard(rows: list, pane: "%2", opened: nil)
            XCTAssertTrue(card.isEmpty)
            XCTAssertEqual(card.text, "no other session on this host")
            turn(&card, from: 0, to: 90)
            XCTAssertNil(card.lift)
        }
    }
}
