import MatchbloxKit
import SwiftUI

/// One horizontal match, after the 0006 mockup: the burnt head on the left, the charred
/// wood up to the front, the ember there, and the unburnt wood after it. A burning match
/// has its flame at the front; a waiting one smokes over a dim ember; done smokes a little
/// and is cold; out is cold. A Live Activity runs no timers, so this is the still frame of
/// each state; the system cross-fades between two content states.
struct MatchView: View {
    let match: IslandLook.Match
    let thickness: CGFloat
    var minimal = false
    /// The strike: the flame 1.6x its size in a brighter glow. The island never sets it.
    var flare = false
    /// The island clips its match to its frame; the tray lets the flame rise over its floor.
    var clips = true

    var body: some View {
        if clips {
            drawing.clipped()
        } else {
            drawing
        }
    }

    private var drawing: some View {
        GeometryReader { geo in
            let size = geo.size
            if match.state == .quiet {
                ClosedBox(size: size)
            } else {
                let g = MatchGeometry(size: size, th: thickness, front: match.front, minimal: minimal)
                ZStack(alignment: .topLeading) {
                    wood(g)
                    if match.state == .burns || match.state == .waits { ember(g, glow: match.state == .burns ? 1 : 0.6) }
                    if match.state == .burns { FlameView(x: g.front, y: g.y, size: g.th * 3.1 * (flare ? 1.6 : 1), glow: flare ? 0.5 : 0.28) }
                    if match.state == .waits { SmokeView(x: g.front, y: g.y - g.th * 0.4, th: g.th, puffs: 4) }
                    if match.state == .done { SmokeView(x: g.front, y: g.y - g.th * 0.4, th: g.th, puffs: 2) }
                }
            }
        }
    }

    @ViewBuilder
    private func wood(_ g: MatchGeometry) -> some View {
        let th = g.th, bt = th * 0.78
        // The unburnt wood.
        if g.front < g.end {
            Path(roundedRect: CGRect(x: g.front, y: g.y - th / 2, width: g.end - g.front, height: th), cornerRadius: th * 0.25)
                .fill(Tokens.markStick)
        }
        // The burnt wood, thinner, with a faint top edge.
        Path(roundedRect: CGRect(x: g.wood0, y: g.y - bt / 2, width: max(0, g.front - g.wood0), height: bt), cornerRadius: bt * 0.3)
            .fill(Tokens.markLeft)
        Path(CGRect(x: g.wood0, y: g.y - bt / 2, width: max(0, g.front - g.wood0), height: max(1, bt * 0.18)))
            .fill(Tokens.markTop)
        // The burnt head.
        Path(ellipseIn: CGRect(x: g.x, y: g.y - th * 0.78, width: g.headW, height: th * 1.56))
            .fill(Tokens.markRight)
        Path(ellipseIn: CGRect(x: g.x + g.headW * 0.15, y: g.y - th * 0.52, width: g.headW * 0.6, height: th * 0.44))
            .fill(Tokens.markTop.opacity(0.6))
    }

    private func ember(_ g: MatchGeometry, glow: Double) -> some View {
        let bt = g.th * 0.78
        let start = max(g.wood0, g.front - g.th * 3)
        return Path(roundedRect: CGRect(x: start, y: g.y - bt / 2, width: g.front - start, height: bt), cornerRadius: bt * 0.3)
            .fill(LinearGradient(
                stops: [
                    .init(color: Tokens.matchEmber.opacity(0), location: 0),
                    .init(color: Tokens.matchEmber.opacity(0.9 * glow), location: 0.6),
                    .init(color: Tokens.flameAmber.opacity(glow), location: 1),
                ],
                startPoint: UnitPoint(x: start / max(1, g.size.width), y: 0.5),
                endPoint: UnitPoint(x: g.front / max(1, g.size.width), y: 0.5)))
    }
}

/// A match not struck yet, as the closed box holds it: the full stick and the red head. The
/// tray draws a layout that is not in use so; the geometry is the island's match.
struct ReadyMatchView: View {
    let thickness: CGFloat

    var body: some View {
        GeometryReader { geo in
            let g = MatchGeometry(size: geo.size, th: thickness, front: 0, minimal: false)
            let th = g.th
            ZStack(alignment: .topLeading) {
                Path(roundedRect: CGRect(x: g.wood0, y: g.y - th / 2, width: g.end - g.wood0, height: th), cornerRadius: th * 0.25)
                    .fill(Tokens.markStick)
                Path(ellipseIn: CGRect(x: g.x, y: g.y - th * 0.78, width: g.headW, height: th * 1.56))
                    .fill(Tokens.matchHead)
            }
        }
    }
}

struct MatchGeometry {
    let size: CGSize
    let th: CGFloat
    let x: CGFloat, y: CGFloat, len: CGFloat
    let headW: CGFloat, wood0: CGFloat, front: CGFloat

