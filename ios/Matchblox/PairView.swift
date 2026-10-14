import MatchbloxKit
import NIOSSH
import SwiftUI

struct PairView: View {
    @State var draft: PairDraft
    let store: HostStore
    let onPaired: (Host) -> Void
    let onScanAgain: () -> Void

    private enum Phase: Equatable {
        case ready
        case pairing
        case failed(String)
    }

    @State private var phase = Phase.ready
    @State private var pulse = false
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var pairing: Bool { phase == .pairing }
    private var canPair: Bool {
        !pairing && !draft.address.trimmingCharacters(in: .whitespaces).isEmpty
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            VStack(alignment: .leading, spacing: 6) {
                Text("Pair with \(draft.payload.name)")
                    .font(.title3)
                Text("as \(draft.payload.user)")
                    .foregroundStyle(Theme.muted)
            }
            field("Address", text: $draft.address, hint: "Change it if the phone can't reach this name.")
                .keyboardType(.URL)
            field("Phone name", text: $draft.phoneName, hint: "\(draft.payload.name) shows this name.")
            if case .failed(let message) = phase {
                Text(message)
                    .foregroundStyle(Theme.neg)
            }
            HStack(spacing: 24) {
                Button {
                    Task { await pair() }
                } label: {
                    Text(pairing ? "Pairing…" : "Pair")
                        .opacity(pairing && pulse ? 0.4 : 1)
                }
                .buttonStyle(.borderedProminent)
                .foregroundStyle(Theme.bg)
                .disabled(!canPair)
                Button("Scan again", action: onScanAgain)
                    .foregroundStyle(Theme.muted)
                    .disabled(pairing)
            }
            Spacer()
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(24)
        .textInputAutocapitalization(.never)
        .autocorrectionDisabled()
        .onChange(of: pairing) { _, now in
            guard now, !reduceMotion else {
                pulse = false
                return
            }
            withAnimation(.easeInOut(duration: 0.8).repeatForever(autoreverses: true)) {
                pulse = true
            }
        }
    }

    private func field(_ label: String, text: Binding<String>, hint: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label)
                .font(.footnote)
                .foregroundStyle(Theme.faint)
            TextField(label, text: text)
                .disabled(pairing)
                .padding(.vertical, 8)
                .overlay(alignment: .bottom) {
                    Rectangle().fill(Theme.hair).frame(height: 1)
                }
            Text(hint)
                .font(.footnote)
                .foregroundStyle(Theme.faint)
        }
    }

    private func pair() async {
        phase = .pairing
        switch await PairFlow.run(draft, store: store) {
        case .paired(let host):
            onPaired(host)
        case .failed(let message):
            phase = .failed(message)
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
            phoneKey = try await Task.detached { try openPhoneKey() }.value
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
            return .failed(Pairing.message(for: error, host: label))
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
            do {
                try store.upsert(host)
            } catch {
                return .failed("This iPhone could not save \(label). Free some space on the iPhone, then run matchblox pair on \(label) again.")
            }
            return .paired(host)
        case .failed(let refusal):
            return .failed(Pairing.message(for: refusal, host: label))
        }
    }

    /// A stored key that no longer opens belongs to an old pairing. A new key replaces it, so every host must pair again.
    private static func openPhoneKey() throws -> String {
        do {
            return try DeviceKey.openSSH()
        } catch DeviceKeyError.keyUnavailable {
            try DeviceKey.reset()
            return try DeviceKey.openSSH()
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
