import MatchbloxKit
import NIOSSH
import SwiftUI

/// The box takes the link and pairs, with no form: the scan is the consent (design/0009 §6.2, §6.5).
struct PairingView: View {
    @State var draft: PairDraft
    let store: HostStore
    let onPaired: (Host) -> Void
    let onScanAgain: () -> Void

    private enum Phase: Equatable {
        case pairing
        case paired(Date)
        /// `address` is true when the host did not answer at the address, so another address can help.
        case failed(String, address: Bool)
    }

    @State private var phase = Phase.pairing
    @State private var attempt = 0
    @State private var pulse = false
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var pose: BoxPose {
        switch phase {
        case .pairing: .shut
        case .paired(let start): .open(start)
        case .failed: .failed
        }
    }

    var body: some View {
        GeometryReader { geo in
            let box = PairStage.boxFrame(in: geo.size)
            ZStack(alignment: .topLeading) {
                MatchBox(pose: pose, label: AnyView(label))
                    .frame(width: box.width, height: box.height)
                    .position(x: box.midX, y: box.midY)
                if case .failed(let message, let address) = phase {
                    failure(message, address: address)
                        .padding(.top, box.maxY + 24)
                        .padding(.horizontal, 24)
                }
            }
        }
        .ignoresSafeArea(.container)
        .textInputAutocapitalization(.never)
        .autocorrectionDisabled()
        .task(id: attempt) { await pair() }
        .onChange(of: phase == .pairing, initial: true) { _, pairing in
            guard pairing, !reduceMotion else {
                pulse = false
                return
            }
            pulse = false
            withAnimation(.easeInOut(duration: 0.8).repeatForever(autoreverses: true)) {
                pulse = true
            }
        }
    }

    /// The host's name on the label. It pulses while the app connects: a value that is not known yet.
    private var label: some View {
        ZStack {
            Theme.fg
            Text(draft.payload.name)
                .font(.headline)
                .multilineTextAlignment(.center)
                .lineLimit(3)
                .minimumScaleFactor(0.4)
                .padding(6)
                .foregroundStyle(phase == .pairing || isPaired ? Theme.bg : Theme.faint)
                .opacity(phase == .pairing && pulse ? 0.4 : 1)
        }
    }

    private var isPaired: Bool {
        if case .paired = phase { return true }
        return false
    }

    private func failure(_ message: String, address: Bool) -> some View {
        VStack(alignment: .leading, spacing: 20) {
            Text(message)
                .foregroundStyle(Theme.neg)
            if address {
                VStack(alignment: .leading, spacing: 6) {
                    Text("Address")
                        .font(.footnote)
                        .foregroundStyle(Theme.faint)
                    TextField("Address", text: $draft.address)
                        .keyboardType(.URL)
                        .padding(.vertical, 8)
                        .overlay(alignment: .bottom) {
                            Rectangle().fill(Theme.hair).frame(height: 1)
                        }
                }
            }
            HStack(spacing: 24) {
                if address {
                    Button("Try again") { attempt += 1 }
                        .buttonStyle(.borderedProminent)
                        .foregroundStyle(Theme.bg)
                        .disabled(draft.address.trimmingCharacters(in: .whitespaces).isEmpty)
                }
                Button("Scan again", action: onScanAgain)
                    .foregroundStyle(Theme.muted)
            }
        }
        .frame(maxWidth: 480, alignment: .leading)
    }

    /// A new code or a new screen cancels the task, so an old pairing never opens a console.
    private func pair() async {
        Haptics.prepare()
        phase = .pairing
        let outcome = await PairFlow.run(draft, store: store)
        guard !Task.isCancelled else { return }
        switch outcome {
        case .paired(let host):
            let clock = ContinuousClock()
            let start = clock.now
            phase = .paired(Date())
            do {
                try await clock.sleep(until: start + .seconds(MatchBox.strikeAt))
                Haptics.strike()
                try await clock.sleep(until: start + .seconds(MatchBox.stoodAt))
                Haptics.stood()
            } catch {
                return
            }
            onPaired(host)
        case .failed(let message):
            phase = .failed(message, address: isConnectionFailure(message))
            Haptics.failed()
        }
    }

    /// PairFlow gives only the message, so the message tells the connection failures from the others.
    private func isConnectionFailure(_ message: String) -> Bool {
        let address = draft.address.trimmingCharacters(in: .whitespacesAndNewlines)
        return [ConnectFailure.unreachable, .hostKeyChanged].contains {
            Pairing.message(for: $0, host: draft.payload.name, address: address) == message
        }
    }
}

enum PairOutcome: Sendable {
    case paired(Host)
    case failed(String)
}

enum PairFlow {
    /// gate pair refuses a request after 30 s, so a host that says nothing by now never will.
    private static let replyDeadline = Duration.seconds(45)

    static func run(_ draft: PairDraft, store: HostStore) async -> PairOutcome {
        let payload = draft.payload
        let label = payload.name
        let address = draft.address.trimmingCharacters(in: .whitespacesAndNewlines)

        let phoneKey: String
        do {
            phoneKey = try await Task.detached { try DeviceKey.openSSH(for: payload.hostkey) }.value
        } catch {
            return .failed(error.localizedDescription)
        }

        let oneTimeKey: NIOSSHPrivateKey
        do {
            oneTimeKey = try SSHKeys.oneTimeKey(seedBase64: payload.key)
        } catch {
            return .failed(Pairing.message(for: .malformed))
        }

        let session = SSHSession(
            host: address,
            port: draft.port,
            user: payload.user,
            key: oneTimeKey,
            hostKeyFingerprint: payload.hostkey
        )
        let output: AsyncStream<Data>
        do {
            output = try await session.open(command: "matchblox gate pair", pty: nil)
        } catch {
            return .failed(Pairing.message(for: error, host: label, address: address))
        }

        let request = Pairing.requestLine(name: Pairing.cleanName(draft.phoneName), key: phoneKey)
        let reply: PairReply
        do {
            reply = try await exchange(request, over: session, output: output)
        } catch {
            await session.close()
            return .failed(Pairing.message(for: error, host: label))
        }
        await session.close()

        switch reply {
        case .ok:
            let host = Host(
                label: label,
                address: address,
                port: draft.port,
                user: payload.user,
                fingerprint: payload.hostkey
            )
            let stale: [Host]
            do {
                stale = try store.upsert(host)
            } catch {
                return .failed("This iPhone could not save \(label). Free some space on the iPhone, then run matchblox pair on \(label) again.")
            }
            for old in stale {
                try? DeviceKey.remove(host: old.fingerprint)
            }
            return .paired(host)
        case .failed(let refusal):
            return .failed(Pairing.message(for: refusal, host: label))
        }
    }

    private static func exchange(
        _ request: Data,
        over session: SSHSession,
        output: AsyncStream<Data>
    ) async throws(PairReplyError) -> PairReply {
        let deadline = Task {
            try? await Task.sleep(for: replyDeadline)
            await session.close()
        }
        defer { deadline.cancel() }
        await session.write(request)
        return try await Pairing.readReply(from: output)
    }
}
