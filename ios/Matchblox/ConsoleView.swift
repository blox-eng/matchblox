import LocalAuthentication
import MatchbloxKit
import NIOSSH
import SwiftTerm
import SwiftUI
import UniformTypeIdentifiers
import os

private let consoleLog = Logger(subsystem: "matchblox", category: "console")

/// Where the console's bytes come from and go to. Each source's output comes from its own start.
protocol ConsoleSource: Sendable {
    func write(_ data: Data) async
    func resize(cols: Int, rows: Int) async
    func close() async
}

extension SSHSession: ConsoleSource {}

/// The console window: `matchblox console` on the host, drawn by SwiftTerm.
/// A failed connect goes to `onFailure`; the router shows the screen that fixes it.
struct ConsoleView: View {
    let host: Host
    let jump: SessionJump?
    let onHosts: (() -> Void)?
    let onScanAgain: () -> Void

    @State private var connection: ConsoleConnection
    @Environment(\.scenePhase) private var scenePhase

    init(
        host: Host,
        open: Int? = nil,
        jump: SessionJump? = nil,
        onFailure: @escaping (ConnectFailure) -> Void,
        onHosts: (() -> Void)?,
        onScanAgain: @escaping () -> Void
    ) {
        self.host = host
        self.jump = jump
        self.onHosts = onHosts
        self.onScanAgain = onScanAgain
        _connection = State(initialValue: ConsoleConnection(host: host, open: open, onFailure: onFailure))
    }

    var body: some View {
        ZStack {
            TerminalRepresentable(connection: connection, onHosts: onHosts)
            if connection.showsCover {
                cover
            } else if let failure = connection.keptFailure {
                keptFailure(failure)
            } else if connection.reconnecting {
                BreathingLine()
            }
        }
        .sheet(isPresented: Binding(get: { connection.invite }, set: { connection.invite = $0 })) {
            InviteSheet(
                onPair: {
                    connection.invite = false
                    onScanAgain()
                },
                onKeepLooking: { connection.invite = false }
            )
        }
        .onAppear { connection.start() }
        .onDisappear { connection.stop() }
        .onChange(of: jump) { _, jump in
            if let jump { connection.jump(to: jump.session) }
        }
        .onChange(of: scenePhase) { _, phase in
            switch phase {
            case .background: connection.paused()
            case .active: connection.resumed()
            default: break
            }
        }
    }

    private var cover: some View {
        VStack(alignment: .leading, spacing: 20) {
            switch connection.phase {
            case .connecting, .live:
                Text("Connecting to \(host.label)")
                    .foregroundStyle(Theme.muted)
                hostsButton
            case .failed(let message, let scanAgain):
                Text(message)
                    .foregroundStyle(Theme.neg)
                HStack(spacing: 24) {
                    if scanAgain {
                        Button("Scan again", action: onScanAgain)
                            .buttonStyle(.borderedProminent)
                            .foregroundStyle(Theme.bg)
                    } else {
                        Button("Try again") { connection.retry() }
                            .buttonStyle(.borderedProminent)
                            .foregroundStyle(Theme.bg)
                    }
                    hostsButton
                }
            case .ended:
                Text(Opening.endedMessage(host: host.label))
                    .foregroundStyle(Theme.muted)
                if host.isDemo {
                    ScrollView {
                        VStack(alignment: .leading, spacing: 12) { demoEndedActions }
                    }
                    .scrollBounceBehavior(.basedOnSize)
                } else {
                    HStack(spacing: 24) {
                        openAgainButton
                        hostsButton
                    }
                }
            }
            Spacer()
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .padding(24)
        .background(Theme.bg)
    }

    /// Past the quiet retries the kept screen says what happened, over its top: no new page.
    private func keptFailure(_ message: String) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(message)
                .foregroundStyle(Theme.fg)
            HStack(spacing: 24) {
                Button("Try again") { connection.retry() }
                    .buttonStyle(.borderedProminent)
                    .foregroundStyle(Theme.bg)
                hostsButton
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(Theme.wash)
        .overlay(alignment: .bottom) { Rectangle().fill(Theme.hair).frame(height: 1) }
        .frame(maxHeight: .infinity, alignment: .top)
    }

    private var openAgainButton: some View {
        Button("Open again") { connection.retry() }
            .buttonStyle(.borderedProminent)
            .foregroundStyle(Theme.bg)
    }

    /// The demo has no real host to come back to, so the way on is to pair one.
    @ViewBuilder
    private var demoEndedActions: some View {
        InstallLine()
        SendToComputerButton()
        Button { connection.retry() } label: {
            Text("Open again").frame(maxWidth: .infinity, minHeight: 44)
        }
        .buttonStyle(.bordered)
        .foregroundStyle(Theme.fg)
        Button(action: onScanAgain) {
            Text("Pair a host").frame(maxWidth: .infinity, minHeight: 44)
        }
        .buttonStyle(.bordered)
        .foregroundStyle(Theme.fg)
        hostsButton
    }

    /// Hosts (whenever a host is paired, so the only one can be removed too).
    /// Live Activity entry is on home only (design review 1.2).
    @ViewBuilder
    private var hostsButton: some View {
        if let onHosts {
            Button("Hosts", action: onHosts)
                .foregroundStyle(Theme.muted)
        }
    }
}

/// The terminal, with the key belt above the keyboard. An input accessory shows only while its
/// view has the keyboard, so a second belt stays under the terminal while the keyboard is down.
private struct TerminalRepresentable: UIViewRepresentable {
    let connection: ConsoleConnection
    var onHosts: (() -> Void)?

