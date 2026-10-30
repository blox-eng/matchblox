import Foundation

/// The matchbox tray: the four layouts lie in it as matches. The match of the layout in use
/// burns; picking another strikes it, and the one it leaves goes out. A closed box comes back
/// fresh: every match is ready again but the one in use.
public struct MatchTray: Equatable, Sendable {
    public enum Match: Equatable, Sendable {
        /// Not struck: the full stick and the red head.
        case ready
        case burns
        /// Struck and left: burnt wood, a cold head, and smoke as it goes out.
        case out

        /// The island's match to draw, or nil for a ready match, which the island never draws.
        public var look: IslandLook.Match? {
            switch self {
            case .ready: nil
            case .burns: IslandLook.Match(state: .burns, front: MatchTray.burningFront)
            case .out: IslandLook.Match(state: .out, front: MatchTray.outFront)
            }
        }
    }

    /// The tray rises this far out of the sleeve as it opens.
    public static let slide: CGFloat = 30
    /// The strike: the flame flares this long, then burns steady.
    public static let flare: TimeInterval = 0.26
    /// The smoke of the match left rises and fades over this long.
    public static let smokeFade: TimeInterval = 1.4
    /// The tray closes this long after a pick, so the strike shows.
    public static let closeDelay: TimeInterval = 0.9
    /// A short burnt length: the layout is in use, not used up.
    public static let burningFront = 0.12
    /// A match left burns a little further before it goes out.
    public static let outFront = 0.34

    public private(set) var inUse: BeltLayout
    private var left: Set<BeltLayout> = []

    public init(inUse: BeltLayout) {
        self.inUse = inUse
    }

    public func match(_ layout: BeltLayout) -> Match {
        if layout == inUse { return .burns }
        return left.contains(layout) ? .out : .ready
    }

    /// Strikes `layout`, and the match in use goes out. Returns the match that went out, or
    /// nil when `layout` is already in use.
    @discardableResult
    public mutating func pick(_ layout: BeltLayout) -> BeltLayout? {
        guard layout != inUse else { return nil }
        let was = inUse
        left.insert(was)
        left.remove(layout)
        inUse = layout
        return was
    }

    /// A fresh box for the next open.
    public mutating func close() {
        left = []
    }
}

/// The editor's strict grid of equal cells. A two-cell key spans two columns. A key that does
/// not fit in what is left of a row starts the next row, and the cells it skips stay empty.
public enum CellGrid {
    public static let columns = 6

    public struct Spot: Equatable, Sendable {
        public let row: Int
        public let column: Int
        public let span: Int

        public init(row: Int, column: Int, span: Int) {
            self.row = row
            self.column = column
            self.span = span
        }
    }

    /// The spot of each key, in order, from the cells each key is wide.
    public static func place(_ cells: [Int], columns: Int = columns) -> [Spot] {
        let columns = max(1, columns)
        var row = 0, column = 0
        return cells.map { wide in
            let span = min(max(1, wide), columns)
            if column + span > columns {
                row += 1
                column = 0
            }
            let spot = Spot(row: row, column: column, span: span)
            column += span
            if column == columns {
                row += 1
                column = 0
            }
            return spot
        }
    }

    public static func rows(_ spots: [Spot]) -> Int {
        spots.map(\.row).max().map { $0 + 1 } ?? 0
    }
}

/// The terminal's size while a matchbox tray is open. The tray makes the screen shorter, but
/// the terminal keeps the size it had: its lines slide up and the top ones go under the edge,
/// as in the mockup, so the host gets no resize. When the trays close, the terminal follows
/// the screen again once the screen is back to the held height, or once its width changes.
public struct ScreenHold: Sendable {
    private var trays = 0
    public private(set) var held: CGSize?

    public init() {}

    /// A tray starts to open over a screen of this size.
    public mutating func open(screen: CGSize) {
        if trays == 0, held == nil { held = screen }
        trays += 1
    }

    /// A tray has closed, its animation done.
    public mutating func close() {
        trays = max(0, trays - 1)
    }

    /// The terminal's height on a screen of this size. It is never less than the screen's.
    public mutating func height(screen: CGSize) -> CGFloat {
        if let held, trays == 0, screen.height >= held.height || screen.width != held.width {
            self.held = nil
        }
        return max(screen.height, held?.height ?? 0)
    }
}
