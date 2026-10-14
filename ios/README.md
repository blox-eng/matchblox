# matchblox for iPhone

This app opens the matchblox console on your iPhone.
It pairs with your host by QR code. Then it connects over SSH.

## Build

You need a Mac with Xcode 16 and Homebrew.

1. `make install` installs XcodeGen.
2. `make gen` makes `Matchblox.xcodeproj` from `project.yml`.
3. `make build` builds the app for the simulator.

To run the app on your iPhone, open `Matchblox.xcodeproj` in Xcode.
Select the Matchblox target. Set your team under Signing & Capabilities.

`make test` runs the tests of the MatchbloxKit package.

## CI

The workflow `.github/workflows/ios.yml` runs on a macOS runner.
It runs `swift test` on MatchbloxKit. Then it runs `make build`.
It starts when you change `ios/`, `testdata/pair/` or the workflow.
