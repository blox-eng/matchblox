# 0006 — matchblox anywhere

A builder is away from the desk. The phone buzzes: `ws-1 · 2 wait`. The
builder taps, Face ID opens the app, and the answer peek of the first
session shows. One tap answers it. The builder is in control in ten seconds.

This spec sets the plan after the comparison with agent-manager
(2026-10-14). It changes the scope of 0001 §3.2 and of some issues (§8).

## 1. Why

agent-manager (YoanWai, Apache-2.0, 594 stars) is a good desk. A builder
sits at it to start agents, prompt them, review diffs and coordinate them.
It has no phone, no remote hosts, no usage limits and no overnight driver.

matchblox does not win at the desk. matchblox wins away from the desk: one
console of your own host, from any terminal, any browser and an iPhone,
that keeps your agents going while you sleep. That is a different category.

The moat is the taste of the console: its design, its words and its calm.
Every window shows that same console. No window draws a second UI.

## 2. Goals, non-goals, anti-goals

**Goals**

1. A builder reaches their own console from a terminal, a browser or the
   iOS app, with the same screens and the same keys.
2. The phone tells the builder when an agent waits, with the app closed.
3. The desk features of agent-manager that builders use each day are in
   matchblox too (§4).

**Non-goals**

1. No accounts, no logins and no hosted console. The host serves its own
   console.
2. matchblox does not give network access to the host. The builder makes
   the host reachable (Tailscale, a public address, a VPN).
3. No diff review, no prompt editor and no model picker. These stay at the
   desk tools.
4. No Android app. Android uses the browser window.

**Anti-goals** (the plan fails if this happens, even when it works)

1. The iOS app or the browser shows a UI that is different from the
   console, and one of them gets a feature late.
2. A server of ours sees pane text, a prompt or an answer.
3. A builder must create an account or type a password.

## 3. Order of work

The owner uses the console of today at the desk, and it is good there. What
is missing is the iOS app. So the order is (owner, 2026-10-14):

1. **The iOS app and the buzz** (§6). §6 does not need §4 or §5.
2. **`matchblox web`**: the console in a browser, from the host (§5).
3. **Catch up at the desk** (§4).
4. **Swarms** (§7). This part is not designed.

## 4. Project 1: catch up at the desk

New issues. Each one is a feature that agent-manager shipped and its users
keep.

