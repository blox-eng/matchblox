# 0005 — Remote mode (v0.1, #23)

A builder works on a laptop. The agents run on a host. From one console the
builder opens the console of any host and works it as if it were local.

This spec replaces the hosts door of 0003 §6 (item 4) and 0002 Task 4.1.

## 1. The experience

1. **The install asks.** The install script ends by starting `matchblox`.
   On the first run (no config file), the first screen asks: "Where do your
   agents run?" The rows are `this machine` and `another machine`.
   - `this machine`: the console of this machine, with the setup doors
     (0003 §6).
   - `another machine`: the hosts of `~/.ssh/config` (no patterns), and a
     row to type a host. The builder picks one. matchblox connects it (§3)
     and opens its console.
2. **Layers.** As in k9s, the console has two layers:
   - **Hosts** (the top layer): `this machine`, each host the builder
     added, and `+ add a host`. `Enter` opens the console of the selected
     row.
   - **Console** (one host): the tabs of DESIGN.md §6. `Esc`, with nothing
     else to cancel or clear, goes back to Hosts.
   The header names the layer: `▰ matchblox · ws-1`. On Hosts it is
   `▰ matchblox · hosts`.
3. **The start.** `matchblox` starts in the console of this machine when the
   builder added no host, else on Hosts. `matchblox ws-1` starts in the
   console of `ws-1`; `Esc` goes to Hosts.
4. **A host that cannot be reached.** The console says why, in one line, and
   shows the one action that fixes it (§3). There is no dead end.
5. **A dropped connection.** The console shows `connection lost`, keeps the
   last state, marks the host `stale` in the header, and tries again with
   backoff.

## 2. Where the hosts are kept

`~/.config/matchblox/hosts`: one host on each line, `#` starts a comment.
matchblox adds a line or removes one line. It never rewrites the file, so
the lines and comments of the builder keep their bytes. A host is a name
that `ssh` reads as a host (`ws-1`, `me@build.example.com`). A later kind
(#15) uses a scheme (`docker://…`); v0.1 reads only SSH names.

## 3. Connecting a host

The console's stream uses its own key, which can start only the matchblox
stream there. Everything interactive (a jump to a pane, a door's command)
uses the builder's own ssh login, as `ssh -t` would. The builder's own keys
and shell stay as they are.

1. **The key.** `~/.ssh/matchblox_ed25519`, made once with
   `ssh-keygen -t ed25519 -N "" -C matchblox`. It has no passphrase: the
   console runs with `BatchMode=yes`, and the host limits the key to the
   stream (§3.3).
2. **Connect** (`matchblox connect <host>`; in the console one action, shown
   as its exact command, after a typed `y`). It runs in this terminal with
   the builder's own ssh login, so a password prompt or a host-key prompt is
   the builder's own:
   `ssh -t <host> /bin/sh -c '…; curl -fsSL https://matchblox.sh | MATCHBLOX_NO_START=1 sh || exit 1; matchblox authorize "$1"' sh <public key>`.
   It always runs the verified install: an older matchblox does not know
   `authorize`. So the same action updates an older host.
3. **The gate.** `matchblox authorize` adds one line to the host's
   `~/.ssh/authorized_keys` (directory 0700, file 0600, a backup first):
   `restrict,pty,command="'<matchblox path>' gate" ssh-ed25519 AAAA… matchblox`.
   `restrict` turns off port, agent and X11 forwarding. `matchblox gate`
   starts the stream when `SSH_ORIGINAL_COMMAND` is exactly
   `matchblox serve --stdio`. Any other command, and a login with no
   command, gets one line on stderr and exit 126. The path may hold letters,
   digits, space and `._+@/-`.
4. **The console's SSH options** (the stream). `-i ~/.ssh/matchblox_ed25519`,
   `IdentitiesOnly=yes`, `IdentityAgent=none`, `BatchMode=yes`,
   `StrictHostKeyChecking=yes`, `ForwardAgent=no`,
   `ClearAllForwardings=yes`, `ControlMaster=no`, `ControlPath=none`,
   `ConnectTimeout=10`, `ServerAliveInterval=5`, `ServerAliveCountMax=3`.
   A jump or a door's command: `ssh -t -o ForwardAgent=no -o
   ClearAllForwardings=yes -- <host> /bin/sh -c 'PATH=…; exec "$@"' sh
   <argv>`. `/bin/sh` runs under every login shell (fish and csh too); the
   PATH puts `~/.local/bin`, `/opt/homebrew/bin` and `/usr/local/bin` first,
   which ssh's own PATH does not have. The console shows it as
   `ssh … <host> <argv>`.
5. **Why it fails, and the fix.**

   | ssh says | The console says | The action |
   |---|---|---|
   | no key, or `Permission denied` | `ws-1 is not connected` | Connect |
   | `matchblox: not found`, exit 127 | `matchblox is not installed on ws-1` | Connect |
   | an older wire in the hello | `ws-1 runs an older matchblox` | Connect |
   | `Host key verification failed` | `ws-1's host key is not known` | Connect (ssh asks) |
   | `REMOTE HOST IDENTIFICATION HAS CHANGED` | `ws-1's host key changed: check it, then ssh-keygen -R ws-1` | none, the builder decides; ⏎ tries again |
   | any other | the last line of ssh's stderr | try again |

## 4. Security (OWASP Top 10)

- **A01 Broken access control.** The matchblox key starts only the stream:
  no shell, no tmux client, no agent, no forwarding (tested against a real
  sshd). The stream is the console: it can answer the agents of the host
  and run the console's guarded actions. So a stolen matchblox key controls
  the agents of the hosts it reaches, as the console does. Keep it 0600, as
  any ssh key.
- **A03 Injection.** The console builds each remote command from argv and
  checks it before ssh wraps it. The remote shell gets argv as `"$@"`, and
  the public key as `$1`, never inside a script.
- **A05 Misconfiguration.** No agent or port forwarding, no shared control
  master. A strict host-key check after the first connect.
- **A07 Authentication.** Keys only for the stream. No password prompt in
  the console. A changed host key stops the connection.
- **A08 Integrity.** The host installs only a release that passes its
  checksum (the install script).
- **A09 Logging.** Each action runs on the host's service, which logs it as
  for a local console.

## 5. Not in v0.1

- Containers, Kubernetes, cloud hosts: #15 (a host kind with a scheme).
- A queue merged across hosts.
