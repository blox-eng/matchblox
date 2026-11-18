import XCTest
@testable import MatchbloxKit

final class RegistrationFailureTests: XCTestCase {
    private struct Odd: Error, CustomStringConvertible {
        var description: String { "the relay is busy" }
    }

    private let at = Date(timeIntervalSince1970: 1_800_000_000)

    func testEachFailureSaysItsFix() {
        let lines = [
            RegistrationFailure(.noDeviceToken, at: at),
            RegistrationFailure(.noNotifyKey, at: at),
            RegistrationFailure(.noGrant, detail: "device", at: at),
            RegistrationFailure(.noAppAttest, at: at),
            RegistrationFailure(.relay, detail: "relay 429: slow down", at: at),
            RegistrationFailure(.relayReply, at: at),
            RegistrationFailure(.unreachable, at: at),
            RegistrationFailure(.hostKeyChanged, at: at),
            RegistrationFailure(.refused, at: at),
            RegistrationFailure(.other, detail: "the session closed", at: at),
            RegistrationFailure(.other, at: at),
        ].map(\.line)
        XCTAssertEqual(lines, [
            "Not registered: no APNs device token — allow notifications for matchblox in Settings",
            "Not registered: the notification key could not be kept — unlock the phone, then open matchblox again to retry",
            "Not registered: the relay gave no grant for device — open matchblox again to retry",
            "Not registered: App Attest is not available on this device — the buzz and the island need it",
            "Not registered: relay 429: slow down — open matchblox again to retry",
            "Not registered: the relay's reply is not what this app reads — update matchblox",
            "Not registered: the host is not reachable — open matchblox again to retry where the phone reaches the host",
            "Not registered: the host has a new host key — pair the host again",
            "Not registered: the host refused the phone's key — pair the host again",
            "Not registered: the session closed — open matchblox again to retry",
            "Not registered: an error with no text — open matchblox again to retry",
        ])
    }

    func testAConnectErrorIsSorted() {
        XCTAssertEqual(RegistrationFailure.connect(ConnectFailure.unreachable, at: at).kind, .unreachable)
        XCTAssertEqual(RegistrationFailure.connect(ConnectFailure.hostKeyChanged, at: at).kind, .hostKeyChanged)
        XCTAssertEqual(RegistrationFailure.connect(ConnectFailure.refused, at: at).kind, .refused)
        let odd = RegistrationFailure.connect(Odd(), at: at)
        XCTAssertEqual(odd.kind, .other)
        XCTAssertEqual(odd.detail, "the relay is busy")
        XCTAssertEqual(odd.at, at)
    }

    func testAFailureIsKept() throws {
        let failure = RegistrationFailure(.noGrant, detail: "start", at: at)
        let data = try JSONEncoder().encode(failure)
        XCTAssertEqual(try JSONDecoder().decode(RegistrationFailure.self, from: data), failure)
    }
}
