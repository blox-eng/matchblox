import Foundation

/// The console's text size, in whole points. A pinch on the terminal or `A−` / `A+` in the
/// Island screen changes it; the phone keeps it for every host.
public struct TextSize: Equatable, Sendable {
    public static let range = 9...24
    public static let standard = TextSize(13)
    /// The UserDefaults key of the kept size.
    public static let storeKey = "console.text-size"

    public let points: Int

    /// A size out of `range` comes back to its nearest end.
    public init(_ points: Int) {
        self.points = min(max(points, Self.range.lowerBound), Self.range.upperBound)
    }

    public var smaller: TextSize { TextSize(points - 1) }
    public var bigger: TextSize { TextSize(points + 1) }
    public var isSmallest: Bool { points == Self.range.lowerBound }
    public var isBiggest: Bool { points == Self.range.upperBound }

    /// The size a pinch that started at this size gives at `scale`: one step for each whole point.
    public func pinched(_ scale: CGFloat) -> TextSize {
        guard scale.isFinite, scale > 0 else { return self }
        return TextSize(Int((CGFloat(points) * scale).rounded()))
    }

    public static func load(from store: UserDefaults) -> TextSize {
        guard let points = store.object(forKey: storeKey) as? Int else { return standard }
        return TextSize(points)
    }

    public func save(to store: UserDefaults) {
        store.set(points, forKey: Self.storeKey)
    }
}
