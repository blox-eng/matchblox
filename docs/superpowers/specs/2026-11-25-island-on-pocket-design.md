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
  A new context also sets `touchIDAuthenticationAllowableReuseDuration = 300`,
  so a Face ID unlock of the phone in the last 5 minutes (from the lock
  screen) opens the key with no second prompt.
- A link that names a session for the host already open keeps the console
  as it is. The named session opens only when no console of that host is
  open (cold start, or another host).
