import LocalAuthentication
import MatchbloxKit
import NIOSSH
import SwiftTerm
import SwiftUI

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
                HStack(spacing: 24) {
                    Button("Open again") { connection.retry() }
                        .buttonStyle(.borderedProminent)
                        .foregroundStyle(Theme.bg)
                    hostsButton
                }
            }
            Spacer()
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .padding(24)
        .background(Theme.bg)
    }

    /// Hosts (with more than one host) and Island (always), beside the console's actions.
    private var hostsButton: some View {
        HStack(spacing: 24) {
            if let onHosts {
                Button("Hosts", action: onHosts)
                    .foregroundStyle(Theme.muted)
            }
            Button("Island", action: onIsland)
                .foregroundStyle(Theme.muted)
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

        let belts = Belts(terminal: terminal)
        terminal.inputAccessoryView = belts.lifted
        terminal.onFocus = { [weak belts] focused in belts?.focusChanged(focused) }
        connection.onAgent = { agent in belts.agentChanged(agent) }
        connection.onOutput = { belts.outputArrived() }
        let stack = UIStackView(arrangedSubviews: [terminal, belts.dock])
        stack.axis = .vertical
        return stack
    }

    func updateUIView(_ stack: UIStackView, context: Context) {}
}

/// The two belts, the one on the keyboard and the one docked under the terminal, kept as one:
/// the same agent, the same live commands, and the same resting place.
@MainActor
private final class Belts: NSObject {
    let lifted = BeltView()
    let dock = BeltView()
    private weak var terminal: ConsoleTerminalView?
    private var agent: String?

    init(terminal: ConsoleTerminalView) {
        self.terminal = terminal
        super.init()
        for belt in [lifted, dock] {
            belt.onSend = { [weak terminal] bytes in terminal?.send(bytes) }
            belt.onKeyboard = { [weak terminal] in
                guard let terminal else { return }
                _ = terminal.isFirstResponder ? terminal.resignFirstResponder() : terminal.becomeFirstResponder()
            }
            belt.onLayout = { [weak self] layout in self?.layoutPicked(layout) }
            belt.onEditMine = { [weak self] in self?.editMine() }
        }
        NotificationCenter.default.addObserver(self, selector: #selector(keyboardChanged), name: UIResponder.keyboardWillChangeFrameNotification, object: nil)
    }

    func agentChanged(_ agent: String?) {
        self.agent = agent
        lifted.agent = agent
        dock.agent = agent
        outputArrived()
    }

    /// A command runs only from the chat: never with no agent, never in a jumped pane.
    func outputArrived() {
        let live = !(agent ?? "").isEmpty && terminal?.inTmuxPane == false
        lifted.commandsLive = live
        dock.commandsLive = live
    }

    func focusChanged(_ focused: Bool) {
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
        guard var top = terminal?.window?.rootViewController else { return }
        while let next = top.presentedViewController { top = next }
        let editor = UIHostingController(rootView: BeltEditor(
            catalog: BeltView.catalog,
            dismiss: { [weak top] in top?.dismiss(animated: true) },
            closed: { [weak self] done in self?.mineEdited(show: done) }
        ))
        editor.view.backgroundColor = BeltLook.sheet
        if let sheet = editor.sheetPresentationController {
            sheet.detents = [.custom { $0.maximumDetentValue * 0.7 }, .large()]
            sheet.preferredCornerRadius = 16
        }
        top.present(editor, animated: true)
    }

    private func mineEdited(show: Bool) {
        lifted.mineEdited(show: show)
        dock.mineEdited(show: show)
    }

    /// The keyboard's frame holds the lifted belt. Only a system keyboard makes it taller than
    /// the belt by more than a key; a hardware keyboard shows the belt alone.
    @objc private func keyboardChanged(_ note: Notification) {
        guard let end = note.userInfo?[UIResponder.keyboardFrameEndUserInfoKey] as? CGRect else { return }
        let onScreen = end.minY < (terminal?.window?.windowScene?.screen.bounds.height ?? .greatestFiniteMagnitude)
        lifted.keyboardUp = onScreen && end.height - max(lifted.bounds.height, BeltView.handleHeight + BeltGeometry.height) > BeltGeometry.keyHeight
    }
}

/// Owns one SSHSession for each connect. A session that ends at once opens again; a good session that ends waits for a tap. It stops when the view goes away or the app goes to the background.
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

    @ObservationIgnored weak var terminal: TerminalView?
    /// The console's agent signal: the agent of the chat's session, or "" for none. A new
    /// session starts with none.
    @ObservationIgnored var onAgent: @MainActor (String?) -> Void = { _ in }
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
    }

    private func run() async {
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
                registration = Task { await Registration.afterOpen(host: host, key: key, context: context, hosts: HostStore.standard.all()) }
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
                await serve(session)
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

    /// Runs one session until its stream ends.
    private func serve(_ session: SSHSession) async -> Ending {
        let size = terminalSize
        let output: AsyncStream<Data>
        do {
            output = try await session.open(command: Opening.consoleCommand(host: host, open: openSession), pty: size)
        } catch {
            return .failed(error)
        }
        guard !Task.isCancelled else { return .ended(sawOutput: false) }

        let (input, sink) = AsyncStream<Outgoing>.makeStream()
        outgoing = sink
        let writer = Task {
            for await item in input {
                switch item {
                case .input(let data):
                    await session.write(data)
                case .resize(let cols, let rows):
                    await session.resize(cols: cols, rows: rows)
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
        onAgent(nil)
        for await chunk in output {
            sawOutput = true
            let (pass, agent) = signal.feed(ArraySlice(chunk))
            if !pass.isEmpty {
                terminal?.feed(byteArray: pass[...])
            }
            if let agent {
                onAgent(agent)
            }
            onOutput()
        }
        return .ended(sawOutput: sawOutput)
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