    func makeUIView(context: Context) -> UIStackView {
        let terminal = ConsoleTerminalView(textSize: TextSize.load(from: .standard))
        terminal.onHome = { [weak connection] in connection?.goHome() }
        let bg = UIColor(Theme.bg)
        terminal.backgroundColor = bg
        terminal.nativeBackgroundColor = bg
        terminal.nativeForegroundColor = UIColor(Theme.fg)
        terminal.caretColor = UIColor(Theme.accent)
        terminal.keyboardAppearance = .dark
        terminal.autocorrectionType = .no
        terminal.spellCheckingType = .no
        terminal.smartQuotesType = .no
        terminal.smartDashesType = .no
        terminal.smartInsertDeleteType = .no
        terminal.autocapitalizationType = .none
        terminal.inlinePredictionType = .no
        terminal.contentInsetAdjustmentBehavior = .never
        terminal.terminalDelegate = connection
        connection.terminal = terminal

        let screen = HeldScreen(terminal: terminal)
        screen.backgroundColor = bg
        let belts = Belts(terminal: terminal, screen: screen, isDemo: connection.isDemo)
        belts.onHosts = onHosts
        terminal.inputAccessoryView = belts.lifted
        terminal.tapRaisesKeyboard = !connection.isDemo
        terminal.onFocus = { [weak belts] focused in belts?.focusChanged(focused) }
        terminal.onScrolledBack = { [weak screen] up in screen?.showJump(up) }
        screen.onJump = { [weak terminal] in terminal?.jumpToBottom() }
        connection.onCopied = { [weak screen] word, ok in screen?.copied(word, ok: ok) }
        terminal.onAsksCopy = { [weak connection] in connection?.copyAsked() }
        terminal.onLink = { [weak terminal] link, spot in
            if let terminal { LinkSheet.present(link, from: terminal, at: spot) }
        }
        terminal.onSelectedLink = { [weak screen] link in screen?.showLink(link) }
        screen.onOpenLink = { [weak terminal] link, spot in
            if let terminal { LinkSheet.present(link, from: terminal, at: spot) }
        }
        connection.onAgent = { agent, pane in belts.agentChanged(agent, pane: pane) }
        connection.onCommand = { command in belts.head.command = command }
        belts.head.onCard = { [weak screen] card in screen?.showStep(card) }
        belts.head.onSessions = { [weak screen] card in screen?.showSessions(card) }
        belts.head.opened = { [weak connection] in connection?.opened }
        belts.head.openSession = { [weak connection] n in connection?.jump(to: n) }
        connection.onBoard = { value in belts.boardSaid(value) }
        connection.onBecameLive = { [weak belts] in belts?.teachBoxIfNeeded() }
        belts.onAnswer = { [weak connection] in connection?.answerBoard() }
        belts.onPaneSeen = { [weak connection] inPane in connection?.paneSeen(inPane) }
        #if DEBUG
        let isDemo = connection.isDemo
        // The UI test's way to a real host's first tap, on the demo's screens.
        if isDemo, ProcessInfo.processInfo.arguments.contains("-demoTapRaisesKeyboard") {
            terminal.tapRaisesKeyboard = true
        }
        // Only the UI test reads this, and only on the demo.
        if isDemo { terminal.accessibilityIdentifier = "demo-terminal" }
        connection.onOutput = { [weak terminal] in
            belts.outputArrived()
            if isDemo { terminal?.accessibilityLabel = terminal?.screenText }
        }
        #else
        connection.onOutput = { belts.outputArrived() }
        #endif
        let stack = UIStackView(arrangedSubviews: [screen, belts.dock])
        stack.axis = .vertical
        return stack
    }

    func updateUIView(_ stack: UIStackView, context: Context) {}
}

/// A tapped link asks first: `Open in Safari` (http and https only), `Copy link`, `Cancel`.
@MainActor
private enum LinkSheet {
    static func present(_ link: String, from view: UIView, at spot: CGRect) {
        guard var top = view.window?.rootViewController else { return }
        while let next = top.presentedViewController { top = next }
        let sheet = UIAlertController(title: link, message: nil, preferredStyle: .actionSheet)
        if TerminalLink.opens(link), let url = URL(string: link) {
            sheet.addAction(UIAlertAction(title: "Open in Safari", style: .default) { _ in
                UIApplication.shared.open(url)
            })
        }
        sheet.addAction(UIAlertAction(title: "Copy link", style: .default) { _ in
            UIPasteboard.general.string = link
        })
        sheet.addAction(UIAlertAction(title: "Cancel", style: .cancel))
        // An iPad shows the sheet as a popover at the tap.
        sheet.popoverPresentationController?.sourceView = view
        sheet.popoverPresentationController?.sourceRect = spot
        top.present(sheet, animated: true)
    }
}

/// The terminal's place on screen. While a matchbox tray is open the place gets shorter, but
/// the terminal keeps its size (`ScreenHold`): its lines slide up and the top ones go under the
/// edge. SwiftTerm resizes only when its bounds change, so the host gets no resize.
private final class HeldScreen: UIView {
    private static let jumpSide: CGFloat = 44
    private static let jumpInset: CGFloat = 12
    private static let noticeHeight: CGFloat = 32
    private static let noticeTime: TimeInterval = 1.2

    private let terminal: UIView
    private var hold = ScreenHold()
    private var heldSize: ScreenHold.Size {
        ScreenHold.Size(width: bounds.width, height: bounds.height)
    }
    /// Shows while the terminal is up from its live bottom; a tap goes back down.
    private let jump = UIButton(type: .system)
    var onJump: @MainActor () -> Void = {}
    /// Shows while the selection holds a URL; a tap offers it whole, as a tap on a link does.
    private let openLink = UIButton(type: .system)
    private var selectedLink: String?
    var onOpenLink: @MainActor (String, CGRect) -> Void = { _, _ in }
    /// A short word over the terminal that fades: `Copied`.
    private let notice = UILabel()
    /// The agent step a fast roll of the head reads past, at the top of the view.
    private let stepCard = UILabel()
    /// The sessions a hold on the head shows, the current one marked.
    private let sessionsCard = UILabel()
    private let copiedHaptic = UINotificationFeedbackGenerator()