    init(size: CGSize, th: CGFloat, front p: Double, minimal: Bool) {
        self.size = size
        self.th = th
        if minimal {
            x = 3; y = size.height * 0.74; len = size.width - 6
        } else {
            x = th * 0.6; y = size.height * 0.7; len = size.width - th * 1.2
        }
        headW = th * 2.1
        wood0 = x + headW * 0.6
        front = wood0 + CGFloat(max(0.03, p)) * (len - headW * 0.6)
    }

    var end: CGFloat { x + len }
}

/// The still flame at the front: a soft glow and three tongues, flame, amber and core.
struct FlameView: View {
    let x: CGFloat, y: CGFloat, size: CGFloat
    var glow = 0.28

    var body: some View {
        let glowR = size * 1.5
        ZStack(alignment: .topLeading) {
            Circle()
                .fill(RadialGradient(colors: [Tokens.flame.opacity(glow), Tokens.flame.opacity(0)], center: .center, startRadius: 0, endRadius: glowR))
                .frame(width: glowR * 2, height: glowR * 2)
                .offset(x: x - glowR, y: y - size * 0.4 - glowR)
            tongue(1, Tokens.flame, 0.92)
            tongue(0.7, Tokens.flameAmber, 0.95)
            tongue(0.4, Tokens.flameCore, 1)
        }
    }

    private func tongue(_ k: CGFloat, _ color: Color, _ alpha: Double) -> some View {
        let h = size * k, w = size * 0.34 * k
        return Path { p in
            p.move(to: CGPoint(x: x - w, y: y))
            p.addCurve(to: CGPoint(x: x, y: y - h), control1: CGPoint(x: x - w, y: y - h * 0.5), control2: CGPoint(x: x - w * 0.3, y: y - h * 0.72))
            p.addCurve(to: CGPoint(x: x + w, y: y), control1: CGPoint(x: x + w * 0.3, y: y - h * 0.72), control2: CGPoint(x: x + w, y: y - h * 0.5))
            p.addQuadCurve(to: CGPoint(x: x - w, y: y), control: CGPoint(x: x, y: y + w * 0.95))
            p.closeSubpath()
        }
        .fill(color.opacity(alpha))
    }
}

/// Still puffs of smoke rising from the front and drifting right, fading as they grow.
struct SmokeView: View {
    let x: CGFloat, y: CGFloat, th: CGFloat, puffs: Int
    /// The tray's dark floor shows the island's puffs this much stronger.
    var gain = 1.0

    private static let steps: [(dx: CGFloat, dy: CGFloat, r: CGFloat, a: Double)] = [
        (0.3, 1.0, 0.9, 0.30), (0.9, 2.4, 1.5, 0.22), (0.5, 4.0, 2.1, 0.15), (1.5, 5.6, 2.7, 0.09),
    ]

    var body: some View {
        ZStack(alignment: .topLeading) {
            ForEach(0..<min(puffs, Self.steps.count), id: \.self) { i in
                let s = Self.steps[i]
                let r = s.r * th
                let a = (puffs < Self.steps.count ? s.a * 0.6 : s.a) * gain
                Circle()
                    .fill(RadialGradient(colors: [Tokens.muted.opacity(a), Tokens.muted.opacity(0)], center: .center, startRadius: 0, endRadius: r))
                    .frame(width: r * 2, height: r * 2)
                    .offset(x: x + s.dx * th - r, y: y - s.dy * th - r)
            }
        }
    }
}

/// The closed box with the heads ready: nothing burns, nothing waits.
struct ClosedBox: View {
    let size: CGSize

    var body: some View {
        let bw = min(150, size.width * 0.5), bh = size.height * 0.46
        let x = (size.width - bw) / 2, y = size.height * 0.38, d = bh * 0.35
        ZStack(alignment: .topLeading) {
            Path { p in
                p.move(to: CGPoint(x: x, y: y)); p.addLine(to: CGPoint(x: x + d, y: y - d * 0.6))
                p.addLine(to: CGPoint(x: x + bw + d, y: y - d * 0.6)); p.addLine(to: CGPoint(x: x + bw, y: y)); p.closeSubpath()
            }.fill(Tokens.markTop)
            Path { p in
                p.move(to: CGPoint(x: x + bw, y: y)); p.addLine(to: CGPoint(x: x + bw + d, y: y - d * 0.6))
                p.addLine(to: CGPoint(x: x + bw + d, y: y + bh - d * 0.6)); p.addLine(to: CGPoint(x: x + bw, y: y + bh)); p.closeSubpath()
            }.fill(Tokens.markRight)
            Path(CGRect(x: x, y: y, width: bw, height: bh)).fill(Tokens.markLeft)
            Path(CGRect(x: x, y: y + bh * 0.62, width: bw, height: bh * 0.22)).fill(Tokens.markStriker)
            ForEach(0..<5, id: \.self) { i in
                let rx = bh * 0.2, ry = bh * 0.14
                let cx = x + d * 0.5 + bw * 0.1 + CGFloat(i) * bw * 0.84 / 5
                Path(ellipseIn: CGRect(x: cx - rx, y: y - d * 0.3 - ry, width: rx * 2, height: ry * 2)).fill(Tokens.matchHead)
            }
        }
    }
}
