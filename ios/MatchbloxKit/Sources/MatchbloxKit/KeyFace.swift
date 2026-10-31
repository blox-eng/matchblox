import Foundation

/// What a belt key shows: its label, the size of the label, the agent's mark on a command,
/// and the words VoiceOver says for it.
public struct KeyFace: Equatable, Sendable {
    public enum Size: Sendable, Equatable {
        /// One glyph: `◂`, `⇞`, a digit.
        case glyph
        /// The two-cell arrows and Return, the keys a thumb looks for most.
        case big
        /// A key name or a command: `Esc`, `⇧Tab`, `compact`.
        case word
    }

    public let label: String
    public let size: Size
    /// The asset name of the agent's mark, on a command key only.
    public let mark: String?
    public let isCommand: Bool
    public let spoken: String

    private static let spokenNames = [
        "◂": "Back to matchblox", "↑": "Up", "↓": "Down", "←": "Left", "→": "Right",
        "⏎": "Return", "Tab": "Tab", "⇧Tab": "Shift Tab", "Esc": "Escape", "⌫": "Delete",
        "⇞": "Page up", "⇟": "Page down", "^C": "Interrupt",
    ]

    public init(_ key: BeltKey, in catalog: KeyCatalog) {
        if let command = catalog.command(key.id) {
            label = command.label
            size = .word
            mark = command.mark
            isCommand = true
            spoken = "/\(command.label), \(KeyCatalog.agentNames[command.agent] ?? command.agent)"
            return
        }
        label = key.id
        size = key.cells == 2 ? .big : key.id.count > 1 ? .word : .glyph
        mark = nil
        isCommand = false
        if let name = Self.spokenNames[key.id] {
            spoken = name
        } else if key.id.hasPrefix("^") {
            spoken = "Control \(key.id.dropFirst())"
        } else {
            spoken = key.id
        }
    }
}

extension KeyCatalog {
    /// The mark of an agent's commands, or nil for an agent with none.
    public func mark(of agent: String?) -> String? {
        commands.first { $0.agent == agent }?.mark
    }
}

extension BeltLayout {
    /// The UserDefaults key of the layout in use.
    public static let storeKey = "belt.layout"

    /// Steer until the builder picks another; a stored value the app does not know is Steer.
    public static func load(from store: UserDefaults) -> BeltLayout {
        store.string(forKey: storeKey).flatMap(BeltLayout.init(rawValue:)) ?? .steer
    }

    public func save(to store: UserDefaults) {
        store.set(rawValue, forKey: Self.storeKey)
    }
}
