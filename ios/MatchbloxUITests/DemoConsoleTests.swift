import UIKit
import XCTest

/// The line a background read brought back; the semaphore orders the write before the read.
private final class PasteboardRead: @unchecked Sendable {
    var value: String?
}

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

    /// A tap on the session's row, a tap on its second option, and the agent finishes: the invite
    /// rises with the line to run, and Copy puts that line on the pasteboard.
    @MainActor
    func testAnAnswerReachesTheInvite() {
        let app = launched()
        let invite = answerTheAgent(in: app)
        let command = app.staticTexts["curl -fsSL https://matchblox.sh | sh"]
        XCTAssertTrue(command.exists, app.debugDescription)
        XCTAssertTrue(app.buttons["Send to my computer"].exists)
        XCTAssertTrue(app.buttons["Pair a host"].exists)
        keep(app, "4 invite")

        app.buttons["Copy"].tap()
        XCTAssertTrue(app.buttons["Copied"].waitForExistence(timeout: 5), "Copy gave no feedback")
        XCTAssertEqual(pasteboardLine(), "curl -fsSL https://matchblox.sh | sh")
        XCTAssertTrue(app.buttons["Copy"].waitForExistence(timeout: 10), "Copied did not go back to Copy")
        XCTAssertTrue(invite.exists)
    }

    /// Keep looking closes the sheet, and the console goes on: the belt and a tap still work, and
    /// the sheet does not come back. Then the ended screen offers both ways on.
    @MainActor
    func testKeepLookingLeavesTheConsoleWorking() {
        let app = launched()
        let invite = answerTheAgent(in: app)
        app.buttons["Keep looking"].tap()
        XCTAssertTrue(invite.waitForNonExistence(timeout: 10), "the sheet did not close")

        let terminal = app.descendants(matching: .any)["demo-terminal"]
        terminal.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
        let home = app.buttons["Back to matchblox"].firstMatch
        XCTAssertTrue(home.waitForExistence(timeout: 10), app.debugDescription)
        home.tap()
        let gone = XCTNSPredicateExpectation(
            predicate: NSPredicate(format: "NOT label CONTAINS %@", "I rotate the token"), object: terminal)
        XCTAssertEqual(XCTWaiter().wait(for: [gone], timeout: 30), .completed, "the belt did not reach the console")
        XCTAssertFalse(invite.exists, "the sheet came back")

        app.buttons["Interrupt"].firstMatch.tap()
        XCTAssertTrue(app.buttons["Open again"].waitForExistence(timeout: 30), app.debugDescription)
        XCTAssertTrue(app.buttons["Pair a host"].exists)
        keep(app, "5 ended")

        app.buttons["Open again"].tap()
        expectTheConsole(in: app)
        XCTAssertFalse(invite.exists, "a fresh opening starts with no sheet")
    }

    /// Taps by the terminal's cells, as a thumb does: the session's row, then its second option.
    /// Returns the invite's heading, once it shows.
    @MainActor
    private func answerTheAgent(in app: XCUIApplication) -> XCUIElement {
        expect(in: app, "label CONTAINS %@", "WAITS FOR YOU")
        // A first tap on the terminal only takes the keyboard. The belt's key does it with no tap.
        let show = app.buttons["Show keyboard"].firstMatch
        XCTAssertTrue(show.waitForExistence(timeout: 10), app.debugDescription)
        show.tap()
        Thread.sleep(forTimeInterval: 2)
        tapRow(in: app, containing: "flint-auth")
        expect(in: app, "label CONTAINS %@", "2. Rotate, keep")
        // The console takes a tap only after its screen has been quiet a moment.
        Thread.sleep(forTimeInterval: 1.5)
        tapRow(in: app, containing: "2. Rotate, keep")
        let heading = app.staticTexts["Your agents wait the same way."]
        XCTAssertTrue(heading.waitForExistence(timeout: 30), app.debugDescription)
        return heading
    }

    @MainActor
    private func tapRow(in app: XCUIApplication, containing text: String) {
        let terminal = app.descendants(matching: .any)["demo-terminal"]
        let rows = terminal.label.components(separatedBy: "\n")
        guard let row = rows.firstIndex(where: { $0.contains(text) }) else {
            return XCTFail("no row shows \(text): \(terminal.label)")
        }
        terminal.coordinate(withNormalizedOffset: CGVector(dx: 0.3, dy: (Double(row) + 0.5) / Double(rows.count))).tap()
    }

    /// The pasteboard belongs to the app, so a read from the test asks to paste, and the read waits
    /// for the answer. It runs off this thread, which taps Allow Paste if the question comes.
    @MainActor
    private func pasteboardLine() -> String? {
        let read = DispatchSemaphore(value: 0)
        let line = PasteboardRead()
        DispatchQueue.global().async {
            line.value = UIPasteboard.general.string
            read.signal()
        }
        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        let deadline = Date().addingTimeInterval(30)
        while Date() < deadline {
            if read.wait(timeout: .now() + 1) == .success { return line.value }
            let allow = springboard.buttons["Allow Paste"]
            if allow.exists { allow.tap() }
        }
        return nil
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
