import XCTest
@testable import MatchbloxKit

final class HostCommandTests: XCTestCase {
    func testWordsAreQuotedAsAShellReadsThem() {
        XCTAssertEqual(HostCommand.line(["sessions", "--json", "%2", "a_b.c:d@e"]), "sessions --json %2 a_b.c:d@e")
        XCTAssertEqual(HostCommand.line(["it's", "", "a;b", "$(x)", "Yes,", "a/b"]), #"'it'\''s' '' 'a;b' '$(x)' 'Yes,' 'a/b'"#)
    }

    func testTheDemoGetsTheLine() async {
        let reply = HostReply(output: Data("[]".utf8), ok: true)
        let seen = Seen()
        let run = HostCommand.demo { line in
            await seen.set(line)
            return reply
        }
        let got = await run(["sessions", "--json"])
        XCTAssertEqual(got, reply)
        let line = await seen.line
        XCTAssertEqual(line, "sessions --json")
    }
}

private actor Seen {
    var line = ""
    func set(_ line: String) { self.line = line }
}
