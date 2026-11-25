import XCTest
@testable import MatchbloxKit

final class BoardBeltTests: XCTestCase {
    private func catalog() throws -> KeyCatalog { try KeyCatalog(commands: testdata("keys/commands.json")) }

    private func board(_ value: String) -> BoardBelt {
        guard let board = BoardBelt(said: value) else {
            XCTFail("\"\(value)\" is no board")
            return .plain
        }
        return board
    }

    private func ids(_ value: String) -> [String] { board(value).keys.map(\.id) }

    private func labels(_ value: String) throws -> [String] {
        let c = try catalog()
        return board(value).keys.map { KeyFace($0, in: c).label }
    }

    private let arrows = ["↑", "↓", "⏎"]
    private let tail = ["c copy reply", "⌃o", "s hosts", "g chain", "/", "m menu", "z park", "N new", "? keys", ", machine", "t shell", "D review", "x kill"]

    // MARK: The keys

    /// The belt's tail follows the desk: ?, machine (face `,`), t and D replace tab (Task 16).
    func testBoardTailHasTheDeskKeys() {
        let want = ["c", "ctrl+o", "s", "g", "/", "m", "z", "N", "?", "machine", "t", "D", "x"]
        XCTAssertEqual(BoardBelt.tailKeys.map(\.id), want.map { "board.\($0)" })
        for id in want.map({ "board.\($0)" }) {
            let spoken = BoardBelt.spoken(of: id)
            XCTAssertNotNil(spoken, id)
            XCTAssertFalse(spoken!.isEmpty, id)
        }
        XCTAssertEqual(BoardBelt.label(of: "board.machine"), ", machine")
        XCTAssertEqual(BoardBelt.bytes(for: "board.machine"), Array("\u{1b}_matchblox;tap=,\u{1b}\\".utf8))
    }

    func testHomeIsBackTheCardsKeysAndTheArrows() throws {
        XCTAssertEqual(try labels("1,2;"), ["◂", "1", "2"] + arrows + tail)
        XCTAssertEqual(try labels("p,y;"), ["◂", "p", "y"] + arrows + tail)
        XCTAssertEqual(try labels(";c,m,g,s,x,z"), ["◂"] + arrows + tail)
    }

    /// The sidebar console's belt: `◂`, the chosen session's answers, `↑ ↓ ⏎`, then `c copy reply`,
    /// `⌃o` and the list's keys. The board's column and page keys are gone.
    func testBeltHasTheSidebarKeys() throws {
        XCTAssertEqual(try labels("1,2,3;"), ["◂", "1", "2", "3", "↑", "↓", "⏎", "c copy reply", "⌃o", "s hosts",
                                             "g chain", "/", "m menu", "z park", "N new", "? keys", ", machine",
                                             "t shell", "D review", "x kill"])
        let ids = board(";").keys.map(\.id)
        for gone in ["board.v", "board.u", "board.]", "board.[", "board.tab", "←", "→"] {
            XCTAssertFalse(ids.contains(gone), gone)
        }
        // A console that still says the old keys draws none of them.
        XCTAssertEqual(try labels(";v,u,left,right,],["), ["◂"] + arrows + tail)
        XCTAssertNil(BoardBelt.bytes(for: "board.v"))
        XCTAssertNil(BoardBelt.bytes(for: "board.]"))
        // Each act key sends its own tap (keycmd.TapKeys and keycmd.TapKey in Go), which no
        // keyboard sends. machine taps `,` (the key), not the tok.
        for key in ["p", "y", "s", "g", "/", "m", "c", "z", "N", "?", "t", "D", "x"] {
            XCTAssertEqual(BoardBelt.bytes(for: "board.\(key)"), Array("\u{1b}_matchblox;tap=\(key)\u{1b}\\".utf8), key)
        }
        XCTAssertEqual(BoardBelt.bytes(for: "board.machine"), Array("\u{1b}_matchblox;tap=,\u{1b}\\".utf8))
    }

    /// While the chat's line holds text the console says the digits and the arrows are off.
    func testTheLinesTextDimsTheDigitsAndTheArrows() {
        let b = board(";1,2,up,down")
        XCTAssertEqual(b.keys.map(\.id).prefix(4), ["◂", "board.1", "board.2", "↑"])
        XCTAssertTrue(b.isDim("board.1"))
        XCTAssertTrue(b.isDim("board.2"))
        XCTAssertTrue(b.isDim("↑"))
        XCTAssertTrue(b.isDim("↓"))
        XCTAssertFalse(b.isDim("⏎"))
        XCTAssertFalse(b.isDim("board.c"))
    }

