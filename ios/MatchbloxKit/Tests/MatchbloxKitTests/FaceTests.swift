import XCTest
@testable import MatchbloxKit

final class FaceTests: XCTestCase {
    let now = Date(timeIntervalSince1970: 1_800_000_000)

    func state(host: String = "f10f7812", stoker: Bool = false, pcts: [Int] = []) -> IslandState {
        IslandState(host: host, burn: 0, wait: 0, top: nil,
                    accounts: [AccountUse(n: 1, windows: pcts.enumerated().map { WindowUse(name: "w\($0.offset)", pct: $0.element, resets: nil) }, lasts: true, runsOut: nil)],
                    stoker: stoker)
    }

    func testAStateOfAHostNotFollowedShowsNothing() {
        let s = state()
        XCTAssertEqual(face(for: s, show: .sessions, followed: "a30534a5", now: now), .nothing)
        XCTAssertEqual(face(for: s, show: .usage, followed: nil, now: now), .nothing)
        XCTAssertEqual(face(for: s, show: .sessions, followed: "f10f7812", now: now), .sessions)
        XCTAssertEqual(face(for: s, show: .usage, followed: "f10f7812", now: now), .usage)
    }

    func testAutoShowsUsageWhileTheStokerRuns() {
        let f = "f10f7812"
        XCTAssertEqual(face(for: state(stoker: true), show: .auto, followed: f, now: now), .usage)
        XCTAssertEqual(face(for: state(stoker: false), show: .auto, followed: f, now: now), .sessions)
    }

    func testAWaitBreaksThroughFor60s() {
        var s = state(stoker: true)
        s.waitAt = now
        let f = "f10f7812"
        XCTAssertEqual(face(for: s, show: .usage, followed: f, now: now), .sessions)
        XCTAssertEqual(face(for: s, show: .auto, followed: f, now: now.addingTimeInterval(59)), .sessions)
        XCTAssertEqual(face(for: s, show: .usage, followed: f, now: now.addingTimeInterval(60)), .usage)
        XCTAssertEqual(face(for: s, show: .usage, followed: "other", now: now), .nothing)
        XCTAssertEqual(breakThroughEnds(s), now.addingTimeInterval(60))
        XCTAssertNil(breakThroughEnds(state()))
    }

    func testTheTightestWindowWins() {
        let s = state(pcts: [31, 93, 71])
        XCTAssertEqual(tightest(s)?.pct, 93)
        XCTAssertNil(tightest(state()))
    }

    /// An idle Cursor login at 100% of its month kept the watch red while Claude worked (2026-11-26).
    func testAnIdleAccountDoesNotLeadTheGlance() throws {
        var s = state()
        s.accounts = [
            AccountUse(n: 1, windows: [WindowUse(name: "wk", pct: 24, resets: nil)], lasts: true),
            AccountUse(n: 2, windows: [WindowUse(name: "mo", pct: 100, resets: nil)], lasts: true, idle: true),
        ]
        XCTAssertEqual(tightest(s)?.pct, 24)
        s.accounts[0].idle = true
        XCTAssertNil(tightest(s))
        let old = try JSONDecoder().decode(AccountUse.self, from: Data(#"{"n":1,"windows":[],"lasts":true}"#.utf8))
        XCTAssertFalse(old.idle)
    }

    /// Cursor went on past 100 % of its included month, and that bar led the glance in red
    /// over Claude's 30 % week (2026-11-27). A spent window stops nothing.
    func testASpentWindowNeitherLeadsNorWarns() throws {
        var s = state()
        var month = WindowUse(name: "mo", pct: 100, resets: nil)
        month.spent = true
        s.accounts = [
            AccountUse(n: 1, windows: [WindowUse(name: "wk", pct: 30, resets: nil)], lasts: true),
            AccountUse(n: 2, windows: [month], lasts: true),
        ]
        XCTAssertEqual(tightest(s)?.name, "wk")
        XCTAssertEqual(level(month), .normal)
        XCTAssertEqual(level(WindowUse(name: "wk", pct: 95, resets: nil)), .neg)
        let old = try JSONDecoder().decode(WindowUse.self, from: Data(#"{"name":"mo","pct":100}"#.utf8))
        XCTAssertFalse(old.spent)
    }

    func testBarsWarnAt60AndNegAt90() {
        XCTAssertEqual(barColorLevel(0), .normal)
        XCTAssertEqual(barColorLevel(59), .normal)
        XCTAssertEqual(barColorLevel(60), .warn)
        XCTAssertEqual(barColorLevel(89), .warn)
        XCTAssertEqual(barColorLevel(90), .neg)
        XCTAssertEqual(barColorLevel(100), .neg)
    }
}