    init(terminal: UIView) {
        self.terminal = terminal
        super.init(frame: .zero)
        clipsToBounds = true
        addSubview(terminal)

        var look = UIButton.Configuration.filled()
        look.title = "↓"
        look.cornerStyle = .capsule
        look.baseBackgroundColor = UIColor(Theme.hair)
        look.baseForegroundColor = UIColor(Theme.fg)
        jump.configuration = look
        jump.accessibilityLabel = "Jump to the bottom"
        jump.isHidden = true
        jump.addAction(UIAction { [weak self] _ in self?.onJump() }, for: .primaryActionTriggered)
        addSubview(jump)

        var linkLook = look
        linkLook.title = "Open link"
        openLink.configuration = linkLook
        openLink.isHidden = true
        openLink.addAction(UIAction { [weak self] _ in
            guard let self, let link = self.selectedLink else { return }
            self.onOpenLink(link, self.openLink.frame)
        }, for: .primaryActionTriggered)
        addSubview(openLink)

        notice.font = UIFont.monospacedSystemFont(ofSize: 13, weight: .medium)
        notice.textColor = UIColor(Theme.fg)
        notice.backgroundColor = UIColor(Theme.hair)
        notice.textAlignment = .center
        notice.layer.cornerRadius = Self.noticeHeight / 2
        notice.layer.masksToBounds = true
        notice.alpha = 0
        addSubview(notice)

        stepCard.font = UIFont.monospacedSystemFont(ofSize: 12, weight: .medium)
        stepCard.textColor = UIColor(Theme.fg)
        stepCard.backgroundColor = UIColor(Theme.hair)
        stepCard.lineBreakMode = .byTruncatingMiddle
        stepCard.layer.cornerRadius = 8
        stepCard.layer.masksToBounds = true
        stepCard.accessibilityIdentifier = "step-card"
        stepCard.isHidden = true
        addSubview(stepCard)

        sessionsCard.font = UIFont.monospacedSystemFont(ofSize: 13, weight: .medium)
        sessionsCard.textColor = UIColor(Theme.fg)
        sessionsCard.backgroundColor = UIColor(Theme.hair)
        sessionsCard.numberOfLines = 0
        sessionsCard.lineBreakMode = .byTruncatingTail
        sessionsCard.layer.cornerRadius = 8
        sessionsCard.layer.masksToBounds = true
        sessionsCard.accessibilityIdentifier = "sessions-card"
        sessionsCard.isHidden = true
        addSubview(sessionsCard)
    }

    required init?(coder: NSCoder) { nil }

    func showJump(_ shows: Bool) {
        jump.isHidden = !shows
    }

    func showLink(_ link: String?) {
        selectedLink = link
        openLink.isHidden = link == nil
        setNeedsLayout()
    }

    /// What came of the host's copy: `Copied`, or why it did not.
    func copied(_ word: String, ok: Bool) {
        copiedHaptic.notificationOccurred(ok ? .success : .error)
        say(word)
    }

    /// A short word over the terminal that fades.
    func say(_ word: String) {
        notice.text = word
        notice.layer.removeAllAnimations()
        notice.alpha = 1
        setNeedsLayout()
        UIView.animate(withDuration: 0.3, delay: Self.noticeTime, options: [], animations: { [weak self] in
            self?.notice.alpha = 0
        })
        UIAccessibility.post(notification: .announcement, argument: word)
    }

    /// The step card's text, or nil: the card goes.
    func showStep(_ card: String?) {
        stepCard.text = card.map { "  \($0)  " }
        stepCard.isHidden = card == nil
        setNeedsLayout()
    }

    /// The sessions card's lines, or nil: the card goes.
    func showSessions(_ card: String?) {
        sessionsCard.text = card.map { $0.split(separator: "\n", omittingEmptySubsequences: false).map { "  \($0)  " }.joined(separator: "\n") }
        sessionsCard.isHidden = card == nil
        setNeedsLayout()
    }

    func trayMoved(open: Bool) {
        if open {
            hold.open(screen: heldSize, drawer: BeltView.drawerOpen)
        } else {
            hold.close()
        }
        setNeedsLayout()
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        let height = hold.height(screen: heldSize)
        terminal.frame = CGRect(x: 0, y: bounds.height - height, width: bounds.width, height: height)
        let side = Self.jumpSide
        jump.frame = CGRect(
            x: bounds.width - side - Self.jumpInset,
            y: bounds.height - side - Self.jumpInset,
            width: side,
            height: side
        )
        openLink.frame = CGRect(
            x: Self.jumpInset,
            y: jump.frame.minY,
            width: openLink.intrinsicContentSize.width,
            height: side
        )
        stepCard.frame = CGRect(x: Self.jumpInset, y: Self.jumpInset, width: bounds.width - 2 * Self.jumpInset, height: Self.noticeHeight)
        let cardWidth = bounds.width - 2 * Self.jumpInset
        let cardHeight = min(sessionsCard.sizeThatFits(CGSize(width: cardWidth, height: .greatestFiniteMagnitude)).height + 16,
                             max(bounds.height - 2 * Self.jumpInset, 0))
        sessionsCard.frame = CGRect(x: Self.jumpInset, y: Self.jumpInset, width: cardWidth, height: cardHeight)
        let width = notice.intrinsicContentSize.width + 2 * Self.jumpInset
        notice.frame = CGRect(
            x: ((bounds.width - width) / 2).rounded(),
            y: jump.frame.midY - Self.noticeHeight / 2,
            width: width,
            height: Self.noticeHeight
        )
    }
}

/// The two belts, the one on the keyboard and the one docked under the terminal, kept as one:
/// the same agent, the same live commands, and the same resting place.
@MainActor
private final class Belts {
    let lifted = BeltView()
    let dock = BeltView()
    /// The match head rides on the dock only: under the keyboard it is out of reach.
    let head = DockHeadView()
    private weak var terminal: ConsoleTerminalView?
    private var agent: String?
    /// The pane of the last `agent=…;pane=` signal, while an agent's pane shows.
    private var agentPane: String?
    /// The Mine editor while it shows: never a second one over it.
    private weak var editor: UIViewController?
    /// A live check waits for the end of this run-loop turn: one for every read of the turn.
    private var liveCheckQueued = false
    /// Commands go live only once the screen has stayed the agent's pane this long.
    private static let liveDelay: Duration = .milliseconds(300)
    private var goingLive: Task<Void, Never>?
    /// Whether the terminal shows a pane's way back, at each live check.
    var onPaneSeen: @MainActor (Bool) -> Void = { _ in }
    /// The belts follow the console's board.
    private var board = BoardFollow()
    /// Sends the app's answer to the console's board (`BoardBelt.answer`).
    var onAnswer: @MainActor () -> Void = {}
    /// Opens Pocket home from the `BOX` stamp.
    var onHosts: (() -> Void)?

