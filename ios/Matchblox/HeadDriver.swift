import MatchbloxKit
import UIKit

/// One match head's touch, roll, coast and cooling, for the head on the belt and the head called
/// up over the terminal. The owner hit-tests the touch and draws the layer.
@MainActor
final class HeadDriver: NSObject {
    /// The head rolled `lines` lines (positive: older). The answer says where they went.
    var onLines: (Int) -> HeadRoute = { _ in .nothing }
    /// Draw the head: its turn in radians, its heat, and whether it is alight.
    var draw: (_ turn: CGPoint, _ heat: CGFloat, _ struck: Bool) -> Void = { _, _, _ in }
    /// A short message for the person: nothing to scroll here.
    var say: (String) -> Void = { _ in }
    /// A still tap: send Enter.
    var onClick: () -> Void = {}
    /// Show that Enter went.
    var flash: () -> Void = {}

    /// A thumb on the head: where it was last, when, and how fast it turns (rad/s).
    private struct Touch {
        var last: CGPoint
        var time: TimeInterval
        var spin: CGFloat = 0
        /// The head hit an end, or found nothing to scroll: said once a touch.
        var said = false
    }

    private static let flick: CGFloat = 150

    /// The head's radius: a flick moves its edge faster than `flick` points a second.
    private let radius: CGFloat
    private var head = MatchHead()
    private let feel = HeadFeel()
    private var touch: Touch?
    /// A flick carries on after the thumb lifts, as the terminal's swipe does: radians.
    private var coast: (start: TimeInterval, spin: CGFloat, from: CGFloat)?
    /// The head's centre, and the finger's angle round it.
    private var centre = CGPoint.zero
    private var angle: CGFloat = 0
    private var link: CADisplayLink?
    private var lastFrame: CFTimeInterval?
    private var turn = CGPoint.zero
    /// Whether the touch is a tap that sends Enter (`HeadClick`).
    private var click = HeadClick()
    /// When the head last rolled, by a thumb or a coast.
    private var lastRoll: TimeInterval?

    init(radius: CGFloat) {
        self.radius = radius
        super.init()
    }

    var isTouched: Bool { touch != nil }

    var isCoasting: Bool { coast != nil }

    /// A thumb came onto the head. `canClick` is false for a thumb that was already moving, as
    /// when a rest and a move call the head up: its lift is no tap and sends no Enter.
    func began(at point: CGPoint, centre: CGPoint, time: TimeInterval, canClick: Bool = true) {
        // A touch stops a flick, as in a scroll view, and says so: it sends nothing.
        let coasting = coast != nil
        coast = nil
        feel.prepare()
        if coasting { feel.stop() }
        click.began(at: time, coasting: coasting || !canClick, lastRoll: lastRoll)
        touch = Touch(last: point, time: time)
        self.centre = centre
        angle = bearing(of: point)
        head.lift()
        _ = head.turn(angle: angle, at: time)
    }

    func moved(to point: CGPoint, time: TimeInterval) {
        guard var t = touch else { return }
        let dt = max(time - t.time, 1.0 / 240)
        let d = CGPoint(x: point.x - t.last.x, y: point.y - t.last.y)
        click.moved(by: hypot(d.x, d.y))
        let to = bearing(of: point)
        t.spin = t.spin * 0.6 + (to - angle).remainder(dividingBy: 2 * .pi) / dt * 0.4
        t.last = point
        t.time = time
        touch = t
        roll(to: to, at: time)
    }

    func ended(at time: TimeInterval) {
        guard let t = touch else { return }
        touch = nil
        if click.isClick(endedAt: time) {
            feel.click()
            flash()
            onClick()
        } else if abs(t.spin) * radius > Self.flick {
            coast = (CACurrentMediaTime(), t.spin, angle)
        }
        startLink()
    }

    /// The system took the touch: no flick, and the head cools.
    func cancelled() {
        touch = nil
        startLink()
    }

    /// The head came under the thumb.
    func seat() {
        feel.seat()
    }

    /// The head's view left the screen: no touch, no flick, no frames.
    func leave() {
        touch = nil
        coast = nil
        stopLink()
    }

    /// A key or typed text ends a flick, so the coast cannot scroll the pane back up under it.
    func stop() {
        coast = nil
    }

    private func bearing(of point: CGPoint) -> CGFloat {
        atan2(point.y - centre.y, point.x - centre.x)
    }

    /// The head turned to `angle`: scroll, and draw it.
    private func roll(to angle: CGFloat, at time: TimeInterval) {
        let delta = (angle - self.angle).remainder(dividingBy: 2 * .pi)
        self.angle = angle
        let rolled = head.turn(angle: angle, at: time)
        if delta != 0 { lastRoll = time }
        var moved = false
        if rolled.lines != 0 {
            switch onLines(rolled.lines) {
            case .wheel, .scrollback:
                moved = true
            case .end:
                coast = nil
                if sayOnce(nil) { feel.end() }
            case .nothing:
                coast = nil
                sayOnce("nothing scrolls here · this program has no scrollback")
            }
        }
        if moved, head.tap(at: time) { feel.notch(heat: head.heat) }
        if rolled.strike { feel.strike() }
        if !UIAccessibility.isReduceMotionEnabled { turn.x += delta }
        redraw()
        startLink()
    }

    /// An end or an empty screen speaks once a touch; a flick's end says it once too. True the
    /// first time.
    @discardableResult
    private func sayOnce(_ message: String?) -> Bool {
        if var t = touch {
            guard !t.said else { return false }
            t.said = true
            touch = t
        }
        if let message { say(message) }
        return true
    }

    private func redraw() {
        draw(turn, head.heat, head.struck)
    }

    private func startLink() {
        guard link == nil else { return }
        let link = CADisplayLink(target: self, selector: #selector(stepped))
        link.add(to: .main, forMode: .common)
        self.link = link
        lastFrame = nil
    }

    private func stopLink() {
        link?.invalidate()
        link = nil
        lastFrame = nil
    }

    /// Each frame: a flick carries the head on; with no roll, the head cools and goes out.
    @objc private func stepped(_ link: CADisplayLink) {
        let frame = lastFrame.map { link.timestamp - $0 } ?? link.duration
        lastFrame = link.timestamp
        if let coast {
            let elapsed = CACurrentMediaTime() - coast.start
            roll(to: coast.from + WheelScroll.coast(velocity: coast.spin, after: elapsed), at: link.timestamp)
            if elapsed >= WheelScroll.coastTime { self.coast = nil }
            return
        }
        if touch.map({ link.timestamp - $0.time > frame * 2 }) ?? true {
            head.cool(frame: frame)
            redraw()
        }
        if touch == nil, head.heat < 0.01 { stopLink() }
    }
}
