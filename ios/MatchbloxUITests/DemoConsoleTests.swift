import XCTest

final class DemoConsoleTests: XCTestCase {
    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testTheDemoHostDrawsTheConsole() {
        expectTheConsole(in: launched())
    }

    /// The chat the person left can only give way to the session list through a new run: the
    /// label changes only with output, so a console stuck connecting would keep showing the chat.
    @MainActor
    func testTheDemoStartsFreshAfterTheBackground() {
        let app = launched()
        expectTheConsole(in: app)
        app.buttons["Return"].tap()
        expect(in: app, "label CONTAINS %@", "keys go to flint-auth")

        XCUIDevice.shared.press(.home)
        _ = app.wait(for: .runningBackgroundSuspended, timeout: 10)
        XCTAssertNotEqual(app.state, .runningForeground, app.debugDescription)
        app.activate()

        expect(in: app, "label CONTAINS %@ AND NOT label CONTAINS %@", "matchblox · demo", "keys go to")
        XCTAssertFalse(app.buttons["Try again"].exists, app.debugDescription)
    }

    /// The real way in: an empty store, the scan view, `Try a demo`, and no camera prompt.
    @MainActor
    func testTryADemoOpensTheConsole() {
        let app = XCUIApplication()
        app.launchArguments = ["-resetHosts"]
        app.launch()

        let tryADemo = app.buttons["Try a demo"]
        XCTAssertTrue(tryADemo.waitForExistence(timeout: 30), app.debugDescription)
        XCTAssertGreaterThanOrEqual(tryADemo.frame.height, 44)
        XCTAssertTrue(app.frame.contains(tryADemo.frame), "Try a demo is off the screen")
        keep(app, "1 scan view")

        tryADemo.tap()
        keep(app, "2 demo scan")
        expectTheConsole(in: app)
        keep(app, "3 console")
        XCTAssertFalse(XCUIApplication(bundleIdentifier: "com.apple.springboard").alerts.firstMatch.exists)

        // The demo is saved: a new launch opens it straight away.
        app.terminate()
        app.launchArguments = []
        app.launch()
        expectTheConsole(in: app)
    }

    @MainActor
    private func keep(_ app: XCUIApplication, _ name: String) {
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
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
        expect(in: app, "label CONTAINS %@", "matchblox · demo")
    }

    @MainActor
    private func expect(in app: XCUIApplication, _ format: String, _ arguments: String...) {
        let terminal = app.descendants(matching: .any)["demo-terminal"]
        XCTAssertTrue(terminal.waitForExistence(timeout: 30), app.debugDescription)
        let shown = XCTNSPredicateExpectation(
            predicate: NSPredicate(format: format, argumentArray: arguments),
            object: terminal
        )
        XCTAssertEqual(XCTWaiter().wait(for: [shown], timeout: 30), .completed, app.debugDescription)
    }
}
