import XCTest

/// The one match head over the dock's belt, on the demo console. A roll is a straight drag round
/// the head's edge: one drag turns it at most about 11 notches.
final class HeadTests: XCTestCase {
    private let replyText = "I rotate the token on each refresh"

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    /// Surface: an agent's tools. A drag along the edge from the right mark to the bottom one
    /// lifts on `⌨`, and sends no key: the line stays as it was and no keyboard rises.
    @MainActor
    func testRollEndingOnEdgeSendsNothing() {
        let app = launched()
        openTheChat(app, row: "spark-docs")
        let terminal = theTerminal(in: app)
        waitUntilSteady(terminal)
        let before = terminal.label
        let presses = terminal.value as? String
        let head = theHead(in: app)
        at(head, 46, 0).press(forDuration: 0.05, thenDragTo: at(head, 0, 46))
        Thread.sleep(forTimeInterval: 1.5)
        waitUntilSteady(terminal)
        XCTAssertEqual(terminal.label, before, "a roll that ended on the edge changed the chat")
        XCTAssertEqual(terminal.value as? String, presses, "a roll that ended on the edge clicked")
        XCTAssertFalse(app.keyboards.firstMatch.exists, "a roll that ended on ⌨ raised the keyboard")
        XCTAssertFalse(app.buttons["Hide keyboard"].firstMatch.exists, "a roll that ended on ⌨ took the keyboard")
    }

    /// One head above the belt on every layout, and no belt key of that name.
    @MainActor
    func testHeadIsOnEveryLayout() {
        let app = launched()
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        for layout in ["Steer", "Tmux", "Mine"] {
            pick(layout, in: app)
            XCTAssertTrue(theHead(in: app).isHittable, "no head on \(layout). " + app.debugDescription)
            let named = app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", "Match head"))
            XCTAssertEqual(named.count, 1, "on \(layout) a belt key is named Match head too")
        }
    }

    /// A notch clockwise at flint-auth's question highlights its second option, and a tap on the
    /// middle answers with it, through the pick the host checks.
    @MainActor
    func testTapAnswers() {
        let app = launched()
        openTheQuestion(in: app)
        rollOneNotchDown(app)
        expectLine(in: app, "2 of ")
        theHead(in: app).tap()
        expect(in: app, step: "flint-auth's reply to the pick", "label CONTAINS %@", replyText)
    }

    /// The agent's cursor moves between the roll and the tap: the host refuses the pick, nothing
    /// is answered, and the handle row says why.
    @MainActor
    func testStalePickRefused() {
        let app = launched(extra: ["-demoQuestionChanges"])
        openTheQuestion(in: app)
        rollOneNotchDown(app)
        expectLine(in: app, "2 of ")
        theHead(in: app).tap()
        expectLine(in: app, "the question changed · roll again")
        Thread.sleep(forTimeInterval: 3)
        XCTAssertFalse(theTerminal(in: app).label.contains(replyText), "a stale pick answered the agent")
    }

    /// A fast roll up hearth-api's output shows a card that names the step at the top of the
    /// view; it goes 0.6 s after the roll ends.
    @MainActor
    func testStepCardShows() {
        let app = launched()
        openTheChat(app, row: "hearth-api")
        expect(in: app, step: "the step in hearth-api's chat", "label CONTAINS %@", "✦ Running go test")
        let head = theHead(in: app)
        // Anticlockwise, close above the middle: about 11 notches, the last at 3 lines a notch.
        at(head, -46, 8).press(forDuration: 0.05, thenDragTo: at(head, 190, 8), withVelocity: .fast, thenHoldForDuration: 0)
        XCTAssertTrue((head.value as? String ?? "").contains("card=✦ Running go test"),
                      "no card named the step: \(head.value ?? "")")
        let card = app.staticTexts["step-card"]
        XCTAssertTrue(card.waitForNonExistence(timeout: 3), "the card stayed after the roll")
    }

    /// A touch shows the four marks round the head; they are gone 0.9 s after the lift.
    @MainActor
    func testTouchShowsMarks() {
        let app = launched()
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        let head = theHead(in: app)
        head.press(forDuration: 0.2)
        for word in ["back", "cursor", "type"] {
            XCTAssertTrue(app.staticTexts[word].firstMatch.exists, "no \(word) mark on touch. " + app.debugDescription)
        }
        Thread.sleep(forTimeInterval: 1.2)
        for word in ["back", "cursor", "type"] {
            XCTAssertFalse(app.staticTexts[word].firstMatch.exists, "the \(word) mark stayed after the lift")
        }
    }

    /// A press on the bottom mark raises the keyboard.
    @MainActor
    func testPressTypeRaisesKeyboard() {
        let app = launched()
        openTheChat(app, row: "spark-docs")
        at(theHead(in: app), 0, 40).tap()
        dismissKeyboardTip(in: app)
        XCTAssertTrue(app.buttons["Hide keyboard"].firstMatch.waitForExistence(timeout: 10), "⌨ raised no keyboard. " + app.debugDescription)
    }

