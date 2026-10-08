# Config

matchblox works without a config. The first run writes
`~/.config/matchblox/config.toml` with goals for this machine: `load1_over`
is the number of cores, and the agents it finds. Every key is optional.

The service reads the config when it starts. To apply a change, stop the
service and run `matchblox` again. The lock file next to the service socket
holds its pid. With `XDG_RUNTIME_DIR` set (most Linux systems):

```sh
kill "$(cat "$XDG_RUNTIME_DIR/matchblox/matchblox.sock.lock")"
matchblox
```

Without it (macOS), the lock is in the temp directory:

```sh
kill "$(cat "${TMPDIR:-/tmp}/matchblox-$(id -u)/matchblox.sock.lock")"
matchblox
```

To use another file, give `matchblox --config <file>`.

## Every key

```toml
--8<-- "config.example.toml"
```
