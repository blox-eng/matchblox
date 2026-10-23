import Foundation
import XCTest
@testable import MatchbloxKit

final class IslandChangeTests: XCTestCase {
    private struct RelayRefused: Error {}

    private func host(_ label: String) -> MatchbloxKit.Host {
        Host(label: label, address: "\(label).local", port: 22, user: "dev", fingerprint: "SHA256:\(label)")
    }

    func testEveryFailedHostIsNotChanged() {
        var change = IslandChange()
        change.failed(host("ws-1"), ConnectFailure.unreachable, cancelled: false)
        change.failed(host("work-1"), RelayRefused(), cancelled: false)
        change.failed(host("lab-1"), URLError(.notConnectedToInternet), cancelled: false)
        change.failed(host("box-1"), ConnectFailure.closed("eof"), cancelled: false)
        XCTAssertEqual(change.notChanged, [
            .init(label: "ws-1", line: "ws-1 is not reachable: it changes at the next connect"),
            .init(label: "work-1", line: "work-1 is not changed: it changes at the next connect"),
            .init(label: "lab-1", line: "lab-1 is not changed: it changes at the next connect"),
            .init(label: "box-1", line: "box-1 is not changed: it changes at the next connect"),
        ])
        XCTAssertEqual(change.untried, [])
    }

    func testACancelledHostStaysPending() {
        var change = IslandChange()
        change.failed(host("ws-1"), CancellationError(), cancelled: false)
        change.failed(host("work-1"), ConnectFailure.closed("closed while opening"), cancelled: true)
        XCTAssertEqual(change.untried, [host("ws-1"), host("work-1")])
        XCTAssertEqual(change.notChanged, [])
    }

    /// A channel the relay did not delete stays, so the next registration tries again: only a
    /// refusal for good (400, 403) forgets it. A limit, a fault or no network must not leak it.
    func testAChannelIsForgottenOnlyWhenTheRelayRefusesItForGood() {
        XCTAssertTrue(IslandChange.forgetsChannel(refusedWith: 400))
        XCTAssertTrue(IslandChange.forgetsChannel(refusedWith: 403))
        for status: Int? in [nil, 404, 429, 500, 502] {
            XCTAssertFalse(IslandChange.forgetsChannel(refusedWith: status), "\(String(describing: status))")
        }
    }

    func testWhatIOSKeepsFromThePhoneIsSaid() {
        XCTAssertEqual(Opening.blocked(notificationsDenied: false, activitiesEnabled: true), [])
        XCTAssertEqual(Opening.blocked(notificationsDenied: true, activitiesEnabled: false), [
            "Notifications are off for matchblox: no buzz comes. Turn them on in Settings.",
            "Live Activities are off for matchblox: no island shows. Turn them on in Settings.",
        ])
    }
}
