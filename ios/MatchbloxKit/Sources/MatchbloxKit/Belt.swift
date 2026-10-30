import Foundation

public struct BeltKey: Sendable, Equatable {
    public let id: String
    public let cells: Int

    public init(id: String, cells: Int) {
        precondition(cells == 1 || cells == 2, "a belt key is one or two cells wide")
        self.id = id
        self.cells = cells
    }
}

/// The home row `◂ ↑ ↓ ⏎` (1 + 2 + 2 + 2 cells, 8 gaps) fills the screen width.
public struct BeltGeometry: Sendable {
    public static let gap: CGFloat = 4
    public static let height: CGFloat = 60
    public static let keyHeight: CGFloat = 44
    public static let minCell: CGFloat = 44

    public let screen: CGFloat
    public let cell: CGFloat

    public init(width: CGFloat) {
        screen = width
        cell = max(Self.minCell, (width - 8 * Self.gap) / 7)
    }

    public func width(of key: BeltKey) -> CGFloat {
        CGFloat(key.cells) * cell + CGFloat(key.cells - 1) * Self.gap
    }
}

/// How a key sits at `x0`: flat between the screen's edges, folding round an edge past it.
public struct BeltFold: Sendable, Equatable {
    public static let maxShade: CGFloat = 0.75

    public let x: CGFloat
    /// Degrees about the vertical axis: + at the left edge, − at the right.
    public let angle: CGFloat
    /// The key turns on its left side (right edge) or its right side (left edge).
    public let anchorLeft: Bool
    public let shade: CGFloat
    public let visible: Bool

    public static func fold(x0: CGFloat, width: CGFloat, screen: CGFloat) -> BeltFold {
        let x1 = x0 + width
        if x1 > screen {
            let past = min(1, (x1 - screen) / width)
            return BeltFold(x: min(x0, screen), angle: -90 * past, anchorLeft: true,
                            shade: maxShade * past, visible: x0 < screen)
        }
        if x0 < 0 {
            let past = min(1, -x0 / width)
            return BeltFold(x: max(x0, -width), angle: 90 * past, anchorLeft: false,
                            shade: maxShade * past, visible: x1 > 0)
        }
        return BeltFold(x: x0, angle: 0, anchorLeft: false, shade: 0, visible: true)
    }
}

public enum BeltTick: Sendable, Equatable { case detent, home }

/// The belt's ring of keys and how it moves. `offset` is unbounded; key positions wrap.
/// A detent rests a key's left side one gap from the screen's left edge. A tick comes
/// each time the nearest detent changes, as a picker wheel ticks: so a drag that comes
/// back to where it started makes no tick.
public struct BeltMotion: Sendable {
    public static let decayPerFrame: CGFloat = 0.93
    public static let frame: TimeInterval = 0.016
    /// Below this speed (pt/s) the coast ends and the belt settles.
    public static let stopSpeed: CGFloat = 80
    /// The settle closes this part of the gap each frame, and snaps at `snap` pt.
    public static let settleEase: CGFloat = 0.35
    public static let snap: CGFloat = 0.4

    private enum Phase: Sendable { case rest, held, coasting, settling }

    /// The layout's keys, repeated so the ring is wider than the screen and one more key:
    /// no key is ever on the screen twice, and none pops in.
    public let keys: [BeltKey]
    public let geometry: BeltGeometry
    /// One turn of the ring, in points.
    public let ring: CGFloat
    public var offset: CGFloat
    public var homeOffset: CGFloat { BeltGeometry.gap }
    public var atRest: Bool { phase == .rest }

    private let layoutCount: Int
    private let starts: [CGFloat]
    private let widths: [CGFloat]
    private var velocity: CGFloat = 0
    private var phase = Phase.rest

