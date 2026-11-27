import Foundation

/// The dock head's colour at rest, set on home under `its colour`. Only the cold head changes:
/// the ember, the amber and the flame's core of a fast roll are the same for every pigment.
public enum HeadPigment: String, CaseIterable, Sendable {
    case matchRed = "match-red"
    case strikeAnywhereBrown = "strike-anywhere-brown"
    case blueTip = "blue-tip"
    case greenTip = "green-tip"
    case charcoal

    public struct Colour: Equatable, Sendable {
        public let red, green, blue: Double

        public init(hex: UInt32) {
            red = Double((hex >> 16) & 0xFF) / 255
            green = Double((hex >> 8) & 0xFF) / 255
            blue = Double(hex & 0xFF) / 255
        }

        init(red: Double, green: Double, blue: Double) {
            self.red = red
            self.green = green
            self.blue = blue
        }

        /// Exactly `self` at 0 and exactly `other` at 1.
        func mixed(with other: Colour, by t: Double) -> Colour {
            let t = min(max(t, 0), 1)
            return Colour(red: red * (1 - t) + other.red * t,
                          green: green * (1 - t) + other.green * t,
                          blue: blue * (1 - t) + other.blue * t)
        }
    }

    /// The UserDefaults key of the chosen pigment.
    public static let storeKey = "head.colour"

    /// The heat colours, as the app's tokens `match-ember`, `flame-amber` and `flame-core`.
    public static let ember = Colour(hex: 0x9C5A46)
    public static let amber = Colour(hex: 0xF5A54A)
    public static let core = Colour(hex: 0xFFD27A)
    private static let black = Colour(hex: 0x000000)
    /// How much darker the rim is than the face once the head warms.
    private static let rimDark = 0.38

    public var name: String {
        switch self {
        case .matchRed: "match red"
        case .strikeAnywhereBrown: "strike-anywhere brown"
        case .blueTip: "blue tip"
        case .greenTip: "green tip"
        case .charcoal: "charcoal"
        }
    }

    /// The face of the cold head.
    public var cold: Colour {
        switch self {
        case .matchRed: Colour(hex: 0x9E2319)
        case .strikeAnywhereBrown: Colour(hex: 0x5E3220)
        case .blueTip: Colour(hex: 0x2B467F)
        case .greenTip: Colour(hex: 0x2E5E37)
        case .charcoal: Colour(hex: 0x34302C)
        }
    }

    /// The rim of the cold head, where it turns from the light.
    public var shade: Colour {
        switch self {
        case .matchRed: Colour(hex: 0x5A120C)
        case .strikeAnywhereBrown: Colour(hex: 0x2E170E)
        case .blueTip: Colour(hex: 0x141F3D)
        case .greenTip: Colour(hex: 0x14301B)
        case .charcoal: Colour(hex: 0x151311)
        }
    }

    /// The face from cold through ember and amber to the flame's core.
    public func face(heat: Double) -> Colour {
        switch heat {
        case ..<0.33: cold.mixed(with: Self.ember, by: heat / 0.33)
        case ..<0.75: Self.ember.mixed(with: Self.amber, by: (heat - 0.33) / 0.42)
        default: Self.amber.mixed(with: Self.core, by: (heat - 0.75) / 0.25)
        }
    }

    /// The rim: the pigment's shade at rest, the warm face darkened once the head is ember.
    public func shade(heat: Double) -> Colour {
        let warm = face(heat: heat).mixed(with: Self.black, by: Self.rimDark)
        return heat < 0.33 ? shade.mixed(with: warm, by: heat / 0.33) : warm
    }

    public static func load(from store: UserDefaults) -> HeadPigment {
        store.string(forKey: storeKey).flatMap(HeadPigment.init(rawValue:)) ?? .matchRed
    }

    public func save(to store: UserDefaults) {
        store.set(rawValue, forKey: Self.storeKey)
    }
}