    func testAtMostThreeCardKeys() {
        XCTAssertEqual(board("1,2,3,p;").card, ["1", "2", "3"])
        XCTAssertEqual(ids("1,2,3;").prefix(5), ["◂", "board.1", "board.2", "board.3", "↑"])
    }

    func testTheCardsKeysAreLitAndTheArmedOneIsMarked() {
        let b = board("1,2*;")
        XCTAssertEqual(b.armed, "2")
        XCTAssertTrue(b.isLit("board.1"))
        XCTAssertTrue(b.isLit("board.2"))
        XCTAssertFalse(b.isArmed("board.1"))
        XCTAssertTrue(b.isArmed("board.2"))
        XCTAssertFalse(b.isLit("board.c"))
        XCTAssertFalse(b.isLit("↑"))
        XCTAssertNil(board("1,2;").armed)
    }

    func testAKeyThatIsOffIsDim() {
        let b = board("p;g,x,ctrl+o")
        XCTAssertTrue(b.isDim("board.g"))
        XCTAssertTrue(b.isDim("board.x"))
        XCTAssertTrue(b.isDim("board.ctrl+o"))
        XCTAssertFalse(b.isDim("board.N"))
        XCTAssertFalse(b.isDim("board.c"))
        XCTAssertFalse(b.isDim("board.p"))
        XCTAssertFalse(b.isDim("◂"))
        XCTAssertFalse(b.isDim("↑"))
    }

    /// A card on another host: its digits come in the dim list, and stay in the card's place.
    func testTheCardsKeysThatAreOffStayInTheirPlaceDim() {
        let b = board(";1,2,c")
        XCTAssertEqual(b.keys.map(\.id).prefix(4), ["◂", "board.1", "board.2", "↑"])
        XCTAssertTrue(b.isDim("board.1"))
        XCTAssertTrue(b.isDim("board.2"))
        XCTAssertFalse(b.isLit("board.1"))
        XCTAssertTrue(b.isDim("board.c"))
    }

    func testMachineDimsTheTailButMachine() {
        let b = board(";ctrl+o,s,g,/,m,c,z,N,?,t,D,x")
        let dim = b.keys.map(\.id).filter(b.isDim)
        XCTAssertEqual(dim, ["board.c", "board.ctrl+o", "board.s", "board.g", "board./", "board.m", "board.z",
                             "board.N", "board.?", "board.t", "board.D", "board.x"])
        XCTAssertFalse(b.isDim("board.machine"))
    }

    func testTheConsentBoxAndTheMenu() throws {
        XCTAssertEqual(try labels("box"), ["◂", "y runs", "esc cancels"])
        XCTAssertTrue(board("box").isLit("board.runs"))
        XCTAssertEqual(try labels("menu"), ["◂", "↑", "↓", "⏎ do", "esc close"])
        XCTAssertFalse(board("menu").keys.contains { board("menu").isDim($0.id) })
    }

    func testAnUnknownTokenIsNotShown() throws {
        XCTAssertEqual(try labels("1,q,c;w,zz"), ["◂", "1"] + arrows + tail)
        XCTAssertTrue(board("1;w").dim.contains("w"))
        XCTAssertFalse(board("1;w").keys.contains { $0.id.hasSuffix("w") })
    }

    func testAValueTheAppCannotReadIsHomeWithEveryKeyOn() throws {
        XCTAssertEqual(board("what"), .plain)
        XCTAssertEqual(try labels("what"), ["◂"] + arrows + tail)
        XCTAssertNil(BoardBelt(said: ""))
    }

    func testTheBoardLayoutIsInTheCatalog() throws {
        let c = try catalog()
        XCTAssertEqual(c.keys(.board, agent: nil, mine: []), BoardBelt.plain.keys)
        XCTAssertEqual(c.keys(.board, agent: "claude", mine: [], board: board("1,2;")), board("1,2;").keys)
        XCTAssertEqual(KeyFace(BeltKey(id: "board.c", cells: 2), in: c).size, .word)
        XCTAssertEqual(KeyFace(BeltKey(id: "board./", cells: 1), in: c).size, .glyph)
        XCTAssertEqual(KeyFace(BeltKey(id: "board.ctrl+o", cells: 2), in: c).size, .word)
        XCTAssertEqual(KeyFace(BeltKey(id: "board.1", cells: 1), in: c).size, .glyph)
    }

