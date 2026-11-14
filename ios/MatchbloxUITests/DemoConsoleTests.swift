import UIKit
import XCTest

/// The line a background read brought back; the semaphore orders the write before the read.
private final class PasteboardRead: @unchecked Sendable {
    var value: String?
}

final class DemoConsoleTests: XCTestCase {
    /// What flint-auth says when its turn ends, on its card and in its peek.
    private let closingText = "Rotated on each refresh"

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testTheDemoHostDrawsTheConsole() {
        expectTheConsole(in: launched())
    }

    /// The stand-in the person left can only give way to the board through a new run: the
    /// label changes only with output, so a console stuck connecting would keep showing the stand-in.
    @MainActor
    func testTheDemoStartsFreshAfterTheBackground() {
        let app = launched()
        expectTheConsole(in: app)
        app.buttons["Return"].tap()
        expect(in: app, "label CONTAINS %@", "type 2 to answer")

        sendToTheBackground(app)
        app.activate()

        expect(in: app, "label CONTAINS %@ AND NOT label CONTAINS %@", "matchblox · demo", "type 2 to answer")
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

    /// Two taps on an option of the card's peek mark it, then send it, and the agent finishes: the
    /// invite rises with the line to run, and Copy puts that line on the pasteboard.
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

    /// The belt drives the stand-in as it drives a pane: Down moves to the second option, and
    /// Return picks it (T17).
    @MainActor
    func testTheBeltsArrowsAndReturnAnswer() {
        let app = launched()
        openTheStandIn(in: app)
        app.buttons["Down"].firstMatch.tap()
        // The moved cursor reaches the screen the stand-in shows before Return picks.
        waitUntilSteady(app.descendants(matching: .any)["demo-terminal"])
        app.buttons["Return"].firstMatch.tap()
        _ = expectTheAnswer(in: app)
    }

    @MainActor
    func testBackToMatchbloxReturnsToTheBoard() {
        let app = launched()
        openTheStandIn(in: app)
        app.buttons["Back to matchblox"].firstMatch.tap()
        expect(in: app, step: "the board after Back to matchblox", "label CONTAINS %@", "NEEDS YOU")
    }

    /// Keep looking closes the sheet over the agent's closing text, and the console goes on with no
    /// keyboard: the taps on a card reach the console, and the belt works. Then the ended screen
    /// carries the install line, and Open again starts a console with no sheet.
    @MainActor
    func testKeepLookingLeavesTheConsoleWorking() {
        let app = launched()
        let invite = answerTheAgent(in: app)
        app.buttons["Keep looking"].tap()
        XCTAssertTrue(invite.waitForNonExistence(timeout: 10), "the sheet did not close")

        let terminal = app.descendants(matching: .any)["demo-terminal"]
        XCTAssertTrue(terminal.label.contains(closingText), "the closing text is not there: \(terminal.label)")
        keep(app, "6 closing")

        // The belt works with the terminal unfocused; the board is where a card can be tapped. The
        // first tap on spark-docs chooses its card, and the second goes in.
        goInto(app, row: "spark-docs")
        expect(in: app, step: "the stand-in of spark-docs after the taps on its card", "label CONTAINS %@", "keys go to spark-docs")
        XCTAssertFalse(invite.exists, "the sheet came back")

        app.buttons["Back to matchblox"].firstMatch.tap()
        expect(in: app, step: "the board after Back to matchblox", "NOT label CONTAINS %@", "keys go to")
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
    /// raises it, and once it is down a tap still clicks. The stand-in takes its answer from the
    /// keys, so the second tap only counts as a click.
    @MainActor
    func testATapDoesNotRaiseTheKeyboard() {
        let app = launched()
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        let terminal = app.descendants(matching: .any)["demo-terminal"]
        waitUntilSteady(terminal)
        goInto(app, row: "flint-auth")
        expect(in: app, step: "the stand-in's question after the taps", "label CONTAINS %@ AND label CONTAINS %@", "type 2 to answer", "2. Rotate, keep")
        XCTAssertFalse(app.keyboards.firstMatch.exists, "a tap raised the keyboard")
        XCTAssertFalse(app.buttons["Hide keyboard"].firstMatch.exists, "a tap took the keyboard")
        keep(app, "7 stand-in with no keyboard")

        app.buttons["Show keyboard"].firstMatch.tap()
        dismissKeyboardTip(in: app)
        let hide = app.buttons["Hide keyboard"].firstMatch
        XCTAssertTrue(hide.waitForExistence(timeout: 10), "the belt's key did not raise the keyboard")
        hide.tap()
        let down = app.buttons["Show keyboard"].firstMatch.waitForExistence(timeout: 10)
        keep(app, "8 keyboard hidden")
        XCTAssertTrue(down, app.debugDescription)
        expect(in: app, step: "the stand-in to be back after the keyboard hid", "label CONTAINS %@ AND label CONTAINS %@", "type 2 to answer", "2. Rotate, keep")
        waitUntilSteady(terminal)
        let before = presses(on: terminal)
        tapRow(in: app, containing: "2. Rotate, keep")
        let deadline = Date().addingTimeInterval(10)
        while presses(on: terminal) <= before, Date() < deadline {
            Thread.sleep(forTimeInterval: 0.3)
        }
        XCTAssertGreaterThan(presses(on: terminal), before, "the tap with the keyboard down did not click")
        XCTAssertFalse(app.keyboards.firstMatch.exists, "the second tap raised the keyboard")
    }

    /// The presses of the left button the terminal has sent, as its accessibility value counts them.
    @MainActor
    private func presses(on terminal: XCUIElement) -> Int {
        let counts = terminal.value as? String ?? ""
        return Int(counts.components(separatedBy: " ").first?.dropFirst("press=".count) ?? "") ?? 0
    }

    /// A real host's tap, on the demo's screens: the console chose flint-auth by itself, so the
    /// first tap only chooses it (ruling 19). The tap on the card the person chose clicks once and
    /// goes into the stand-in (#351), its question is not clicked through, and the keyboard rises.
    @MainActor
    func testATapOnTheChosenCardClicksOnceAndRaisesTheKeyboard() {
        let app = launched(extra: ["-demoTapRaisesKeyboard"])
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        let terminal = app.descendants(matching: .any)["demo-terminal"]
        waitUntilSteady(terminal)
        tapRow(in: app, containing: "flint-auth")
        dismissKeyboardTip(in: app)
        waitUntilSteady(terminal)
        XCTAssertFalse(terminal.label.contains("‹ board"), "the first tap on a card the console chose went in")
        let before = presses(on: terminal)
        XCTAssertEqual(terminal.value as? String, "press=\(before) release=\(before)", "the first tap did not release")
        tapRow(in: app, containing: "flint-auth")
        expect(in: app, step: "the stand-in after the second tap on flint-auth", "label CONTAINS %@", "type 2 to answer")
        dismissKeyboardTip(in: app)
        XCTAssertTrue(app.buttons["Hide keyboard"].firstMatch.waitForExistence(timeout: 10), "the taps did not raise the keyboard. " + app.debugDescription)
        Thread.sleep(forTimeInterval: 1)
        XCTAssertEqual(terminal.value as? String, "press=\(before + 1) release=\(before + 1)", "the tap on the chosen card did not click exactly once")
        waitUntilSteady(terminal)
        XCTAssertFalse(terminal.label.contains("I rotate the token on each refresh"), "the tap clicked through to the answer: \(terminal.label)")
        keep(app, "9 real host tap")
    }

    /// Two quick taps in different places are two clicks at most, each at its own place.
    @MainActor
    func testTwoQuickTapsOnARealHostClickTwiceAtMost() {
        let app = launched(extra: ["-demoTapRaisesKeyboard"])
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        let terminal = app.descendants(matching: .any)["demo-terminal"]
        waitUntilSteady(terminal)
        let rows = terminal.label.components(separatedBy: "\n")
        guard let row = rows.firstIndex(where: { $0.contains("flint-auth") }) else {
            return XCTFail("no card shows flint-auth: \(terminal.label)")
        }
        terminal.coordinate(withNormalizedOffset: CGVector(dx: 0.3, dy: (Double(row) + 0.5) / Double(rows.count))).tap()
        terminal.coordinate(withNormalizedOffset: CGVector(dx: 0.8, dy: 0.3)).tap()
        dismissKeyboardTip(in: app)
        Thread.sleep(forTimeInterval: 2)
        let counts = terminal.value as? String ?? ""
        let presses = Int(counts.components(separatedBy: " ").first?.dropFirst("press=".count) ?? "") ?? -1
        XCTAssertTrue((1...2).contains(presses), "two taps made \(counts)")
    }

    /// The second option of flint-auth's question, in its card's peek, tapped by the terminal's
    /// cells as a thumb does: the first tap marks it, and the second sends it. The send moves the
    /// choice to the first card of NEEDS YOU, so once the turn ends and flint-auth is back there
    /// under its stuck ship, a tap chooses its card and the peek shows its last words. Returns the
    /// invite's heading, once it shows.
    @MainActor
    private func answerTheAgent(in app: XCUIApplication) -> XCUIElement {
        expect(in: app, "label CONTAINS %@ AND label CONTAINS %@", "NEEDS YOU", "2. Rotate, keep")
        tapRow(in: app, containing: "2. Rotate, keep", until: "2 again sends")
        expect(in: app, step: "the mark on the second option", "label CONTAINS %@", "2 again sends")
        waitUntilSteady(app.descendants(matching: .any)["demo-terminal"])
        tapRow(in: app, containing: "2. Rotate, keep")
        expect(in: app, step: "flint-auth back in NEEDS YOU after its turn", "label CONTAINS %@", "stuck conflicts with main")
        chooseBeforeTheInvite(app, row: "flint-auth")
        return expectTheAnswer(in: app)
    }

    /// Chooses the card at once, with no wait for a steady screen: a working spinner keeps the
    /// screen from ever being steady, so that wait takes its whole 30 s, and the invite rises in
    /// the meantime. A tap outside the invite's sheet closes it, and it never rises again. A
    /// second tap goes only to a card the first did not choose: on a chosen card it goes in.
    @MainActor
    private func chooseBeforeTheInvite(_ app: XCUIApplication, row text: String) {
        let terminal = app.descendants(matching: .any)["demo-terminal"]
        let isChosen = {
            terminal.label.components(separatedBy: "\n").first(where: { $0.contains(text) })?.contains("▌") == true
        }
        tapRow(in: app, containing: text)
        let deadline = Date().addingTimeInterval(4)
        while !isChosen(), Date() < deadline {
            Thread.sleep(forTimeInterval: 0.3)
        }
        if !isChosen() { tapRow(in: app, containing: text) }
    }

    @MainActor
    private func openTheStandIn(in app: XCUIApplication) {
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        goInto(app, row: "flint-auth")
        expect(in: app, step: "the stand-in after the tap on flint-auth", "label CONTAINS %@", "type 2 to answer")
    }

    /// The closing text shows first, and the invite rises over it after. The stand-in shows the
    /// agent's reply in its pane; the board shows the turn's last words in the chosen card's peek.
    @MainActor
    private func expectTheAnswer(in app: XCUIApplication) -> XCUIElement {
        let heading = app.staticTexts["Your agents wait the same way."]
        expect(in: app, "label CONTAINS %@ OR label CONTAINS %@", "I rotate the token on each refresh", closingText)
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

    /// Under 152 columns a tap on the chosen card goes into the pane, as Return does: the stand-in
    /// here (#351). A card that is not chosen takes two taps: the first chooses it.
    @MainActor
    private func goInto(_ app: XCUIApplication, row text: String) {
        // A card the console chose by itself takes two taps (ruling 19): the first chooses it.
        let terminal = app.descendants(matching: .any)["demo-terminal"]
        waitUntilSteady(terminal)
        tapRow(in: app, containing: text)
        waitUntilSteady(terminal)
        if terminal.label.contains("‹ board") { return }
        tapRow(in: app, containing: text, until: "‹ board")
    }

    /// Taps a row, and taps once more only when the first tap visibly did nothing in 6 seconds:
    /// the screen shows none of the texts in shown.
    @MainActor
    private func tapRow(in app: XCUIApplication, containing text: String, until shown: String...) {
        let terminal = app.descendants(matching: .any)["demo-terminal"]
        waitUntilSteady(terminal)
        tapRow(in: app, containing: text)
        let format = shown.map { _ in "label CONTAINS %@" }.joined(separator: " OR ")
        let reacted = XCTNSPredicateExpectation(predicate: NSPredicate(format: format, argumentArray: shown), object: terminal)
        if XCTWaiter().wait(for: [reacted], timeout: 6) != .completed {
            waitUntilSteady(terminal)
            if !shown.contains(where: { terminal.label.contains($0) }) { tapRow(in: app, containing: text) }
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
    private func launched(extra: [String] = []) -> XCUIApplication {
        let app = XCUIApplication()
        // The invite waits 20 s, not 2, after the turn ends: this runner reads the screen about
        // once a second, and the closing text must be seen before the invite covers it.
        app.launchArguments = ["-openDemoHost", "-demoSlowInvite"] + extra
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
