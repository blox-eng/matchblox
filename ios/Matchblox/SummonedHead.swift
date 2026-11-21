import MatchbloxKit
import UIKit

/// The match head that a rest, then a move, calls up over the terminal. It sits under the
/// finger, rolls as the belt's head does, waits `HeadSummon.wait` after the last touch or the end
/// of its coast, and goes. Only its circle is the head; a touch anywhere else sends it away.
@MainActor
final class SummonedHead: UIView {
    let driver = HeadDriver(radius: HeadSummon.size)
    /// The head's center for a finger at a point of the superview: clear of ↓, the notice and
    /// the keyboard.
    var place: (CGPoint) -> CGPoint = { $0 }

    private let ball = HeadLayer()
    private var lastActive: TimeInterval = 0
    private var timer: Timer?
    /// Counts each showing, so a fade that ends after the head came back does not hide it.
    private var showing = 0
    nonisolated(unsafe) private var keyboardObserver: NSObjectProtocol?

    init() {
        super.init(frame: .zero)
        isHidden = true
        // It comes only under a finger; VoiceOver keeps the belt's head.
        accessibilityElementsHidden = true
        layer.shadowColor = UIColor.black.cgColor
        layer.shadowOpacity = 0.5
        layer.shadowRadius = 6
        layer.shadowOffset = CGSize(width: 0, height: 2)
        layer.addSublayer(ball)
        driver.draw = { [weak self] turn, heat, struck in
            guard let ball = self?.ball else { return }
            CATransaction.begin()
            CATransaction.setDisableActions(true)
            ball.turnX = turn.x
            ball.turnY = turn.y
            ball.heat = heat
            ball.struck = struck
            CATransaction.commit()
        }
        driver.flash = { [weak self] in self?.ball.flash() }
        keyboardObserver = NotificationCenter.default.addObserver(
            forName: UIResponder.keyboardWillShowNotification, object: nil, queue: .main
        ) { [weak self] _ in MainActor.assumeIsolated { self?.go() } }
    }

    required init?(coder: NSCoder) { nil }

    deinit {
        if let keyboardObserver { NotificationCenter.default.removeObserver(keyboardObserver) }
    }

    var isShown: Bool { !isHidden }

    override func point(inside point: CGPoint, with event: UIEvent?) -> Bool {
        isShown && hypot(point.x - bounds.midX, point.y - bounds.midY) <= bounds.width / 2
    }

    override func didMoveToWindow() {
        super.didMoveToWindow()
        ball.contentsScale = window?.screen.scale ?? traitCollection.displayScale
        if window == nil { go() }
    }

    // MARK: A finger from the terminal

    /// A rest, then a move, at `point` of `view`: the head comes under the finger, or moves there.
    func grab(at point: CGPoint, in view: UIView, time: TimeInterval) {
        guard let superview else { return }
        frame = HeadSummon.frame(center: place(superview.convert(point, from: view)))
        ball.frame = bounds
        ball.setNeedsDisplay()
        if isHidden || alpha < 1 { appear() }
        driver.began(at: point, time: time, canClick: false)
        touched()
    }

    func drag(to point: CGPoint, time: TimeInterval) {
        driver.moved(to: point, time: time)
        touched()
    }

    func release(time: TimeInterval) {
        driver.ended(at: time)
        touched()
    }

    // MARK: A finger on the waiting head

    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let finger = touches.first else { return }
        driver.began(at: finger.location(in: self), time: finger.timestamp)
        touched()
    }

    override func touchesMoved(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let finger = touches.first else { return }
        drag(to: finger.location(in: self), time: finger.timestamp)
    }

    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        release(time: touches.first?.timestamp ?? ProcessInfo.processInfo.systemUptime)
    }

    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        driver.cancelled()
        touched()
    }

    // MARK: Showing and going

    /// The head comes: a seat tap and the `Fade` in (DESIGN.md §5), at once with Reduce Motion.
    private func appear() {
        showing += 1
        isHidden = false
        driver.seat()
        layer.removeAllAnimations()
        if UIAccessibility.isReduceMotionEnabled {
            alpha = 1
        } else {
            alpha = 0
            UIView.animate(withDuration: 0.14) { self.alpha = 1 }
        }
        timer?.invalidate()
        timer = Timer.scheduledTimer(withTimeInterval: 0.25, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.check() }
        }
    }

    /// The head goes: after its wait, on a touch elsewhere, or when the keyboard rises.
    func go() {
        guard isShown else { return }
        timer?.invalidate()
        timer = nil
        driver.leave()
        let gone = showing
        if UIAccessibility.isReduceMotionEnabled {
            isHidden = true
            return
        }
        UIView.animate(withDuration: 0.14, animations: { self.alpha = 0 }, completion: { [weak self] _ in
            guard let self, self.showing == gone else { return }
            self.isHidden = true
        })
    }

    private func touched() {
        lastActive = ProcessInfo.processInfo.systemUptime
    }

    /// The wait counts from the last touch, or from the end of a flick's coast.
    private func check() {
        if driver.isTouched || driver.isCoasting { return touched() }
        if HeadSummon.isGone(lastTouch: lastActive, now: ProcessInfo.processInfo.systemUptime) { go() }
    }
}
