import MatchbloxKit
import SwiftUI
import UIKit

/// The key belt: one row of keys from the phone's left side to its right side, turning round
/// both sides and on behind the phone, with no end. It moves in detents and clicks under the
/// thumb; a key sends when a touch lifts without a drag. Over it is the handle row: the
/// layout's name, the grip, and the keyboard key. A pull up on the handle row, or a tap on its
/// grip or name, opens the matchbox drawer between the two: the tray of layouts. One belt
/// rides on the keyboard and one docks under the terminal while the keyboard is down.
final class BeltView: UIInputView, UIGestureRecognizerDelegate {
    private struct Piece {
        let layer: CALayer
        let face: CAGradientLayer
        let text: CATextLayer
        let shade: CALayer
        let key: KeyFace
        let width: CGFloat
    }

    private struct Touch {
        let start: CGPoint
        let wasAtRest: Bool
        var tap = TapOrDrag()
        var travel: CGFloat = 0
    }

    static let handleHeight: CGFloat = 24
    private static let gripSize = CGSize(width: 36, height: 4)
    private static let handlePadding: CGFloat = 10
    private static let keyTop: CGFloat = 8
    private static let strikerInset: CGFloat = 6
    private static let radius: CGFloat = 6
    private static let markSide: CGFloat = 10
    /// 3 pt inside the key's 1 pt edge.
    private static let markInset: CGFloat = 4
    private static let perspective: CGFloat = 180
    private static let gritStep: CGFloat = 3 * 2.squareRoot()
    private static let gritWidth: CGFloat = 1.4
    private static let pressTime: TimeInterval = 0.14
    private static let flashTime: TimeInterval = 1.1
    private static let dim: Float = 0.4
    /// A layout change runs the old keys off this far each frame, for this many frames.
    private static let runOffStep: CGFloat = 18
    private static let runOffFrames: CGFloat = 12
    /// The drawer: the tray, 4 pt under the handle row and 4 pt over the belt.
    private static let drawerOpen: CGFloat = MatchTrayView.height + 8
    private static let trayInset: CGFloat = 6
    /// A tap this near the grip's centre, on either side, is a tap on the grip.
    private static let gripReach: CGFloat = 30
    /// A pull up or down this far opens or closes the drawer.
    private static let pullDistance: CGFloat = 12
    private static let drawerGrowTime: TimeInterval = 0.26
    private static let traySlideTime: TimeInterval = 0.3

    private static let glyphFont = UIFont.monospacedSystemFont(ofSize: 16, weight: .medium)
    private static let bigFont = UIFont.monospacedSystemFont(ofSize: 21, weight: .medium)
    private static let wordFont = UIFont.monospacedSystemFont(ofSize: 11.5, weight: .medium)
    private static let nameFont = UIFont.monospacedSystemFont(ofSize: 10, weight: .medium)

    /// The app's copy of the console's command table.
    static let catalog: KeyCatalog = {
        if let url = Bundle.main.url(forResource: "commands", withExtension: "json"),
           let data = try? Data(contentsOf: url),
           let catalog = try? KeyCatalog(commands: data) {
            return catalog
        }
        // An empty table always decodes: the belt then has no command keys.
        return try! KeyCatalog(commands: Data("[]".utf8))
    }()

    var layout: BeltLayout {
        didSet {
            guard layout != oldValue else { return }
            layout.save(to: .standard)
            runOff()
        }
    }

    /// The agent of the chat's session; nil or empty for none.
    var agent: String? {
        didSet {
            if agent?.isEmpty == true { agent = nil }
            trayModel.agent = agent
            guard agent != oldValue else { return }
            if layout == .commands { rebuild() }
        }
    }

    /// False in a jumped pane, or with no agent: command keys dim and send nothing.
    var commandsLive = false {
        didSet {
            guard commandsLive != oldValue else { return }
            spoken = nil
            place()
        }
    }

    var onSend: ([UInt8]) -> Void = { _ in }
    var onKeyboard: () -> Void = {}
    /// The builder picked a layout in the tray.
    var onLayout: (BeltLayout) -> Void = { _ in }
    var onEditMine: () -> Void = {}

    private(set) var trayOpen = false