    /// Design decision 10: the belt is seven cells wide at rest, and home is ◂, the card's keys,
    /// then ↑ ↓ ⏎. A card key is one cell, its glyph only, and so are the arrows and Return.
    func testHomeFitsTheBeltAtRest() {
        let three = board("1,2,3;").keys
        XCTAssertEqual(three.prefix(7).map(\.id), ["◂", "board.1", "board.2", "board.3", "↑", "↓", "⏎"])
        XCTAssertEqual(three.prefix(7).map(\.cells), [1, 1, 1, 1, 1, 1, 1])
        XCTAssertEqual(board("p;").keys.prefix(6).map(\.id), ["◂", "board.p", "↑", "↓", "⏎", "board.c"])
        XCTAssertEqual(board("menu").keys.map(\.cells), [1, 1, 1, 2, 2])
        XCTAssertEqual(board("box").keys.map(\.cells), [1, 2, 2])
    }

    func testVoiceOverSaysTheWords() throws {
        let c = try catalog()
        let spoken = ["board.1", "board.p", "board.y", "board.machine", "board.?", "board.t", "board.D", "board.ctrl+o", "board.N", "board./", "board.c", "board.runs", "◂"]
            .map { KeyFace(BeltKey(id: $0, cells: 1), in: c).spoken }
        XCTAssertEqual(spoken, ["1, answer", "p, merge", "y, go", "machine", "keys", "shell", "review", "expand", "new session", "find", "copy the last reply", "y, run", "Back to matchblox"])
    }

    // MARK: Away from the board

    func testAwayEveryKeyButBackIsDim() {
        let away = BoardBelt.away
        XCTAssertEqual(away.keys.map(\.id), BoardBelt.plain.keys.map(\.id))
        XCTAssertEqual(away.keys.map(\.id).filter { !away.isDim($0) }, ["◂"])
        XCTAssertFalse(away.keys.contains { away.isLit($0.id) })
    }

    func testTheBeltIsAwayWithNoValueOrInAPane() {
        var follow = BoardFollow()
        XCTAssertEqual(follow.belt, .away)
        _ = follow.heard("1,2;", on: .board)
        XCTAssertEqual(follow.belt, board("1,2;"))
        _ = follow.pane(true, on: .board)
        XCTAssertEqual(follow.belt, .away)
        _ = follow.pane(false, on: .board)
        XCTAssertEqual(follow.belt, board("1,2;"))
        _ = follow.heard("", on: .board)
        XCTAssertEqual(follow.belt, .away)
        _ = follow.heard("what", on: .board)
        XCTAssertEqual(follow.belt, .plain)
    }

    // MARK: The terminal's touches

    func testTheLongPressIsTheMenuOnlyWhileTheConsoleShows() {
        var follow = BoardFollow()
        XCTAssertFalse(follow.opensMenu)
        _ = follow.heard("1,2;", on: .steer)
        XCTAssertTrue(follow.opensMenu)
        _ = follow.pane(true, on: .board)
        XCTAssertFalse(follow.opensMenu)
        _ = follow.pane(false, on: .steer)
        XCTAssertTrue(follow.opensMenu)
        _ = follow.heard("", on: .board)
        XCTAssertFalse(follow.opensMenu)
    }

    /// While the chat takes the screen the hold selects text: the row's menu is not there (#485).
    func testTheLongPressSelectsWhileTheChatTakesTheScreen() throws {
        var follow = BoardFollow()
        _ = follow.heard("1,2;;chat", on: .board)
        XCTAssertTrue(follow.shows)
        XCTAssertFalse(follow.opensMenu)
        _ = follow.heard("1,2;", on: .board)
        XCTAssertTrue(follow.opensMenu)
        XCTAssertEqual(try labels("1,2;c;chat"), try labels("1,2;c"))
        XCTAssertTrue(board("1,2;c;chat").chat)
        XCTAssertFalse(board("1,2;c").chat)
    }

    // MARK: What each key sends

    /// An act key sends its tap, which its tmux binding hands the console (`beltTap` in Go); the
    /// digits, `⌃o`, the arrows and Return send as a keyboard does.
    func testEachKeySendsWhatTheConsolesBindingReads() {
        let esc: UInt8 = 0x1b
        func tap(_ key: String) -> [UInt8] { Array("\u{1b}_matchblox;tap=\(key)\u{1b}\\".utf8) }
        let sent: [String: [UInt8]] = [
            "board.1": [0x31], "board.2": [0x32], "board.3": [0x33], "board.p": tap("p"),
            "board.y": tap("y"), "board.c": tap("c"), "board.m": tap("m"), "board.g": tap("g"),
            "board.s": tap("s"), "board./": tap("/"), "board.z": tap("z"), "board.N": tap("N"),
            "board.?": tap("?"), "board.machine": tap(","), "board.t": tap("t"), "board.D": tap("D"),
            "board.x": tap("x"), "board.ctrl+o": [0x0f],
            "board.runs": Array("y".utf8), "board.cancels": [esc], "board.do": [0x0d], "board.close": [esc],
            "↑": [esc, 0x5b, 0x41], "↓": [esc, 0x5b, 0x42], "⏎": [0x0d], "◂": HomeKey.bytes,
        ]
        for (id, bytes) in sent {
            XCTAssertEqual(KeyBar.bytes(for: id), bytes, id)
        }
        for value in ["1,2,3;", "p,y;", "box", "menu", ";"] {
            for key in board(value).keys {
                XCTAssertNotNil(sent[key.id], key.id)
            }
        }
        XCTAssertNil(BoardBelt.bytes(for: "board.w"))
        XCTAssertNil(BoardBelt.bytes(for: "c"))
    }

