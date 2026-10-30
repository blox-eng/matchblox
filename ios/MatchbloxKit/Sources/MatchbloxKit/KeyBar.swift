import Foundation

/// The seven keys above the system keyboard. The host's chat reads these bytes as its keys.
public enum KeyBar {
    public struct Key: Sendable {
        public let label: String
        public let bytes: [UInt8]
        public let accessibilityLabel: String
    }

    public static let keys: [Key] = [
        Key(label: "↑", bytes: [0x1b, 0x5b, 0x41], accessibilityLabel: "Up"),
        Key(label: "↓", bytes: [0x1b, 0x5b, 0x42], accessibilityLabel: "Down"),
        Key(label: "⏎", bytes: [0x0d], accessibilityLabel: "Return"),
        Key(label: "Tab", bytes: [0x09], accessibilityLabel: "Tab"),
        Key(label: "⇧Tab", bytes: [0x1b, 0x5b, 0x5a], accessibilityLabel: "Shift Tab"),
        Key(label: "Esc", bytes: [0x1b], accessibilityLabel: "Escape"),
        Key(label: "^C", bytes: [0x03], accessibilityLabel: "Control C"),
    ]

    /// The smallest side of a key, in points: Apple's smallest tap target.
    public static let minSide: CGFloat = 44
}
