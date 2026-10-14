import CryptoKit
import Foundation
import LocalAuthentication
import MatchbloxKit
import NIOSSH
import Security

enum DeviceKeyError: Error, Equatable {
    case noPasscode
    case keyUnavailable
    case locked
    case keychain(OSStatus)
}

extension DeviceKeyError: LocalizedError {
    var errorDescription: String? {
        switch self {
        case .noPasscode:
            "This iPhone has no passcode. Set a passcode in Settings, then try again."
        case .keyUnavailable:
            "The phone key cannot be used. Unlock the iPhone and try again. If it still fails, remove the app and install it again."
        case .locked:
            "The iPhone is locked. Unlock the iPhone, then try again."
        case .keychain(let status):
            "The Keychain refused the phone key (error \(status)). Restart the iPhone, then try again."
        }
    }
}

enum DeviceKey {
    private static let service = "com.blox.matchblox.device"

    static func loadOrCreate(context: LAContext? = nil) throws -> SecureEnclave.P256.Signing.PrivateKey {
        if let stored = try storedRepresentation() {
            return try load(stored, context: context)
        }
        var probeError: NSError?
        guard LAContext().canEvaluatePolicy(.deviceOwnerAuthentication, error: &probeError) else {
            throw DeviceKeyError.noPasscode
        }
        var error: Unmanaged<CFError>?
        guard let access = SecAccessControlCreateWithFlags(
            nil,
            kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
            [.privateKeyUsage, .biometryAny, .or, .devicePasscode],
            &error
        ) else { throw DeviceKeyError.noPasscode }
        let key: SecureEnclave.P256.Signing.PrivateKey
        do {
            key = try SecureEnclave.P256.Signing.PrivateKey(
                accessControl: access,
                authenticationContext: context
            )
        } catch {
            throw DeviceKeyError.noPasscode
        }
        let status = add(key.dataRepresentation)
        if status == errSecDuplicateItem, let stored = try storedRepresentation() {
            return try load(stored, context: context)
        }
        guard status == errSecSuccess else { throw keychainError(status) }
        return key
    }

    static func openSSH() throws -> String {
        let key = try loadOrCreate()
        return SSHKeys.openSSH(p256X963: key.publicKey.x963Representation, comment: "")
    }

    static func nioKey(_ key: SecureEnclave.P256.Signing.PrivateKey) -> NIOSSHPrivateKey {
        NIOSSHPrivateKey(secureEnclaveP256Key: key)
    }

    private static func load(
        _ representation: Data,
        context: LAContext?
    ) throws -> SecureEnclave.P256.Signing.PrivateKey {
        do {
            return try SecureEnclave.P256.Signing.PrivateKey(
                dataRepresentation: representation,
                authenticationContext: context
            )
        } catch {
            throw DeviceKeyError.keyUnavailable
        }
    }

    private static func keychainError(_ status: OSStatus) -> DeviceKeyError {
        status == errSecInteractionNotAllowed ? .locked : .keychain(status)
    }

    private static var identity: [CFString: Any] {
        [kSecClass: kSecClassGenericPassword, kSecAttrService: service]
    }

    private static func add(_ representation: Data) -> OSStatus {
        var item = identity
        item[kSecValueData] = representation
        item[kSecAttrAccessible] = kSecAttrAccessibleWhenUnlockedThisDeviceOnly
        return SecItemAdd(item as CFDictionary, nil)
    }
}
