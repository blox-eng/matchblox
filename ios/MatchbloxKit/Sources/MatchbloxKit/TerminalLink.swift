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

    /// The http or https URL in `cells`, one character for each cell of a row, that covers the
    /// column `col`; nil when the cell is not in one. A URL that wraps to the next row ends at
    /// the row's end.
    public static func url(in cells: [Character], at col: Int) -> String? {
        guard cells.indices.contains(col), urlCharacters.contains(cells[col]) else { return nil }
        var start = col
        while start > 0, urlCharacters.contains(cells[start - 1]) { start -= 1 }
        var end = col + 1
        while end < cells.count, urlCharacters.contains(cells[end]) { end += 1 }

        guard let head = (start...col).first(where: { startsScheme(cells, at: $0) }) else { return nil }
        var url = Array(cells[head..<end])
        while let last = url.last, trailing.contains(last) || isUnpaired(last, in: url) {
            url.removeLast()
        }
        guard col < head + url.count else { return nil }
        let text = String(url)
        guard URLComponents(string: text)?.host?.isEmpty == false else { return nil }
        return text
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
