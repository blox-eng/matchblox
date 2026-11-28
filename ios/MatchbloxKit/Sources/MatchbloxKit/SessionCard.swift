import Foundation

/// The sessions the hold shows. The session the console shows is marked and chosen first; a push
/// `↑` or `↓` steps the choice. The rows stay as they came while the finger is down, so a lift
/// goes to the row the card showed chosen, and a lift on the marked one goes nowhere.
public struct SessionCard: Equatable, Sendable {
    public static let unreachable = "sessions not reachable · try again"
    public static let alone = "no other session on this host"

    public let rows: [SessionRow]
    /// The row of the session the console shows; nil when it is not known.
    public let current: Int?
    /// The row a lift goes to; nil for none.
    public private(set) var chosen: Int?

    /// The current session is the row of `pane`, the pane the console shows, else the row of
    /// `opened`, the number this console opened, else none.
    public init(rows: [SessionRow], pane: String?, opened: Int?) {
        self.rows = rows
        let byPane = pane.flatMap { pane in rows.firstIndex { $0.pane == pane } }
        current = byPane ?? opened.flatMap { n in rows.firstIndex { $0.n == n } }
        chosen = current
    }

    /// One session or none: there is nowhere to go.
    public var isEmpty: Bool { rows.count < 2 }

    /// `by` rows down the list (negative: up), never past either end; with none marked, a step
    /// down chooses the first row and a step up chooses none. True when the choice changed.
    public mutating func step(by count: Int) -> Bool {
        guard !isEmpty else { return false }
        let next: Int?
        if let chosen {
            next = min(max(chosen + count, 0), rows.count - 1)
        } else {
            next = count > 0 ? min(count - 1, rows.count - 1) : nil
        }
        defer { chosen = next }
        return next != chosen
    }

    /// The number a lift opens: the chosen row's, never the current one's, and none when two rows
    /// share that number.
    public var lift: Int? {
        guard let chosen, chosen != current, rows.indices.contains(chosen) else { return nil }
        let n = rows[chosen].n
        guard rows.filter({ $0.n == n }).count == 1 else { return nil }
        return n
    }

    /// The card's text: a line a session, `▸` before the chosen one and `· here` after the current one.
    public var text: String {
        guard !isEmpty else { return Self.alone }
        return rows.indices.map { i in
            let row = rows[i]
            var line = (i == chosen ? "▸ " : "  ") + row.name
            if !row.word.isEmpty { line += " · " + row.word }
            if i == current { line += " · here" }
            return line
        }.joined(separator: "\n")
    }
}