    /// Teach BOX on the belt that is visible (dock when the keyboard is down).
    func teachBoxIfNeeded() {
        let visible = dock.isHidden ? lifted : dock
        visible.teachBoxIfNeeded()
    }

    init(terminal: ConsoleTerminalView, screen: HeldScreen, isDemo: Bool) {
        self.terminal = terminal
        for belt in [lifted, dock] {
            belt.isDemo = isDemo
            belt.onScreenHold = { [weak screen] holding in screen?.trayMoved(open: holding) }
            belt.onSend = { [weak self] bytes in self?.send(bytes) }
            belt.onCtrl = { [weak self, weak terminal] in
                guard let terminal else { return }
                terminal.controlModifier.toggle()
                self?.ctrlChanged(terminal.controlModifier)
            }
            belt.onKeyboard = { [weak terminal] in
                guard let terminal else { return }
                _ = terminal.isFirstResponder ? terminal.resignFirstResponder() : terminal.becomeFirstResponder()
            }
            belt.onLayout = { [weak self] layout in self?.layoutPicked(layout) }
            belt.onEditMine = { [weak self] in self?.editMine() }
            belt.onBox = { [weak self] in self?.onHosts?() }
        }
        dock.carry(head)
        head.onSend = { [weak self] bytes in self?.send(bytes) }
        head.onLines = { [weak terminal] lines in terminal?.head(lines: lines) ?? .nothing }
        head.route = { [weak terminal] older in terminal?.headRoute(older: older) ?? .nothing }
        head.onKeyboard = { [weak terminal] in _ = terminal?.becomeFirstResponder() }
        head.onHighlight = { [weak terminal] text in terminal?.highlightRow(showing: text) }
        head.visibleLines = { [weak terminal] in terminal?.visibleLines ?? [] }
        terminal.onReturnSent = { [weak self] in self?.foldKeyboard() }
        ctrlObserver = NotificationCenter.default.addObserver(forName: Self.ctrlReset, object: terminal, queue: .main) {
            [weak self] _ in MainActor.assumeIsolated { self?.ctrlChanged(false) }
        }
    }

    private static let copyReply = KeyBar.bytes(for: "board.c")
    private static let esc = KeyBar.bytes(for: "Esc")
    private static let enter = KeyBar.bytes(for: "⏎")

    /// SwiftTerm disarms ctrl after the next typed key and posts this; its name is not public.
    private static let ctrlReset = Notification.Name("SwiftTerm.TerminalView.controlModifierReset")
    nonisolated(unsafe) private var ctrlObserver: NSObjectProtocol?

    deinit {
        if let ctrlObserver { NotificationCenter.default.removeObserver(ctrlObserver) }
    }

    private func ctrlChanged(_ armed: Bool) {
        lifted.ctrlArmed = armed
        dock.ctrlArmed = armed
    }

    /// The agent of the pane the app jumped into, and that pane, where the head reads its menu.
    func agentChanged(_ agent: String?, pane: String?) {
        self.agent = agent
        agentPane = agent == nil ? nil : pane
        lifted.agent = agent
        dock.agent = agent
        headFollows()
        outputArrived()
    }

    /// The head reads the pane the app jumped into, else the pane of the chat the console shows.
    private func headFollows() {
        head.pane = agentPane ?? board.belt.chatPane
    }

    /// A command types into a known agent's pane: never with no agent, never on the console.
    /// The screen is read once for all the reads of a run-loop turn.
    func outputArrived() {
        guard !liveCheckQueued else { return }
        liveCheckQueued = true
        Task { [weak self] in
            guard let self else { return }
            self.liveCheckQueued = false
            self.checkLive()
        }
    }

    private var paneShows: Bool { !(agent ?? "").isEmpty && terminal?.inTmuxPane == true }

    /// Off at once; on only once the pane has shown for `liveDelay`, so a screen that flickers
    /// past a pane never lights the keys.
    private func checkLive() {
        let inPane = terminal?.inTmuxPane == true
        if !inPane { terminal?.forgetScrollBack() }
        onPaneSeen(inPane)
        let layout = board.pane(inPane, on: lifted.layout)
        followBoard(turn: layout)
        if layout == .board { answerBoard() }
        guard paneShows else {
            goingLive?.cancel()
            goingLive = nil
            setCommandsLive(false)
            return
        }
        guard !lifted.commandsLive, goingLive == nil else { return }
        goingLive = Task { [weak self] in
            try? await Task.sleep(for: Self.liveDelay)
            guard let self, !Task.isCancelled else { return }
            self.goingLive = nil
            if self.paneShows { self.setCommandsLive(true) }
        }
    }

    private func setCommandsLive(_ live: Bool) {
        lifted.commandsLive = live
        dock.commandsLive = live
    }

    /// The keyboard key says what a tap does: a focused terminal has its keys, the system
    /// keyboard's or a hardware keyboard's, and a tap hides them.
    func focusChanged(_ focused: Bool) {
        lifted.keyboardUp = focused
        dock.isHidden = focused
        if focused {
            dock.setTray(open: false, animated: false)
            lifted.follow(dock)
        } else {
            dock.follow(lifted)
        }
    }

