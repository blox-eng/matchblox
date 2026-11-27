import Foundation

/// What the head's four marks and its hold do, as key ids. The owner sets them on home.
public struct HeadMarks: Codable, Sendable {
    public enum Place: String, CaseIterable, CodingKey, Sendable {
        case top, left, right, bottom, hold
    }

    public enum Refusal: Error, Sendable {
        /// `^C` stays on the belt: a roll can brush the edge.
        case staysOnBelt
        /// The sessions card opens from the hold only.
        case holdOnly
        case notAMark
    }

    /// The UserDefaults key of the encoded marks.
    public static let storeKey = "head.marks.2"
    /// The key TestFlight build 9 kept its marks under, read once by `load(from:)`.
    public static let buildNineKey = "head.marks"
    private static let interrupt = "^C"
    private static let sessions = "⇄"
    private static let arrows = ["↑", "↓"]
    private static let edgeKeys = arrows + ["◂", "⌨", "←", "→", "Esc", "Tab", "⇧Tab", "⇞", "⇟", "/c", "·"]
    private static let firstMarks: [Place: String] = [
        .top: "↑", .left: "←", .right: "→", .bottom: "↓", .hold: sessions,
    ]
    private static let buildNineMarks: [Place: String] = [
        .top: "◂", .left: "←", .right: "→", .bottom: "⌨", .hold: sessions,
    ]

    private var marks = firstMarks
    private var history: [[Place: String]] = []

    public init() {}

    /// The keys a place can take, in home's order.
    public static func keys(for place: Place) -> [String] {
        place == .hold ? edgeKeys.filter { !arrows.contains($0) } + [sessions] : edgeKeys
    }

    public subscript(place: Place) -> String {
        marks[place]!
    }

    public mutating func set(_ id: String, at place: Place) throws {
        if id == Self.interrupt { throw Refusal.staysOnBelt }
        if id == Self.sessions && place != .hold { throw Refusal.holdOnly }
        guard Self.keys(for: place).contains(id) else { throw Refusal.notAMark }
        var next = marks
        next[place] = id
        change(to: next)
    }

    public mutating func undo() {
        if let last = history.popLast() { marks = last }
    }

    public mutating func asItCame() {
        change(to: Self.firstMarks)
    }

    private mutating func change(to next: [Place: String]) {
        guard next != marks else { return }
        history.append(marks)
        marks = next
    }

    /// What a mark's press sends; nil for `⌨` and `⇄`, which the app answers itself.
    public static func bytes(for id: String) -> [UInt8]? {
        switch id {
        case "⌨", sessions: nil
        case "·": []
        case "/c": Array("/compact".utf8)
        default: KeyBar.bytes(for: id)
        }
    }

    /// What a held `←` or `→` sends once it moves a word at a time.
    public static func wordBytes(for id: String) -> [UInt8]? {
        switch id {
        case "←": [0x1b, UInt8(ascii: "b")]
        case "→": [0x1b, UInt8(ascii: "f")]
        default: nil
        }
    }

    /// Build 9's marks move here once: a place the owner changed keeps its key, and the rest
    /// take today's first marks.
    public static func load(from store: UserDefaults) -> HeadMarks {
        if let data = store.data(forKey: storeKey) {
            return (try? JSONDecoder().decode(HeadMarks.self, from: data)) ?? HeadMarks()
        }
        guard let data = store.data(forKey: buildNineKey) else { return HeadMarks() }
        var marks = HeadMarks()
        if let old = try? JSONDecoder().decode(HeadMarks.self, from: data) {
            for place in Place.allCases where old[place] != buildNineMarks[place] {
                try? marks.set(old[place], at: place)
            }
            marks.history = []
        }
        marks.save(to: store)
        store.removeObject(forKey: buildNineKey)
        return marks
    }

    public func save(to store: UserDefaults) {
        store.set(try? JSONEncoder().encode(self), forKey: Self.storeKey)
    }

    // MARK: Codable

    /// A stored mark the place cannot take (an unknown id, `^C`, sessions off the hold) is its first mark.
    public init(from decoder: Decoder) throws {
        let stored = try decoder.container(keyedBy: Place.self)
        for place in Place.allCases {
            if let id = try? stored.decodeIfPresent(String.self, forKey: place) {
                try? set(id, at: place)
            }
        }
        history = []
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: Place.self)
        for place in Place.allCases {
            try container.encode(marks[place], forKey: place)
        }
    }
}
