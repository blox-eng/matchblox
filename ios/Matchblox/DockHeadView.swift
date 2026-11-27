import MatchbloxKit
import UIKit

/// The one match head, over the dock's belt. Where the finger lands decides the meaning: a tap on
/// the middle answers, a press on the edge sends the key of the mark beside it, and a roll round
/// the edge walks the agent's options, then the output (`HeadWalk`). A menu is answered only by a
/// pick the host checks, never by arrow keys. The ring and the marks show while it is touched. A
/// still hold on the middle fills a ring, then shows the sessions (`SessionCard`).
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
    /// A held `←` or `→` repeats after this, then every `repeatGap`, and past `wordAfter` repeats
    /// it moves a word at a time.
    private static let repeatDelay: Duration = .milliseconds(420)
    private static let repeatGap: Duration = .milliseconds(110)
    private static let wordAfter = 9
    /// From this many lines a notch the card names the step at the top of the view.
    private static let cardPace = 3
    private static let cardStays: Duration = .milliseconds(600)
    private static let enter = KeyBar.bytes(for: "⏎") ?? [0x0d]
    /// The hold's ring starts to fill this long into a still press, and is full at `HeadClick.hold`.
    private static let ringFrom: TimeInterval = 0.15
    private static let ringFills: TimeInterval = HeadClick.hold - ringFrom
    private static let sessionsKey = "⇄"

    /// The agent's pane the head reads its menu from and picks in: the pane the app jumped into,
    /// or the pane of the session the console's chat shows; nil on the console's list.
    var pane: String? {
        didSet {
            guard pane != oldValue else { return }
            walk = HeadWalk()
            sight = nil
            menuRead = nil
            menuReady = false
            voiceLine = nil
            if pane == nil { walk.menu(nil) }
            onHighlight(nil)
        }
    }
    /// Runs Pocket's matchblox commands where the console is.
    var command: HostCommand?
    /// The roll moved `lines` lines (positive: older). The answer says where they went.
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
    /// Pocket's highlight over the row that shows this option, or nil for none.
    var onHighlight: (String?) -> Void = { _ in }
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
        let time: TimeInterval
        let zone: HeadZone.Zone
        var moved: CGFloat = 0
        var notched = false
        var repeated = false
        /// An end bumps once a roll.
        var bumped = false
        /// The hold began: moves step the sessions, and the lift is the hold's.
        var held = false
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
        element.accessibilityHint = "Swipe up or down for the options, else a page. Double-tap to answer, or for Enter."
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
    private var walk = HeadWalk()
    /// The menu the walk was last given.
    private var sight: MenuSight?
    private var marks = HeadMarks()
    private var touch: Touch?
    private var lastNotch: TimeInterval?
    private var menuRead: Task<Void, Never>?
    /// The last menu read has answered: `sight` is what the screen showed.
    private var menuReady = false
    /// What the head last told VoiceOver; its value until the next touch or focus.
    private var voiceLine: String?
    private var repeating: Task<Void, Never>?
    private var cardGoes: Task<Void, Never>?
    private var holding: Task<Void, Never>?
    /// The sessions the hold shows; nil until the list reads.
    private var sessions: SessionCard?
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
        walk.menu(nil)

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
        ball.frame = bounds.insetBy(dx: -HeadLayer.margin, dy: -HeadLayer.margin)
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
        stopRepeat()
        endHold()
        driver.leave()
    }

    // MARK: Touch

    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard touch == nil, let finger = touches.first else { return }
        let from = offset(of: finger)
        let zone = HeadZone.at(point: from, radius: Self.side / 2)
        touch = Touch(start: from, time: finger.timestamp, zone: zone)
        voiceLine = nil
        marks = HeadMarks.load(from: .standard)
        showMarks()
        readMenu()
        ball.pressed = true
        driver.began(at: finger.location(in: self), centre: CGPoint(x: bounds.midX, y: bounds.midY), time: finger.timestamp)
        switch zone {
        case .edge(let mark):
            startRepeat(marks[place(of: mark)])
        case .middle:
            if !rolledJustBefore(finger.timestamp) { startHold() }
        }
    }

    override func touchesMoved(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard var t = touch, let finger = touches.first else { return }
        let from = offset(of: finger)
        if t.held {
            if sessions?.moved(to: from, radius: Self.side / 2) == true {
                driver.feel.notch(heat: 0)
                showSessions()
            }
        } else {
            t.moved = max(t.moved, hypot(from.x - t.start.x, from.y - t.start.y))
            touch = t
            if t.moved >= HeadClick.slop, holding != nil { endHold() }
        }
        if !HeadZone.isPress(moved: t.moved) { stopRepeat() }
        driver.moved(to: finger.location(in: self), time: finger.timestamp)
    }

    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let t = touch, let finger = touches.first else { return }
        let goTo = sessions?.lift
        lifted()
        switch t.zone {
        case .middle where t.held:
            let id = marks[.hold]
            if id != Self.sessionsKey {
                press(id)
            } else if let goTo {
                openSession(goTo)
            }
        case .middle:
            let rolled = rolledJustBefore(t.time) || t.moved >= HeadClick.slop
            let lift = HeadClick.lift(after: finger.timestamp - t.time, rolled: rolled, coasting: false)
            if lift == .enter { tapped() }
        case .edge(let mark):
            if !t.notched, !t.repeated, HeadZone.isPress(moved: t.moved) { press(marks[place(of: mark)]) }
        }
    }

    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard touch != nil else { return }
        lifted()
    }

    private func lifted() {
        touch = nil
        stopRepeat()
        endHold()
        ball.pressed = false
        driver.ended()
        walk.lift()
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

    // MARK: The roll

    /// A roll from the middle only turns the head: it sends nothing and walks nothing.
    private func notch(_ dir: HeadWalk.Dir, at time: TimeInterval) {
        guard var t = touch, case .edge = t.zone, !t.repeated else { return }
        t.notched = true
        touch = t
        lastNotch = time
        let up = walk.linesUp
        switch walk.notch(dir, route: route(dir == .up), at: time) {
        case .highlight(let n):
            driver.tick(at: time)
            guard let sight, let option = sight.options.first(where: { $0.n == n }) else { return }
            say(HeadHint.option(n: n, of: sight.options.count, text: option.text).line)
            onHighlight(option.text)
        case .wait:
            say(HeadHint.waiting.line)
        case .cross:
            driver.cross()
            driver.feel.cross()
            onHighlight(nil)
            say(HeadHint.crossed.line)
        case .scroll(let lines):
            scrolled(lines, at: time)
        case .home:
            _ = onLines(-up)
            driver.feel.cross()
            say(nil)
        case .bump(let bump):
            if touch?.bumped == false { driver.feel.bump() }
            touch?.bumped = true
            say(HeadHint.bump(bump).line)
        }
    }

    private func scrolled(_ lines: Int, at time: TimeInterval) {
        switch onLines(lines) {
        case .wheel, .scrollback:
            driver.tick(at: time)
            say(HeadHint.up(walk.linesUp).line)
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
            let card = "\(step) · \(walk.linesUp) lines up"
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

    /// Up in the output a tap comes back down; at a menu it sends the highlighted option, which
    /// the host types only while the menu reads as it did; on a screen read with no menu it is
    /// Enter; on one not read it sends nothing and says so. A menu read still on its way is waited
    /// for: a tap never answers a menu it has not seen.
    private func tapped() {
        let reading = menuRead
        Task { [weak self] in
            await reading?.value
            guard let self else { return }
            let up = self.walk.linesUp
            switch self.walk.tap() {
            case .home:
                _ = self.onLines(-up)
                self.driver.feel.cross()
                self.say(nil)
            case .enter:
                self.sent()
                self.onSend(Self.enter)
            case .pick(let n, let text, let sign):
                await self.pick(n, text: text, sign: sign)
            case .unreadable:
                self.refused(.unreadable)
            }
        }
    }

    private func pick(_ n: Int, text: String, sign: String) async {
        guard let pane, let command else { return }
        onHighlight(nil)
        let reply = await command(["choose", pane, String(n), "--text", text, "--sign", sign])
        if let hint = HeadHint.afterPick(reply) {
            refused(hint)
        } else {
            sent()
            say(nil)
        }
    }

    private func refused(_ hint: HeadHint) {
        driver.feel.bump()
        say(hint.line)
        announce(hint.line)
    }

    private func sent() {
        driver.feel.click()
        ball.flash()
    }

    private func readMenu() {
        guard let pane else {
            sight = nil
            walk.menu(nil)
            menuReady = true
            return
        }
        guard let command else {
            sight = nil
            walk.read(.unreadable)
            menuReady = true
            return
        }
        menuReady = false
        menuRead = Task { [weak self] in
            let reply = await command(["menu", pane, "--json"])
            guard let self, self.pane == pane else { return }
            let reading = MenuSight.reading(reply)
            if case .menu(let menu) = reading { self.sight = menu } else { self.sight = nil }
            self.walk.read(reading)
            self.menuReady = true
        }
    }

    // MARK: The hold

    /// A still press on the middle: the ring fills from 0.15 s to 0.6 s, then the hold begins. The
    /// sessions are read as the ring starts, so the card can show when it is full. A move past the
    /// tap's slop or a lift before it is full ends it with nothing sent.
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
            var list: Task<HostReply?, Never>?
            if id == Self.sessionsKey {
                list = Task { () -> HostReply? in
                    guard let command else { return nil }
                    return await command(["sessions", "--json"])
                }
            }
            try? await Task.sleep(for: .seconds(Self.ringFills))
            guard !Task.isCancelled, var t = self.touch else { return }
            t.held = true
            self.touch = t
            self.driver.feel.cross()
            guard let list else { return }
            let reply = await list.value
            guard !Task.isCancelled, self.touch?.held == true else { return }
            self.sessionsRead(reply)
        }
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
        onSessions(nil)
    }

    // MARK: The marks

    private func press(_ id: String) {
        if id == "⌨" { return onKeyboard() }
        guard let bytes = HeadMarks.bytes(for: id), !bytes.isEmpty else { return }
        onSend(bytes)
    }

    private func startRepeat(_ id: String) {
        guard let word = HeadMarks.wordBytes(for: id), let bytes = HeadMarks.bytes(for: id) else { return }
        repeating = Task { [weak self] in
            try? await Task.sleep(for: Self.repeatDelay)
            var repeats = 0
            while !Task.isCancelled {
                guard let self, var t = self.touch else { return }
                t.repeated = true
                self.touch = t
                self.onSend(repeats < Self.wordAfter ? bytes : word)
                repeats += 1
                try? await Task.sleep(for: Self.repeatGap)
            }
        }
    }

    private func stopRepeat() {
        repeating?.cancel()
        repeating = nil
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

    /// The head as VoiceOver reaches it: a swipe up or down walks the options or pages, a
    /// double-tap is the middle's tap, and the marks and the hold are its actions.
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

    /// The menu is read as the head is reached, so the first swipe already knows it.
    private func voiceFocused() {
        voiceLine = nil
        readMenu()
    }

    /// A swipe on a menu already read answers at once, so VoiceOver reads the new value; one
    /// that waits for the read says its line when it comes.
    private func voiceSwipe(older: Bool) {
        if menuRead == nil { readMenu() }
        guard menuReady else {
            let reading = menuRead
            Task { [weak self] in
                await reading?.value
                guard let self, self.menuReady else { return }
                self.voiceSwipe(older: older)
                if let line = self.voiceLine { self.announce(line) }
            }
            return
        }
        let step = HeadVoice.swipe(&walk, older: older, menu: sight)
        let line: String
        switch step {
        case .option(_, _, let text):
            onHighlight(text)
            line = step.line ?? text
        case .first, .last:
            driver.feel.bump()
            line = step.line ?? ""
        case .page(let older):
            line = HeadVoice.paged(page(older), older: older)
        }
        voiceLine = line
        say(line)
    }

    /// The double-tap is the middle's tap: at a menu the swiped option, through the pick the host
    /// checks; up in the output it comes back; on a screen read with no menu Enter; on one not read
    /// nothing.
    private func voiceTap() -> Bool {
        if menuRead == nil { readMenu() }
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
