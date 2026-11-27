import CoreHaptics
import MatchbloxKit
import UIKit

/// The match head, drawn as its material: chlorate and ground glass in glue, so a fine, dry grain
/// with no gloss (DESIGN.md §2, the match). It sits in a shallow well with a soft shadow, and
/// sinks 1 pt under a press. The grain and its grit turn with the roll; the light stays fixed.
/// Cold at rest, ember, then amber as it heats, and alight when it strikes.
final class HeadLayer: CALayer {
    private struct Speck {
        let x, y, z: CGFloat
        let size: CGFloat
    }

    /// A noise height field, made once per size: medium grain, features about 1.7 pt across.
    private struct Grain {
        let side: Int
        let heights: [Float]

        init(side: Int, feature: CGFloat) {
            var generator = SystemRandomNumberGenerator()
            let cells = Int((CGFloat(side) / feature).rounded(.up)) + 2
            let grid = (0..<(cells * cells)).map { _ in Float.random(in: 0...1, using: &generator) }
            var heights = [Float](repeating: 0, count: side * side)
            let step = Float(feature)
            for y in 0..<side {
                let gy = Float(y) / step
                let iy = Int(gy)
                let sy = Self.smooth(gy - Float(iy))
                for x in 0..<side {
                    let gx = Float(x) / step
                    let ix = Int(gx)
                    let sx = Self.smooth(gx - Float(ix))
                    let top = grid[iy * cells + ix] + (grid[iy * cells + ix + 1] - grid[iy * cells + ix]) * sx
                    let low = grid[(iy + 1) * cells + ix] + (grid[(iy + 1) * cells + ix + 1] - grid[(iy + 1) * cells + ix]) * sx
                    heights[y * side + x] = top + (low - top) * sy
                }
            }
            self.side = side
            self.heights = heights
        }

        private static func smooth(_ t: Float) -> Float { t * t * (3 - 2 * t) }

