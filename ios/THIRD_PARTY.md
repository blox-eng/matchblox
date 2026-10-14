# Third-party software

| Package | Version | License | Used by |
| --- | --- | --- | --- |
| [SwiftTerm](https://github.com/migueldeicaza/SwiftTerm) | 1.20.0 | MIT | app (terminal view) |
| [swift-nio](https://github.com/apple/swift-nio) | 2.103.0 | Apache-2.0 | MatchbloxKit (network core) |
| [swift-nio-ssh](https://github.com/apple/swift-nio-ssh) | 0.15.0 | Apache-2.0 | MatchbloxKit (SSH) |
| [swift-nio-transport-services](https://github.com/apple/swift-nio-transport-services) | 1.28.0 | Apache-2.0 | MatchbloxKit (network) |
| [swift-crypto](https://github.com/apple/swift-crypto) | 4.5.2 | Apache-2.0 | swift-nio-ssh (crypto) |
| [swift-asn1](https://github.com/apple/swift-asn1) | 1.7.3 | Apache-2.0 | swift-crypto |
| [swift-atomics](https://github.com/apple/swift-atomics) | 1.3.1 | Apache-2.0 | swift-nio |
| [swift-collections](https://github.com/apple/swift-collections) | 1.6.0 | Apache-2.0 | swift-nio |
| [swift-system](https://github.com/apple/swift-system) | 1.8.1 | Apache-2.0 | swift-nio |

The versions are the resolved set of the CI build. SwiftTerm also
resolves [swift-argument-parser](https://github.com/apple/swift-argument-parser)
(Apache-2.0) for its `termcast` tool. That tool is not in the app.

We checked each license with the GitHub API on 2026-10-14.
We do not use any GPL code.
