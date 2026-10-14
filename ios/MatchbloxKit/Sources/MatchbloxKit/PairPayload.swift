import Foundation

public enum PairError: Error, Equatable, Sendable {
    case notMatchblox
    case version(Int)
    case malformed
}

public struct PairPayload: Codable, Equatable, Sendable {
    public var v: Int
    public var host: String
    public var port: Int
    public var user: String
    public var name: String
    public var hostkey: String
    public var key: String

    public init(url: URL) throws(PairError) {
        guard let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
              components.scheme?.lowercased() == "matchblox",
              components.host == "pair"
        else { throw .notMatchblox }

        guard let encoded = components.queryItems?.first(where: { $0.name == "p" })?.value,
              !encoded.isEmpty,
              let json = Self.decodeBase64URL(encoded),
              let raw = try? JSONDecoder().decode(Raw.self, from: json)
        else { throw .malformed }

        let version = raw.v ?? 0
        guard version == 1 else { throw .version(version) }

        guard let host = raw.host, !host.isEmpty,
              let user = raw.user, !user.isEmpty,
              let name = raw.name, !name.isEmpty,
              let hostkey = raw.hostkey, !hostkey.isEmpty,
              let key = raw.key, !key.isEmpty,
              let port = raw.port, (1...65535).contains(port),
              Data(base64Encoded: key)?.count == 32
        else { throw .malformed }

        self.init(v: version, host: host, port: port, user: user, name: name, hostkey: hostkey, key: key)
    }

    public init(v: Int, host: String, port: Int, user: String, name: String, hostkey: String, key: String) {
        self.v = v
        self.host = host
        self.port = port
        self.user = user
        self.name = name
        self.hostkey = hostkey
        self.key = key
    }

    private struct Raw: Decodable {
        let v: Int?
        let host: String?
        let port: Int?
        let user: String?
        let name: String?
        let hostkey: String?
        let key: String?
    }

    private static func decodeBase64URL(_ text: String) -> Data? {
        guard !text.contains("="), text.count % 4 != 1 else { return nil }
        let standard = text
            .replacingOccurrences(of: "-", with: "+")
            .replacingOccurrences(of: "_", with: "/")
        return Data(base64Encoded: standard + String(repeating: "=", count: (4 - standard.count % 4) % 4))
    }
}
