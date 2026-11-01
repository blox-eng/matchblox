import XCTest
@testable import MatchbloxKit

final class BeltLayoutsTests: XCTestCase {
    private struct Row: Decodable {
        let id: String
        let label: String
        let agent: String
    }

    private func fixture() throws -> Data { try testdata("keys/commands.json") }
    private func catalog() throws -> KeyCatalog { try KeyCatalog(commands: fixture()) }
    private func rows() throws -> [Row] { try JSONDecoder().decode([Row].self, from: fixture()) }

    private func defaults() -> UserDefaults {
        let name = "BeltLayoutsTests.\(UUID().uuidString)"
        let d = UserDefaults(suiteName: name)!
        addTeardownBlock { d.removePersistentDomain(forName: name) }
        return d
    }

    private let homeRow = ["◂", "↑", "↓", "⏎"]

    // MARK: Layouts

    func testSteerLayout() throws {
        let keys = try catalog().keys(.steer, agent: "claude", mine: [])
        XCTAssertEqual(keys.map(\.id), ["◂", "↑", "↓", "⏎", "Esc", "^C", "⇞", "⇟"])
        XCTAssertEqual(keys.map(\.cells), [1, 2, 2, 2, 1, 1, 1, 1])
    }

    func testTmuxLayout() throws {
        let keys = try catalog().keys(.tmux, agent: nil, mine: [])
        XCTAssertEqual(keys.map(\.id), ["◂", "↑", "↓", "⏎", "⇞", "⇟", "Tab", "⇧Tab", "^B"])
        XCTAssertEqual(keys.map(\.cells), [1, 2, 2, 2, 1, 1, 1, 1, 1])
    }

    func testCommandsFollowTheAgent() throws {
        let c = try catalog()
        let claude = c.keys(.commands, agent: "claude", mine: [])
        XCTAssertEqual(claude.map(\.id), homeRow + ["compact", "clear", "model", "review"])
        XCTAssertEqual(claude.map(\.cells), [1, 2, 2, 2, 2, 2, 2, 2])

        let codex = c.keys(.commands, agent: "codex", mine: [])
        XCTAssertEqual(codex.map(\.id), homeRow + ["codex-compact", "codex-new", "codex-model", "codex-review"])
        XCTAssertEqual(codex.dropFirst(4).map(\.cells), [2, 2, 2, 2])

        XCTAssertEqual(c.command("codex-compact")?.label, "compact")
        XCTAssertEqual(c.command("codex-compact")?.mark, "openai")
        XCTAssertNil(c.command("↑"))
    }

    func testCommandsWithNoAgent() throws {
        let c = try catalog()
        XCTAssertEqual(c.keys(.commands, agent: nil, mine: []).map(\.id), homeRow)
        XCTAssertEqual(c.keys(.commands, agent: "", mine: []).map(\.id), homeRow)
        XCTAssertEqual(c.keys(.commands, agent: "nobody", mine: []).map(\.id), homeRow)
    }

    func testGroups() throws {
        let groups = try catalog().groups
        XCTAssertEqual(groups.map(\.name), ["Steer", "Edit", "Digits", "Ctrl", "Claude Code", "Codex", "OpenCode", "Cursor"])
        XCTAssertEqual(groups[0].keys.map(\.id), ["◂", "↑", "↓", "⏎", "Esc", "^C", "⇞", "⇟"])
        XCTAssertEqual(groups[1].keys.map(\.id), ["Tab", "⇧Tab", "←", "→", "⌫"])
        XCTAssertEqual(groups[2].keys.map(\.id), (1...9).map(String.init))
        XCTAssertEqual(groups[3].keys.map(\.id), ["^A", "^B", "^D", "^L", "^R"])
        XCTAssertEqual(groups[7].keys.map(\.id), ["cursor-summarize", "cursor-clear", "cursor-model"])
        XCTAssertTrue(groups[4...].allSatisfy { $0.keys.allSatisfy { $0.cells == 2 } })
    }

