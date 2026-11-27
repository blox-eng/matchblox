import Foundation

/// Where a finger lands on the head: the middle is the stick, the edge is the wheel.
public enum HeadZone {
    public enum Zone: Equatable, Sendable {
        case middle
        case edge(Mark)
    }

    public enum Mark: Sendable {
        case top, left, right, bottom
    }

    /// The middle's share of the head's radius.
    public static let middleShare: CGFloat = 0.48

    /// `point` is from the head's centre, in UIKit points (y grows down).
    public static func at(point: CGPoint, radius: CGFloat) -> Zone {
        if hypot(point.x, point.y) < radius * middleShare { return .middle }
        if abs(point.x) > abs(point.y) { return .edge(point.x < 0 ? .left : .right) }
        return .edge(point.y < 0 ? .top : .bottom)
    }
}
