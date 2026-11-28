import MatchbloxKit
import SwiftTerm
import UIKit

/// The console's terminal, with three ways back to matchblox: a swipe in from the left edge,
/// a tap on "◂ matchblox" in the tmux status line, and ⌘M or ctrl+M. Each one sends `HomeKey`.
/// While the program reads the mouse, a vertical swipe sends wheel steps (`WheelScroll`), each step
/// ticks once. In a chat, a long-press selects text to copy. A tap sends a click: the
/// console opens the row's chat. While the console shows Sessions or Machine, a long-press sends a
/// right click (`BoardTouch`). The match head over the belt scrolls through `head(lines:)`. A sideways
/// swipe turns no page: the list and the chat have none. A pinch makes the text bigger or smaller,
/// and a tap on a link asks what to do with it.
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
    /// Called when the terminal goes up from its live bottom (true) or comes back (false).
    var onScrolledBack: @MainActor (Bool) -> Void = { _ in }
    /// Called for each key typed or deleted: the belt head's coast stops, so it cannot scroll
    /// the pane back into copy mode under the new text.
    var onTyped: @MainActor () -> Void = {}
    /// Called with the lines a finger's swipe, or its coast, moved the view (positive: older).
    var onPanned: @MainActor (Int) -> Void = { _ in }
    /// Called after a Return from the system keyboard has reached the program.
    var onReturnSent: @MainActor () -> Void = {}
    /// Called for each key typed into the terminal: the console's own `c` may ask for a copy.
    var onAsksCopy: @MainActor () -> Void = {}
    /// A tap on a link: the link, and the place on screen it was tapped.
    var onLink: @MainActor (String, CGRect) -> Void = { _, _ in }
    /// Called as the selection changes: the URL it holds, whole, or nil.
    var onSelectedLink: @MainActor (String?) -> Void = { _ in }
    /// The console shows Sessions or Machine (`BoardFollow`): a long-press opens the row's menu.
    /// Otherwise the terminal's own long-press acts.
    var showsConsole = false
    /// False on the demo: a tap only clicks, and the keyboard comes from the belt's key, so it does
    /// not cover the question the demo shows.
    var tapRaisesKeyboard = true

    /// SwiftTerm does not declare this trait, so UIKit would keep inline predictions on.
    @objc var inlinePredictionType: UITextInlinePredictionType = .no

    private let haptic = UIImpactFeedbackGenerator(style: .light)
    /// The swipe's tick: one for each step it moves, never closer than `MatchHead.tapGap`.
    private let tick = UISelectionFeedbackGenerator()
    private var lastTick: TimeInterval = 0
    private var edge: UIScreenEdgePanGestureRecognizer?
    private var labelTap: UITapGestureRecognizer?
    private var clickTap: UITapGestureRecognizer?
    /// Whether the keyboard was this view's when the touch began. SwiftTerm spends a tap on an
    /// unfocused terminal only on the keyboard and never sends it to the program.
    private var focusedAtTouch = false
    private var wheelPan: UIPanGestureRecognizer?
    private var wheel = WheelScroll(cellHeight: 0)
    private var coast: Coast?
    private var hold: UILongPressGestureRecognizer?
    /// The word a hold selected, while its press drags the selection on.
    private var heldWord: (start: Position, end: Position)?
    /// SwiftTerm's long-press is selecting: the terminal takes no keys meanwhile.
    fileprivate var selecting = false
    private var selectionHandles: [SelectionHandle] = []
    private var selectionLens: UIImageView?
    private var selectionDragOrigin: CGPoint?
    private let selectGate = SelectGate()
    private lazy var editMenu = UIEditMenuInteraction(delegate: selectGate)
    private let holdGate = HoldGate()
    /// The size the text has now; a pinch changes it, in whole points.
    private(set) var textSize: TextSize
    private var pinchStart: TextSize?
    /// The wheel steps a swipe sent up in a tmux pane, less the steps down.
    private var scrollBack = ScrollBack()
    /// The swipe began in a tmux pane: its steps go into `scrollBack`.
    private var countsBack = false
    private var scrolledBack = false
    nonisolated(unsafe) private var sizeObserver: NSObjectProtocol?

    static func font(_ size: TextSize) -> UIFont {
        UIFont.monospacedSystemFont(ofSize: CGFloat(size.points), weight: .regular)
    }

    init(textSize: TextSize) {
        self.textSize = textSize
        super.init(frame: .zero, font: Self.font(textSize))

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

        // The first tap on an unfocused console is a click too: a session's row opens at once.
        let click = UITapGestureRecognizer(target: self, action: #selector(clicked))
        click.cancelsTouchesInView = false
        // SwiftTerm's single tap decides up to 0.3 s after the touch, for a double tap. It waits
        // for the click too, so a tap the click answers is not answered a second time, whatever
        // touch comes in that window.
        for other in gestureRecognizers ?? [] {
            if let tap = other as? UITapGestureRecognizer, tap.numberOfTapsRequired == 1 {
                tap.require(toFail: click)
            }
        }
        addGestureRecognizer(click)
        clickTap = click

        // SwiftTerm's mouse pan sends a drag, which tmux and the console do not scroll on. A
        // vertical swipe goes to this pan first; the scroll view's own pan waits for it too.
        let wheelPan = UIPanGestureRecognizer(target: self, action: #selector(wheelPanned))
        wheelPan.maximumNumberOfTouches = 1
        panGestureRecognizer.require(toFail: wheelPan)
        addGestureRecognizer(wheelPan)
        self.wheelPan = wheelPan

        // SwiftTerm's long-press (its copy menu, at 0.7 s) waits for this one, which a program
        // that reads no clicks fails at once.
        let hold = UILongPressGestureRecognizer(target: self, action: #selector(held))
        hold.minimumPressDuration = BoardTouch.longPress
        hold.cancelsTouchesInView = false
        // The view's gestureRecognizerShouldBegin did not stop this hold in a chat (the demo's UI
        // test, #485): its delegate asks instead, and a hold it refuses fails at once.
        hold.delegate = holdGate
        holdGate.view = self
        for other in gestureRecognizers ?? [] where other is UILongPressGestureRecognizer {
            other.require(toFail: hold)
        }
        addGestureRecognizer(hold)
        self.hold = hold
        // SwiftTerm's long-press only shows its menu, and the drag after it went to tmux as a
        // mouse drag (#492): the same press selects the word and drags the selection on.
        // Its start gave the terminal the keys, which moved the belt or raised the keyboard: each
        // resize made SwiftTerm drop the selection (processSizeChange). The hold takes no keys,
        // and the menu is the app's own, which needs none.
        selectGate.view = self
        for other in gestureRecognizers ?? [] where other is UILongPressGestureRecognizer && other !== hold {
            other.delegate = selectGate
            other.addTarget(self, action: #selector(selectHeld))
        }
        addInteraction(editMenu)
        selectionHandleColor = .clear
        for isStart in [true, false] {
            let handle = SelectionHandle(isStart: isStart)
            let pan = UIPanGestureRecognizer(target: self, action: #selector(resizeSelection))
            handle.addGestureRecognizer(pan)
            for other in gestureRecognizers ?? [] where other is UIPanGestureRecognizer || other is UILongPressGestureRecognizer {
                other.require(toFail: pan)
            }
            addSubview(handle)
            selectionHandles.append(handle)
        }
        #if DEBUG
        for other in gestureRecognizers ?? [] where other is UILongPressGestureRecognizer && other !== hold {
            other.addTarget(self, action: #selector(terminalHeld))
        }
        #endif

        // Two fingers: the one-finger pans, the taps and the long-presses fail at once.
        addGestureRecognizer(UIPinchGestureRecognizer(target: self, action: #selector(pinched)))

        // A change in the Island screen reaches a console that is open.
        sizeObserver = NotificationCenter.default.addObserver(forName: UserDefaults.didChangeNotification, object: nil, queue: .main) {
            [weak self] _ in MainActor.assumeIsolated { self?.show(TextSize.load(from: .standard)) }
        }
        #if DEBUG
        showNote()
        #endif
    }

    required init?(coder: NSCoder) { nil }

    /// The hold opens the row's menu only on the list of Sessions or Machine, with clicks on and
    /// off the label; elsewhere SwiftTerm's own long-press, which waits for it, selects.
    fileprivate func holdOpensMenu(at point: CGPoint) -> Bool {
        showsConsole && sendsClicks && !inTmuxPane && !tapsLabel(at: point)
    }

    #if DEBUG
    /// The demo's UI test reads the clicks the terminal sent and the row of the last left press
    /// (`countClicks`), how often SwiftTerm's own long-press began, how many wheel steps a finger
    /// sent (`pan=`), and the height of a row: the rows start at the frame's top, and the part of
    /// a row left at its foot is no row.
    var clickNote = "press=0 release=0 right=0 row=-1" { didSet { showNote() } }
    private var terminalHolds = 0
    private var panSteps = 0

    @objc private func terminalHeld(_ press: UILongPressGestureRecognizer) {
        guard press.state == .began else { return }
        terminalHolds += 1
        showNote()
    }

    private func showNote() {
        accessibilityValue = "\(clickNote) hold=\(terminalHolds) pan=\(panSteps) cell=\(cellSize.height)"
    }
    #endif

    deinit {
        if let sizeObserver { NotificationCenter.default.removeObserver(sizeObserver) }
    }

    /// SwiftTerm adds its mouse and selection pans only once it needs them: they wait for the
    /// edge swipe and the wheel pan too, or a swipe with the tmux mouse on would drag in the pane.
    override func addGestureRecognizer(_ gesture: UIGestureRecognizer) {
        super.addGestureRecognizer(gesture)
        if gesture is UIPanGestureRecognizer || gesture is UILongPressGestureRecognizer {
            for handle in selectionHandles {
                for pan in handle.gestureRecognizers ?? [] { gesture.require(toFail: pan) }
            }
        }
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
        focusedAtTouch = isFirstResponder
        super.touchesBegan(touches, with: event)
    }

    /// A lift ends any hold: the terminal may take the keys again.
    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        selecting = false
        super.touchesEnded(touches, with: event)
    }

    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        selecting = false
        super.touchesCancelled(touches, with: event)
    }

    override func becomeFirstResponder() -> Bool {
        if selecting, !isFirstResponder { return false }
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
        let command = UIKeyCommand(title: "Back to matchblox", action: #selector(goHome), input: "m", modifierFlags: .command)
        // SwiftTerm's pressesBegan would send ctrl+M as a CR. The app sees the modifier, so the
        // Return key, which has no ctrl, still goes to the terminal.
        let control = UIKeyCommand(title: "Back to matchblox", action: #selector(goHome), input: "m", modifierFlags: .control)
        let home = [command, control]
        home.forEach { $0.wantsPriorityOverSystemBehavior = true }
        return (super.keyCommands ?? []) + home
    }

    override func gestureRecognizerShouldBegin(_ gesture: UIGestureRecognizer) -> Bool {
        // Two fingers pinch: the scroll view's pan and SwiftTerm's pans do not take them.
        if gesture is UIPanGestureRecognizer, gesture.numberOfTouches > 1 {
            return false
        }
        if gesture === labelTap {
            return tapsLabel(at: gesture.location(in: self))
        }
        if gesture === clickTap {
            // A tap ends a selection: SwiftTerm's own tap clears it, with no click.
            if selection.active { return false }
            // A tap on a link asks what to do with it, focused or not.
            if linkUnder(gesture.location(in: self)) != nil, !tapsLabel(at: gesture.location(in: self)) {
                return true
            }
            let first = !focusedAtTouch
            let clicks = sendsClicks && (first || !tapRaisesKeyboard)
            return (clicks || (first && tapRaisesKeyboard)) && !tapsLabel(at: gesture.location(in: self))
        }
        // The click answers a first tap, and takes the keyboard on it; SwiftTerm's single tap
        // waits for the click to fail, so it answers only the taps on a focused terminal. On the
        // demo, no tap takes the keyboard.
        if !tapRaisesKeyboard, let tap = gesture as? UITapGestureRecognizer, tap.numberOfTapsRequired == 1 {
            return false
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
        case .ended where BoardTouch.sideways(fromEdge: true, dx: edge.translation(in: self).x, velocity: edge.velocity(in: self).x) != nil:
            goHome()
        default:
            break
        }
    }

    @objc private func wheelPanned(_ pan: UIPanGestureRecognizer) {
        switch pan.state {
        case .began, .changed:
            if pan.state == .began {
                tick.prepare()
                stopCoast()
                countsBack = inTmuxPane
                wheel = WheelScroll(cellHeight: cellSize.height, linesPerStep: countsBack ? WheelScroll.tmuxLines : 1)
            }
            let dy = pan.translation(in: self).y
            pan.setTranslation(.zero, in: self)
            panWheel(wheel.move(dy), at: pan.location(in: self))
        case .ended:
            let link = CADisplayLink(target: self, selector: #selector(coasted))
            coast = Coast(link: link, start: CACurrentMediaTime(), velocity: pan.velocity(in: self).y, point: pan.location(in: self))
            link.add(to: .main, forMode: .common)
        default:
            break
        }
    }

    @objc private func held(_ hold: UILongPressGestureRecognizer) {
        guard hold.state == .began, let spot = cell(at: hold.location(in: self)) else { return }
        haptic.impactOccurred()
        send(BoardTouch.rightClick(col: spot.col, row: spot.row))
    }

    /// SwiftTerm's long-press began: the word under the finger is selected. A drag of the same
    /// press moves the far end, the word's other end staying put, and the menu, Copy with it,
    /// shows when the finger lifts.
    @objc private func selectHeld(_ press: UILongPressGestureRecognizer) {
        switch press.state {
        case .began:
            if let spot = cell(at: press.location(in: self)) {
                let buffer = getTerminal().buffer
                selection.selectWordOrExpression(at: Position(col: spot.col, row: spot.row + buffer.yDisp), in: buffer)
            }
            heldWord = selection.active ? (selection.start, selection.end) : nil
            if selection.active { showSelectionLens(at: press.location(in: self)) }
        case .changed:
            guard let word = heldWord, let spot = cell(at: press.location(in: self)) else { return }
            let at = Position(col: spot.col, row: spot.row + getTerminal().buffer.yDisp)
            selection.pivot = Position.compare(at, word.start) == .before ? word.end : word.start
            selection.pivotExtend(row: spot.row, col: spot.col)
            showSelectionLens(at: press.location(in: self))
        case .ended:
            heldWord = nil
            selecting = false
            hideSelectionLens()
            guard selection.active else { return }
            UIMenuController.shared.hideMenu()
            editMenu.presentEditMenu(with: UIEditMenuConfiguration(identifier: nil, sourcePoint: press.location(in: self)))
        default:
            heldWord = nil
            selecting = false
            hideSelectionLens()
        }
    }

    @objc private func resizeSelection(_ pan: UIPanGestureRecognizer) {
        guard let handle = pan.view as? SelectionHandle else { return }
        let point = pan.location(in: self)
        switch pan.state {
        case .began:
            editMenu.dismissMenu()
            selecting = true
            let endpoint = handle.isStart ? selection.start : selection.end
            selection.pivot = handle.isStart ? selection.end : selection.start
            selection.selectionMode = .character
            selectionDragOrigin = CGPoint(x: (CGFloat(endpoint.col) + 0.5) * cellSize.width,
                                          y: (CGFloat(endpoint.row) + 0.5) * cellSize.height)
            showSelectionLens(at: selectionDragOrigin ?? point)
        case .changed:
            guard let origin = selectionDragOrigin else { return }
            let translation = pan.translation(in: self)
            let target = CGPoint(x: origin.x + translation.x, y: origin.y + translation.y)
            guard let spot = cell(at: target) else { return }
            selection.pivotExtend(row: spot.row, col: spot.col)
            showSelectionLens(at: target)
        case .ended, .cancelled, .failed:
            selecting = false
            selectionDragOrigin = nil
            hideSelectionLens()
            if pan.state == .ended, selection.active {
                editMenu.presentEditMenu(with: UIEditMenuConfiguration(identifier: nil, sourcePoint: point))
            }
        default: break
        }
    }

    private func updateSelectionHandles() {
        guard selectionHandles.count == 2 else { return }
        for handle in selectionHandles {
            handle.isHidden = !selection.active
            guard selection.active else { continue }
            let position = handle.isStart ? selection.start : selection.end
            let x = CGFloat(position.col + (handle.isStart ? 0 : 1)) * cellSize.width
            let y = CGFloat(position.row + (handle.isStart ? 0 : 1)) * cellSize.height
            handle.center = CGPoint(x: x, y: y)
            bringSubviewToFront(handle)
        }
    }

    private func showSelectionLens(at point: CGPoint) {
        let size = CGSize(width: 120, height: 54)
        let lens = selectionLens ?? UIImageView(frame: CGRect(origin: .zero, size: size))
        if selectionLens == nil {
            lens.layer.cornerRadius = size.height / 2
            lens.layer.borderWidth = 2
            lens.layer.borderColor = UIColor.secondaryLabel.cgColor
            lens.clipsToBounds = true
            lens.isUserInteractionEnabled = false
            addSubview(lens)
            selectionLens = lens
        }
        lens.isHidden = true
        let renderer = UIGraphicsImageRenderer(size: size)
        lens.image = renderer.image { context in
            context.cgContext.translateBy(x: size.width / 2 - point.x * 1.8, y: size.height / 2 - point.y * 1.8)
            context.cgContext.scaleBy(x: 1.8, y: 1.8)
            layer.render(in: context.cgContext)
        }
        let visible = CGRect(origin: contentOffset, size: bounds.size)
        lens.center = CGPoint(x: min(max(point.x, visible.minX + size.width / 2), visible.maxX - size.width / 2),
                              y: max(visible.minY + size.height / 2, point.y - 70))
        lens.isHidden = false
        bringSubviewToFront(lens)
    }

    private func hideSelectionLens() {
        selectionLens?.removeFromSuperview()
        selectionLens = nil
    }

    /// Each whole point of a pinch ticks once. The phone keeps the size when the fingers lift.
    @objc private func pinched(_ pinch: UIPinchGestureRecognizer) {
        switch pinch.state {
        case .began:
            tick.prepare()
            pinchStart = textSize
        case .changed:
            guard let pinchStart else { return }
            let before = textSize
            show(pinchStart.pinched(pinch.scale))
            if textSize != before { tick.selectionChanged() }
        case .ended, .cancelled:
            pinchStart = nil
            textSize.save(to: .standard)
        default:
            pinchStart = nil
        }
    }

    /// SwiftTerm counts the columns and rows again, and `sizeChanged` tells the host.
    private func show(_ size: TextSize) {
        guard size != textSize else { return }
        textSize = size
        font = Self.font(size)
        #if DEBUG
        showNote()
        #endif
    }

    @objc private func coasted(_ link: CADisplayLink) {
        guard var coast else { return }
        let elapsed = link.timestamp - coast.start
        let carried = WheelScroll.coast(velocity: coast.velocity, after: elapsed)
        panWheel(wheel.move(carried - coast.carried), at: coast.point)
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

    /// SwiftTerm sends a tap as a press and a release in these modes only. In X10 mode it sends
    /// nothing, and so does the click.
    private var sendsClicks: Bool {
        guard allowMouseReporting else { return false }
        switch getTerminal().mouseMode {
        case .vt200, .buttonEventTracking, .anyEvent: return true
        default: return false
        }
    }

    /// A finger's wheel steps, told to the match head as lines: the head's walk counts them.
    private func panWheel(_ steps: [WheelScroll.Step], at point: CGPoint) {
        guard readsMouse, !steps.isEmpty, cell(at: point) != nil else { return }
        sendWheel(steps, at: point)
        let older = steps.reduce(0) { $0 + ($1 == .up ? 1 : -1) }
        onPanned(older * wheel.linesPerStep)
        #if DEBUG
        panSteps += steps.count
        showNote()
        #endif
    }

    /// Each step at the touch's cell, encoded by SwiftTerm for the program's mouse protocol. The
    /// match head plays its own taps, so its steps do not tick.
    private func sendWheel(_ steps: [WheelScroll.Step], at point: CGPoint, ticks: Bool = true) {
        guard readsMouse, !steps.isEmpty, let spot = cell(at: point) else { return }
        let now = ProcessInfo.processInfo.systemUptime
        if ticks, now - lastTick >= MatchHead.tapGap {
            lastTick = now
            tick.selectionChanged()
        }
        if countsBack {
            scrollBack.wheeled(steps)
            scrollChanged()
        }
        let terminal = getTerminal()
        let pixelX = Int(point.x)
        let pixelY = Int(point.y - contentOffset.y)
        for step in steps {
            let flags = terminal.encodeButton(button: step.button, release: false, shift: false, meta: false, control: false)
            terminal.sendEvent(buttonFlags: flags, x: spot.col, y: spot.row, pixelX: pixelX, pixelY: pixelY)
        }
    }

    /// The visible cell under `point`, kept on the screen; nil before the terminal has a size.
    private func cell(at point: CGPoint) -> (col: Int, row: Int)? {
        let terminal = getTerminal()
        guard terminal.cols > 0, terminal.rows > 0 else { return nil }
        let size = cellSize
        let col = min(max(Int(point.x / size.width), 0), terminal.cols - 1)
        let row = min(max(Int(point.y / size.height) - terminal.buffer.yDisp, 0), terminal.rows - 1)
        return (col, row)
    }

    /// The press and release at the touch's cell, then the keyboard on a first tap.
    @objc private func clicked(_ tap: UITapGestureRecognizer) {
        guard tap.state == .ended else { return }
        let point = tap.location(in: self)
        if let link = linkUnder(point) {
            haptic.impactOccurred()
            onLink(link, CGRect(origin: point, size: .zero))
            return
        }
        sendClick(at: point)
        if tapRaisesKeyboard, !focusedAtTouch { _ = becomeFirstResponder() }
    }

    private func sendClick(at point: CGPoint) {
        guard sendsClicks, let spot = cell(at: point) else { return }
        let terminal = getTerminal()
        let pixelX = Int(point.x)
        let pixelY = Int(point.y - contentOffset.y)
        for release in [false, true] {
            let flags = terminal.encodeButton(button: 0, release: release, shift: false, meta: false, control: false)
            terminal.sendEvent(buttonFlags: flags, x: spot.col, y: spot.row, pixelX: pixelX, pixelY: pixelY)
        }
    }

    /// The link under `point`: a hyperlink the program marked (OSC 8), else an http or https URL
    /// in the row's text.
    /// SwiftTerm drops a selection at each output while mouse reporting is on (`feedPrepare`),
    /// and tmux and the agents write all the time (#492): a selection turns it off, and its end
    /// turns it back on.
    override func selectionChanged(source: Terminal) {
        allowMouseReporting = !selection.active
        super.selectionChanged(source: source)
        updateSelectionHandles()
        onSelectedLink(selection.active ? TerminalLink.url(inSelection: selection.getSelectedText()) : nil)
    }

    private func linkUnder(_ point: CGPoint) -> String? {
        guard let spot = cell(at: point) else { return nil }
        let terminal = getTerminal()
        if let marked = terminal.link(at: .screen(Position(col: spot.col, row: spot.row)), mode: .explicitOnly) {
            return marked
        }
        let rows = (0..<terminal.rows).map { row in (0..<terminal.cols).map { terminal.getCharacter(col: $0, row: row) ?? " " } }
        return TerminalLink.url(in: rows, row: spot.row, col: spot.col)
    }

    @objc private func goHome() {
        haptic.impactOccurred()
        forgetScrollBack()
        onHome()
    }

    // MARK: The match head

    /// Where a notch of the match head would go now (true: toward older lines).
    func headRoute(older: Bool) -> HeadRoute {
        HeadRoute.route(readsMouse: readsMouse, inTmuxPane: inTmuxPane, canScroll: canScroll,
                        position: scrollPosition, scrollBackSteps: scrollBack.steps, older: older)
    }

    /// The match head rolled `lines` lines (positive: older). Returns where they went.
    func head(lines: Int) -> HeadRoute {
        stopCoast()
        let route = headRoute(older: lines > 0)
        switch route {
        case .wheel:
            let events = WheelBurst.steps(lines: lines, linesPerStep: inTmuxPane ? WheelScroll.tmuxLines : 1)
            countsBack = inTmuxPane
            let step: WheelScroll.Step = events > 0 ? .up : .down
            sendWheel(Array(repeating: step, count: abs(events)), at: CGPoint(x: bounds.midX, y: bounds.midY), ticks: false)
        case .scrollback:
            if lines > 0 { scrollUp(lines: lines) } else { scrollDown(lines: -lines) }
            scrollChanged()
        case .end, .nothing:
            break
        }
        return route
    }

    /// VoiceOver pages with the match head: a screen of lines at once.
    func headPage(older: Bool) -> HeadRoute {
        head(lines: (older ? 1 : -1) * max(getTerminal().rows, 1))
    }

    // MARK: The live bottom

    /// A tmux pane the wheel scrolled up: it is in copy mode.
    var isUpInPane: Bool { scrollBack.isUp }

    /// Up from the live bottom: a tmux pane the wheel scrolled up, or SwiftTerm's own scrollback.
    private var isScrolledBack: Bool {
        scrollBack.isUp || (canScroll && scrollPosition < 1)
    }

    /// Tells `onScrolledBack` when the terminal leaves the live bottom or comes back to it.
    func scrollChanged() {
        let up = isScrolledBack
        guard up != scrolledBack else { return }
        scrolledBack = up
        onScrolledBack(up)
    }

    /// The person types, or leaves the pane: the wheel steps no longer say where the pane is.
    func forgetScrollBack() {
        guard scrollBack.isUp else { return }
        scrollBack.typed()
        scrollChanged()
    }

    /// The wheel steps that bring the pane back down; at the bottom tmux leaves copy mode. A
    /// plain `q` would leave it at once, but it types into the agent when the pane is not in copy
    /// mode; a step down past the bottom does nothing.
    func jumpToBottom() {
        stopCoast()
        sendWheel(scrollBack.jump(), at: CGPoint(x: bounds.midX, y: bounds.midY))
        if canScroll { scroll(toPosition: 1) }
        scrollChanged()
    }

    override func insertText(_ text: String) {
        onTyped()
        forgetScrollBack()
        onAsksCopy()
        super.insertText(text)
        if text.unicodeScalars.contains(where: { $0 == "\n" || $0 == "\r" }) { onReturnSent() }
    }

    override func deleteBackward() {
        onTyped()
        forgetScrollBack()
        super.deleteBackward()
    }

    override func pressesBegan(_ presses: Set<UIPress>, with event: UIPressesEvent?) {
        onTyped()
        forgetScrollBack()
        onAsksCopy()
        super.pressesBegan(presses, with: event)
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
    var inTmuxPane: Bool {
        let terminal = getTerminal()
        return [0, terminal.rows - 1].contains { labelColumns(row: $0) != nil }
    }

    private func labelColumns(row: Int) -> Range<Int>? {
        let terminal = getTerminal()
        let cells = (0..<terminal.cols).map { terminal.getCharacter(col: $0, row: row) ?? " " }
        return HomeKey.labelColumns(in: cells)
    }

    /// The screen's rows as text, from the top. A cell the program never drew holds NUL, and a
    /// space may be such a cell.
    var visibleLines: [String] {
        let terminal = getTerminal()
        return (0..<terminal.rows).map { row in
            let cells: [Character] = (0..<terminal.cols).map { col in
                let cell = terminal.getCharacter(col: col, row: row) ?? " "
                return cell == "\0" ? " " : cell
            }
            return String(cells)
        }
    }

    /// Pocket's own highlight of a chat question's option, over the first row that shows its
    /// text; nil for none. The chat's mark stays where it is until the tap types the number.
    func highlightRow(showing text: String?) {
        let probe = text.map { String($0.prefix(Self.highlightProbe)) }
        guard let probe, !probe.isEmpty, let row = visibleLines.firstIndex(where: { $0.contains(probe) }) else {
            rowHighlight.isHidden = true
            return
        }
        let cell = cellSize
        if rowHighlight.superview == nil {
            rowHighlight.backgroundColor = BeltLook.accent.withAlphaComponent(0.18)
            rowHighlight.isUserInteractionEnabled = false
            addSubview(rowHighlight)
        }
        rowHighlight.frame = CGRect(x: 0, y: CGFloat(row + getTerminal().buffer.yDisp) * cell.height,
                                    width: bounds.width, height: cell.height)
        rowHighlight.isHidden = false
    }

    /// A wrapped option shows its first words on its first row.
    private static let highlightProbe = 24
    private let rowHighlight = UIView()

    #if DEBUG
    var screenText: String { visibleLines.joined(separator: "\n") }
    #endif
}

/// The selecting hold's gate and menu: the hold marks that it selects before SwiftTerm's own
/// handler asks for the keys, and the menu offers Copy, Open link (a URL in the selection, whole)
/// and Paste.
@MainActor
private final class SelectGate: NSObject, UIGestureRecognizerDelegate, UIEditMenuInteractionDelegate {
    weak var view: ConsoleTerminalView?

    func gestureRecognizerShouldBegin(_ gesture: UIGestureRecognizer) -> Bool {
        view?.selecting = true
        return true
    }

    func editMenuInteraction(_ interaction: UIEditMenuInteraction, menuFor configuration: UIEditMenuConfiguration,
                             suggestedActions: [UIMenuElement]) -> UIMenu? {
        guard let view, view.selection.active else { return nil }
        let text = view.selection.getSelectedText()
        var items: [UIMenuElement] = [UIAction(title: "Copy") { [weak view] _ in
            UIPasteboard.general.string = text
            view?.selection.selectNone()
        }]
        if let link = TerminalLink.url(inSelection: text) {
            items.append(UIAction(title: "Open link") { [weak view] _ in
                guard let view else { return }
                view.onLink(link, CGRect(origin: configuration.sourcePoint, size: .zero))
            })
        }
        if UIPasteboard.general.hasStrings {
            items.append(UIAction(title: "Paste") { [weak view] _ in view?.paste(nil) })
        }
        return UIMenu(children: items)
    }
}

/// The delegate of the row menu's hold: it begins only where `holdOpensMenu` says.
private final class HoldGate: NSObject, UIGestureRecognizerDelegate {
    weak var view: ConsoleTerminalView?

    func gestureRecognizerShouldBegin(_ gesture: UIGestureRecognizer) -> Bool {
        guard let view else { return false }
        return view.holdOpensMenu(at: gesture.location(in: view))
    }
}

/// A generous touch target around each end of the highlighted text.
private final class SelectionHandle: UIView {
    let isStart: Bool

    init(isStart: Bool) {
        self.isStart = isStart
        super.init(frame: CGRect(x: 0, y: 0, width: 44, height: 44))
        backgroundColor = .clear
        isHidden = true
    }

    required init?(coder: NSCoder) { nil }

    override func draw(_ rect: CGRect) {
        UIColor.systemBlue.setFill()
        let y: CGFloat = isStart ? 12 : 24
        UIBezierPath(roundedRect: CGRect(x: 21, y: y, width: 2, height: 8), cornerRadius: 1).fill()
        UIBezierPath(ovalIn: CGRect(x: 17, y: isStart ? 4 : 32, width: 10, height: 10)).fill()
    }
}
