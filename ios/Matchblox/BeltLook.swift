import UIKit

/// The key belt's colours. The tokens come from `Theme`; the mockup's few colours that are no
/// token (the key bevel, the card ground under the grit, the grip, the light and dark lines,
/// the tray's grain, the `EDIT MINE` stamp and the editor's sheet) are named here and nowhere
/// else.
@MainActor
enum BeltLook {
    // Tokens.
    static let fg = UIColor(Theme.fg)
    static let muted = UIColor(Theme.muted)
    static let faint = UIColor(Theme.faint)
    static let hair = UIColor(Theme.hair)
    static let accent = UIColor(Theme.accent)
    /// The sleeve: the handle row and the belt's ground.
    static let sleeve = UIColor(Theme.markRight)
    /// The sleeve's top edge, over the handle row.
    static let sleeveEdge = UIColor(Theme.markLeft)
    static let striker = UIColor(Theme.markStriker)
    /// The key's 1 pt edge, `#3a3328`: the same value as the `markTop` token.
    static let keyEdge = UIColor(Theme.markTop)

    // Not tokens: the mockup's own.
    static let keyBevelTop = UIColor(hex: 0x2C261D)
    static let keyBevelBottom = UIColor(hex: 0x1D1913)
    /// The light line inside a key's top edge.
    static let keyTopLine = UIColor(white: 1, alpha: 0x14 / 255)
    /// The dark line under a key.
    static let keyDrop = UIColor.black
    /// The card the striker's grit is printed on.
    static let cardGround = UIColor(hex: 0x241E16)
    static let strikerTopLine = UIColor(white: 0, alpha: 0xAA / 255)
    static let strikerBottomLine = UIColor(white: 1, alpha: 0x10 / 255)
    static let grip = UIColor(hex: 0x5A5244)
    /// The tray floor's card grain: light lines down every 3 pt, dark lines across every 4 pt.
    static let grainLight = UIColor(white: 1, alpha: 0x05 / 255)
    static let grainDark = UIColor(white: 0, alpha: 0x18 / 255)
    /// The shadow the tray's walls throw in: deep under the top wall, light over the bottom.
    static let trayShadowTop = UIColor(white: 0, alpha: 0.85)
    static let trayShadowBottom = UIColor(white: 0, alpha: 0.6)
    /// `EDIT MINE`, stamped on the tray floor: its ink and its edge.
    static let stampInk = UIColor(hex: 0x8A7A5C)
    static let stampEdge = UIColor(hex: 0x3A3226)
    /// The editor's sheet.
    static let sheet = UIColor(hex: 0x17140F)
}

private extension UIColor {
    convenience init(hex: UInt32) {
        self.init(
            red: CGFloat((hex >> 16) & 0xFF) / 255,
            green: CGFloat((hex >> 8) & 0xFF) / 255,
            blue: CGFloat(hex & 0xFF) / 255,
            alpha: 1
        )
    }
}