    var keyboardUp = false {
        didSet {
            keyboardButton.tintColor = keyboardUp ? BeltLook.fg : BeltLook.faint
            keyboardButton.accessibilityLabel = keyboardUp ? "Hide keyboard" : "Show keyboard"
        }
    }

    private let handle = UIView()
    private let markView = UIImageView()
    private let nameLabel = UILabel()
    private let nameStack = UIStackView()
    private let grip = UIView()
    private let keyboardButton = UIButton(type: .system)
    private let belt = UIView()
    private let strikerGround = CALayer()
    private let grit = CAShapeLayer()
    private let strikerTop = CALayer()
    private let strikerBottom = CALayer()
    private let track = CALayer()
    private let pan = UIPanGestureRecognizer()
    private let handleTap = UITapGestureRecognizer()
    private let pull = UIPanGestureRecognizer()
    private let drawer = UIView()
    private lazy var drawerHeight: NSLayoutConstraint = drawer.heightAnchor.constraint(equalToConstant: 0)
    private let trayModel: TrayModel
    private let trayHost: UIHostingController<MatchTrayView>
    /// Counts each open, close and pick, so a close that was due after a pick knows it is stale.
    private var trayMoves = 0

    private let selection = UISelectionFeedbackGenerator()
    private let impact = UIImpactFeedbackGenerator(style: .medium)

    private var motion: BeltMotion?
    /// The layout's keys the belt holds now.
    private var builtKeys: [BeltKey] = []
    private var pieces: [Piece] = []
    private var builtWidth: CGFloat = 0
    private var touch: Touch?
    /// The finger's last x while it drags the belt.
    private var dragX: CGFloat = 0
    private var link: CADisplayLink?
    private var lastFrame: CFTimeInterval?
    /// Frames left of a layout change's run-off.
    private var runOffLeft: CGFloat = 0
    private var flashes = 0
    /// VoiceOver's elements, built when asked and dropped when the keys change or come to rest.
    private var spoken: [Any]?

    init() {
        let layout = BeltLayout.load(from: .standard)
        let model = TrayModel(inUse: layout)
        self.layout = layout
        trayModel = model
        trayHost = UIHostingController(rootView: MatchTrayView(model: model))
        super.init(frame: CGRect(x: 0, y: 0, width: 0, height: Self.handleHeight + BeltGeometry.height), inputViewStyle: .default)
        allowsSelfSizing = true
        backgroundColor = BeltLook.sleeve
        model.onPick = { [weak self] picked in self?.picked(picked) }
        model.onEdit = { [weak self] in self?.editMine() }
        buildHandle()
        buildDrawer()
        buildBelt()
        updateName()
    }

    required init?(coder: NSCoder) { nil }

    // MARK: Building

