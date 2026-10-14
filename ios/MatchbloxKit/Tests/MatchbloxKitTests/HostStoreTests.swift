import Foundation
import XCTest
@testable import MatchbloxKit
import struct MatchbloxKit.Host

final class HostStoreTests: XCTestCase {
    private var directory: URL!

    override func setUpWithError() throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: directory)
    }

    private func host(_ fingerprint: String, address: String) -> Host {
        Host(label: "ws-1", address: address, port: 22, user: "builder", fingerprint: fingerprint)
    }

    func testEmptyWithNoFile() {
        XCTAssertEqual(HostStore(file: directory.appendingPathComponent("hosts.json")).all(), [])
    }

    func testHostStoreKeysByFingerprint() throws {
        let file = directory.appendingPathComponent("hosts.json")
        let store = HostStore(file: file)
        try store.upsert(host("SHA256:a", address: "ws-1.tail1234.ts.net"))
        try store.upsert(host("SHA256:a", address: "192.168.1.20"))

        XCTAssertEqual(store.all(), [host("SHA256:a", address: "192.168.1.20")])
        XCTAssertEqual(HostStore(file: file).all(), store.all())
        XCTAssertEqual(store.all().first?.id, "SHA256:a")
    }

    func testUpdateAddressAndRemove() throws {
        let store = HostStore(file: directory.appendingPathComponent("hosts.json"))
        try store.upsert(host("SHA256:a", address: "a.example.com"))
        try store.upsert(host("SHA256:b", address: "b.example.com"))

        try store.updateAddress(fingerprint: "SHA256:b", address: "10.0.0.2")
        XCTAssertEqual(store.all(), [host("SHA256:a", address: "a.example.com"), host("SHA256:b", address: "10.0.0.2")])

        try store.remove(fingerprint: "SHA256:a")
        XCTAssertEqual(store.all(), [host("SHA256:b", address: "10.0.0.2")])
    }

    func testDamagedFileReadsAsEmpty() throws {
        let file = directory.appendingPathComponent("hosts.json")
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        try Data("not json".utf8).write(to: file)
        let store = HostStore(file: file)

        XCTAssertEqual(store.all(), [])
        try store.upsert(host("SHA256:a", address: "a.example.com"))
        XCTAssertEqual(store.all(), [host("SHA256:a", address: "a.example.com")])
    }
}
