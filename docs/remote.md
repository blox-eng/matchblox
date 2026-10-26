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

## Open the console of a host

```sh
matchblox ws-1
```

`esc` goes back to Hosts: `this machine`, each host you added, and
`+ add a host`. `Enter` opens a row.

- The header names the host. While the connection is lost, it says `stale`.
- A host that the console cannot reach says why, and shows the connect.
- A jump to a pane uses your own ssh login. See [Jump into a session](#jump-into-a-session).

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

To go back to the console, use one of these:

- Tap `◂ console` in the status line. This needs tmux 3.4 or newer.
- Press the home key on the phone.
- Press prefix `m`, if you added the way back (see [tmux](tmux.md)).
- Press your prefix twice, then `m` or `d`. The host side takes the same
  prefix as your laptop.

Leaving ends the jump. Nothing stays on your laptop or on the host. If ssh
fails, the jump waits and says `ssh exited <n>. Press Enter to go back.`

A jump needs tmux 3.2 or newer on your laptop and on the host. If tmux is
older, the console says so and does not jump.

## Log in once

The console opens one ssh connection to a host and shares it for 10
minutes. The jumps in that time ask nothing. The connection lives in
`/tmp/matchblox-<uid>/ssh`. This directory is yours only (mode 0700). If
another user owns it, or others can enter it, the console refuses to use
it. It says why, and the fix is to remove it and open the host again.

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
