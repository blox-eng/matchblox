# Safety

matchblox runs commands on your machine for you. These rules hold for every
command.

- It reads, and does nothing to your agents, processes or checkouts by
  itself. Every action starts from a key. The one exception is yours:
  `[hooks] alert` in the config runs the command you name on each alert.
- Every action shows its exact command before it runs.
- `Enter` never runs a destructive action. That needs `x`, then a typed
  `y`.
- Each step checks its facts again just before it runs: the same process
  and start time, a worktree still clean and unused, a session still idle.
  If a fact changed, the step does not run.
- Two consoles that confirm the same action run it once.
- Each step is a list of arguments, never a shell string built from data.
- matchblox runs as you, at the lowest CPU priority. It opens no network
  port. The service socket is in a directory that only you can open.
- It sends no pane text to a model.
- A setup step that edits a file (`~/.claude/settings.json`,
  `~/.tmux.conf`) shows the exact change first and keeps a backup.
- A remote host's stream uses its own key. That key can start only the
  stream, with a strict host key check and nothing forwarded.

To report a security problem, see
[SECURITY.md](https://github.com/blox-eng/matchblox/blob/main/SECURITY.md).
