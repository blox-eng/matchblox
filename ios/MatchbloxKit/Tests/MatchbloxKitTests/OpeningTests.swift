import XCTest
@testable import MatchbloxKit

final class OpeningTests: XCTestCase {
    private func host(_ label: String) -> Host {
        Host(label: label, address: "\(label).local", port: 22, user: "dev", fingerprint: "SHA256:\(label)")
    }

    func testFirstRoute() {
        XCTAssertEqual(Opening.first(hosts: []), .scan)
        XCTAssertEqual(Opening.first(hosts: [host("ws-1")]), .console(host("ws-1")))
        XCTAssertEqual(Opening.first(hosts: [host("ws-1"), host("ws-2")]), .hosts)
    }

    func testReconnectMessages() {
        XCTAssertEqual(
            Opening.message(for: .refused, host: "ws-1"),
            "This phone's key no longer opens ws-1. Run matchblox pair on ws-1, then scan the new code."
        )
        XCTAssertEqual(
            Opening.message(for: .hostKeyChanged, host: "ws-1"),
            Pairing.message(for: .hostKeyChanged, host: "ws-1")
        )
        XCTAssertEqual(
            Opening.message(for: .unreachable, host: "ws-1"),
            Pairing.message(for: .unreachable, host: "ws-1")
        )
    }

    func testClosedMessageHidesTheDebugText() {
        let debug = "NWError: POSIX 54 secret-detail"
        let message = Opening.message(for: .closed(debug), host: "ws-1")
        XCTAssertFalse(message.contains(debug))
        XCTAssertTrue(message.hasSuffix("."))
        XCTAssertNotEqual(message, Pairing.message(for: .closed(debug), host: "ws-1"))
    }

    func testReconnectRefusedIsNotCodeUsed() {
        XCTAssertNotEqual(
            Opening.message(for: .refused, host: "ws-1"),
            Pairing.message(for: .refused, host: "ws-1")
        )
    }
}
