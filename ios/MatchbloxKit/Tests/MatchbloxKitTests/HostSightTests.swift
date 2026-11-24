import XCTest
@testable import MatchbloxKit

final class HostSightTests: XCTestCase {
    private var suite: String!
    private var defaults: UserDefaults!

    override func setUp() {
        suite = "HostSightTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
        suite = nil
    }

    private let fp = "SHA256:work-1"

    func testSightLiveBeatsLastSeen() {
        var sight = HostSight(defaults: defaults)
        sight.markSeen(fp, at: Date(timeIntervalSince1970: 1_000))
        let status = sight.status(fingerprint: fp, liveFingerprint: fp, now: Date(timeIntervalSince1970: 10_000))
        XCTAssertEqual(status, .live)
    }

    func testSightFailMarksNotReachable() {
        var sight = HostSight(defaults: defaults)
        sight.markSeen(fp, at: Date(timeIntervalSince1970: 1_000))
        sight.markFailed(fp, at: Date(timeIntervalSince1970: 2_000))
        let status = sight.status(fingerprint: fp, liveFingerprint: nil, now: Date(timeIntervalSince1970: 3_000))
        if case .notReachable(let since) = status {
            XCTAssertEqual(since.timeIntervalSince1970, 2_000)
        } else {
            XCTFail("expected notReachable, got \(status)")
        }
    }

    func testMarkSeenClearsNotReachable() {
        var sight = HostSight(defaults: defaults)
        sight.markFailed(fp, at: Date(timeIntervalSince1970: 2_000))
        sight.markSeen(fp, at: Date(timeIntervalSince1970: 3_000))
        let status = sight.status(fingerprint: fp, liveFingerprint: nil, now: Date(timeIntervalSince1970: 4_000))
        if case .lastSeen(let at) = status {
            XCTAssertEqual(at.timeIntervalSince1970, 3_000)
        } else {
            XCTFail("expected lastSeen, got \(status)")
        }
    }
}
