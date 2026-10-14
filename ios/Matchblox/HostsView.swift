import MatchbloxKit
import SwiftUI

struct HostsView: View {
    let store: HostStore
    let onOpen: (Host) -> Void
    let onPair: () -> Void
    let onNoHosts: () -> Void

    @State private var hosts: [Host] = []
    @State private var removed: String?
    @State private var problem: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Hosts")
                .font(.title3)
            if let removed {
                Text("\(removed) is removed. Remove its key on the host with matchblox devices rm.")
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
                    .listRowBackground(Theme.bg)
                    .listRowSeparatorTint(Theme.hair)
                }
                .onDelete(perform: remove)
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
            Button("+ Pair a host", action: onPair)
                .foregroundStyle(Theme.accent)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(24)
        .onAppear { hosts = store.all() }
    }

    private func remove(at offsets: IndexSet) {
        let gone = offsets.map { hosts[$0] }
        do {
            for host in gone {
                try store.remove(fingerprint: host.fingerprint)
            }
        } catch {
            problem = "The phone could not save the list. Free some space on the phone, then try again."
            return
        }
        problem = nil
        removed = gone.map(\.label).joined(separator: ", ")
        hosts = store.all()
        if hosts.isEmpty { onNoHosts() }
    }
}