    func testTheAnswerIsTheConsolesBeltKey() {
        // keycmd.BeltKey in Go.
        XCTAssertEqual(BoardBelt.answer, Array("\u{1b}_matchblox;belt\u{1b}\\".utf8))
    }

    // MARK: Following the console

    func testTheBeltTurnsToBoardWhenTheConsoleShowsTheBoard() {
        var follow = BoardFollow()
        XCTAssertFalse(follow.answers(on: .commands))
        XCTAssertEqual(follow.heard("1,2;", on: .commands), .board)
        XCTAssertTrue(follow.answers(on: .board))
        XCTAssertEqual(follow.said, board("1,2;"))
        // A new value, or the same again, keeps Board; each is answered.
        XCTAssertNil(follow.heard("1,2*;", on: .board))
        XCTAssertNil(follow.heard("1,2*;", on: .board))
        XCTAssertTrue(follow.answers(on: .board))
        XCTAssertNil(follow.heard("box", on: .board))
        XCTAssertTrue(follow.answers(on: .board))
    }

    func testTheBeltGoesBackWhenTheConsoleLeavesTheBoard() {
        var follow = BoardFollow()
        XCTAssertEqual(follow.heard(";", on: .tmux), .board)
        XCTAssertEqual(follow.heard("", on: .board), .tmux)
        XCTAssertNil(follow.said)
        XCTAssertFalse(follow.answers(on: .board))
    }

    func testTheBeltGoesBackToThePanesLayoutWhenTheOwnerGoesIntoAPane() {
        var follow = BoardFollow()
        XCTAssertEqual(follow.heard("p;", on: .commands), .board)
        XCTAssertEqual(follow.pane(true, on: .board), .commands)
        XCTAssertFalse(follow.answers(on: .board))
        // A value in the pane is held, and not answered.
        XCTAssertNil(follow.heard("p;", on: .commands))
        XCTAssertFalse(follow.answers(on: .commands))
        // Back on the console's board, the belt is on Board again.
        XCTAssertEqual(follow.pane(false, on: .commands), .board)
        XCTAssertTrue(follow.answers(on: .board))
    }

    func testTheOwnersPickStays() {
        var follow = BoardFollow()
        XCTAssertEqual(follow.heard("1;", on: .steer), .board)
        follow.picked()
        // The owner picked Mine: no answer, and the board does not take the belt back.
        XCTAssertFalse(follow.answers(on: .mine))
        XCTAssertNil(follow.heard("1,2;", on: .mine))
        XCTAssertFalse(follow.answers(on: .mine))
        XCTAssertNil(follow.heard("", on: .mine))
        // The console shows the board again: the belt turns to Board, and goes back to Mine.
        XCTAssertEqual(follow.heard(";", on: .mine), .board)
        XCTAssertTrue(follow.answers(on: .board))
        XCTAssertEqual(follow.heard("", on: .board), .mine)
    }

    func testAPickOfBoardWhileTheConsoleShowsTheBoardIsAnswered() {
        var follow = BoardFollow()
        XCTAssertEqual(follow.heard("1;", on: .steer), .board)
        follow.picked()
        XCTAssertFalse(follow.answers(on: .steer))
        follow.picked()
        XCTAssertTrue(follow.answers(on: .board))
    }

    func testABoardTheOwnerChoseStaysWhenTheConsoleLeaves() {
        var follow = BoardFollow()
        XCTAssertNil(follow.heard("1;", on: .board))
        XCTAssertNil(follow.heard("", on: .board))
        XCTAssertFalse(follow.answers(on: .board))
    }

    /// A console key on the builder's own layout follows Board's rule: away from the console,
    /// `x kill` and the rest are dim and send nothing into an agent's pane.
    func testAwayDimsConsoleKeys() {
        for key in BoardBelt.tailKeys {
            XCTAssertTrue(BoardBelt.away.isDim(key.id), key.id)
            XCTAssertFalse(BoardBelt.plain.isDim(key.id), key.id)
        }
    }
}
