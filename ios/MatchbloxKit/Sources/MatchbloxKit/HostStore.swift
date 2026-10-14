import Foundation

public struct Host: Codable, Equatable, Identifiable, Sendable {
    public var label: String
    public var address: String
    public var port: Int
    public var user: String
    public var fingerprint: String

    public var id: String { fingerprint }

    public init(label: String, address: String, port: Int, user: String, fingerprint: String) {
        self.label = label
        self.address = address
        self.port = port
        self.user = user
        self.fingerprint = fingerprint
    }
}

/// The paired hosts, as one JSON file. A host is known by its key fingerprint, not by its address.
public final class HostStore: Sendable {
    public let file: URL

    public static let standard = HostStore(file: .applicationSupportDirectory.appendingPathComponent("hosts.json"))

    public init(file: URL) {
        self.file = file
    }

    /// A missing or damaged file gives no hosts; the next write replaces it.
    public func all() -> [Host] {
        guard let data = try? Data(contentsOf: file) else { return [] }
        return (try? JSONDecoder().decode([Host].self, from: data)) ?? []
    }

    public func upsert(_ host: Host) throws {
        var hosts = all()
        if let index = hosts.firstIndex(where: { $0.fingerprint == host.fingerprint }) {
            hosts[index] = host
        } else {
            hosts.append(host)
        }
        try save(hosts)
    }

    public func updateAddress(fingerprint: String, address: String) throws {
        var hosts = all()
        guard let index = hosts.firstIndex(where: { $0.fingerprint == fingerprint }) else { return }
        hosts[index].address = address
        try save(hosts)
    }

    public func remove(fingerprint: String) throws {
        try save(all().filter { $0.fingerprint != fingerprint })
    }

    private func save(_ hosts: [Host]) throws {
        try FileManager.default.createDirectory(at: file.deletingLastPathComponent(), withIntermediateDirectories: true)
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        try encoder.encode(hosts).write(to: file, options: .atomic)
    }
}
