import CryptoKit
import Network
import NIOCore
import NIOSSH
import NIOTransportServices
import XCTest
@testable import MatchbloxKit

final class ConnectFailureTests: XCTestCase {
    private let dnsNoSuchRecord: DNSServiceErrorType = -65554

    func testClassify() {
        XCTAssertEqual(ConnectFailure.classify(NWError.posix(.ECONNREFUSED)), .unreachable)
        XCTAssertEqual(ConnectFailure.classify(NWError.posix(.ETIMEDOUT)), .unreachable)
        XCTAssertEqual(ConnectFailure.classify(NWError.posix(.EHOSTUNREACH)), .unreachable)
        XCTAssertEqual(ConnectFailure.classify(NWError.dns(dnsNoSuchRecord)), .unreachable)
        XCTAssertEqual(ConnectFailure.classify(ChannelError.connectTimeout(.seconds(10))), .unreachable)
        XCTAssertEqual(
            ConnectFailure.classify(HostKeyMismatch(expected: "SHA256:a", presented: "SHA256:b")),
            .hostKeyChanged
        )
        XCTAssertEqual(ConnectFailure.classify(AuthRefused()), .refused)
        XCTAssertEqual(ConnectFailure.classify(ConnectFailure.refused), .refused)
        guard case .closed(let reason) = ConnectFailure.classify(ChannelError.eof) else {
            return XCTFail("an unknown error must give .closed")
        }
        XCTAssertFalse(reason.isEmpty)
    }

    func testPinnedHostKeyAcceptsOnlyThePairedKey() async throws {
        let fixture = try Fixture.load()
        let hostKey = try NIOSSHPublicKey(openSSHPublicKey: fixture.hostkeyPub)
        let group = NIOTSEventLoopGroup(loopCount: 1)
        addTeardownBlock { try await group.shutdownGracefully() }
        let loop = group.next()

        let match = loop.makePromise(of: Void.self)
        PinnedHostKey(fingerprint: fixture.hostkeyFp)
            .validateHostKey(hostKey: hostKey, validationCompletePromise: match)
        try await match.futureResult.get()

        let mismatch = loop.makePromise(of: Void.self)
        PinnedHostKey(fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
            .validateHostKey(hostKey: hostKey, validationCompletePromise: mismatch)
        do {
            try await mismatch.futureResult.get()
            XCTFail("a different host key must fail")
        } catch {
            XCTAssertEqual(ConnectFailure.classify(error), .hostKeyChanged)
        }
    }

    func testSilentHostGivesUpAndOpensOnce() async throws {
        let group = NIOTSEventLoopGroup(loopCount: 1)
        addTeardownBlock { try await group.shutdownGracefully() }
        let listener = try await NIOTSListenerBootstrap(group: group).bind(host: "127.0.0.1", port: 0).get()
        addTeardownBlock { try await listener.close().get() }
        let port = try XCTUnwrap(listener.localAddress?.port)
        let session = SSHSession(
            host: "127.0.0.1",
            port: port,
            user: "builder",
            key: NIOSSHPrivateKey(ed25519Key: Curve25519.Signing.PrivateKey()),
            hostKeyFingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
            deadline: .milliseconds(500)
        )

        await assertOpenFails(session, with: .unreachable)
        await assertOpenFails(session, with: .closed("already open"))
    }

    private func assertOpenFails(
        _ session: SSHSession,
        with expected: ConnectFailure,
        line: UInt = #line
    ) async {
        do {
            _ = try await session.open(command: "matchblox console", pty: nil)
            XCTFail("open must fail", line: line)
        } catch {
            XCTAssertEqual(ConnectFailure.classify(error), expected, line: line)
        }
    }
}
