import Foundation
import XCTest
@testable import MatchbloxKit
import struct MatchbloxKit.Host

final class DemoHostTests: XCTestCase {
    private var directory: URL!

    override func setUpWithError() throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: directory)
    }

    private var file: URL { directory.appendingPathComponent("hosts.json") }

    private func real(_ fingerprint: String = "SHA256:a", address: String = "ws-1.local") -> Host {
        Host(label: "ws-1", address: address, port: 22, user: "builder", fingerprint: fingerprint)
    }

    func testADemoHostSavesAndLoads() throws {
        let store = HostStore(file: file)
        try store.upsert(.demo)

        XCTAssertEqual(store.all(), [.demo])
        XCTAssertTrue(store.all()[0].isDemo)
        let row = try JSONDecoder().decode([[String: JSONValue]].self, from: Data(contentsOf: file))
        XCTAssertEqual(Set(row[0].keys), ["label", "address", "port", "user", "fingerprint"])
    }

    func testOneDemoHostOpensItsConsole() {
        XCTAssertEqual(Opening.first(hosts: [.demo]), .console(.demo, open: nil))
    }

    func testPairingARealHostRemovesTheDemo() throws {
        let store = HostStore(file: file)
        try store.upsert(.demo)
        let stale = try store.upsert(real())

        XCTAssertEqual(store.all(), [real()])
        XCTAssertEqual(stale, [])
    }

    func testTheDemoNeverMatchesARealHost() throws {
        XCTAssertFalse(real().isDemo)
        XCTAssertFalse(Host.demoFingerprint.hasPrefix("SHA256:"))
        let store = HostStore(file: file)
        try store.upsert(real(address: "this phone"))
        try store.upsert(.demo)

        XCTAssertEqual(store.all().count, 2)
    }

    func testTheIslandAndPushSeeOnlyRealHosts() throws {
        let store = HostStore(file: file)
        try store.upsert(.demo)
        XCTAssertEqual(store.real(), [])
        try store.upsert(real())
        try store.upsert(.demo)
        XCTAssertEqual(store.all().count, 2)
        XCTAssertEqual(store.real(), [real()])
    }
}

private enum JSONValue: Decodable {
    case text(String)
    case number(Int)

    init(from decoder: Decoder) throws {
        let value = try decoder.singleValueContainer()
        if let number = try? value.decode(Int.self) { self = .number(number) } else { self = .text(try value.decode(String.self)) }
    }
}
