import MatchbloxKit
import SwiftUI

struct HostsView: View {
    let store: HostStore
    let onOpen: (Host) -> Void
    let onPair: () -> Void
    let onIsland: () -> Void
    let onDemo: () -> Void
    let onNoHosts: (String) -> Void

    @State private var hosts: [Host] = []
    @State private var removed: [Host] = []
    @State private var problem: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Hosts")
                .font(.title3)
            if !removed.isEmpty {
                Text(Opening.removedNotice(removed))
                    .foregroundStyle(Theme.muted)
            }
            if let problem {
                Text(problem)
                    .foregroundStyle(Theme.neg)
            }
            List {
                ForEach(hosts) { host in
                    Button { onOpen(host) } label: {
                        VStack(alignment: .leading, spacing: 4) {
                            Text(host.label)
                            Text(host.address)
                                .font(.footnote)
                                .foregroundStyle(Theme.faint)
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .contextMenu {
                        if !host.isDemo { Button("Island", action: onIsland) }
                    }
                    .listRowBackground(Theme.bg)
                    .listRowSeparatorTint(Theme.hair)
                }
                .onDelete(perform: remove)
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
            // Wraps under a large text size instead of running off the screen.
            ViewThatFits(in: .horizontal) {
                HStack(spacing: 24) { actions }
                VStack(alignment: .leading, spacing: 0) { actions }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(24)
        .onAppear { hosts = store.all() }
    }

    @ViewBuilder
    private var actions: some View {
        Button("+ Pair a host", action: onPair)
            .frame(minHeight: 44)
            .foregroundStyle(Theme.accent)
        if hosts.contains(where: { !$0.isDemo }) {
            Button("Island", action: onIsland)
                .frame(minHeight: 44)
                .foregroundStyle(Theme.muted)
        }
        if !hosts.contains(where: \.isDemo) {
            Button("Try a demo", action: onDemo)
                .frame(minHeight: 44)
                .foregroundStyle(Theme.muted)
        }
    }

    private func remove(at offsets: IndexSet) {
        let gone = offsets.map { hosts[$0] }
        do {
            for host in gone {
                try store.remove(fingerprint: host.fingerprint)
            }
        } catch {
            problem = "This device could not save the list. Free some space on this device, then try again."
            hosts = store.all()
            return
        }
        for host in gone where !host.isDemo {
            try? DeviceKey.remove(host: host.fingerprint)
            IslandSettings.forget(host: host)
            if let kept = IslandSettings.channel(host: host) {
                Task { await Registration.dropChannel(kept, host: host, removed: true) }
            }
        }
        problem = nil
        removed = gone
        hosts = store.all()
        if hosts.isEmpty { onNoHosts(Opening.removedNotice(gone)) }
    }
}
