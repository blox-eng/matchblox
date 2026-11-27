import Foundation

/// One session as `matchblox sessions --json` lists it: the number `matchblox console --open <n>` opens, its name
/// and the word for what it does.
public struct SessionRow: Equatable, Sendable {
    public let n: Int
    public let name: String
    public let word: String

    public init(n: Int, name: String, word: String) {
        self.n = n; self.name = name; self.word = word
    }
}

public enum SessionList {
    static let maxName = 200

    /// A row as it comes; any field may be missing or of another kind, and drops only its own row.
    private struct Wire: Decodable {
        let n: Int?
        let name: String?
        let word: String?

        init(from decoder: Decoder) throws {
            let c = try decoder.container(keyedBy: CodingKeys.self)
            n = try? c.decode(Int.self, forKey: .n)
            name = try? c.decode(String.self, forKey: .name)
            word = try? c.decode(String.self, forKey: .word)
        }

        enum CodingKeys: String, CodingKey { case n, name, word }
    }

    /// The rows Pocket can open, in the host's order. A row with no number, or a name empty or over 200
    /// characters, is dropped. Nil when the list does not read.
    public static func decode(_ data: Data) -> [SessionRow]? {
        guard let wire = try? JSONDecoder().decode([Wire].self, from: data) else { return nil }
        return wire.compactMap { row in
            guard let n = row.n, n > 0, let name = row.name, (1...maxName).contains(name.count) else { return nil }
            return SessionRow(n: n, name: name, word: row.word ?? "")
        }
    }
}
