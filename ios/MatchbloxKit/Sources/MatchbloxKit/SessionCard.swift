import Foundation

/// The sessions the hold shows. The session the console shows is marked and chosen first; a turn
/// round the head's centre steps the choice. The rows stay as they came while the finger is down,
/// so a lift goes to the row the card showed chosen, and a lift on the marked one goes nowhere.
public struct SessionCard: Equatable, Sendable {
    /// A turn this wide steps one session (30°).
    public static let step: CGFloat = .pi / 6
    /// Nearer the centre than this share of the radius, the finger's angle is not steady.
    public static let steadyShare: CGFloat = 0.25
    public static let unreachable = "sessions not reachable · try again"
    public static let alone = "no other session on this host"

    public let rows: [SessionRow]
    /// The row of the session the console shows; nil when it is not known.
    public let current: Int?
    /// The row a lift goes to; nil for none.
    public private(set) var chosen: Int?
    private var angle: CGFloat?
    private var turned: CGFloat = 0

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

    /// The finger at `point` from the head's centre (UIKit points, y down) on a head of `radius`.
    /// Clockwise steps down the list; with none marked, the first step clockwise chooses the first
    /// row. A turn past either end is not counted, so a turn back steps at once. True when the
    /// choice changed.
    public mutating func moved(to point: CGPoint, radius: CGFloat) -> Bool {
        guard !isEmpty else { return false }
        guard hypot(point.x, point.y) >= radius * Self.steadyShare else {
            angle = nil
            return false
        }
        let to = atan2(point.y, point.x)
        if let angle { turned += (to - angle).remainder(dividingBy: 2 * .pi) }
        angle = to
        let base = current ?? -1
        turned = min(max(turned, CGFloat(-max(base, 0)) * Self.step), CGFloat(rows.count - 1 - base) * Self.step)
        let steps = Int((turned / Self.step + (turned < 0 ? -1e-6 : 1e-6)).rounded(.towardZero))
        let index = base + steps
        let next: Int? = index < 0 ? nil : index
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
