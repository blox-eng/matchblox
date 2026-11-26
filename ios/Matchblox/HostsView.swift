import MatchbloxKit
import SwiftUI

/// Pocket home: hosts first, then Live Activity, text size, quiet footer.
struct HostsView: View {
    let store: HostStore
    /// Console behind home, if any (`‹ host`).
    var backHost: Host? = nil
    let onOpen: (Host) -> Void
    let onBack: () -> Void
    let onPair: () -> Void
    let onIsland: () -> Void
    let onDemo: () -> Void
    /// Called after each confirmed remove (clear `‹ host` when that host went; last host stays on home).
    let onHostRemoved: (Host) -> Void

    @State private var hosts: [Host] = []
    @State private var removedNotice: String?
    @State private var problem: String?
    @State private var pendingRemove: Host?
    @State private var textSize = TextSize.load(from: .standard)
    @State private var prefs = IslandSettings.prefs
    /// First-run mark play start; nil after it finishes or when skipped.
    @State private var firstRunPlayStart: Date?
    @State private var flags = HomeFlags()
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    private let sight = HostSight()

    private var realHosts: [Host] { hosts.filter { !$0.isDemo } }
    private var firstRun: Bool { hosts.isEmpty }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
                if let backHost {
                    Button(action: onBack) {
                        Text("‹ \(backHost.label)")
                            .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
                    }
                    .foregroundStyle(Theme.accent)
                    .accessibilityLabel(HomeCopy.voiceOverBack(label: backHost.label))
                }

                if let start = firstRunPlayStart {
                    MatchBox(pose: .open(start))
                        .frame(maxWidth: 160)
                        .accessibilityHidden(true)
                        .task(id: start) { await finishFirstRunPlay(from: start) }
                }

                Text("▰ matchblox")
                    .foregroundStyle(Theme.accent)

                if let removedNotice {
                    Text(removedNotice).foregroundStyle(Theme.muted)
                }
                if let problem {
                    Text(problem).foregroundStyle(Theme.neg)
                }

                region("HOSTS" + (hosts.isEmpty ? "" : " · \(hosts.count)")) {
                    if firstRun {
                        Text("Pair a host to open its console on this phone.")
                            .foregroundStyle(Theme.muted)
                        Button(action: onPair) { tall("pair a host") }
                            .foregroundStyle(Theme.accent)
                            .accessibilityLabel("Pair a host")
                        Button(action: onDemo) { tall("try the demo") }
                            .foregroundStyle(Theme.muted)
                            .accessibilityLabel("Try the demo")
                    } else {
                        ForEach(hosts) { host in
                            hostBlock(host)
                        }
                        Button(action: onPair) { tall("+ pair a host") }
                            .foregroundStyle(Theme.accent)
                            .accessibilityLabel("Pair a host")
                    }
                }

                if !realHosts.isEmpty {
                    region("LIVE ACTIVITY") {
                        Button(action: onIsland) {
                            Text(HomeCopy.liveActivityRow(prefs: prefs, hosts: realHosts) + "    ›")
                                .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
                        }
                        .foregroundStyle(Theme.muted)
                        .accessibilityLabel(
                            "Live Activity, " + HomeCopy.liveActivityRow(prefs: prefs, hosts: realHosts)
                        )
                    }
                }

                HStack(spacing: 16) {
                    Text("text")
                        .foregroundStyle(Theme.faint)
                    Button { textSize = textSize.smaller } label: { tall("−", width: 44) }
                        .disabled(textSize.isSmallest)
                        .accessibilityLabel("Smaller text")
                    Text("\(textSize.points) pt")
                        .monospacedDigit()
                        .foregroundStyle(Theme.faint)
                    Button { textSize = textSize.bigger } label: { tall("+", width: 44) }
                        .disabled(textSize.isBiggest)
                        .accessibilityLabel("Bigger text")
                    Text("▰ sample")
                        .font(.system(size: CGFloat(textSize.points), design: .monospaced))
                        .foregroundStyle(Theme.faint)
                }
                .buttonStyle(.borderless)

