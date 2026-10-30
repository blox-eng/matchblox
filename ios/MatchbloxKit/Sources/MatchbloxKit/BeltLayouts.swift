import Foundation

public enum BeltLayout: String, CaseIterable, Codable, Sendable {
    case steer, commands, tmux, mine
}

/// One row of the console's command table (`testdata/keys/commands.json`). A command key
/// shows `label` with the `mark` of its agent.
public struct KeyCommand: Decodable, Equatable, Sendable {
    public let id: String
    public let label: String
    public let agent: String
    public let mark: String
}

/// Every key the belt can hold: the fixed keys and the agents' commands.
public struct KeyCatalog: Sendable {
    public enum Failure: Error, Equatable {
        /// An id that is no command id: it must start with a lowercase letter, then have
        /// lowercase letters, digits and dashes only.
        case badCommand(String)
    }

    /// The agents of the command table, as the console names them.
    public static let agentNames = ["claude": "Claude Code", "codex": "Codex", "opencode": "OpenCode", "cursor": "Cursor"]

    static let homeRow = row("◂", "↑", "↓", "⏎")
    static let steer = homeRow + row("Esc", "^C", "⇞", "⇟")
    static let edit = row("Tab", "⇧Tab", "←", "→", "⌫")
    static let digits = row("1", "2", "3", "4", "5", "6", "7", "8", "9")
    static let ctrl = row("^A", "^B", "^D", "^L", "^R")
    static let tmux = homeRow + row("⇞", "⇟", "Tab", "⇧Tab", "^B")

    /// `↑ ↓ ⏎` are two cells wide, so at home they fill the screen right of `◂`.
    private static func row(_ ids: String...) -> [BeltKey] {
        ids.map { BeltKey(id: $0, cells: ["↑", "↓", "⏎"].contains($0) ? 2 : 1) }
    }

    public let commands: [KeyCommand]
    private let known: [String: BeltKey]

    public init(commands json: Data) throws {
        commands = try JSONDecoder().decode([KeyCommand].self, from: json)
        if let bad = commands.first(where: { !KeyBar.isCommandID($0.id) }) {
            throw Failure.badCommand(bad.id)
        }
        let fixed = Self.steer + Self.edit + Self.digits + Self.ctrl
        known = Dictionary((fixed + commands.map(Self.key)).map { ($0.id, $0) }) { first, _ in first }
    }

    public func command(_ id: String) -> KeyCommand? {
        commands.first { $0.id == id }
    }

    /// Commands follows `agent`, the agent of the session the console shows. With no agent,
    /// or one with no commands, it is the home row only. Mine keeps the keys of the catalog,
    /// and starts at `◂`, its home stop, in the ring order of the editor.
    public func keys(_ layout: BeltLayout, agent: String?, mine: [BeltKey]) -> [BeltKey] {
        switch layout {
        case .steer: Self.steer
        case .tmux: Self.tmux
        case .commands: Self.homeRow + commands.filter { $0.agent == agent }.map(Self.key)
        case .mine: Self.fromHome(mine.compactMap { known[$0.id] })
        }
    }

    /// `keys` turned on the ring so that `◂` comes first.
    private static func fromHome(_ keys: [BeltKey]) -> [BeltKey] {
        guard let home = keys.firstIndex(where: { $0.id == homeRow[0].id }) else { return keys }
        return Array(keys[home...] + keys[..<home])
    }

    /// The editor's groups: Steer, Edit, Digits, Ctrl, then one for each agent in table order.
    public var groups: [(name: String, keys: [BeltKey])] {
        var agents: [String] = []
        for c in commands where !agents.contains(c.agent) { agents.append(c.agent) }
        var groups: [(name: String, keys: [BeltKey])] = [
            ("Steer", Self.steer), ("Edit", Self.edit), ("Digits", Self.digits), ("Ctrl", Self.ctrl),
        ]
        for agent in agents {
            groups.append((Self.agentNames[agent] ?? agent, commands.filter { $0.agent == agent }.map(Self.key)))
        }
        return groups
    }

    private static func key(_ command: KeyCommand) -> BeltKey { BeltKey(id: command.id, cells: 2) }
}

/// The builder's own layout: one per device, for every host. `◂` is always in it, as it is the
/// way back; it may move.
public struct Mine: Codable, Equatable, Sendable {
    /// The UserDefaults key of the encoded Mine.
    public static let storeKey = "belt.mine"
    public static let defaults = Mine(keys: KeyCatalog.homeRow)
    private static let home = BeltKey(id: "◂", cells: 1)

    public private(set) var keys: [BeltKey]

    /// Each id once; `◂` goes first when it is missing.
    public init(keys: [BeltKey]) {
        var seen = Set<String>()
        var kept = keys.filter { seen.insert($0.id).inserted }
        if !seen.contains(Self.home.id) { kept.insert(Self.home, at: 0) }
        self.keys = kept
    }

    /// Adds the key at the end, or takes it off. Returns false, and changes nothing, for `◂`.
    public mutating func toggle(_ key: BeltKey) -> Bool {
        guard key.id != Self.home.id else { return false }
        if let i = keys.firstIndex(where: { $0.id == key.id }) {
            keys.remove(at: i)
        } else {
            keys.append(key)
        }
        return true
    }

    /// A move out of range does nothing.
    public mutating func move(from: Int, to: Int) {
        guard keys.indices.contains(from), keys.indices.contains(to) else { return }
        keys.insert(keys.remove(at: from), at: to)
    }

    public static func load(from store: UserDefaults) -> Mine {
        guard let data = store.data(forKey: storeKey),
              let mine = try? JSONDecoder().decode(Mine.self, from: data) else { return defaults }
        return mine
    }

    public func save(to store: UserDefaults) {
        store.set(try? JSONEncoder().encode(self), forKey: Self.storeKey)
    }

    // MARK: Codable

    private enum CodingKeys: String, CodingKey { case keys }

    private struct Stored: Codable {
        let id: String
        let cells: Int
    }

    /// A stored row the app cannot use (a bad cell count, a missing field) is dropped, so
    /// stored data never stops the app.
    private struct Lenient: Decodable {
        let key: BeltKey?

        init(from decoder: Decoder) throws {
            guard let row = try? Stored(from: decoder), row.cells == 1 || row.cells == 2 else {
                key = nil
                return
            }
            key = BeltKey(id: row.id, cells: row.cells)
        }
    }

    public init(from decoder: Decoder) throws {
        let rows = try decoder.container(keyedBy: CodingKeys.self).decode([Lenient].self, forKey: .keys)
        self.init(keys: rows.compactMap(\.key))
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(keys.map { Stored(id: $0.id, cells: $0.cells) }, forKey: .keys)
    }
}
