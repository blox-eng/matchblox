import XCTest

/// The one match head over the dock's belt, on the demo console, on Joystick. A roll is a straight
/// drag round the head's edge: one drag turns it at most about 11 notches. A push is a drag from
/// the middle; a push sends its key at 16 pt.
final class HeadTests: XCTestCase {
    private let replyText = "I rotate the token on each refresh"
    /// What spark-docs says to any line sent in its chat.
    private let sparkReply = "Got it. I am working on that."
    /// `DockHeadView.row`: the dock grows by this on Joystick, and the terminal shrinks by it.
    private static let headRow: CGFloat = 72

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    /// Surface: an agent's tools. At flint-auth's question a slow roll up the edge, then a flick
    /// down that coasts on after the lift, send no key: no option is answered, the chat stays on
    /// flint-auth (an arrow there would choose another session), nothing clicks, and no keyboard
    /// rises.
    @MainActor
    func testRollSendsNoKey() {
        let app = launched()
        openTheQuestion(in: app)
        let terminal = theTerminal(in: app)
        let presses = terminal.value as? String
        let head = theHead(in: app)
        rollUp(head)
        flickDown(head)
        Thread.sleep(forTimeInterval: 3)
        XCTAssertEqual(terminal.value as? String, presses, "a roll or a flick clicked")
        XCTAssertTrue(terminal.label.contains("send to flint-auth"), "a roll or a flick left flint-auth's chat: \(terminal.label)")
        XCTAssertFalse(terminal.label.contains(replyText), "a roll or a flick answered the agent")
        XCTAssertFalse(app.keyboards.firstMatch.exists, "a roll or a flick raised the keyboard")
    }

    /// Surface: an agent's tools. A drag along the edge from the right mark to the bottom one
    /// lifts on `↓`, and sends no key: the line stays as it was and no keyboard rises.
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

