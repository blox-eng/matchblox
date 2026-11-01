import XCTest
@testable import MatchbloxKit

final class KeyFaceTests: XCTestCase {
    private func catalog() throws -> KeyCatalog { try KeyCatalog(commands: testdata("keys/commands.json")) }

    private func defaults() -> UserDefaults {
        let name = "KeyFaceTests.\(UUID().uuidString)"
        let d = UserDefaults(suiteName: name)!
        addTeardownBlock { d.removePersistentDomain(forName: name) }
        return d
    }

    func testFixedKeysShowTheirId() throws {
        let c = try catalog()
        let faces = c.keys(.steer, agent: nil, mine: []).map { KeyFace($0, in: c) }
        XCTAssertEqual(faces.map(\.label), ["◂", "↑", "↓", "⏎", "Esc", "^C", "⇞", "⇟"])
        XCTAssertEqual(faces.map(\.size), [.glyph, .big, .big, .big, .word, .word, .glyph, .glyph])
        XCTAssertTrue(faces.allSatisfy { $0.mark == nil && !$0.isCommand })
    }

    func testCommandShowsItsLabelAndMark() throws {
        let c = try catalog()
        let face = KeyFace(BeltKey(id: "codex-compact", cells: 2), in: c)
        XCTAssertEqual(face.label, "compact")
        XCTAssertEqual(face.size, .word)
        XCTAssertEqual(face.mark, "openai")
        XCTAssertTrue(face.isCommand)
        XCTAssertEqual(face.spoken, "/compact, Codex")
    }

    func testSpokenNames() throws {
        let c = try catalog()
        let spoken = ["◂", "⏎", "⇧Tab", "^C", "^R", "7", "⇟"].map { KeyFace(BeltKey(id: $0, cells: 1), in: c).spoken }
        XCTAssertEqual(spoken, ["Back to matchblox", "Return", "Shift Tab", "Interrupt", "Control R", "7", "Page down"])
    }

    func testEveryCatalogKeyHasAFace() throws {
        let c = try catalog()
        for (_, keys) in c.groups {
            for key in keys {
                let face = KeyFace(key, in: c)
                XCTAssertFalse(face.label.isEmpty, key.id)
                XCTAssertFalse(face.spoken.isEmpty, key.id)
                XCTAssertEqual(face.isCommand, face.mark != nil, key.id)
            }
        }
    }

    func testMarkOfAgent() throws {
        let c = try catalog()
        XCTAssertEqual(c.mark(of: "claude"), "claude-color")
        XCTAssertEqual(c.mark(of: "codex"), "openai")
        XCTAssertNil(c.mark(of: ""))
        XCTAssertNil(c.mark(of: nil))
        XCTAssertNil(c.mark(of: "nobody"))
    }

    func testLayoutOpensOnSteer() {
        XCTAssertEqual(BeltLayout.load(from: defaults()), .steer)
    }

    func testLayoutRoundTripsThroughTheStore() {
        let store = defaults()
        BeltLayout.tmux.save(to: store)
        XCTAssertEqual(BeltLayout.storeKey, "belt.layout")
        XCTAssertEqual(BeltLayout.load(from: store), .tmux)
        store.set("dial", forKey: BeltLayout.storeKey)
        XCTAssertEqual(BeltLayout.load(from: store), .steer)
    }
}
