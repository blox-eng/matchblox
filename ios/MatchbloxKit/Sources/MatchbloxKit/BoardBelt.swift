import Foundation

/// The Board layout's keys, as the console's board signal says them (`keycmd.BoardSignal`,
/// `beltWord` in Go). The value is `box` under the consent box, `menu` under the card's menu,
/// or `card;dim`: the chosen card's keys that are on (`*` after the armed one), then the keys
/// that are off. Home is `◂`, the card's keys (at most three), then `↑ ↓ ⏎`; the tail has every
/// other key of the board. A key that is off is dim and sends nothing. A token the app does not
/// know is not drawn.
public struct BoardBelt: Equatable, Sendable {
    public enum Shape: Equatable, Sendable { case board, box, menu }

    public let shape: Shape
    /// The card's keys on the belt, as tokens: the keys that are on, then the card's keys that
    /// are off.
    public let card: [String]
    public let armed: String?
    /// The tokens of the keys that are off.
    public let dim: Set<String>

    /// The app's answer to each value it takes while its belt shows Board (`keycmd.BeltKey`).
    public static let answer = Array("\u{1b}_matchblox;belt\u{1b}\\".utf8)

    /// Board with no value from the console: home with no card keys, and every key on.
    public static let plain = BoardBelt(shape: .board, card: [], armed: nil, dim: [])

    private static let cardTokens: Set<String> = ["1", "2", "3", "p", "y"]
    private static let maxCard = 3
    private static let tail = ["c", "m", "g", "v", "s", "u", "tab", "x", "left", "right", "]", "[", "/", "z"]

    private init(shape: Shape, card: [String], armed: String?, dim: Set<String>) {
        self.shape = shape
        self.card = card
        self.armed = armed
        self.dim = dim
    }

    /// The board a value says, or nil for the empty value: the console left the board. A value
    /// with no `;` that is not `box` or `menu` is plain Board.
    public init?(said value: String) {
        switch value {
        case "": return nil
        case "box": self.init(shape: .box, card: [], armed: nil, dim: [])
        case "menu": self.init(shape: .menu, card: [], armed: nil, dim: [])
        default:
            guard let split = value.firstIndex(of: ";") else {
                self.init(shape: .board, card: [], armed: nil, dim: [])
                return
            }
            let list = { (part: Substring) in part.split(separator: ",").map(String.init) }
            var card: [String] = []
            var armed: String?
            for token in list(value[..<split]) {
                let key = token.hasSuffix("*") ? String(token.dropLast()) : token
                guard Self.cardTokens.contains(key), !card.contains(key) else { continue }
                card.append(key)
                if key != token { armed = key }
            }
            let dim = list(value[value.index(after: split)...])
            let off = dim.filter { Self.cardTokens.contains($0) && !card.contains($0) }
            card += off
            self.init(shape: .board, card: Array(card.prefix(Self.maxCard)), armed: armed, dim: Set(dim))
        }
    }

    /// The layout's keys, in order.
    public var keys: [BeltKey] {
        let home = Self.key("◂")
        let arrows = ["up", "down"].map(Self.key)
        switch shape {
        case .box: return [home, Self.key("runs"), Self.key("cancels")]
        case .menu: return [home] + arrows + [Self.key("do"), Self.key("close")]
        case .board: return [home] + card.map(Self.key) + arrows + [Self.key("enter")] + Self.tail.map(Self.key)
        }
    }

    /// Dim, and sends nothing: a key the console says is off.
    public func isDim(_ id: String) -> Bool {
        guard shape == .board, let token = Self.token(of: id) else { return false }
        return dim.contains(token)
    }

    /// In the accent: the card's keys that are on, and `y runs`.
    public func isLit(_ id: String) -> Bool {
        guard let token = Self.token(of: id) else { return false }
        switch shape {
        case .board: return card.contains(token) && !dim.contains(token)
        case .box: return token == "runs"
        case .menu: return false
        }
    }

    /// The card key a first press marked: the same key again sends.
    public func isArmed(_ id: String) -> Bool {
        shape == .board && armed != nil && Self.token(of: id) == armed
    }

    // MARK: The keys