    /// A belt key, or the match head, reaches the terminal.
    private func send(_ bytes: [UInt8]) {
        guard let terminal else { return }
        // A pane scrolled up is in tmux copy mode, which would take the key as its own
        // command: the steps back down leave it first. Esc there only comes back down:
        // the agent would read it as an interrupt.
        let wasUp = terminal.isUpInPane
        terminal.jumpToBottom()
        if wasUp, bytes == Self.esc { return }
        if bytes == Self.copyReply { terminal.onAsksCopy() }
        terminal.send(bytes)
        if bytes == Self.enter, terminal.isFirstResponder { foldKeyboard() }
    }

    /// An Enter typed or sent from the lifted belt has gone: the keyboard folds and the head is
    /// back. It folds on the next turn of the run loop, never inside the keyboard's own call.
    private func foldKeyboard() {
        Task { [weak self] in _ = self?.terminal?.resignFirstResponder() }
    }

    /// A layout picked on one belt is the layout of both. A pick of Board while the console
    /// shows the board answers the last value the console sent.
    private func layoutPicked(_ layout: BeltLayout) {
        lifted.layout = layout
        dock.layout = layout
        board.picked()
        answerBoard()
    }

    /// The console's board signal. The belts turn to Board when the console starts to show the
    /// board and back when it leaves, and they answer each value while they show Board, also a
    /// value they already had: the console counts the answers to know the app is Pocket.
    func boardSaid(_ value: String) {
        followBoard(turn: board.heard(value, on: lifted.layout))
        if !value.isEmpty { answerBoard() }
    }

    /// The belts and the terminal's touches as the board is now. A belt that leaves Board turns
    /// before its keys change, so it never shows the keys of no board.
    private func followBoard(turn layout: BeltLayout?) {
        if let layout, layout != .board { turn(to: layout) }
        lifted.board = board.belt
        dock.board = board.belt
        headFollows()
        terminal?.showsConsole = board.opensMenu
        if layout == .board { turn(to: .board) }
    }

    private func turn(to layout: BeltLayout) {
        lifted.turn(to: layout)
        dock.turn(to: layout)
    }

    private func answerBoard() {
        if board.answers(on: lifted.layout) { onAnswer() }
    }

    /// The Mine editor, as a sheet over the console.
    private func editMine() {
        guard editor?.presentingViewController == nil, var top = terminal?.window?.rootViewController else { return }
        while let next = top.presentedViewController { top = next }
        let editor = UIHostingController(rootView: BeltEditor(
            catalog: BeltView.catalog,
            dismiss: { [weak top] in top?.dismiss(animated: true) },
            closed: { [weak self] done in self?.mineEdited(show: done) }
        ))
        editor.view.backgroundColor = BeltLook.sheet
        if let sheet = editor.sheetPresentationController {
            let part = UISheetPresentationController.Detent.Identifier("mine")
            sheet.detents = [.custom(identifier: part) { $0.maximumDetentValue * 0.7 }, .large()]
            // The console stays bright under the sheet at its first detent, as the mockup shows.
            sheet.largestUndimmedDetentIdentifier = part
            sheet.preferredCornerRadius = 16
        }
        self.editor = editor
        top.present(editor, animated: true)
    }

    private func mineEdited(show: Bool) {
        lifted.mineEdited(show: show)
        dock.mineEdited(show: show)
    }

}

/// Owns one SSHSession for each connect. A session that ends at once opens again; a good session that ends waits for a tap. It stops when the view goes away or the app goes to the background. The demo host has a DemoSession instead, and each opening starts it fresh.
@MainActor
@Observable
final class ConsoleConnection {
    enum Phase: Equatable {
        case connecting
        case live
        case ended
        case failed(String, scanAgain: Bool)
    }

    private enum Outgoing: Sendable {
        case input(Data)
        case resize(cols: Int, rows: Int)
    }

    private enum Ending {
        case failed(ConnectFailure)
        case ended(sawOutput: Bool)
    }

    private struct UnlockFailed: Error {
        let message: String
    }

    private(set) var phase = Phase.connecting
    /// The invite shows over the console. It turns true once for each opening of the demo.
    var invite = false

    @ObservationIgnored weak var terminal: ConsoleTerminalView?
    /// The agent of the pane this app jumped into, or nil for none. A new session starts with none.
    @ObservationIgnored var onAgent: @MainActor (_ agent: String?, _ pane: String?) -> Void = { _, _ in }
    /// How Pocket runs its matchblox commands on this host, once it can.
    @ObservationIgnored var onCommand: @MainActor (HostCommand) -> Void = { _ in }
    /// Each value of the console's board signal; "" when it left the board. A new session
    /// starts off the board.
    @ObservationIgnored var onBoard: @MainActor (String) -> Void = { _ in }
    /// The console announced that it reads the app's marks; an older console never does.
    @ObservationIgnored private var consoleMarks = false
    @ObservationIgnored private var paneAgent = PaneAgent()
    /// A second look at the screen once the way back has stayed away for `PaneAgent.settle`.
    @ObservationIgnored private var settling: Task<Void, Never>?
    /// Called each time a read has reached the terminal.
    @ObservationIgnored var onOutput: @MainActor () -> Void = {}
    /// Called with what came of each copy the person asked for: `Copied`, or why it did not.
    @ObservationIgnored var onCopied: @MainActor (String, Bool) -> Void = { _, _ in }
    /// BOX teach once the session is live (visible belt).
    @ObservationIgnored var onBecameLive: @MainActor () -> Void = {}
    @ObservationIgnored private var copyGate = CopyGate()
    @ObservationIgnored private var task: Task<Void, Never>?
    @ObservationIgnored private var outgoing: AsyncStream<Outgoing>.Continuation?
    /// One unlock serves every reconnect, and a return to the app inside `UnlockGrace.window`.
    @ObservationIgnored private var unlocked: LAContext?
    @ObservationIgnored private var grace = UnlockGrace()
    /// Keys typed on the kept screen while it connects again.
    @ObservationIgnored private var held = HeldKeys()
    /// True once the console was live: a reconnect then keeps the last screen, with no cover.
    private(set) var wasLive = false
    /// True while Pocket is in the background: the cover hides the console from the app switcher.
    private(set) var away = false
    /// Registration runs once for each open, after the unlock.
    @ObservationIgnored private var registered = false
    /// Cancelled with the console: nothing registers from the background.
    @ObservationIgnored private var registration: Task<Void, Never>?
    /// The session a tapped link opens; the console opens plainly once it has been live.
    @ObservationIgnored private var openSession: Int?
    /// The session the console opened with `--open`; nil when it opened plainly.
    @ObservationIgnored private(set) var opened: Int?
    private let host: Host
    private let onFailure: (ConnectFailure) -> Void
    #if DEBUG
    @ObservationIgnored var presses = 0
    @ObservationIgnored var releases = 0
    @ObservationIgnored var rights = 0
    #endif

