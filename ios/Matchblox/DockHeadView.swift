import MatchbloxKit
import UIKit

/// The one match head, over the dock's belt, on Joystick. Where the finger lands decides the
/// meaning. The middle is a thumbstick (`HeadStick`): a push toward a mark sends its key once and
/// repeats while held, a still tap is Enter, and a still hold fills a ring, then shows the
/// sessions (`SessionCard`). The edge is a wheel: a roll scrolls the output (`HeadScroll`), a flick
/// coasts on, and neither sends a key. Past the agent's first option, read from the screen
/// (`ScreenMenu`), a push `↑` reads up the output. The ring and the marks show while it is touched.
@MainActor
final class DockHeadView: UIView {
    static let side: CGFloat = 96
    /// The head stands this high above the belt; the rest of it lies over the keys' upper third,
    /// so a tap on a key's middle still reaches the key.
    static let row: CGFloat = 72

    private static let markSize = CGSize(width: 52, height: 30)
    private static let ringGap: CGFloat = 4
    private static let marksIn: TimeInterval = 0.12
    /// The ring and the marks are gone 0.9 s after the lift.
    private static let marksStay: TimeInterval = 0.75
    private static let marksOut: TimeInterval = 0.15
    /// The ball springs back on the lift with a small overshoot.
    private static let springBack: TimeInterval = 0.24
    private static let springBounce: CGFloat = 0.4
    /// How often a held push asks the stick for its repeat.
    private static let tickEvery: Duration = .milliseconds(15)
    /// The marks whose push repeats while held: the arrows and the cursor.
    private static let repeating: Set<String> = ["↑", "↓", "←", "→"]
    /// From this many lines a notch the card names the step at the top of the view.
    private static let cardPace = 3
    private static let cardStays: Duration = .milliseconds(600)
    private static let enter = KeyBar.bytes(for: "⏎") ?? [0x0d]
    /// The hold's ring starts to fill this long into a still press, and is full at `HeadClick.hold`.
    private static let ringFrom: TimeInterval = 0.15
    private static let ringFills: TimeInterval = HeadClick.hold - ringFrom
    private static let sessionsKey = "⇄"

    /// The agent's pane the console shows: the pane the app jumped into, or the pane of the
    /// session the console's chat shows; nil on the console's list. The sessions card marks it.
    var pane: String?
    /// Runs Pocket's matchblox commands where the console is.
    var command: HostCommand?
    /// The head moved the view `lines` lines (positive: older). The answer says where they went.
    var onLines: (Int) -> HeadRoute = { _ in .nothing }
    /// Where a notch of output would go now (true: toward older lines).
    var route: (Bool) -> HeadRoute = { _ in .nothing }
    /// VoiceOver's swipe with no menu: a screen of lines (true: older). The answer says where they went.
    var page: (Bool) -> HeadRoute = { _ in .nothing }
    var onSend: ([UInt8]) -> Void = { _ in }
    var onKeyboard: () -> Void = {}
    /// The handle row's line; nil gives the row back its rest.
    var say: (String?) -> Void = { _ in }
    /// The step card's text, or nil: the card goes.
    var onCard: (String?) -> Void = { _ in }
    /// The terminal's visible lines, from the top.
    var visibleLines: () -> [String] = { [] }
    /// The sessions card's text, or nil: the card goes.
    var onSessions: @MainActor (String?) -> Void = { _ in }
    /// The number of the session this console opened, if it opened one.
    var opened: @MainActor () -> Int? = { nil }
    /// Opens session `n` of this host, as a tap on it in the Live Activity does.
    var openSession: @MainActor (Int) -> Void = { _ in }

    private struct Touch {
        /// From the centre.
        let start: CGPoint
        let zone: HeadZone.Zone
        /// It stopped a coast or came right after a roll: its lift is never Enter, nor a hold.
        let quiet: Bool
    }

