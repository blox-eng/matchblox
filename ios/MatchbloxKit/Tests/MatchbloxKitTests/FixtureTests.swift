import XCTest

final class FixtureTests: XCTestCase {
    func testFixtureLoads() throws {
        let fixture = try Fixture.load()
        XCTAssertEqual(fixture.payload.name, "ws-1")
    }
}
