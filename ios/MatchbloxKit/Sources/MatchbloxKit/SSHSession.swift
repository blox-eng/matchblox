import Foundation
import NIOCore
import NIOSSH
import NIOTransportServices

public actor SSHSession {
    private let host: String
    private let port: Int
    private let user: String
    private let key: NIOSSHPrivateKey
    private let hostKeyFingerprint: String
    private let deadline: TimeAmount

    private var opened = false
    private var group: NIOTSEventLoopGroup?
    private var connection: Channel?
    private var session: Channel?

    public init(host: String, port: Int, user: String, key: NIOSSHPrivateKey, hostKeyFingerprint: String) {
        self.init(host: host, port: port, user: user, key: key, hostKeyFingerprint: hostKeyFingerprint, deadline: .seconds(10))
    }

    init(host: String, port: Int, user: String, key: NIOSSHPrivateKey, hostKeyFingerprint: String, deadline: TimeAmount) {
        self.host = host
        self.port = port
        self.user = user
        self.key = key
        self.hostKeyFingerprint = hostKeyFingerprint
        self.deadline = deadline
    }

    /// Opens once. A new connection needs a new `SSHSession`.
    public func open(command: String, pty: (cols: Int, rows: Int)?) async throws(ConnectFailure) -> AsyncStream<Data> {
        guard !opened else { throw .closed("already open") }
        opened = true
        let giveUp = NIODeadline.now() + deadline
        let group = NIOTSEventLoopGroup(loopCount: 1)
        self.group = group
        let cause = group.next().makePromise(of: Error?.self)
        let (stream, output) = AsyncStream<Data>.makeStream()
        do {
            let connection = try await connect(group: group, cause: cause)
            self.connection = connection
            let timer = connection.eventLoop.scheduleTask(deadline: giveUp) {
                connection.pipeline.fireErrorCaught(SilentHost())
            }
            do {
                let session = try await openSession(on: connection, output: output)
                self.session = session
                if let pty {
                    try await session.triggerUserOutboundEvent(
                        SSHChannelRequestEvent.PseudoTerminalRequest(
                            wantReply: true,
                            term: "xterm-256color",
                            terminalCharacterWidth: pty.cols,
                            terminalRowHeight: pty.rows,
                            terminalPixelWidth: 0,
                            terminalPixelHeight: 0,
                            terminalModes: SSHTerminalModes([:])
                        )
                    ).get()
                }
                try await session.triggerUserOutboundEvent(
                    SSHChannelRequestEvent.ExecRequest(command: command, wantReply: true)
                ).get()
                timer.cancel()
            } catch {
                timer.cancel()
                // A failed handshake only gives the session a plain EOF. The first pipeline error is the real cause.
                connection.close(promise: nil)
                throw (try? await cause.futureResult.get()) ?? error
            }
        } catch {
            cause.succeed(nil)
            output.finish()
            await close()
            throw ConnectFailure.classify(error)
        }
        guard self.group != nil else {
            output.finish()
            throw .closed("closed while opening")
        }
        output.onTermination = { [weak self] _ in
            guard let self else { return }
            Task { await self.close() }
        }
        return stream
    }

    public func write(_ data: Data) async {
        try? await session?.writeAndFlush(SSHChannelData(type: .channel, data: .byteBuffer(ByteBuffer(bytes: data)))).get()
    }

    public func resize(cols: Int, rows: Int) async {
        try? await session?.triggerUserOutboundEvent(
            SSHChannelRequestEvent.WindowChangeRequest(
                terminalCharacterWidth: cols,
                terminalRowHeight: rows,
                terminalPixelWidth: 0,
                terminalPixelHeight: 0
            )
        ).get()
    }

    public func close() async {
        let session = self.session
        let connection = self.connection
        let group = self.group
        self.session = nil
        self.connection = nil
        self.group = nil
        try? await session?.close().get()
        try? await connection?.close().get()
        try? await group?.shutdownGracefully()
    }

    private func connect(group: NIOTSEventLoopGroup, cause: EventLoopPromise<Error?>) async throws -> Channel {
        let user = self.user
        let key = self.key
        let fingerprint = self.hostKeyFingerprint
        return try await NIOTSConnectionBootstrap(group: group)
            .connectTimeout(deadline)
            .channelOption(NIOTSChannelOptions.waitForActivity, value: false)
            .channelInitializer { channel in
                channel.eventLoop.makeCompletedFuture {
                    let client = SSHClientConfiguration(
                        userAuthDelegate: OneKeyAuth(user: user, key: key),
                        serverAuthDelegate: PinnedHostKey(fingerprint: fingerprint)
                    )
                    try channel.pipeline.syncOperations.addHandlers([
                        NIOSSHHandler(role: .client(client), allocator: channel.allocator, inboundChildChannelInitializer: nil),
                        FirstError(cause: cause),
                    ])
                }
            }
            .connect(host: host, port: port)
            .get()
    }

    private func openSession(on connection: Channel, output: AsyncStream<Data>.Continuation) async throws -> Channel {
        try await connection.eventLoop.flatSubmit {
            let opened = connection.eventLoop.makePromise(of: Channel.self)
            do {
                let ssh = try connection.pipeline.syncOperations.handler(type: NIOSSHHandler.self)
                ssh.createChannel(opened, channelType: .session) { child, _ in
                    child.eventLoop.makeCompletedFuture {
                        try child.pipeline.syncOperations.addHandler(SessionOutput(output: output))
                    }
                }
            } catch {
                opened.fail(error)
            }
            return opened.futureResult
        }.get()
    }
}