    func testMineLayoutKeepsKnownKeysOnly() throws {
        let mine = [BeltKey(id: "◂", cells: 1), BeltKey(id: "nope", cells: 1), BeltKey(id: "compact", cells: 1)]
        let keys = try catalog().keys(.mine, agent: nil, mine: mine)
        XCTAssertEqual(keys, [BeltKey(id: "◂", cells: 1), BeltKey(id: "compact", cells: 2)])
    }

    // MARK: Bytes

    func testEveryKeyHasBytes() throws {
        let c = try catalog()
        var keys = c.groups.flatMap(\.keys)
        for layout in BeltLayout.allCases {
            for agent in [nil, "claude", "codex", "opencode", "cursor"] {
                keys += c.keys(layout, agent: agent, mine: Mine.defaults.keys)
            }
        }
        for key in keys {
            XCTAssertNotNil(KeyBar.bytes(for: key.id), key.id)
        }

        let esc: UInt8 = 0x1b
        XCTAssertEqual(KeyBar.bytes(for: "◂"), HomeKey.bytes)
        XCTAssertEqual(KeyBar.bytes(for: "↑"), [esc, 0x5b, 0x41])
        XCTAssertEqual(KeyBar.bytes(for: "↓"), [esc, 0x5b, 0x42])
        XCTAssertEqual(KeyBar.bytes(for: "→"), [esc, 0x5b, 0x43])
        XCTAssertEqual(KeyBar.bytes(for: "←"), [esc, 0x5b, 0x44])
        XCTAssertEqual(KeyBar.bytes(for: "⏎"), [0x0d])
        XCTAssertEqual(KeyBar.bytes(for: "Tab"), [0x09])
        XCTAssertEqual(KeyBar.bytes(for: "⇧Tab"), [esc, 0x5b, 0x5a])
        XCTAssertEqual(KeyBar.bytes(for: "Esc"), [esc])
        XCTAssertEqual(KeyBar.bytes(for: "⌫"), [0x7f])
        XCTAssertEqual(KeyBar.bytes(for: "^A"), [0x01])
        XCTAssertEqual(KeyBar.bytes(for: "^B"), [0x02])
        XCTAssertEqual(KeyBar.bytes(for: "^C"), [0x03])
        XCTAssertEqual(KeyBar.bytes(for: "^D"), [0x04])
        XCTAssertEqual(KeyBar.bytes(for: "^L"), [0x0c])
        XCTAssertEqual(KeyBar.bytes(for: "^R"), [0x12])
        XCTAssertEqual(KeyBar.bytes(for: "1"), [0x31])
        XCTAssertEqual(KeyBar.bytes(for: "9"), [0x39])
        XCTAssertEqual(KeyBar.bytes(for: "⇞"), Array("\u{1b}[5~".utf8))
        XCTAssertEqual(KeyBar.bytes(for: "⇟"), Array("\u{1b}[6~".utf8))
    }

    func testNoBytesForAStrangeId() {
        XCTAssertNil(KeyBar.bytes(for: ""))
        XCTAssertNil(KeyBar.bytes(for: "0"))
        XCTAssertNil(KeyBar.bytes(for: "Compact"))
        XCTAssertNil(KeyBar.bytes(for: "x\u{1b}\\"), "an id never ends the signal early")
        XCTAssertNil(KeyBar.bytes(for: "a;b"))
    }

    func testCommandBytes() throws {
        let all = try rows()
        XCTAssertFalse(all.isEmpty)
        for row in all {
            // keycmd.Sequence in Go.
            let sequence = "\u{1b}_matchblox;cmd=" + row.id + "\u{1b}\\"
            XCTAssertEqual(KeyBar.bytes(for: row.id), Array(sequence.utf8), row.id)
        }
    }

    // MARK: Mine

    func testMineDefaults() {
        XCTAssertEqual(Mine.defaults.keys.map(\.id), homeRow)
        XCTAssertEqual(Mine.defaults.keys.map(\.cells), [1, 2, 2, 2])
        XCTAssertEqual(Mine.load(from: defaults()), Mine.defaults)
    }

