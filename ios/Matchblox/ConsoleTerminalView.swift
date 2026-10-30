import MatchbloxKit
import SwiftTerm
import UIKit

/// The console's terminal, with three ways back to matchblox: a swipe in from the left edge,
/// a tap on "◂ matchblox" in the tmux status line, and ⌘M. Each one sends `HomeKey`.
/// While the program reads the mouse, a vertical swipe sends wheel steps (`WheelScroll`).
final class ConsoleTerminalView: TerminalView {
    private struct Coast {
        let link: CADisplayLink
        let start: CFTimeInterval
        let velocity: CGFloat
        let point: CGPoint
        var carried: CGFloat = 0
    }

    var onHome: @MainActor () -> Void = {}
    /// Called when the terminal takes or gives up the keyboard.
    var onFocus: @MainActor (Bool) -> Void = { _ in }

    /// SwiftTerm does not declare this trait, so UIKit would keep inline predictions on.
    @objc var inlinePredictionType: UITextInlinePredictionType = .no

    private let haptic = UIImpactFeedbackGenerator(style: .light)
    private var edge: UIScreenEdgePanGestureRecognizer?
    private var labelTap: UITapGestureRecognizer?
    private var wheelPan: UIPanGestureRecognizer?
    private var wheel = WheelScroll(cellHeight: 0)
    private var coast: Coast?

