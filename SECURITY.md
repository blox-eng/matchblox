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
- Secrets that reach a model provider, a log or the audit file.
- The install script or a release asset that does not match its checksum or
  provenance.

## Supported versions

matchblox is pre-1.0. Only the latest release receives fixes.

## The model

- The service runs as you, at the lowest CPU priority, and listens only on a
  Unix socket in a 0700 directory. Remote mode uses your own SSH. It opens no
  port.
- The console draws state. It never reads the machine and never runs a step
  itself. Every step is an argv list, shown before it runs.
- The stoker is off by default. In auto mode it runs only safe steps that are on
  your allowlist. Pane text goes to a provider only for panes you opt in, after
  secrets are removed. Each decision is in the audit log.
