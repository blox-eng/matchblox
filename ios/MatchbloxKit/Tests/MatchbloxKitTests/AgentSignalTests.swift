import XCTest
@testable import MatchbloxKit

final class AgentSignalTests: XCTestCase {
    /// keycmd.AgentSignal in Go.
    private func signal(_ agent: String) -> [UInt8] { Array("\u{1b}_matchblox;agent=\(agent)\u{1b}\\".utf8) }
    private func bytes(_ s: String) -> [UInt8] { Array(s.utf8) }

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

    func testNoSignalNoAgent() {
        var f = AgentSignalFilter()
        let out = f.feed(ArraySlice(bytes("plain")))
        XCTAssertEqual(out.pass, bytes("plain"))
        XCTAssertNil(out.agent)
    }
}
