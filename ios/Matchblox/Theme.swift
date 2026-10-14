import SwiftUI

/// The dark console tokens of DESIGN.md §2.1.
enum Theme {
    static let bg = Color(hex: 0x0F0D0A)
    static let fg = Color(hex: 0xEBE6DC)
    static let muted = Color(hex: 0xA69D8D)
    static let faint = Color(hex: 0x776F60)
    static let hair = Color(hex: 0x2E2921)
    static let accent = Color(hex: 0xC3A56A)
    static let neg = Color(hex: 0xB0563F)
}

private extension Color {
    init(hex: UInt32) {
        self.init(
            red: Double((hex >> 16) & 0xFF) / 255,
            green: Double((hex >> 8) & 0xFF) / 255,
            blue: Double(hex & 0xFF) / 255
        )
    }
}
