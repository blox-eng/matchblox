import Foundation

/// The match head that a rest, then a move, calls up over the terminal: where it sits, and when
/// it goes.
public enum HeadSummon {
    /// The head's width: larger than a thumb's contact, so its rim shows around the thumb.
    public static let size: CGFloat = 56
    /// The head waits this long after the last touch or the end of its coast.
    public static let wait: TimeInterval = 3
    /// The room the head keeps from a control it must not cover.
    static let gap: CGFloat = 4

    /// The head's center for a finger at `finger`: under it, inside `bounds` (the safe area),
    /// above the keyboard, and clear of each frame in `avoid` (↓, a notice), so a tap meant for
    /// them never lands on the head.
    public static func center(finger: CGPoint, bounds: CGRect, keyboardTop: CGFloat? = nil, avoid: [CGRect] = []) -> CGPoint {
        let r = size / 2
        var area = bounds
        if let keyboardTop, keyboardTop < area.maxY { area.size.height = max(keyboardTop - area.minY, size) }
        func clamped(_ p: CGPoint) -> CGPoint {
            CGPoint(x: min(max(p.x, area.minX + r), area.maxX - r), y: min(max(p.y, area.minY + r), area.maxY - r))
        }
        var center = clamped(finger)
        for blocked in avoid where frame(center: center).intersects(blocked) {
            let above = CGPoint(x: center.x, y: blocked.minY - r - gap)
            center = above.y >= area.minY + r ? above : clamped(CGPoint(x: blocked.minX - r - gap, y: center.y))
        }
        return center
    }

    public static func frame(center: CGPoint) -> CGRect {
        CGRect(x: center.x - size / 2, y: center.y - size / 2, width: size, height: size)
    }

    public static func isGone(lastTouch: TimeInterval, now: TimeInterval) -> Bool {
        now - lastTouch >= wait
    }
}
