import Foundation

/// The nine keys above the system keyboard. The host's chat reads the bytes as its keys.
public enum KeyBar {
    public struct Key: Sendable {
        public enum Action: Sendable, Equatable {
            case send([UInt8])
            /// Shows or hides the system keyboard.
            case keyboard
        }

        public let label: String
        public let action: Action
        public let accessibilityLabel: String
        /// The width, in widths of a plain key.
        public let width: CGFloat
        /// The height, in points.
        public let height: CGFloat

        init(_ label: String, _ action: Action, _ accessibilityLabel: String, width: CGFloat = 1, height: CGFloat = KeyBar.minSide) {
            self.label = label
            self.action = action
            self.accessibilityLabel = accessibilityLabel
            self.width = width
            self.height = height
        }
    }

    /// The smallest side of a key, in points: Apple's smallest tap target.
    public static let minSide: CGFloat = 44
    /// The arrows are the keys a thumb looks for most.
    static let arrowWidth: CGFloat = 1.5
    static let arrowHeight: CGFloat = 52

    public static let keys: [Key] = [
        Key("◂", .send(HomeKey.bytes), "Back to matchblox"),
        Key("↑", .send([0x1b, 0x5b, 0x41]), "Up", width: arrowWidth, height: arrowHeight),
        Key("↓", .send([0x1b, 0x5b, 0x42]), "Down", width: arrowWidth, height: arrowHeight),
        Key("⏎", .send([0x0d]), "Return"),
        Key("Tab", .send([0x09]), "Tab"),
        Key("⇧Tab", .send([0x1b, 0x5b, 0x5a]), "Shift Tab"),
        Key("Esc", .send([0x1b]), "Escape"),
        Key("^C", .send([0x03]), "Interrupt"),
        Key("⌨", .keyboard, "Keyboard"),
    ]

    /// The height of the tallest key.
    public static let height = keys.map(\.height).max() ?? minSide
}