    private func buildHandle() {
        handle.backgroundColor = BeltLook.sleeve
        handle.translatesAutoresizingMaskIntoConstraints = false
        addSubview(handle)

        let edge = UIView()
        edge.backgroundColor = BeltLook.sleeveEdge
        edge.translatesAutoresizingMaskIntoConstraints = false
        handle.addSubview(edge)

        markView.contentMode = .scaleAspectFit
        let name = nameStack
        name.addArrangedSubview(markView)
        name.addArrangedSubview(nameLabel)
        name.spacing = 5
        name.alignment = .center
        name.translatesAutoresizingMaskIntoConstraints = false
        handle.addSubview(name)

        grip.backgroundColor = BeltLook.grip
        grip.layer.cornerRadius = Self.gripSize.height / 2
        grip.translatesAutoresizingMaskIntoConstraints = false
        handle.addSubview(grip)

        var config = UIButton.Configuration.plain()
        config.image = UIImage(systemName: "keyboard", withConfiguration: UIImage.SymbolConfiguration(pointSize: 12, weight: .light))
        config.contentInsets = NSDirectionalEdgeInsets(top: 0, leading: 8, bottom: 0, trailing: Self.handlePadding)
        keyboardButton.configuration = config
        keyboardButton.tintColor = BeltLook.faint
        keyboardButton.accessibilityLabel = "Show keyboard"
        keyboardButton.translatesAutoresizingMaskIntoConstraints = false
        keyboardButton.addAction(UIAction { [weak self] _ in self?.onKeyboard() }, for: .touchUpInside)
        handle.addSubview(keyboardButton)

        handleTap.addTarget(self, action: #selector(handleTapped))
        handleTap.delegate = self
        handle.addGestureRecognizer(handleTap)
        pull.addTarget(self, action: #selector(pulled))
        pull.maximumNumberOfTouches = 1
        pull.delegate = self
        handle.addGestureRecognizer(pull)

        NSLayoutConstraint.activate([
            handle.topAnchor.constraint(equalTo: topAnchor),
            handle.leadingAnchor.constraint(equalTo: leadingAnchor),
            handle.trailingAnchor.constraint(equalTo: trailingAnchor),
            handle.heightAnchor.constraint(equalToConstant: Self.handleHeight),
            edge.topAnchor.constraint(equalTo: handle.topAnchor),
            edge.leadingAnchor.constraint(equalTo: handle.leadingAnchor),
            edge.trailingAnchor.constraint(equalTo: handle.trailingAnchor),
            edge.heightAnchor.constraint(equalToConstant: 1),
            name.leadingAnchor.constraint(equalTo: handle.safeAreaLayoutGuide.leadingAnchor, constant: Self.handlePadding),
            name.centerYAnchor.constraint(equalTo: handle.centerYAnchor, constant: 0.5),
            name.trailingAnchor.constraint(lessThanOrEqualTo: grip.leadingAnchor, constant: -8),
            markView.widthAnchor.constraint(equalToConstant: Self.markSide),
            markView.heightAnchor.constraint(equalToConstant: Self.markSide),
            grip.centerXAnchor.constraint(equalTo: handle.centerXAnchor),
            grip.centerYAnchor.constraint(equalTo: handle.centerYAnchor, constant: 0.5),
            grip.widthAnchor.constraint(equalToConstant: Self.gripSize.width),
            grip.heightAnchor.constraint(equalToConstant: Self.gripSize.height),
            keyboardButton.trailingAnchor.constraint(equalTo: handle.safeAreaLayoutGuide.trailingAnchor),
            keyboardButton.topAnchor.constraint(equalTo: handle.topAnchor, constant: 1),
            keyboardButton.bottomAnchor.constraint(equalTo: handle.bottomAnchor),
            keyboardButton.widthAnchor.constraint(greaterThanOrEqualToConstant: 44),
        ])
    }

    /// The matchbox drawer: shut, it has no height; open, the tray slides up out of the sleeve.
    private func buildDrawer() {
        drawer.backgroundColor = BeltLook.sleeve
        drawer.clipsToBounds = true
        drawer.isHidden = true
        drawer.translatesAutoresizingMaskIntoConstraints = false
        addSubview(drawer)

        trayHost.safeAreaRegions = []
        let tray: UIView = trayHost.view
        tray.backgroundColor = .clear
        tray.translatesAutoresizingMaskIntoConstraints = false
        tray.transform = CGAffineTransform(translationX: 0, y: MatchTray.slide)
        drawer.addSubview(tray)

        NSLayoutConstraint.activate([
            drawer.topAnchor.constraint(equalTo: handle.bottomAnchor),
            drawer.leadingAnchor.constraint(equalTo: leadingAnchor),
            drawer.trailingAnchor.constraint(equalTo: trailingAnchor),
            drawerHeight,
            tray.topAnchor.constraint(equalTo: drawer.topAnchor, constant: 4),
            tray.leadingAnchor.constraint(equalTo: drawer.safeAreaLayoutGuide.leadingAnchor, constant: Self.trayInset),
            tray.trailingAnchor.constraint(equalTo: drawer.safeAreaLayoutGuide.trailingAnchor, constant: -Self.trayInset),
            tray.heightAnchor.constraint(equalToConstant: MatchTrayView.height),
        ])
    }

    private func buildBelt() {
        belt.backgroundColor = BeltLook.sleeve
        belt.clipsToBounds = true
        belt.isMultipleTouchEnabled = false
        belt.translatesAutoresizingMaskIntoConstraints = false
        addSubview(belt)

        strikerGround.backgroundColor = BeltLook.cardGround.cgColor
        strikerGround.masksToBounds = true
        grit.strokeColor = BeltLook.striker.cgColor
        grit.fillColor = nil
        grit.lineWidth = Self.gritWidth
        strikerTop.backgroundColor = BeltLook.strikerTopLine.cgColor
        strikerBottom.backgroundColor = BeltLook.strikerBottomLine.cgColor
        strikerGround.addSublayer(grit)
        strikerGround.addSublayer(strikerTop)
        strikerGround.addSublayer(strikerBottom)
        belt.layer.addSublayer(strikerGround)

        var perspective = CATransform3DIdentity
        perspective.m34 = -1 / Self.perspective
        track.sublayerTransform = perspective
        belt.layer.addSublayer(track)

        pan.addTarget(self, action: #selector(panned))
        pan.maximumNumberOfTouches = 1
        pan.delegate = self
        belt.addGestureRecognizer(pan)

        // Below required, so the docked belt can collapse when its stack view hides it.
        let bottom = belt.bottomAnchor.constraint(equalTo: safeAreaLayoutGuide.bottomAnchor)
        bottom.priority = .init(999)
        NSLayoutConstraint.activate([
            belt.topAnchor.constraint(equalTo: drawer.bottomAnchor),
            belt.leadingAnchor.constraint(equalTo: leadingAnchor),
            belt.trailingAnchor.constraint(equalTo: trailingAnchor),
            belt.heightAnchor.constraint(equalToConstant: BeltGeometry.height),
            bottom,
        ])
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        let bounds = belt.bounds
        guard bounds.width > 0 else { return }
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        track.frame = bounds
        let ground = bounds.insetBy(dx: 0, dy: Self.strikerInset)
        strikerGround.frame = ground
        grit.frame = strikerGround.bounds
        grit.path = Self.gritPath(width: ground.width, height: ground.height)
        strikerTop.frame = CGRect(x: 0, y: 0, width: ground.width, height: 1)
        strikerBottom.frame = CGRect(x: 0, y: ground.height - 1, width: ground.width, height: 1)
        CATransaction.commit()
        if bounds.width != builtWidth { rebuild() }
    }

    /// The striker: `MatchBox`'s grit, strips 1.4 pt wide every 3 pt at 45°.
    private static func gritPath(width: CGFloat, height: CGFloat) -> CGPath {
        let path = CGMutablePath()
        var x = -height
        while x < width + height {
            path.move(to: CGPoint(x: x, y: height))
            path.addLine(to: CGPoint(x: x + height, y: 0))
            x += gritStep
        }
        return path
    }

    /// The layout's keys, at home.
    private func rebuild() {
        let width = belt.bounds.width
        guard width > 0 else { return }
        stopLink()
        runOffLeft = 0
        builtWidth = width
        let keys = Self.catalog.keys(layout, agent: agent, mine: Mine.load(from: .standard).keys)
        builtKeys = keys
        guard !keys.isEmpty else { return }
        let geometry = BeltGeometry(width: width)
        let motion = BeltMotion(keys: keys, geometry: geometry)
        let scale = max(traitCollection.displayScale, 2)

        CATransaction.begin()
        CATransaction.setDisableActions(true)
        pieces.forEach { $0.layer.removeFromSuperlayer() }
        pieces = motion.keys.map { key in
            let piece = Self.piece(for: key, width: geometry.width(of: key), scale: scale)
            track.addSublayer(piece.layer)
            return piece
        }
        CATransaction.commit()

        self.motion = motion
        spoken = nil
        place()
        updateName()
        UIAccessibility.post(notification: .layoutChanged, argument: nil)
    }

    private static func piece(for key: BeltKey, width: CGFloat, scale: CGFloat) -> Piece {
        let face = KeyFace(key, in: catalog)
        let bounds = CGRect(x: 0, y: 0, width: width, height: BeltGeometry.keyHeight)

        let layer = CALayer()
        layer.bounds = bounds
        layer.isDoubleSided = false
        layer.shadowColor = BeltLook.keyDrop.cgColor
        layer.shadowOpacity = 1
        layer.shadowRadius = 0
        layer.shadowOffset = CGSize(width: 0, height: 2)
        layer.shadowPath = UIBezierPath(roundedRect: bounds, cornerRadius: radius).cgPath

        let bevel = CAGradientLayer()
        bevel.frame = bounds
        bevel.cornerRadius = radius
        bevel.masksToBounds = true
        bevel.borderWidth = 1
        bevel.borderColor = BeltLook.keyEdge.cgColor
        layer.addSublayer(bevel)

        let topLine = CALayer()
        topLine.frame = CGRect(x: 1, y: 1, width: width - 2, height: 1)
        topLine.backgroundColor = BeltLook.keyTopLine.cgColor
        bevel.addSublayer(topLine)

        let labelFont = font(for: face.size)
        let text = CATextLayer()
        let lineHeight = ceil(labelFont.lineHeight)
        text.frame = CGRect(x: 0, y: ((BeltGeometry.keyHeight - lineHeight) / 2).rounded(), width: width, height: lineHeight)
        text.alignmentMode = .center
        text.truncationMode = .none
        text.contentsScale = scale
        bevel.addSublayer(text)

        if let mark = face.mark, let image = UIImage(named: "mark-\(mark)")?.cgImage {
            let markLayer = CALayer()
            markLayer.frame = CGRect(x: width - markInset - markSide, y: markInset, width: markSide, height: markSide)
            markLayer.contents = image
            markLayer.contentsGravity = .resizeAspect
            markLayer.contentsScale = scale
            bevel.addSublayer(markLayer)
        }

        let shade = CALayer()
        shade.frame = bounds
        shade.backgroundColor = UIColor.black.cgColor
        shade.opacity = 0
        bevel.addSublayer(shade)

        let piece = Piece(layer: layer, face: bevel, text: text, shade: shade, key: face, width: width)
        paint(piece, pressed: false)
        return piece
    }

    private static func font(for size: KeyFace.Size) -> UIFont {
        switch size {
        case .glyph: glyphFont
        case .big: bigFont
        case .word: wordFont
        }
    }

    /// A pressed key is `hair` with its label in `accent`.
    private static func paint(_ piece: Piece, pressed: Bool) {
        // No implicit fade: the press is 140 ms, shorter than a layer's default animation.
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        defer { CATransaction.commit() }
        let ground = pressed ? [BeltLook.hair, BeltLook.hair] : [BeltLook.keyBevelTop, BeltLook.keyBevelBottom]
        piece.face.colors = ground.map(\.cgColor)
        piece.text.string = NSAttributedString(string: piece.key.label, attributes: [
            .font: font(for: piece.key.size),
            .foregroundColor: pressed ? BeltLook.accent : BeltLook.fg,
        ])
    }

    // MARK: Placing

    /// Flat between the phone's sides; past a side a key folds round it and goes behind.
    private func place() {
        guard let motion else { return }
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        for (i, piece) in pieces.enumerated() {
            let fold = motion.fold(ofKeyAt: i)
            let layer = piece.layer
            layer.anchorPoint = CGPoint(x: fold.anchorLeft ? 0 : 1, y: 0.5)
            layer.position = CGPoint(
                x: fold.anchorLeft ? fold.x : fold.x + piece.width,
                y: Self.keyTop + BeltGeometry.keyHeight / 2
            )
            layer.transform = fold.angle == 0
                ? CATransform3DIdentity
                : CATransform3DMakeRotation(fold.angle * .pi / 180, 0, 1, 0)
            layer.opacity = fold.visible ? (piece.key.isCommand && !commandsLive ? Self.dim : 1) : 0
            piece.shade.opacity = Float(fold.shade)
        }
        CATransaction.commit()
    }

    private func updateName() {
        let mark = layout == .commands ? Self.catalog.mark(of: agent) : nil
        markView.image = mark.flatMap { UIImage(named: "mark-\($0)") }
        markView.isHidden = markView.image == nil
        setName(layout.rawValue.uppercased(), color: BeltLook.faint, kern: 0.8)
    }

    private func setName(_ name: String, color: UIColor, kern: CGFloat) {
        nameLabel.attributedText = NSAttributedString(string: name, attributes: [
            .font: Self.nameFont,
            .foregroundColor: color,
            .kern: kern,
        ])
    }

    /// A short line in the handle row, in place of the layout's name.
    private func flash(_ message: String) {
        flashes += 1
        let flash = flashes
        markView.isHidden = true
        setName(message, color: BeltLook.muted, kern: 0)
        UIAccessibility.post(notification: .announcement, argument: message)
        Task { [weak self] in
            try? await Task.sleep(for: .seconds(Self.flashTime))
            guard let self, self.flashes == flash else { return }
            self.updateName()
        }
    }

    // MARK: Touch

    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let first = touches.first, first.view === belt, touch == nil else {
            return super.touchesBegan(touches, with: event)
        }
        selection.prepare()
        impact.prepare()
        // A touch during a layout change ends it, and is no tap: the keys under it just came.
        let wasRunningOff = runOffLeft > 0
        if wasRunningOff { rebuild() }
        let wasAtRest = !wasRunningOff && (motion?.atRest ?? true)
        if !wasAtRest {
            // A touch stops a coast, as in a scroll view.
            stopLink()
            _ = motion?.drag(by: 0)
        }
        let start = first.location(in: belt)
        dragX = start.x
        touch = Touch(start: start, wasAtRest: wasAtRest)
    }

    override func touchesMoved(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let first = touches.first, first.view === belt, touch != nil else {
            return super.touchesMoved(touches, with: event)
        }
        measure(first)
    }

    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let first = touches.first, first.view === belt, touch != nil else {
            return super.touchesEnded(touches, with: event)
        }
        measure(first)
        if let touch, !touch.tap.isDrag, touch.wasAtRest, let i = keyIndex(at: first.location(in: belt)) {
            press(i)
        }
        touch = nil
        settleIfHeld()
    }

    /// The pan took the touch (it is a drag, and the pan settles the belt), or the system did.
    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard touches.first?.view === belt else {
            return super.touchesCancelled(touches, with: event)
        }
        touch = nil
        if pan.state != .began && pan.state != .changed { settleIfHeld() }
    }

