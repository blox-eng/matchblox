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
    private let tail = [
        "c copy", "m menu", "g chain", "v one line", "s hosts", "u strip", "⇥ machine", "x kill",
        "←", "→", "]", "[", "/", "z park",
    ]

    // MARK: The keys

    func testHomeIsBackTheCardsKeysAndTheArrows() throws {
        XCTAssertEqual(try labels("1,2;"), ["◂", "1 answer", "2 answer"] + arrows + tail)
        XCTAssertEqual(try labels("p,y;"), ["◂", "p merge", "y go"] + arrows + tail)
        XCTAssertEqual(try labels(";c,m,g,s,x,],[,z"), ["◂"] + arrows + tail)
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
        let b = board("p;g,x,left")
        XCTAssertTrue(b.isDim("board.g"))
        XCTAssertTrue(b.isDim("board.x"))
        XCTAssertTrue(b.isDim("←"))
        XCTAssertFalse(b.isDim("→"))
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
        let b = board(";c,m,g,v,s,u,x,left,right,],[,/,z")
        let dim = b.keys.map(\.id).filter(b.isDim)
        XCTAssertEqual(dim, ["board.c", "board.m", "board.g", "board.v", "board.s", "board.u", "board.x",
                             "←", "→", "board.]", "board.[", "board./", "board.z"])
        XCTAssertFalse(b.isDim("board.tab"))
    }

    func testTheConsentBoxAndTheMenu() throws {
        XCTAssertEqual(try labels("box"), ["◂", "y runs", "esc cancels"])
        XCTAssertTrue(board("box").isLit("board.runs"))
        XCTAssertEqual(try labels("menu"), ["◂", "↑", "↓", "⏎ do", "esc close"])
        XCTAssertFalse(board("menu").keys.contains { board("menu").isDim($0.id) })
    }

    func testAnUnknownTokenIsNotShown() throws {
        XCTAssertEqual(try labels("1,q,c;w,zz"), ["◂", "1 answer"] + arrows + tail)
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
        XCTAssertEqual(board("1,2;").keys.map(\.cells).prefix(6), [1, 2, 2, 2, 2, 2])
        XCTAssertEqual(KeyFace(BeltKey(id: "board.c", cells: 2), in: c).size, .word)
        XCTAssertEqual(KeyFace(BeltKey(id: "board.]", cells: 1), in: c).size, .glyph)
    }

    // MARK: What each key sends

    func testEachKeySendsWhatTheConsolesBindingReads() {
        let esc: UInt8 = 0x1b
        let sent: [String: [UInt8]] = [
            "board.1": [0x31], "board.2": [0x32], "board.3": [0x33], "board.p": Array("p".utf8),
            "board.y": Array("y".utf8), "board.c": Array("c".utf8), "board.m": Array("m".utf8),
            "board.g": Array("g".utf8), "board.v": Array("v".utf8), "board.s": Array("s".utf8),
            "board.u": Array("u".utf8), "board.tab": [0x09], "board.x": Array("x".utf8),
            "board.]": Array("]".utf8), "board.[": Array("[".utf8), "board./": Array("/".utf8),
            "board.z": Array("z".utf8), "board.runs": Array("y".utf8), "board.cancels": [esc],
            "board.do": [0x0d], "board.close": [esc],
            "←": [esc, 0x5b, 0x44], "→": [esc, 0x5b, 0x43], "↑": [esc, 0x5b, 0x41], "↓": [esc, 0x5b, 0x42],
            "⏎": [0x0d], "◂": HomeKey.bytes,
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
}
