import XCTest

final class DemoConsoleTests: XCTestCase {
    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testTheDemoHostDrawsTheConsole() {
        expectTheConsole(in: launched())
    }

    /// A start is refused while the last run lives, which the Try again screen would show.
    @MainActor
    func testTheDemoOpensAgainAfterTheBackground() {
        let app = launched()
        expectTheConsole(in: app)
        XCUIDevice.shared.press(.home)
        app.activate()
        XCTAssertFalse(app.buttons["Try again"].waitForExistence(timeout: 5), app.debugDescription)
        expectTheConsole(in: app)
    }

    @MainActor
    private func launched() -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-openDemoHost"]
        app.launch()
        return app
    }

    @MainActor
    private func expectTheConsole(in app: XCUIApplication) {
        let terminal = app.descendants(matching: .any)["demo-terminal"]
        XCTAssertTrue(terminal.waitForExistence(timeout: 30), app.debugDescription)
        let header = XCTNSPredicateExpectation(
            predicate: NSPredicate(format: "label CONTAINS %@", "matchblox · demo"),
            object: terminal
        )
        XCTAssertEqual(XCTWaiter().wait(for: [header], timeout: 30), .completed, app.debugDescription)
    }
}
