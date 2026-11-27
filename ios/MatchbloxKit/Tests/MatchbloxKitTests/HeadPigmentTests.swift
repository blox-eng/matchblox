import XCTest
@testable import MatchbloxKit

/// The head's colour at rest, set on home: five pigments, and only the cold head changes.
final class HeadPigmentTests: XCTestCase {
    private func defaults() -> UserDefaults {
        let name = "HeadPigmentTests.\(UUID().uuidString)"
        let d = UserDefaults(suiteName: name)!
        addTeardownBlock { d.removePersistentDomain(forName: name) }
        return d
    }

    func testUnknownIsMatchRed() {
        let store = defaults()
        XCTAssertEqual(HeadPigment.load(from: store), .matchRed, "nothing stored")
        store.set("pink", forKey: HeadPigment.storeKey)
        XCTAssertEqual(HeadPigment.load(from: store), .matchRed)
        store.set(7, forKey: HeadPigment.storeKey)
        XCTAssertEqual(HeadPigment.load(from: store), .matchRed, "a stored number")

        HeadPigment.blueTip.save(to: store)
        XCTAssertEqual(HeadPigment.load(from: store), .blueTip)
    }

    func testFiveColours() {
        XCTAssertEqual(HeadPigment.allCases, [.matchRed, .strikeAnywhereBrown, .blueTip, .greenTip, .charcoal])
        XCTAssertEqual(HeadPigment.allCases.map(\.name),
                       ["match red", "strike-anywhere brown", "blue tip", "green tip", "charcoal"])
        XCTAssertEqual(HeadPigment.allCases.map(\.cold),
                       [0x9E2319, 0x5E3220, 0x2B467F, 0x2E5E37, 0x34302C].map(HeadPigment.Colour.init(hex:)))
        XCTAssertEqual(HeadPigment.allCases.map(\.shade),
                       [0x5A120C, 0x2E170E, 0x141F3D, 0x14301B, 0x151311].map(HeadPigment.Colour.init(hex:)))
    }

    func testColdOnly() {
        for pigment in HeadPigment.allCases {
            XCTAssertEqual(pigment.face(heat: 0), pigment.cold, "\(pigment.name) at rest")
            XCTAssertEqual(pigment.shade(heat: 0), pigment.shade, "\(pigment.name)'s rim at rest")
            for heat in [0.33, 0.5, 0.75, 1.0] {
                XCTAssertEqual(pigment.face(heat: heat), HeadPigment.matchRed.face(heat: heat),
                               "\(pigment.name) at heat \(heat)")
                XCTAssertEqual(pigment.shade(heat: heat), HeadPigment.matchRed.shade(heat: heat),
                               "\(pigment.name)'s rim at heat \(heat)")
            }
        }
        XCTAssertEqual(HeadPigment.charcoal.face(heat: 0.33), HeadPigment.ember)
        XCTAssertEqual(HeadPigment.blueTip.face(heat: 0.75), HeadPigment.amber)
        XCTAssertEqual(HeadPigment.greenTip.face(heat: 1), HeadPigment.core)
    }
}
