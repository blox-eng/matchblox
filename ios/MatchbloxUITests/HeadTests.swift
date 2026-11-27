import XCTest

/// The one match head over the dock's belt, on the demo console. A roll is a straight drag round
/// the head's edge: one drag turns it at most about 11 notches.
final class HeadTests: XCTestCase {
    private let replyText = "I rotate the token on each refresh"
    /// What spark-docs says to any line sent in its chat.
    private let sparkReply = "Got it. I am working on that."

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

    /// A still hold in flint-auth's chat fills the ring, then lists this host's sessions with
    /// flint-auth marked. The card goes at the lift, so the test reads it from the head's DEBUG value.
    @MainActor
    func testHoldShowsSessions() {
        let app = launched()
        openTheQuestion(in: app)
        let head = theHead(in: app)
        head.press(forDuration: 1.0)
        let shown = sessionsShown(on: head)
        for row in ["▸ flint-auth · asks · here", "hearth-api", "spark-docs"] {
            XCTAssertTrue(shown.contains(row), "the card has no \(row): \(shown)")
        }
        XCTAssertTrue(app.staticTexts["sessions-card"].waitForNonExistence(timeout: 3), "the card stayed after the lift")
    }

    /// Surface: the owner's sessions. The list does not answer: the card says so, and the lift
    /// opens nothing and leaves the chat as it was.
    @MainActor
    func testHostListFails() {
        let app = launched(extra: ["-demoSessionsFail"])
        openTheQuestion(in: app)
        let head = theHead(in: app)
        head.press(forDuration: 1.0)
        XCTAssertEqual(sessionsShown(on: head), "sessions not reachable · try again")
        stillInTheChat(app)
    }

    /// Surface: the owner's sessions. A lift on the marked session goes nowhere: the console
    /// does not connect again, and the chat stays.
    @MainActor
    func testLiftOnCurrentChangesNothing() {
        let app = launched()
        openTheQuestion(in: app)
        let head = theHead(in: app)
        head.press(forDuration: 1.0)
        XCTAssertTrue(sessionsShown(on: head).contains("▸ flint-auth · asks · here"), "flint-auth is not the chosen and marked row")
        stillInTheChat(app)
    }

    /// Surface: an agent's tools. A press of 0.45 s is neither a tap nor a hold: it answers
    /// nothing, clicks nothing, and shows no card.
    @MainActor
    func testHoldEndsEarlySendsNothing() {
        let app = launched()
        openTheQuestion(in: app)
        let terminal = theTerminal(in: app)
        let presses = terminal.value as? String
        let head = theHead(in: app)
        head.press(forDuration: 0.45)
        Thread.sleep(forTimeInterval: 3)
        XCTAssertFalse(terminal.label.contains(replyText), "a 0.45 s press answered the agent")
        XCTAssertEqual(terminal.value as? String, presses, "a 0.45 s press clicked")
        XCTAssertFalse((head.value as? String ?? "").contains("sessions="), "a 0.45 s press showed the sessions")
        XCTAssertFalse(app.staticTexts["sessions-card"].exists, "a 0.45 s press left a card")
    }

    /// A drag on the belt brings it in front of the head: the head is out of reach while the
    /// belt moves, and back 1.4 s after it stops.
    @MainActor
    func testBeltDragBringsItForward() {
        let app = launched()
        openTheChat(app, row: "spark-docs")
        let head = theHead(in: app)
        XCTAssertTrue(head.isHittable, "the head is out of reach before the drag")
        // Along the belt's keys, left of the head and under it, then still before the lift.
        at(head, -150, 60).press(forDuration: 0.05, thenDragTo: at(head, 60, 60), withVelocity: .slow, thenHoldForDuration: 0.2)
        XCTAssertFalse(head.exists && head.isHittable, "the head stayed in front of the dragged belt")
        XCTAssertTrue(becomesHittable(head, within: 3), "the head did not come back after the belt stopped")
    }

