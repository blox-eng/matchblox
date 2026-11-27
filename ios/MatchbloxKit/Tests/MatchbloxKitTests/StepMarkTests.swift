import XCTest
@testable import MatchbloxKit

/// The step card names the agent step nearest the top of the view.
final class StepMarkTests: XCTestCase {
    func testNearestStep() {
        let lines = ["  output of the last step", "", "  ✦ Reading a.go", "    package a", "", "⏺ Bash(go test)"]
        XCTAssertEqual(StepMark.nearest(in: lines), "✦ Reading a.go")
        XCTAssertNil(StepMark.nearest(in: ["no step here", "a line with ✦ inside"]))
        XCTAssertNil(StepMark.nearest(in: []))
    }
}