    public init(keys layout: [BeltKey], geometry: BeltGeometry) {
        precondition(!layout.isEmpty, "a belt has at least one key")
        let gap = BeltGeometry.gap
        let layoutWidths = layout.map(geometry.width(of:))
        let turn = layoutWidths.reduce(0) { $0 + $1 + gap }
        let copies = max(1, Int(((geometry.screen + layoutWidths.max()!) / turn).rounded(.up)))
        keys = Array(repeating: layout, count: copies).flatMap { $0 }
        widths = Array(repeating: layoutWidths, count: copies).flatMap { $0 }
        var x: CGFloat = 0
        var starts: [CGFloat] = []
        for w in widths {
            starts.append(x)
            x += w + gap
        }
        self.starts = starts
        self.geometry = geometry
        ring = x
        layoutCount = layout.count
        offset = BeltGeometry.gap
    }

    /// Key `i`'s left side on the screen, in [−width, ring − width).
    public func x(ofKeyAt i: Int) -> CGFloat {
        wrap(starts[i] + offset + widths[i]) - widths[i]
    }

    public func fold(ofKeyAt i: Int) -> BeltFold {
        BeltFold.fold(x0: x(ofKeyAt: i), width: widths[i], screen: geometry.screen)
    }

    public func settleTarget() -> CGFloat {
        BeltGeometry.gap - detentPosition(nearestDetent(to: offset))
    }

    public mutating func drag(by dx: CGFloat) -> [BeltTick] {
        phase = .held
        velocity = 0
        return move(by: dx)
    }

    /// `velocity` is in pt/s, as a pan gesture gives it.
    public mutating func release(velocity: CGFloat) {
        self.velocity = velocity
        phase = .coasting
    }

    public mutating func step(dt: TimeInterval) -> [BeltTick] {
        let frames = CGFloat(dt / Self.frame)
        switch phase {
        case .rest, .held:
            return []
        case .coasting:
            // The per-frame decay, integrated so any frame rate coasts the same way.
            let decay = pow(Self.decayPerFrame, frames)
            let distance = velocity * CGFloat(Self.frame) * (1 - decay) / -log(Self.decayPerFrame)
            velocity *= decay
            if abs(velocity) < Self.stopSpeed { phase = .settling }
            return move(by: distance)
        case .settling:
            let target = settleTarget()
            let remaining = (target - offset) * pow(1 - Self.settleEase, frames)
            if abs(remaining) <= Self.snap {
                phase = .rest
                return move(by: target - offset)
            }
            return move(by: target - offset - remaining)
        }
    }

    private mutating func move(by dx: CGFloat) -> [BeltTick] {
        let from = nearestDetent(to: offset)
        offset += dx
        let to = nearestDetent(to: offset)
        guard from != to else { return [] }
        let dir = to > from ? 1 : -1
        return stride(from: from + dir, through: to, by: dir).map { d -> BeltTick in
            d.modulo(layoutCount) == 0 ? .home : .detent
        }
    }

    /// Detents counted along the unwrapped belt: detent `d` rests key `d mod keys.count`
    /// at the left edge, `d div keys.count` turns on from home.
    private func nearestDetent(to offset: CGFloat) -> Int {
        let along = BeltGeometry.gap - offset
        let turns = (along / ring).rounded(.down)
        let r = along - turns * ring
        var best = keys.count
        var bestDistance = abs(ring - r)
        for (i, start) in starts.enumerated() where abs(start - r) < bestDistance {
            best = i
            bestDistance = abs(start - r)
        }
        return Int(turns) * keys.count + best
    }

    private func detentPosition(_ d: Int) -> CGFloat {
        let n = keys.count
        let turns = (d - d.modulo(n)) / n
        return CGFloat(turns) * ring + starts[d.modulo(n)]
    }

    private func wrap(_ v: CGFloat) -> CGFloat {
        let r = v.truncatingRemainder(dividingBy: ring)
        return r < 0 ? r + ring : r
    }
}

/// A touch is a tap until it has moved 8 pt from where it went down; then it is a drag for good.
public struct TapOrDrag: Sendable {
    public static let threshold: CGFloat = 8

    public private(set) var isDrag = false
    private var travel: CGFloat = 0

    public init() {}

    public mutating func moved(by dx: CGFloat) {
        travel += dx
        if abs(travel) >= Self.threshold { isDrag = true }
    }
}

private extension Int {
    func modulo(_ n: Int) -> Int {
        let r = self % n
        return r < 0 ? r + n : r
    }
}