    /// Return on the system keyboard sends the line, then the keyboard folds and the head is back.
    @MainActor
    func testKeyboardFoldsAfterSend() {
        let app = launched()
        openTheChat(app, row: "spark-docs")
        raiseTheKeyboard(app)
        theTerminal(in: app).typeText("ok\n")
        expect(in: app, step: "spark-docs's reply to ok", "label CONTAINS %@", sparkReply)
        XCTAssertTrue(app.buttons["Show keyboard"].firstMatch.waitForExistence(timeout: 10), "the keyboard stayed up after the send")
        XCTAssertFalse(app.keyboards.firstMatch.exists, "the keyboard stayed up after the send")
        XCTAssertTrue(becomesHittable(theHead(in: app), within: 5), "the head did not come back after the fold")
    }

    /// Anti-goal: the keyboard folds before the line is sent. Text with no Return keeps the
    /// keyboard up and stays in the line; Return sends all of it, and only then the keyboard folds.
    @MainActor
    func testFoldKeepsText() {
        let app = launched()
        openTheChat(app, row: "spark-docs")
        raiseTheKeyboard(app)
        let terminal = theTerminal(in: app)
        terminal.typeText("ok")
        expect(in: app, step: "ok in the chat's line", "label CONTAINS %@", "› ok")
        Thread.sleep(forTimeInterval: 1.5)
        XCTAssertTrue(app.buttons["Hide keyboard"].firstMatch.exists, "the keyboard folded before Return")
        XCTAssertTrue(terminal.label.contains("› ok"), "the line lost its text before Return: \(terminal.label)")
        terminal.typeText("\n")
        expect(in: app, step: "spark-docs's reply to ok", "label CONTAINS %@", sparkReply)
        expect(in: app, step: "the line empty after the send", "NOT label CONTAINS %@", "› ok")
        XCTAssertTrue(app.buttons["Show keyboard"].firstMatch.waitForExistence(timeout: 10), "the keyboard stayed up after the send")
    }

    /// With the keyboard up, the lifted belt's `⏎` sends Enter, and the keyboard folds.
    @MainActor
    func testLiftedEnterFolds() {
        let app = launched()
        openTheChat(app, row: "spark-docs")
        raiseTheKeyboard(app)
        theTerminal(in: app).typeText("ok")
        expect(in: app, step: "ok in the chat's line", "label CONTAINS %@", "› ok")
        // The lifted belt's Return, not the system keyboard's: the belt sits above the keyboard.
        let keyboard = app.keyboards.firstMatch.frame
        let returns = app.buttons.matching(NSPredicate(format: "label == %@", "Return")).allElementsBoundByIndex
        guard let enter = returns.first(where: { $0.isHittable && $0.frame.maxY <= keyboard.minY + 1 }) else {
            return XCTFail("no ⏎ on the lifted belt. " + app.debugDescription)
        }
        enter.tap()
        expect(in: app, step: "spark-docs's reply to the belt's ⏎", "label CONTAINS %@", sparkReply)
        XCTAssertTrue(app.buttons["Show keyboard"].firstMatch.waitForExistence(timeout: 10), "the keyboard stayed up after the belt's ⏎")
        XCTAssertTrue(becomesHittable(theHead(in: app), within: 5), "the head did not come back after the fold")
    }

    // MARK: Home sets the head

    /// Home sets the left mark, and the console's head shows it at its next touch; `undo` takes
    /// it back. A 0.45 s press on the middle sends nothing, so the marks are read safely.
    @MainActor
    func testHomeSetsLeftMark() {
        let app = launched(extra: ["-resetHead"])
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        openHome(app)
        app.buttons["home-head-left"].tap()
        app.buttons["Esc cancel"].tap()
        backToTheConsole(app)
        XCTAssertEqual(leftMark(in: app), "cancel", "the left mark did not take Esc")

        openHome(app)
        app.buttons["undo"].tap()
        backToTheConsole(app)
        XCTAssertEqual(leftMark(in: app), "cursor", "undo did not bring back ← cursor")
    }

    /// Surface: an agent's tools. `^C` is in the list, dim, with its reason, and a tap on it
    /// leaves the mark as it was.
    @MainActor
    func testInterruptDim() {
        let app = launched(extra: ["-resetHead"])
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        openHome(app)
        let left = app.buttons["home-head-left"]
        left.tap()
        let interrupt = app.buttons["^C interrupt"]
        XCTAssertTrue(interrupt.waitForExistence(timeout: 5), "no ^C in the list. " + app.debugDescription)
        XCTAssertFalse(interrupt.isEnabled, "^C can be set on a mark")
        XCTAssertTrue(app.staticTexts["stays on the belt: a roll can brush the edge"].exists, "^C shows no reason")
        interrupt.tap()
        XCTAssertEqual(left.label, "left mark, cursor", "a tap on ^C changed the mark")
    }

