import XCTest
@testable import MatchbloxKit

final class AgentSignalTests: XCTestCase {
    /// keycmd.AgentSignal in Go.
    private func signal(_ agent: String) -> [UInt8] { Array("\u{1b}_matchblox;agent=\(agent)\u{1b}\\".utf8) }
    private func bytes(_ s: String) -> [UInt8] { Array(s.utf8) }
    /// keycmd.BoardSignal in Go.
    private func board(_ keys: String) -> [UInt8] { Array("\u{1b}_matchblox;board=\(keys)\u{1b}\\".utf8) }

    func testAgentSignalTakenOut() {
        var f = AgentSignalFilter()
        let out = f.feed(ArraySlice(bytes("ab") + signal("codex") + bytes("\u{1b}[1mcd")))
        XCTAssertEqual(out.pass, bytes("ab\u{1b}[1mcd"))
        XCTAssertEqual(out.agent, "codex")
    }

    func testTheLastSignalOfAReadWins() {
        var f = AgentSignalFilter()
        let out = f.feed(ArraySlice(signal("claude") + bytes("x") + signal("cursor")))
        XCTAssertEqual(out.pass, bytes("x"))
        XCTAssertEqual(out.agent, "cursor")
    }

    func testAgentSignalSplitAcrossReads() {
        let stream = bytes("hi ") + signal("opencode") + bytes(" there")
        for cut in 1..<stream.count {
            var f = AgentSignalFilter()
            let a = f.feed(stream[..<cut])
            let b = f.feed(stream[cut...])
            XCTAssertEqual(a.pass + b.pass, bytes("hi  there"), "cut at \(cut)")
            XCTAssertEqual(a.agent ?? b.agent, "opencode", "cut at \(cut)")
            XCTAssertFalse(a.agent != nil && b.agent != nil, "cut at \(cut)")
        }

        var f = AgentSignalFilter()
        var pass: [UInt8] = []
        var agent: String?
        for byte in stream {
            let out = f.feed([byte])
            pass += out.pass
            agent = out.agent ?? agent
        }
        XCTAssertEqual(pass, bytes("hi  there"))
        XCTAssertEqual(agent, "opencode")
    }

    func testOtherBytesPassUntouched() {
        let others: [[UInt8]] = [
            HomeKey.bytes,
            bytes("\u{1b}_matchblox;cmd=compact\u{1b}\\"),
            bytes("\u{1b}_matchblox;agentx\u{1b}\\"),
            bytes("\u{1b}_matchbl\u{1b}[A"),
            bytes("\u{1b}\u{1b}[B\u{1b}"),
            bytes("\u{1b}]0;title\u{07}plain ✓ text"),
            bytes("\u{1b}_matchblox;agent=cl\u{1b}x"),
            bytes("\u{1b}_matchblox;agent=" + String(repeating: "a", count: 200) + "\u{1b}\\"),
            Array(0...255),
        ]
        for other in others {
            var f = AgentSignalFilter()
            let a = f.feed(ArraySlice(other))
            let b = f.feed(ArraySlice(bytes("z")))
            XCTAssertEqual(a.pass + b.pass, other + bytes("z"), String(decoding: other, as: UTF8.self))
            XCTAssertNil(a.agent)
            XCTAssertNil(b.agent)
        }
    }

    func testAFalseStartThenASignal() {
        var f = AgentSignalFilter()
        let out = f.feed(ArraySlice(bytes("\u{1b}_matchblox;agen") + signal("claude")))
        XCTAssertEqual(out.pass, bytes("\u{1b}_matchblox;agen"))
        XCTAssertEqual(out.agent, "claude")
    }

    func testEmptyAgent() {
        var f = AgentSignalFilter()
        let out = f.feed(ArraySlice(signal("")))
        XCTAssertEqual(out.pass, [])
        XCTAssertEqual(out.agent, "")
    }

    func testTheSignalNamesItsPaneAndTheMarks() {
        XCTAssertEqual(ConsoleSignal("claude;pane=%12"), ConsoleSignal(agent: "claude", pane: "%12", marks: false))
        XCTAssertEqual(ConsoleSignal(";marks"), ConsoleSignal(agent: "", pane: nil, marks: true))
        // An old console never announces marks, so the app never marks (T20).
        XCTAssertEqual(ConsoleSignal("claude"), ConsoleSignal(agent: "claude", pane: nil, marks: false))
        XCTAssertEqual(ConsoleSignal(""), ConsoleSignal(agent: "", pane: nil, marks: false))
    }

