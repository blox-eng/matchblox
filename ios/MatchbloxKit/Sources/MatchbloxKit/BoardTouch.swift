import Foundation

/// The console's touches on its terminal. The list and the chat have no pages, so a sideways
/// swipe sends nothing. The edge swipe goes back: it sends `◂`, as the belt's key does. A tap is
/// a click, and the console opens the row's chat. A long-press sends a right click at its cell,
/// and the console opens the row's menu.
public enum BoardTouch {
    /// The console's long-press.
    public static let longPress: TimeInterval = 0.48
    /// The edge swipe goes back once it has run this far, or left the finger this fast (pt/s).
    public static let backTravel: CGFloat = 60
    public static let backSpeed: CGFloat = 500

    /// The key a sideways swipe sends when the finger lifts: `◂` for the edge swipe that ran far
    /// or fast enough, else nil. A swipe that does not start at the edge turns no page.
    public static func sideways(fromEdge: Bool, dx: CGFloat, velocity: CGFloat) -> [UInt8]? {
        guard fromEdge, dx > backTravel || velocity > backSpeed else { return nil }
        return KeyBar.bytes(for: "◂")
    }

    /// An SGR right-button press and release at a cell, from 0.
    public static func rightClick(col: Int, row: Int) -> [UInt8] {
        let at = "2;\(col + 1);\(row + 1)"
        return Array("\u{1b}[<\(at)M\u{1b}[<\(at)m".utf8)
    }
}