    /// A tap is a touch that stays within 8 pt of where it went down, on either axis.
    private func measure(_ finger: UITouch) {
        guard var touch else { return }
        let p = finger.location(in: belt)
        let travel = max(abs(p.x - touch.start.x), abs(p.y - touch.start.y))
        if travel > touch.travel {
            touch.tap.moved(by: travel - touch.travel)
            touch.travel = travel
        }
        self.touch = touch
    }

    /// A belt a touch stopped between two detents settles on the nearest.
    private func settleIfHeld() {
        guard motion?.atRest == false, link == nil else { return }
        motion?.release(velocity: 0)
        startLink()
    }

    override func gestureRecognizerShouldBegin(_ gesture: UIGestureRecognizer) -> Bool {
        if gesture === pan {
            let v = pan.velocity(in: belt)
            return abs(v.x) > abs(v.y)
        }
        if gesture === pull {
            let v = pull.velocity(in: handle)
            return abs(v.y) > abs(v.x)
        }
        return super.gestureRecognizerShouldBegin(gesture)
    }

    /// The keyboard key is its own button: no touch on it opens the drawer. A tap opens it
    /// only on the grip or the name.
    func gestureRecognizer(_ gesture: UIGestureRecognizer, shouldReceive touch: UITouch) -> Bool {
        guard gesture.view === handle else { return true }
        if touch.view?.isDescendant(of: keyboardButton) ?? false { return false }
        guard gesture === handleTap else { return true }
        let p = touch.location(in: handle)
        let onGrip = abs(p.x - grip.center.x) <= Self.gripReach
        let onName = p.x <= nameStack.frame.maxX + Self.handlePadding
        return onGrip || onName
    }

