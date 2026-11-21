import Foundation

/// A link the console shows. A tap on one asks first: `Open in Safari` for http and https,
/// `Copy link` for any link.
public enum TerminalLink {
    private static let schemes: [[Character]] = ["https://", "http://"].map { Array($0) }
    /// The characters of a URL (RFC 3986). Any other character, a space, a cell the program
    /// never drew, a box line, ends it.
    private static let urlCharacters = Set("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~:/?#[]@!$&'()*+,;=%")
    /// Punctuation after a URL in a sentence: never its last character.
    private static let trailing = Set(".,;:!?'\"*")
    private static let pairs: [Character: Character] = [")": "(", "]": "[", "}": "{"]

    /// A row's URL that ends this close to the row's right edge may go on in the next row.
    static let edgeSlack = 10

    /// The http or https URL in `cells`, one character for each cell of a row, that covers the
    /// column `col`; nil when the cell is not in one.
    public static func url(in cells: [Character], at col: Int) -> String? {
        url(in: [cells], row: 0, col: col)
    }

    /// The http or https URL that covers the cell at `col` of `rows[row]`; nil when the cell is
    /// not in one. An agent draws a long URL over several rows: a URL that runs to the right
    /// edge of its row goes on in the next row when that row holds an indent and one word only.
    public static func url(in rows: [[Character]], row: Int, col: Int) -> String? {
        guard rows.indices.contains(row), let tapped = run(in: rows[row], at: col) else { return nil }
        var top = row, topRun = tapped
        var head = schemeStart(rows[top], in: topRun, upTo: top == row ? col : nil)
        while head == nil, top > 0, topRun == onlyRun(rows[top]), let up = lastRun(rows[top - 1]),
              goesOn(rows[top - 1], up, into: rows[top], topRun) {
            top -= 1
            topRun = up
            head = schemeStart(rows[top], in: topRun, upTo: nil)
        }
        guard let head else { return nil }

        var url = Array(rows[top][head..<topRun.upperBound])
        var at = top, atRun = topRun
        while at + 1 < rows.count, let next = onlyRun(rows[at + 1]), goesOn(rows[at], atRun, into: rows[at + 1], next) {
            at += 1
            atRun = next
            url += rows[at][next]
        }
        guard at >= row else { return nil }
        while let last = url.last, trailing.contains(last) || isUnpaired(last, in: url) {
            url.removeLast()
        }
        if top == row, at == row, col >= head + url.count { return nil }
        let text = String(url)
        guard URLComponents(string: text)?.host?.isEmpty == false else { return nil }
        return text
    }

    /// The cells of URL characters round `col`.
    private static func run(in cells: [Character], at col: Int) -> Range<Int>? {
        guard cells.indices.contains(col), urlCharacters.contains(cells[col]) else { return nil }
        var start = col
        while start > 0, urlCharacters.contains(cells[start - 1]) { start -= 1 }
        var end = col + 1
        while end < cells.count, urlCharacters.contains(cells[end]) { end += 1 }
        return start..<end
    }

    /// Where a scheme starts in `run`, at or before `limit` when given.
    private static func schemeStart(_ cells: [Character], in run: Range<Int>, upTo limit: Int?) -> Int? {
        run.first { startsScheme(cells, at: $0) && $0 <= (limit ?? Int.max) }
    }

    /// The row's last run of URL characters, when nothing but blanks or box lines follows it.
    private static func lastRun(_ cells: [Character]) -> Range<Int>? {
        guard let last = cells.lastIndex(where: { !isFiller($0) }) else { return nil }
        return run(in: cells, at: last)
    }

    /// The row's one run of URL characters, with nothing but blanks or box lines round it.
    private static func onlyRun(_ cells: [Character]) -> Range<Int>? {
        guard let first = cells.firstIndex(where: { !isFiller($0) }), let found = run(in: cells, at: first),
              cells[found.upperBound...].allSatisfy(isFiller) else { return nil }
        return found
    }

    /// `run` reaches the right edge of `cells`, and `next` is a lone word with no scheme of its own.
    private static func goesOn(_ cells: [Character], _ run: Range<Int>, into nextCells: [Character], _ next: Range<Int>) -> Bool {
        run.upperBound >= cells.count - edgeSlack && cells[run.upperBound...].allSatisfy(isFiller)
            && !startsScheme(nextCells, at: next.lowerBound)
    }

    /// A blank, a cell never drawn, or a box line.
    private static func isFiller(_ c: Character) -> Bool {
        c == " " || c == "\0" || c.unicodeScalars.allSatisfy { (0x2500...0x257F).contains($0.value) }
    }

    /// Only a web link opens in the browser; any other link may only be copied.
    public static func opens(_ link: String) -> Bool {
        guard let scheme = URL(string: link)?.scheme?.lowercased() else { return false }
        return scheme == "http" || scheme == "https"
    }

    private static func startsScheme(_ cells: [Character], at i: Int) -> Bool {
        schemes.contains { scheme in
            i + scheme.count <= cells.count
                && zip(cells[i..<i + scheme.count], scheme).allSatisfy { $0.lowercased() == String($1) }
        }
    }

    /// A closing bracket with no opening one in the URL belongs to the sentence round it.
    private static func isUnpaired(_ last: Character, in url: [Character]) -> Bool {
        guard let open = pairs[last] else { return false }
        return url.filter { $0 == open }.count < url.filter { $0 == last }.count
    }
}
