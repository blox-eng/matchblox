import Foundation

/// The board's two touches on the console's terminal. A sideways swipe turns the board's page:
/// a swipe to the right sends `←`, a swipe to the left sends `→`. A long-press sends a right
/// click at its cell, and the console opens the card's menu.
public enum BoardTouch {
    /// The console's long-press.
    public static let longPress: TimeInterval = 0.48
    /// A swipe turns the page once it has run this far, or left the finger this fast (pt/s),
    /// as the edge swipe does.
    public static let pageTravel: CGFloat = 60
    public static let pageSpeed: CGFloat = 500
    /// A swipe that starts this near the left side is the edge swipe's: it goes back.
    public static let edge: CGFloat = 24

    /// The wheel's rule turned round: a pan that runs more sideways than up or down is a page.
    public static func isSideways(dx: CGFloat, dy: CGFloat) -> Bool {
        abs(dx) > abs(dy)
    }

    public static func fromEdge(x: CGFloat) -> Bool {
        x < edge
    }

    /// The key a sideways swipe sends when the finger lifts, or nil for a swipe too short and
    /// too slow.
    public static func page(dx: CGFloat, velocity: CGFloat) -> [UInt8]? {
        guard abs(dx) > pageTravel || abs(velocity) > pageSpeed else { return nil }
        let right = dx != 0 ? dx > 0 : velocity > 0
        return KeyBar.bytes(for: right ? "←" : "→")
    }

    /// An SGR right-button press and release at a cell, from 0.
    public static func rightClick(col: Int, row: Int) -> [UInt8] {
        let at = "2;\(col + 1);\(row + 1)"
        return Array("\u{1b}[<\(at)M\u{1b}[<\(at)m".utf8)
    }
}
