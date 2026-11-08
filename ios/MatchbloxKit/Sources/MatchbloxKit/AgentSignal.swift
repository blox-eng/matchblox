/// Takes the console's agent signal, `ESC _ matchblox;agent=<id> ESC \`, out of the terminal
/// stream, also when a read ends inside it. Every other byte passes through unchanged and in
/// order. Bytes that may start a signal wait for the next read.
public struct AgentSignalFilter: Sendable {
    private static let prefix = Array("\u{1b}_matchblox;agent=".utf8)
    private static let esc: UInt8 = 0x1b
    private static let backslash = UInt8(ascii: "\\")
    /// Longer than any agent id; a longer run is not a signal.
    private static let maxID = 32

    private var held: [UInt8] = []

    public init() {}

    public mutating func feed(_ bytes: ArraySlice<UInt8>) -> (pass: [UInt8], agent: String?) {
        // Most reads hold no ESC at all: with nothing held, they pass whole.
        if held.isEmpty && !bytes.contains(Self.esc) {
            return (Array(bytes), nil)
        }
        var pass: [UInt8] = []
        pass.reserveCapacity(bytes.count + held.count)
        var agent: String?
        for byte in bytes {
            consume(byte, into: &pass, agent: &agent)
        }
        return (pass, agent)
    }

    private mutating func consume(_ byte: UInt8, into pass: inout [UInt8], agent: inout String?) {
        let prefix = Self.prefix
        if held.isEmpty {
            if byte == Self.esc { held = [byte] } else { pass.append(byte) }
            return
        }
        if held.count < prefix.count {
            if byte == prefix[held.count] { held.append(byte); return }
            return giveUp(byte, into: &pass, agent: &agent)
        }
        if held.last == Self.esc && held.count > prefix.count {
            guard byte == Self.backslash else { return giveUp(byte, into: &pass, agent: &agent) }
            agent = String(decoding: held[prefix.count..<(held.count - 1)], as: UTF8.self)
            held = []
            return
        }
        let id = held.count - prefix.count
        if byte == Self.esc || (id < Self.maxID && (0x21...0x7e).contains(byte)) {
            held.append(byte)
            return
        }
        giveUp(byte, into: &pass, agent: &agent)
    }

    /// The held bytes are no signal: the first passes, and the rest are read again, as one of
    /// them may start a signal.
    private mutating func giveUp(_ byte: UInt8, into pass: inout [UInt8], agent: inout String?) {
        let again = held.dropFirst() + [byte]
        pass.append(held[0])
        held = []
        for b in again {
            consume(b, into: &pass, agent: &agent)
        }
    }
}

/// A signal's id as the console writes it, `<agent>[;pane=<pane>][;marks]` (`keycmd.PaneSignal`,
/// `keycmd.MarksSignal`). A console that reads the app's marks announces it with no agent.
public struct ConsoleSignal: Equatable, Sendable {
    public let agent: String
    public let pane: String?
    public let marks: Bool

    public init(_ raw: String) {
        let parts = raw.split(separator: ";", omittingEmptySubsequences: false)
        var pane: String?
        var marks = false
        for part in parts.dropFirst() {
            if part == "marks" {
                marks = true
            } else if part.hasPrefix("pane="), part.count > "pane=".count {
                pane = String(part.dropFirst("pane=".count))
            }
        }
        self.init(agent: parts.first.map(String.init) ?? "", pane: pane, marks: marks)
    }

    init(agent: String, pane: String?, marks: Bool) {
        self.agent = agent
        self.pane = pane
        self.marks = marks
    }
}
