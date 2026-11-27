import XCTest
@testable import MatchbloxKit

final class HostCommandTests: XCTestCase {
    func testWordsAreQuotedAsAShellReadsThem() {
        XCTAssertEqual(HostCommand.line(["menu", "%2", "--json"]), "menu %2 --json")
        XCTAssertEqual(
            HostCommand.line(["choose", "%2", "2", "--text", "Rotate, keep it's old one", "--sign", "0a"]),
            #"choose %2 2 --text 'Rotate, keep it'\''s old one' --sign 0a"#
        )
        XCTAssertEqual(HostCommand.line(["", "a;b", "$(x)"]), #"'' 'a;b' '$(x)'"#)
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
