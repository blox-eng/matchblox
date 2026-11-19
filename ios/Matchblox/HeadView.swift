import CoreHaptics
import UIKit

/// The match head on the belt: a ball whose speckles turn with the thumb's roll, cold red at
/// rest, ember, then amber as it heats, and alight when it strikes (DESIGN.md §2, the match).
final class HeadLayer: CALayer {
    private struct Speck {
        let x, y, z: CGFloat
        let size: CGFloat
    }

    private static let specks: [Speck] = {
        var generator = SystemRandomNumberGenerator()
        return (0..<70).map { _ in
            let u = CGFloat.random(in: 0..<(2 * .pi), using: &generator)
            let v = acos(CGFloat.random(in: -1...1, using: &generator))
            return Speck(x: sin(v) * cos(u), y: cos(v), z: sin(v) * sin(u), size: .random(in: 0.4...1.0, using: &generator))
        }
    }()

    private static let cold = UIColor(Theme.matchHead)
    private static let warm = UIColor(Theme.matchEmber)
    private static let hot = UIColor(Theme.flameAmber)
    private static let white = UIColor(Theme.flameCore)

    /// Radians the head turned about its x axis (a roll up or down) and its y axis (sideways).
    var turnX: CGFloat = 0 { didSet { setNeedsDisplay() } }
    var turnY: CGFloat = 0 { didSet { setNeedsDisplay() } }
    var heat: CGFloat = 0 { didSet { if heat != oldValue { setNeedsDisplay() } } }
    var struck = false { didSet { if struck != oldValue { setNeedsDisplay() } } }

    override init() {
        super.init()
        needsDisplayOnBoundsChange = true
    }

    override init(layer: Any) {
        super.init(layer: layer)
    }

    required init?(coder: NSCoder) { nil }

    override func draw(in ctx: CGContext) {
        let r = min(bounds.width, bounds.height) / 2 - 2
        let c = CGPoint(x: bounds.midX, y: bounds.midY)
        let color = Self.color(heat: heat)
        if struck {
            let glow = [UIColor(Theme.flameCore).withAlphaComponent(0.9).cgColor, UIColor(Theme.flame).withAlphaComponent(0).cgColor]
            if let g = CGGradient(colorsSpace: nil, colors: glow as CFArray, locations: [0, 1]) {
                ctx.drawRadialGradient(g, startCenter: CGPoint(x: c.x, y: c.y - r * 0.3), startRadius: 0,
                                       endCenter: c, endRadius: r * 1.6, options: [])
            }
        }
        let lit = color.mixed(with: .white, by: 0.25).cgColor
        let dark = color.mixed(with: .black, by: 0.55).cgColor
        ctx.saveGState()
        ctx.addEllipse(in: CGRect(x: c.x - r, y: c.y - r, width: 2 * r, height: 2 * r))
        ctx.clip()
        if let g = CGGradient(colorsSpace: nil, colors: [lit, color.cgColor, dark] as CFArray, locations: [0, 0.6, 1]) {
            ctx.drawRadialGradient(g, startCenter: CGPoint(x: c.x - r * 0.35, y: c.y - r * 0.35), startRadius: 0,
                                   endCenter: c, endRadius: r, options: [.drawsAfterEndLocation])
        }
        let (sx, cx) = (sin(turnX), cos(turnX))
        let (sy, cy) = (sin(turnY), cos(turnY))
        for s in Self.specks {
            let y = s.y * cx - s.z * sx
            let z = s.y * sx + s.z * cx
            let x = s.x * cy + z * sy
            let depth = -s.x * sy + z * cy
            guard depth > 0 else { continue }
            let size = (s.size * depth + 0.3) * r / 22
            ctx.setFillColor(UIColor.black.withAlphaComponent(0.35 * depth).cgColor)
            ctx.fillEllipse(in: CGRect(x: c.x + x * r * 0.95 - size, y: c.y + y * r * 0.95 - size, width: 2 * size, height: 2 * size))
        }
        ctx.restoreGState()
        ctx.setStrokeColor(UIColor.black.withAlphaComponent(0.6).cgColor)
        ctx.setLineWidth(1)
        ctx.strokeEllipse(in: CGRect(x: c.x - r, y: c.y - r, width: 2 * r, height: 2 * r))
    }

    /// Cold red through ember and amber to the flame's core.
    private static func color(heat: CGFloat) -> UIColor {
        switch heat {
        case ..<0.33: cold.mixed(with: warm, by: heat / 0.33)
        case ..<0.75: warm.mixed(with: hot, by: (heat - 0.33) / 0.42)
        default: hot.mixed(with: white, by: min((heat - 0.75) / 0.25, 1))
        }
    }
}

/// What the thumb feels on the head: a short, sharp tap a notch, sharper as the head heats; one
/// hard tap when it strikes; one heavy bump at an end. A phone with no haptics plays nothing.
@MainActor
final class HeadFeel {
    private let engine: CHHapticEngine?

    init() {
        if CHHapticEngine.capabilitiesForHardware().supportsHaptics, let engine = try? CHHapticEngine() {
            engine.isAutoShutdownEnabled = true
            self.engine = engine
        } else {
            engine = nil
        }
    }

    /// The thumb is on the head: wake the engine before the first notch.
    func prepare() {
        try? engine?.start()
    }

    func notch(heat: CGFloat) {
        play(intensity: 0.5 + 0.4 * Float(heat), sharpness: 0.6 + 0.4 * Float(heat))
    }

    func strike() {
        play(intensity: 1, sharpness: 0.3)
    }

    func end() {
        play(intensity: 1, sharpness: 0.1)
    }

    /// A tap that cannot play is lost: the scroll it marks still happens.
    private func play(intensity: Float, sharpness: Float) {
        guard let engine else { return }
        let tap = CHHapticEvent(eventType: .hapticTransient, parameters: [
            CHHapticEventParameter(parameterID: .hapticIntensity, value: min(intensity, 1)),
            CHHapticEventParameter(parameterID: .hapticSharpness, value: min(sharpness, 1)),
        ], relativeTime: 0)
        guard let pattern = try? CHHapticPattern(events: [tap], parameters: []),
              let player = try? engine.makePlayer(with: pattern) else { return }
        if (try? player.start(atTime: CHHapticTimeImmediate)) == nil {
            try? engine.start()
            try? player.start(atTime: CHHapticTimeImmediate)
        }
    }
}

private extension UIColor {
    func mixed(with other: UIColor, by t: CGFloat) -> UIColor {
        var (r1, g1, b1, a1): (CGFloat, CGFloat, CGFloat, CGFloat) = (0, 0, 0, 0)
        var (r2, g2, b2, a2): (CGFloat, CGFloat, CGFloat, CGFloat) = (0, 0, 0, 0)
        getRed(&r1, green: &g1, blue: &b1, alpha: &a1)
        other.getRed(&r2, green: &g2, blue: &b2, alpha: &a2)
        let t = min(max(t, 0), 1)
        return UIColor(red: r1 + (r2 - r1) * t, green: g1 + (g2 - g1) * t, blue: b1 + (b2 - b1) * t, alpha: a1 + (a2 - a1) * t)
    }
}
