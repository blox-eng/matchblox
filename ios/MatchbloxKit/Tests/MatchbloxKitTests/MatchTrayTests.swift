import XCTest
@testable import MatchbloxKit

final class MatchTrayTests: XCTestCase {
    private func matches(_ tray: MatchTray) -> [MatchTray.Match] {
        BeltLayout.allCases.map(tray.match)
    }

    // MARK: The matches

    func testTheLayoutInUseBurnsAndTheRestAreReady() {
        let tray = MatchTray(inUse: .steer)
        XCTAssertEqual(BeltLayout.allCases, [.steer, .commands, .tmux, .mine])
        XCTAssertEqual(matches(tray), [.burns, .ready, .ready, .ready])
    }

    func testAPickStrikesTheMatchAndTheOneLeftGoesOut() {
        var tray = MatchTray(inUse: .steer)
        XCTAssertEqual(tray.pick(.tmux), .steer)
        XCTAssertEqual(tray.inUse, .tmux)
        XCTAssertEqual(matches(tray), [.out, .ready, .burns, .ready])
    }

    func testTheBurntMatchStaysBurntWhileTheTrayIsOpen() {
        var tray = MatchTray(inUse: .steer)
        tray.pick(.tmux)
        XCTAssertEqual(tray.pick(.mine), .tmux)
        XCTAssertEqual(matches(tray), [.out, .ready, .out, .burns])
    }

    func testPickingTheLayoutInUseChangesNothing() {
        var tray = MatchTray(inUse: .commands)
        XCTAssertNil(tray.pick(.commands))
        XCTAssertEqual(matches(tray), [.ready, .burns, .ready, .ready])
    }

    func testAMatchLeftAndPickedAgainBurns() {
        var tray = MatchTray(inUse: .steer)
        tray.pick(.tmux)
        XCTAssertEqual(tray.pick(.steer), .tmux)
        XCTAssertEqual(matches(tray), [.burns, .ready, .out, .ready])
    }

    func testAClosedBoxIsFreshButTheMatchInUseBurns() {
        var tray = MatchTray(inUse: .steer)
        tray.pick(.tmux)
        tray.pick(.mine)
        tray.close()
        XCTAssertEqual(tray.inUse, .mine)
        XCTAssertEqual(matches(tray), [.ready, .ready, .ready, .burns])
    }

    func testTheIslandMatchOfEachState() {
        XCTAssertNil(MatchTray.Match.ready.look)
        XCTAssertEqual(MatchTray.Match.burns.look, IslandLook.Match(state: .burns, front: 0.12))
        XCTAssertEqual(MatchTray.Match.out.look?.state, .out)
        XCTAssertGreaterThan(MatchTray.Match.out.look?.front ?? 0, 0.12)
    }

    func testTheTraysTimesAndSlide() {
        XCTAssertEqual(MatchTray.slide, 30)
        XCTAssertEqual(MatchTray.flare, 0.26, accuracy: 1e-9)
        XCTAssertEqual(MatchTray.smokeFade, 1.4, accuracy: 1e-9)
        XCTAssertEqual(MatchTray.closeDelay, 0.9, accuracy: 1e-9)
    }

    // MARK: The editor's grid

    private func spots(_ cells: [Int]) -> [[Int]] {
        CellGrid.place(cells).map { [$0.row, $0.column, $0.span] }
    }

    func testOneCellKeysFillSixColumnsThenWrap() {
        XCTAssertEqual(CellGrid.columns, 6)
        XCTAssertEqual(spots([1, 1, 1, 1, 1, 1, 1]), [
            [0, 0, 1], [0, 1, 1], [0, 2, 1], [0, 3, 1], [0, 4, 1], [0, 5, 1],
            [1, 0, 1],
        ])
    }

    func testTwoCellKeysSpanTwoColumns() {
        // The Steer group: ◂ ↑ ↓ ⏎ Esc ^C ⇞ ⇟.
        XCTAssertEqual(spots([1, 2, 2, 2, 1, 1, 1, 1]), [
            [0, 0, 1], [0, 1, 2], [0, 3, 2],
            [1, 0, 2], [1, 2, 1], [1, 3, 1], [1, 4, 1], [1, 5, 1],
        ])
    }

    func testATwoCellKeyThatDoesNotFitStartsTheNextRow() {
        XCTAssertEqual(spots([1, 1, 1, 1, 1, 2, 1]), [
            [0, 0, 1], [0, 1, 1], [0, 2, 1], [0, 3, 1], [0, 4, 1],
            [1, 0, 2], [1, 2, 1],
        ])
    }

    func testCommandsLieThreeToARow() {
        XCTAssertEqual(spots([2, 2, 2, 2]), [[0, 0, 2], [0, 2, 2], [0, 4, 2], [1, 0, 2]])
    }

    func testRows() {
        XCTAssertEqual(CellGrid.rows(CellGrid.place([])), 0)
        XCTAssertEqual(CellGrid.rows(CellGrid.place([1, 1, 1, 1, 1, 1])), 1)
        XCTAssertEqual(CellGrid.rows(CellGrid.place([1, 1, 1, 1, 1, 1, 2])), 2)
        XCTAssertEqual(CellGrid.rows(CellGrid.place(Array(repeating: 1, count: 9))), 2)
    }

    func testAWideKeyNeverSpansMoreThanTheGrid() {
        XCTAssertEqual(CellGrid.place([9], columns: 6), [CellGrid.Spot(row: 0, column: 0, span: 6)])
        XCTAssertEqual(CellGrid.place([0]), [CellGrid.Spot(row: 0, column: 0, span: 1)])
    }
}
