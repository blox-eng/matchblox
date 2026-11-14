# matchblox for iPhone and iPad

This app opens the matchblox console on your iPhone or iPad.
It pairs with your host by QR code. Then it connects over SSH.

On the store it is **Matchblox Pocket**, in beta on TestFlight:
[join First Strikers](https://testflight.apple.com/join/t2rtbjHz). This page
is about building and shipping it yourself.

## Build

You need a Mac with Xcode 26 and Homebrew. The app needs iOS 18.0 or later.
(App Store Connect requires the iOS 26 SDK, which Xcode 26 ships.)

1. `make install` installs XcodeGen.
2. `make gen` makes `Matchblox.xcodeproj` from `project.yml`.
3. `make build` builds the app for the simulator.

To run the app on your iPhone or iPad, open `Matchblox.xcodeproj` in Xcode.
Select the Matchblox target. Set your team under Signing & Capabilities.
The first build asks you to trust the build plugin of SwiftTerm. Tap
Trust & Enable. Xcode asks only once.

If the build says that the Metal Toolchain is missing, run
`xcodebuild -downloadComponent MetalToolchain`. SwiftTerm needs it.

The app has two extensions: MatchbloxIsland (the Live Activity) and
MatchbloxBuzz (the buzz). Set your team on all three targets. The app needs
these capabilities: Push Notifications, App Attest, App Groups
(`group.com.blox.matchblox`) and Keychain Sharing
(`com.blox.matchblox`, the app's own and first, then
`com.blox.matchblox.shared`). The extensions carry the App Group, and the
buzz also the shared keychain group, which holds only the notification key.

If Xcode says that the bundle identifier is not available, give your
build its own identifier. Do not change `project.yml`. The app, both
extensions, the App Group and the keychain group all derive from the one
setting `MATCHBLOX_BUNDLE_ID`, so one override is enough:

```sh
xcodebuild -scheme Matchblox -destination 'id=<device id>' \
  -allowProvisioningUpdates DEVELOPMENT_TEAM=<team> \
  MATCHBLOX_BUNDLE_ID=com.blox.matchblox.<you> build
```

`xcrun devicectl list devices` shows the device id.

`make icon` makes the app icon from `AppIcon.svg`, the mark of DESIGN.md §3.
It needs `rsvg-convert` (`brew install librsvg`).

`make test` runs the tests of the MatchbloxKit package.
`make uitest DESTINATION=id=<simulator udid>` runs the UI tests. The `ios-demo` workflow runs them on each pull request and each push to main.

## CI

The workflow `.github/workflows/ios.yml` runs on a macOS runner.
It runs `swift test` on MatchbloxKit. Then it runs `make build` and `make release`.
`make release` archives the app in Release as TestFlight does, without signing.
If it fails, the run keeps the compiler's full output as an artifact.
It starts when you change `ios/`, `testdata/pair/` or the workflow.

## Publish to TestFlight

The workflow `.github/workflows/ios-release.yml` builds a signed Release and
uploads it to TestFlight. It needs no Mac of yours: it runs on GitHub's
macOS runner, with fastlane and Xcode automatic ("cloud") signing driven by
an App Store Connect API key. The one-time Apple-side setup (the three App
IDs, the App Group, the App Store Connect app record, the API key and the
GitHub secrets) is in [RELEASING.md](../RELEASING.md#ios-testflight).

To cut a release, push a tag:

```sh
git tag ios-v0.2.0 && git push origin ios-v0.2.0
```

The tag after `ios-v` is the marketing version (`CFBundleShortVersionString`).
Or run the workflow by hand from the Actions tab and type a `marketing_version`.
The build number (`CFBundleVersion`) is the next one after the latest on
TestFlight, the same for the app and both extensions; the first ever build
is 1.

A build from Xcode signs the `development` entitlements (push and App
Attest), which is right for a build you install yourself. The Release
configuration the workflow archives signs the `production` ones
(`Matchblox/Matchblox.production.entitlements`), which TestFlight needs.
The push relay must then run with no `APNS_ENV`, or it answers
`BadDeviceToken`; see [RELEASING.md](../RELEASING.md#ios-testflight).

## Check on a device

CI has no Secure Enclave, no camera and no Face ID. Do these checks on a
device before a release. `ws-1` is the host.

1. On the host, run `matchblox pair`, or open Pair a device in its console.
   Scan the code with the app. There is no Pair button: the scan is the
   consent. The box opens on both screens, then the app opens the console.
   The host prints `Paired <name> (<fingerprint>)`.
2. `matchblox devices` on the host lists the device.
3. Turn the device on its side. The console takes the landscape layout.
   Open the keyboard. The console and the key row fit above it.
4. Attach a laptop to the same session. The size follows the device that
   you used last.
5. Quit and open the app again. You see one Face ID, then the console.
   With one host, there is no hosts list.
6. Change the address of the host to a wrong one. You see
   `Can't reach ws-1.` Type the right address. You see
   `Connecting to ws-1`, then the console.
7. Scan the same code again. You see
   "This code was used or expired. Run matchblox pair on ws-1 again."
8. Open the console and cancel Face ID. You see "The key is still locked.
   Tap Try again, then use Face ID or the passcode." Tap Try again. You
   do not see "Can't reach".
9. Fail Face ID, then type the passcode slowly, for more than 10 s. The
   console still opens.
10. Signing after the unlock does not show a second Face ID.
11. Make `matchblox console` stop at once on the host. The app tries 3
    times with no new Face ID. Then it says
    "matchblox console on ws-1 stops at once. …". Fix the host, then tap
    Try again. The console opens with no Face ID.
12. Quit the console on the host. The app says
    "matchblox console on ws-1 ended." Tap Open again. The console opens.
13. Put the app in the background and open it again. You see one Face ID,
    then the console.
14. Change the network while the console is open (for example, Wi-Fi to
    mobile data). The app has no keepalive yet: write down what you see.
15. Pair two hosts. Swipe to remove one. The app says how to remove its
    key on the host.
16. Select a session in tmux and tap `A switch` under it. Switch agent
    opens; tap another agent, then `Enter`, then type `y`. The console
    jumps to the pane, where the new agent starts.
17. On a host with a question waiting, tap that session once: you are in
    its pane, at the question, and typing reaches the agent. Turn the phone
    to landscape, then back. The pane still shows the question.
18. On a real host, tap a row of the session list once. You are in the
    agent's own tmux pane; the tap clicks once and the pane is not clicked
    through. The keyboard rises on that tap. A desk attached to the same
    session stays where it was.

### The demo

Do these on an iPhone and on an iPad, with a build that has no host paired
and again with two hosts.

1. With no host, the pairing screen shows `Try a demo`. With two hosts, the
   Hosts screen shows it beside `Island`.
2. On a fresh install, the pairing screen asks for the camera first. Allow it
   once and deny it once (reinstall between): `Try a demo` works both ways.
   Tap it. The scan view finds its own code in under three seconds. It turns on
   no camera and asks for no permission. The app opens the console on the
   host `demo`.
3. The board opens on flint-auth's card in `NEEDS YOU`, with its question's
   options in the peek. Tap `2. Rotate, keep the old one a minute`: the card
   marks it, and the key line says `2 again sends`. Tap it again to send it.
   The card strikes into `WORKING`. Or tap the card again to open the
   stand-in for its pane (`type 2 to answer`), and press `↓` then `⏎` on the
   belt. The agent works for a few seconds, then the sheet rises with
   `Your agents wait the same way.`, the line, `Copy`, `Send to my computer`,
   `Pair a host` and `Keep looking`. `Copy` puts the line on the pasteboard.
   `Send to my computer` opens the share sheet.
4. On an iPhone, turn the device to landscape at the question. Under 16 rows,
   the question card shows in place of the screen. Turn the device back, and
   the stand-in shows the question again.
5. Tap `Keep looking`, then quit the console with `q`. The ended screen offers
   `Open again`, `Pair a host`, the line, `Copy` and `Send to my computer`.
6. Kill the app in the middle of the demo and open it again. The demo starts
   fresh, and the first launch left nothing of the old one.
7. Open a host you paired. Its console, the Island and the host list are the
   same as before the demo.
8. Pair a real host. The host `demo` is gone.

### The buzz and the island

Use a private test host with its own `matchblox serve` (the door
`Keep matchblox running` in `matchblox setup`). `ws-1` is the host; `work-1`
is a second paired host.

19. Open the console of `ws-1` once, so the app registers. Allow
    notifications when the app asks. `matchblox devices` on `ws-1` shows
    `push` next to the device.
20. The buzz with the app closed: quit the app. Make a session on `ws-1`
    wait. The device shows `ws-1 · <session> waits: "<question>"` within
    30 s, with no app open.
21. Push to start: with no island on the device, start a session on
    `ws-1`. The island starts with the app closed.
22. Both faces: on the Island screen (Hosts, then Island), pick `Sessions`.
    The island shows the Sessions face. Pick `Usage`: it shows the windows
    and the forecast. Pick `Auto (Usage while the stoker runs)`, then turn
    the stoker on and off on the host: the face follows.
23. Show changes the face at once, with no Face ID and no connect.
24. A second host buzzes and does not take the island: keep `Follow` on
    `ws-1`, make a session on `work-1` wait. The device buzzes for `work-1`;
    the island still shows `ws-1`.
25. Change `Follow` to `work-1`. One Face ID; then the island of `ws-1` ends
    and `work-1` can start one. Turn off the network of `ws-1` first, and
    the screen says `ws-1 is not reachable: it changes at the next connect`.
26. Turn `Buzz` off for `ws-1`. One Face ID. A wait on `ws-1` no longer
    buzzes; `work-1` still does.
27. A tap opens the session: tap the island, and tap a buzz. After Face ID,
    the console of that host opens on that session. A session that ended
    says `That session ended.`
28. Stray activities end at launch: follow `ws-1`, let its island run,
    then make `ws-1` unreachable and change `Follow` to `work-1`. The island
    of `ws-1` shows nothing. Quit and open the app: the activity of `ws-1`
    is gone.

### The pane and the key belt

Use an iPhone held upright, with a Claude Code session that waits on a
question.

29. Tap the session's card to choose it, then tap it again: you are in the agent's tmux pane, with the agent's own menu at the bottom. `↓ ↓ ⏎` picks the third
    option.
30. Each key of Steer (`◂ ↑ ↓ ⏎ Esc ^C ⇞ ⇟`) works on the first tap, with
    the keyboard up and with it down. `^C` reaches the agent as tmux sends
    it. A drag on the belt never presses a key, even when it starts on one.
31. Turn the belt. You feel a tick for each stop, halfway between two stops,
    and a firmer click at home (`◂` at the left). Keys fold round the phone
    at both edges, and the belt turns on without end. It always stops with
    a key at the left. Check at 60 and at 120 fps (ProMotion): no dropped
    frames.
32. `⌨` on the handle row hides the keyboard, and the belt stays at the
    bottom. `⌨` again brings the keyboard back. It never opens the tray.
    Typing has no autocorrect or suggestions.
33. Tap the grip, tap the name, and pull up on the handle row: each opens
    the tray. Pick another match: it strikes, the old one goes out, and
    the tray closes after 0.9 s. The screen does not change size. Open the
    tray again: every match is new, and the one in use burns. The layout
    stays after you quit the app.
34. Choose Commands in a Claude Code pane: the commands carry its mark, and
    `compact` types `/compact` into the pane. Go into a session of another
    agent: the commands change, and each types its own text. Back on the
    console, the command keys are dim, and a tap says `commands run in an
    agent's pane`.
35. Choose Mine, then `EDIT MINE`. Tap a key to add it and to take it off.
    Drag a key to move it. A tap on `◂` says `◂ always stays: it is the
    way back`. Quit the app and open it again: Mine is the same.
36. Swipe down in a session's live view: earlier lines show, and the key
    line says `↑ n lines back`.
37. Press `Ctrl+O` on a hardware keyboard to go into the tmux pane. A swipe
    scrolls its history, about as fast as the finger. A long press, then a
    drag, selects text. `◂` goes back to the console.
38. On a 320 pt screen (Slide Over on iPad), no key clips and the belt
    still folds at both edges.
