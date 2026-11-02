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

        sendToTheBackground(app)
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

    /// Keep looking closes the sheet over the agent's closing text, and the console goes on with no
    /// keyboard: one tap on a row reaches the console, and the belt works. Then the ended screen
    /// carries the install line, and Open again starts a console with no sheet.
    @MainActor
    func testKeepLookingLeavesTheConsoleWorking() {
        let app = launched()
        let invite = answerTheAgent(in: app)
        app.buttons["Keep looking"].tap()
        XCTAssertTrue(invite.waitForNonExistence(timeout: 10), "the sheet did not close")

        let terminal = app.descendants(matching: .any)["demo-terminal"]
        XCTAssertTrue(terminal.label.contains("I rotate the token on each refresh"), "the closing text is not there: \(terminal.label)")
        keep(app, "6 closing")

        // The belt works with the terminal unfocused; the list is where a row can be tapped.
        app.buttons["Back to matchblox"].firstMatch.tap()
        expect(in: app, "NOT label CONTAINS %@", "I rotate the token")
        tapRow(in: app, containing: "hearth-api", until: "keys go to hearth-api")
        expect(in: app, step: "the chat of hearth-api after the tap on its row", "label CONTAINS %@", "keys go to hearth-api")
        XCTAssertFalse(invite.exists, "the sheet came back")

        app.buttons["Back to matchblox"].firstMatch.tap()
        expect(in: app, step: "the list after the second Back to matchblox", "NOT label CONTAINS %@", "keys go to")
        // A tap does not raise the keyboard on the demo; the belt's key does, and typing works.
        app.buttons["Show keyboard"].firstMatch.tap()
        dismissKeyboardTip(in: app)
        XCTAssertTrue(app.buttons["Hide keyboard"].firstMatch.waitForExistence(timeout: 10), app.debugDescription)
        terminal.typeText("q")
        XCTAssertTrue(app.buttons["Open again"].waitForExistence(timeout: 30), app.debugDescription)
        XCTAssertTrue(app.buttons["Pair a host"].isHittable, "the keyboard covers the ended screen")
        XCTAssertFalse(app.keyboards.firstMatch.exists, "the keyboard stayed up after the quit")
        XCTAssertTrue(app.buttons["Pair a host"].exists)
        XCTAssertTrue(app.buttons["Copy"].exists)
        XCTAssertTrue(app.buttons["Send to my computer"].exists)
        XCTAssertTrue(app.staticTexts["curl -fsSL https://matchblox.sh | sh"].exists)
        keep(app, "5 ended")

        app.buttons["Open again"].tap()
        expectTheConsole(in: app)
        XCTAssertFalse(invite.exists, "a fresh opening starts with no sheet")
    }

    /// A tap clicks and leaves the keyboard down, so it never covers the question. The belt's key
    /// raises it, and once it is down a tap still clicks.
    @MainActor
    func testATapDoesNotRaiseTheKeyboard() {
        let app = launched()
        expect(in: app, "label CONTAINS %@", "WAITS FOR YOU")
        let terminal = app.descendants(matching: .any)["demo-terminal"]
        waitUntilSteady(terminal)
        tapRow(in: app, containing: "flint-auth")
        expect(in: app, step: "the chat's question after the first tap", "label CONTAINS %@", "2. Rotate, keep")
        XCTAssertFalse(app.keyboards.firstMatch.exists, "a tap raised the keyboard")
        XCTAssertFalse(app.buttons["Hide keyboard"].firstMatch.exists, "a tap took the keyboard")
        keep(app, "7 chat with no keyboard")

        app.buttons["Show keyboard"].firstMatch.tap()
        dismissKeyboardTip(in: app)
        let hide = app.buttons["Hide keyboard"].firstMatch
        XCTAssertTrue(hide.waitForExistence(timeout: 10), "the belt's key did not raise the keyboard")
        hide.tap()
        let down = app.buttons["Show keyboard"].firstMatch.waitForExistence(timeout: 10)
        keep(app, "8 keyboard hidden")
        XCTAssertTrue(down, app.debugDescription)
        expect(in: app, step: "the chat to be back after the keyboard hid", "label CONTAINS %@ AND label CONTAINS %@", "keys go to flint-auth", "2. Rotate, keep")
        waitUntilSteady(terminal)
        tapRow(in: app, containing: "2. Rotate, keep")
        if !terminal.label.contains("I rotate the token on each refresh") {
            // The tap is a second one only when the first visibly did nothing.
            let answered = NSPredicate(format: "label CONTAINS %@", "I rotate the token on each refresh")
            let shown = XCTNSPredicateExpectation(predicate: answered, object: terminal)
            if XCTWaiter().wait(for: [shown], timeout: 6) != .completed, terminal.label.contains("2. Rotate, keep") {
                waitUntilSteady(terminal)
                tapRow(in: app, containing: "2. Rotate, keep")
            }
        }
        expect(in: app, step: "the answer after the tap on the option", "label CONTAINS %@", "I rotate the token on each refresh")
    }

    /// Taps by the terminal's cells, as a thumb does, with no keyboard taken first: the session's
    /// row opens its chat, and the second option ends in the invite, which rises after the closing
    /// text has been shown. Returns the invite's heading, once it shows.
    @MainActor
    private func answerTheAgent(in app: XCUIApplication) -> XCUIElement {
        expect(in: app, "label CONTAINS %@", "WAITS FOR YOU")
        let terminal = app.descendants(matching: .any)["demo-terminal"]
        tapRow(in: app, containing: "flint-auth", until: "2. Rotate, keep")
        expect(in: app, step: "the chat's question after the tap on flint-auth", "label CONTAINS %@", "2. Rotate, keep")
        tapRow(in: app, containing: "2. Rotate, keep", until: "I rotate the token on each refresh")
        let heading = app.staticTexts["Your agents wait the same way."]
        expect(in: app, "label CONTAINS %@", "I rotate the token on each refresh")
        XCTAssertFalse(heading.exists, "the invite covered the closing text")
        XCTAssertTrue(heading.waitForExistence(timeout: 30), app.debugDescription)
        return heading
    }

    /// The screen and its place have stopped changing: the keyboard has come or gone and the
    /// console's quiet has passed, so a tap goes to the row it was computed for.
    @MainActor
    private func waitUntilSteady(_ terminal: XCUIElement) {
        var last = ""
        var same = 0
        let deadline = Date().addingTimeInterval(30)
        while same < 4, Date() < deadline {
            let now = "\(terminal.frame)\(terminal.label)"
            same = now == last ? same + 1 : 0
            last = now
            Thread.sleep(forTimeInterval: 0.3)
        }
    }

    /// Taps a row, and taps once more only when the first tap visibly did nothing in 6 seconds.
    @MainActor
    private func tapRow(in app: XCUIApplication, containing text: String, until shown: String) {
        let terminal = app.descendants(matching: .any)["demo-terminal"]
        waitUntilSteady(terminal)
        tapRow(in: app, containing: text)
        let reacted = XCTNSPredicateExpectation(predicate: NSPredicate(format: "label CONTAINS %@", shown), object: terminal)
        if XCTWaiter().wait(for: [reacted], timeout: 6) != .completed {
            waitUntilSteady(terminal)
            if !terminal.label.contains(shown) { tapRow(in: app, containing: text) }
        }
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

    /// The system's one-time "slide to type" tip covers the screen the first time the keyboard rises.
    @MainActor
    private func dismissKeyboardTip(in app: XCUIApplication) {
        let next = app.buttons["Continue"]
        if next.waitForExistence(timeout: 2) { next.tap() }
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
    private func expect(in app: XCUIApplication, step: String = "", _ format: String, _ arguments: String...) {
        let terminal = app.descendants(matching: .any)["demo-terminal"]
        let at = step.isEmpty ? "" : "Timed out waiting for \(step). "
        XCTAssertTrue(terminal.waitForExistence(timeout: 30), at + app.debugDescription)
        let shown = XCTNSPredicateExpectation(
            predicate: NSPredicate(format: format, argumentArray: arguments),
            object: terminal
        )
        XCTAssertEqual(XCTWaiter().wait(for: [shown], timeout: 30), .completed, at + app.debugDescription)
    }

    /// The home button sometimes leaves the app in front on a busy runner, so it is pressed again
    /// until the app is out of the foreground.
    @MainActor
    private func sendToTheBackground(_ app: XCUIApplication) {
        let deadline = Date().addingTimeInterval(30)
        while app.state == .runningForeground, Date() < deadline {
            XCUIDevice.shared.press(.home)
            _ = app.wait(for: .runningBackgroundSuspended, timeout: 5)
        }
        XCTAssertNotEqual(app.state, .runningForeground, "The app never left the foreground after the home button. " + app.debugDescription)
    }
}
