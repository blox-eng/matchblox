import ActivityKit
import MatchbloxKit
import SwiftUI
import UserNotifications

/// The Island setting (design 0010 §4.3). Show changes only the phone; Follow and Buzz
/// register again with the hosts concerned, with one Face ID.
struct IslandView: View {
    let store: HostStore
    let onBack: () -> Void

    @State private var hosts: [Host] = []
    @State private var prefs = IslandSettings.prefs
    @State private var notChanged: [IslandChange.Notice] = []
    @State private var blocked: [String] = []
    @State private var pending: Set<String> = []
    @State private var task: Task<Void, Never>?
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Island")
                .font(.title3)
            ForEach(blocked, id: \.self) { line in
                Text(line)
                    .foregroundStyle(Theme.muted)
            }
            ForEach(notChanged, id: \.self) { notice in
                Text(notice.line)
                    .foregroundStyle(Theme.muted)
            }
            Form {
                Section("Follow") {
                    Picker("Follow", selection: follow) {
                        Text("None").tag("")
                        ForEach(hosts) { host in
                            Text(host.label).tag(host.fingerprint)
                        }
                    }
                    .pickerStyle(.inline)
                    .labelsHidden()
                }
                .listRowBackground(Theme.bg)
                Section("Show") {
                    Picker("Show", selection: $prefs.show) {
                        Text("Sessions").tag(Show.sessions)
                        Text("Usage").tag(Show.usage)
                        Text("Auto (Usage while the stoker runs)").tag(Show.auto)
                    }
                    .pickerStyle(.inline)
                    .labelsHidden()
                }
                .listRowBackground(Theme.bg)
                Section("Buzz") {
                    ForEach(hosts) { host in
                        Toggle(host.label, isOn: buzz(host))
                    }
                }
                .listRowBackground(Theme.bg)
            }
            .scrollContentBackground(.hidden)
            Button("Back", action: onBack)
                .foregroundStyle(Theme.muted)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(24)
        .onAppear {
            hosts = store.all()
            prefs = IslandSettings.prefs
        }
        .task { await checkBlocked() }
        .onChange(of: scenePhase) { _, phase in
            switch phase {
            case .background:
                // Nothing connects from the background; the hosts not yet changed stay pending.
                task?.cancel()
            case .active:
                // The builder may come back from Settings.
                Task { await checkBlocked() }
                if task?.isCancelled == true { schedule() }
            default:
                break
            }
        }
        .onChange(of: prefs) { old, new in
            IslandSettings.prefs = new
            if old.show != new.show || old.follow != new.follow {
                IslandSettings.refreshActivities()
            }
            let concerned = IslandPrefs.concerned(old, new, hosts: hosts.map(\.fingerprint))
            guard !concerned.isEmpty else { return }
            pending.formUnion(concerned)
            schedule()
        }
    }

    private var follow: Binding<String> {
        Binding(
            get: { prefs.follow.flatMap { fp in hosts.contains { $0.fingerprint == fp } ? fp : nil } ?? "" },
            set: { prefs.choice = $0.isEmpty ? FollowChoice.none : .host($0) }
        )
    }

    private func buzz(_ host: Host) -> Binding<Bool> {
        Binding(
            get: { prefs.buzzOn(host.fingerprint) },
            set: { prefs.buzz[host.fingerprint] = $0 }
        )
    }

    /// A short pause gathers quick changes into one Face ID. A change waits for the one it
    /// cancels, which puts back the hosts it did not try.
    private func schedule() {
        let previous = task
        previous?.cancel()
        task = Task {
            _ = await previous?.value
            try? await Task.sleep(for: .milliseconds(800))
            guard !Task.isCancelled, !pending.isEmpty else { return }
            let due = hosts.filter { pending.contains($0.fingerprint) }
            pending = []
            let change = await Registration.afterChange(hosts: due)
            let untried = Set(change.untried.map(\.fingerprint))
            pending.formUnion(untried)
            let tried = Set(due.filter { !untried.contains($0.fingerprint) }.map(\.label))
            notChanged = notChanged.filter { !tried.contains($0.label) } + change.notChanged
        }
    }

    /// Buzz and the island need what iOS can keep from the phone: notifications and Live Activities.
    private func checkBlocked() async {
        let status = await UNUserNotificationCenter.current().notificationSettings().authorizationStatus
        blocked = Opening.blocked(
            notificationsDenied: status == .denied,
            activitiesEnabled: ActivityAuthorizationInfo().areActivitiesEnabled
        )
    }
}
