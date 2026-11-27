import XCTest
@testable import MatchbloxKit

/// The owner's four marks and hold: set on home, taken back with undo, never an interrupt.
final class HeadMarksTests: XCTestCase {
    private func defaults() -> UserDefaults {
        let name = "HeadMarksTests.\(UUID().uuidString)"
        let d = UserDefaults(suiteName: name)!
        addTeardownBlock { d.removePersistentDomain(forName: name) }
        return d
    }

    private func decoded(_ json: String) throws -> HeadMarks {
        try JSONDecoder().decode(HeadMarks.self, from: Data(json.utf8))
    }

    private func faces(_ marks: HeadMarks) -> [String] {
        HeadMarks.Place.allCases.map { "\(marks[$0]) \(KeyFace.word(of: marks[$0]) ?? "")" }
    }

    func testFirstMarks() throws {
        XCTAssertEqual(faces(HeadMarks()), ["↑ ❯ up", "← cursor", "→ cursor", "↓ ❯ down", "⇄ sessions"])
        for place in [HeadMarks.Place.top, .left, .right, .bottom] {
            XCTAssertEqual(HeadMarks.keys(for: place).prefix(2), ["↑", "↓"], "\(place)")
        }
        XCTAssertEqual(KeyFace.word(of: "↑"), "❯ up")
        XCTAssertEqual(KeyFace.word(of: "↓"), "❯ down")
        var marks = HeadMarks()
        try marks.set("↓", at: .left)
        XCTAssertEqual(marks[.left], "↓")
        for id in ["↑", "↓"] {
            XCTAssertFalse(HeadMarks.keys(for: .hold).contains(id))
            XCTAssertThrowsError(try marks.set(id, at: .hold)) {
                XCTAssertEqual($0 as? HeadMarks.Refusal, .notAMark)
            }
        }
        XCTAssertEqual(marks[.hold], "⇄")
    }

