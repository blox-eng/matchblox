import Foundation

/// What a matchblox command printed, and whether it did what it asks.
public struct HostReply: Equatable, Sendable {
    public let output: Data
    public let ok: Bool
    /// The command's exit status, the demo's as a host's; nil when it never ran.
    public let status: Int?

    public init(output: Data, ok: Bool, status: Int? = nil) {
        self.output = output; self.ok = ok; self.status = status
    }
}

/// One way to run a matchblox command where Pocket is: over SSH on a host, in the process in the demo.
public struct HostCommand: Sendable {
    private let run: @Sendable (String) async -> HostReply

    /// More than this from one command is not an answer Pocket reads.
    static let maxOutput = 256 * 1024

    private init(run: @escaping @Sendable (String) async -> HostReply) {
        self.run = run
    }

    /// Each command opens a session of its own, as an `SSHSession` opens once.
    public static func host(_ session: @escaping @Sendable () -> SSHSession) -> HostCommand {
        HostCommand { line in
            let ssh = session()
            guard let stream = try? await ssh.open(command: "matchblox " + line, pty: nil) else {
                await ssh.close()
                return HostReply(output: Data(), ok: false)
            }
            var output = Data()
            for await chunk in stream {
                output.append(chunk)
                if output.count > HostCommand.maxOutput { break }
            }
            let status = await ssh.exitStatus
            await ssh.close()
            return HostReply(output: output, ok: status == 0 && output.count <= HostCommand.maxOutput, status: status)
        }
    }

    /// The demo answers the line in the process: the app passes `MBDemoRun` in.
    public static func demo(_ run: @escaping @Sendable (String) async -> HostReply) -> HostCommand {
        HostCommand(run: run)
    }

    /// Runs `matchblox` with these words.
    public func callAsFunction(_ words: [String]) async -> HostReply {
        await run(Self.line(words))
    }

    /// The words as a shell reads them: a word of plain characters as it is, any other in single quotes.
    static func line(_ words: [String]) -> String {
        words.map { quoted($0) }.joined(separator: " ")
    }

    private static func quoted(_ word: String) -> String {
        let plain = !word.isEmpty && word.unicodeScalars.allSatisfy {
            $0.isASCII && (CharacterSet.alphanumerics.contains($0) || "%-_.,:/=@+".unicodeScalars.contains($0))
        }
        return plain ? word : "'" + word.replacingOccurrences(of: "'", with: "'\\''") + "'"
    }
}