    init(font: UIFont) {
        super.init(frame: .zero, font: font)

        let edge = UIScreenEdgePanGestureRecognizer(target: self, action: #selector(edgeSwiped))
        edge.edges = .left
        for other in gestureRecognizers ?? [] where other is UIPanGestureRecognizer {
            other.require(toFail: edge)
        }
        addGestureRecognizer(edge)
        self.edge = edge

        // SwiftTerm's own taps (keyboard, selection) wait until this one sees that the tap
        // is not on the label.
        let tap = UITapGestureRecognizer(target: self, action: #selector(goHome))
        for other in gestureRecognizers ?? [] where other is UITapGestureRecognizer {
            other.require(toFail: tap)
        }
        addGestureRecognizer(tap)
        labelTap = tap

        // SwiftTerm's mouse pan sends a drag, which tmux and the console do not scroll on. A
        // vertical swipe goes to this pan first; the scroll view's own pan waits for it too.
        let wheelPan = UIPanGestureRecognizer(target: self, action: #selector(wheelPanned))
        wheelPan.maximumNumberOfTouches = 1
        panGestureRecognizer.require(toFail: wheelPan)
        addGestureRecognizer(wheelPan)
        self.wheelPan = wheelPan
    }

    required init?(coder: NSCoder) { nil }

    /// SwiftTerm adds its mouse and selection pans only once it needs them: they wait for the
    /// edge swipe and the wheel pan too, or a swipe with the tmux mouse on would drag in the pane.
    override func addGestureRecognizer(_ gesture: UIGestureRecognizer) {
        super.addGestureRecognizer(gesture)
        guard let edge, gesture is UIPanGestureRecognizer, gesture !== edge else { return }
        gesture.require(toFail: edge)
        if let wheelPan, gesture !== wheelPan {
            gesture.require(toFail: wheelPan)
        }
    }

    override func willMove(toWindow newWindow: UIWindow?) {
        super.willMove(toWindow: newWindow)
        if newWindow == nil { stopCoast() }
    }

    /// A touch stops a flick, as in a scroll view.
    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        stopCoast()
        super.touchesBegan(touches, with: event)
    }

    override func becomeFirstResponder() -> Bool {
        let became = super.becomeFirstResponder()
        if became { onFocus(true) }
        return became
    }

    override func resignFirstResponder() -> Bool {
        let resigned = super.resignFirstResponder()
        if resigned { onFocus(false) }
        return resigned
    }

    override var keyCommands: [UIKeyCommand]? {
        let home = UIKeyCommand(title: "Back to matchblox", action: #selector(goHome), input: "m", modifierFlags: .command)
        home.wantsPriorityOverSystemBehavior = true
        return (super.keyCommands ?? []) + [home]
    }

    override func gestureRecognizerShouldBegin(_ gesture: UIGestureRecognizer) -> Bool {
        if gesture === labelTap {
            return tapsLabel(at: gesture.location(in: self))
        }
        if gesture === wheelPan, let pan = gesture as? UIPanGestureRecognizer {
            let velocity = pan.velocity(in: self)
            return readsMouse && !selection.active && abs(velocity.y) > abs(velocity.x)
        }
        return super.gestureRecognizerShouldBegin(gesture)
    }

    @objc private func edgeSwiped(_ edge: UIScreenEdgePanGestureRecognizer) {
        switch edge.state {
        case .began:
            haptic.prepare()
        case .ended where edge.translation(in: self).x > 60 || edge.velocity(in: self).x > 500:
            goHome()
        default:
            break
        }
    }

    @objc private func wheelPanned(_ pan: UIPanGestureRecognizer) {
        switch pan.state {
        case .began, .changed:
            if pan.state == .began {
                stopCoast()
                wheel = WheelScroll(cellHeight: cellSize.height, linesPerStep: inTmuxPane ? WheelScroll.tmuxLines : 1)
            }
            let dy = pan.translation(in: self).y
            pan.setTranslation(.zero, in: self)
            sendWheel(wheel.move(dy), at: pan.location(in: self))
        case .ended:
            let link = CADisplayLink(target: self, selector: #selector(coasted))
            coast = Coast(link: link, start: CACurrentMediaTime(), velocity: pan.velocity(in: self).y, point: pan.location(in: self))
            link.add(to: .main, forMode: .common)
        default:
            break
        }
    }

    @objc private func coasted(_ link: CADisplayLink) {
        guard var coast else { return }
        let elapsed = link.timestamp - coast.start
        let carried = WheelScroll.coast(velocity: coast.velocity, after: elapsed)
        sendWheel(wheel.move(carried - coast.carried), at: coast.point)
        coast.carried = carried
        self.coast = coast
        if elapsed >= WheelScroll.coastTime { stopCoast() }
    }

    /// The display link holds the view until it stops.
    private func stopCoast() {
        coast?.link.invalidate()
        coast = nil
    }

    private var readsMouse: Bool {
        allowMouseReporting && getTerminal().mouseMode != .off
    }

    /// Each step at the touch's cell, encoded by SwiftTerm for the program's mouse protocol.
    private func sendWheel(_ steps: [WheelScroll.Step], at point: CGPoint) {
        let terminal = getTerminal()
        guard readsMouse, !steps.isEmpty, terminal.cols > 0, terminal.rows > 0 else { return }
        let cell = cellSize
        let col = min(max(Int(point.x / cell.width), 0), terminal.cols - 1)
        let row = min(max(Int(point.y / cell.height) - terminal.buffer.yDisp, 0), terminal.rows - 1)
        let pixelX = Int(point.x)
        let pixelY = Int(point.y - contentOffset.y)
        for step in steps {
            let flags = terminal.encodeButton(button: step.button, release: false, shift: false, meta: false, control: false)
            terminal.sendEvent(buttonFlags: flags, x: col, y: row, pixelX: pixelX, pixelY: pixelY)
        }
    }

    @objc private func goHome() {
        haptic.impactOccurred()
        onHome()
    }

    /// SwiftTerm's own cell size (`computeFontDimensions`), which it does not make public.
    private var cellSize: CGSize {
        CGSize(
            width: "W".size(withAttributes: [.font: font]).width,
            height: ceil((font.ascender - font.descender + font.leading) * lineSpacing)
        )
    }

    /// The status line is the first or the last row, and a finger may land a row or a column off.
    /// The content holds the whole buffer, so the visible row is the content row less `yDisp`.
    private func tapsLabel(at point: CGPoint) -> Bool {
        let terminal = getTerminal()
        let cell = cellSize
        let col = Int(point.x / cell.width)
        let row = Int(point.y / cell.height) - terminal.buffer.yDisp
        for status in [0, terminal.rows - 1] where abs(row - status) <= 1 {
            if let label = labelColumns(row: status), (label.lowerBound - 1...label.upperBound).contains(col) {
                return true
            }
        }
        return false
    }

    /// A jumped tmux pane shows the way back in its status line; the console does not.
    private var inTmuxPane: Bool {
        let terminal = getTerminal()
        return [0, terminal.rows - 1].contains { labelColumns(row: $0) != nil }
    }

    private func labelColumns(row: Int) -> Range<Int>? {
        let terminal = getTerminal()
        let cells = (0..<terminal.cols).map { terminal.getCharacter(col: $0, row: row) ?? " " }
        return HomeKey.labelColumns(in: cells)
    }
}