    private let ball = HeadLayer()
    private let rim = UIView()
    private let ring = CAShapeLayer()
    private let holdRing = CAShapeLayer()
    private var markLabels: [HeadZone.Mark: UILabel] = [:]
    private lazy var element: VoiceHead = {
        let element = VoiceHead(accessibilityContainer: self)
        element.accessibilityLabel = "Match head"
        element.accessibilityIdentifier = "match-head"
        element.accessibilityTraits = .adjustable
        element.accessibilityHint = "Swipe up or down for a page. Double-tap for Enter."
        element.swipe = { [weak self] older in self?.voiceSwipe(older: older) }
        element.activate = { [weak self] in self?.voiceTap() ?? false }
        element.focused = { [weak self] in self?.voiceFocused() }
        element.readValue = { [weak self] in self?.spokenValue }
        element.actions = { [weak self] in self?.voiceActions() ?? [] }
        return element
    }()
    private lazy var driver: HeadDriver = {
        let driver = HeadDriver(radius: Self.side / 2)
        driver.onNotch = { [weak self] dir, time in self?.notch(dir, at: time) }
        driver.draw = { [weak self] turn, heat, struck in self?.draw(turn: turn.x, heat: heat, struck: struck) }
        return driver
    }()
    private var scroll = HeadScroll()
    private var stick = HeadStick()
    private var marks = HeadMarks()
    private var touch: Touch?
    private var lastNotch: TimeInterval?
    /// An end bumps once a roll, its coast included.
    private var rollBumped = false
    /// What the head last told VoiceOver; its value until the next touch or focus.
    private var voiceLine: String?
    private var ticking: Task<Void, Never>?
    private var cardGoes: Task<Void, Never>?
    private var holding: Task<Void, Never>?
    /// The sessions the hold shows; nil until the list reads.
    private var sessions: SessionCard?
    /// The hold's list once it has answered (nil inside: it did not answer), kept until the ring fills.
    private var listed: HostReply??
    /// Counts each fade, so a fade out that ends after a new touch leaves the marks shown.
    private var fades = 0
    /// Home sets the colour and the marks; the head only reads them, and never saves one.
    nonisolated(unsafe) private var settingsObserver: NSObjectProtocol?
    #if DEBUG
    /// The last card the head showed: the UI test reads it, as the card stays only 0.6 s.
    private var lastCard: String?
    /// The last sessions card, which goes at the lift.
    private var lastSessions: String?
    #endif

    override init(frame: CGRect) {
        super.init(frame: frame)
        isMultipleTouchEnabled = false

        ball.contentsScale = max(traitCollection.displayScale, 2)
        ball.pigment = HeadPigment.load(from: .standard)
        layer.addSublayer(ball)

        rim.isUserInteractionEnabled = false
        rim.alpha = 0
        rim.isHidden = true
        ring.fillColor = nil
        ring.strokeColor = BeltLook.muted.cgColor
        ring.lineWidth = 1
        rim.layer.addSublayer(ring)
        holdRing.fillColor = nil
        holdRing.strokeColor = BeltLook.fg.cgColor
        holdRing.lineWidth = 3
        holdRing.lineCap = .round
        holdRing.isHidden = true
        layer.addSublayer(holdRing)
        for mark in [HeadZone.Mark.top, .left, .right, .bottom] {
            let label = UILabel()
            label.numberOfLines = 2
            label.textAlignment = .center
            label.font = UIFont.monospacedSystemFont(ofSize: 10, weight: .medium)
            label.textColor = BeltLook.fg
            label.backgroundColor = BeltLook.sleeve.withAlphaComponent(0.92)
            label.layer.cornerRadius = 6
            label.layer.masksToBounds = true
            label.accessibilityIdentifier = "head-mark-\(place(of: mark).rawValue)"
            rim.addSubview(label)
            markLabels[mark] = label
        }
        addSubview(rim)

        // A change on home reaches the head at once: home covers the console, which stays.
        settingsObserver = NotificationCenter.default.addObserver(forName: UserDefaults.didChangeNotification, object: nil, queue: .main) {
            [weak self] _ in MainActor.assumeIsolated { self?.settingsChanged() }
        }
    }