    func testMineToggle() {
        var mine = Mine.defaults
        XCTAssertTrue(mine.toggle(BeltKey(id: "Esc", cells: 1)))
        XCTAssertEqual(mine.keys.map(\.id), homeRow + ["Esc"])
        XCTAssertTrue(mine.toggle(BeltKey(id: "↑", cells: 2)))
        XCTAssertEqual(mine.keys.map(\.id), ["◂", "↓", "⏎", "Esc"])
    }

    func testHomeKeyNeverLeavesMine() {
        var mine = Mine.defaults
        XCTAssertFalse(mine.toggle(BeltKey(id: "◂", cells: 1)))
        XCTAssertEqual(mine, Mine.defaults)
        XCTAssertEqual(Mine(keys: [BeltKey(id: "Esc", cells: 1)]).keys.map(\.id), ["◂", "Esc"])
    }

    func testMineReorder() {
        var mine = Mine.defaults
        mine.move(from: 0, to: 3)
        XCTAssertEqual(mine.keys.map(\.id), ["↑", "↓", "⏎", "◂"], "◂ may move")
        mine.move(from: 2, to: 0)
        XCTAssertEqual(mine.keys.map(\.id), ["⏎", "↑", "↓", "◂"])
        mine.move(from: 9, to: 0)
        mine.move(from: 0, to: -1)
        XCTAssertEqual(mine.keys.map(\.id), ["⏎", "↑", "↓", "◂"], "a move out of range does nothing")
    }

    func testMineOfOneKeyStillHasHome() throws {
        var mine = Mine.defaults
        for id in ["↑", "↓", "⏎"] { _ = mine.toggle(BeltKey(id: id, cells: 2)) }
        XCTAssertEqual(mine.keys.map(\.id), ["◂"])

        let keys = try catalog().keys(.mine, agent: nil, mine: mine.keys)
        var m = BeltMotion(keys: keys, geometry: BeltGeometry(width: 370))
        XCTAssertEqual(m.offset, m.homeOffset)
        _ = m.drag(by: -100)
        m.release(velocity: -800)
        var frames = 0
        while !m.atRest && frames < 2000 {
            _ = m.step(dt: 1.0 / 60)
            frames += 1
        }
        XCTAssertTrue(m.atRest)
        XCTAssertEqual(m.offset, m.settleTarget(), accuracy: 1e-9)
        XCTAssertTrue(m.keys.indices.contains { abs(m.x(ofKeyAt: $0) - BeltGeometry.gap) < 1e-6 })
    }

    func testMineRoundTripsThroughTheStore() {
        let store = defaults()
        var mine = Mine.defaults
        _ = mine.toggle(BeltKey(id: "compact", cells: 2))
        mine.move(from: 4, to: 1)
        mine.save(to: store)
        XCTAssertNotNil(store.data(forKey: Mine.storeKey))
        XCTAssertEqual(Mine.storeKey, "belt.mine")
        XCTAssertEqual(Mine.load(from: store), mine)
    }

    func testStoredMineWithBadCells() {
        let store = defaults()
        let json = #"{"keys":[{"id":"↑","cells":3},{"id":"Esc","cells":1},{"id":"⏎","cells":0},{"id":7,"cells":1},{"id":"^C"}]}"#
        store.set(Data(json.utf8), forKey: Mine.storeKey)
        XCTAssertEqual(Mine.load(from: store).keys, [BeltKey(id: "◂", cells: 1), BeltKey(id: "Esc", cells: 1)])
    }

    func testStoredMineThatIsNoMine() {
        let store = defaults()
        store.set(Data("not json".utf8), forKey: Mine.storeKey)
        XCTAssertEqual(Mine.load(from: store), Mine.defaults)
        store.set(Data(#"{"keys":[]}"#.utf8), forKey: Mine.storeKey)
        XCTAssertEqual(Mine.load(from: store).keys.map(\.id), ["◂"])
    }
}