    var isDemo: Bool { host.isDemo }

    /// How long the agent's closing text shows before the invite rises. The UI tests stretch it
    /// (`-demoSlowInvite`): a busy runner reads the screen about once a second, too seldom to
    /// see the text in 2 seconds.
    private static var readingTime: Duration {
        ProcessInfo.processInfo.arguments.contains("-demoSlowInvite") ? .seconds(20) : .seconds(2)
    }

    init(host: Host, open: Int?, onFailure: @escaping (ConnectFailure) -> Void) {
        self.host = host
        self.openSession = open
        self.onFailure = onFailure
    }

    func start() {
        guard task == nil else { return }
        phase = .connecting
        task = Task { await run() }
    }

    func stop() {
        cancel()
        unlocked?.invalidate()
        unlocked = nil
        registered = false
        registration?.cancel()
        registration = nil
    }

    /// Pocket left the screen: the connection goes (iOS would stop it), the unlock stays for the grace window.
    func paused() {
        cancel()
        registered = false
        registration?.cancel()
        registration = nil
        grace.backgrounded(at: .now)
        away = true
    }

    /// Pocket is back: past the grace window Face ID asks again, and the cover hides the old
    /// screen until it does.
    func resumed() {
        if !grace.returned(at: .now) {
            unlocked?.invalidate()
            unlocked = nil
            wasLive = false
        }
        away = false
        start()
    }

    /// A tap on the island for a session of this host: the console connects again on it, with
    /// its screen and its unlock. Away from the screen, `resumed()` connects on it.
    func jump(to session: Int) {
        openSession = session
        guard !away else { return }
        cancel()
        start()
    }

    /// The kept screen shows while the console connects again: a line breathes over it.
    var reconnecting: Bool { phase == .connecting && wasLive && !away }

    /// The failure that the kept screen says, past the quiet retries.
    var keptFailure: String? {
        guard !showsCover, case .failed(let message, scanAgain: false) = phase else { return nil }
        return message
    }

    var showsCover: Bool {
        let page = switch phase {
        case .ended, .failed(_, scanAgain: true): true
        case .connecting, .live, .failed(_, scanAgain: false): false
        }
        return Opening.coverShows(live: phase == .live, page: page, wasLive: wasLive, away: away)
    }

    func retry() {
        cancel()
        start()
    }

    func goHome() {
        outgoing?.yield(.input(Data(HomeKey.bytes)))
    }

    /// The person sent a key that can ask the host for a copy.
    func copyAsked() {
        copyGate.asked(at: .now)
    }

    /// The answer goes with no mark: the console's binding names the terminal that sent it.
    func answerBoard() {
        outgoing?.yield(.input(Data(BoardBelt.answer)))
    }

    private func cancel() {
        task?.cancel()
        task = nil
        outgoing?.finish()
        outgoing = nil
        // Keys held for the session that went are never sent to the next one.
        held.drop()
        phase = .connecting
        invite = false
    }

    private func run() async {
        if host.isDemo {
            await runDemo()
            return
        }
        var delay = ReconnectDelay()
        var quiet = QuietRetry(keptScreen: wasLive)
        while !Task.isCancelled {
            phase = .connecting
            let key: NIOSSHPrivateKey
            do {
                key = try await unlockKey()
            } catch let failure as UnlockFailed {
                guard !Task.isCancelled else { return }
                phase = .failed(failure.message, scanAgain: false)
                return
            } catch {
                guard !Task.isCancelled else { return }
                let keyError = error as? DeviceKeyError ?? .keyUnavailable
                phase = .failed(keyError.localizedDescription, scanAgain: keyError == .keyUnavailable)
                return
            }
            guard !Task.isCancelled else { return }
            let (address, port, user, fingerprint) = (host.address, host.port, host.user, host.fingerprint)
            onCommand(.host { SSHSession(host: address, port: port, user: user, key: key, hostKeyFingerprint: fingerprint) })
            if !registered {
                registered = true
                let host = host
                let context = unlocked
                // Beside the console, never before it: an error is logged, nothing more.
                registration = Task { await Registration.afterOpen(host: host, key: key, context: context, hosts: HostStore.standard.real()) }
            }

            let session = SSHSession(
                host: host.address,
                port: host.port,
                user: host.user,
                key: key,
                hostKeyFingerprint: host.fingerprint
            )
            let opened = ContinuousClock.now
            let ending = await withTaskCancellationHandler {
                await connect(session)
            } onCancel: {
                Task { await session.close() }
            }
            await session.close()
            guard !Task.isCancelled else { return }
            switch ending {
            case .failed(let failure):
                consoleLog.error("connect failed: \(String(describing: failure), privacy: .public)")
                switch quiet.next(after: failure, at: .now) {
                case .retry(let wait):
                    phase = .connecting
                    try? await Task.sleep(for: wait)
                case .say:
                    phase = .failed(Opening.message(for: .unreachable, host: host.label), scanAgain: false)
                    return
                case .leave:
                    onFailure(failure)
                    return
                }
            case .ended(let sawOutput):
                quiet = QuietRetry(keptScreen: wasLive)
                switch delay.next(after: ContinuousClock.now - opened, sawOutput: sawOutput) {
                case .reconnect(let wait):
                    phase = .connecting
                    try? await Task.sleep(for: wait)
                case .ended:
                    markSeenCleanEnd()
                    phase = .ended
                    return
                case .stop:
                    phase = .failed(Opening.stopsAtOnceMessage(host: host.label), scanAgain: false)
                    return
                }
            }
        }
    }

