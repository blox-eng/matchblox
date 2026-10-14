import Foundation
import Network
import NIOCore

public enum ConnectFailure: Error, Equatable {
    case unreachable
    case hostKeyChanged
    case refused
    case closed(String)

    private static let unreachableCodes: Set<POSIXErrorCode> = [
        .ECONNREFUSED, .ETIMEDOUT, .EHOSTUNREACH, .ENETUNREACH, .EHOSTDOWN, .ENETDOWN,
    ]

    public static func classify(_ error: Error) -> ConnectFailure {
        switch error {
        case let failure as ConnectFailure:
            return failure
        case is HostKeyMismatch:
            return .hostKeyChanged
        case is AuthRefused:
            return .refused
        case let channel as ChannelError:
            if case .connectTimeout = channel { return .unreachable }
        case let network as NWError:
            if case .dns = network { return .unreachable }
            if case .posix(let code) = network, unreachableCodes.contains(code) { return .unreachable }
        default:
            break
        }
        return .closed(String(describing: error))
    }
}

struct HostKeyMismatch: Error {
    let expected: String
    let presented: String
}

struct AuthRefused: Error {}
