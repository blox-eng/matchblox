# Remote hosts

Your agents can run on another machine. matchblox runs there as a service.
Your console connects to it over SSH.

## Connect a host once

The first run asks where your agents run. Pick "another machine", then a
host from `~/.ssh/config`, or type one. Or connect it yourself:

```sh
matchblox connect ws-1
```

This uses your own ssh login one time. It installs the verified matchblox
on `ws-1`. It adds a key that can start only the console's stream there: no
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

## Keep the key private

The stream can answer your agents, as the console does. Keep
`~/.ssh/matchblox_ed25519` as private as any ssh key. The host key check is
strict.

## For scripts

```sh
matchblox status --text
```

This prints the state of the service as a short summary. Without `--text`
it prints JSON.
