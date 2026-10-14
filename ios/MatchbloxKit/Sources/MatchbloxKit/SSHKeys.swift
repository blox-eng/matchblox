import CryptoKit
import Foundation
import NIOSSH

public enum SSHKeyError: Error, Equatable, Sendable {
    case badKeyLine
    case badSeed
}

public enum SSHKeys {
    private static let p256Name = "ecdsa-sha2-nistp256"
    private static let p256Curve = "nistp256"

    public static func openSSH(p256X963: Data, comment: String) -> String {
        var blob = Data()
        for field in [Data(p256Name.utf8), Data(p256Curve.utf8), p256X963] {
            withUnsafeBytes(of: UInt32(field.count).bigEndian) { blob.append(contentsOf: $0) }
            blob.append(field)
        }
        let line = "\(p256Name) \(blob.base64EncodedString())"
        return comment.isEmpty ? line : "\(line) \(comment)"
    }

    public static func fingerprint(openSSH line: String) throws(SSHKeyError) -> String {
        let fields = line.split(whereSeparator: \.isWhitespace)
        guard fields.count >= 2,
              let blob = Data(base64Encoded: String(fields[1])),
              !blob.isEmpty
        else { throw .badKeyLine }
        return fingerprint(blob: blob)
    }

    public static func fingerprint(hostKey: NIOSSHPublicKey) -> String {
        let line = String(openSSHPublicKey: hostKey)
        let blob = line.split(separator: " ").dropFirst().first
            .flatMap { Data(base64Encoded: String($0)) }
        return fingerprint(blob: blob ?? Data())
    }

    public static func oneTimeKey(seedBase64: String) throws(SSHKeyError) -> NIOSSHPrivateKey {
        guard let seed = Data(base64Encoded: seedBase64),
              let key = try? Curve25519.Signing.PrivateKey(rawRepresentation: seed)
        else { throw .badSeed }
        return NIOSSHPrivateKey(ed25519Key: key)
    }

    private static func fingerprint(blob: Data) -> String {
        let digest = SHA256.hash(data: blob)
        let encoded = Data(digest).base64EncodedString().replacingOccurrences(of: "=", with: "")
        return "SHA256:\(encoded)"
    }
}