    func testOldMarksMove() throws {
        let store = defaults()
        let buildNine = #"{"top":"◂","left":"Esc","right":"→","bottom":"⌨","hold":"⇄"}"#
        store.set(Data(buildNine.utf8), forKey: "head.marks")
        let marks = HeadMarks.load(from: store)
        XCTAssertEqual(faces(marks), ["↑ ❯ up", "Esc cancel", "→ cursor", "↓ ❯ down", "⇄ sessions"])
        XCTAssertEqual(HeadMarks.storeKey, "head.marks.2")
        let saved = try XCTUnwrap(store.data(forKey: "head.marks.2"), "the moved marks were not saved")
        XCTAssertEqual(faces(try JSONDecoder().decode(HeadMarks.self, from: saved)), faces(marks))
        XCTAssertNil(store.data(forKey: "head.marks"), "the build-9 marks are read once")

        let changed = defaults()
        changed.set(Data(#"{"top":"⇞","bottom":"/c","hold":"·"}"#.utf8), forKey: "head.marks")
        XCTAssertEqual(faces(HeadMarks.load(from: changed)), ["⇞ page up", "← cursor", "→ cursor", "/c compact", "· nothing"])

        var undone = HeadMarks.load(from: changed)
        undone.undo()
        XCTAssertEqual(undone[.top], "⇞", "a move leaves nothing to undo")

        let both = defaults()
        both.set(Data(#"{"left":"Tab"}"#.utf8), forKey: HeadMarks.storeKey)
        both.set(Data(buildNine.utf8), forKey: "head.marks")
        XCTAssertEqual(HeadMarks.load(from: both)[.left], "Tab", "stored new marks win over build 9's")
    }

    func testSetAndUndo() throws {
        var marks = HeadMarks()
        try marks.set("Esc", at: .left)
        try marks.set("⇞", at: .top)
        XCTAssertEqual(marks[.left], "Esc")
        XCTAssertEqual(marks[.top], "⇞")
        marks.undo()
        XCTAssertEqual(marks[.top], "↑")
        XCTAssertEqual(marks[.left], "Esc")
        marks.undo()
        XCTAssertEqual(marks[.left], "←")
        marks.undo()
        XCTAssertEqual(faces(marks), faces(HeadMarks()), "undo with nothing to take back changes nothing")
    }

    func testAsItCameThenUndo() throws {
        var marks = HeadMarks()
        try marks.set("Esc", at: .left)
        try marks.set("·", at: .hold)
        marks.asItCame()
        XCTAssertEqual(faces(marks), faces(HeadMarks()))
        marks.undo()
        XCTAssertEqual(marks[.left], "Esc")
        XCTAssertEqual(marks[.hold], "·")
    }

    func testInterruptRefused() throws {
        var marks = HeadMarks()
        try marks.set("Esc", at: .left)
        for place in HeadMarks.Place.allCases {
            let before = marks[place]
            XCTAssertThrowsError(try marks.set("^C", at: place)) {
                XCTAssertEqual($0 as? HeadMarks.Refusal, .staysOnBelt)
            }
            XCTAssertEqual(marks[place], before)
        }
        marks.undo()
        XCTAssertEqual(marks[.left], "←", "a refused set leaves nothing to undo")
    }

    func testSessionsOnlyOnHold() throws {
        var marks = HeadMarks()
        for place in [HeadMarks.Place.top, .left, .right, .bottom] {
            XCTAssertThrowsError(try marks.set("⇄", at: place)) {
                XCTAssertEqual($0 as? HeadMarks.Refusal, .holdOnly)
            }
        }
        try marks.set("Tab", at: .hold)
        try marks.set("⇄", at: .hold)
        XCTAssertEqual(marks[.hold], "⇄")
        XCTAssertFalse(HeadMarks.keys(for: .left).contains("⇄"))
        XCTAssertEqual(HeadMarks.keys(for: .hold).last, "⇄")
    }

    func testUnknownKeyRefused() {
        var marks = HeadMarks()
        XCTAssertThrowsError(try marks.set("ctrlc", at: .left)) {
            XCTAssertEqual($0 as? HeadMarks.Refusal, .notAMark)
        }
        XCTAssertEqual(marks[.left], "←")
    }

    func testDecodesLeniently() throws {
        XCTAssertEqual(faces(try decoded(#"{"left":"ctrlc"}"#)), faces(HeadMarks()))
        XCTAssertEqual(try decoded(#"{"left":"^C","top":"⇄","hold":7}"#)[.left], "←")
        XCTAssertEqual(try decoded(#"{"top":"⇄"}"#)[.top], "↑")
        XCTAssertEqual(try decoded(#"{"hold":7}"#)[.hold], "⇄")
        XCTAssertEqual(try decoded(#"{"left":"Esc","hold":"·"}"#)[.left], "Esc")
        XCTAssertEqual(try decoded(#"{"left":"Esc","hold":"·"}"#)[.hold], "·")
    }

    func testLoadAndSave() throws {
        let store = defaults()
        XCTAssertEqual(faces(HeadMarks.load(from: store)), faces(HeadMarks()))
        store.set(Data("nonsense".utf8), forKey: HeadMarks.storeKey)
        XCTAssertEqual(faces(HeadMarks.load(from: store)), faces(HeadMarks()))
        var marks = HeadMarks()
        try marks.set("/c", at: .bottom)
        marks.save(to: store)
        XCTAssertEqual(HeadMarks.storeKey, "head.marks.2")
        XCTAssertEqual(HeadMarks.load(from: store)[.bottom], "/c")
    }

    func testWords() {
        let words = ["Esc", "Tab", "⇧Tab", "⇞", "⇟", "/c", "·", "^C"].map { KeyFace.word(of: $0) }
        XCTAssertEqual(words, ["cancel", "tab", "mode", "page up", "page down", "compact", "nothing", "interrupt"])
        for place in HeadMarks.Place.allCases {
            for id in HeadMarks.keys(for: place) {
                XCTAssertNotNil(KeyFace.word(of: id), id)
            }
        }
    }

    func testBytes() {
        XCTAssertEqual(HeadMarks.bytes(for: "◂"), HomeKey.bytes)
        XCTAssertEqual(HeadMarks.bytes(for: "←"), [0x1b, 0x5b, 0x44])
        XCTAssertEqual(HeadMarks.bytes(for: "Esc"), [0x1b])
        XCTAssertEqual(HeadMarks.bytes(for: "/c"), Array("/compact".utf8), "no Return")
        XCTAssertEqual(HeadMarks.bytes(for: "·"), [])
        XCTAssertNil(HeadMarks.bytes(for: "⌨"), "the app raises the keyboard")
        XCTAssertNil(HeadMarks.bytes(for: "⇄"), "the app shows the sessions")
        XCTAssertEqual(HeadMarks.wordBytes(for: "←"), [0x1b, 0x62])
        XCTAssertEqual(HeadMarks.wordBytes(for: "→"), [0x1b, 0x66])
        XCTAssertNil(HeadMarks.wordBytes(for: "Esc"))
        for place in HeadMarks.Place.allCases {
            for id in HeadMarks.keys(for: place) where id != "⌨" && id != "⇄" {
                XCTAssertNotNil(HeadMarks.bytes(for: id), id)
            }
        }
    }
}