    private struct Face: Sendable {
        let id: String
        let label: String
        let bytes: [UInt8]
    }

    /// Each token's key: its id on the belt, its face, and the bytes the console's binding reads.
    /// `◂`, the arrows and Return are the belt's own keys, and send as `KeyBar` says.
    private static let faces: [String: Face] = {
        let esc: UInt8 = 0x1b
        func own(_ id: String) -> Face { Face(id: id, label: id, bytes: []) }
        func word(_ token: String, _ label: String, _ bytes: [UInt8]? = nil) -> Face {
            Face(id: "board.\(token)", label: label, bytes: bytes ?? Array(token.utf8))
        }
        return [
            "◂": own("◂"), "up": own("↑"), "down": own("↓"), "enter": own("⏎"),
            "left": own("←"), "right": own("→"),
            "1": word("1", "1 answer"), "2": word("2", "2 answer"), "3": word("3", "3 answer"),
            "p": word("p", "p merge"), "y": word("y", "y go"),
            "c": word("c", "c copy"), "m": word("m", "m menu"), "g": word("g", "g chain"),
            "v": word("v", "v one line"), "s": word("s", "s hosts"), "u": word("u", "u strip"),
            "tab": word("tab", "⇥ machine", [0x09]), "x": word("x", "x kill"),
            "]": word("]", "]"), "[": word("[", "["), "/": word("/", "/"), "z": word("z", "z park"),
            "runs": word("runs", "y runs", Array("y".utf8)), "cancels": word("cancels", "esc cancels", [esc]),
            "do": word("do", "⏎ do", [0x0d]), "close": word("close", "esc close", [esc]),
        ]
    }()

    private static let tokens: [String: String] = Dictionary(uniqueKeysWithValues: faces.map { ($0.value.id, $0.key) })

    private static func key(_ token: String) -> BeltKey {
        let face = faces[token]!
        return BeltKey(id: face.id, cells: face.label.count > 1 || ["up", "down", "enter"].contains(token) ? 2 : 1)
    }

    private static func token(of id: String) -> String? { tokens[id] }

    /// The label of a Board key of the app's own (`board.<token>`), or nil for any other id.
    public static func label(of id: String) -> String? {
        guard id.hasPrefix("board."), let token = tokens[id] else { return nil }
        return faces[token]?.label
    }

    /// The bytes of a Board key of the app's own, or nil for any other id.
    public static func bytes(for id: String) -> [UInt8]? {
        guard id.hasPrefix("board."), let token = tokens[id] else { return nil }
        return faces[token]?.bytes
    }
}

/// The belt follows the console's board, as Commands follows the agent: it turns to Board when
/// the console starts to show the board, and back to the layout it had when the console leaves
/// the board or the owner goes into a pane. A layout the owner picks in the tray stays theirs.
public struct BoardFollow: Sendable {
    /// What the console said last; nil once it left the board.
    public private(set) var said: BoardBelt?
    private var inPane = false
    /// The layout the belt had when it turned to Board by itself.
    private var before: BeltLayout?

    public init() {}

    /// The console shows the board, and the terminal shows the console.
    public var shows: Bool { said != nil && !inPane }

    /// The console sent `value`. Returns the layout to turn to, or nil to stay.
    public mutating func heard(_ value: String, on layout: BeltLayout) -> BeltLayout? {
        let was = shows
        said = BoardBelt(said: value)
        return turn(from: was, on: layout)
    }

    /// The terminal shows a pane's way back, or the console. Returns the layout to turn to.
    public mutating func pane(_ inPane: Bool, on layout: BeltLayout) -> BeltLayout? {
        let was = shows
        self.inPane = inPane
        return turn(from: was, on: layout)
    }

    /// The owner picked `layout` in the tray.
    public mutating func picked() {
        before = nil
    }

    /// The belt answers the console while it shows Board and the console shows the board.
    public func answers(on layout: BeltLayout) -> Bool {
        layout == .board && shows
    }

    private mutating func turn(from was: Bool, on layout: BeltLayout) -> BeltLayout? {
        if shows && !was && layout != .board {
            before = layout
            return .board
        }
        if !shows && was, let before {
            self.before = nil
            return layout == .board ? before : nil
        }
        return nil
    }
}
