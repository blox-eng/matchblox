# Host jumps that never collide (#214) — design

## Intent

A builder runs matchblox on a laptop and adds a host. Today, a jump into a
session on that host wraps matchblox inside matchblox: the host's tmux runs
inside the laptop's tmux, both have a `matchblox` session, a prefix m, a
`◂ matchblox` tap and the HomeKey, and the way back leads into the host
again. The jump also asks the builder's password on every open, in the raw
terminal.

After this work, a jump from any console to any host shows one tmux, has
one way back (to the console the builder came from), moves nobody else,
and asks for a login at most once — in the console, never in the raw
terminal. matchblox sets all of it up itself: the host needs no tmux
config, and the laptop needs none beyond what the console offers.

## Two ways to reach a host (owner, 2026-10-26)

**A. Visit.** A device sshes into host H and runs `matchblox` there (the
phone app, Termius, plain ssh). It gets H's one shared console in H's tmux
session `matchblox`. Every visitor sees the same console. This stays as it
is (design/0006-anywhere.md §6.2).

**B. Drive.** A builder runs matchblox on their own machine and opens H in
the Hosts layer. H streams its state; the builder jumps into its agent
panes. Each driver's view is its own. A driver never touches H's shared
console, never moves a visitor, and never moves another driver.

Two drivers that jump into the same agent pane both see it and can both
type into it: there is one agent process. That is accepted (owner).

## Goals

1. A jump in B shows exactly one tmux status line and one way back: to the
   driver's own console.
2. A jump in B never changes the current window of any other tmux client,
   on either machine. The active pane, zoom and size belong to a tmux
   window, so they stay shared; agents that matchblox starts have one pane
   per window, and split windows are out of scope (owner, 2026-10-26).
3. A jump in B asks for a login at most once per host, inside the TUI. The
   console recommends the most secure way to make it permanent and sets it
   up after a typed y.
4. A host answered by Tailscale SSH works, including Tailscale's check mode
   and a tailnet where Tailscale SSH is required.
5. Nothing on the host needs config, and an older matchblox on the host
   cannot break a jump.
6. A test with two real tmux servers guards goals 1 and 2.

## Non-goals

- matchblox streams panes itself (no tmux on the host side). Rejected: it
  makes matchblox a terminal multiplexer (Approaches, 3).
- Read-only or locked panes for a second driver.
- Split windows: a jump's pane selection and size are shared there.
- Password managers, storing a password in any form, mosh.
- Changing the gate or what the matchblox (stream) key may start.
- Mode A changes, beyond not being disturbed by B.

## Anti-goals (a failure even if jumps "work")

- A jump that moves another client's window.
- A keystroke or tap in a driver's jump that reaches the host's shared
  console.
- A password or passphrase on argv, in an env var, on disk or in a log.
- A weaker host-key check, or a key that matchblox generates without a
  passphrase for the builder's own login.
- An ssh prompt that draws in the raw terminal over or after the TUI.

## Constraints

- The stream keeps its locked ssh (`remote.Target.ssh`): its own key,
  `ControlMaster=no`, `BatchMode=yes`. Nothing here touches it.
- Jumps keep using the builder's own ssh login (design/0005 §3).
- Laptop: macOS or Linux, OpenSSH ≥ 8.4 (`SSH_ASKPASS_REQUIRE`). This
  machine has 9.6; macOS ships 9.x.
- tmux on both sides; the minimum version is set by the features below
  (left to the builder, see §8).
- Every remote command stays argv, quoted by `remote.Quote`, run by the
  `/bin/sh` of `onHost`, and checked by an allowlist before ssh wraps it.

## Approaches

1. **A see-through host tmux (picked, owner 2026-10-26).** The laptop opens
   each jump in its own local session; the host attaches the jump through a
   private grouped session with no status line, no prefix and no mouse.
   Uses tmux features only; covers all three collisions.
2. **Laptop side only.** A local session per host. It ends the loop, but the
   host's status line and `◂` tap still show inside, and `select-window` on
   the host still moves every client of that session. Half a fix.
3. **No host tmux for jumps.** matchblox streams the pane's bytes itself.
   No nesting ever, but weeks of work and a new attack surface.

## Design

