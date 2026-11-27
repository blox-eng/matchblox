import MatchbloxKit
import UIKit

/// The one match head, over the dock's belt. Where the finger lands decides the meaning: a tap on
/// the middle answers, a press on the edge sends the key of the mark beside it, and a roll round
/// the edge walks the agent's options, then the output (`HeadWalk`). A menu is answered only by a
/// pick the host checks, never by arrow keys. The ring and the marks show while it is touched.
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

    /// The agent's pane the head reads its menu from and picks in: the pane the app jumped into,
    /// or the pane of the session the console's chat shows; nil on the console's list.
    var pane: String? {
        didSet {
            guard pane != oldValue else { return }
            walk = HeadWalk()
            sight = nil
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
    }

    private let ball = HeadLayer()
    private let rim = UIView()
    private let ring = CAShapeLayer()
    private var markLabels: [HeadZone.Mark: UILabel] = [:]
    private lazy var element: UIAccessibilityElement = {
        let element = UIAccessibilityElement(accessibilityContainer: self)
        element.accessibilityLabel = "Match head"
        element.accessibilityIdentifier = "match-head"
        element.accessibilityTraits = .allowsDirectInteraction
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
    private var repeating: Task<Void, Never>?
    private var cardGoes: Task<Void, Never>?
    /// Counts each fade, so a fade out that ends after a new touch leaves the marks shown.
    private var fades = 0
    #if DEBUG
    /// The last card the head showed: the UI test reads it, as the card stays only 0.6 s.
    private var lastCard: String?
    #endif

    override init(frame: CGRect) {
        super.init(frame: frame)
        isMultipleTouchEnabled = false
        walk.menu(nil)

        ball.contentsScale = max(traitCollection.displayScale, 2)
        layer.addSublayer(ball)

        rim.isUserInteractionEnabled = false
        rim.alpha = 0
        rim.isHidden = true
        ring.fillColor = nil
        ring.strokeColor = BeltLook.muted.cgColor
        ring.lineWidth = 1
        rim.layer.addSublayer(ring)
        for mark in [HeadZone.Mark.top, .left, .right, .bottom] {
            let label = UILabel()
            label.numberOfLines = 2
            label.textAlignment = .center
            label.font = UIFont.monospacedSystemFont(ofSize: 10, weight: .medium)
            label.textColor = BeltLook.fg
            label.backgroundColor = BeltLook.sleeve.withAlphaComponent(0.92)
            label.layer.cornerRadius = 6
            label.layer.masksToBounds = true
            rim.addSubview(label)
            markLabels[mark] = label
        }
        addSubview(rim)
    }

    required init?(coder: NSCoder) { nil }

    override func layoutSubviews() {
        super.layoutSubviews()
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        ball.frame = bounds.insetBy(dx: -HeadLayer.margin, dy: -HeadLayer.margin)
        rim.frame = bounds
        let r = Self.side / 2 + Self.ringGap
        let c = CGPoint(x: bounds.midX, y: bounds.midY)
        ring.path = UIBezierPath(ovalIn: CGRect(x: c.x - r, y: c.y - r, width: 2 * r, height: 2 * r)).cgPath
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
        driver.leave()
    }

    // MARK: Touch

    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard touch == nil, let finger = touches.first else { return }
        let from = offset(of: finger)
        let zone = HeadZone.at(point: from, radius: Self.side / 2)
        touch = Touch(start: from, time: finger.timestamp, zone: zone)
        marks = HeadMarks.load(from: .standard)
        showMarks()
        readMenu()
        ball.pressed = true
        driver.began(at: finger.location(in: self), centre: CGPoint(x: bounds.midX, y: bounds.midY), time: finger.timestamp)
        if case .edge(let mark) = zone { startRepeat(marks[place(of: mark)]) }
    }

    override func touchesMoved(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard var t = touch, let finger = touches.first else { return }
        let from = offset(of: finger)
        t.moved = max(t.moved, hypot(from.x - t.start.x, from.y - t.start.y))
        touch = t
        if !HeadZone.isPress(moved: t.moved) { stopRepeat() }
        driver.moved(to: finger.location(in: self), time: finger.timestamp)
    }

    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let t = touch, let finger = touches.first else { return }
        lifted()
        switch t.zone {
        case .middle:
            let rolledBefore = lastNotch.map { t.time - $0 < HeadClick.still } ?? false
            let lift = HeadClick.lift(after: finger.timestamp - t.time, rolled: rolledBefore || t.moved >= HeadClick.slop, coasting: false)
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
        ball.pressed = false
        driver.ended()
        walk.lift()
        hideMarks()
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
    /// the host types only while the menu reads as it did; with no menu it is Enter. A menu read
    /// still on its way is waited for: a tap never answers a menu it has not seen.
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
            }
        }
    }

    private func pick(_ n: Int, text: String, sign: String) async {
        guard let pane, let command else { return }
        onHighlight(nil)
        let reply = await command(["choose", pane, String(n), "--text", text, "--sign", sign])
        if let hint = HeadHint.afterPick(reply) {
            driver.feel.bump()
            say(hint.line)
        } else {
            sent()
            say(nil)
        }
    }

    private func sent() {
        driver.feel.click()
        ball.flash()
    }

    private func readMenu() {
        guard let pane, let command else {
            sight = nil
            walk.menu(nil)
            return
        }
        menuRead = Task { [weak self] in
            let reply = await command(["menu", pane, "--json"])
            guard let self, self.pane == pane else { return }
            let menu = reply.ok ? MenuSight.decode(reply.output) : nil
            self.sight = menu
            self.walk.menu(menu)
        }
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

    // MARK: Accessibility

    override var accessibilityElements: [Any]? {
        get {
            #if DEBUG
            element.accessibilityValue = lastCard.map { "card=\($0)" }
            #endif
            var elements: [Any] = [element]
            if !rim.isHidden { elements += markLabels.values.map { $0 as Any } }
            return elements
        }
        set {}
    }
}
