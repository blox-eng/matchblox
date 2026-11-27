import MatchbloxKit
import UIKit

/// The dock head's edge: the turn, its coast after a flick, and the cooling. The view hit-tests
/// the touch, says what each notch does, and draws the layer.
@MainActor
final class HeadDriver: NSObject {
    /// One notch round the head at `time`, from the finger or from the coast.
    var onNotch: (_ dir: HeadScroll.Dir, _ time: TimeInterval) -> Void = { _, _ in }
    /// Draw the head: its turn in radians, its heat, and whether it is alight.
    var draw: (_ turn: CGPoint, _ heat: CGFloat, _ struck: Bool) -> Void = { _, _, _ in }
    let feel = HeadFeel()

    /// Nearer the centre than this share of the radius, the finger's angle is not steady.
    private static let steadyShare: CGFloat = 0.15
    /// The share of a coast's speed left after one 60 Hz frame.
    private static let coastKeeps: CGFloat = 0.93
    /// A lift turning slower than this (radians a second) does not coast, and a coast stops below it.
    private static let coastFrom: CGFloat = MatchHead.notch * 4
    /// A finger that rested this long before the lift flicked nothing.
    private static let flickWithin: TimeInterval = 0.05

    private let radius: CGFloat
    private var head = MatchHead()
    private var centre = CGPoint.zero
    /// The finger's angle round the centre, once it is steady.
    private var angle: CGFloat?
    /// When the finger last moved; nil once it lifted.
    private var lastMove: TimeInterval?
    /// The finger's turn, radians a second (positive: clockwise).
    private var speed: CGFloat = 0
    /// The coast's turn, radians a second (positive: clockwise); nil when the head does not coast.
    private var coast: CGFloat?
    /// The coast's turn since its last notch.
    private var carried: CGFloat = 0
    private var link: CADisplayLink?
    private var lastFrame: CFTimeInterval?
    private var turn = CGPoint.zero

    var coasting: Bool { coast != nil }

    init(radius: CGFloat) {
        self.radius = radius
        super.init()
    }

    func began(at point: CGPoint, centre: CGPoint, time: TimeInterval) {
        stopCoast()
        feel.prepare()
        self.centre = centre
        head.lift()
        angle = nil
        lastMove = nil
        speed = 0
        moved(to: point, time: time)
    }

    func moved(to point: CGPoint, time: TimeInterval) {
        let since = lastMove.map { time - $0 }
        lastMove = time
        let d = CGPoint(x: point.x - centre.x, y: point.y - centre.y)
        guard hypot(d.x, d.y) >= radius * Self.steadyShare else { return }
        let to = atan2(d.y, d.x)
        let delta = angle.map { (to - $0).remainder(dividingBy: 2 * .pi) } ?? 0
        angle = to
        if let since, since > 0 { speed = 0.4 * speed + 0.6 * delta / CGFloat(since) }
        let rolled = head.turn(angle: to, at: time)
        for _ in 0..<abs(rolled.notches) {
            onNotch(rolled.notches < 0 ? .up : .down, time)
        }
        if rolled.strike { feel.strike() }
        if !UIAccessibility.isReduceMotionEnabled { turn.x += delta }
        redraw()
        startLink()
    }

    /// The finger left the head at `time`, or the system took the touch (nil): a flick coasts on.
    func ended(at time: TimeInterval?) {
        head.lift()
        angle = nil
        let flicked = time.flatMap { time in lastMove.map { time - $0 < Self.flickWithin } } ?? false
        lastMove = nil
        if flicked, abs(speed) >= Self.coastFrom {
            coast = speed
            carried = 0
        }
        speed = 0
        startLink()
    }

    /// A touch, or an end of the output, stops the coast.
    func stopCoast() {
        coast = nil
        carried = 0
    }

    /// A notch that moved something taps, unless the last tap was too near to feel apart.
    func tick(at time: TimeInterval) {
        if head.tap(at: time) { feel.notch(heat: head.heat) }
    }

    /// The head's view left the screen: no touch, no coast, no frames.
    func leave() {
        head.lift()
        angle = nil
        lastMove = nil
        stopCoast()
        stopLink()
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

    /// Each frame a coast turns on and slows; with no roll the head cools and goes out.
    @objc private func stepped(_ link: CADisplayLink) {
        let frame = lastFrame.map { link.timestamp - $0 } ?? link.duration
        lastFrame = link.timestamp
        if let coast { coasted(coast, frame: frame, at: link.timestamp) }
        if lastMove.map({ link.timestamp - $0 > frame * 2 }) ?? true {
            head.cool(frame: frame)
            redraw()
        }
        if lastMove == nil, coast == nil, head.heat < 0.01 { stopLink() }
    }

    /// One frame of a coast: the notches it turned, then the speed it keeps. A notch that bumps
    /// an end stops it.
    private func coasted(_ speed: CGFloat, frame: CFTimeInterval, at time: TimeInterval) {
        let step = speed * CGFloat(frame)
        carried += step
        if !UIAccessibility.isReduceMotionEnabled { turn.x += step }
        let dir: HeadScroll.Dir = speed < 0 ? .up : .down
        while coast != nil, abs(carried) >= MatchHead.notch {
            carried -= speed < 0 ? -MatchHead.notch : MatchHead.notch
            onNotch(dir, time)
        }
        guard coast != nil else { return }
        let left = speed * pow(Self.coastKeeps, CGFloat(frame * 60))
        coast = abs(left) < Self.coastFrom ? nil : left
    }
}