                footer
            }
            .padding(24)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .onAppear {
            hosts = store.all()
            prefs = IslandSettings.prefs
            textSize = TextSize.load(from: .standard)
            maybeStartFirstRunPlay()
        }
        .onChange(of: textSize) { _, size in size.save(to: .standard) }
    }

    /// Spec 3.1 / 6.2: play MatchBox open once on empty home; skip under Reduce Motion; always mark.
    private func maybeStartFirstRunPlay() {
        guard hosts.isEmpty, flags.shouldPlayFirstRun else { return }
        if reduceMotion {
            flags.markFirstRunPlayPlayed()
            return
        }
        firstRunPlayStart = Date()
    }

    private func finishFirstRunPlay(from start: Date) async {
        let clock = ContinuousClock()
        let origin = clock.now
        let hold: Duration = .seconds(0.4)
        do {
            try await clock.sleep(until: origin + .seconds(MatchBox.stoodAt) + hold)
        } catch {
            return
        }
        guard firstRunPlayStart == start else { return }
        firstRunPlayStart = nil
        flags.markFirstRunPlayPlayed()
    }

    @ViewBuilder
    private func hostBlock(_ host: Host) -> some View {
        let status = sight.status(
            fingerprint: host.fingerprint,
            liveFingerprint: backHost?.fingerprint
        )
        let spoken = HomeCopy.hostRowSpoken(label: host.label, address: host.address, status: status)
        VStack(alignment: .leading, spacing: 4) {
            Button { onOpen(host) } label: {
                HStack {
                    Text(host.label)
                    Spacer()
                    statusWord(status)
                }
                .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
            }
            .accessibilityLabel(spoken)
            .accessibilityAction(named: Text("Remove")) { pendingRemove = host }
            Text(host.address)
                .font(.footnote)
                .foregroundStyle(Theme.faint)
            if pendingRemove?.fingerprint == host.fingerprint {
                Text(HomeCopy.removeQuestion(label: host.label, isLive: backHost?.fingerprint == host.fingerprint))
                    .foregroundStyle(Theme.muted)
                    .font(.footnote)
                HStack(spacing: 24) {
                    Button { confirmRemove(host) } label: { tall("remove") }
                        .foregroundStyle(Theme.neg)
                    Button { pendingRemove = nil } label: { tall("keep") }
                        .foregroundStyle(Theme.muted)
                }
            }
        }
        .gesture(
            DragGesture(minimumDistance: 24).onEnded { value in
                if value.translation.width < -40 { pendingRemove = host }
            }
        )
    }

    @ViewBuilder
    private func statusWord(_ status: HostRowStatus) -> some View {
        switch status {
        case .live:
            Text("● live").foregroundStyle(Theme.accent)
        case .notReachable(let since):
            Text("○ not reachable · since \(HomeCopy.clock(since))")
                .foregroundStyle(Theme.faint)
                .font(.footnote)
        case .lastSeen(let at):
            Text(HomeCopy.relative(from: at, to: Date()))
                .foregroundStyle(Theme.faint)
                .font(.footnote)
        case .unknown:
            EmptyView()
        }
    }

    private var footer: some View {
        let version = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? ""
        return HStack(spacing: 8) {
            if HomeCopy.showDemoInFooter(hosts: hosts) {
                Button(action: onDemo) { tall("demo") }
                    .accessibilityLabel("Try the demo")
                Text("·").foregroundStyle(Theme.faint)
            }
            Link(destination: URL(string: "https://docs.matchblox.com/privacy")!) { tall("privacy ↗") }
                .accessibilityLabel("Privacy policy")
            Text("·").foregroundStyle(Theme.faint)
            Link(destination: URL(string: "https://github.com/sponsors/blox-eng")!) { tall("♥ sponsor ↗") }
                .accessibilityLabel("Sponsor Matchblox")
            if !version.isEmpty {
                Text("  \(version)").foregroundStyle(Theme.faint)
            }
        }
        .font(.footnote)
        .foregroundStyle(Theme.faint)
        .frame(minHeight: 44, alignment: .leading)
    }

    /// A label whose tap area is at least 44 pt tall (spec 6.4); the frame on the button alone
    /// leaves the tap area at the text's height.
    private func tall(_ text: String, width: CGFloat? = nil) -> some View {
        Text(text)
            .frame(minWidth: width, minHeight: 44, alignment: .leading)
            .contentShape(Rectangle())
    }

    private func region<Content: View>(_ title: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title)
                .font(.caption)
                .foregroundStyle(Theme.faint)
            Rectangle()
                .fill(Theme.hair)
                .frame(height: 1)
            content()
        }
    }

    private func confirmRemove(_ host: Host) {
        do {
            try store.remove(fingerprint: host.fingerprint)
        } catch {
            problem = "This device could not save the list. Free some space on this device, then try again."
            hosts = store.all()
            pendingRemove = nil
            return
        }
        if HomeRemove.forgetsDeviceKey(host) {
            try? DeviceKey.remove(host: host.fingerprint)
            IslandSettings.forget(host: host)
            if let kept = IslandSettings.channel(host: host) {
                Task { await Registration.dropChannel(kept, host: host, removed: true) }
            }
        }
        problem = nil
        pendingRemove = nil
        removedNotice = Opening.removedNotice([host])
        hosts = store.all()
        onHostRemoved(host)
    }
}