        /// The grain turned by `turn` radians and lit from the top left, as grey to multiply over
        /// the colour: white where it is flat or faces the light, darker in its pits.
        func image(turn: CGFloat) -> CGImage? {
            let c = Float(side) / 2
            let (cs, sn) = (Float(cos(-turn)), Float(sin(-turn)))
            func height(_ x: Float, _ y: Float) -> Float {
                let (dx, dy) = (x - c, y - c)
                let rx = Int(c + dx * cs - dy * sn)
                let ry = Int(c + dx * sn + dy * cs)
                guard rx >= 0, ry >= 0, rx < side, ry < side else { return 0.5 }
                return heights[ry * side + rx]
            }
            var grey = [UInt8](repeating: 255, count: side * side)
            for y in 0..<side {
                for x in 0..<side {
                    let (fx, fy) = (Float(x), Float(y))
                    let here = height(fx, fy)
                    // Ground that rises toward the light shades this point.
                    let away = max(0, height(fx - 1, fy - 1) - here)
                    let pit = max(0, 0.45 - here)
                    let dark = min(1, away * 2.2 + pit * 0.9)
                    grey[y * side + x] = UInt8(255 * (1 - 0.42 * dark))
                }
            }
            guard let provider = CGDataProvider(data: Data(grey) as CFData) else { return nil }
            return CGImage(width: side, height: side, bitsPerComponent: 8, bitsPerPixel: 8, bytesPerRow: side,
                           space: CGColorSpaceCreateDeviceGray(), bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.none.rawValue),
                           provider: provider, decode: nil, shouldInterpolate: true, intent: .defaultIntent)
        }
    }

    private static let specks: [Speck] = {
        var generator = SystemRandomNumberGenerator()
        return (0..<70).map { _ in
            let u = CGFloat.random(in: 0..<(2 * .pi), using: &generator)
            let v = acos(CGFloat.random(in: 0...1, using: &generator))
            return Speck(x: sin(v) * cos(u), y: sin(v) * sin(u), z: cos(v), size: .random(in: 0.4...1.0, using: &generator))
        }
    }()

    /// Room round the head for the well and the shadow.
    static let margin: CGFloat = 8
    private static let feature: CGFloat = 1.7
    private static let well: CGFloat = 3
    private static let sink: CGFloat = 1

    /// The head's colour at rest, set on home; the heat colours are the same for every pigment.
    var pigment = HeadPigment.matchRed { didSet { if pigment != oldValue { setNeedsDisplay() } } }
    /// Radians the head turned round its centre.
    var turn: CGFloat = 0 { didSet { if turn != oldValue { setNeedsDisplay() } } }
    var heat: CGFloat = 0 { didSet { if heat != oldValue { setNeedsDisplay() } } }
    var struck = false { didSet { if struck != oldValue { setNeedsDisplay() } } }
    var pressed = false { didSet { if pressed != oldValue { setNeedsDisplay() } } }

    private var grain: Grain?
    private var turnedGrain: (turn: CGFloat, image: CGImage)?

    override init() {
        super.init()
        needsDisplayOnBoundsChange = true
    }

    override init(layer: Any) {
        super.init(layer: layer)
    }

    required init?(coder: NSCoder) { nil }

    private var radius: CGFloat { min(bounds.width, bounds.height) / 2 - Self.margin }

    override func draw(in ctx: CGContext) {
        let r = radius
        guard r > 0 else { return }
        let sink = pressed ? Self.sink : 0
        let c = CGPoint(x: bounds.midX, y: bounds.midY + sink)
        let head = CGRect(x: c.x - r, y: c.y - r, width: 2 * r, height: 2 * r)
        let color = UIColor(pigment.face(heat: Double(heat)))
        drawWell(in: ctx, around: CGPoint(x: bounds.midX, y: bounds.midY), radius: r + Self.well)
        if struck {
            let glow = [UIColor(Theme.flameCore).withAlphaComponent(0.9).cgColor, UIColor(Theme.flame).withAlphaComponent(0).cgColor]
            if let g = CGGradient(colorsSpace: nil, colors: glow as CFArray, locations: [0, 1]) {
                ctx.drawRadialGradient(g, startCenter: CGPoint(x: c.x, y: c.y - r * 0.3), startRadius: 0,
                                       endCenter: c, endRadius: r * 1.6, options: [])
            }
        }
        ctx.saveGState()
        ctx.setShadow(offset: CGSize(width: 0, height: 3 - 2 * sink), blur: pressed ? 3 : 6,
                      color: UIColor.black.withAlphaComponent(0.5).cgColor)
        ctx.setFillColor(color.cgColor)
        ctx.fillEllipse(in: head)
        ctx.restoreGState()

        ctx.saveGState()
        ctx.addEllipse(in: head)
        ctx.clip()
        // Matte: a soft fall-off to the rim and no highlight.
        let lit = color.mixed(with: .black, by: 0.04).cgColor
        let dark = UIColor(pigment.shade(heat: Double(heat))).cgColor
        if let g = CGGradient(colorsSpace: nil, colors: [lit, color.cgColor, dark] as CFArray, locations: [0, 0.55, 1]) {
            ctx.drawRadialGradient(g, startCenter: CGPoint(x: c.x - r * 0.3, y: c.y - r * 0.3), startRadius: 0,
                                   endCenter: c, endRadius: r, options: [.drawsAfterEndLocation])
        }
        if let image = grainImage() {
            ctx.saveGState()
            ctx.setBlendMode(.multiply)
            ctx.translateBy(x: head.minX, y: head.maxY)
            ctx.scaleBy(x: 1, y: -1)
            ctx.draw(image, in: CGRect(origin: .zero, size: head.size))
            ctx.restoreGState()
        }
        let (sn, cs) = (sin(turn), cos(turn))
        for s in Self.specks {
            let x = s.x * cs - s.y * sn
            let y = s.x * sn + s.y * cs
            let size = (s.size * s.z + 0.3) * r / 30
            ctx.setFillColor(UIColor.black.withAlphaComponent(0.3 * s.z).cgColor)
            ctx.fillEllipse(in: CGRect(x: c.x + x * r * 0.95 - size, y: c.y + y * r * 0.95 - size, width: 2 * size, height: 2 * size))
        }
        ctx.restoreGState()
        ctx.setStrokeColor(UIColor.black.withAlphaComponent(0.5).cgColor)
        ctx.setLineWidth(1)
        ctx.strokeEllipse(in: head)
    }

    /// The shallow well the head sits in: darker than the dock, its upper rim in shadow.
    private func drawWell(in ctx: CGContext, around c: CGPoint, radius r: CGFloat) {
        let well = CGRect(x: c.x - r, y: c.y - r, width: 2 * r, height: 2 * r)
        ctx.saveGState()
        ctx.addEllipse(in: well)
        ctx.clip()
        ctx.setFillColor(UIColor.black.withAlphaComponent(0.25).cgColor)
        ctx.fill(well)
        ctx.setShadow(offset: CGSize(width: 0, height: Self.well / 2), blur: Self.well,
                      color: UIColor.black.withAlphaComponent(0.6).cgColor)
        ctx.setStrokeColor(UIColor.black.cgColor)
        ctx.setLineWidth(Self.well * 2)
        ctx.strokeEllipse(in: well.insetBy(dx: -Self.well, dy: -Self.well))
        ctx.restoreGState()
    }

    private func grainImage() -> CGImage? {
        let side = Int((2 * radius * contentsScale).rounded())
        guard side > 0 else { return nil }
        if grain?.side != side {
            grain = Grain(side: side, feature: Self.feature * contentsScale)
            turnedGrain = nil
        }
        if let turnedGrain, turnedGrain.turn == turn { return turnedGrain.image }
        guard let image = grain?.image(turn: turn) else { return nil }
        turnedGrain = (turn, image)
        return image
    }

    /// Enter went: a `flame-core` ring at the rim for 140 ms (DESIGN.md §5), never the strike's
    /// glow, which means "rolled too fast".
    func flash() {
        let r = radius
        let ring = CAShapeLayer()
        ring.frame = bounds
        ring.path = UIBezierPath(ovalIn: CGRect(x: bounds.midX - r, y: bounds.midY - r, width: 2 * r, height: 2 * r)).cgPath
        ring.fillColor = nil
        ring.strokeColor = UIColor(Theme.flameCore).cgColor
        ring.lineWidth = 2
        ring.opacity = 0
        addSublayer(ring)
        let fade = CABasicAnimation(keyPath: "opacity")
        fade.fromValue = 1
        fade.toValue = 0
        fade.duration = 0.14
        CATransaction.begin()
        CATransaction.setCompletionBlock { ring.removeFromSuperlayer() }
        ring.add(fade, forKey: "flash")
        CATransaction.commit()
    }
}

