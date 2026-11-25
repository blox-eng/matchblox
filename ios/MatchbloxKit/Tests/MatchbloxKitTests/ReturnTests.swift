import XCTest
import MatchbloxKit
import struct MatchbloxKit.Host

/// The way back into Pocket from the island: the console stays, and so does its screen.
final class ReturnTests: XCTestCase {
    private func host(_ byte: UInt8) -> Host {
        let fingerprint = "SHA256:" + Data(repeating: byte, count: 32).base64EncodedString()
        return Host(label: "ws-\(byte)", address: "ws-\(byte).local", port: 22, user: "dev", fingerprint: fingerprint)
    }

    private func link(to host: Host, session: Int?) -> OpenLink {
        OpenLink(host: host8(host.fingerprint), session: session)!
    }

    func testALinkForTheOpenHostKeepsItsConsole() {
        let a = host(1)
        XCTAssertTrue(Opening.keepsConsole(for: link(to: a, session: 3), current: .console(a, open: nil)))
        XCTAssertTrue(Opening.keepsConsole(for: link(to: a, session: nil), current: .console(a, open: 2)))
    }

    func testALinkForAnotherHostOpensIt() {
        XCTAssertFalse(Opening.keepsConsole(for: link(to: host(2), session: 1), current: .console(host(1), open: nil)))
    }

    func testALinkOnTheHostsListOpensTheConsole() {
        XCTAssertFalse(Opening.keepsConsole(for: link(to: host(1), session: 1), current: .hosts))
    }

    func testTheFirstConnectShowsTheCover() {
        XCTAssertTrue(Opening.coverShows(live: false, failed: false, wasLive: false, away: false))
    }

    func testAResumeKeepsTheScreen() {
        XCTAssertFalse(Opening.coverShows(live: false, failed: false, wasLive: true, away: false))
    }

    func testAFailureShowsTheCoverOnResume() {
        XCTAssertTrue(Opening.coverShows(live: false, failed: true, wasLive: true, away: false))
    }

    func testLiveShowsNoCover() {
        XCTAssertFalse(Opening.coverShows(live: true, failed: false, wasLive: true, away: false))
    }

    /// The app switcher keeps a snapshot of the screen: away from the screen, the cover hides the console.
    func testAwayTheCoverHidesTheConsole() {
        XCTAssertTrue(Opening.coverShows(live: true, failed: false, wasLive: true, away: true))
        XCTAssertTrue(Opening.coverShows(live: false, failed: false, wasLive: true, away: true))
    }
}
