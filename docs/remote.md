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
- A jump to a pane uses your own ssh login.

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

## Tailscale SSH and the gate

Laptop and phone keys are held to `matchblox gate` only when OpenSSH
answers. If Tailscale SSH is on, the host's MagicDNS name and Tailscale
IPs on port 22 are answered by tailscaled, which ignores
`authorized_keys` and ForcedCommand. Keep the gate on every path with
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
