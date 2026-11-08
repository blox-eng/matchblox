import LocalAuthentication
import MatchbloxKit
import NIOSSH
import SwiftTerm
import SwiftUI

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
    let onHosts: (() -> Void)?
    let onIsland: () -> Void
    let onScanAgain: () -> Void

    @State private var connection: ConsoleConnection
    @Environment(\.scenePhase) private var scenePhase

    init(
        host: Host,
        open: Int? = nil,
        onFailure: @escaping (ConnectFailure) -> Void,
        onHosts: (() -> Void)?,
        onIsland: @escaping () -> Void,
        onScanAgain: @escaping () -> Void
    ) {
        self.host = host
        self.onHosts = onHosts
        self.onIsland = onIsland
        self.onScanAgain = onScanAgain
        _connection = State(initialValue: ConsoleConnection(host: host, open: open, onFailure: onFailure))
    }

    var body: some View {
        ZStack {
            TerminalRepresentable(connection: connection)
            if connection.phase != .live {
                cover
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
        .onChange(of: scenePhase) { _, phase in
            switch phase {
            case .background: connection.stop()
            case .active: connection.start()
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

    /// Hosts (with more than one host) and Island (for a real host), beside the console's actions.
    private var hostsButton: some View {
        HStack(spacing: 24) {
            if let onHosts {
                Button("Hosts", action: onHosts)
                    .foregroundStyle(Theme.muted)
            }
            if !host.isDemo {
                Button("Island", action: onIsland)
                    .foregroundStyle(Theme.muted)
            }
        }
    }
}

/// The terminal, with the key belt above the keyboard. An input accessory shows only while its
/// view has the keyboard, so a second belt stays under the terminal while the keyboard is down.
private struct TerminalRepresentable: UIViewRepresentable {
    let connection: ConsoleConnection

    func makeUIView(context: Context) -> UIStackView {
        let terminal = ConsoleTerminalView(font: UIFont.monospacedSystemFont(ofSize: 13, weight: .regular))
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
        terminal.inputAccessoryView = belts.lifted
        terminal.tapRaisesKeyboard = !connection.isDemo
        terminal.onFocus = { [weak belts] focused in belts?.focusChanged(focused) }
        connection.onAgent = { agent in belts.agentChanged(agent) }
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

/// The terminal's place on screen. While a matchbox tray is open the place gets shorter, but
/// the terminal keeps its size (`ScreenHold`): its lines slide up and the top ones go under the
/// edge. SwiftTerm resizes only when its bounds change, so the host gets no resize.
private final class HeldScreen: UIView {
    private let terminal: UIView
    private var hold = ScreenHold()
    private var heldSize: ScreenHold.Size {
        ScreenHold.Size(width: bounds.width, height: bounds.height)
    }

    init(terminal: UIView) {
        self.terminal = terminal
        super.init(frame: .zero)
        clipsToBounds = true
        addSubview(terminal)
    }

    required init?(coder: NSCoder) { nil }

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
    }
}

/// The two belts, the one on the keyboard and the one docked under the terminal, kept as one:
/// the same agent, the same live commands, and the same resting place.
@MainActor
private final class Belts {
    let lifted = BeltView()
    let dock = BeltView()
    private weak var terminal: ConsoleTerminalView?
    private var agent: String?
    /// The Mine editor while it shows: never a second one over it.
    private weak var editor: UIViewController?
    /// A live check waits for the end of this run-loop turn: one for every read of the turn.
    private var liveCheckQueued = false
    /// Commands go live only once the screen has stayed the agent's pane this long.
    private static let liveDelay: Duration = .milliseconds(300)
    private var goingLive: Task<Void, Never>?
    /// Whether the terminal shows a pane's way back, at each live check.
    var onPaneSeen: @MainActor (Bool) -> Void = { _ in }

    init(terminal: ConsoleTerminalView, screen: HeldScreen, isDemo: Bool) {
        self.terminal = terminal
        for belt in [lifted, dock] {
            belt.isDemo = isDemo
            belt.onScreenHold = { [weak screen] holding in screen?.trayMoved(open: holding) }
            belt.onSend = { [weak terminal] bytes in terminal?.send(bytes) }
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
        }
        ctrlObserver = NotificationCenter.default.addObserver(forName: Self.ctrlReset, object: terminal, queue: .main) {
            [weak self] _ in MainActor.assumeIsolated { self?.ctrlChanged(false) }
        }
    }

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

    func agentChanged(_ agent: String?) {
        self.agent = agent
        lifted.agent = agent
        dock.agent = agent
        outputArrived()
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
        onPaneSeen(terminal?.inTmuxPane == true)
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

    /// A layout picked on one belt is the layout of both.
    private func layoutPicked(_ layout: BeltLayout) {
        lifted.layout = layout
        dock.layout = layout
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
    @ObservationIgnored var onAgent: @MainActor (String?) -> Void = { _ in }
    /// The console announced that it reads the app's marks; an older console never does.
    @ObservationIgnored private var consoleMarks = false
    @ObservationIgnored private var paneAgent = PaneAgent()
    /// A second look at the screen once the way back has stayed away for `PaneAgent.settle`.
    @ObservationIgnored private var settling: Task<Void, Never>?
    /// Called each time a read has reached the terminal.
    @ObservationIgnored var onOutput: @MainActor () -> Void = {}
    @ObservationIgnored private var task: Task<Void, Never>?
    @ObservationIgnored private var outgoing: AsyncStream<Outgoing>.Continuation?
    /// One unlock serves every reconnect until the app goes to the background.
    @ObservationIgnored private var unlocked: LAContext?
    /// Registration runs once for each open, after the unlock.
    @ObservationIgnored private var registered = false
    /// Cancelled with the console: nothing registers from the background.
    @ObservationIgnored private var registration: Task<Void, Never>?
    /// The session a tapped link opens; the console opens plainly once it has been live.
    @ObservationIgnored private var openSession: Int?
    private let host: Host
    private let onFailure: (ConnectFailure) -> Void
    #if DEBUG
    @ObservationIgnored var presses = 0
    @ObservationIgnored var releases = 0
    #endif

    var isDemo: Bool { host.isDemo }

    /// How long the agent's closing text shows before the invite rises.
    private static let readingTime: Duration = .seconds(2)

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

    func retry() {
        cancel()
        start()
    }

    func goHome() {
        outgoing?.yield(.input(Data(HomeKey.bytes)))
    }

    private func cancel() {
        task?.cancel()
        task = nil
        outgoing?.finish()
        outgoing = nil
        phase = .connecting
        invite = false
    }

    private func run() async {
        if host.isDemo {
            await runDemo()
            return
        }
        var delay = ReconnectDelay()
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
                onFailure(failure)
                return
            case .ended(let sawOutput):
                switch delay.next(after: ContinuousClock.now - opened, sawOutput: sawOutput) {
                case .reconnect(let wait):
                    phase = .connecting
                    try? await Task.sleep(for: wait)
                case .ended:
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
            phase = .ended
        }
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
        }

        terminal?.getTerminal().resetToInitialState()
        phase = .live
        openSession = nil
        let now = terminalSize
        if now != size {
            sink.yield(.resize(cols: now.cols, rows: now.rows))
        }
        var sawOutput = false
        // The agent signal comes out before SwiftTerm sees the bytes: the terminal never draws it.
        var signal = AgentSignalFilter()
        consoleMarks = false
        paneAgent = PaneAgent()
        settling?.cancel()
        settling = nil
        onAgent(nil)
        for await chunk in output {
            sawOutput = true
            let (pass, raw) = signal.feed(ArraySlice(chunk))
            if !pass.isEmpty {
                terminal?.feed(byteArray: pass[...])
            }
            if let raw {
                heard(ConsoleSignal(raw))
            }
            onOutput()
        }
        return sawOutput
    }

    private func heard(_ signal: ConsoleSignal) {
        if signal.marks { consoleMarks = true }
        guard signal.pane != nil else { return }
        paneAgent.signal(signal, at: ContinuousClock.now)
        onAgent(paneAgent.agent)
    }

    /// Output may stop while the console shows, so a way back that went is looked for again.
    func paneSeen(_ inPane: Bool) {
        if paneAgent.shows(pane: inPane, at: ContinuousClock.now) {
            onAgent(nil)
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
        if consoleMarks, terminal?.inTmuxPane == false, InputMark.marks(data) {
            outgoing?.yield(.input(Data(InputMark.bytes)))
            paneAgent.marked(at: ContinuousClock.now)
        }
        outgoing?.yield(.input(Data(data)))
    }

    func sizeChanged(source: TerminalView, newCols: Int, newRows: Int) {
        outgoing?.yield(.resize(cols: newCols, rows: newRows))
    }

    func setTerminalTitle(source: TerminalView, title: String) {}
    func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
    func scrolled(source: TerminalView, position: Double) {}
    func requestOpenLink(source: TerminalView, link: String, params: [String: String]) {}
    func rangeChanged(source: TerminalView, startY: Int, endY: Int) {}
}

#if DEBUG
extension ConsoleConnection {
    /// The UI test reads the presses and releases of the left button the terminal sent, as the
    /// terminal's accessibility value. Reports are SGR (`ESC [ < b ; x ; y M|m`) or classic
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
                }
                at = end + 1
            } else if bytes[at + 2] == 0x4d, at + 5 < bytes.count {
                let button = Int(bytes[at + 3]) - 32
                if button == 0 { presses += 1 } else if button == 3 { releases += 1 }
                at += 6
            } else {
                at += 1
            }
        }
        terminal?.accessibilityValue = "press=\(presses) release=\(releases)"
    }
}
#endif