    /// The demo has no key and no host to reach, so one run is all there is: a quit shows the
    /// ended screen, and a run that fails or dies says why.
    private func runDemo() async {
        phase = .connecting
        invite = false
        onCommand(.demo { line in await DemoSession.run(line) })
        let session = DemoSession()
        let watch = Task { await inviteWhenDone(session) }
        defer { watch.cancel() }
        let failure = await withTaskCancellationHandler {
            await serveDemo(session)
        } onCancel: {
            Task { await session.close() }
        }
        await session.close()
        guard !Task.isCancelled else { return }
        // The keyboard would cover the buttons of the screen that follows.
        _ = terminal?.resignFirstResponder()
        if let failure {
            phase = .failed(failure, scanAgain: false)
        } else {
            markSeenCleanEnd()
            phase = .ended
        }
    }

    /// Plan Task 3: last-seen updates on connect and on a clean end.
    private func markSeenCleanEnd() {
        var sight = HostSight()
        sight.markSeen(host.fingerprint)
    }

    /// The person's answer ended the agent's turn. The invite waits a moment so the closing text
    /// is read first. The terminal then gives up the keyboard, so the sheet is not under it; the
    /// next tap on the console takes it back.
    private func inviteWhenDone(_ session: DemoSession) async {
        while !Task.isCancelled, !session.done {
            try? await Task.sleep(for: .milliseconds(250))
        }
        try? await Task.sleep(for: Self.readingTime)
        guard !Task.isCancelled, phase == .live else { return }
        _ = terminal?.resignFirstResponder()
        invite = true
    }

    private func serveDemo(_ session: DemoSession) async -> String? {
        let size = terminalSize
        let output: AsyncStream<Data>
        do {
            output = try await session.start(cols: size.cols, rows: size.rows)
        } catch {
            return error.message
        }
        guard !Task.isCancelled else { return nil }
        _ = await serve(session, output: output, openedAt: size)
        return session.failure
    }

    private func connect(_ session: SSHSession) async -> Ending {
        let size = terminalSize
        let output: AsyncStream<Data>
        do {
            output = try await session.open(command: Opening.consoleCommand(host: host, open: openSession), pty: size)
        } catch {
            return .failed(error)
        }
        guard !Task.isCancelled else { return .ended(sawOutput: false) }
        return .ended(sawOutput: await serve(session, output: output, openedAt: size))
    }

    /// Runs one source until its output ends, and says whether it drew anything.
    private func serve(_ source: any ConsoleSource, output: AsyncStream<Data>, openedAt size: (cols: Int, rows: Int)) async -> Bool {
        let (input, sink) = AsyncStream<Outgoing>.makeStream()
        outgoing = sink
        let writer = Task {
            for await item in input {
                switch item {
                case .input(let data):
                    await source.write(data)
                case .resize(let cols, let rows):
                    await source.resize(cols: cols, rows: rows)
                }
            }
        }
        defer {
            sink.finish()
            writer.cancel()
            // Keys typed until the next session opens are held, not sent to an ended one.
            outgoing = nil
        }

        // A reconnect keeps the last screen: the tmux attach draws over it, with no blank between.
        if !wasLive {
            terminal?.getTerminal().resetToInitialState()
            terminal?.forgetScrollBack()
        }
        phase = .live
        // The demo starts over each time it opens: it keeps no screen.
        wasLive = !host.isDemo
        var sight = HostSight()
        sight.markSeen(host.fingerprint)
        // Teach BOX once after the first console land (pair or demo) on the visible belt.
        onBecameLive()
        opened = openSession
        openSession = nil
        let now = terminalSize
        if now != size {
            sink.yield(.resize(cols: now.cols, rows: now.rows))
        }
        if let keys = held.take() {
            sink.yield(.input(keys))
        }
        var sawOutput = false
        // The agent signal comes out before SwiftTerm sees the bytes: the terminal never draws it.
        var signal = AgentSignalFilter()
        consoleMarks = false
        paneAgent = PaneAgent()
        if opened != nil { paneAgent.opened(at: ContinuousClock.now) }
        settling?.cancel()
        settling = nil
        onAgent(nil, nil)
        onBoard("")
        for await chunk in output {
            sawOutput = true
            let (pass, raw, board) = signal.feed(ArraySlice(chunk))
            if !pass.isEmpty {
                terminal?.feed(byteArray: pass[...])
            }
            if let raw {
                heard(ConsoleSignal(raw))
            }
            if let board {
                onBoard(board)
            }
            onOutput()
        }
        return sawOutput
    }

    private func heard(_ signal: ConsoleSignal) {
        if signal.marks { consoleMarks = true }
        guard signal.pane != nil else { return }
        paneAgent.signal(signal, at: ContinuousClock.now)
        onAgent(paneAgent.agent, paneAgent.pane)
    }

    /// Output may stop while the console shows, so a way back that went is looked for again.
    func paneSeen(_ inPane: Bool) {
        if paneAgent.shows(pane: inPane, at: ContinuousClock.now) {
            onAgent(nil, nil)
            return
        }
        guard paneAgent.leaving, settling == nil else { return }
        settling = Task { [weak self] in
            try? await Task.sleep(for: PaneAgent.settle)
            guard let self, !Task.isCancelled else { return }
            self.settling = nil
            self.paneSeen(self.terminal?.inTmuxPane == true)
        }
    }

