import UIKit

/// The haptics of the pair screens (design/0009 §6.2, §6.5). They play also
/// with Reduce Motion.
enum Haptics {
    /// The scanner found a pair code.
    @MainActor static func lock() {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
    }

    /// The match head reaches the end of the striker.
    @MainActor static func strike() {
        UIImpactFeedbackGenerator(style: .rigid).impactOccurred()
    }

    /// The match stands and burns: the device is paired.
    @MainActor static func stood() {
        UINotificationFeedbackGenerator().notificationOccurred(.success)
    }

    /// The pairing failed.
    @MainActor static func failed() {
        UINotificationFeedbackGenerator().notificationOccurred(.error)
    }
}
