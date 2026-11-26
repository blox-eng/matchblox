# The island on Pocket: live when collapsed, one tap back

Status: scope agreed in chat (owner, 2026-11-25: "1. relay setup 2. island
back to the app"); this file awaits the owner's review. Language: ASD-STE100.
Builds on design/0010-the-island.md. Where this file and 0010 differ, this
file applies.

## 1. Who, and the moment

The builder collapses Matchblox Pocket on the iPhone. The Dynamic Island and
the lock screen show the live match and stay current. The builder taps the
island. Pocket comes back on the same console, at the same place, with no
blank screen. If the builder left less than five minutes ago, no Face ID
prompt shows.

## 2. What is true today (2026-11-25)

- `matchblox devices` shows `buzz off · island no` for each phone.
- `POST https://matchblox.sh/push/grant` answers
  `500 {"error":"relay not configured"}`: the five Worker secrets
  (RELEASING.md) are not set. The island never had a relay.
- `ConsoleView.swift`: at `.background` the console calls `stop()`, which
  drops the connection and invalidates the Face ID unlock. At `.active` it
  connects again: Face ID, the `Connecting` cover, and a reset terminal.
- `MatchbloxApp.open(_:)`: a tapped island link bumps `linkOpens`, so a new
  `ConsoleView` replaces the open one, also for the host already open.

## 3. Goals

1. **Relay setup.** The shared relay at `matchblox.sh` has its secrets, in
   production APNs. Each phone registers: `buzz on · island yes`.
2. **Back to the app.** A tap on the island, or a plain return to Pocket,
   shows the same console, at the same place, fast:
   - the console for the host already open stays; it is not rebuilt;
   - the terminal keeps its last screen while it connects again; no cover,
     no reset; the tmux attach repaints it in place;
   - the Face ID unlock holds for **5 minutes** after Pocket goes to the
     background (owner, 2026-11-25: "keep face id lock to unlock for some
     minutes"). After 5 minutes, Face ID asks again.

## 4. Non-goals

- A relay of our own per user, or a third-party push service (owner,
  2026-11-25, after research: one stateless relay is the normal pattern).
- Buttons on the Live Activity. A tap opens the session, as 0010 says.
- Keeping the SSH connection open in the background. iOS stops it within
  about 30 seconds; tmux holds the session on the host.
- #153 (rotation and bad-token robustness), #155 (VoiceOver), #385.

## 5. Anti-goals

- A key opens a host with no Face ID or passcode in the last 5 minutes.
  0010's rule "no key opens a host without Face ID" now reads "without Face
  ID or the passcode in the last 5 minutes" (owner, 2026-11-25).
- A tap on the island for a **different** host keeps the old console.
- A failed reconnect hides behind the old screen: a failure still shows
  the cover with its reason.

## 6. Approaches

- **Grace window on the unlock (picked).** Keep the evaluated `LAContext`
  across the background with a timestamp; reuse it inside 5 minutes. Small,
  local, testable as a pure rule.
- **Face ID every time.** The owner's fallback if the grace is complex. It
  is not: kept as the behaviour after 5 minutes.
- **Keep the connection alive in the background.** Ruled out: iOS
  suspends the app; the research (Termius docs) finds no reliable way.

## 7. Calls made by the agent

- The window is 300 s: Apple's `LATouchIDAuthenticationMaximumAllowableReuseDuration`.
- Owner ruling (2026-11-25): the 5 minutes count only for a return. A new
  context keeps `touchIDAuthenticationAllowableReuseDuration = 10`, so a
  first open asks Face ID even soon after the phone was unlocked.
- Owner ruling (2026-11-25): a link that names a session for the host
  already open keeps the console and jumps to that session. The island
  shows the session that matters most (0006: a waiting one beats a burning
  one), so the tap goes where the island pointed. The console connects
  again with `--open <n>`; the screen and the unlock stay.
- Owner ruling (2026-11-26, after a device report: a return often showed
  "The connection to <host> closed"): a return never leaves the console for
  a short failure. iOS brings the network back after Pocket is active, and
  the 5-minute reuse took away the Face ID pause that hid it. On a kept
  screen a failed connect tries again after 1, 2, then 4 s, behind a brass
  line that breathes over the top. After 20 s the kept screen says
  `Can't reach <host>.` with `Try again`, with no new page. A changed host
  key or a refused key still opens the Reach screen. Keys typed meanwhile
  go once the console is back, up to 4 KB; a failure drops them.
- Owner ruling (2026-11-26, after a report: the watch always showed 100 %
  and a red bar while Claude had 76 % of its week left): an account that no
  session works for now does not lead the glance. The compact island and
  its minimal bar show the fullest window of the accounts with a busy
  session; with none, the host mark `▰`. The lock screen and the expanded
  island still list every account. An account with no week window (a
  Cursor month) says `resets Dec 17`, not `lasts the week`. Cursor at 100 %
  of its included usage is information, not `out`: a team plan can go on
  past it. Calls of this session, open to change.
- Owner report (2026-11-27): the glance still showed Cursor's month at
  100 % in red while its session worked. A window at 100 % while a session
  of its account works is spent: it stops nothing, never leads the glance
  and draws without warning. A full window with no session working stays a
  limit.