| # | Title | What it adds |
|---|---|---|
| new | A builder gets each session back on its own conversation after a reboot | matchblox records the conversation id of each session (Claude Code `--session-id`, the store of each agent). After a reboot or a tmux restart, `v` starts the agent again with `--resume <id>` in the same directory. It follows `/clear` and `/resume`. |
| new | A builder types a line to any session from the peek, not only to one that waits | The peek opens on any session. A busy agent gets the line when it is at rest (#69 `send`). |
| new | A builder finds a session by name and copies its last reply in one key | `/` filters the list. `y` copies the last reply (OSC 52, also over SSH). |
| new | A builder installs matchblox with brew and keeps it current with `matchblox update` | A Homebrew tap, an AUR package, and an update command that checks the sha256 and the attestation. |
| new | A builder sees the agents that other tools start on their own tmux server | Config `[tmux] servers = ["agentmgr"]`. agent-manager runs every agent on `tmux -L agentmgr`, so matchblox shows none of them today. |
| new | A builder gets a desktop alert when an agent waits | `notify-send` on Linux, a notification on macOS, OSC 777 for Ghostty over SSH. It uses the same "something waits" event as the buzz (§6.4). |
| #74 | (change) | A new session can start in its own worktree: `<repo>-worktrees/<name>`, branch `mb/<name>`. |
| #69 | (change) | `send` delivers a line only when the agent is at rest: no dialog, no typed draft. It delivers once and reports `queued`, `held`, `delivered` or `dropped`. It puts a fence around text from another session so that it cannot look like the builder. This is the root fix for #82. |

**Security work** in the same project:

- CI runs gitleaks on each PR.
- Each window (§5, §6) has its own key. `matchblox devices` shows each key
  with its name, kind and last use, and removes a key in one action.

## 5. Project 2: `matchblox web`

1. `matchblox web` serves one page on the host. The page shows the console
   in a browser terminal. The console is the real program in a pty, in the
   shared tmux session `matchblox` (docs/phone.md), so a browser, a laptop
   and the phone show the same console at the same time.
2. Terminal: ghostty-web first, xterm.js if ghostty-web does not work well.
   The files are in the binary. The page has no other UI around the
   console, only the matchblox mark and the connection state.
3. Login: a passkey. The builder adds a browser from the app or from the
   console (`matchblox pair --web`). A passkey needs HTTPS on a hostname.
   With Tailscale, matchblox uses `tailscale cert` for the name of the
   host. On a bare IP address, the page asks for a one-time code from the
   console. Docs say that the IP case is weaker.
4. matchblox listens only on the address that the builder names. It does
   not listen on all addresses by default.

## 6. Project 3: the iOS app

### 6.1 The window

1. The app is a terminal that opens one thing: the console of a host. It has
   no shell and no other screens, except the hosts, pairing and settings.
2. It connects with SSH. The terminal is SwiftTerm (MIT). SSH is SwiftNIO
   SSH (Apache-2.0). Verify both licenses before the first commit. Do not
   fork Blink Shell: it is GPL-3.0.
3. The app attaches to the shared tmux session `matchblox`, like Termius
   today (docs/phone.md). It does not start a second console. The session
   uses `window-size latest`: the size follows the device that the builder
   used last, and the console changes its layout to that size. The phone
   layouts already use touch. (My call.)
4. The app pairs with each host on its own: one key, one buzz source and one
   label for each host. (My call.)
5. The phone layouts of v0.6.0 (docs/phone.md) are the app UI. The golden
   screens at 44×31 and 86×30 (testdata/screens) are its tests.

### 6.2 Keep the console and the app in sync

There is nothing to sync. The console is the only UI. The app, the browser
and each terminal only draw what the console sends. A change to the
console is in the app at the next host update, with no app release.

The app has three native parts. Each one is small and has a clear contract:

| Part | Contract | How it stays correct |
|---|---|---|
| Pairing and hosts | The QR payload (§6.3) | One JSON fixture in `testdata/`. A Go test and a Swift test read the same file. |
| The buzz and Live Activity | The push payload (§6.4) | The same: one fixture, read by both tests. |
| Colours of the Live Activity | DESIGN.md tokens | A script writes `Tokens.swift` from the tokens. CI fails when the file is old. |

The app is in this repo, in `ios/`. One PR changes the contract and both
sides. The app has its own release (TestFlight, then the App Store). Its
version does not follow the matchblox version.

### 6.3 Pairing

Pairing uses the sshd of the host. matchblox opens no port. (My call.)

1. On the host, `matchblox pair` makes a one-time ed25519 key. It adds the
   public key to `authorized_keys` with
   `restrict,expiry-time="<system-zone YYYYMMDDHHMM>",command="<exe> gate pair"`.
   The time is 5 minutes from now, rounded up to the next whole minute, in
   the system zone with no `Z`. sshd reads that from OpenSSH 7.7. A `Z` (UTC)
   needs OpenSSH 9.1, and older sshd rejects the whole line.
2. The console shows a QR code. It holds the host address, the host key
   fingerprint, the name of the host and the one-time private key.
3. The app scans it. The app makes a P-256 key in the Secure Enclave. Face
   ID or the device passcode opens the key.
4. The app connects with the one-time key and sends its public key, its
   name and its push tokens. `gate pair` adds the phone key with
   `restrict,pty,command="<exe> gate phone"` (0005 §3.3) and removes the one-time
   key, in one locked write. A one-time key works once. A key that already
   opens a login of its own is refused (`key in use`). The host prints the
   name and fingerprint of the phone that paired.
5. The gate today lets through only `matchblox serve --stdio`
   (cmd/matchblox/gate.go). It must let through exactly these commands and
   nothing else. A laptop key (`gate`) keeps the stream alone; only a phone
   key (`gate phone`) gets the console, because the console is a tmux client
   and can open a shell:
   - `matchblox serve --stdio` (the laptop console, as today; every key);
   - the console, attached to the shared session (the app; a phone key only);
   - `matchblox open <target>` (a tap on an alert opens the answer peek of
     that session: the service tells the console);
   - `matchblox devices token` (the app sends fresh push tokens on each
     connect);
   - `matchblox gate pair`, only for a one-time key.
5. The console in the app can go to each agent pane. An agent pane can run
   commands. So a phone key has the power of the builder. Docs must say
   this. The protection is the Secure Enclave and Face ID, not the gate.
6. `matchblox devices` lists the keys of phones and browsers and removes one.

### 6.4 The buzz

1. The service already knows when a session waits. For each paired phone,
   the service sends an event to the relay when the set of waiting sessions
   changes. It sends at most one event each 30 seconds for each host.
2. The relay is a route on the Cloudflare Worker of matchblox.sh. It keeps
   no data and has no accounts. It signs the event with the matchblox APNs
   key and sends it to Apple.
3. An alert holds an encrypted blob. The host encrypts it to a key of the
   phone. A Notification Service Extension opens it on the phone:
   `ws-1 · 2 wait · api: "Run the migration?"`. The relay sees only the
   device token and the blob.
4. A Live Activity update cannot be encrypted (Apple shows it with no
   extension). It holds only a session number, the state, the progress and
   the counts. The phone changes the number to a session name from its own
   list, which it saved at the last connect. The docs say this. §6.6 tells
   what the activity shows.
5. A tap opens the app on the answer peek of the first session that waits.
6. The relay keeps no data, so it checks a grant. At pairing, the app asks
   the relay for a grant with an App Attest attestation (only the real app
   gets one). The grant is an HMAC of the device token with a relay secret.
   The host sends the grant with each event. The relay checks the HMAC and
   sends nothing without it. Cloudflare rate rules limit each token. (My
   call.)
7. The buzz must work when no console is open. `matchblox setup` adds a
   door that installs the service as a user unit (systemd on Linux, launchd
   on macOS), so it starts at boot. (My call.)

### 6.5 Reconnect

1. The app knows a host by its key fingerprint, not by its address.
2. When the host does not answer, the app shows one screen:
   `Can't reach ws-1. New address?` with the last address, a field and
   `Scan again`. A new address keeps the pairing.
3. A changed host key stops the connection and shows a warning. It never
   connects silently.

### 6.6 The match on the lock screen

Mockup (approved by the owner, 2026-10-14):
[0006-live-match.html](0006-live-match.html).

1. The Live Activity is always on. It shows one match: the session that is
   most important now. A session that waits comes before a session that
   burns. Of the sessions that wait, the oldest wait comes first. Of the
   sessions that burn, the one that the builder prompted last comes first.
   #40 (importance levels) can replace this order later.
2. The match lies horizontally. The burnt head is on the left. The flame
   moves to the right as the session progresses. The progress is the last
   `Progress [████░░░░]` line of the agent. With no such line, the flame
   stays near the head.
3. States. Each one has the same meaning as in the console (DESIGN.md §4):

   | State | The match | The word |
   |---|---|---|
   | burns | the flame burns at the progress point | `✦ burns` |
   | waits | the flame is out, the ember breathes each 2.4 s, smoke rises | `╿ waits` (accent) |
   | done | burnt to the end, cold | `╿ done` |
   | out | cold, no smoke | `out until 16:40` (warn) |
   | quiet | the box is closed, the heads show | `All quiet · 7 sessions rest` |

4. The top line has `▰ ws-1` and the counts `✦ 5 ╿ 2`. The bottom line has
   the session name and its state word. There is no percentage on the lock
   screen. The match is the number.
5. Dynamic Island: compact shows a small match and the state word; minimal
   shows the front of the match; expanded shows the match, the percentage
   and the button `Answer` (or `Open`).
6. A tap on the activity or on `Answer` opens the answer peek of that
   session.
7. Only fire and smoke move (DESIGN.md §1, rule 1).
8. The activity lives 8 hours. The host starts a new one with push-to-start
   before the old one ends. Progress updates go out in steps of 5 %, at
   most every few minutes. A new wait, done or out goes out at once, with
   high priority.
9. The colours come from the DESIGN.md tokens (`flame`, `flame-amber`,
   `flame-core`, `match-head`, `match-ember`, `mark-*`), through
   `Tokens.swift` (§6.2).

## 7. Project 4: swarms (not designed)

A swarm is a **box of matches, ready to burn** (owner, 2026-10-14). A box is
a set of sessions for one goal, on a host that matchblox prepares. The
builder fills a box with the matches it needs, and the stoker strikes them
and keeps them burning. The quiet state of §6.6 (a closed box, the heads
ready) is the first picture of it. The issues that belong here: #10 (a container), #15
(prepare a host), #12 (MCP server), #55 (accounts). Design it after §6
ships.

## 8. Changes to existing issues

| Issue | Change |
|---|---|
| 0001 §3.2 | "Not planned: starting agents" goes. #74 starts sessions. Diff review and a prompt editor stay not planned. |
| docs/phone.md | Termius stays as the way without the app. The app becomes the first way. |
| #74 | Add: a new session can have its own worktree (§4). |
| #69 | Add: the at-rest delivery, status and fence of `send` (§4). |
| #82 | Link to #69: the at-rest check is the root fix. |
| #13 | Close as done by #69. #69 is the answer to its question. |
| #12 | Move to the swarms project (§7). Keep the rule: a destructive act waits for a person. |
| #10, #15, #55 | Move to the swarms project (§7). |
| #22 | No change. The app reuses its "back from any pane in one tap". |

## 9. Security summary

1. No server of ours sees pane text. The relay sees device tokens, encrypted
   alerts and the counts of Live Activities.
2. Each window has its own key. The phone key is in the Secure Enclave. The
   browser key is a passkey. Each one can be removed.
3. The host key is pinned. A changed key stops the connection.
4. `matchblox web` listens only on the address that the builder names.
5. The text that one session sends to another has a fence (§4, #69).

## 10. Next step

1. The iOS issues: (a) #87, the gate lets the console through (§6.3.5),
   (b) #88, the window and pairing (§6.1, §6.3, §6.5), (c) #89, the buzz
   and the relay (§6.4), (d) #90, the match on the lock screen (§6.6).
2. The other issues: #91 (§5), #92–#97 (§4), #98 (§7). The changes of §8
   are on #10, #12, #13 (closed), #15, #55, #69, #74 and #82.
3. Start with 1(a) and 1(b). With them, the builder uses the app each day
   instead of Termius. 1(c) and 1(d) follow.
4. Before the first TestFlight build: an Apple Developer team and, for
   1(c), an APNs key.

## 11. Calls made in this design

These are the calls of the writer, not of the builder. Check them:

1. The app attaches to the shared console; the size follows the last device
   (§6.1.3).
2. The app pairs with each host on its own (§6.1.4).
3. Pairing uses a one-time SSH key that expires in 5 minutes (§6.3).
4. The relay checks an App Attest grant, with no stored data (§6.4.6).
5. The service runs as a user unit, so the buzz works after a reboot
   (§6.4.7).
6. The app is in this repo, in `ios/` (§6.2).

## 12. Left to the builder

- The names of the Swift packages and targets, and the minimum iOS version
  (Live Activity push-to-start needs iOS 17.2).
- The QR payload format, if it holds the fields of §6.3.1.
- The encryption scheme of §6.4.3, if the relay cannot open it.
- How the app shows the keyboard keys that a console needs (Esc, Tab,
  arrows, Ctrl).
