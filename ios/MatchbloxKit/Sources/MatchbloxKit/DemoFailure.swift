import Foundation

/// What the person reads when the demo does not start or stops. The library's own text goes to
/// the log only: it carries paths and Go error wording.
public enum DemoFailure {
    public static func sentence(for raw: String?) -> String {
        let text = raw ?? ""
        if text.contains("no space left") {
            return "The phone is out of space. Free some space, then tap Try again."
        }
        if text.contains("already running") {
            return "The demo is still closing. Tap Try again in a moment."
        }
        return "The demo could not run. Tap Try again."
    }
}