    required init?(coder: NSCoder) { nil }

    deinit {
        if let settingsObserver { NotificationCenter.default.removeObserver(settingsObserver) }
    }

    /// The marks need no reload here: each touch reads them afresh.
    private func settingsChanged() {
        ball.pigment = HeadPigment.load(from: .standard)
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        // Bounds and position, not the frame: the ball's transform moves it with the thumb.
        let ballFrame = bounds.insetBy(dx: -HeadLayer.margin, dy: -HeadLayer.margin)
        ball.bounds = CGRect(origin: .zero, size: ballFrame.size)
        ball.position = CGPoint(x: bounds.midX, y: bounds.midY)
        rim.frame = bounds
        let r = Self.side / 2 + Self.ringGap
        let c = CGPoint(x: bounds.midX, y: bounds.midY)
        ring.path = UIBezierPath(ovalIn: CGRect(x: c.x - r, y: c.y - r, width: 2 * r, height: 2 * r)).cgPath
        holdRing.path = UIBezierPath(arcCenter: c, radius: r, startAngle: -.pi / 2, endAngle: 1.5 * .pi, clockwise: true).cgPath
        let size = Self.markSize
        let reach = r + size.height / 2 + 2
        let side = r + size.width / 2 + 2
        let places: [HeadZone.Mark: CGPoint] = [
            .top: CGPoint(x: c.x, y: c.y - reach), .bottom: CGPoint(x: c.x, y: c.y + reach),
            .left: CGPoint(x: c.x - side, y: c.y), .right: CGPoint(x: c.x + side, y: c.y),
        ]
        for (mark, label) in markLabels {
            guard let at = places[mark] else { continue }
            label.frame = CGRect(x: at.x - size.width / 2, y: at.y - size.height / 2, width: size.width, height: size.height)
        }
        CATransaction.commit()
        element.accessibilityFrameInContainerSpace = bounds
    }

    /// Only the round head takes a touch; its corners belong to the belt under it.
    override func point(inside point: CGPoint, with event: UIEvent?) -> Bool {
        hypot(point.x - bounds.midX, point.y - bounds.midY) <= Self.side / 2
    }

    override func willMove(toWindow newWindow: UIWindow?) {
        super.willMove(toWindow: newWindow)
        guard newWindow == nil else { return }
        touch = nil
        stopTicks()
        endHold()
        driver.leave()
        moveBall(to: .zero)
    }

