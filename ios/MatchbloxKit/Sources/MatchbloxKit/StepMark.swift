import Foundation

/// The agent step a view shows: the first `✦` or `⏺` line from the top of the visible lines.
public enum StepMark {
    private static let marks: Set<Character> = ["✦", "⏺"]

    public static func nearest(in lines: [String]) -> String? {
        lines.lazy
            .map { $0.trimmingCharacters(in: .whitespaces) }
            .first { line in line.first.map { marks.contains($0) } ?? false }
    }
}