/// What the thumb feels on the head: a short, sharp tap a notch, sharper as the head heats; one
/// hard tap when it strikes; one firm tap back at the bottom or as a hold begins; one heavy bump
/// at an end. A phone with no haptics plays nothing.
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

    /// The view came back to the bottom, or the hold began.
    func cross() {
        play(intensity: 0.8, sharpness: 0.4)
    }

    /// An end of the lines, or nothing to scroll.
    func bump() {
        play(intensity: 1, sharpness: 0.1)
    }

    /// Enter went: two firm taps 60 ms apart, unlike any notch or strike.
    func click() {
        play(intensity: 0.9, sharpness: 0.6, at: [0, 0.06])
    }

    /// A touch stopped the belt head's flick and sent nothing: one soft, dull tap.
    func stop() {
        play(intensity: 0.4, sharpness: 0.1)
    }

    /// A tap that cannot play is lost: the scroll it marks still happens.
    private func play(intensity: Float, sharpness: Float, at times: [TimeInterval] = [0]) {
        guard let engine else { return }
        let taps = times.map { time in
            CHHapticEvent(eventType: .hapticTransient, parameters: [
                CHHapticEventParameter(parameterID: .hapticIntensity, value: min(intensity, 1)),
                CHHapticEventParameter(parameterID: .hapticSharpness, value: min(sharpness, 1)),
            ], relativeTime: time)
        }
        guard let pattern = try? CHHapticPattern(events: taps, parameters: []),
              let player = try? engine.makePlayer(with: pattern) else { return }
        if (try? player.start(atTime: CHHapticTimeImmediate)) == nil {
            try? engine.start()
            try? player.start(atTime: CHHapticTimeImmediate)
        }
    }
}

extension UIColor {
    convenience init(_ colour: HeadPigment.Colour) {
        self.init(red: colour.red, green: colour.green, blue: colour.blue, alpha: 1)
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
