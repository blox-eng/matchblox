import XCTest
@testable import MatchbloxKit

/// The head is a thumbstick: a push sends its mark's key once, and only a still tap is Enter.
/// Surface: an agent's tools. A second key moves the agent's `❯` past the option the owner meant.
final class HeadStickTests: XCTestCase {
    func testOnePushOneKey() {
        var stick = HeadStick()
        stick.began(at: 0)
        XCTAssertNil(stick.moved(to: CGPoint(x: 0, y: -8), at: 0.02), "under 16 pt is no push")
        XCTAssertNil(stick.moved(to: CGPoint(x: 0, y: -15.9), at: 0.04), "under 16 pt is no push")
        XCTAssertEqual(stick.moved(to: CGPoint(x: 0, y: -17), at: 0.05), .key(.top, word: false))

        // Until 0.42 s after the push the thumb wobbles between 16 and 11 pt, swings right of
        // the axis it pushed and out past the rim: nothing more goes, by move or by tick.
        let wobble = [
            CGPoint(x: 0, y: -16), CGPoint(x: 0, y: -11), CGPoint(x: 3, y: -12), CGPoint(x: 0, y: -40),
            CGPoint(x: 14, y: -9), CGPoint(x: 18, y: -2), CGPoint(x: 0, y: -11.5), CGPoint(x: -16, y: -6),
        ]
        var sent: [HeadStick.Send] = []
        for i in 0..<41 {
            let time = 0.05 + Double(i) * 0.01
            if let send = stick.moved(to: wobble[i % wobble.count], at: time) { sent.append(send) }
            if let send = stick.tick(at: time) { sent.append(send) }
        }
        XCTAssertEqual(sent, [], "a push that never came back under 10 pt sent a second key")
        XCTAssertEqual(stick.lifted(at: 0.47), .nothing, "a lift after a push is no Enter")
        XCTAssertNil(stick.tick(at: 0.6), "a lifted thumb repeats nothing")
    }

    func testRearmUnderTen() {
        var stick = HeadStick()
        stick.began(at: 0)
        XCTAssertEqual(stick.moved(to: CGPoint(x: 0, y: -20), at: 0.05), .key(.top, word: false))
        XCTAssertNil(stick.moved(to: CGPoint(x: 0, y: -10), at: 0.1), "10 pt is not under 10")
        XCTAssertNil(stick.moved(to: CGPoint(x: 17, y: 0), at: 0.15), "no rearm: the push the other way sends nothing")
        XCTAssertNil(stick.moved(to: CGPoint(x: 0, y: -9), at: 0.2))
        XCTAssertNil(stick.tick(at: 0.9), "back under 10 pt, the push stops repeating")
        XCTAssertEqual(stick.moved(to: CGPoint(x: 17, y: 0), at: 1.0), .key(.right, word: false))
        XCTAssertNil(stick.moved(to: CGPoint(x: 6, y: 2), at: 1.05))
        XCTAssertEqual(stick.moved(to: CGPoint(x: 1, y: 18), at: 1.1), .key(.bottom, word: false))
        XCTAssertNil(stick.moved(to: CGPoint(x: 0, y: 0), at: 1.15))
        XCTAssertEqual(stick.moved(to: CGPoint(x: -16, y: 3), at: 1.2), .key(.left, word: false))
    }

    func testRepeatAndWords() {
        var stick = HeadStick()
        stick.began(at: 0)
        XCTAssertEqual(stick.moved(to: CGPoint(x: -20, y: 0), at: 0), .key(.left, word: false))
        XCTAssertNil(stick.tick(at: 0.415), "no repeat before 0.42 s")
        var repeats: [HeadStick.Send?] = []
        for k in 0..<12 {
            let due = 0.42 + Double(k) * 0.11
            repeats.append(stick.tick(at: due + 0.005))
            XCTAssertNil(stick.tick(at: due + 0.05), "a repeat came before its 0.11 s")
        }
        let chars = Array(repeating: Optional(HeadStick.Send.key(.left, word: false)), count: 9)
        let words = Array(repeating: Optional(HeadStick.Send.key(.left, word: true)), count: 3)
        XCTAssertEqual(repeats, chars + words, "9 repeats move a character, then a word")

        let marks = HeadMarks()
        XCTAssertEqual(HeadMarks.wordBytes(for: marks[.left]), [0x1b, UInt8(ascii: "b")], "Esc b")
        XCTAssertEqual(HeadMarks.wordBytes(for: marks[.right]), [0x1b, UInt8(ascii: "f")], "Esc f")
        XCTAssertNil(HeadMarks.wordBytes(for: marks[.top]), "↑ has no word move: it repeats the arrow")

        var right = HeadStick()
        right.began(at: 5)
        XCTAssertEqual(right.moved(to: CGPoint(x: 30, y: 4), at: 5), .key(.right, word: false))
        let sent = (0..<10).compactMap { right.tick(at: 5.425 + Double($0) * 0.11) }
        XCTAssertEqual(sent.last, .key(.right, word: true))
        XCTAssertEqual(sent.count, 10)
    }

