/// Takes the console's two signals out of the terminal stream: the agent signal,
/// `ESC _ matchblox;agent=<id> ESC \`, and the board signal, `ESC _ matchblox;board=<keys> ESC \`
/// (`keycmd.BoardSignal`), also when a read ends inside one. Every other byte passes through
/// unchanged and in order. Bytes that may start a signal wait for the next read.
public struct AgentSignalFilter: Sendable {
    private static let agentPrefix = Array("\u{1b}_matchblox;agent=".utf8)
    private static let boardPrefix = Array("\u{1b}_matchblox;board=".utf8)
    /// Where the two prefixes part: `a` or `b`.
    private static let fork = agentPrefix.count - "agent=".utf8.count
    private static let esc: UInt8 = 0x1b
    private static let backslash = UInt8(ascii: "\\")
    /// Longer than any agent id; a longer run is not a signal.
    private static let maxID = 32
    /// The console's `beltMax`: a longer board value is not a signal.
    private static let maxBoard = 48

    private struct Heard {
        var agent: String?
        var board: String?
    }

    private var held: [UInt8] = []

    public init() {}

    /// `agent` and `board` are the last of each signal the read finished, or nil for none.
    public mutating func feed(_ bytes: ArraySlice<UInt8>) -> (pass: [UInt8], agent: String?, board: String?) {
        // Most reads hold no ESC at all: with nothing held, they pass whole.
        if held.isEmpty && !bytes.contains(Self.esc) {
            return (Array(bytes), nil, nil)
        }
        var pass: [UInt8] = []
        pass.reserveCapacity(bytes.count + held.count)
        var heard = Heard()
        for byte in bytes {
            consume(byte, into: &pass, heard: &heard)
        }
        return (pass, heard.agent, heard.board)
    }

    /// The board's once the held bytes have parted from the agent's.
    private var isBoard: Bool {
        held.count > Self.fork && held[Self.fork] == Self.boardPrefix[Self.fork]
    }

    private mutating func consume(_ byte: UInt8, into pass: inout [UInt8], heard: inout Heard) {
        if held.isEmpty {
            if byte == Self.esc { held = [byte] } else { pass.append(byte) }
            return
        }
        let board = isBoard || (held.count == Self.fork && byte == Self.boardPrefix[Self.fork])
        let prefix = board ? Self.boardPrefix : Self.agentPrefix
        if held.count < prefix.count {
            if byte == prefix[held.count] { held.append(byte); return }
            return giveUp(byte, into: &pass, heard: &heard)
        }
        if held.last == Self.esc && held.count > prefix.count {
            guard byte == Self.backslash else { return giveUp(byte, into: &pass, heard: &heard) }
            let value = String(decoding: held[prefix.count..<(held.count - 1)], as: UTF8.self)
            if board { heard.board = value } else { heard.agent = value }
            held = []
            return
        }
        let id = held.count - prefix.count
        let limit = board ? Self.maxBoard : Self.maxID
        if byte == Self.esc || (id < limit && (0x21...0x7e).contains(byte)) {
            held.append(byte)
            return
        }
        giveUp(byte, into: &pass, heard: &heard)
    }

    /// The held bytes are no signal: the first passes, and the rest are read again, as one of
    /// them may start a signal.
    private mutating func giveUp(_ byte: UInt8, into pass: inout [UInt8], heard: inout Heard) {
        let again = held.dropFirst() + [byte]
        pass.append(held[0])
        held = []
        for b in again {
            consume(b, into: &pass, heard: &heard)
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
