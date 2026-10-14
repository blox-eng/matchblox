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

    private var damagedFile: URL { file.appendingPathExtension("damaged") }

    /// A missing or damaged file gives no hosts. The next write moves a damaged file aside to `hosts.json.damaged`.
    public func all() -> [Host] {
        guard let data = try? Data(contentsOf: file) else { return [] }
        return (try? JSONDecoder().decode([Host].self, from: data)) ?? []
    }

    /// Adds the host, or updates the entry with its fingerprint. An entry with the same label, address and user but another fingerprint is the host before its host key changed: the host replaces it. Returns those stale entries, so their keys can go too.
    @discardableResult
    public func upsert(_ host: Host) throws -> [Host] {
        var hosts = all()
        let stale = hosts.filter {
            $0.fingerprint != host.fingerprint && $0.label == host.label && $0.address == host.address && $0.user == host.user
        }
        let replaces = { (entry: Host) in entry.fingerprint == host.fingerprint || stale.contains(entry) }
        let index = hosts.firstIndex(where: replaces)
        hosts.removeAll(where: replaces)
        hosts.insert(host, at: index ?? hosts.count)
        try save(hosts)
        return stale
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
        try moveAsideIfDamaged()
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        try encoder.encode(hosts).write(to: file, options: .atomic)
    }

    private func moveAsideIfDamaged() throws {
        guard let data = try? Data(contentsOf: file),
              (try? JSONDecoder().decode([Host].self, from: data)) == nil
        else { return }
        try? FileManager.default.removeItem(at: damagedFile)
        try FileManager.default.moveItem(at: file, to: damagedFile)
    }
}