struct PinnedHostKey: NIOSSHClientServerAuthenticationDelegate {
    let fingerprint: String

    func validateHostKey(hostKey: NIOSSHPublicKey, validationCompletePromise: EventLoopPromise<Void>) {
        let presented = SSHKeys.fingerprint(hostKey: hostKey)
        if presented == fingerprint {
            validationCompletePromise.succeed(())
        } else {
            validationCompletePromise.fail(HostKeyMismatch(expected: fingerprint, presented: presented))
        }
    }
}

/// Offers the one key once. swift-nio-ssh hangs when the delegate answers nil, so a second ask fails instead.
private final class OneKeyAuth: NIOSSHClientUserAuthenticationDelegate {
    private var offer: NIOSSHUserAuthenticationOffer?

    init(user: String, key: NIOSSHPrivateKey) {
        offer = NIOSSHUserAuthenticationOffer(username: user, serviceName: "", offer: .privateKey(.init(privateKey: key)))
    }

    func nextAuthenticationType(
        availableMethods: NIOSSHAvailableUserAuthenticationMethods,
        nextChallengePromise: EventLoopPromise<NIOSSHUserAuthenticationOffer?>
    ) {
        guard let offer, availableMethods.contains(.publicKey) else {
            return nextChallengePromise.fail(AuthRefused())
        }
        self.offer = nil
        nextChallengePromise.succeed(offer)
    }
}

private final class FirstError: ChannelInboundHandler {
    typealias InboundIn = Any

    private let cause: EventLoopPromise<Error?>

    init(cause: EventLoopPromise<Error?>) {
        self.cause = cause
    }

    // NIO ignores a second completion, so only the first error counts.
    func errorCaught(context: ChannelHandlerContext, error: Error) {
        cause.succeed(error)
        context.close(promise: nil)
    }

    func handlerRemoved(context: ChannelHandlerContext) {
        cause.succeed(nil)
    }
}

/// Drops stderr: a pty sends all output as stdout, and the pairing reply is the stdout line only.
private final class SessionOutput: ChannelInboundHandler {
    typealias InboundIn = SSHChannelData

    private let output: AsyncStream<Data>.Continuation

    init(output: AsyncStream<Data>.Continuation) {
        self.output = output
    }

    func channelRead(context: ChannelHandlerContext, data: NIOAny) {
        let message = unwrapInboundIn(data)
        guard case .channel = message.type, case .byteBuffer(let buffer) = message.data else { return }
        output.yield(Data(buffer.readableBytesView))
    }

    func userInboundEventTriggered(context: ChannelHandlerContext, event: Any) {
        if event is ChannelFailureEvent {
            context.close(promise: nil)
        }
        context.fireUserInboundEventTriggered(event)
    }

    func channelInactive(context: ChannelHandlerContext) {
        output.finish()
        context.fireChannelInactive()
    }

    func handlerRemoved(context: ChannelHandlerContext) {
        output.finish()
    }
}
