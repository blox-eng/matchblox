import Foundation
import XCTest
@testable import MatchbloxKit

final class PairPayloadTests: XCTestCase {
    private func url(of payload: PairPayload) throws -> URL {
        try url(carrying: JSONEncoder().encode(payload))
    }

    private func url(carrying json: Data) -> URL {
        URL(string: "matchblox://pair?p=" + base64URL(json))!
    }

    private func base64URL(_ data: Data) -> String {
        data.base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    func testFixtureURL() throws {
        let fixture = try Fixture.load()
        let parsed = try PairPayload(url: URL(string: fixture.url)!)
        XCTAssertEqual(parsed, fixture.payload)
    }

    func testIgnoresUnknownFields() throws {
        let fixture = try Fixture.load()
        var object = try JSONSerialization.jsonObject(with: JSONEncoder().encode(fixture.payload)) as! [String: Any]
        object["extra"] = true
        let json = try JSONSerialization.data(withJSONObject: object)
        XCTAssertEqual(try PairPayload(url: url(carrying: json)), fixture.payload)
    }

    func testRefuses() throws {
        let fixture = try Fixture.load()
        func with(_ change: (inout PairPayload) -> Void) throws -> URL {
            var payload = fixture.payload
            change(&payload)
            return try url(of: payload)
        }
        let cases: [(String, URL, PairError)] = [
            ("v 2", try with { $0.v = 2 }, .version(2)),
            ("no host", try with { $0.host = "" }, .malformed),
            ("no user", try with { $0.user = "" }, .malformed),
            ("no name", try with { $0.name = "" }, .malformed),
            ("no hostkey", try with { $0.hostkey = "" }, .malformed),
            ("port 0", try with { $0.port = 0 }, .malformed),
            ("port 65536", try with { $0.port = 65536 }, .malformed),
            ("31-byte key", try with { $0.key = Data(count: 31).base64EncodedString() }, .malformed),
            ("not base64", URL(string: "matchblox://pair?p=!!!")!, .malformed),
            ("padded base64", URL(string: fixture.url + "%3D")!, .malformed),
            ("not json", url(carrying: Data("x".utf8)), .malformed),
            ("no p", URL(string: "matchblox://pair")!, .malformed),
            ("scheme", URL(string: fixture.url.replacingOccurrences(of: "matchblox://", with: "https://"))!, .notMatchblox),
            ("host", URL(string: fixture.url.replacingOccurrences(of: "://pair", with: "://other"))!, .notMatchblox),
        ]
        for (name, url, expected) in cases {
            XCTAssertThrowsError(try PairPayload(url: url), name) { error in
                XCTAssertEqual(error as? PairError, expected, name)
            }
        }
    }
}
