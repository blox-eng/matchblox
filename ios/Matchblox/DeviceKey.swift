import CryptoKit
import Foundation
import LocalAuthentication
import MatchbloxKit
import NIOSSH
import Security

enum DeviceKeyError: Error, Equatable {
    case noAccessControl
    case keychain(OSStatus)
}

extension DeviceKeyError: LocalizedError {
    var errorDescription: String? {
        switch self {
        case .noAccessControl:
            "This phone cannot make a protected key. Set a passcode on the phone, then try again."
        case .keychain(let status):
            "The phone key is not readable (Keychain error \(status)). Unlock the phone, then try again."
        }
    }
}

enum DeviceKey {
    private static let service = "com.blox.matchblox.device"

    static func loadOrCreate(context: LAContext? = nil) throws -> SecureEnclave.P256.Signing.PrivateKey {
        if let stored = try storedRepresentation() {
            return try SecureEnclave.P256.Signing.PrivateKey(
                dataRepresentation: stored,
                authenticationContext: context
            )
        }
        var error: Unmanaged<CFError>?
        guard let access = SecAccessControlCreateWithFlags(
            nil,
            kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
            [.privateKeyUsage, .biometryAny, .or, .devicePasscode],
            &error
        ) else { throw DeviceKeyError.noAccessControl }
        let key = try SecureEnclave.P256.Signing.PrivateKey(
            accessControl: access,
            authenticationContext: context
        )
        try store(key.dataRepresentation)
        return key
    }

    static func openSSH(name: String) throws -> String {
        let key = try loadOrCreate()
        return SSHKeys.openSSH(p256X963: key.publicKey.x963Representation, comment: name)
    }

    static func nioKey(_ key: SecureEnclave.P256.Signing.PrivateKey) -> NIOSSHPrivateKey {
        NIOSSHPrivateKey(secureEnclaveP256Key: key)
    }

    private static var identity: [CFString: Any] {
        [kSecClass: kSecClassGenericPassword, kSecAttrService: service]
    }

    private static func storedRepresentation() throws -> Data? {
        var query = identity
        query[kSecReturnData] = true
        var item: CFTypeRef?
        switch SecItemCopyMatching(query as CFDictionary, &item) {
        case errSecSuccess: return item as? Data
        case errSecItemNotFound: return nil
        case let status: throw DeviceKeyError.keychain(status)
        }
    }

    private static func store(_ representation: Data) throws {
        var item = identity
        item[kSecValueData] = representation
        item[kSecAttrAccessible] = kSecAttrAccessibleWhenUnlockedThisDeviceOnly
        let status = SecItemAdd(item as CFDictionary, nil)
        guard status == errSecSuccess else { throw DeviceKeyError.keychain(status) }
    }
}
