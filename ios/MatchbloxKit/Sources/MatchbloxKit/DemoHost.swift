import Foundation

extension Host {
    /// Not an SSH key fingerprint, so it can never equal the key of a real host. The demo keeps the Host format: an older build reads its row like any other.
    public static let demoFingerprint = "demo:this-phone"

    public static let demo = Host(label: "demo", address: "this phone", port: 0, user: "", fingerprint: demoFingerprint)

    public var isDemo: Bool { fingerprint == Self.demoFingerprint }
}
