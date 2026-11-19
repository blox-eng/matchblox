/// How far the wheel scrolled a tmux pane up from its live bottom, in wheel steps. A step up
/// puts the pane in copy mode (`copy-mode -e`), and the same steps down bring it back to the
/// bottom, where tmux leaves copy mode. A step down at the bottom does nothing, so the count
/// never goes under zero. A typed key ends the count: the person acts on the pane again.
public struct ScrollBack: Equatable, Sendable {
    public private(set) var steps = 0

    public init() {}

    /// The pane shows older lines: the jump to the bottom shows.
    public var isUp: Bool { steps > 0 }

    public mutating func wheeled(_ moved: [WheelScroll.Step]) {
        for step in moved {
            steps = step == .up ? steps + 1 : max(steps - 1, 0)
        }
    }

    public mutating func typed() {
        steps = 0
    }

    /// The steps that bring the pane back to its live bottom. The count is then zero.
    public mutating func jump() -> [WheelScroll.Step] {
        defer { steps = 0 }
        return Array(repeating: .down, count: steps)
    }
}
