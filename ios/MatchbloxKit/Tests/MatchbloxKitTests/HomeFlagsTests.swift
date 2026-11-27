import XCTest
@testable import MatchbloxKit

final class HomeFlagsTests: XCTestCase {
    private var suite: String!
    private var defaults: UserDefaults!

    override func setUp() {
        suite = "HomeFlagsTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
        suite = nil
    }

    func testFirstRunPlayDefaultsFalse() {
        let flags = HomeFlags(defaults: defaults)
        XCTAssertTrue(flags.shouldPlayFirstRun)
        XCTAssertTrue(flags.shouldTeachBox)
    }

    func testMarkFirstRunPlayPlayed() {
        var flags = HomeFlags(defaults: defaults)
        flags.markFirstRunPlayPlayed()
        XCTAssertFalse(flags.shouldPlayFirstRun)
        XCTAssertTrue(HomeFlags(defaults: defaults).shouldPlayFirstRun == false)
    }

    func testMarkBoxTaught() {
        var flags = HomeFlags(defaults: defaults)
        flags.markBoxTaught()
        XCTAssertFalse(flags.shouldTeachBox)
        XCTAssertTrue(HomeFlags(defaults: defaults).shouldTeachBox == false)
    }
}
