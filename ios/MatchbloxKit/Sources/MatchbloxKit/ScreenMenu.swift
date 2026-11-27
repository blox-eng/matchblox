import Foundation

/// The numbered menu an agent draws at the foot of the visible screen, read on the phone with the
/// rules of the host's `internal/menu`: the last run of options N down to 1, each with a `›`, `❯`
/// or `>` mark or blanks; under it only blank lines, box lines and a keys hint. One status line
/// may close the screen under all of that: a jumped tmux pane's, or the demo's key line.
public struct ScreenMenu: Equatable, Sendable {
    /// The options' texts, option 1 first, each with the lines it wraps onto joined.
    public let options: [String]
    /// The number of the option the agent's mark is on.
    public let chosen: Int

    public var count: Int { options.count }
    public var text: String { options[chosen - 1] }

    private static let marks: Set<Character> = ["›", "❯", ">"]

    /// The menu at the foot of `lines` (the screen's rows, from the top), or nil: no menu, a
    /// numbered list with other text under it, or no option marked.
    public static func read(lines: [String]) -> ScreenMenu? {
        let rows = lines.map(unboxed)
        var i = rows.count - 1
        var statusSkipped = false
        while i >= 0 {
            let row = rows[i]
            if row.isEmpty || isHint(row) {
                i -= 1
            } else if !statusSkipped, i == lastFilled(rows), option(in: row) == nil, indent(of: row) < 3, !isPrompt(row) {
                statusSkipped = true
                i -= 1
            } else {
                break
            }
        }
        var texts: [Int: String] = [:]
        var wrapped: [String] = []
        var want = 0
        var chosen = 0
        while i >= 0, want != 1 {
            let row = rows[i]
            if let found = option(in: row) {
                let n = found.n
                guard n >= 1, want == 0 || n == want - 1 else { return nil }
                if found.marked { chosen = n }
                texts[n] = ([found.text] + wrapped).joined(separator: " ")
                want = n
                wrapped = []
            } else if row.isEmpty {
                // a blank or a rule between options
            } else if indent(of: row) >= 3 {
                wrapped.insert(row.trimmingCharacters(in: .whitespaces), at: 0)
            } else {
                return nil
            }
            i -= 1
        }
        guard want == 1, chosen > 0 else { return nil }
        return ScreenMenu(options: (1...texts.count).map { texts[$0] ?? "" }, chosen: chosen)
    }

    /// The menu of the question the console's chat shows: the agent's screen ends over the chat's
    /// own line (`›`), its rule and its key line, which are no part of the menu. Nil with no chat
    /// line on the screen.
    public static func read(chat lines: [String]) -> ScreenMenu? {
        guard let line = lines.lastIndex(where: isChatLine) else { return nil }
        return read(lines: Array(lines[..<line]))
    }

    private static func isChatLine(_ line: String) -> Bool {
        let row = unboxed(line)
        return row.trimmingCharacters(in: .whitespaces).hasPrefix("›") && option(in: row) == nil
    }

    /// The index of the last row with anything on it: the only row a status line may take.
    private static func lastFilled(_ rows: [String]) -> Int {
        rows.lastIndex { !$0.isEmpty } ?? -1
    }

    /// The agent's input line (`> `) is never a status line: a numbered list over it is no menu.
    private static func isPrompt(_ row: String) -> Bool {
        let t = row.trimmingCharacters(in: .whitespaces)
        return t.hasPrefix(">") || t.hasPrefix("›") || t.hasPrefix("❯")
    }

    /// An option row: blanks, a mark or none, blanks, its number, a dot, a blank, its text.
    private static func option(in row: String) -> (marked: Bool, n: Int, text: String)? {
        var rest = Substring(row).drop { $0.isWhitespace }
        let marked = rest.first.map { marks.contains($0) } ?? false
        if marked { rest = rest.dropFirst().drop { $0.isWhitespace } }
        let digits = rest.prefix { $0.isASCII && $0.isNumber }
        guard !digits.isEmpty, let n = Int(digits) else { return nil }
        rest = rest.dropFirst(digits.count)
        guard rest.first == ".", let gap = rest.dropFirst().first, gap.isWhitespace else { return nil }
        return (marked, n, rest.dropFirst().trimmingCharacters(in: .whitespaces))
    }

    /// The keys line under a menu ("Enter to select · ↑/↓ to navigate · Esc to cancel").
    private static func isHint(_ row: String) -> Bool {
        let words = row.lowercased().split { !($0.isLetter || $0.isNumber || $0 == "_") }
        return zip(words, words.dropFirst()).contains { $0 == "to" && ["select", "navigate", "cancel"].contains($1) }
    }

    /// A row with a box's sides and its end blanks taken off; "" for a blank row or a box's rule.
    private static func unboxed(_ line: String) -> String {
        var l = trimEnd(line)
        let left = l.drop { $0 == " " }
        if left.hasPrefix("│") {
            var inner = trimEnd(String(left.dropFirst()))
            if inner.hasSuffix("│") { inner.removeLast() }
            l = trimEnd(inner)
        }
        if l.allSatisfy({ " ─│╭╮╰╯".contains($0) }) { return "" }
        return l
    }

    private static func trimEnd(_ s: String) -> String {
        var s = s
        while s.last == " " { s.removeLast() }
        return s
    }

    private static func indent(of row: String) -> Int {
        row.prefix { $0 == " " }.count
    }
}