    // MARK: Touch

    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard touch == nil, let finger = touches.first else { return }
        let stoppedCoast = driver.coasting
        driver.stopCoast()
        let from = offset(of: finger)
        let zone = HeadZone.at(point: from, radius: Self.side / 2)
        let quiet = stoppedCoast || rolledJustBefore(finger.timestamp)
        touch = Touch(start: from, zone: zone, quiet: quiet)
        voiceLine = nil
        marks = HeadMarks.load(from: .standard)
        showMarks()
        ball.pressed = true
        switch zone {
        case .edge:
            scroll.lift()
            rollBumped = false
            driver.began(at: finger.location(in: self), centre: CGPoint(x: bounds.midX, y: bounds.midY), time: finger.timestamp)
        case .middle:
            stick.began(at: finger.timestamp)
            startTicks()
            if !quiet { startHold() }
        }
    }

    override func touchesMoved(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let t = touch, let finger = touches.first else { return }
        switch t.zone {
        case .edge:
            driver.moved(to: finger.location(in: self), time: finger.timestamp)
        case .middle:
            let from = offset(of: finger)
            let wasHeld = stick.held
            let sent = stick.moved(to: CGPoint(x: from.x - t.start.x, y: from.y - t.start.y), at: finger.timestamp)
            moveBall(to: stick.offset)
            if !stick.still, !stick.held, holding != nil { endHold() }
            if stick.held, !wasHeld { holdBegan() }
            if let sent { pushed(sent, again: false) }
        }
    }

    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let t = touch, let finger = touches.first else { return }
        let goTo = sessions?.lift
        switch t.zone {
        case .edge:
            lifted()
            driver.ended(at: finger.timestamp)
        case .middle:
            let lift = stick.lifted(at: finger.timestamp)
            lifted()
            guard !t.quiet else { return }
            switch lift {
            case .enter:
                tapped()
            case .hold:
                let id = marks[.hold]
                if id != Self.sessionsKey {
                    press(id)
                } else if let goTo {
                    openSession(goTo)
                }
            case .nothing:
                break
            }
        }
    }

    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let t = touch else { return }
        if case .middle = t.zone { _ = stick.lifted(at: 0) }
        lifted()
        driver.ended(at: nil)
    }

    private func lifted() {
        touch = nil
        stopTicks()
        endHold()
        ball.pressed = false
        moveBall(to: .zero, spring: true)
        hideMarks()
    }

    private func rolledJustBefore(_ time: TimeInterval) -> Bool {
        lastNotch.map { time - $0 < HeadClick.still } ?? false
    }

    private func offset(of finger: UITouch) -> CGPoint {
        let p = finger.location(in: self)
        return CGPoint(x: p.x - bounds.midX, y: p.y - bounds.midY)
    }

    private func place(of mark: HeadZone.Mark) -> HeadMarks.Place {
        switch mark {
        case .top: .top
        case .left: .left
        case .right: .right
        case .bottom: .bottom
        }
    }

    /// The ball sits where the thumb pushed it; at the lift it springs back to the middle.
    private func moveBall(to offset: CGPoint, spring: Bool = false) {
        let to = CATransform3DMakeTranslation(offset.x, offset.y, 0)
        let from = ball.presentation()?.transform ?? ball.transform
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        ball.removeAnimation(forKey: "spring")
        ball.transform = to
        CATransaction.commit()
        guard spring, !UIAccessibility.isReduceMotionEnabled, !CATransform3DEqualToTransform(from, to) else { return }
        let back = CASpringAnimation(perceptualDuration: Self.springBack, bounce: Self.springBounce)
        back.keyPath = "transform"
        back.fromValue = NSValue(caTransform3D: from)
        back.toValue = NSValue(caTransform3D: to)
        back.duration = back.settlingDuration
        ball.add(back, forKey: "spring")
    }

    // MARK: The push

    /// A held push repeats: the stick says when, from the clock the touches keep.
    private func startTicks() {
        stopTicks()
        ticking = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: Self.tickEvery)
                guard let self, !Task.isCancelled, self.touch != nil else { return }
                let wasHeld = self.stick.held
                let sent = self.stick.tick(at: CACurrentMediaTime())
                if self.stick.held, !wasHeld { self.holdBegan() }
                if let sent { self.pushed(sent, again: true) }
            }
        }
    }

    private func stopTicks() {
        ticking?.cancel()
        ticking = nil
    }

    /// One push, or its repeat. In the hold a push `↑` or `↓` steps the sessions; else the mark
    /// at that place sends its key once: only the arrows and the cursor repeat.
    private func pushed(_ sent: HeadStick.Send, again: Bool) {
        guard case .key(let place, let word) = sent else { return }
        if inHold {
            let by = place == .top ? -1 : place == .bottom ? 1 : 0
            if by != 0, sessions?.step(by: by) == true {
                driver.feel.notch(heat: 0)
                showSessions()
            }
            return
        }
        let id = marks[place]
        if again, !Self.repeating.contains(id) { return }
        driver.feel.notch(heat: 0)
        switch id {
        case "↑", "↓":
            arrow(up: id == "↑")
        default:
            if word, let bytes = HeadMarks.wordBytes(for: id) { onSend(bytes) } else { press(id) }
        }
    }

    /// `↑` or `↓`: the agent's own arrow key, as on a keyboard. Up in the output a push reads a
    /// line older or newer instead, and at the first option of the menu on screen `↑` goes up into
    /// the output: no key goes. The menu is read from the screen at each push, never from the host.
    private func arrow(up: Bool) {
        if scroll.linesUp > 0 { return readUp(up) }
        let menu = ScreenMenu.read(lines: visibleLines())
        if up, menu?.chosen == 1 { return readUp(true) }
        onSend(KeyBar.bytes(for: up ? "↑" : "↓") ?? [])
        guard let menu else { return }
        let n = up ? max(menu.chosen - 1, 1) : min(menu.chosen + 1, menu.count)
        say(HeadHint.option(n: n, of: menu.count, text: menu.options[n - 1]).line)
    }

    /// One line through the output: older for `up`.
    private func readUp(_ up: Bool) {
        let was = scroll.linesUp
        switch scroll.push(up ? .up : .down, route: route(up)) {
        case .scroll(let lines):
            _ = onLines(lines)
            say(HeadHint.up(scroll.linesUp).line)
        case .home:
            _ = onLines(-was)
            driver.feel.cross()
            say(nil)
        case .bump(let bump):
            driver.feel.bump()
            say(HeadHint.bump(bump).line)
        }
    }

    // MARK: The roll

    /// The edge's roll and its coast scroll the output; they send no key.
    private func notch(_ dir: HeadScroll.Dir, at time: TimeInterval) {
        if case .middle? = touch?.zone { return }
        lastNotch = time
        let up = scroll.linesUp
        switch scroll.notch(dir, route: route(dir == .up), at: time) {
        case .scroll(let lines):
            scrolled(lines, at: time)
        case .home:
            _ = onLines(-up)
            driver.feel.cross()
            say(nil)
        case .bump(let bump):
            if !rollBumped { driver.feel.bump() }
            rollBumped = true
            driver.stopCoast()
            say(HeadHint.bump(bump).line)
        }
    }

    private func scrolled(_ lines: Int, at time: TimeInterval) {
        switch onLines(lines) {
        case .wheel, .scrollback:
            driver.tick(at: time)
            say(HeadHint.up(scroll.linesUp).line)
        case .end, .nothing:
            break
        }
        if abs(lines) >= Self.cardPace { showCard() }
    }

    /// The nearest step at the top of the view, and how far up the view is. It goes 0.6 s after the
    /// last notch at the card's pace.
    private func showCard() {
        cardGoes?.cancel()
        if let step = StepMark.nearest(in: visibleLines()) {
            let card = "\(step) · \(scroll.linesUp) lines up"
            onCard(card)
            #if DEBUG
            lastCard = card
            #endif
        }
        cardGoes = Task { [weak self] in
            try? await Task.sleep(for: Self.cardStays)
            guard let self, !Task.isCancelled else { return }
            self.onCard(nil)
        }
    }

    // MARK: The middle

    /// Up in the output a tap comes back down and sends nothing; else it is Enter, at once.
    private func tapped() {
        let up = scroll.linesUp
        switch scroll.tap() {
        case .home:
            _ = onLines(-up)
            driver.feel.cross()
            say(nil)
        case .enter:
            driver.feel.click()
            ball.flash()
            onSend(Self.enter)
        }
    }

    // MARK: The hold

    /// A still press on the middle: the ring fills from 0.15 s to 0.6 s, and the stick says when
    /// the hold begins (`HeadStick.held`), on the touches' clock. The sessions are read as the ring
    /// starts, so the card can show when it is full. A push or a lift before it is full ends it
    /// with nothing sent.
    private func startHold() {
        endHold()
        #if DEBUG
        lastSessions = nil
        #endif
        let id = marks[.hold]
        let command = command
        holding = Task { [weak self] in
            try? await Task.sleep(for: .seconds(Self.ringFrom))
            guard let self, !Task.isCancelled, self.touch != nil else { return }
            self.fillRing()
            guard id == Self.sessionsKey else { return }
            var reply: HostReply?
            if let command { reply = await command(["sessions", "--json"]) }
            guard !Task.isCancelled else { return }
            self.listed = .some(reply)
            if self.inHold { self.sessionsRead(reply) }
        }
    }

    /// The hold the stick began on a touch that may hold: pushes step the sessions.
    private var inHold: Bool {
        stick.held && holding != nil
    }

    /// The ring is full: the hold begins, and shows the sessions if the list has answered.
    private func holdBegan() {
        guard inHold else { return }
        driver.feel.cross()
        if case .some(let reply) = listed { sessionsRead(reply) }
    }

    /// A list that does not read says so, and the lift then opens nothing.
    private func sessionsRead(_ reply: HostReply?) {
        guard let reply, reply.ok, let rows = SessionList.decode(reply.output) else {
            sessions = nil
            say(SessionCard.unreachable)
            return show(SessionCard.unreachable)
        }
        sessions = SessionCard(rows: rows, pane: pane, opened: opened())
        showSessions()
    }

    private func showSessions() {
        if let sessions { show(sessions.text) }
    }

    private func show(_ card: String) {
        onSessions(card)
        #if DEBUG
        lastSessions = card
        #endif
    }

    private func fillRing() {
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        holdRing.isHidden = false
        holdRing.strokeEnd = 1
        CATransaction.commit()
        let fill = CABasicAnimation(keyPath: "strokeEnd")
        fill.fromValue = 0
        fill.toValue = 1
        fill.duration = Self.ringFills
        holdRing.add(fill, forKey: "fill")
    }

    private func endHold() {
        holding?.cancel()
        holding = nil
        holdRing.removeAllAnimations()
        holdRing.isHidden = true
        sessions = nil
        listed = nil
        onSessions(nil)
    }

    // MARK: The marks

    private func press(_ id: String) {
        if id == "⌨" { return onKeyboard() }
        guard let bytes = HeadMarks.bytes(for: id), !bytes.isEmpty else { return }
        onSend(bytes)
    }

    private func showMarks() {
        fades += 1
        for (mark, label) in markLabels {
            let id = marks[place(of: mark)]
            let word = KeyFace.word(of: id) ?? ""
            label.text = "\(id)\n\(word)"
            label.accessibilityLabel = word
        }
        rim.isHidden = false
        UIView.animate(withDuration: Self.marksIn, delay: 0, options: [.beginFromCurrentState], animations: { [weak self] in
            self?.rim.alpha = 1
        })
    }

    private func hideMarks() {
        fades += 1
        let fade = fades
        UIView.animate(withDuration: Self.marksOut, delay: Self.marksStay, options: [.beginFromCurrentState], animations: { [weak self] in
            self?.rim.alpha = 0
        }, completion: { [weak self] _ in
            guard let self, self.fades == fade else { return }
            self.rim.isHidden = true
        })
    }

    private func draw(turn: CGFloat, heat: CGFloat, struck: Bool) {
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        ball.turn = turn
        ball.heat = heat
        ball.struck = struck
        CATransaction.commit()
    }

    // MARK: VoiceOver

    /// The head as VoiceOver reaches it: a swipe up or down pages, a double-tap is the middle's
    /// tap, and the marks and the hold are its actions.
    private final class VoiceHead: UIAccessibilityElement {
        /// True: VoiceOver's decrement, a swipe down.
        var swipe: (Bool) -> Void = { _ in }
        var activate: () -> Bool = { false }
        var focused: () -> Void = {}
        /// Read when asked, as the colour and the head's last line change after the element is made.
        var readValue: () -> String? = { nil }
        /// Made when asked, from the marks home set last.
        var actions: () -> [UIAccessibilityCustomAction] = { [] }

        override func accessibilityIncrement() { swipe(false) }
        override func accessibilityDecrement() { swipe(true) }
        override func accessibilityActivate() -> Bool { activate() }
        override func accessibilityElementDidBecomeFocused() { focused() }
        override var accessibilityValue: String? {
            get { readValue() }
            set {}
        }
        override var accessibilityCustomActions: [UIAccessibilityCustomAction]? {
            get { actions() }
            set {}
        }
    }

    override var accessibilityElements: [Any]? {
        get {
            var elements: [Any] = [element]
            if !rim.isHidden { elements += markLabels.values.map { $0 as Any } }
            return elements
        }
        set {}
    }

    /// What the head last told VoiceOver, else its colour. The DEBUG lines are what the UI tests read.
    private var spokenValue: String {
        var shown = [voiceLine ?? ball.pigment.name]
        #if DEBUG
        shown.append("actions=" + voiceMarks().map { $0.name }.joined(separator: "|"))
        shown += [lastCard.map { "card=\($0)" }, lastSessions.map { "sessions=\($0)" }].compactMap { $0 }
        #endif
        return shown.joined(separator: "\n")
    }

    private func voiceFocused() {
        voiceLine = nil
    }

    private func voiceSwipe(older: Bool) {
        let line = HeadVoice.paged(page(older), older: older)
        voiceLine = line
        say(line)
    }

    /// The double-tap is the middle's tap: up in the output it comes back, else Enter.
    private func voiceTap() -> Bool {
        voiceLine = nil
        tapped()
        return true
    }

    /// The four marks and the hold, named from the marks home set last. A mark that sends nothing
    /// (`·`) is no action.
    private func voiceMarks() -> [(name: String, id: String)] {
        let marks = HeadMarks.load(from: .standard)
        var named: [(name: String, id: String)] = []
        for place in [HeadMarks.Place.top, .left, .right, .bottom] {
            let id = marks[place]
            guard id != "·" else { continue }
            named.append(("\(KeyFace.word(of: id) ?? id), \(place.rawValue) mark", id))
        }
        let hold = marks[.hold]
        if hold == Self.sessionsKey {
            named.append((KeyFace.word(of: hold) ?? hold, hold))
        } else if hold != "·" {
            named.append(("\(KeyFace.word(of: hold) ?? hold), hold", hold))
        }
        return named
    }

    private func voiceActions() -> [UIAccessibilityCustomAction] {
        voiceMarks().map { mark in
            let id = mark.id
            return UIAccessibilityCustomAction(name: mark.name) { [weak self] _ in
                MainActor.assumeIsolated {
                    guard let self else { return false }
                    if id == Self.sessionsKey {
                        self.voiceSessions()
                    } else {
                        self.press(id)
                    }
                    return true
                }
            }
        }
    }

    /// The sessions as a sheet: each other session opens as the hold's lift opens it. The one the
    /// console shows, and two that share a number, are not offered.
    private func voiceSessions() {
        guard let command else { return announce(SessionCard.unreachable) }
        let pane = pane
        Task { [weak self] in
            let reply = await command(["sessions", "--json"])
            guard let self else { return }
            guard reply.ok, let rows = SessionList.decode(reply.output) else { return self.announce(SessionCard.unreachable) }
            self.presentSessions(SessionCard(rows: rows, pane: pane, opened: self.opened()))
        }
    }

    private func presentSessions(_ card: SessionCard) {
        let open = card.rows.indices.filter { i in
            i != card.current && card.rows.filter { $0.n == card.rows[i].n }.count == 1
        }
        guard !open.isEmpty else { return announce(SessionCard.alone) }
        guard var top = window?.rootViewController else { return }
        while let next = top.presentedViewController { top = next }
        let sheet = UIAlertController(title: "Sessions", message: nil, preferredStyle: .actionSheet)
        for i in open {
            let row = card.rows[i]
            let n = row.n
            sheet.addAction(UIAlertAction(title: row.word.isEmpty ? row.name : "\(row.name) · \(row.word)", style: .default) {
                [weak self] _ in MainActor.assumeIsolated { self?.openSession(n) }
            })
        }
        sheet.addAction(UIAlertAction(title: "Cancel", style: .cancel))
        // An iPad shows the sheet as a popover at the head.
        sheet.popoverPresentationController?.sourceView = self
        sheet.popoverPresentationController?.sourceRect = bounds
        top.present(sheet, animated: true)
    }

    private func announce(_ line: String) {
        UIAccessibility.post(notification: .announcement, argument: line)
    }
}
