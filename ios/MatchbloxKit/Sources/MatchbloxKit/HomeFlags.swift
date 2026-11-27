import Foundation

/// “Once” flags for Pocket home: the first-run mark play and the BOX teach.
public struct HomeFlags {
    private let defaults: UserDefaults

    private static let firstRunKey = "pocket.home.firstRunPlayPlayed"
    private static let boxTeachKey = "pocket.home.boxTaught"

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    public var shouldPlayFirstRun: Bool {
        !defaults.bool(forKey: Self.firstRunKey)
    }

    public var shouldTeachBox: Bool {
        !defaults.bool(forKey: Self.boxTeachKey)
    }

    public mutating func markFirstRunPlayPlayed() {
        defaults.set(true, forKey: Self.firstRunKey)
    }

    public mutating func markBoxTaught() {
        defaults.set(true, forKey: Self.boxTeachKey)
    }
}
