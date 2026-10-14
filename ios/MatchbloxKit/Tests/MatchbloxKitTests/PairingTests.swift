import Foundation
import XCTest
@testable import MatchbloxKit

final class PairingTests: XCTestCase {
    func testDraftAddressOverride() throws {
        let payload = try Fixture.load().payload
        var draft = PairDraft(payload: payload)
        XCTAssertEqual(draft.address, payload.host)
        XCTAssertEqual(draft.port, payload.port)

        draft.address = "192.168.1.20"
        XCTAssertEqual(draft.address, "192.168.1.20")
        XCTAssertEqual(draft.payload.host, payload.host)
    }

    func testCleanName() throws {
        let names = try Fixture.load().names
        for good in names.good {
            XCTAssertEqual(Pairing.cleanName(good), good, good)
            XCTAssertTrue(Pairing.isValidName(good), good)
        }
        for bad in names.bad {
            XCTAssertFalse(Pairing.isValidName(bad), bad)
            XCTAssertTrue(Pairing.isValidName(Pairing.cleanName(bad)), bad)
        }
        XCTAssertEqual(names.bad.map(Pairing.cleanName), [
            "iPhone",
            String(repeating: "a", count: 40),
            "ab",
            "ab",
            "iPhone",
            "lead",
        ])
        XCTAssertEqual(Pairing.cleanName(String(repeating: "a", count: 39) + " b"), String(repeating: "a", count: 39))
    }

    func testRequestLineMatchesTheFixture() throws {
        let fixture = try Fixture.load()
        let line = Pairing.requestLine(name: fixture.request.name, key: fixture.request.key)
        XCTAssertEqual(String(decoding: line, as: UTF8.self), fixture.requestLine)
    }

    func testRequestLineEscapes() throws {
        let line = Pairing.requestLine(name: "a\"b\\c\n", key: "k")
        let object = try JSONSerialization.jsonObject(with: line) as? [String: String]
        XCTAssertEqual(object, ["name": "a\"b\\c\n", "key": "k"])
        XCTAssertEqual(line.filter { $0 == UInt8(ascii: "\n") }.count, 1)
    }

    func testRepliesFromTheFixture() throws {
        let replies = try Fixture.load().replies
        XCTAssertEqual(Set(replies.keys), Set(["ok"] + PairRefusal.allCases.map(\.rawValue)))
        for (word, json) in replies {
            let expected: PairReply = word == "ok" ? .ok(host: "ws-1") : .failed(PairRefusal(rawValue: word)!)
            XCTAssertEqual(try Pairing.reply(Data(json.utf8)), expected, word)
        }
    }

    func testReplyRefusesUnknownAnswers() {
        let cases = [
            "",
            "welcome\n",
            "[1]\n",
            "{\"ok\":false,\"error\":\"bad request\"}\n",
            "{\"ok\":true}\n",
        ]
        for text in cases {
            XCTAssertThrowsError(try Pairing.reply(Data(text.utf8)), text) { error in
                XCTAssertNotNil(error as? PairReplyError, text)
            }
        }
    }

    func testUsedCodeMessage() {
        XCTAssertEqual(
            Pairing.message(for: .refused, host: "ws-1"),
            "This code was used or expired. Run matchblox pair on ws-1 again."
        )
        XCTAssertEqual(
            Pairing.message(for: .unreachable, host: "ws-1"),
            "Can't reach ws-1. Check the address, or that the phone is on your network."
        )
        XCTAssertEqual(
            Pairing.message(for: .hostKeyChanged, host: "ws-1"),
            "ws-1 has a new host key. Run matchblox pair on ws-1 again."
        )
    }

    func testEveryFailureHasAMessage() {
        let debug = "NWError: POSIX 54 secret-detail"
        var messages: [String] = []
        messages += [ConnectFailure.unreachable, .hostKeyChanged, .refused, .closed(debug)]
            .map { Pairing.message(for: $0, host: "ws-1") }
        messages += PairRefusal.allCases.map { Pairing.message(for: $0, host: "ws-1") }
        messages += PairReplyError.allCases.map { Pairing.message(for: $0, host: "ws-1") }
        messages += [PairError.notMatchblox, .version(2), .malformed].map { Pairing.message(for: $0) }
        for message in messages {
            XCTAssertFalse(message.isEmpty)
            XCTAssertFalse(message.contains(debug), message)
            XCTAssertTrue(message.hasSuffix("."), message)
        }
        XCTAssertEqual(Set(messages).count, messages.count, "each failure has its own text")
    }

    func testReplySkipsShellNoise() throws {
        let text = "welcome\n{\"ok\":true,\"host\":\"ws-1\"}\n"
        XCTAssertEqual(try Pairing.reply(Data(text.utf8)), .ok(host: "ws-1"))
    }

    func testReadReplyJoinsChunks() async throws {
        let chunks = ["wel", "come\r\n{\"ok\":fal", "se,\"error\":\"code", " used\"}", "\nlater\n"]
        let reply = try await Pairing.readReply(from: stream(chunks, finish: false))
        XCTAssertEqual(reply, .failed(.codeUsed))
    }

    func testReadReplyTakesALastLineWithNoNewline() async throws {
        let reply = try await Pairing.readReply(from: stream(["motd\n", "{\"ok\":true,\"host\":\"ws-1\"}"], finish: true))
        XCTAssertEqual(reply, .ok(host: "ws-1"))
    }

    func testReadReplyWithNoJSONLine() async {
        do {
            _ = try await Pairing.readReply(from: stream(["welcome\n", "bye\n"], finish: true))
            XCTFail("expected no reply")
        } catch {
            XCTAssertEqual(error, .noReply)
        }
    }

    private func stream(_ chunks: [String], finish: Bool) -> AsyncStream<Data> {
        let (stream, output) = AsyncStream<Data>.makeStream()
        for chunk in chunks {
            output.yield(Data(chunk.utf8))
        }
        if finish {
            output.finish()
        }
        return stream
    }
}