    @objc private func panned(_ pan: UIPanGestureRecognizer) {
        switch pan.state {
        case .began, .changed:
            if pan.state == .began {
                stopLink()
                touch = nil
            }
            let x = pan.location(in: belt).x
            let ticks = motion?.drag(by: x - dragX) ?? []
            dragX = x
            place()
            play(ticks)
        case .ended, .cancelled, .failed:
            motion?.release(velocity: pan.state == .ended ? pan.velocity(in: belt).x : 0)
            startLink()
        default:
            break
        }
    }

    @objc private func handleTapped() {
        setTray(open: !trayOpen)
    }

    @objc private func pulled(_ pull: UIPanGestureRecognizer) {
        guard pull.state == .ended else { return }
        let dy = pull.translation(in: handle).y
        if dy < -Self.pullDistance {
            setTray(open: true)
        } else if dy > Self.pullDistance {
            setTray(open: false)
        }
    }

    /// The key under `point`; the gap between two keys goes half to each.
    private func keyIndex(at point: CGPoint) -> Int? {
        guard let motion else { return nil }
        let half = BeltGeometry.gap / 2
        return pieces.indices.first { i in
            let x0 = motion.x(ofKeyAt: i)
            let x1 = x0 + pieces[i].width
            return motion.fold(ofKeyAt: i).visible
                && point.x >= max(x0 - half, 0) && point.x < min(x1 + half, belt.bounds.width)
        }
    }

