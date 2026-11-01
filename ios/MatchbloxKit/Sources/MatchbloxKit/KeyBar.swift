/// What each belt key sends. The host's chat reads the bytes as its keys.
public enum KeyBar {
    private static let esc: UInt8 = 0x1b

    private static let fixed: [String: [UInt8]] = {
        var keys: [String: [UInt8]] = [
            "◂": HomeKey.bytes,
            "↑": [esc, 0x5b, 0x41],
            "↓": [esc, 0x5b, 0x42],
            "→": [esc, 0x5b, 0x43],
            "←": [esc, 0x5b, 0x44],
            "⏎": [0x0d],
            "Tab": [0x09],
            "⇧Tab": [esc, 0x5b, 0x5a],
            "Esc": [esc],
            "⌫": [0x7f],
            "⇞": Array("\u{1b}[5~".utf8),
            "⇟": Array("\u{1b}[6~".utf8),
        ]
        for letter in "ABCDLR" {
            keys["^\(letter)"] = [letter.asciiValue! - 0x40]
        }
        for digit in 1...9 {
            keys[String(digit)] = Array(String(digit).utf8)
        }
        return keys
    }()

    /// The bytes of a key id, or nil for an id that is no key. Any other id that starts with
    /// a lowercase letter, then has lowercase letters, digits and dashes, is an agent command:
    /// the console runs it, and refuses an id it does not know.
    public static func bytes(for id: String) -> [UInt8]? {
        if let bytes = fixed[id] { return bytes }
        guard isCommandID(id) else { return nil }
        return Array("\u{1b}_matchblox;cmd=\(id)\u{1b}\\".utf8)
    }

    static func isCommandID(_ id: String) -> Bool {
        let lower: ClosedRange<UInt8> = 0x61...0x7a
        let digit: ClosedRange<UInt8> = 0x30...0x39
        guard let first = id.utf8.first, lower.contains(first) else { return false }
        return id.utf8.allSatisfy { lower.contains($0) || digit.contains($0) || $0 == UInt8(ascii: "-") }
    }
}