    /// `blue tip` colours the dock's head at once, and it stays blue after a relaunch.
    @MainActor
    func testHomeSetsColour() {
        var app = launched(extra: ["-resetHead"])
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        XCTAssertTrue((theHead(in: app).value as? String ?? "").hasPrefix("match red"), "the head did not come match red")
        openHome(app)
        let blue = app.buttons["blue tip"]
        blue.tap()
        XCTAssertTrue(blue.isSelected, "blue tip is not the chosen colour")
        backToTheConsole(app)
        XCTAssertTrue((theHead(in: app).value as? String ?? "").hasPrefix("blue tip"),
                      "the head is \(theHead(in: app).value ?? "nothing"), not blue tip")

        app.terminate()
        app = launched()
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        XCTAssertTrue((theHead(in: app).value as? String ?? "").hasPrefix("blue tip"), "the colour did not stay after a relaunch")
    }

    // MARK: Helpers

    /// BOX opens home over the console. The one-time BOX teach may hold the drawer open for a
    /// moment; a tap on the grip toggles it.
    @MainActor
    private func openHome(_ app: XCUIApplication) {
        let box = app.buttons["Home, the box"]
        let layouts = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Layouts")).firstMatch
        let deadline = Date().addingTimeInterval(20)
        while !box.isHittable, Date() < deadline {
            let width = layouts.frame.width
            layouts.coordinate(withNormalizedOffset: CGVector(dx: (width - 18) / width, dy: 0.5)).tap()
            _ = box.waitForExistence(timeout: 2)
        }
        box.tap()
        XCTAssertTrue(app.buttons["home-head-left"].waitForExistence(timeout: 10), "home shows no head. " + app.debugDescription)
    }

    @MainActor
    private func backToTheConsole(_ app: XCUIApplication) {
        let back = app.buttons["Back to demo"]
        XCTAssertTrue(back.waitForExistence(timeout: 10), app.debugDescription)
        back.tap()
        _ = theHead(in: app)
    }

    /// The word of the head's left mark, read while a 0.45 s press shows the marks.
    @MainActor
    private func leftMark(in app: XCUIApplication) -> String {
        theHead(in: app).press(forDuration: 0.45)
        let mark = app.staticTexts["head-mark-left"]
        XCTAssertTrue(mark.waitForExistence(timeout: 2), "no left mark on touch. " + app.debugDescription)
        return mark.label
    }

    /// The head's bottom mark, `⌨`, raises the keyboard.
    @MainActor
    private func raiseTheKeyboard(_ app: XCUIApplication) {
        at(theHead(in: app), 0, 40).tap()
        dismissKeyboardTip(in: app)
        XCTAssertTrue(app.buttons["Hide keyboard"].firstMatch.waitForExistence(timeout: 10), "⌨ raised no keyboard. " + app.debugDescription)
    }

    @MainActor
    private func becomesHittable(_ element: XCUIElement, within seconds: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(seconds)
        while !(element.exists && element.isHittable), Date() < deadline {
            Thread.sleep(forTimeInterval: 0.2)
        }
        return element.exists && element.isHittable
    }

    /// The last sessions card the head showed.
    @MainActor
    private func sessionsShown(on head: XCUIElement) -> String {
        let value = head.value as? String ?? ""
        guard let at = value.range(of: "sessions=") else { return "" }
        return String(value[at.upperBound...])
    }

    /// flint-auth's chat still shows 3 s on, with its question unanswered: nothing was opened.
    @MainActor
    private func stillInTheChat(_ app: XCUIApplication) {
        Thread.sleep(forTimeInterval: 3)
        let terminal = theTerminal(in: app)
        XCTAssertTrue(terminal.label.contains("send to flint-auth"), "the chat went: \(terminal.label)")
        XCTAssertTrue(terminal.label.contains("2. Rotate, keep"), "the question went: \(terminal.label)")
        XCTAssertFalse(terminal.label.contains(replyText), "the hold answered the agent")
    }

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
