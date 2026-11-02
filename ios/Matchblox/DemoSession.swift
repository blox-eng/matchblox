import Foundation

struct DemoStartFailed: Error {
    let message: String
}

/// The demo as the console's source: the Go library in this process, in place of an SSH session.
final class DemoSession: ConsoleSource, @unchecked Sendable {
    /// Each start and stop waits here for its turn. The library refuses a start while a run is
    /// live, and a read or a write left over from one run would reach the next one.
    private static let turns = DispatchQueue(label: "com.blox.matchblox.demo.turns", qos: .userInitiated)
    private static let readSize = 64 * 1024

    /// `MBDemoWrite` blocks until the console reads, so writes keep off the cooperative threads.
    private let writes = DispatchQueue(label: "com.blox.matchblox.demo.writes", qos: .userInitiated)
    private let lock = NSLock()
    private var closed = false
    private var deathReason: String?
    private var readerEnded: DispatchSemaphore?

    /// The person has finished what the demo asks of them.
    var done: Bool {
        lock.withLock { !closed } && MBDemoDone() != 0
    }

    /// Why the run died, once `output` has ended. Nil after a clean quit or a `close`.
    var failure: String? {
        lock.withLock { deathReason }
    }

    /// Starts after any earlier run has fully ended. Returns the console's output.
    func start(cols: Int, rows: Int) async throws(DemoStartFailed) -> AsyncStream<Data> {
        let started = await withCheckedContinuation { (done: CheckedContinuation<Result<AsyncStream<Data>, DemoStartFailed>, Never>) in
            Self.turns.async { done.resume(returning: self.startInTurn(cols: cols, rows: rows)) }
        }
        switch started {
        case .success(let output): return output
        case .failure(let failure): throw failure
        }
    }

    func write(_ data: Data) async {
        await onWrites {
            data.withUnsafeBytes { raw in
                guard let bytes = raw.bindMemory(to: CChar.self).baseAddress else { return }
                MBDemoWrite(UnsafeMutablePointer(mutating: bytes), Int32(raw.count))
            }
        }
    }

    func resize(cols: Int, rows: Int) async {
        await onWrites { MBDemoResize(Int32(cols), Int32(rows)) }
    }

    /// Returns when the run has ended and no thread of this session can touch the library again.
    func close() async {
        lock.withLock { closed = true }
        await withCheckedContinuation { (done: CheckedContinuation<Void, Never>) in
            Self.turns.async {
                self.stopInTurn()
                done.resume()
            }
        }
    }

    private func onWrites(_ call: @escaping @Sendable () -> Void) async {
        await withCheckedContinuation { (done: CheckedContinuation<Void, Never>) in
            writes.async {
                if !self.lock.withLock({ self.closed }) { call() }
                done.resume()
            }
        }
    }

    private func startInTurn(cols: Int, rows: Int) -> Result<AsyncStream<Data>, DemoStartFailed> {
        guard !lock.withLock({ closed }) else {
            return .failure(DemoStartFailed(message: "The demo closed before it started."))
        }
        guard let tree = Bundle.main.path(forResource: "machine", ofType: "tar") else {
            return .failure(DemoStartFailed(message: "This build of the app has no demo machine."))
        }
        let code = tree.withCString { MBDemoStart(UnsafeMutablePointer(mutating: $0), Int32(cols), Int32(rows)) }
        guard code == 0 else {
            return .failure(DemoStartFailed(message: Self.lastError() ?? "The demo did not start."))
        }
        let (output, sink) = AsyncStream<Data>.makeStream()
        let ended = DispatchSemaphore(value: 0)
        lock.withLock { readerEnded = ended }
        let reader = Thread {
            self.read(into: sink)
            ended.signal()
        }
        reader.name = "com.blox.matchblox.demo.read"
        reader.qualityOfService = .userInitiated
        reader.start()
        return .success(output)
    }

    /// `MBDemoStop` ends a blocked `MBDemoRead` with -1, so the reader always comes back. A write
    /// in flight comes back too, and the ones behind it see `closed`.
    private func stopInTurn() {
        guard let ended = lock.withLock({ readerEnded.take() }) else { return }
        MBDemoStop()
        ended.wait()
        writes.sync {}
    }

    /// Runs on its own thread: `MBDemoRead` blocks, and the buffer is valid for the call only.
    private func read(into sink: AsyncStream<Data>.Continuation) {
        let buffer = UnsafeMutablePointer<CChar>.allocate(capacity: Self.readSize)
        defer { buffer.deallocate() }
        while true {
            let got = MBDemoRead(buffer, Int32(Self.readSize))
            guard got >= 0 else { break }
            if got > 0 {
                sink.yield(Data(bytes: buffer, count: Int(got)))
            }
        }
        // The library sets the reason before the reads end, and it means nothing after a stop.
        lock.withLock {
            if !closed { deathReason = Self.lastError() }
        }
        sink.finish()
    }

    private static func lastError() -> String? {
        var text = [CChar](repeating: 0, count: 1024)
        let length = MBDemoError(&text, Int32(text.count))
        return length > 0 ? String(cString: text) : nil
    }
}
