import XCTest
@testable import MatchbloxKit

final class CopyGateTests: XCTestCase {
    private let start = Date(timeIntervalSince1970: 1_000_000)
    private let reply = Data("the last reply".utf8)

    func testACopyNobodyAskedForIsDropped() {
        var gate = CopyGate()
        XCTAssertEqual(gate.take(reply, at: start), .unasked)
    }

    func testACopySoonAfterTheAskPasses() {
        var gate = CopyGate()
        gate.asked(at: start)
        XCTAssertEqual(gate.take(reply, at: start.addingTimeInterval(2.9)), .copy("the last reply"))
    }

    func testOneAskLetsOneCopyThrough() {
        var gate = CopyGate()
        gate.asked(at: start)
        XCTAssertEqual(gate.take(reply, at: start.addingTimeInterval(1)), .copy("the last reply"))
        XCTAssertEqual(gate.take(reply, at: start.addingTimeInterval(1.5)), .unasked)
    }

    func testALateCopyIsDroppedAndDisarms() {
        var gate = CopyGate()
        gate.asked(at: start)
        XCTAssertEqual(gate.take(reply, at: start.addingTimeInterval(3.1)), .unasked)
        XCTAssertEqual(gate.take(reply, at: start.addingTimeInterval(3.2)), .unasked)
    }

    func testACopyFromBeforeTheAskIsDropped() {
        var gate = CopyGate()
        gate.asked(at: start)
        XCTAssertEqual(gate.take(reply, at: start.addingTimeInterval(-1)), .unasked)
    }

    func testANewAskStartsTheWindowAgain() {
        var gate = CopyGate()
        gate.asked(at: start)
        gate.asked(at: start.addingTimeInterval(2))
        XCTAssertEqual(gate.take(reply, at: start.addingTimeInterval(4.5)), .copy("the last reply"))
    }

    func testABigCopyIsRefusedAndDisarms() {
        var gate = CopyGate()
        gate.asked(at: start)
        XCTAssertEqual(gate.take(Data(count: CopyGate.maxBytes + 1), at: start), .tooLarge)
        XCTAssertEqual(gate.take(reply, at: start), .unasked)
        gate.asked(at: start)
        if case .copy = gate.take(Data(count: CopyGate.maxBytes), at: start) {} else {
            XCTFail("a copy of exactly the limit passes")
        }
    }
}
