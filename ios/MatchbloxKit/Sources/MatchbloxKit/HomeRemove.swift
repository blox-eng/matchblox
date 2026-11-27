import Foundation

/// What Pocket home does after a confirmed host remove (plan Task 5 Surface).
public enum HomeRemove {
    /// A real host's Secure Enclave key must be forgotten; the demo has none.
    public static func forgetsDeviceKey(_ host: Host) -> Bool { !host.isDemo }

    /// `‹ host` clears when the removed host was the live console behind home.
    public static func clearsBack(removedFingerprint: String, backFingerprint: String?) -> Bool {
        backFingerprint == removedFingerprint
    }
}
