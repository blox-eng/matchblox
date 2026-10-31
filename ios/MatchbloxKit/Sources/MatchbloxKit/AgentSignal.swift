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
        var pass: [UInt8] = []
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
