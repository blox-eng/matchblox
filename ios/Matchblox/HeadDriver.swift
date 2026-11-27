import MatchbloxKit
import UIKit

/// The dock head's turn and cooling. The view hit-tests the touch, says what each notch does,
/// and draws the layer.
@MainActor
final class HeadDriver: NSObject {
    /// One notch round the head at `time`.
    var onNotch: (_ dir: HeadWalk.Dir, _ time: TimeInterval) -> Void = { _, _ in }
    /// Draw the head: its turn in radians, its heat, and whether it is alight.
    var draw: (_ turn: CGPoint, _ heat: CGFloat, _ struck: Bool) -> Void = { _, _, _ in }
    let feel = HeadFeel()

    /// Nearer the centre than this share of the radius, the finger's angle is not steady.
    private static let steadyShare: CGFloat = 0.15

    private let radius: CGFloat
    private var head = MatchHead()
    private var centre = CGPoint.zero
    /// The finger's angle round the centre, once it is steady.
    private var angle: CGFloat?
    /// When the finger last moved; nil once it lifted.
    private var lastMove: TimeInterval?
    private var link: CADisplayLink?
    private var lastFrame: CFTimeInterval?
    private var turn = CGPoint.zero

    init(radius: CGFloat) {
        self.radius = radius
        super.init()
    }

    func began(at point: CGPoint, centre: CGPoint, time: TimeInterval) {
        feel.prepare()
        self.centre = centre
        head.lift()
        angle = nil
        moved(to: point, time: time)
    }

    func moved(to point: CGPoint, time: TimeInterval) {
        lastMove = time
        let d = CGPoint(x: point.x - centre.x, y: point.y - centre.y)
        guard hypot(d.x, d.y) >= radius * Self.steadyShare else { return }
        let to = atan2(d.y, d.x)
        let delta = angle.map { (to - $0).remainder(dividingBy: 2 * .pi) } ?? 0
        angle = to
        let rolled = head.turn(angle: to, at: time)
        for _ in 0..<abs(rolled.notches) {
            onNotch(rolled.notches < 0 ? .up : .down, time)
        }
        if rolled.strike { feel.strike() }
        if !UIAccessibility.isReduceMotionEnabled { turn.x += delta }
        redraw()
        startLink()
    }

    /// The finger left the head, or the system took the touch: the head cools.
    func ended() {
        head.lift()
        angle = nil
        lastMove = nil
        startLink()
    }

    /// The roll crossed out of the options into the output: the heat and the strike count from here.
    func cross() {
        head.cross()
    }

    /// A notch that moved something taps, unless the last tap was too near to feel apart.
    func tick(at time: TimeInterval) {
        if head.tap(at: time) { feel.notch(heat: head.heat) }
    }

    /// The head's view left the screen: no touch, no frames.
    func leave() {
        head.lift()
        angle = nil
        lastMove = nil
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

    /// Each frame with no roll, the head cools and goes out.
    @objc private func stepped(_ link: CADisplayLink) {
        let frame = lastFrame.map { link.timestamp - $0 } ?? link.duration
        lastFrame = link.timestamp
        if lastMove.map({ link.timestamp - $0 > frame * 2 }) ?? true {
            head.cool(frame: frame)
            redraw()
        }
        if lastMove == nil, head.heat < 0.01 { stopLink() }
    }
}