    func testTheFilterPassesTheWholeSignal() {
        var f = AgentSignalFilter()
        XCTAssertEqual(f.feed(ArraySlice(signal("claude;pane=%12"))).agent, "claude;pane=%12")
        XCTAssertEqual(f.feed(ArraySlice(signal(";marks"))).agent, ";marks")
    }

    func testNoSignalNoAgent() {
        var f = AgentSignalFilter()
        let out = f.feed(ArraySlice(bytes("plain")))
        XCTAssertEqual(out.pass, bytes("plain"))
        XCTAssertNil(out.agent)
    }

    // MARK: The board signal

    func testBoardSignalTakenOut() {
        var f = AgentSignalFilter()
        let out = f.feed(ArraySlice(bytes("ab") + board("1,2*;c,m") + bytes("cd")))
        XCTAssertEqual(out.pass, bytes("abcd"))
        XCTAssertEqual(out.board, "1,2*;c,m")
        XCTAssertNil(out.agent)
    }

    func testTheEmptyBoardValueMeansTheConsoleLeft() {
        var f = AgentSignalFilter()
        let out = f.feed(ArraySlice(board("")))
        XCTAssertEqual(out.pass, [])
        XCTAssertEqual(out.board, "")
    }

    func testBothSignalsInOneRead() {
        var f = AgentSignalFilter()
        let out = f.feed(ArraySlice(signal("claude") + bytes("x") + board("menu") + board("box")))
        XCTAssertEqual(out.pass, bytes("x"))
        XCTAssertEqual(out.agent, "claude")
        XCTAssertEqual(out.board, "box")
    }

    func testBoardSignalSplitAcrossReads() {
        let value = ";c,m,g,v,s,u,x,left,right,],[,/,z"
        let stream = bytes("hi ") + board(value) + bytes(" there")
        for cut in 1..<stream.count {
            var f = AgentSignalFilter()
            let a = f.feed(stream[..<cut])
            let b = f.feed(stream[cut...])
            XCTAssertEqual(a.pass + b.pass, bytes("hi  there"), "cut at \(cut)")
            XCTAssertEqual(a.board ?? b.board, value, "cut at \(cut)")
            XCTAssertNil(a.agent ?? b.agent, "cut at \(cut)")
        }
    }

    /// The console's beltMax: 48 printable bytes pass, 49 are no signal.
    func testABoardValueHasAtMost48Bytes() {
        let longest = String(repeating: "c", count: 48)
        var f = AgentSignalFilter()
        XCTAssertEqual(f.feed(ArraySlice(board(longest))).board, longest)

        let over = board(longest + "c")
        var g = AgentSignalFilter()
        let a = g.feed(ArraySlice(over))
        let b = g.feed(ArraySlice(bytes("z")))
        XCTAssertEqual(a.pass + b.pass, over + bytes("z"))
        XCTAssertNil(a.board)
        XCTAssertNil(b.board)
    }

    func testTheAgentKeepsItsOwnLimit() {
        let id = String(repeating: "a", count: 33)
        var f = AgentSignalFilter()
        let out = f.feed(ArraySlice(signal(id)))
        XCTAssertEqual(out.pass, signal(id))
        XCTAssertNil(out.agent)
    }

    func testABoardValueWithAByteThatIsNotPrintableIsNoSignal() {
        let others: [[UInt8]] = [
            bytes("\u{1b}_matchblox;board=1 2;\u{1b}\\"),
            bytes("\u{1b}_matchblox;board=1;\u{1b}x"),
            bytes("\u{1b}_matchblox;boards\u{1b}\\"),
            bytes("\u{1b}_matchblox;belt\u{1b}\\"),
        ]
        for other in others {
            var f = AgentSignalFilter()
            let a = f.feed(ArraySlice(other))
            let b = f.feed(ArraySlice(bytes("z")))
            XCTAssertEqual(a.pass + b.pass, other + bytes("z"), String(decoding: other, as: UTF8.self))
            XCTAssertNil(a.board ?? b.board)
        }
    }
}