    /// Only Joystick carries the head, and a switch in Layouts carries it in and drops it. Steer
    /// gives the head's row back to the terminal, keeps the keyboard up after Enter, and a drag on
    /// its belt hides nothing; Mine shows the belt's small head key; Joystick again has the head.
    @MainActor
    func testHeadOnlyOnJoystick() {
        // A Mine that holds the belt's small head key: the demo opens no Mine editor.
        let mine = Data(#"{"keys":[{"id":"◂","cells":1},{"id":"Wheel","cells":1}]}"#.utf8)
        let app = launched(extra: ["-belt.mine", "<" + mine.map { String(format: "%02x", $0) }.joined() + ">"], joystick: false)
        openTheChat(app, row: "spark-docs")
        let terminal = theTerminal(in: app)
        let head = app.descendants(matching: .any)["match-head"]

        pick("Joystick", in: app)
        XCTAssertTrue(theHead(in: app).isHittable, "no head on Joystick. " + app.debugDescription)
        XCTAssertGreaterThan(head.frame.minY, terminal.frame.maxY, "the head is not in the dock, above the belt")
        // The tray's hold on the terminal's size lets go once the drawer has closed.
        Thread.sleep(forTimeInterval: 0.5)
        let joystickHeight = terminal.frame.height

        pick("Steer", in: app)
        XCTAssertTrue(head.waitForNonExistence(timeout: 5), "the head stayed on Steer. " + app.debugDescription)
        XCTAssertTrue(heightBecomes(terminal, joystickHeight + Self.headRow),
                      "the terminal is \(terminal.frame.height) pt on Steer, not \(joystickHeight) + the head's row")
        app.buttons["Show keyboard"].firstMatch.tap()
        dismissKeyboardTip(in: app)
        XCTAssertTrue(app.buttons["Hide keyboard"].firstMatch.waitForExistence(timeout: 10), "no keyboard on Steer. " + app.debugDescription)
        terminal.typeText("ok\n")
        expect(in: app, step: "spark-docs's reply to ok", "label CONTAINS %@", sparkReply)
        Thread.sleep(forTimeInterval: 1.5)
        XCTAssertTrue(app.buttons["Hide keyboard"].firstMatch.exists, "the keyboard folded after Enter on Steer")
        app.buttons["Hide keyboard"].firstMatch.tap()
        XCTAssertTrue(app.buttons["Show keyboard"].firstMatch.waitForExistence(timeout: 10), "the keyboard stayed up. " + app.debugDescription)
        let row = layoutsRow(in: app)
        // Along Steer's keys, under the handle row, then still before the lift.
        let keys = row.coordinate(withNormalizedOffset: CGVector(dx: 0, dy: 0.5)).withOffset(CGVector(dx: 60, dy: 40))
        keys.press(forDuration: 0.05, thenDragTo: keys.withOffset(CGVector(dx: 150, dy: 0)), withVelocity: .slow, thenHoldForDuration: 0.2)
        XCTAssertFalse(head.exists, "a drag on Steer's belt brought a head")
        XCTAssertTrue(row.exists, "a drag on Steer's belt hid the handle row")
        XCTAssertTrue(heightBecomes(terminal, joystickHeight + Self.headRow), "a drag on Steer's belt moved the terminal")

        pick("Mine", in: app)
        let small = app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", "Match head")).firstMatch
        XCTAssertTrue(small.waitForExistence(timeout: 5), "no small head key on Mine. " + app.debugDescription)
        XCTAssertFalse(head.exists, "the dock's head shows on Mine")

        pick("Joystick", in: app)
        XCTAssertTrue(becomesHittable(theHead(in: app), within: 5), "the head did not come back on Joystick")
        XCTAssertTrue(heightBecomes(terminal, joystickHeight), "the terminal did not give the head its row back")
    }

    /// Surface: an agent's tools. In flint-auth's pane one push `↓` moves the agent's own `❯` to
    /// option 2, as its arrow key does, and a tap on the middle is Enter: the agent takes option 2.
    @MainActor
    func testPushDownMovesTheAgent() {
        let app = launched()
        openTheStandIn(in: app)
        let terminal = theTerminal(in: app)
        XCTAssertTrue(terminal.label.contains("❯ 1. Rotate it"), "the agent's mark is not on option 1: \(terminal.label)")
        let head = theHead(in: app)
        push(head, dy: 24)
        expect(in: app, step: "the agent's mark on option 2", "label CONTAINS %@", "❯ 2. Rotate, keep")
        XCTAssertFalse(terminal.label.contains(replyText), "a push answered the agent")
        waitUntilSteady(terminal)
        head.tap()
        expect(in: app, step: "flint-auth's reply to option 2", "label CONTAINS %@", replyText)
    }

    /// Surface: an agent's tools. A push `↓` held repeats: the agent's `❯` goes on down to the
    /// last option. The lift after the pushes is no Enter.
    @MainActor
    func testPushHeldRepeats() {
        let app = launched()
        openTheStandIn(in: app)
        push(theHead(in: app), dy: 24, hold: 1.2)
        expect(in: app, step: "the agent's mark on the last option", "label CONTAINS %@", "❯ 5. Chat about this")
        Thread.sleep(forTimeInterval: 2)
        let terminal = theTerminal(in: app)
        XCTAssertFalse(terminal.label.contains(replyText), "the lift after a held push answered the agent")
        XCTAssertTrue(terminal.label.contains("type 2 to answer"), "the question went: \(terminal.label)")
    }

    /// Surface: an agent's tools. At the first option a push `↑` sends no key: it reads up the
    /// output, a line, and the handle row says how to come back. A tap then comes back down and
    /// sends nothing: the question stays unanswered.
    @MainActor
    func testPushUpPastFirstReadsUp() {
        let app = launched()
        openTheStandIn(in: app)
        let head = theHead(in: app)
        push(head, dy: -24)
        expectLine(in: app, "1 line up · push ↓ or tap to come back")
        head.tap()
        expectLine(in: app, "roll: the output")
        Thread.sleep(forTimeInterval: 3)
        let terminal = theTerminal(in: app)
        XCTAssertFalse(terminal.label.contains(replyText), "the tap up in the output answered the agent")
        XCTAssertTrue(terminal.label.contains("type 2 to answer"), "the question went: \(terminal.label)")
        XCTAssertTrue(terminal.label.contains("❯ 1. Rotate it"), "the agent's mark moved: \(terminal.label)")
    }

    /// Surface: an agent's tools. In flint-auth's chat `↑` `↓` choose a session, so a push at its
    /// question sends no arrow: the chat stays on flint-auth, and nothing is answered.
    @MainActor
    func testChatPushSendsNoArrow() {
        let app = launched()
        openTheQuestion(in: app)
        push(theHead(in: app), dy: 24)
        Thread.sleep(forTimeInterval: 2)
        let terminal = theTerminal(in: app)
        XCTAssertTrue(terminal.label.contains("send to flint-auth"), "a push in the chat chose another session: \(terminal.label)")
        XCTAssertTrue(terminal.label.contains("2. Rotate, keep"), "the question went: \(terminal.label)")
        XCTAssertFalse(terminal.label.contains(replyText), "a push answered the agent")
    }

    /// Surface: an agent's tools. In flint-auth's chat one push `↓` moves Pocket's highlight to
    /// option 2, and the handle row names it; a tap types its number, and the chat answers with
    /// option 2.
    @MainActor
    func testChatPushHighlightsThenTapAnswers() {
        let app = launched()
        openTheQuestion(in: app)
        let head = theHead(in: app)
        push(head, dy: 24)
        expectLine(in: app, "2 of 5 · Rotate, keep")
        head.tap()
        expect(in: app, step: "flint-auth's reply to option 2", "label CONTAINS %@", replyText)
    }

    /// A flick on the edge coasts on after the lift: from well up hearth-api's output, a flick of
    /// a few notches down comes all the way to the bottom, and stops there with the bump line.
    @MainActor
    func testFlickCoasts() {
        let app = launched()
        openTheChat(app, row: "hearth-api")
        waitUntilSteady(theTerminal(in: app))
        let head = theHead(in: app)
        rollUp(head)
        expectLine(in: app, "lines up · push ↓ or tap to come back", anywhere: true)
        flickDown(head)
        expectLine(in: app, "bottom of the output")
    }

    /// A fast roll up hearth-api's output shows a card that names the step at the top of the
    /// view; it goes 0.6 s after the roll ends. Its pane draws `⏺ Read` over `✦ Running`, so
    /// 13 lines up the first step from the top of the view is `⏺ Read`.
    @MainActor
    func testStepCardShows() {
        let app = launched()
        openTheChat(app, row: "hearth-api")
        expect(in: app, step: "the step in hearth-api's chat", "label CONTAINS %@", "✦ Running go test")
        let head = theHead(in: app)
        // Anticlockwise, close left of the middle: 11 notches, the 11th at 3 lines a notch. The
        // finger rests before the lift, so no coast reads on past the step.
        at(head, -8, -46).press(forDuration: 0.5, thenDragTo: at(head, -8, 190), withVelocity: .fast, thenHoldForDuration: 0.15)
        XCTAssertTrue((head.value as? String ?? "").contains("card=⏺ Read internal/limit/store.go"),
                      "no card named the step: \(head.value ?? "")")
        let card = app.staticTexts["step-card"]
        XCTAssertTrue(card.waitForNonExistence(timeout: 3), "the card stayed after the roll")
    }

    /// A touch shows the four marks round the head, `↑ ❯ up`, `← cursor`, `→ cursor` and
    /// `↓ ❯ down`; they are gone 0.9 s after the lift. A 0.45 s press sends nothing.
    @MainActor
    func testTouchShowsMarks() {
        let app = launched()
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        let head = theHead(in: app)
        head.press(forDuration: 0.45)
        for word in ["❯ up", "cursor", "❯ down"] {
            XCTAssertTrue(app.staticTexts[word].firstMatch.exists, "no \(word) mark on touch. " + app.debugDescription)
        }
        Thread.sleep(forTimeInterval: 1.2)
        for word in ["❯ up", "cursor", "❯ down"] {
            XCTAssertFalse(app.staticTexts[word].firstMatch.exists, "the \(word) mark stayed after the lift")
        }
    }

    /// The marks fade in and out, never cut: just after the touch-down their opacity was between 0
    /// and 1 (XCUITest reads no alpha, so the head's DEBUG value keeps it), and half a second
    /// after the lift they still show, as they start to go 0.9 s after it.
    @MainActor
    func testMarksFadeNotCut() {
        let app = launched()
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        let head = theHead(in: app)
        head.press(forDuration: 0.45)
        Thread.sleep(forTimeInterval: 0.5)
        XCTAssertTrue(app.staticTexts["❯ up"].firstMatch.exists, "the marks were gone 0.5 s after the lift")
        let value = head.value as? String ?? ""
        let line = value.components(separatedBy: "\n").first { $0.hasPrefix("marks=") }
        guard let seen = line.flatMap({ Double($0.dropFirst("marks=".count)) }) else {
            return XCTFail("the head kept no opacity of its marks: \(value)")
        }
        XCTAssertGreaterThan(seen, 0, "the marks were not there just after the touch-down")
        XCTAssertLessThan(seen, 1, "the marks cut in, with no fade")
    }

    /// Joystick's belt carries `⌨`: a tap on it raises the keyboard. The head's bottom mark is `↓`.
    @MainActor
    func testKeyboardKeyRaisesKeyboard() {
        let app = launched()
        openTheChat(app, row: "spark-docs")
        raiseTheKeyboard(app)
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

    /// On Joystick, Return on the system keyboard sends the line, then the keyboard folds and the
    /// head is back. Under the keyboard the head is out of reach, so this is the Enter that folds.
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

    // MARK: VoiceOver

    /// VoiceOver names the head: a swipe up or down pushes `↑` or `↓`, a double-tap is the tap,
    /// and its marks and the hold are its actions, named from the marks home set. A UI test cannot
    /// swipe as VoiceOver does: the DEBUG `actions=` line says what each gesture does, from
    /// `HeadVoice`, whose tests hold the mapping.
    @MainActor
    func testHeadAccessibility() {
        let app = launched(extra: ["-resetHead"])
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        let head = theHead(in: app)
        XCTAssertEqual(head.label, "Match head")
        let gestures = ["swipe up: ↑", "swipe down: ↓", "double-tap: tap"]
        XCTAssertEqual(voiceActions(on: head), gestures + ["❯ up, top mark", "cursor, left mark", "cursor, right mark", "❯ down, bottom mark", "sessions"])

        openHome(app)
        app.buttons["home-head-left"].tap()
        app.buttons["Esc cancel"].tap()
        backToTheConsole(app)
        XCTAssertEqual(voiceActions(on: theHead(in: app)), gestures + ["❯ up, top mark", "cancel, left mark", "cursor, right mark", "❯ down, bottom mark", "sessions"])
    }

    // MARK: Helpers

    /// The names of the head's VoiceOver actions, from its DEBUG value.
    @MainActor
    private func voiceActions(on head: XCUIElement) -> [String] {
        let value = head.value as? String ?? ""
        guard let line = value.components(separatedBy: "\n").first(where: { $0.hasPrefix("actions=") }) else {
            XCTFail("the head names no actions: \(value)")
            return []
        }
        return line.dropFirst("actions=".count).components(separatedBy: "|")
    }

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

    /// The word of the head's left mark, read while a 0.45 s press shows the marks. They go 0.9 s
    /// after the lift, and a wait polls first a second on: one look, as `testTouchShowsMarks` does.
    @MainActor
    private func leftMark(in app: XCUIApplication) -> String {
        theHead(in: app).press(forDuration: 0.45)
        let mark = app.staticTexts["head-mark-left"]
        XCTAssertTrue(mark.exists, "no left mark on touch. " + app.debugDescription)
        return mark.label
    }

    /// Joystick's belt key `⌨` raises the keyboard.
    @MainActor
    private func raiseTheKeyboard(_ app: XCUIApplication) {
        let key = app.buttons["Keyboard"].firstMatch
        XCTAssertTrue(key.waitForExistence(timeout: 10), "no ⌨ on Joystick's belt. " + app.debugDescription)
        key.tap()
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

    /// A push from the middle, `dy` points down (negative: up), held `hold` seconds before the lift.
    @MainActor
    private func push(_ head: XCUIElement, dy: CGFloat, hold: TimeInterval = 0.05) {
        at(head, 0, 0).press(forDuration: 0.05, thenDragTo: at(head, 0, dy), withVelocity: .default, thenHoldForDuration: hold)
    }

    /// Anticlockwise, close left of the middle, about 11 notches up the output; the finger rests
    /// before the lift, so it does not coast.
    @MainActor
    private func rollUp(_ head: XCUIElement) {
        at(head, -8, -46).press(forDuration: 0.05, thenDragTo: at(head, -8, 190), withVelocity: .slow, thenHoldForDuration: 0.3)
    }

    /// Clockwise down the right edge, about 4 notches, lifted while it moves: the head coasts on.
    @MainActor
    private func flickDown(_ head: XCUIElement) {
        at(head, 40, -20).press(forDuration: 0.05, thenDragTo: at(head, 40, 30), withVelocity: .fast, thenHoldForDuration: 0)
    }

    /// flint-auth's own pane, the demo's stand-in for a jumped tmux pane, at its question: a long
    /// press on its row in the list, then `go in`. The keys go to the agent, as in a pane.
    @MainActor
    private func openTheStandIn(in app: XCUIApplication) {
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        let terminal = theTerminal(in: app)
        waitUntilSteady(terminal)
        tapRow(in: app, containing: "flint-auth", press: 0.8)
        expect(in: app, step: "the menu of flint-auth after the long-press", "label CONTAINS %@", "go in")
        waitUntilSteady(terminal)
        tapRow(in: app, containing: "go in")
        let shown = XCTNSPredicateExpectation(predicate: NSPredicate(format: "label CONTAINS %@", "type 2 to answer"), object: terminal)
        if XCTWaiter().wait(for: [shown], timeout: 6) != .completed, terminal.label.contains("go in") {
            // The menu rises from the foot: a tap that landed on its box's line misses go in.
            waitUntilSteady(terminal)
            tapRow(in: app, containing: "go in")
        }
        expect(in: app, step: "flint-auth's pane at its question", "label CONTAINS %@", "type 2 to answer")
        waitUntilSteady(terminal)
    }

    /// A tap, or a press of `press` seconds, on the terminal's row that shows `text`.
    @MainActor
    private func tapRow(in app: XCUIApplication, containing text: String, press: TimeInterval = 0) {
        let terminal = theTerminal(in: app)
        guard let row = terminal.label.components(separatedBy: "\n").firstIndex(where: { $0.contains(text) }) else {
            return XCTFail("no row shows \(text): \(terminal.label)")
        }
        let spot = terminal.coordinate(withNormalizedOffset: CGVector(dx: 0.3, dy: terminal.middle(ofRow: row)))
        if press > 0 { spot.press(forDuration: press) } else { spot.tap() }
    }

    /// flint-auth's chat, where its question shows.
    @MainActor
    private func openTheQuestion(in app: XCUIApplication) {
        expect(in: app, "label CONTAINS %@", "NEEDS YOU")
        openTheChat(app, row: "flint-auth")
        expect(in: app, step: "the options in flint-auth's chat", "label CONTAINS %@", "2. Rotate, keep")
        waitUntilSteady(theTerminal(in: app))
    }

    /// The dock's handle row reads a line that starts with `text`, or holds it `anywhere`.
    @MainActor
    private func expectLine(in app: XCUIApplication, _ text: String, anywhere: Bool = false) {
        let row = layoutsRow(in: app)
        let reads = { () -> Bool in
            guard let line = row.value as? String else { return false }
            return anywhere ? line.contains(text) : line.hasPrefix(text)
        }
        let deadline = Date().addingTimeInterval(10)
        while !reads(), Date() < deadline {
            Thread.sleep(forTimeInterval: 0.2)
        }
        XCTAssertTrue(reads(), "the handle row reads \(row.value ?? "nothing"), not \(text). " + app.debugDescription)
    }

    /// The dock's handle row: the layout's name, or the head's line.
    @MainActor
    private func layoutsRow(in app: XCUIApplication) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label BEGINSWITH %@", "Layouts")).firstMatch
    }

    /// The terminal comes to `height` within 5 s: it lays out again after a layout change.
    @MainActor
    private func heightBecomes(_ terminal: XCUIElement, _ height: CGFloat) -> Bool {
        let deadline = Date().addingTimeInterval(5)
        while abs(terminal.frame.height - height) > 1, Date() < deadline {
            Thread.sleep(forTimeInterval: 0.2)
        }
        return abs(terminal.frame.height - height) <= 1
    }

    /// The drawer opens on a tap near the grip, and the layout's match in it is picked.
    @MainActor
    private func pick(_ layout: String, in app: XCUIApplication) {
        let layouts = layoutsRow(in: app)
        XCTAssertTrue(layouts.waitForExistence(timeout: 30), "no Layouts row. " + app.debugDescription)
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
        terminal.coordinate(withNormalizedOffset: CGVector(dx: 0.3, dy: terminal.middle(ofRow: row))).tap()
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

    /// The demo console, on Joystick: the head rides on that layout only.
    @MainActor
    private func launched(extra: [String] = [], joystick: Bool = true) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-openDemoHost", "-demoSlowInvite"] + extra
        app.launch()
        if joystick { pick("Joystick", in: app) }
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
