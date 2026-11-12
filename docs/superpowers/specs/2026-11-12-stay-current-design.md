# matchblox stays current: an offer at the console, and an opt-in that updates by itself (#390)

## Intent

A builder runs matchblox on a laptop and on one or more hosts. The machines
drift apart: one gets `matchblox update`, the others do not. On 2026-11-12 a
console on v0.42.0 drew no chain fuses for a host on v0.38.1, and nothing
said that the host ran an older release (#389). The fix was `matchblox
update` on the host, which the builder had no reason to think of.

matchblox now tells the builder when a machine is behind and offers the
update in place. A machine that opts in updates by itself, with no console
open. The builder can turn the offer off.

## Goals

- When a console opens and a newer release exists for the machine it sits
  on, the console offers the update. One action installs it.
- When a console reaches a host whose release differs from its own, the
  host's row says so and offers the update of that host.
- A machine with `mode = "auto"` installs a new release by itself, with no
  console open and no person at it.
- The builder can hide the offer for one release, and can turn every check
  off.

## Non-goals

- No update of the iOS app: the App Store owns it.
- No update of a binary a package manager installed. The offer names that
  manager's command (`brew upgrade matchblox`), as `matchblox update` does.
- No update of a build that is not a release (`dev`): no check, no offer.
- No downgrade, no pinned version and no release channels.
- No change to the wire version. A console and a host on two releases keep
  talking as they do today; they now say that they differ.
- No update of a host from the laptop without a person's key press. A host
  updates by itself only from its own `mode = "auto"`.

## Anti-goals

- An update must never end, stall or lose a session. Agents live in tmux;
  only the matchblox service restarts.
- A machine that did not opt in must never update by itself.
- With `mode = "off"` matchblox makes no request to find a release.
- A failed check or a failed install must never show as "up to date". It
  says what failed and the fix.
- The phones must not go silent after an update: the service that took the
  update is the one that runs again, with no console needed.

## Constraints

- An update is the existing updater (`cmd/matchblox/update.go`): the
  release's checksum, its build provenance when `gh` can check it, and the
  version the new binary names. No second way to install.
- A console never sends a command line. It names an act; the service builds
  and runs it (`proto.Act`). A host is reached only through its gate.
- Today the service stops when its binary changes on disk. That stays.
- Today a console replaces a local service of any other build at hello,
  a newer one too (`olderLocal`, `internal/app/model.go`). That changes:
  see "An old console and a new service".
- Offers a builder can close exist already: the setup doors
  (`internal/doors`), with `Closed` kept by the service.

Approved by Martin on 2026-11-12.

## Approaches

1. **Match on connect.** The console makes a lagging host update when it
   connects. It fixes the incident. It leaves the builder's own machine and
   any host nobody opens out of date.
2. **Each machine asks when a console opens.** One setting picks ask, do it,
   remind or off. It is simple. A host that is rarely opened stays old,
   which was the incident.
3. **Opt-in, in the background, plus an offer at the console.** Picked by
   Martin on 2026-11-12. The service of each machine owns its own update:
   it offers by default and updates by itself when told to. The console
   shows the offer of its own machine and of each host. A host on `auto`
   never drifts; a host on the default is one key away.

A timer that updates every machine with no opt-in was dropped: it swaps the
service under running work on a release nobody chose.

## Design

### One setting

```toml
[update]
mode = "offer"   # "offer" (default), "auto" or "off"
```

Each machine reads its own file. `offer`: check, and show the offer.
`auto`: check, and install. `off`: no check, no request, no offer.

### The check

The service looks for the latest release when it starts and once a day
after that, the way `matchblox update` finds it. It puts what it found in
the state every console reads: the latest release, when it looked, and why
a look failed. The console never makes the request itself.

A new default request leaves the machine once a day, to GitHub, with no
data of the builder in it. `docs/privacy.md` says so, and names `mode =
"off"`.

### The offer on this machine

The offer is a door: "matchblox v0.43.0 is out", with what it runs as the
preview. Its action installs the release through the service. The service
then runs again on the new binary. The console then starts itself again on
the new binary too, so the release it names and compares hosts with is the
one on disk. Closing the door hides it for that release
only; the next release shows a new one. `matchblox setup` shows a closed
one again, as for every door.

A Homebrew install gets the same door with the Homebrew command as its
text, and no action that replaces the binary.

A console whose home is a remote host (`matchblox <host>`) has no local
service to ask. It makes no request. It compares its own release with the
latest one its home host knows; when it is behind, it shows the notice with
the command to run on this machine: `matchblox update`.

### An old console and a new service

A service can now be newer than a console that is still open: auto updated
it, or another console did. That console must not put the old build back.
A console replaces a local service only when the service is older. When
the service is newer, the console says so and offers to start itself again
on the new binary.

### The offer for a host

The hello of a host names its release. When it differs from the console's,
the host's row says both releases. When the host knows a newer release than
its own, the row offers one action: update this host. The act goes through
the gate to the host's service, which installs with its own updater and
runs again; the console reconnects.

A host older than this feature does not know the act. Its row gives the
command to run there: `matchblox update`. This is the notice #389 asks for.

When the host is newer than the console, the row says so and points at the
offer of this machine.

A host with `mode = "off"`, or one whose check failed, knows no newer
release. Its row still says that the releases differ and still offers the
act. A key press is the person's consent, as `matchblox update` typed there
is: the act looks for the release then. `off` stops only the checks nobody
asked for.

Closing the notice hides it for that host and that pair of releases.

### Auto

With `mode = "auto"` the service installs a release one day after this
machine first saw it, then runs again on it. The day lets a bad release be
withdrawn before machines take it by themselves, and needs no second kind
of request: the check as it is today carries no publish date. A machine
that was off for a week waits one more day. The offer door still shows
during that day, for the builder who wants it now.

Auto waits for a service with no work of its own in flight: no stoker run
on and no clone running. It installs at the first check after that.

Auto installs on the same proof as `matchblox update`: the checksum always,
and the build provenance when `gh` can check it. It records which of the
two it had. Ruling, Martin, 2026-11-12: the same proof as the manual
update. Reason: a rule that needs `gh` would leave auto doing nothing on
most hosts.

The service records each automatic update (from, to, when, or why it
failed) where the console shows it once: "updated to v0.43.0 at 03:12".

### What the builder sees

```
SETUP
  matchblox v0.43.0 is out          enter: update   x: not this one

HOSTS
  ws-1   v0.38.1 · this console v0.42.0     u: update ws-1
```

## Calls made in this document (Claude's, not Martin's)

- The offer also covers a lagging host. Martin asked for the opt-in and the
  offer at start; the host row is added because it was the incident.
- One key, `mode`, with three values, in place of two switches.
- "Suppress" is two things: close the door for one release, or `mode =
  "off"` for all of them.
- Auto waits one day after the machine first saw a release, and waits for
  a service with no stoker run and no clone in flight.
- A person's key press on a host's row updates it even when that host's
  mode is `off`.
- After an update the console starts itself again; a console never
  replaces a newer service with its own older build.
- A console whose home is a remote host gets the notice and the command,
  not an action.
- The service makes the check, not the console, so `off` is one place.
- The daily check is on by default.

## Left to the builder

- How the service runs again on the new binary by itself (re-exec, or the
  keep-running unit where it exists), as long as no console is needed.
- Where the door and the host notice sit in each layout, and the keys.
- The state file for a closed offer and for the record of automatic
  updates.

## Done when

- A console on an older release than the latest shows the offer; its action
  leaves `matchblox version` at the latest and every session listed.
- A console and a host on two releases show both on the host's row; the
  action updates a host that knows the act, and an older host shows the
  command.
- A service with `mode = "auto"` that first saw a release more than a day
  ago, with no stoker run and no clone in flight, updates with no console,
  runs again, and still pushes to the phones.
- A console left open on the old build does not replace the updated
  service; it offers to start again.
- `mode = "off"` makes no request (a test with a server that fails on any
  request).
- A closed offer stays closed for that release and returns for the next.
- A failed check and a failed install each show their reason and the fix.
- A test draws a chain of a peer host in the Sessions list (#389).
- The README, `docs/`, the landing page (`www/`) and the changelog describe
  it (Martin, 2026-11-12).