    func testTravelClamps() {
        var stick = HeadStick()
        XCTAssertEqual([stick.offset.x, stick.offset.y], [0, 0])
        stick.began(at: 0)
        _ = stick.moved(to: CGPoint(x: 3, y: 4), at: 0.01)
        XCTAssertEqual([stick.offset.x, stick.offset.y], [3, 4], "under 12 pt the head goes with the thumb")
        _ = stick.moved(to: CGPoint(x: 0, y: -40), at: 0.02)
        XCTAssertEqual(stick.offset.x, 0, accuracy: 0.001)
        XCTAssertEqual(stick.offset.y, -12, accuracy: 0.001)
        _ = stick.moved(to: CGPoint(x: 30, y: 40), at: 0.03)
        XCTAssertEqual(stick.offset.x, 7.2, accuracy: 0.001, "12 pt along the push")
        XCTAssertEqual(stick.offset.y, 9.6, accuracy: 0.001, "12 pt along the push")
        XCTAssertLessThanOrEqual(hypot(stick.offset.x, stick.offset.y), 12.001)
        _ = stick.lifted(at: 0.1)
        XCTAssertEqual([stick.offset.x, stick.offset.y], [0, 0], "the head springs back on the lift")
    }

    /// The dock reads the hold from `held` alone, for the pushes and the lift: a push before
    /// 0.6 s leaves it false for good, and a still thumb sets it at 0.6 s on the stick's clock.
    func testHeldOnlyFromTheStick() {
        var stick = HeadStick()
        stick.began(at: 0)
        XCTAssertEqual(stick.moved(to: CGPoint(x: 0, y: 20), at: 0.3), .key(.bottom, word: false))
        _ = stick.moved(to: CGPoint(x: 0, y: 0), at: 0.4)
        XCTAssertNil(stick.tick(at: 1.0))
        XCTAssertFalse(stick.held, "a push before 0.6 s cancels the hold")
        XCTAssertEqual(stick.lifted(at: 1.1), .nothing, "the lift after a push before 0.6 s does nothing")

        stick.began(at: 0)
        XCTAssertNil(stick.tick(at: 0.59))
        XCTAssertFalse(stick.held)
        XCTAssertNil(stick.tick(at: 0.6))
        XCTAssertTrue(stick.held, "still for 0.6 s: the ring filled")
        XCTAssertEqual(stick.lifted(at: 0.7), .hold)
    }

    func testTapIsEnterOnlyStill() {
        func lift(_ moves: [(CGFloat, CGFloat, TimeInterval)], at end: TimeInterval) -> HeadStick.Lift {
            var stick = HeadStick()
            stick.began(at: 0)
            for (x, y, time) in moves { _ = stick.moved(to: CGPoint(x: x, y: y), at: time) }
            return stick.lifted(at: end)
        }
        XCTAssertEqual(lift([], at: 0.1), .enter)
        XCTAssertEqual(lift([(3, 4, 0.05), (-6, 7, 0.1)], at: 0.3), .enter, "under 10 pt is still")
        XCTAssertEqual(lift([], at: 0.36), .nothing, "0.35 s or more is no tap")
        XCTAssertEqual(lift([(0, -10, 0.05), (0, 0, 0.1)], at: 0.2), .nothing, "10 pt is a push, even back in the middle")
        XCTAssertEqual(lift([(0, 17, 0.05), (0, 0, 0.1)], at: 0.2), .nothing, "a push before the lift makes the lift nothing")
        XCTAssertEqual(lift([(-16.5, 0, 0.05)], at: 0.1), .nothing)

        XCTAssertEqual(lift([], at: 0.5), .nothing, "between 0.35 s and 0.6 s the lift sends nothing")
        XCTAssertEqual(lift([(2, 2, 0.3)], at: 0.61), .hold, "still for 0.6 s fills the ring")
        XCTAssertEqual(lift([(0, -11, 0.3), (0, 0, 0.4)], at: 0.8), .nothing, "a push before 0.6 s cancels the hold")
        XCTAssertEqual(lift([(0, -11, 0.7)], at: 0.8), .hold, "the ring filled at 0.6 s, before the move")

        var stick = HeadStick()
        stick.began(at: 0)
        XCTAssertNil(stick.tick(at: 0.65))
        XCTAssertEqual(stick.moved(to: CGPoint(x: 0, y: 20), at: 0.7), .key(.bottom, word: false), "a push steps the sessions")
        XCTAssertEqual(stick.lifted(at: 0.9), .hold, "lift to go there")
    }
}