### 1. The laptop side: one local session per host

When the console runs inside tmux, a jump never takes the console's pane.
It runs:

1. One tmux command list:
   `new-session -d -s <S> -- <ssh argv>` `;`
   `set-option -t =<S>: detach-on-destroy off` `;`
   `set-option -w -t =<S>: remain-on-exit failed` `;`
   the way back (§1.1) `;`
   `switch-client -t =<S>`.

`S` is `matchblox-<host>-<8 hex>`, random per jump, with `.` and `:` of
the host replaced by `_` (tmux renames them, and `=` targets match the
stored name). The laptop's `matchblox` session can have several clients
(the laptop's terminal and a phone visiting the laptop), so a jump never
kills or replaces another one: each S lives as long as its ssh. A dead S
(a failed ssh) closes on any key. Verified on tmux 3.4,
2026-10-26:

- A session target in `set-option` must end in `:` (`=S:`); `=S` alone
  resolves as a window and fails.
- A failing command does not stop the rest of the list.
- `--` keeps the ssh argv as argv; a `';'` inside the quoted remote line
  stays one argument.
- When ssh exits 0 (the builder left the agent), the session goes and the
  client returns to the console (detach-on-destroy off). When ssh fails,
  the pane stays dead with its status for the builder to read. **Open:** in
  the probe, ssh's own error line did not survive in the dead pane; the
  builder makes the error readable (§3 makes most failures happen before
  the jump anyway).

**Outside tmux** the console keeps today's path: it gives the terminal to
the ssh argv and comes back when it exits. There is no outer tmux to
collide with.

#### 1.1 The way back without config

The way back must not depend on the "Add the way back" door. Session S
gets its own key table (`key-table` is a session option) bound at jump
time on the laptop's server:

- the HomeKey user key → `switch-client -t =matchblox` (the console's own
  session; when the console runs in another session of the builder, that
  one);
- a click on the status range `matchblox` → the same;
- a status-left that shows `◂ console` as that range.

`mouse` is set on S only, so the tap works without turning the mouse on
for the builder's other sessions. prefix m keeps working when the door
added it. Keys not in S's table go to the pane, as today.

### 2. The host side: a see-through grouped session

The jump's remote command no longer attaches to the agent's session. In one
tmux command list on H, it:

1. binds two tables on H's server (idempotent), named for the laptop's
   prefix key P (the console reads it with `show-options -gv prefix`):
   in `matchblox-jump-<P>`, P switches to `matchblox-jump-<P>-prefix`;
   there, `d` and `m` both detach the client. So the keyboard way back is
   P P m (or d) whatever the builder's prefix is. Drivers with different
   prefixes use different tables.
2. `new-session -d -t <the pane's session> -s <G>`: a session grouped with
   the agent's. Grouped sessions share windows and panes, and each has its
   own current window.
3. sets on G: `destroy-unattached on`, `status off` (inside an outer tmux;
   outside, a one-line status that says `◂ console: <P> m`), `prefix None`,
   `key-table matchblox-jump-<P>`, `mouse off`.
4. selects the agent's window in G only, and its pane (shared, Goal 2).
5. `attach-session -t =<G>`.

`G` is `matchblox-jump-<8 hex>`, random per jump, so two drivers never
share one. Its name matches `^matchblox-jump-[0-9a-f]{8}$`, checked by the
allowlist.

The result:

- Inside an outer tmux, the outer catches C-b; the inner sees no prefix
  and no root bindings, so nothing reaches H's `matchblox` session. P P
  then `m` or `d` detaches the inner client, ssh exits 0, and
  §1 brings the builder back to the console.
- Outside tmux, P m (or d) leaves the jump and the console comes back.
- Other clients of the agent's session keep their window.
- G goes away when its client detaches or the ssh drops.

**Other shapes.** "Open a shell there" (`tmux new-window -c <dir>`) on a
host opens in a private session of its own, `new-session -d -s <G> -c
<dir>` with the same options as above, then attaches it: a bare
`new-window` on H could land in the shared `matchblox` session and move
its visitors. Every other tmux shape is refused for hosts; `NavArgv` goes.

H's service must not show G as a session of its own: the sampler skips
sessions named `matchblox-jump-*`, and the laptop's sampler skips
`matchblox-<host>`.

The console builds the whole command on the laptop from plain tmux
commands (my call). An older matchblox on H cannot break a jump, and
nothing on H needs an update for it. A new allowlist (`JumpAllowed`)
replaces `NavAllowed` for this shape.

### 3. The login: once, in the TUI, secure first

#### 3.1 One connection per host

Jumps to a host share one ssh master on the builder's own login, owned by
matchblox: `ControlMaster=auto`, `ControlPath=~/.cache/matchblox/ssh/%C`
(directory 0700), `ControlPersist=10m`. The login happens once. Each jump
then opens a channel on that master: no prompt, and faster. This master is
never used for the stream (§Constraints).

#### 3.2 The check before a jump

When the builder opens a host's console, the console checks the builder's
own login in the background, with the master options above and
`BatchMode=yes`: `ssh … <host> true`. The result is one of:

| Result | What the console shows |
|---|---|
| ok | nothing; jumps are ready |
| Tailscale SSH, check mode | §4 |
| a password or keyboard-interactive login | the ladder (§3.3) |
| host key unknown or changed | today's host-key screens |
| no route, refused, timeout | why, and "try again" |

The check is a read: it changes nothing on either machine. A jump started
before the check is done waits for it.

#### 3.3 The ladder (secure first)

When the login needs a password, the console offers the first step that
applies, in this order, each after a typed y, each showing exactly what it
runs and which key (with fingerprint):

1. A key the builder's ssh-agent (or macOS Keychain) holds, preferring a
   security key (`sk-ssh-ed25519`), then `ssh-ed25519`: install its public
   key on the host.
2. A key in `~/.ssh` (`id_ed25519_sk`, `id_ed25519`, `id_ecdsa_sk`, then
   others): install it. If it has no passphrase (`ssh-keygen -y -P ""`
   succeeds), say so and give the fix (`ssh-keygen -p -f <key>`); still
   allowed.
3. No key: create `~/.ssh/id_ed25519` **with a passphrase** typed in the
   TUI, add it to the agent (macOS: the Keychain), then install it.
4. "Not now": each new master asks the password in the TUI; the master
   keeps it to once per 10 minutes.

**Install** is one command on the host, through the builder's login, with
the password typed in the TUI: create `~/.ssh` (0700) and
`authorized_keys` (0600) if needed, and append the public key only if the
exact line is absent. The key line is checked by a strict pattern before it
is quoted (as `ValidPubKey` does). On macOS the console also offers
`AddKeysToAgent yes` and `UseKeychain yes` for that host in
`~/.ssh/config`, as a shown diff after a typed y (as the way-back door
does).

#### 3.4 Prompts in the TUI (askpass)

Every ssh and ssh-add the console starts for the builder's login runs with
`SSH_ASKPASS=<this matchblox> askpass`, `SSH_ASKPASS_REQUIRE=force` and
`MATCHBLOX_ASKPASS=<socket>`. `matchblox askpass` sends ssh's prompt to the
console over that socket and prints the answer to ssh.

- The socket is per console run, in a 0700 directory, mode 0600, with a
  random token that the helper must present.
- The console shows the prompt with control characters removed (a
  keyboard-interactive prompt comes from the server), in a masked field,
  with the host's name. Esc cancels the ssh.
- A yes/no prompt (a new host key) shows the fingerprint and needs a
  typed `yes`.
- The answer is never logged or stored; the console drops it after it is
  sent.

This reverses one line of design/0005 §4 (A07: "No password prompt in the
console") for the builder's own login only (owner, 2026-10-26). The
stream stays keys-only.

### 4. Tailscale SSH

The check (§3.2) runs ssh with `-v` and reads the server's version line.
When the host is answered by Tailscale SSH:

- **The ladder is skipped.** Tailscale SSH authenticates by tailnet
  identity and ignores `authorized_keys`, so installing a key does nothing.
- **Check mode.** Tailscale prints a URL and waits. The console shows the
  URL in the TUI (an OSC 8 link, and a key that copies it), and waits for
  the check, with a time limit and esc to cancel. The master (§3.1) then
  serves the jumps.
- **Denied by policy.** The console shows Tailscale's reason and points to
  the tailnet's SSH rules (docs/remote.md).
- **The stream is not gated there** (#185): the Hosts row shows that, with
  the #185 message. The secure way, shown first: `tailscale set
  --ssh=false` on the host, so OpenSSH answers and the gate holds. When the
  tailnet requires Tailscale SSH, jumps work as above and the warning stays
  on the row: the tailnet's rules are the gate then.

The exact server version string and check-mode output are verified against
a real Tailscale SSH host before the parser is written; tests use recorded
fixtures.

### 5. What the console learns from the host

`proto.Hello` gets `Tmux string` (the host's tmux version; empty when
there is none), `omitempty`. An older host sends nothing, and the console
treats that as "unknown": it tries the jump. A host without tmux, or older
than the minimum, shows the existing tmux door before any jump.

## Security (OWASP)

- **A01.** No new power for the stream key. Jumps keep the builder's own
  login. The grouped session takes nothing from other clients.
- **A02.** No password, passphrase or key material on argv, in env, on
  disk or in logs. The askpass socket is 0600 with a token.
- **A03.** Every remote command is argv: quoted once, checked by
  `JumpAllowed` (pane id, session names by pattern, no `#` or `;` inside
  values). Prompt text is sanitized before it draws.
- **A05.** The control master lives in a 0700 directory and is only for the
  builder's own login. Host-key checking is the builder's own, never
  loosened.
- **A07.** Keys over passwords, and security keys first. matchblox never
  generates a passphrase-less key for the builder.

## Tests (the guards)

1. **Two tmux servers** (`-L outer`, `-L inner`; ssh replaced by a fake that
   runs the remote line under `/bin/sh` against `-L inner`). They assert:
   - one status line;
   - the HomeKey and the `◂` tap in S return to the outer home session;
   - P P m detaches the inner client, and the outer returns home, with
     P = C-b and with P = C-a;
   - two outer clients jump to the same host at once, and neither moves;
   - a second inner client on the agent's session keeps its window;
   - G and S are gone after the jump ends;
   - a failing ssh leaves a readable dead pane.
   This is the guard for #214.
2. Exact argv tests for §1 and §2, `JumpAllowed`, and the name rules.
3. Check classification from recorded stderr: OpenSSH denied, password,
   Tailscale check URL, Tailscale policy deny, host key unknown or changed.
4. The ladder's order from fake agent and `~/.ssh` contents.
5. The askpass protocol: token refused, prompt sanitized, nothing logged.
6. Against the real sshd of `sshd_test.go`: password via askpass opens the
   master; a second ssh reuses it with no prompt.

## Done when

From the laptop console in tmux, with ws-1 added as a host:

- opening any session shows one status line;
- the first open asks one time, in the TUI, and offers the key; after a y,
  no open asks again;
- prefix m, the `◂` tap or prefix prefix m returns to the laptop's console;
- the phone on ws-1's shared console is not moved;
- a second laptop driving ws-1 at the same time is not moved;
- a Tailscale SSH host in check mode shows its URL in the TUI.

docs/remote.md and the CHANGELOG say all of this.

## Calls made

- owner, 2026-10-26: the two modes (A shared, B private); approach 1;
  login A with secure ways first; Tailscale SSH, including when it is
  required; two drivers in one pane is fine; a password prompt in the TUI
  for the builder's login; Goal 2 holds per window, split windows out of
  scope.
- Mine: the jump command is built on the laptop (version skew); a local
  session per jump, never one shared per host (the laptop's console can
  have several clients); the keyboard way back uses the laptop's own
  prefix; "open a shell there" gets a private session on the host and
  every other shape is refused there; `matchblox-<host>-<hex>` and
  `matchblox-jump-<hex>` names; a 10-minute master; the per-session key
  tables; `Hello.Tmux` with `omitempty`; the check runs when a host's
  console opens.

## Left to the builder

- The minimum tmux version on each side, from the features used.
- Making ssh's error readable in a dead pane (§1).
- How to address the agent's window inside G (by window id, from the pane).
- The status-line copy, the TUI field layout, the check's time limits.
- How the console learns the jump ended outside tmux (today's
  ExecProcess return is enough).
