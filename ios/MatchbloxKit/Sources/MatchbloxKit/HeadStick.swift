import Foundation

/// The middle of the head as a thumbstick: a push toward a mark sends its key once, a held push
/// repeats, a still tap is Enter and a still hold opens the sessions. Points are the finger's
/// travel from where it landed, in UIKit points (y grows down); times are touch timestamps.
public struct HeadStick: Sendable {
    public enum Send: Equatable, Sendable {
        /// The key of the mark at that place. `word` is set from the tenth repeat on, where a mark
        /// with a word move (`←` `→`, `HeadMarks.wordBytes`) moves a word.
        case key(HeadMarks.Place, word: Bool)
    }

    public enum Lift: Equatable, Sendable {
        case enter
        /// The ring filled: the hold's mark.
        case hold
        case nothing
    }

    static let push: CGFloat = 16
    static let rearm: CGFloat = 10
    static let reach: CGFloat = 12
    static let tapUnder: TimeInterval = 0.35
    static let repeatAfter: TimeInterval = 0.42
    static let repeatEvery: TimeInterval = 0.11
    static let holdAfter: TimeInterval = 0.6
    static let wordAfter = 9

    /// Where the head sits, the thumb's travel clamped to 12 pt along it.
    public private(set) var offset = CGPoint(x: 0, y: 0)

    /// The thumb has not moved 10 pt from where it landed.
    public private(set) var still = true
    /// The thumb stayed still for 0.6 s: the ring filled, and the lift is the hold's.
    public private(set) var held = false
    private var start: TimeInterval?
    private var pushed: HeadMarks.Place?
    private var nextRepeat: TimeInterval = 0
    private var repeats = 0

    public init() {}

    public mutating func began(at time: TimeInterval) {
        self = HeadStick()
        start = time
    }

    public mutating func moved(to travel: CGPoint, at time: TimeInterval) -> Send? {
        guard start != nil else { return nil }
        settle(at: time)
        let distance = hypot(travel.x, travel.y)
        offset = distance > Self.reach
            ? CGPoint(x: travel.x * Self.reach / distance, y: travel.y * Self.reach / distance)
            : travel
        if distance >= Self.rearm { still = false }
        if pushed != nil {
            if distance < Self.rearm { pushed = nil }
            return nil
        }
        guard distance >= Self.push else { return nil }
        let place: HeadMarks.Place = abs(travel.x) > abs(travel.y)
            ? (travel.x < 0 ? .left : .right)
            : (travel.y < 0 ? .top : .bottom)
        pushed = place
        nextRepeat = time + Self.repeatAfter
        repeats = 0
        return .key(place, word: false)
    }

    /// The held push's repeat, once its time has come.
    public mutating func tick(at time: TimeInterval) -> Send? {
        guard start != nil else { return nil }
        settle(at: time)
        guard let place = pushed, time >= nextRepeat else { return nil }
        nextRepeat += Self.repeatEvery
        let word = repeats >= Self.wordAfter
        repeats += 1
        return .key(place, word: word)
    }

    public mutating func lifted(at time: TimeInterval) -> Lift {
        guard let start else { return .nothing }
        settle(at: time)
        defer { self = HeadStick() }
        if held { return .hold }
        return still && time - start < Self.tapUnder ? .enter : .nothing
    }

    /// A thumb still for 0.6 s has filled the ring, before whatever it does next.
    private mutating func settle(at time: TimeInterval) {
        guard let start, still, !held, time - start >= Self.holdAfter else { return }
        held = true
    }
}
