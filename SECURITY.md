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
- Secrets that reach a log, or (from v0.1) a model provider or the audit file.
- (From v0.1) the install script, or a release asset that does not match its
  checksum or provenance.

## Supported versions

matchblox is pre-1.0. Only the latest release receives fixes.

## The model today

- matchblox runs as you, at the lowest CPU priority, and opens no port.
- Every step is an argv list, never a shell string, and is shown before it runs.
- A destructive step needs `x` and then a typed `y`. Its guard (the same
  process and start time, a worktree still clean and unused, a session still
  idle) is checked again just before it runs.
- Steps run with a time limit, no terminal prompts, and SSH in batch mode.

## What v0.1 adds

The design ([design/0001-v0.1.md](design/0001-v0.1.md) §4, §8, §9) adds a
service on a Unix socket in a 0700 directory, remote mode over your own SSH, and
the stoker: off by default, safe allowlisted steps only in auto mode, pane text
only for panes you opt in after secrets are removed, and an audit log. This
section moves up as each part ships.
