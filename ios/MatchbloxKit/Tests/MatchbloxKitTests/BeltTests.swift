import XCTest
@testable import MatchbloxKit

final class BeltTests: XCTestCase {
    private let screen: CGFloat = 370
    private var geometry: BeltGeometry { BeltGeometry(width: screen) }
    private var homeRow: [BeltKey] {
        [BeltKey(id: "◂", cells: 1), BeltKey(id: "↑", cells: 2),
         BeltKey(id: "↓", cells: 2), BeltKey(id: "⏎", cells: 2)]
    }
    /// Ten one-cell keys: every detent one cell and one gap apart, and one copy fills the ring.
    private var tenKeys: [BeltKey] { (0..<10).map { BeltKey(id: "k\($0)", cells: 1) } }
    private var detent: CGFloat { geometry.cell + BeltGeometry.gap }

    // MARK: Cells

    func testCellFillsTheHomeRow() {
        let g = geometry
        XCTAssertEqual(g.cell, 48.2857, accuracy: 0.0001)
        let keys = homeRow.reduce(0) { $0 + g.width(of: $1) }
        XCTAssertEqual(keys + 5 * BeltGeometry.gap, screen, accuracy: 1e-9)
        XCTAssertEqual(g.width(of: BeltKey(id: "↑", cells: 2)), 2 * g.cell + BeltGeometry.gap, accuracy: 1e-9)
    }

    func testCellNeverUnder44() {
        XCTAssertEqual(BeltGeometry(width: 300).cell, 44)
    }

    // MARK: Fold

    func testFoldFlatInside() {
        let f = BeltFold.fold(x0: 100, width: 48, screen: screen)
        XCTAssertEqual(f.x, 100)
        XCTAssertEqual(f.angle, 0)
        XCTAssertEqual(f.shade, 0)
        XCTAssertTrue(f.visible)
    }

    func testFoldRightEdgeHalfway() {
        let f = BeltFold.fold(x0: screen - 24, width: 48, screen: screen)
        XCTAssertEqual(f.x, screen - 24)
        XCTAssertEqual(f.angle, -45, accuracy: 1e-9)
        XCTAssertEqual(f.shade, 0.375, accuracy: 1e-9)
        XCTAssertTrue(f.anchorLeft)
        XCTAssertTrue(f.visible)
    }

    func testFoldLeftEdge() {
        let f = BeltFold.fold(x0: -12, width: 48, screen: screen)
        XCTAssertEqual(f.x, -12)
        XCTAssertEqual(f.angle, 22.5, accuracy: 1e-9)
        XCTAssertEqual(f.shade, 0.1875, accuracy: 1e-9)
        XCTAssertFalse(f.anchorLeft)
        XCTAssertTrue(f.visible)
    }

    func testFoldGoneBehind() {
        let right = BeltFold.fold(x0: screen, width: 48, screen: screen)
        XCTAssertEqual(right.angle, -90)
        XCTAssertEqual(right.shade, 0.75)
        XCTAssertFalse(right.visible)
        let left = BeltFold.fold(x0: -60, width: 48, screen: screen)
        XCTAssertEqual(left.x, -48)
        XCTAssertEqual(left.angle, 90)
        XCTAssertEqual(left.shade, 0.75)
        XCTAssertFalse(left.visible)
    }

    // MARK: Ring and detents

    func testOpensAtHome() {
        let m = BeltMotion(keys: homeRow, geometry: geometry)
        XCTAssertTrue(m.atRest)
        XCTAssertEqual(m.offset, m.homeOffset)
        XCTAssertEqual(m.x(ofKeyAt: 0), BeltGeometry.gap, accuracy: 1e-9)
        XCTAssertEqual(m.x(ofKeyAt: 3) + geometry.width(of: homeRow[3]), screen - BeltGeometry.gap, accuracy: 1e-9)
    }

    func testRingNeverEnds() {
        var m = BeltMotion(keys: homeRow, geometry: geometry)
        XCTAssertGreaterThanOrEqual(m.ring, screen + geometry.width(of: homeRow[1]))
        let start = m.keys.indices.map { m.x(ofKeyAt: $0) }
        for _ in 0..<100 { _ = m.drag(by: -m.ring / 10) }
        for i in m.keys.indices {
            XCTAssertEqual(m.x(ofKeyAt: i), start[i], accuracy: 1e-6)
        }
        _ = m.drag(by: -0.37 * m.ring)
        let xs = m.keys.indices.map { m.x(ofKeyAt: $0) }
        XCTAssertTrue(xs.allSatisfy(\.isFinite))
        let order = m.keys.indices.sorted { xs[$0] < xs[$1] }
        for (a, b) in zip(order, order.dropFirst()) {
            XCTAssertEqual(b, (a + 1) % m.keys.count, "keys keep their order round the ring")
        }
    }

    func testDragTicksOncePerDetent() {
        var m = BeltMotion(keys: tenKeys, geometry: geometry)
        var ticks: [BeltTick] = []
        for _ in 0..<30 { ticks += m.drag(by: -detent / 10) }
        XCTAssertEqual(ticks, [.detent, .detent, .detent])
    }

    func testDragThatComesBackDoesNotTick() {
        var m = BeltMotion(keys: tenKeys, geometry: geometry)
        XCTAssertEqual(m.drag(by: 0.4 * detent), [])
        XCTAssertEqual(m.drag(by: -0.8 * detent), [])
        XCTAssertEqual(m.drag(by: 0.4 * detent), [])
    }

    func testFlickTicksOncePerDetent() {
        var m = BeltMotion(keys: tenKeys, geometry: geometry)
        _ = m.drag(by: 0)
        m.release(velocity: -3380)
        let before = m.offset
        let ticks = m.step(dt: 0.1)
        XCTAssertEqual((before - m.offset) / detent, 5.2, accuracy: 0.1)
        XCTAssertEqual(ticks, Array(repeating: .detent, count: 5))
    }

    func testHomeTick() {
        var m = BeltMotion(keys: tenKeys, geometry: geometry)
        XCTAssertEqual(m.drag(by: detent), [.detent])
        XCTAssertEqual(m.drag(by: -detent), [.home])
        XCTAssertEqual(m.drag(by: -detent * 10), Array(repeating: .detent, count: 9) + [.home])
    }

    func testSettlesOnADetent() {
        var m = BeltMotion(keys: homeRow, geometry: geometry)
        _ = m.drag(by: -70)
        m.release(velocity: -500)
        XCTAssertFalse(m.atRest)
        var frames = 0
        while !m.atRest && frames < 2000 {
            _ = m.step(dt: 1.0 / 60)
            frames += 1
        }
        XCTAssertTrue(m.atRest)
        XCTAssertEqual(m.offset, m.settleTarget(), accuracy: 1e-9)
        let onEdge = m.keys.indices.contains { abs(m.x(ofKeyAt: $0) - BeltGeometry.gap) < 1e-6 }
        XCTAssertTrue(onEdge, "a key's left side rests one gap from the left edge")
    }

    // MARK: Tap versus drag

    func testDragThatReturnsIsNotATap() {
        var t = TapOrDrag()
        t.moved(by: 9)
        t.moved(by: -9)
        XCTAssertTrue(t.isDrag)
    }

    func testSmallMoveIsATap() {
        var t = TapOrDrag()
        t.moved(by: 7)
        XCTAssertFalse(t.isDrag)
        t.moved(by: 1)
        XCTAssertTrue(t.isDrag)
    }
}
