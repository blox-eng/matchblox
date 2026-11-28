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
    /// How often a held push asks the stick for its repeat.
    private static let tickEvery: Duration = .milliseconds(15)
    /// The marks whose push repeats while held: the arrows and the cursor.
    private static let repeating: Set<String> = ["↑", "↓", "←", "→"]
    /// From this many lines a notch the card names the step at the top of the view.
    private static let cardPace = 3
    private static let cardStays: Duration = .seconds(HeadMotion.cardStays)
    private static let enter = KeyBar.bytes(for: "⏎") ?? [0x0d]
    /// The hold's ring starts to fill this long into a still press, and is full at `HeadClick.hold`.
    private static let ringFrom: TimeInterval = 0.15
    private static let ringFills: TimeInterval = HeadClick.hold - ringFrom
    private static let sessionsKey = "⇄"

    /// The agent's pane the console shows: the pane the app jumped into, or the pane of the
    /// session the console's chat shows; nil on the console's list. The sessions card marks it.
    /// Another pane shows its own live bottom.
    var pane: String? {
        didSet {
            if pane != oldValue { cameDown() }
        }
    }
    /// The console's chat takes the screen, not a pane the app jumped into: there `↑` `↓` choose a
    /// session, so a push moves Pocket's highlight over the question's options instead.
    var inChat = false {
        didSet {
            guard inChat != oldValue else { return }
            chatPick = ChatPick()
            onHighlight(nil)
            cameDown()
        }
    }
    /// Pocket's highlight over the row that shows this option, or nil for none.
    var onHighlight: (String?) -> Void = { _ in }
    /// Runs Pocket's matchblox commands where the console is.
    var command: HostCommand?
    /// The head moved the view `lines` lines (positive: older). The answer says where they went.
    var onLines: (Int) -> HeadRoute = { _ in .nothing }
    /// Where a notch of output would go now (true: toward older lines).
    var route: (Bool) -> HeadRoute = { _ in .nothing }
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
        element.accessibilityHint = HeadVoice.hint
        element.gesture = { [weak self] gesture in
            guard let self else { return false }
            self.voice(gesture)
            return true
        }
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
    private var chatPick = ChatPick()
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
    /// The marks' fade in or out that runs now, stopped where it is when the next one starts.
    private var marksMove: UIViewPropertyAnimator?
    /// Home sets the colour and the marks; the head only reads them, and never saves one.
    nonisolated(unsafe) private var settingsObserver: NSObjectProtocol?
    #if DEBUG
    /// The last card the head showed: the UI test reads it, as the card stays only 0.6 s.
    private var lastCard: String?
    /// The last sessions card, which goes at the lift.
    private var lastSessions: String?
    /// The marks' opacity just after the last touch-down: between 0 and 1 while they fade in.
    private var marksSeen: Double?
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
        holdRing.opacity = 0
        // In the rim, so the ring fades and scales with the marks.
        rim.layer.addSublayer(holdRing)
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
        // Bounds and centre, not the frame: the rim's transform scales it as it fades.
        rim.bounds = CGRect(origin: .zero, size: bounds.size)
        rim.center = CGPoint(x: bounds.midX, y: bounds.midY)
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
        stopMarks()
        rim.isHidden = true
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        holdRing.opacity = 0
        CATransaction.commit()
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

    /// The ball sits where the thumb pushed it, with no delay; at the lift it springs back to the
    /// middle, past it and back (`HeadMotion.springCurve`).
    private func moveBall(to offset: CGPoint, spring: Bool = false) {
        let to = CATransform3DMakeTranslation(offset.x, offset.y, 0)
        let from = ball.presentation()?.transform ?? ball.transform
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        ball.removeAnimation(forKey: "spring")
        ball.transform = to
        CATransaction.commit()
        let rule = HeadMotion.rule(reduceMotion: UIAccessibility.isReduceMotionEnabled)
        guard spring, rule.springs, !CATransform3DEqualToTransform(from, to) else { return }
        let back = CABasicAnimation(keyPath: "transform")
        back.fromValue = NSValue(caTransform3D: from)
        back.toValue = NSValue(caTransform3D: to)
        back.duration = HeadMotion.springBack
        back.timingFunction = HeadMotion.springCurve.media
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
        if word, let bytes = HeadMarks.wordBytes(for: id) { return onSend(bytes) }
        push(id)
    }

    /// A push toward `id`, or VoiceOver's: `↑` `↓` are the agent's arrows, the rest their keys.
    private func push(_ id: String) {
        switch id {
        case "↑", "↓": arrow(up: id == "↑")
        default: press(id)
        }
    }

    /// `↑` or `↓`: the agent's own arrow key, as on a keyboard. Up in the output a push reads a
    /// line older or newer instead, and at the first option of the menu on screen `↑` goes up into
    /// the output: no key goes. The menu is read from the screen at each push, never from the host.
    private func arrow(up: Bool) {
        checkBottom()
        if scroll.linesUp > 0 { return readUp(up) }
        if inChat { return chatArrow(up: up) }
        let menu = ScreenMenu.read(lines: visibleLines())
        if up, menu?.chosen == 1 { return readUp(true) }
        onSend(KeyBar.bytes(for: up ? "↑" : "↓") ?? [])
        guard let menu else { return }
        let n = up ? max(menu.chosen - 1, 1) : min(menu.chosen + 1, menu.count)
        say(HeadHint.option(n: n, of: menu.count, text: menu.options[n - 1]).line)
    }

    /// In the chat a push at a question moves Pocket's highlight, and `↑` at its first option reads
    /// up; with no options on screen it is the chat's own arrow, which chooses a session.
    private func chatArrow(up: Bool) {
        switch chatPick.push(up: up, menu: ScreenMenu.read(chat: visibleLines())) {
        case .on(let n, let count, let text):
            say(HeadHint.option(n: n, of: count, text: text).line)
            onHighlight(text)
        case .readUp:
            onHighlight(nil)
            readUp(true)
        case .arrow:
            onHighlight(nil)
            onSend(KeyBar.bytes(for: up ? "↑" : "↓") ?? [])
        }
    }

    /// The view came back down another way (a typed send, a key, another pane): the head's walk
    /// ends, and the handle row stops saying it is up.
    func cameDown() {
        guard scroll.linesUp > 0 else { return }
        scroll.cameDown()
        say(nil)
    }

    /// A terminal that knows it is at its live bottom ends a walk the head still counts as up.
    private func checkBottom() {
        if scroll.check(newer: route(false)) { say(nil) }
    }

    /// After a step down the terminal may be at its live bottom before the head's count is: a
    /// tmux wheel step moves more lines than one. Then the walk ends there, as at home.
    private func landedHome() -> Bool {
        guard scroll.check(newer: route(false)) else { return false }
        driver.feel.cross()
        say(nil)
        return true
    }

    /// One line through the output: older for `up`.
    private func readUp(_ up: Bool) {
        let was = scroll.linesUp
        switch scroll.push(up ? .up : .down, route: route(up)) {
        case .scroll(let lines):
            _ = onLines(lines)
            if lines < 0, landedHome() { return }
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
            if lines < 0, landedHome() { break }
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

    /// Up in the output a tap comes back down and sends nothing; else it is Enter, at once. At a
    /// question in the chat it types the highlighted option's number, which the chat answers with.
    private func tapped() {
        checkBottom()
        let up = scroll.linesUp
        switch scroll.tap() {
        case .home:
            _ = onLines(-up)
            driver.feel.cross()
            say(nil)
        case .enter:
            driver.feel.click()
            ball.flash()
            let answer = inChat ? chatPick.tap(menu: ScreenMenu.read(chat: visibleLines())) : nil
            onHighlight(nil)
            if answer != nil { say(nil) }
            onSend(answer ?? Self.enter)
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
        holdRing.removeAllAnimations()
        holdRing.opacity = 1
        holdRing.strokeEnd = 1
        CATransaction.commit()
        let fill = CABasicAnimation(keyPath: "strokeEnd")
        fill.fromValue = 0
        fill.toValue = 1
        fill.duration = Self.ringFills
        holdRing.add(fill, forKey: "fill")
    }

    /// The ring stops where it is filled. After a lift it goes with the marks; during a touch (a
    /// push ended the hold) it fades now.
    private func endHold() {
        holding?.cancel()
        holding = nil
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        holdRing.strokeEnd = holdRing.presentation()?.strokeEnd ?? holdRing.strokeEnd
        holdRing.removeAnimation(forKey: "fill")
        CATransaction.commit()
        if touch != nil {
            CATransaction.begin()
            CATransaction.setAnimationDuration(HeadMotion.marksFade)
            CATransaction.setAnimationTimingFunction(HeadMotion.marksCurve.media)
            holdRing.opacity = 0
            CATransaction.commit()
        }
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

    /// The marks and the ring fade in and grow from `HeadMotion.marksFrom`, from wherever a fade
    /// out had brought them.
    private func showMarks() {
        for (mark, label) in markLabels {
            let id = marks[place(of: mark)]
            let word = KeyFace.word(of: id) ?? ""
            label.text = "\(id)\n\(word)"
            label.accessibilityLabel = word
        }
        stopMarks()
        let rule = HeadMotion.rule(reduceMotion: UIAccessibility.isReduceMotionEnabled)
        if rim.isHidden {
            rim.alpha = 0
            rim.transform = Self.scaled(rule.fromScale)
            rim.isHidden = false
        }
        let move = UIViewPropertyAnimator(duration: rule.fade, timingParameters: HeadMotion.marksCurve.cubic)
        move.addAnimations { [rim] in
            rim.alpha = 1
            rim.transform = .identity
        }
        move.startAnimation()
        marksMove = move
        #if DEBUG
        marksSeen = nil
        Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(30))
            guard let self else { return }
            self.marksSeen = Double(self.rim.layer.presentation()?.opacity ?? self.rim.layer.opacity)
        }
        #endif
    }

    /// `HeadMotion.marksStay` after the lift the marks and the ring fade out and shrink. A touch
    /// before then stops it, and they stay.
    private func hideMarks() {
        stopMarks()
        let rule = HeadMotion.rule(reduceMotion: UIAccessibility.isReduceMotionEnabled)
        let shrunk = Self.scaled(rule.fromScale)
        let move = UIViewPropertyAnimator(duration: rule.fade, timingParameters: HeadMotion.marksCurve.cubic)
        move.addAnimations { [rim] in
            rim.alpha = 0
            rim.transform = shrunk
        }
        move.addCompletion { [weak self] position in
            guard let self, position == .end else { return }
            self.rim.isHidden = true
            CATransaction.begin()
            CATransaction.setDisableActions(true)
            self.holdRing.opacity = 0
            CATransaction.commit()
        }
        move.startAnimation(afterDelay: HeadMotion.marksStay)
        marksMove = move
    }

    /// Leaves the marks where the fade that runs has brought them.
    private func stopMarks() {
        if let marksMove, marksMove.state == .active { marksMove.stopAnimation(true) }
        marksMove = nil
    }

    private static func scaled(_ scale: Double) -> CGAffineTransform {
        CGAffineTransform(scaleX: CGFloat(scale), y: CGFloat(scale))
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

    /// The head as VoiceOver reaches it: a swipe up or down pushes, a double-tap is the middle's
    /// tap (`HeadVoice`), and the marks and the hold are its actions.
    private final class VoiceHead: UIAccessibilityElement {
        /// False once the head is gone.
        var gesture: (HeadVoice.Gesture) -> Bool = { _ in false }
        var focused: () -> Void = {}
        /// Read when asked, as the colour and the head's last line change after the element is made.
        var readValue: () -> String? = { nil }
        /// Made when asked, from the marks home set last.
        var actions: () -> [UIAccessibilityCustomAction] = { [] }

        override func accessibilityIncrement() { _ = gesture(.swipeUp) }
        override func accessibilityDecrement() { _ = gesture(.swipeDown) }
        override func accessibilityActivate() -> Bool { gesture(.doubleTap) }
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
        let gestures = HeadVoice.Gesture.allCases.map { Self.gestureLine($0) }
        shown.append("actions=" + (gestures + voiceMarks().map(\.name)).joined(separator: "|"))
        if let marksSeen { shown.append("marks=" + String(format: "%.2f", marksSeen)) }
        shown += [lastCard.map { "card=\($0)" }, lastSessions.map { "sessions=\($0)" }].compactMap { $0 }
        #endif
        return shown.joined(separator: "\n")
    }

    private func voiceFocused() {
        voiceLine = nil
    }

    #if DEBUG
    /// A gesture and what it does, as the UI test reads them: `swipe up: ↑`.
    private static func gestureLine(_ gesture: HeadVoice.Gesture) -> String {
        let name = switch gesture {
        case .swipeUp: "swipe up"
        case .swipeDown: "swipe down"
        case .doubleTap: "double-tap"
        }
        return switch HeadVoice.does(gesture) {
        case .push(let id): "\(name): \(id)"
        case .tap: "\(name): tap"
        }
    }
    #endif

    /// A swipe is a push down the thumb's path: in the chat it moves Pocket's highlight, in a pane
    /// it sends the arrow. VoiceOver then says the line the push gave the handle row. The
    /// double-tap is the middle's tap: up in the output it comes back, else Enter or the chat's digits.
    private func voice(_ gesture: HeadVoice.Gesture) {
        switch HeadVoice.does(gesture) {
        case .push(let id):
            let said = say
            var line: String?
            say = { text in
                line = text
                said(text)
            }
            push(id)
            say = said
            voiceLine = HeadVoice.spoken(key: id, said: line)
        case .tap:
            voiceLine = nil
            tapped()
        }
    }

    /// The four marks and the hold, from the marks home set last.
    private func voiceMarks() -> [HeadVoice.Action] {
        HeadVoice.actions(for: HeadMarks.load(from: .standard))
    }

    /// A mark's action is its push; the sessions open as a sheet.
    private func voiceActions() -> [UIAccessibilityCustomAction] {
        voiceMarks().map { mark in
            let id = mark.id
            return UIAccessibilityCustomAction(name: mark.name) { [weak self] _ in
                MainActor.assumeIsolated {
                    guard let self else { return false }
                    if id == Self.sessionsKey {
                        self.voiceSessions()
                    } else {
                        self.push(id)
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

@MainActor
extension HeadMotion.Curve {
    /// The curve for a view's animator.
    var cubic: UICubicTimingParameters {
        UICubicTimingParameters(controlPoint1: CGPoint(x: x1, y: y1), controlPoint2: CGPoint(x: x2, y: y2))
    }

    /// The curve for a layer's animation.
    var media: CAMediaTimingFunction {
        CAMediaTimingFunction(controlPoints: Float(x1), Float(y1), Float(x2), Float(y2))
    }
}