    private func press(_ i: Int) {
        guard let motion, pieces.indices.contains(i) else { return }
        let piece = pieces[i]
        if piece.key.isCommand && !commandsLive {
            flash("commands run from the chat")
            return
        }
        guard let bytes = KeyBar.bytes(for: motion.keys[i].id) else { return }
        Self.paint(piece, pressed: true)
        Task {
            try? await Task.sleep(for: .seconds(Self.pressTime))
            Self.paint(piece, pressed: false)
        }
        onSend(bytes)
    }

    // MARK: Coast, settle and run-off

    private func startLink() {
        guard link == nil else { return }
        let link = CADisplayLink(target: self, selector: #selector(stepped))
        link.preferredFrameRateRange = CAFrameRateRange(minimum: 60, maximum: 120, preferred: 120)
        link.add(to: .main, forMode: .common)
        self.link = link
        lastFrame = nil
    }

    /// The display link holds the view until it stops.
    private func stopLink() {
        link?.invalidate()
        link = nil
        lastFrame = nil
    }

    @objc private func stepped(_ link: CADisplayLink) {
        let dt = lastFrame.map { link.timestamp - $0 } ?? link.duration
        lastFrame = link.timestamp
        if runOffLeft > 0 {
            let frames = CGFloat(dt / BeltMotion.frame)
            let ticks = motion?.drag(by: -Self.runOffStep * frames) ?? []
            place()
            play(ticks.filter { $0 == .detent })
            runOffLeft -= frames
            if runOffLeft <= 0 { rebuild() }
            return
        }
        let ticks = motion?.step(dt: dt) ?? []
        place()
        play(ticks)
        if motion?.atRest ?? true {
            stopLink()
            spoken = nil
            UIAccessibility.post(notification: .layoutChanged, argument: nil)
        }
    }

    /// The old keys run off a few detents, clicking, then the new set settles on its home.
    private func runOff() {
        guard window != nil, !isHidden, motion != nil else { return rebuild() }
        touch = nil
        selection.prepare()
        stopLink()
        runOffLeft = Self.runOffFrames
        startLink()
    }

    /// One click a frame at most: home is the firmer one.
    private func play(_ ticks: [BeltTick]) {
        guard window != nil, !isHidden, !ticks.isEmpty else { return }
        if ticks.contains(.home) {
            impact.impactOccurred()
            impact.prepare()
        } else {
            selection.selectionChanged()
            selection.prepare()
        }
    }

    override func willMove(toWindow newWindow: UIWindow?) {
        super.willMove(toWindow: newWindow)
        guard newWindow == nil else { return }
        setTray(open: false, animated: false)
        stopLink()
        if runOffLeft > 0 { rebuild() }
        // A belt that leaves the screen mid-coast rests on its nearest detent at once.
        guard motion?.atRest == false else { return }
        motion?.release(velocity: 0)
        for _ in 0..<3 where motion?.atRest == false {
            _ = motion?.step(dt: 1)
        }
        place()
    }

    // MARK: The matchbox drawer

    /// Opens or closes the drawer. A closed box comes back fresh: every match ready again but
    /// the one in use.
    func setTray(open: Bool, animated: Bool = true) {
        trayMoves += 1
        guard open != trayOpen else { return }
        trayOpen = open
        let moves = trayMoves
        if open {
            trayModel.freshBox(inUse: layout)
            drawer.isHidden = false
        }
        spoken = nil
        drawerHeight.constant = open ? Self.drawerOpen : 0
        invalidateIntrinsicContentSize()

        let tray: UIView = trayHost.view
        let slid = open ? .identity : CGAffineTransform(translationX: 0, y: MatchTray.slide)
        let host = superview ?? self
        let finish = { [weak self] in
            guard let self, !open, self.trayMoves == moves else { return }
            self.drawer.isHidden = true
            self.trayModel.freshBox(inUse: self.layout)
        }
        defer { UIAccessibility.post(notification: .layoutChanged, argument: nil) }
        guard animated, window != nil, !UIAccessibility.isReduceMotionEnabled else {
            tray.transform = slid
            host.layoutIfNeeded()
            finish()
            return
        }
        let grow = UIViewPropertyAnimator(duration: Self.drawerGrowTime, timingParameters: Self.drawerCurve())
        grow.addAnimations { host.layoutIfNeeded() }
        let slide = UIViewPropertyAnimator(duration: Self.traySlideTime, timingParameters: Self.drawerCurve())
        slide.addAnimations { tray.transform = slid }
        slide.addCompletion { _ in finish() }
        grow.startAnimation()
        slide.startAnimation()
    }

    /// The mockup's drawer curve: quick out of the sleeve, soft at the end.
    private static func drawerCurve() -> UICubicTimingParameters {
        UICubicTimingParameters(controlPoint1: CGPoint(x: 0.3, y: 0.7), controlPoint2: CGPoint(x: 0.2, y: 1))
    }

    /// A picked match strikes, the belt runs to its layout, and the tray closes once the strike
    /// shows.
    private func picked(_ picked: BeltLayout) {
        layout = picked
        onLayout(picked)
        trayMoves += 1
        let moves = trayMoves
        Task { [weak self] in
            try? await Task.sleep(for: .seconds(MatchTray.closeDelay))
            guard let self, self.trayMoves == moves else { return }
            self.setTray(open: false)
        }
    }

    private func editMine() {
        setTray(open: false)
        onEditMine()
    }

    /// The editor closed. Done shows Mine; a Mine on the belt runs to its new keys.
    func mineEdited(show: Bool) {
        if show && layout != .mine {
            layout = .mine
        } else if layout == .mine,
                  Self.catalog.keys(.mine, agent: agent, mine: Mine.load(from: .standard).keys) != builtKeys {
            runOff()
        }
    }

    // MARK: The other belt

    /// Rests where `other` rests, so the belt holds still when the keyboard comes or goes.
    func follow(_ other: BeltView) {
        guard layout == other.layout, agent == other.agent, let theirs = other.motion,
              motion?.ring == theirs.ring else { return }
        motion?.offset = theirs.atRest ? theirs.offset : theirs.settleTarget()
        place()
    }

    // MARK: Accessibility

    private final class Element: UIAccessibilityElement {
        var activate: () -> Bool = { false }
        override func accessibilityActivate() -> Bool { activate() }
    }

    override var accessibilityElements: [Any]? {
        get {
            if let spoken { return spoken }
            let name = Element(accessibilityContainer: self)
            name.accessibilityLabel = "Layouts, \(layout.rawValue)"
            name.accessibilityTraits = .button
            name.accessibilityFrameInContainerSpace = CGRect(x: 0, y: 0, width: grip.frame.maxX, height: Self.handleHeight)
            name.accessibilityValue = trayOpen ? "open" : nil
            name.activate = { [weak self] in
                guard let self else { return false }
                self.setTray(open: !self.trayOpen)
                return true
            }
            var elements: [Any] = [name, keyboardButton]
            if trayOpen { elements.append(trayHost.view as Any) }
            guard let motion else {
                spoken = elements
                return elements
            }
            for (i, piece) in pieces.enumerated() {
                let x0 = motion.x(ofKeyAt: i)
                guard x0 >= 0, x0 + piece.width <= belt.bounds.width else { continue }
                let key = Element(accessibilityContainer: self)
                key.accessibilityLabel = piece.key.spoken
                key.accessibilityTraits = piece.key.isCommand && !commandsLive ? [.button, .notEnabled] : .button
                key.accessibilityFrameInContainerSpace = CGRect(
                    x: x0, y: belt.frame.minY + Self.keyTop, width: piece.width, height: BeltGeometry.keyHeight)
                // A key element outlives its key when the belt rebuilds: it finds its key by layer.
                let layer = piece.layer
                key.activate = { [weak self] in
                    guard let self, let i = self.pieces.firstIndex(where: { $0.layer === layer }) else { return false }
                    self.press(i)
                    return true
                }
                elements.append(key)
            }
            spoken = elements
            return elements
        }
        set {}
    }
}
