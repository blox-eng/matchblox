import Foundation

/// The menu a session draws, as `matchblox menu <who> --json` prints it: its options, the one the agent marks, and
/// the sign the host checks a pick against.
public struct MenuSight: Equatable, Sendable {
    public struct Option: Equatable, Sendable {
        public let n: Int
        public let text: String

        public init(n: Int, text: String) {
            self.n = n; self.text = text
        }
    }

    public let options: [Option]
    public let chosen: Int
    public let sign: String

    public init(options: [Option], chosen: Int, sign: String) {
        self.options = options; self.chosen = chosen; self.sign = sign
    }

    /// A pick is one digit, so a menu has at most 9 options.
    static let maxOptions = 9
    static let maxText = 500
    static let maxSign = 64

    private struct Wire: Decodable {
        struct Option: Decodable {
            let n: Int
            let text: String
        }

        let options: [Option]
        let chosen: Int
        let sign: String
    }

    /// The menu Pocket can pick from, or nil: no menu, options not 1…n in order, more than 9, a text empty or over
    /// 500 characters, a mark on no option, or a sign that is not a short hex.
    public static func decode(_ data: Data) -> MenuSight? {
        guard let wire = try? JSONDecoder().decode(Wire.self, from: data),
              (1...maxOptions).contains(wire.options.count),
              wire.options.enumerated().allSatisfy({
                  $0.element.n == $0.offset + 1 && !$0.element.text.isEmpty && $0.element.text.count <= maxText
              }),
              (1...wire.options.count).contains(wire.chosen),
              (1...maxSign).contains(wire.sign.count),
              wire.sign.allSatisfy({ $0.isASCII && $0.isHexDigit })
        else { return nil }
        return MenuSight(options: wire.options.map { Option(n: $0.n, text: $0.text) }, chosen: wire.chosen, sign: wire.sign)
    }
}