    // MARK: Helpers

    @MainActor
    private func theHead(in app: XCUIApplication) -> XCUIElement {
        let head = app.descendants(matching: .any)["match-head"]
        XCTAssertTrue(head.waitForExistence(timeout: 30), "no match head. " + app.debugDescription)
        return head
    }

    @MainActor
    private func theTerminal(in app: XCUIApplication) -> XCUIElement {
        app.descendants(matching: .any)["demo-terminal"]
    }

    /// A point `x`, `y` points from the head's centre.
    private func at(_ head: XCUIElement, _ x: CGFloat, _ y: CGFloat) -> XCUICoordinate {
        head.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).withOffset(CGVector(dx: x, dy: y))
    }

    /// From the top mark about 24° clockwise: one notch. The press first waits for the menu read
    /// the touch starts; the top mark does not repeat.
    @MainActor
    private func rollOneNotchDown(_ app: XCUIApplication) {
        let head = theHead(in: app)
        at(head, 0, -46).press(forDuration: 0.5, thenDragTo: at(head, 20, -44))
    }

    /// flint-auth's chat, where its question shows.
    @MainActor
    private func openTheQuestion(in app: XCUIApplication) {
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        openTheChat(app, row: "flint-auth")
        expect(in: app, step: "the options in flint-auth's chat", "label CONTAINS %@", "2. Rotate, keep")
        waitUntilSteady(theTerminal(in: app))
    }

    /// The dock's handle row reads a line that starts with `prefix`.
    @MainActor
    private func expectLine(in app: XCUIApplication, _ prefix: String) {
        let row = app.descendants(matching: .any).matching(NSPredicate(format: "label BEGINSWITH %@", "Layouts")).firstMatch
        let deadline = Date().addingTimeInterval(10)
        while !((row.value as? String)?.hasPrefix(prefix) ?? false), Date() < deadline {
            Thread.sleep(forTimeInterval: 0.2)
        }
        XCTAssertTrue((row.value as? String)?.hasPrefix(prefix) ?? false,
                      "the handle row reads \(row.value ?? "nothing"), not \(prefix). " + app.debugDescription)
    }

    /// The drawer opens on a tap near the grip, and the layout's match in it is picked.
    @MainActor
    private func pick(_ layout: String, in app: XCUIApplication) {
        let layouts = app.descendants(matching: .any).matching(NSPredicate(format: "label BEGINSWITH %@", "Layouts")).firstMatch
        let match = app.buttons[layout]
        let deadline = Date().addingTimeInterval(20)
        while !match.isHittable, Date() < deadline {
            let width = layouts.frame.width
            layouts.coordinate(withNormalizedOffset: CGVector(dx: (width - 18) / width, dy: 0.5)).tap()
            _ = match.waitForExistence(timeout: 2)
        }
        match.tap()
        _ = match.waitForNonExistence(timeout: 5)
    }

    /// On a phone the list shows first, and one tap on a row opens its chat.
    @MainActor
    private func openTheChat(_ app: XCUIApplication, row text: String) {
        expect(in: app, "label CONTAINS %@", text)
        let terminal = theTerminal(in: app)
        waitUntilSteady(terminal)
        let rows = terminal.label.components(separatedBy: "\n")
        guard let row = rows.firstIndex(where: { $0.contains(text) }) else {
            return XCTFail("no row shows \(text): \(terminal.label)")
        }
        terminal.coordinate(withNormalizedOffset: CGVector(dx: 0.3, dy: (Double(row) + 0.5) / Double(rows.count))).tap()
        expect(in: app, step: "the chat of \(text) after the tap on its row", "label CONTAINS %@", "send to \(text)")
    }

    /// The screen and its place have stopped changing.
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

    /// The system's one-time "slide to type" tip covers the screen the first time the keyboard rises.
    @MainActor
    private func dismissKeyboardTip(in app: XCUIApplication) {
        let next = app.buttons["Continue"]
        if next.waitForExistence(timeout: 2) { next.tap() }
    }

    @MainActor
    private func launched(extra: [String] = []) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-openDemoHost", "-demoSlowInvite"] + extra
        app.launch()
        return app
    }

    @MainActor
    private func expect(in app: XCUIApplication, step: String = "", _ format: String, _ arguments: String...) {
        let terminal = theTerminal(in: app)
        let at = step.isEmpty ? "" : "Timed out waiting for \(step). "
        XCTAssertTrue(terminal.waitForExistence(timeout: 30), at + app.debugDescription)
        let shown = XCTNSPredicateExpectation(predicate: NSPredicate(format: format, argumentArray: arguments), object: terminal)
        XCTAssertEqual(XCTWaiter().wait(for: [shown], timeout: 30), .completed, at + app.debugDescription)
    }
}
