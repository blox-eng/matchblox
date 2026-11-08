import XCTest
@testable import MatchbloxKit

final class InputMarkTests: XCTestCase {
    func testAReturnAndAPressAreMarked() {
        XCTAssertTrue(InputMark.marks(ArraySlice([0x0d])))
        XCTAssertTrue(InputMark.marks(ArraySlice(Array("\u{1b}[<0;20;6M".utf8))))
        XCTAssertTrue(InputMark.marks(ArraySlice([0x1b, 0x5b, 0x4d, 32, 52, 38])))
    }

    func testOtherBytesAreNot() {
        for other in ["a", "2", "\u{1b}[A", "\u{1b}[<0;20;6m", "\u{1b}[<64;20;6M", "\u{1b}_matchblox\u{1b}\\"] {
            XCTAssertFalse(InputMark.marks(ArraySlice(Array(other.utf8))), other)
        }
    }

    /// keycmd.MarkKey in Go.
    func testTheMarkIsTheConsolesKey() {
        XCTAssertEqual(InputMark.bytes, Array("\u{1b}_matchblox;mark\u{1b}\\".utf8))
    }

    /// T15.
    func testAnOwnJumpKeepsItsAgent() {
        var p = PaneAgent()
        let t0 = ContinuousClock.now
        p.marked(at: t0)
        p.signal(ConsoleSignal("claude;pane=%12"), at: t0 + .milliseconds(200))
        XCTAssertEqual(p.agent, "claude")
        p.left()
        XCTAssertNil(p.agent)
    }

    /// T15: no mark of ours came before the other screen's jump.
    func testAnotherScreensJumpIsIgnored() {
        var p = PaneAgent()
        let t0 = ContinuousClock.now
        p.signal(ConsoleSignal("codex;pane=%3"), at: t0)
        XCTAssertNil(p.agent)
        p.marked(at: t0)
        p.signal(ConsoleSignal("claude;pane=%12"), at: t0 + .milliseconds(100))
        p.signal(ConsoleSignal("codex;pane=%3"), at: t0 + .milliseconds(150))
        XCTAssertEqual(p.agent, "claude", "the mark is spent")
    }

    func testALateSignalIsIgnored() {
        var p = PaneAgent()
        let t0 = ContinuousClock.now
        p.marked(at: t0)
        p.signal(ConsoleSignal("claude;pane=%12"), at: t0 + .milliseconds(1001))
        XCTAssertNil(p.agent)
    }

    func testASignalWithNoPaneKeepsTheMark() {
        var p = PaneAgent()
        let t0 = ContinuousClock.now
        p.marked(at: t0)
        p.signal(ConsoleSignal(";marks"), at: t0 + .milliseconds(50))
        p.signal(ConsoleSignal("claude;pane=%12"), at: t0 + .milliseconds(100))
        XCTAssertEqual(p.agent, "claude")
    }

    /// An own jump into claude's pane at t0, whose way back shows from t0 + 100 ms.
    private func inClaudesPane(at t0: ContinuousClock.Instant) -> PaneAgent {
        var p = PaneAgent()
        p.marked(at: t0)
        p.signal(ConsoleSignal("claude;pane=%12"), at: t0 + .milliseconds(50))
        XCTAssertFalse(p.shows(pane: true, at: t0 + .milliseconds(100)))
        return p
    }

    func testAMomentWithNoWayBackKeepsTheAgent() {
        let t0 = ContinuousClock.now
        var p = inClaudesPane(at: t0)
        XCTAssertFalse(p.shows(pane: false, at: t0 + .milliseconds(200)))
        XCTAssertTrue(p.leaving)
        XCTAssertFalse(p.shows(pane: true, at: t0 + .milliseconds(300)))
        XCTAssertFalse(p.leaving)
        XCTAssertFalse(p.shows(pane: false, at: t0 + .milliseconds(400)))
        XCTAssertFalse(p.shows(pane: false, at: t0 + .milliseconds(650)))
        XCTAssertEqual(p.agent, "claude")
    }

    func testBackOnTheConsoleTheAgentGoes() {
        let t0 = ContinuousClock.now
        var p = inClaudesPane(at: t0)
        XCTAssertFalse(p.shows(pane: false, at: t0 + .milliseconds(200)))
        XCTAssertTrue(p.shows(pane: false, at: t0 + .milliseconds(200) + PaneAgent.settle))
        XCTAssertNil(p.agent)
        XCTAssertFalse(p.leaving)
    }

    func testTheConsoleBeforeTheJumpLandsKeepsTheAgent() {
        var p = PaneAgent()
        let t0 = ContinuousClock.now
        p.marked(at: t0)
        p.signal(ConsoleSignal("claude;pane=%12"), at: t0)
        XCTAssertFalse(p.shows(pane: false, at: t0))
        XCTAssertFalse(p.shows(pane: false, at: t0 + .seconds(2)))
        XCTAssertFalse(p.leaving)
        XCTAssertEqual(p.agent, "claude")
    }

    func testANewJumpFromTheConsoleKeepsItsAgent() {
        let t0 = ContinuousClock.now
        var p = inClaudesPane(at: t0)
        XCTAssertFalse(p.shows(pane: false, at: t0 + .milliseconds(200)))
        p.marked(at: t0 + .milliseconds(250))
        p.signal(ConsoleSignal("codex;pane=%3"), at: t0 + .milliseconds(300))
        XCTAssertFalse(p.shows(pane: false, at: t0 + .milliseconds(700)))
        XCTAssertEqual(p.agent, "codex")
    }
}