    private var terminalSize: (cols: Int, rows: Int) {
        guard let terminal = terminal?.getTerminal() else { return (80, 25) }
        return (terminal.cols, terminal.rows)
    }

    /// Face ID (or the passcode) runs here, before SSHSession starts its deadline. The unlocked context then lets the key sign with no second prompt.
    private func unlockKey() async throws -> NIOSSHPrivateKey {
        let context: LAContext
        if let unlocked {
            context = unlocked
        } else {
            context = LAContext()
            context.touchIDAuthenticationAllowableReuseDuration = 10
            try await Self.evaluate(context, reason: "Open the console of \(host.label).")
            guard !Task.isCancelled else {
                context.invalidate()
                throw CancellationError()
            }
            unlocked = context
        }
        return DeviceKey.nioKey(try DeviceKey.load(host: host.fingerprint, context: context))
    }

    private static func evaluate(_ context: LAContext, reason: String) async throws {
        try await withCheckedThrowingContinuation { (done: CheckedContinuation<Void, Error>) in
            context.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: reason) { @Sendable success, error in
                if success {
                    done.resume()
                } else {
                    done.resume(throwing: UnlockFailed(message: unlockMessage(for: error)))
                }
            }
        }
    }

    private nonisolated static func unlockMessage(for error: Error?) -> String {
        switch (error as? LAError)?.code {
        case .userCancel, .systemCancel, .appCancel:
            "The key is still locked. Tap Try again, then use Face ID or the passcode."
        case .passcodeNotSet:
            DeviceKeyError.noPasscode.localizedDescription
        default:
            "This device could not check Face ID or the passcode. Tap Try again."
        }
    }
}

extension ConsoleConnection: @preconcurrency TerminalViewDelegate {
    func send(source: TerminalView, data: ArraySlice<UInt8>) {
        #if DEBUG
        if isDemo { countClicks(in: data) }
        #endif
        // Every key reaches this: the soft and the hardware keyboard's, the belt's, and taps.
        let marked = consoleMarks && terminal?.inTmuxPane == false
        if marked { paneAgent.marked(at: ContinuousClock.now) }
        let keys = Data(InputMark.sent(data, marked: marked))
        if let outgoing {
            outgoing.yield(.input(keys))
        } else if reconnecting {
            held.hold(keys)
        }
    }

    func sizeChanged(source: TerminalView, newCols: Int, newRows: Int) {
        outgoing?.yield(.resize(cols: newCols, rows: newRows))
    }

    func scrolled(source: TerminalView, position: Double) {
        terminal?.scrollChanged()
    }

    /// SwiftTerm's own link tap (a pointer's hover on an iPad) asks first too.
    func requestOpenLink(source: TerminalView, link: String, params: [String: String]) {
        guard let terminal else { return }
        terminal.onLink(link, CGRect(x: terminal.bounds.midX, y: terminal.bounds.midY, width: 0, height: 0))
    }

    /// The host's copy (OSC 52), as `c copy reply` asks for it. Only a copy the person asked
    /// for reaches the clipboard (`CopyGate`), and only this device's: not Universal Clipboard.
    func clipboardCopy(source: TerminalView, content: Data) {
        switch copyGate.take(content, at: .now) {
        case .copy(let text):
            UIPasteboard.general.setItems([[UTType.utf8PlainText.identifier: text]], options: [.localOnly: true])
            onCopied("Copied", true)
        case .tooLarge:
            onCopied("Too large to copy", false)
        case .unasked:
            consoleLog.info("a copy nobody asked for is dropped (\(content.count, privacy: .public) bytes)")
        }
    }

    func setTerminalTitle(source: TerminalView, title: String) {}
    func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
    func rangeChanged(source: TerminalView, startY: Int, endY: Int) {}
}

#if DEBUG
extension ConsoleConnection {
    /// The UI test reads the presses and releases of the left button the terminal sent, and the
    /// right presses, as the terminal's accessibility value (`ConsoleTerminalView.clickNote`). Reports are SGR (`ESC [ < b ; x ; y M|m`) or classic
    /// (`ESC [ M cb x y`).
    func countClicks(in data: ArraySlice<UInt8>) {
        let bytes = Array(data)
        var at = 0
        while at + 2 < bytes.count {
            guard bytes[at] == 0x1b, bytes[at + 1] == 0x5b else { at += 1; continue }
            if bytes[at + 2] == 0x3c {
                var end = at + 3
                while end < bytes.count, bytes[end] != 0x4d, bytes[end] != 0x6d { end += 1 }
                guard end < bytes.count else { break }
                let button = String(decoding: bytes[(at + 3)..<end].prefix { $0 != 0x3b }, as: UTF8.self)
                if button == "0" {
                    if bytes[end] == 0x4d { presses += 1 } else { releases += 1 }
                } else if button == "2", bytes[end] == 0x4d {
                    rights += 1
                }
                at = end + 1
            } else if bytes[at + 2] == 0x4d, at + 5 < bytes.count {
                let button = Int(bytes[at + 3]) - 32
                if button == 0 { presses += 1 } else if button == 3 { releases += 1 } else if button == 2 { rights += 1 }
                at += 6
            } else {
                at += 1
            }
        }
        terminal?.clickNote = "press=\(presses) release=\(releases) right=\(rights)"
    }
}
#endif

/// The kept screen connects again: a thin brass line breathes over its top, and nothing else moves.
private struct BreathingLine: View {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var lit = false

    var body: some View {
        Rectangle()
            .fill(Theme.accent)
            .frame(height: 2)
            .opacity(reduceMotion ? 0.7 : (lit ? 1 : 0.25))
            .frame(maxHeight: .infinity, alignment: .top)
            .allowsHitTesting(false)
            .accessibilityLabel("Connecting again")
            .onAppear {
                guard !reduceMotion else { return }
                withAnimation(.easeInOut(duration: 0.9).repeatForever(autoreverses: true)) { lit = true }
            }
    }
}
