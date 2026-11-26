import XCTest
@testable import MatchbloxKit
import struct MatchbloxKit.Host

final class HomeCopyTests: XCTestCase {
    private func host(_ label: String, demo: Bool = false) -> Host {
        if demo { return .demo }
        return Host(label: label, address: "\(label).local", port: 22, user: "dev", fingerprint: "SHA256:\(label)")
    }

    func testHostRowSpokenLive() {
        let line = HomeCopy.hostRowSpoken(
            label: "work-1",
            address: "10.0.0.4",
            status: .live
        )
        XCTAssertTrue(line.contains("work-1"), line)
        XCTAssertTrue(line.contains("10.0.0.4"), line)
        XCTAssertTrue(line.contains("live"), line)
    }

    func testHostRowSpokenNotReachable() {
        let since = Date(timeIntervalSince1970: 1_700_000_000) // fixed
        let line = HomeCopy.hostRowSpoken(
            label: "work-1",
            address: "10.0.0.4",
            status: .notReachable(since: since),
            now: since
        )
        XCTAssertTrue(line.contains("not reachable since"), line)
    }

    func testHostRowSpokenLastSeen() {
        let now = Date(timeIntervalSince1970: 10_000)
        let seen = Date(timeIntervalSince1970: 10_000 - 2 * 3600)
        let line = HomeCopy.hostRowSpoken(
            label: "laptop",
            address: "laptop.local",
            status: .lastSeen(at: seen),
            now: now
        )
        XCTAssertTrue(line.contains("2h ago"), line)
    }

    func testRemoveQuestionLive() {
        let q = HomeCopy.removeQuestion(label: "work-1", isLive: true)
        XCTAssertTrue(q.contains("remove work-1?"), q)
        XCTAssertTrue(q.contains("its console closes"), q)
        XCTAssertTrue(q.contains("matchblox pair"), q)
    }

    func testRemoveQuestionNotLive() {
        let q = HomeCopy.removeQuestion(label: "work-1", isLive: false)
        XCTAssertTrue(q.contains("remove work-1?"), q)
        XCTAssertFalse(q.contains("its console closes"), q)
    }

    /// First-run home already offers `try the demo`: a second `Try the demo` in the footer
    /// gave VoiceOver and the UI test two controls with one name.
    func testFooterHidesDemoOnFirstRun() {
        XCTAssertFalse(HomeCopy.showDemoInFooter(hosts: []))
    }

    func testFooterHidesDemoWhenDemoPaired() {
        XCTAssertTrue(HomeCopy.showDemoInFooter(hosts: [host("work-1")]))
        XCTAssertFalse(HomeCopy.showDemoInFooter(hosts: [host("ws-1", demo: true)]))
        XCTAssertFalse(HomeCopy.showDemoInFooter(hosts: [host("work-1"), host("ws-1", demo: true)]))
    }

    func testVoiceOverBack() {
        XCTAssertEqual(HomeCopy.voiceOverBack(label: "work-1"), "Back to work-1")
    }

    func testVoiceOverBox() {
        XCTAssertEqual(HomeCopy.voiceOverBox, "Home, the box")
    }

    func testLiveActivityRow() {
        let work = host("work-1")
        let prefs = IslandPrefs(choice: .host(work.fingerprint), show: .sessions, buzz: [work.fingerprint: true])
        let line = HomeCopy.liveActivityRow(prefs: prefs, hosts: [work])
        XCTAssertEqual(line, "work-1 · buzz on")
    }

    func testLiveActivityRowBuzzOffAndAuto() {
        let work = host("work-1")
        let prefs = IslandPrefs(choice: .host(work.fingerprint), show: .auto, buzz: [work.fingerprint: false])
        let line = HomeCopy.liveActivityRow(prefs: prefs, hosts: [work])
        XCTAssertEqual(line, "work-1 · buzz off")
    }

    func testLiveActivityRowFollowNone() {
        let work = host("work-1")
        let prefs = IslandPrefs(choice: FollowChoice.none, lastOpened: work.fingerprint, show: .usage, buzz: [:])
        let line = HomeCopy.liveActivityRow(prefs: prefs, hosts: [work])
        // Follow is none: no host name; buzz on if any host buzzes (default true).
        XCTAssertEqual(line, "buzz on")
    }

    func testLiveActivityRowDefaultFollowUsesLastOpened() {
        let work = host("work-1")
        let prefs = IslandPrefs(choice: nil, lastOpened: work.fingerprint, show: .sessions, buzz: [work.fingerprint: true])
        let line = HomeCopy.liveActivityRow(prefs: prefs, hosts: [work])
        XCTAssertEqual(line, "work-1 · buzz on")
    }

    /// The show line names each choice once, in the Live Activity screen's order.
    func testShowLineOffersEachChoice() {
        XCTAssertEqual(HomeCopy.showChoices, [.sessions, .usage, .auto])
    }

    func testShowSpoken() {
        XCTAssertEqual(HomeCopy.showSpoken(.sessions, selected: true), "Show sessions, selected")
        XCTAssertEqual(HomeCopy.showSpoken(.usage, selected: false), "Show usage")
        XCTAssertEqual(HomeCopy.showSpoken(.auto, selected: false), "Show auto, usage while the stoker runs")
    }
}
