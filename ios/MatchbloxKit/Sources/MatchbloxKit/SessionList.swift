import Foundation

/// One session as `matchblox sessions --json` lists it: the number `matchblox console --open <n>` opens, its name,
/// the word for what it does, and its pane, which tells the session the console shows.
public struct SessionRow: Equatable, Sendable {
    public let n: Int
    public let name: String
    public let word: String
    /// A bare pane id (`%` and digits), or nil.
    public let pane: String?

    public init(n: Int, name: String, word: String, pane: String? = nil) {
        self.n = n; self.name = name; self.word = word; self.pane = pane
    }
}

public enum SessionList {
    static let maxName = 200

    /// A row as it comes; any field may be missing or of another kind, and drops only its own row.
    private struct Wire: Decodable {
        let n: Int?
        let name: String?
        let word: String?
        let pane: String?

        init(from decoder: Decoder) throws {
            let c = try decoder.container(keyedBy: CodingKeys.self)
            n = try? c.decode(Int.self, forKey: .n)
            name = try? c.decode(String.self, forKey: .name)
            word = try? c.decode(String.self, forKey: .word)
            pane = try? c.decode(String.self, forKey: .pane)
        }

        enum CodingKeys: String, CodingKey { case n, name, word, pane }
    }

    /// The rows Pocket can open, in the host's order. A row with no number, or a name empty or over 200
    /// characters, is dropped; a pane that is not a bare pane id reads as none. Nil when the list does not read.
    public static func decode(_ data: Data) -> [SessionRow]? {
        guard let wire = try? JSONDecoder().decode([Wire].self, from: data) else { return nil }
        return wire.compactMap { row in
            guard let n = row.n, n > 0, let name = row.name, (1...maxName).contains(name.count) else { return nil }
            return SessionRow(n: n, name: name, word: row.word ?? "", pane: row.pane.flatMap { BoardBelt.pane($0[...]) })
        }
    }
}
