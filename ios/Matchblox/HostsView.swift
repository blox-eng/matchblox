import MatchbloxKit
import SwiftUI

/// Pocket home: hosts first, then Live Activity, the head, text size, quiet footer.
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
    @State private var marks = HeadMarks.load(from: .standard)
    /// The place of the head whose keys show; nil shows none.
    @State private var editing: HeadMarks.Place?
    @State private var pigment = HeadPigment.load(from: .standard)
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
                        showLine
                    }
                }

                region("THE HEAD") {
                    headDrawing
                    if let editing { keyList(for: editing) }
                    HStack(spacing: 24) {
                        Button { change { $0.undo() } } label: { tall("undo") }
                        Button { change { $0.asItCame() } } label: { tall("as it came") }
                    }
                    .buttonStyle(.borderless)
                    .foregroundStyle(Theme.muted)
                    colourRow
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
            marks = HeadMarks.load(from: .standard)
            pigment = HeadPigment.load(from: .standard)
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

    /// One tap picks what the island and the lock screen show; the phone alone keeps it.
    private var showLine: some View {
        HStack(spacing: 12) {
            Text("show").foregroundStyle(Theme.faint)
            ForEach(HomeCopy.showChoices, id: \.self) { choice in
                Button { pickShow(choice) } label: { tall(choice.rawValue) }
                    .foregroundStyle(prefs.show == choice ? Theme.accent : Theme.faint)
                    .accessibilityLabel(HomeCopy.showSpoken(choice, selected: prefs.show == choice))
            }
        }
        .buttonStyle(.borderless)
    }

    private func pickShow(_ choice: Show) {
        var saved = IslandSettings.prefs
        guard saved.show != choice else { return }
        saved.show = choice
        IslandSettings.prefs = saved
        prefs = saved
        IslandSettings.refreshActivities()
    }

    // MARK: The head

    private static let interruptReason = "stays on the belt: a roll can brush the edge"

    /// The head as the dock draws it, with its four marks round it and its hold in its middle.
    /// A tap on one shows the keys it can take.
    private var headDrawing: some View {
        VStack(spacing: 4) {
            markButton(.top)
            HStack(spacing: 8) {
                markButton(.left)
                Button { toggle(.hold) } label: {
                    ZStack {
                        head(pigment, side: 72)
                        if editing == .hold {
                            Circle().stroke(Theme.accent, lineWidth: 2).padding(-4)
                        }
                        VStack(spacing: 0) {
                            Text(marks[.hold])
                            Text(KeyFace.word(of: marks[.hold]) ?? "").font(.caption2)
                            Text("hold").font(.caption2).opacity(0.7)
                        }
                        .foregroundStyle(Theme.fg)
                    }
                    .frame(width: 80, height: 80)
                    .contentShape(Circle())
                }
                .accessibilityLabel("hold, " + (KeyFace.word(of: marks[.hold]) ?? marks[.hold]))
                .accessibilityIdentifier("home-head-hold")
                .accessibilityAddTraits(editing == .hold ? .isSelected : [])
                markButton(.right)
            }
            markButton(.bottom)
        }
        .frame(maxWidth: .infinity)
        .buttonStyle(.borderless)
        .font(.footnote.monospaced())
    }

    private func markButton(_ place: HeadMarks.Place) -> some View {
        let id = marks[place]
        return Button { toggle(place) } label: {
            Text(Self.face(id))
                .frame(minWidth: 88, minHeight: 44)
                .contentShape(Rectangle())
        }
        .foregroundStyle(editing == place ? Theme.accent : Theme.muted)
        .accessibilityLabel("\(place.rawValue) mark, " + (KeyFace.word(of: id) ?? id))
        .accessibilityIdentifier("home-head-\(place.rawValue)")
        .accessibilityAddTraits(editing == place ? .isSelected : [])
    }

    /// The keys a place can take, two to a row, then `^C`, dim, with why it cannot.
    private func keyList(for place: HeadMarks.Place) -> some View {
        let keys = HeadMarks.keys(for: place)
        let rows = stride(from: 0, to: keys.count, by: 2).map { Array(keys[$0..<min($0 + 2, keys.count)]) }
        return VStack(alignment: .leading, spacing: 0) {
            Text(place == .hold ? "the hold sends" : "the \(place.rawValue) mark sends")
                .font(.footnote)
                .foregroundStyle(Theme.faint)
            ForEach(rows, id: \.self) { row in
                HStack(spacing: 16) {
                    ForEach(row, id: \.self) { id in
                        Button { set(id, at: place) } label: {
                            tall(Self.face(id)).frame(maxWidth: .infinity, alignment: .leading)
                        }
                        .foregroundStyle(marks[place] == id ? Theme.accent : Theme.fg)
                        .accessibilityAddTraits(marks[place] == id ? .isSelected : [])
                    }
                    if row.count < 2 { Spacer().frame(maxWidth: .infinity) }
                }
            }
            Button {} label: { tall(Self.face("^C")) }
                .disabled(true)
            Text(Self.interruptReason)
                .font(.footnote)
                .foregroundStyle(Theme.faint)
        }
        .buttonStyle(.borderless)
        .font(.body.monospaced())
    }

    /// Five small heads; one tap colours the dock's head at once. VoiceOver hears them as one
    /// group of choices, the chosen one selected.
    private var colourRow: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("its colour").foregroundStyle(Theme.faint)
            HStack(spacing: 4) {
                ForEach(HeadPigment.allCases, id: \.self) { choice in
                    Button { pick(choice) } label: {
                        head(choice, side: 28)
                            .overlay {
                                if choice == pigment {
                                    Circle().stroke(Theme.accent, lineWidth: 2).padding(-4)
                                }
                            }
                            .frame(width: 44, height: 44)
                            .contentShape(Rectangle())
                    }
                    .accessibilityLabel(choice.name)
                    .accessibilityAddTraits(choice == pigment ? .isSelected : [])
                }
            }
            .buttonStyle(.borderless)
            .accessibilityElement(children: .contain)
            .accessibilityLabel("its colour")
            Text(pigment.name)
                .font(.footnote)
                .foregroundStyle(Theme.muted)
                .accessibilityHidden(true)
        }
    }

    /// A cold head: the pigment's face, falling to its shade at the rim.
    private func head(_ pigment: HeadPigment, side: CGFloat) -> some View {
        Circle()
            .fill(RadialGradient(colors: [Self.color(pigment.cold), Self.color(pigment.shade)],
                                 center: UnitPoint(x: 0.35, y: 0.35), startRadius: 0, endRadius: side * 0.75))
            .frame(width: side, height: side)
            .accessibilityHidden(true)
    }

    private static func color(_ colour: HeadPigment.Colour) -> Color {
        Color(red: colour.red, green: colour.green, blue: colour.blue)
    }

    /// A mark as the dock shows it: `Esc cancel`.
    private static func face(_ id: String) -> String {
        KeyFace.word(of: id).map { "\(id) \($0)" } ?? id
    }

    private func toggle(_ place: HeadMarks.Place) {
        editing = editing == place ? nil : place
    }

    /// The list offers only the keys the place takes, so the set cannot be refused.
    private func set(_ id: String, at place: HeadMarks.Place) {
        change { edited in _ = try? edited.set(id, at: place) }
        editing = nil
    }

    private func change(_ edit: (inout HeadMarks) -> Void) {
        edit(&marks)
        marks.save(to: .standard)
    }

    private func pick(_ choice: HeadPigment) {
        pigment = choice
        choice.save(to: .standard)
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
