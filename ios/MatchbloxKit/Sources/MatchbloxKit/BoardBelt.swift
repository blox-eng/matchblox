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

    /// Board for a value the app cannot read: home with no card keys, and every key on.
    public static let plain = BoardBelt(shape: .board, card: [], armed: nil, dim: [])
    /// Board while the console does not show the board, or the screen is a pane: every key but
    /// `◂` is dim, as none of them means there what it means on the board.
    public static let away = BoardBelt(shape: .board, card: [], armed: nil, dim: Set(["up", "down", "enter"] + BoardBelt.tail))

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
        var row = [Self.key("◂")]
        switch shape {
        case .box:
            row.append(Self.key("runs"))
            row.append(Self.key("cancels"))
        case .menu:
            row.append(Self.key("up"))
            row.append(Self.key("down"))
            row.append(Self.key("do"))
            row.append(Self.key("close"))
        case .board:
            for token in card { row.append(Self.key(token)) }
            row.append(Self.key("up"))
            row.append(Self.key("down"))
            row.append(Self.key("enter"))
            for token in Self.tail { row.append(Self.key(token)) }
        }
        return row
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
        let spoken: String
        let bytes: [UInt8]
    }

    /// Each token's key: its id on the belt, its face, what VoiceOver says, and the bytes the
    /// console's binding reads. `◂`, the arrows and Return are the belt's own keys, and send as
    /// `KeyBar` says. A glyph is one cell, the arrows and Return too, so home (`◂`, three card
    /// keys, `↑ ↓ ⏎`) is the belt's seven cells at rest. A card key shows its glyph only: the
    /// card on the screen says what it does.
    private static let faces: [String: Face] = {
        let esc: UInt8 = 0x1b
        func own(_ id: String) -> Face { Face(id: id, label: id, spoken: id, bytes: []) }
        func word(_ token: String, _ label: String, _ spoken: String, _ bytes: [UInt8]? = nil) -> Face {
            Face(id: "board.\(token)", label: label, spoken: spoken, bytes: bytes ?? Array(token.utf8))
        }
        return [
            "◂": own("◂"), "up": own("↑"), "down": own("↓"), "enter": own("⏎"),
            "left": own("←"), "right": own("→"),
            "1": word("1", "1", "1, answer"), "2": word("2", "2", "2, answer"), "3": word("3", "3", "3, answer"),
            "p": word("p", "p", "p, merge"), "y": word("y", "y", "y, go"),
            "c": word("c", "c copy", "copy"), "m": word("m", "m menu", "menu"), "g": word("g", "g chain", "chain"),
            "v": word("v", "v one line", "one line"), "s": word("s", "s hosts", "hosts"),
            "u": word("u", "u strip", "strip"), "tab": word("tab", "⇥ machine", "machine", [0x09]),
            "x": word("x", "x kill", "kill"), "]": word("]", "]", "next chain"),
            "[": word("[", "[", "previous chain"), "/": word("/", "/", "find"), "z": word("z", "z park", "park"),
            "runs": word("runs", "y runs", "y, run", Array("y".utf8)),
            "cancels": word("cancels", "esc cancels", "escape, cancel", [esc]),
            "do": word("do", "⏎ do", "return, do", [0x0d]), "close": word("close", "esc close", "escape, close", [esc]),
        ]
    }()

    private static let tokens: [String: String] = Dictionary(uniqueKeysWithValues: faces.map { ($0.value.id, $0.key) })

    private static func key(_ token: String) -> BeltKey {
        let face = faces[token]!
        return BeltKey(id: face.id, cells: face.label.count > 1 ? 2 : 1)
    }

    private static func token(of id: String) -> String? { tokens[id] }

    /// The label of a Board key of the app's own (`board.<token>`), or nil for any other id.
    public static func label(of id: String) -> String? {
        guard id.hasPrefix("board."), let token = tokens[id] else { return nil }
        return faces[token]?.label
    }

    /// What VoiceOver says for a Board key of the app's own, or nil for any other id.
    public static func spoken(of id: String) -> String? {
        guard id.hasPrefix("board."), let token = tokens[id] else { return nil }
        return faces[token]?.spoken
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

    /// The Board layout's keys now: what the console said while it shows the board, else every
    /// key dim but `◂`.
    public var belt: BoardBelt {
        guard shows, let said else { return .away }
        return said
    }

    /// The terminal's long-press is the card's menu only while the console shows the board.
    public var opensMenu: Bool { shows }

    /// A sideways swipe turns the board's page only while the board shows with `←` and `→` on:
    /// not on Machine, under the consent box or in the menu.
    public var turnsPages: Bool {
        let now = self.belt
        return shows && now.shape == .board && !now.isDim("←") && !now.isDim("→")
    }

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
