# Remote hosts

Your agents can run on another machine. matchblox runs there as a service.
Your console connects to it over SSH.

## Connect a host once

The first run asks where your agents run. Pick "another machine", then a
host from `~/.ssh/config`, or type one. Or connect it yourself:

```sh
matchblox connect ws-1
```

This uses your own ssh login one time. It runs the install script on
`ws-1`, and adds a key that can start only the console's stream there: no
shell, no forwarding.

A connect from Hosts runs in the background. If ssh asks something (a new
host key, a password, a key's passphrase), the question shows in the
console's field, as at a jump. A login by key, agent or Tailscale SSH asks
nothing. `matchblox connect` typed in a shell asks in that terminal.

## See every host in one console

```sh
matchblox
```

With hosts added, this opens one console over the ticked hosts. Sessions
is one queue. A wait on any host is at the top. `matchblox ws-1` opens
`ws-1` alone.

The home host is the first ticked host. It is `this machine`, unless you
hide `this machine`. Then it is the first ticked host after it.

### Tick the hosts

`esc` goes back to Hosts: `this machine`, each host you added, and
`+ add a host`. The header says `HOSTS · 3 · 2 SHOWN`.

- `space` ticks or unticks the selected host.
- matchblox keeps the hidden hosts in `~/.config/matchblox/hidden`, one per
  line. The line `this machine` hides your laptop.
- A new host is ticked.
- `Enter` opens one console of the ticked hosts.
- With no host ticked, `Enter` opens the selected host alone. The key line
  says `No host is ticked: <host> opens alone.`
- A ticked host has a live note, such as `5 sessions · 1 waits`.
- A hidden host has the note `hidden`. It has no connection, so matchblox
  has no count for it.
- `esc` and `Enter` again keep the hosts that stay ticked. They keep the
  selection and the lanes too.

### Read the rows

- The header says `3 hosts`.
- When a host is hidden, the header adds `· 1 hidden`.
- A row names its host after the session: `train-loop gpu-1`.
- A row with an account says `api-auth ws-1 · work`.
- A long host name ends in `…`. The detail shows it whole.
- `n` and the ghost row start a session on the host of the selected row.
  With no row selected, they start it on the home host.

### A host is lost

Each host connects, reconnects and fails on its own. One lost host does not
stop the others.

- A lost host keeps its rows, in their groups.
- A reconnect of a few seconds changes nothing on the screen.
- After 30 seconds, the header says `· gpu-1 stale`.
- After 30 seconds, each row of that host says `stale` and shows the age
  since the host last spoke.
- The detail and the dock say `stale · as of 14:02`.
- The key line names the lost host: `gpu-1: connection lost. Tries again in
  8s.`
- With two lost hosts, it says `2 hosts lost: gpu-1, ws-1. Tries again in
  8s.`
- A host that the console cannot reach is one row at the end of the list.
  The row shows its fix: `gpu-1  is not connected  ⏎ connect`.
- `Enter` on that row runs the connect.
- The other hosts keep going.

### Acts go to the row's host

- Each key on a row goes to the host of that row. This holds for `a`,
  `Enter`, `x`, `r`, park, pick and the peek.
- Two hosts can have the same pane id. The console never mixes them.
- The console refuses an act on a lost host: `gpu-1 is not connected. It
  tries again in 8s.`
- While the console dials a host for the first time, the text is
  `gpu-1 is not connected.`
- If acts to a lost host get no answer, the console says `ws-1: 1 act has
  no answer`.
- A jump to a lost host works. It uses your own ssh, not the stream.

### The dock and the tabs

- Ships and Panes list the rows of every ticked host. Each row names its
  host.
- Git names the host on each repository heading, not on each worktree row.
- Machine, History and Stoker show one section for each ticked host, in
  Hosts order. Each section has the name of its host.
- The dock shows one usage row for each account label.
- Each host adds a short cell to that row, in Hosts order: `work  5h  ws-1
  58% · gpu-1 61%`.
- matchblox never adds or averages the readings of two hosts.
- A short line drops whole cells and ends in ` …`.
- The dock shows one machine line for each host.

### The stoker, for each host

- The stoker is one for each host. The header counts the hosts where it
  runs: `stoker 2/3`.
- A key on a row asks about the stoker of that row's host.
- `y` rests that stoker only.
- `s` starts the stoker of the selected row's host.
- The Stoker tab shows the section of another host for reading only.
- The dock claims the stoker of the home host.

### What stays with the home host

Recs, the doors, clones, the scan question and pairing stay with the home
host. They are not per host in this version.

### The ssh login of each host

- Each host keeps its own ssh login: its connection, its background check,
  its key and its Tailscale link.
- The checks run at open and never ask.
- A question comes only at a jump.
- The field shows one question at a time and names its host.

### Older hosts and keys

A host that runs an older matchblox works in the combined console. No
service changes, and matchblox adds no key between hosts.

A host that the console cannot reach says why, and shows the connect. A
jump to a pane uses your own ssh login. See
[Jump into a session](#jump-into-a-session).

## See and remove the devices of a host

```sh
matchblox devices
```

This prints one row for each key that opens this host: `phone` or `laptop`,
its name and its fingerprint. Phones come first. A laptop key comes from
`matchblox connect`, and its name is the comment of the key: `matchblox`. A
phone key comes from `matchblox pair`, and its name is the name of the phone.

To remove one, give its name or its fingerprint:

```sh
matchblox devices rm <name>
```

matchblox prints `Removed <name>. Backup: <path>`. The backup is the
`authorized_keys` file as it was. If two devices have the same name (for
example, two laptops that are both called `matchblox`), matchblox says so and
removes none. Run the command again with the fingerprint. If no device has the
name, matchblox tells you to run `matchblox devices` and to give a name or a
fingerprint from that list.

## Keep the key private

The stream can answer your agents, as the console does. Keep
`~/.ssh/matchblox_ed25519` as private as any ssh key. The host key check is
strict.

## Jump into a session

`Enter` on a session of a host jumps into it. The jump uses your own ssh
login, never the key of the stream.

In tmux, the jump opens in its own tmux session on your laptop:
`matchblox-<host>-<8 hex digits>`. On the host, it opens a private session
that shares the windows of the agent's session. Nobody else who looks at
the host moves. You see one status line.

Outside tmux, the jump takes the terminal and gives it back when it ends.

To go back to the console in tmux, use one of these:

- Tap `◂ console` in the status line. This needs tmux 3.4 or newer.
- Press the home key on the phone.
- Press prefix `m`, if you added the way back (see [tmux](tmux.md)).
- Press your prefix twice, then `m` or `d`. The tmux of your laptop takes
  the first prefix and sends the second to the host. This needs the
  `send-prefix` binding on the prefix key. tmux has it for `C-b`. With
  another prefix, such as `C-a`, run `bind-key C-a send-prefix`.
  A prefix is one key with `C-`, `M-` or `C-M-` (`C-a`, `C-Space`, a
  backtick). With another prefix, the host's keys are `C-b`, and the
  console says so when you open the host.

Outside tmux, only the keys of the host work: press your prefix once, then
`m` or `d`. The status line of the host shows them. The tap, the home key
and prefix `m` are for a jump in tmux.

Every way back ends the jump: the tap, the home key, prefix `m`, the keys
of the host, and any other move of your client away from it. Nothing stays
on your laptop or on the host. If ssh fails, the jump waits and says
`ssh exited <n>. Press Enter to go back.`

A jump needs tmux 3.2 or newer on your laptop and on the host. If tmux is
older, the console says so and does not jump.

## Log in once

The console opens one ssh connection to a host and shares it for 10
minutes. The jumps in that time ask nothing. The connection lives in
`$XDG_RUNTIME_DIR/matchblox/ssh` when your system has that directory, and
in `/tmp/matchblox-<uid>/ssh` when it does not. This directory is yours
only (mode 0700). If another user made `/tmp/matchblox-<uid>` first, the
console makes a new directory of its own next to it. If others can enter
the directory, the console refuses to use it, says why, and the fix is to
remove it and open the host again.

The connection ignores what your `~/.ssh/config` forwards for the host
(ports, the agent, X11) and its `RemoteCommand`. A connection on a network
that went away (the laptop slept) ends in 15 seconds, and the next jump
opens a new one.

When you open the console of a host, it checks your login in the
background. Before a jump, the console opens the connection first. If ssh
asks something, the question shows in a field in the console, and your
answer shows as dots. It never shows in the raw terminal. `esc` ends the
ssh.

- **A password.** Type it in the field. matchblox does not store it.
- **A new host key.** The field shows ssh's question. Check the
  fingerprint, then type `yes` or `no`. A short answer does not count.
- **A changed host key.** The console refuses the jump. Run
  `ssh-keygen -R <host>` only if you trust the change.

### Put your key on a host

If a host asks your password, the first jump offers to put your key there.
The offer shows the exact commands, and asks for a typed `y`. Any other key
means not now: the jump goes on, and the console asks your password at the
jump. `esc` cancels the jump. matchblox offers once for each console.

matchblox takes the first key that applies, the most secure first:

1. A security key in your ssh agent, then an ed25519 key in your agent.
2. A key file in `~/.ssh`: `id_ed25519_sk`, `id_ed25519`, `id_ecdsa_sk`,
   `id_ecdsa`, `id_rsa`. Each needs its `.pub` file.
3. A new ed25519 key at `~/.ssh/id_ed25519`, locked by a passphrase that you
   type in the field. matchblox adds it to your agent. An empty passphrase
   is refused: matchblox removes the key it made and installs nothing.

If a key file has no passphrase, the offer says so and shows the command to
lock it.

The install runs one command on the host, through your own login. It makes
`~/.ssh` and `authorized_keys` if they are not there. It adds a missing
newline to the last line, so the new key does not join it. It adds the key
only if it is not there yet. It never truncates or rewrites
`authorized_keys`: it only appends. After the install, the console checks
that your key logs in. If the key needs its passphrase, the jump asks it in
the field.

On macOS, the console then offers to add `AddKeysToAgent yes` and
`UseKeychain yes` for the host to `~/.ssh/config`. It shows the change,
and asks for a typed `y`. The Keychain then keeps the passphrase of the key.
`UseKeychain` is Apple's own: an OpenSSH from Homebrew stops at it, and
then no ssh runs. So the lines start with `Match all` and
`IgnoreUnknown UseKeychain`, and the console writes nothing that the
`ssh` on your PATH cannot read.

A host that takes keys only, and none of yours, gets no offer. The console
says to add your public key to `~/.ssh/authorized_keys` on the host another
way, then to open the host again.

## Tailscale SSH

Tailscale SSH can answer on port 22 of a host instead of OpenSSH.
matchblox knows it from the server's banner. A jump works on such a host.

- **Check mode.** Tailscale can ask for a check in a browser. The console
  shows the link in the field. Open it in a browser and wait. `c` copies
  the link. `esc` cancels the ssh, and the next jump asks again.
- **A policy that denies you.** The console says that Tailscale SSH refuses
  you, gives the reason, and points here. Change the SSH rules of your
  tailnet.
- **No key offer.** Tailscale SSH does not read `authorized_keys`, so the
  console does not offer to put a key there.

Laptop and phone keys are held to `matchblox gate` only when OpenSSH
answers. If Tailscale SSH is on, the host's MagicDNS name and Tailscale
IPs on port 22 are answered by tailscaled, which ignores
`authorized_keys` and ForcedCommand. The Hosts row of such a host warns
you of this. Keep the gate on every path with
`tailscale set --ssh=false`, or reach OpenSSH on another address or port.
Tailscale ACLs can limit Tailscale SSH; they do not restore the gate.
See [The phone](phone.md) for the pair preflight and the same warning on
`serve`, `status`, and `setup`.

## For scripts

```sh
matchblox status --text
```

This prints the state of the service as a short summary. Without `--text`
it prints JSON.
