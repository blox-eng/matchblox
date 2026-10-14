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

    private var group: NIOTSEventLoopGroup?
    private var connection: Channel?
    private var session: Channel?

    public init(host: String, port: Int, user: String, key: NIOSSHPrivateKey, hostKeyFingerprint: String) {
        self.host = host
        self.port = port
        self.user = user
        self.key = key
        self.hostKeyFingerprint = hostKeyFingerprint
    }

    public func open(command: String, pty: (cols: Int, rows: Int)?) async throws(ConnectFailure) -> AsyncStream<Data> {
        await close()
        let group = NIOTSEventLoopGroup(loopCount: 1)
        self.group = group
        let (stream, output) = AsyncStream<Data>.makeStream()
        do {
            let connection = try await connect(group: group)
            self.connection = connection.channel
            do {
                let session = try await openSession(on: connection.channel, output: output)
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
            } catch {
                // A failed handshake only gives the session a plain EOF. The first pipeline error is the real cause.
                connection.channel.close(promise: nil)
                throw (try? await connection.cause.get()) ?? error
            }
        } catch {
            output.finish()
            await close()
            throw ConnectFailure.classify(error)
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

    private func connect(group: NIOTSEventLoopGroup) async throws -> (channel: Channel, cause: EventLoopFuture<Error?>) {
        let cause = group.next().makePromise(of: Error?.self)
        let user = self.user
        let key = self.key
        let fingerprint = self.hostKeyFingerprint
        let channel = try await NIOTSConnectionBootstrap(group: group)
            .connectTimeout(.seconds(10))
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
        return (channel, cause.futureResult)
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
    private var done = false

    init(cause: EventLoopPromise<Error?>) {
        self.cause = cause
    }

    func errorCaught(context: ChannelHandlerContext, error: Error) {
        complete(error)
        context.close(promise: nil)
    }

    func handlerRemoved(context: ChannelHandlerContext) {
        complete(nil)
    }

    private func complete(_ error: Error?) {
        guard !done else { return }
        done = true
        cause.succeed(error)
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
