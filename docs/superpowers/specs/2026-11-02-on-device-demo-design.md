# Try Matchblox Pocket with no host

Issue: #282. Status: for the owner's approval. Date: 2026-11-02.

## Why

A person who opens Matchblox Pocket with no host meets a pairing screen they
cannot use. They leave without seeing what the app does. The moment that sells
it is an agent that waits, a question you can read, and one tap that sends it
on its way. The demo gives them that moment in under two minutes, and then
asks them to set up matchblox on their computer.

It may also help TestFlight review. A build was rejected for external testers.
App Store Connect shows "rejected" and no reason, so the cause is not known. See
[TestFlight review](#testflight-review).

## Goals

1. With no host and no account, a person sees the real console and answers a
   real-looking agent by tapping.
2. The first agent they answer ends in an invite they can act on, even though a phone cannot run
   the install.
3. The demo works offline and costs nothing to run.

## Non-goals

- Real agents, tmux or a real shell on the phone. iOS does not allow it, and an
  agent would freeze when the app leaves the screen.
- A hosted demo server.
- Accounts, sign-up or analytics.

## Anti-goals

This fails, even if it runs, when:

- A person takes the demo host for one of their own machines. Its name is `demo`
  in the console's header, the host list, the ended screen and the invite.
- The demo touches real data: saved hosts, device keys, the Island or push.
- It drifts from the product, because it is a copy of the console's screens.
- "Pair a host" gets harder to find for a person who has a host.

## Owner's decisions

- Mock the whole thing, with mock sessions. (2026-11-01)
- A throwaway QR code shows and gets scanned, and then you use the product.
  (2026-11-01)
- It ends with an invite to set up matchblox. (2026-11-01)
- Run on the device, approach A below. (2026-11-01)
- The demo is a demo host itself, and everything from then on is a demo.
  (2026-11-02)

## Approaches

| | What | Gain | Cost |
|---|---|---|---|
| **A. Real console and service on the phone (chosen)** | The Go console and the real service over the fixture machine, linked into the app | Every screen is the real one, offline, no server, cannot drift | Go in the app, 12.44 MB stripped for a test program (3.87 MB gzipped; the app is not yet measured), and a longer build |
| B. Hosted demo | The same console on our server, with a real throwaway QR code | Uses real pairing; it is the "sample QR code" Apple's guideline 2.1 names | A public server that strangers connect to: isolation, uptime, abuse |
| C. Mock screens in Swift | Canned screens with tap areas | Quickest | Fake, and stale after each console change |

A was chosen because it is the real product and has no server to secure. If
Apple will not accept a demo mode, a small B can be added for review alone.

Inside A, a scripted connection with no service was also spiked. It is smaller,
about 10 MB, 2 MB less than the real service, but each screen would need mock data written by hand, and "everything
from then on" would drift from the product. It is the fallback if the service
cannot run on iOS.

## What the spike proved

Branch `spike/on-device-demo`, run on macOS 15 with Xcode 26:

- The console and the scripted machine build for device and simulator with cgo.
  They do not pull in the service, so its `gopsutil` and `purego` dependency
  does not apply.
- A small test program ran on an iOS simulator. It pressed Enter and `2`, and
  the demo reached the invite.
- Size: 10.36 MB stripped on device, about 3.04 MB gzipped. This is a test
  program, not the app.
- On Linux at 44x31, mouse taps opened the session and picked the option.

A second run linked the console and the real service for a device with cgo, so the
`gopsutil` and `purego` dependency builds. That run keeps almost none of the code,
so its 6.8 MB says nothing about size. On Linux, the full `matchblox` binary is
14.9 MB stripped for arm64.

The real service over `testdata/demo` (516 KB, 78 files) on Linux at 44x31 fills
Sessions, Machine and Panes with no new work. Ships, Git, History and Stoker are
empty and need seed data. The host is named `ws-1` and needs the name `demo`.

The first task then ran the real console and service together on an iOS simulator
(macOS 15, Xcode 26), over `testdata/demo`, at 44x31. It drew the 7 sessions, went
to another tab and back with keys, and stopped cleanly. Measured from a test
program, not the app:

- Device, stripped: 12,443,096 bytes (12.44 MB). Gzipped: 3,865,591 bytes (3.87 MB).
- Simulator, peak resident memory: 50,937,856 bytes (50.9 MB) from `getrusage`.

Not proven: a real device, where a process may not start another, a tap through the
app's terminal view, and the size and memory inside the app.

## The experience

1. **The way in.** With no hosts, the pairing screen shows **Try a demo** as a
   second action under the scan. With two or more hosts, the Hosts screen shows it
   beside `Island`, quieter than `+ Pair a host`.
2. **The throwaway code.** A phone cannot scan its own screen. So the scan view
   shows a QR code inside the viewfinder, labelled `demo`. It finds the code by
   itself and confirms like a real pair, in under three seconds. A tap skips it.
   The camera does not turn on, and no permission is asked. The result is a host
   called `demo`.
3. **The host.** `demo` is a host like any other: it has a row in the host list,
   swipe removes it, and with no other host the app opens straight into it. Each
   time it opens, it starts fresh.
4. **The console.** The real console and the real key belt at the screen's size.
   Every tab and key works against the demo host. Sessions can be opened, answered,
   started, renamed and killed. A session waits at the top with its question, so
   the first thing to do is plain.
5. **The answer.** A tap opens its chat. Its own menu asks how to rotate a token. A
   tap on an option picks it, and the agent works for a few seconds.
6. **The invite.** When that agent finishes, a native sheet rises over the console:
   - "Your agents wait the same way."
   - The command `curl -fsSL https://matchblox.sh | sh`, with **Copy**.
   - **Send to my computer**: the share sheet, with the command and the
     get-started link.
   - **Pair a host**, and **Keep looking**, which closes the sheet.
7. **The way out.** The same as any host: quit the console, and the ended screen
   offers **Open again**, with **Pair a host** added for the demo host. Pairing a
   real host removes `demo`.

What the phone cannot do answers: `This is a demo. Install matchblox to do that.`
That covers going into tmux, connecting to another host, pairing a device,
updating, a merge, a new worktree, a clone, and `^C` inside a chat. Nothing else
is refused. The console's ctrl+o "into tmux" hint is hidden in the demo; the key
still answers with the line.

## How it fits together

- **`internal/demo`** starts the real service over a copy of the fixture machine
  in a temp folder, as `--fixtures` does today, connected to the console with
  `net.Pipe`. The tree ships as an app resource.
- **The scripted world** that answers the service's commands exists today as test
  code in `cmd/matchblox/replay_world_test.go`. It already reacts to picks and typed
  text. It moves into `internal/demo`, and the replay test uses it from there, so
  the site's replay and the demo share one world.
- **Seed data** fills Ships, Git, History and Stoker in the fixture tree, or through
  the world.
- **`cmd/mbdemolib`** exports start, write, read, resize, stop and done as C
  functions. It builds to an XCFramework for device and simulator.
- **The host.** `Host` gets a way to say it is the demo host. `ConsoleConnection`
  picks its source from that: the SSH session or the demo. Unlock, `DeviceKey`,
  `Registration` and the Island are skipped for it. The terminal, the belt and the
  gestures do not change.
- **The invite** is SwiftUI. It shows when `done` turns true.

### Rules that guard the anti-goals

Each has a test:

- The demo never reaches a real command. `internal/app` has `os/exec` calls, and
  the service has its own, that cannot run on iOS. An `Options` field in the console
  and the service's `Actions.Run` seam send them to the world or to the line above.
  A test drives each door and key of the console against the demo, and fails on any
  command that reaches the operating system.
- The demo writes nothing outside a temp folder, but its one row in the host store:
  no keychain, no `Registration`, no push, no Island.
- The demo opens no network connection. The Linux test runs it with no route out.

## First task: the riskiest thing

Run the real service on an iOS simulator, over the fixture tree, with the console
on top. It reads the machine through `gopsutil` and paths that may fail on iOS. If
it cannot run without large changes, stop and bring that back to the owner, with the
scripted connection as the fallback. Done: it runs with no change to the service.
The numbers are in [What the spike proved](#what-the-spike-proved).

## Build and CI

- The XCFramework is built by `make -C ios`, in `ios.yml` and `ios-release.yml`.
  Both need Go. The cost on a cold runner is to be measured.
- A simulator job runs the spike's probe on each PR that touches `internal/app`,
  `internal/demo` or `ios/`.
- `ios/THIRD_PARTY.md` lists the Go code the app now ships.
- Export compliance: the app declares only exempt encryption. Check that the Go
  code linked in does not change this.

## Depends on

- #278 (the agent's own menu and the tap) merges first. The demo uses it.

## TestFlight review

Apple gave no reason, so this is a plan under uncertainty. The owner will not ask
Apple for the reason or check the rejected build (2026-11-02), so the plan does
not depend on either.

1. Submit a new build that includes the demo. We learn from its outcome.
2. Fill in Test Information for that build: beta app description, feedback email,
   contact and review notes. Assume they need filling.
3. Guideline 2.1 allows a demo mode in place of a demo account only with Apple's
   prior approval, and it must show full features. So the notes say plainly: the
   app needs the user's own computer running matchblox, and here is the demo. Ask
   for approval in the same message.
4. If Apple names guideline 4.2.7 (remote desktop clients), the review plan stops
   and goes back to the owner. The demo does not answer that rule. The demo stays,
   as it still serves people with no host.
5. A new build goes up only with the owner's yes for the tag. The owner first
   tries it through an internal TestFlight build, which skips review.

Building does not wait for Apple. External testers get the demo when a build passes
review, as with any build.

## Done when

- A person with no host goes from opening the app to the invite in under two
  minutes, by tapping alone, on an iPhone and an iPad.
- Other hosts, keys and the Island are the same before and after the demo.
- Every tab shows real-looking data on the demo host, and no door or key runs a
  command on the phone.
- The owner has tried it on a phone.
- The app's size increase is measured and stated in the PR.
- The rules above have tests, and they run in CI.

## Calls made in writing this

- Building does not depend on Apple's answer, because the demo serves people with
  no host whatever Apple says.
- The belt's Mine editor is off in the demo, as it saves to the person's real
  layout. The rest of the belt works.
- A person with exactly one host opens straight into its console and does not see
  **Try a demo**. They have what the demo sells. The way in on the Hosts screen is
  for two or more hosts, and the pairing screen is for none.
- The demo host is saved in the host list, so it behaves as a host. Pairing a real
  host removes it, so a person is not left with a mock machine they did not ask to
  keep. It starts fresh each time it opens.
- Leaving the demo is the same as leaving a host, with no special strip. The chat's
  title does not say `demo`: the owner's "everything from then on" makes a label on
  each screen needless.
- Real service over hand-written mock data, because it is the only way "everything"
  stays true as the console changes.

## Left to the builder

- The exact words of the script, in the tone of the site's replay.
- The animation's look, within the three-second limit.
- How the Go program is stopped and restarted between entries.
- The names of the `Options` field and the C functions.
