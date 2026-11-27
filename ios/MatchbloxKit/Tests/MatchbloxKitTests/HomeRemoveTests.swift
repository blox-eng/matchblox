import XCTest
@testable import MatchbloxKit
import struct MatchbloxKit.Host

final class HomeRemoveTests: XCTestCase {
    private func host(_ label: String) -> Host {
        Host(label: label, address: "\(label).local", port: 22, user: "dev", fingerprint: "SHA256:\(label)")
    }

    func testConfirmRemoveForgetsDeviceKeyForRealHost() {
        XCTAssertTrue(HomeRemove.forgetsDeviceKey(host("ws-1")))
        XCTAssertFalse(HomeRemove.forgetsDeviceKey(.demo))
    }

    func testClearsBackWhenRemovedHostWasLive() {
        XCTAssertTrue(HomeRemove.clearsBack(removedFingerprint: "SHA256:ws-1", backFingerprint: "SHA256:ws-1"))
        XCTAssertFalse(HomeRemove.clearsBack(removedFingerprint: "SHA256:ws-1", backFingerprint: "SHA256:ws-2"))
        XCTAssertFalse(HomeRemove.clearsBack(removedFingerprint: "SHA256:ws-1", backFingerprint: nil))
    }
}
