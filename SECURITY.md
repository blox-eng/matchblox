# Security

matchblox runs commands on your machine on your behalf, so security reports come
first.

## Reporting

Use [GitHub private vulnerability reporting](https://github.com/blox-eng/matchblox/security/advisories/new).
Do not open a public issue. Include the version (`matchblox version`), the OS,
how it ran (on the host, over SSH, remote mode), and the steps to reproduce.
We answer within 3 working days.

In scope:
- Anything that runs a step the person did not see, or a destructive step
  without its typed confirm.
- A guard that does not stop a step when its fact no longer holds.
- A shell string built from data, or text from a pane or a model that becomes a
  command.
- Secrets that reach a log or the console.
- The service socket, the hook command (`matchblox hook`), the ssh forced
  command, and the install script.
- A release asset that does not match its checksum or provenance.

## Supported versions

matchblox is pre-1.0. Only the latest release receives fixes.

## The model today

- matchblox runs as you, at the lowest CPU priority, and opens no port.
- Every step is an argv list, never a shell string, and is shown before it runs.
- A destructive step needs `x` and then a typed `y`. Its guard (the same
  process and start time, a worktree still clean and unused, a session still
  idle) is checked again just before it runs.
- Steps run with a time limit, no terminal prompts, and SSH in batch mode.
- The service listens on a Unix socket in a directory that only you can
  open. The console draws only what the service sends.
- Setup edits `~/.claude/settings.json` and the tmux config only after it
  shows the exact change, and keeps a backup.
- Remote mode uses your own SSH. A connected host gets a key that can start
  only the console's stream: no shell, no forwarding, a strict host-key
  check.
- matchblox sends no pane text to a model.
