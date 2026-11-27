import XCTest
@testable import MatchbloxKit

/// The hold's sessions card: a lift goes only to the row the card shows chosen, and never to the
/// session the console already shows.
final class SessionCardTests: XCTestCase {
    private let rows = [
        SessionRow(n: 1, name: "spark-docs", word: "idle", pane: "%3"),
        SessionRow(n: 2, name: "hearth-api", word: "busy", pane: "%1"),
        SessionRow(n: 5, name: "flint-auth", word: "asks", pane: "%2"),
        SessionRow(n: 6, name: "ember-billing", word: "", pane: "%8"),
    ]

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

    /// A push `↓` chooses the next session and `↑` the one before; neither goes past an end, so
    /// a push back leaves the end at once.
    func testPushSteps() {
        var card = SessionCard(rows: rows, pane: "%2", opened: nil)
        XCTAssertTrue(card.step(by: 1))
        XCTAssertEqual(card.chosen, 3)
        XCTAssertFalse(card.step(by: 1), "past the last row")
        XCTAssertEqual(card.chosen, 3)
        XCTAssertTrue(card.step(by: -1))
        XCTAssertEqual(card.chosen, 2)
        for _ in 0..<5 { _ = card.step(by: -1) }
        XCTAssertEqual(card.chosen, 0, "past the first row")
        XCTAssertTrue(card.step(by: 1))
        XCTAssertEqual(card.chosen, 1)
    }

    /// Surface: the owner's sessions. A lift on the current session, still or after pushes that
    /// came back to it, opens nothing.
    func testLiftOnTheCurrentOpensNothing() {
        var card = SessionCard(rows: rows, pane: "%2", opened: nil)
        XCTAssertNil(card.lift)
        _ = card.step(by: 1)
        XCTAssertEqual(card.lift, 6)
        _ = card.step(by: -1)
        XCTAssertEqual(card.chosen, 2)
        XCTAssertNil(card.lift, "a push back onto the current session opens nothing")
    }

    /// Surface: the owner's sessions. The lift opens the number of the row the card shows chosen,
    /// never a neighbour's.
    func testLiftGoesToTheRowShownChosen() {
        var card = SessionCard(rows: rows, pane: "%2", opened: nil)
        _ = card.step(by: -1)
        XCTAssertEqual(card.chosen, 1)
        XCTAssertTrue(card.text.contains("▸ hearth-api"), card.text)
        XCTAssertEqual(card.lift, 2)
        _ = card.step(by: -1)
        XCTAssertTrue(card.text.contains("▸ spark-docs"), card.text)
        XCTAssertEqual(card.lift, 1)
    }

    /// With no session marked, nothing is chosen until the first push `↓` chooses the first row.
    func testWithNoneMarkedTheFirstStepChoosesTheFirst() {
        var card = SessionCard(rows: rows, pane: nil, opened: nil)
        XCTAssertNil(card.chosen)
        XCTAssertNil(card.lift)
        XCTAssertFalse(card.step(by: -1))
        XCTAssertNil(card.lift, "a push ↑ from none chooses none")
        XCTAssertTrue(card.step(by: 1))
        XCTAssertEqual(card.lift, 1, "a push ↓ from none chooses the first")
    }

    /// Two rows with one number: a lift on either opens nothing, as the number names no one session.
    func testASharedNumberOpensNothing() {
        let twins = [SessionRow(n: 4, name: "a", word: "", pane: "%1"), SessionRow(n: 4, name: "b", word: "", pane: "%2"),
                     SessionRow(n: 7, name: "c", word: "", pane: "%3")]
        var card = SessionCard(rows: twins, pane: "%3", opened: nil)
        _ = card.step(by: -1)
        XCTAssertEqual(card.chosen, 1)
        XCTAssertNil(card.lift)
    }

    func testOneSessionOrNoneHasItsOwnLine() {
        for list in [[], [rows[2]]] {
            var card = SessionCard(rows: list, pane: "%2", opened: nil)
            XCTAssertTrue(card.isEmpty)
            XCTAssertEqual(card.text, "no other session on this host")
            XCTAssertFalse(card.step(by: 1))
            XCTAssertNil(card.lift)
        }
    }
}
