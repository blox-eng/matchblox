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

## Go (the demo)

The demo is the matchblox console and service, built from Go as a static
library inside the app. These are the modules it links, from
`GOOS=ios go list -deps ./cmd/mbdemolib`, at the versions of `go.mod`.

| Module | Version | License | Used by |
| --- | --- | --- | --- |
| Go standard library | go1.26.11 | BSD-3-Clause | the Go runtime |
| [charm.land/bubbles/v2](https://pkg.go.dev/charm.land/bubbles/v2) | v2.2.1 | MIT | demo |
| [charm.land/bubbletea/v2](https://pkg.go.dev/charm.land/bubbletea/v2) | v2.0.10 | MIT | demo |
| [charm.land/lipgloss/v2](https://pkg.go.dev/charm.land/lipgloss/v2) | v2.0.6 | MIT | demo |
| [github.com/BurntSushi/toml](https://pkg.go.dev/github.com/BurntSushi/toml) | v1.6.0 | MIT | demo |
| [github.com/charmbracelet/colorprofile](https://pkg.go.dev/github.com/charmbracelet/colorprofile) | v0.4.3 | MIT | demo |
| [github.com/charmbracelet/ultraviolet](https://pkg.go.dev/github.com/charmbracelet/ultraviolet) | v0.0.0-20260928045949-bbf040aedf25 | MIT | demo |
| [github.com/charmbracelet/x/ansi](https://pkg.go.dev/github.com/charmbracelet/x/ansi) | v0.11.8 | MIT | demo |
| [github.com/charmbracelet/x/term](https://pkg.go.dev/github.com/charmbracelet/x/term) | v0.2.2 | MIT | demo |
| [github.com/charmbracelet/x/termios](https://pkg.go.dev/github.com/charmbracelet/x/termios) | v0.1.1 | MIT | demo |
| [github.com/charmbracelet/x/windows](https://pkg.go.dev/github.com/charmbracelet/x/windows) | v0.2.2 | MIT | demo |
| [github.com/clipperhouse/displaywidth](https://pkg.go.dev/github.com/clipperhouse/displaywidth) | v0.11.0 | MIT | demo |
| [github.com/clipperhouse/uax29/v2](https://pkg.go.dev/github.com/clipperhouse/uax29/v2) | v2.7.0 | MIT | demo |
| [github.com/lrstanley/bubblezone/v2](https://pkg.go.dev/github.com/lrstanley/bubblezone/v2) | v2.0.0 | MIT | demo |
| [github.com/lucasb-eyer/go-colorful](https://pkg.go.dev/github.com/lucasb-eyer/go-colorful) | v1.4.1 | MIT | demo |
| [github.com/mattn/go-runewidth](https://pkg.go.dev/github.com/mattn/go-runewidth) | v0.0.30 | MIT | demo |
| [github.com/muesli/cancelreader](https://pkg.go.dev/github.com/muesli/cancelreader) | v0.2.2 | MIT | demo |
| [github.com/NimbleMarkets/ntcharts/v2](https://pkg.go.dev/github.com/NimbleMarkets/ntcharts/v2) | v2.7.0 | MIT | demo |
| [github.com/rivo/uniseg](https://pkg.go.dev/github.com/rivo/uniseg) | v0.4.7 | MIT | demo |
| [github.com/xo/terminfo](https://pkg.go.dev/github.com/xo/terminfo) | v1.2.0 | MIT | demo |
| [golang.org/x/sync](https://pkg.go.dev/golang.org/x/sync) | v0.23.0 | BSD-3-Clause | demo |
| [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys) | v0.48.0 | BSD-3-Clause | demo |
| [rsc.io/qr](https://pkg.go.dev/rsc.io/qr) | v0.2.0 | BSD-3-Clause | demo |

We checked each license in the module's own LICENSE file on 2026-11-02.
