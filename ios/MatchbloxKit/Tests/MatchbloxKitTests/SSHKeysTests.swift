import Foundation
import NIOSSH
import XCTest
@testable import MatchbloxKit

final class SSHKeysTests: XCTestCase {
    private func data(hex: String) -> Data {
        var bytes = [UInt8]()
        var index = hex.startIndex
        while index < hex.endIndex {
            let next = hex.index(index, offsetBy: 2)
            bytes.append(UInt8(hex[index..<next], radix: 16)!)
            index = next
        }
        return Data(bytes)
    }

    func testOpenSSHMatchesTheFixture() throws {
        let fixture = try Fixture.load()
        let point = data(hex: fixture.p256X963Hex)
        XCTAssertEqual(SSHKeys.openSSH(p256X963: point, comment: ""), fixture.p256Openssh)
    }

    func testOpenSSHAppendsTheComment() throws {
        let fixture = try Fixture.load()
        let point = data(hex: fixture.p256X963Hex)
        XCTAssertEqual(
            SSHKeys.openSSH(p256X963: point, comment: "Desk-2"),
            fixture.p256Openssh + " Desk-2"
        )
    }

    func testFingerprintOfALineMatchesSSHKeygen() throws {
        let fixture = try Fixture.load()
        XCTAssertEqual(try SSHKeys.fingerprint(openSSH: fixture.hostkeyPub), fixture.hostkeyFp)
    }

    func testFingerprintIgnoresTheComment() throws {
        let fixture = try Fixture.load()
        XCTAssertEqual(
            try SSHKeys.fingerprint(openSSH: fixture.hostkeyPub + " root@ws-1"),
            fixture.hostkeyFp
        )
    }

    func testFingerprintRefusesANonKeyLine() {
        XCTAssertThrowsError(try SSHKeys.fingerprint(openSSH: "nonsense")) {
            XCTAssertEqual($0 as? SSHKeyError, .badKeyLine)
        }
        XCTAssertThrowsError(try SSHKeys.fingerprint(openSSH: "ssh-ed25519 !!!")) {
            XCTAssertEqual($0 as? SSHKeyError, .badKeyLine)
        }
    }

    func testFingerprintOfAHostKeyMatchesSSHKeygen() throws {
        let fixture = try Fixture.load()
        let hostKey = try NIOSSHPublicKey(openSSHPublicKey: fixture.hostkeyPub)
        XCTAssertEqual(SSHKeys.fingerprint(hostKey: hostKey), fixture.hostkeyFp)
    }

    func testOneTimeKeyIsTheKeyOfThePayload() throws {
        let fixture = try Fixture.load()
        let key = try SSHKeys.oneTimeKey(seedBase64: fixture.payload.key)
        let line = String(openSSHPublicKey: key.publicKey)
        XCTAssertTrue(line.hasPrefix("ssh-ed25519 "))
    }

    func testOneTimeKeyRefusesBadSeeds() {
        XCTAssertThrowsError(try SSHKeys.oneTimeKey(seedBase64: "***")) {
            XCTAssertEqual($0 as? SSHKeyError, .badSeed)
        }
        XCTAssertThrowsError(try SSHKeys.oneTimeKey(seedBase64: "AAEC")) {
            XCTAssertEqual($0 as? SSHKeyError, .badSeed)
        }
    }
}
