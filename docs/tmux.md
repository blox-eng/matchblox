# tmux, the engine

matchblox stands on [tmux](https://github.com/tmux/tmux), a terminal
multiplexer: many terminals on one screen that keep running when you
detach, and that you attach again from any other terminal. tmux is free
software under the ISC license, for OpenBSD, FreeBSD, NetBSD, Linux, macOS
and Solaris.

Your agents run in tmux panes. matchblox finds them there, takes you to
them with `tmux switch-client`, and answers them with `tmux send-keys`.

## What you get from it

- **Agents that keep working when you leave.** Close the laptop: the
  agents, the matchblox service and the stoker go on.
- **The same console everywhere.** Run outside tmux, the console lives in
  the tmux session `matchblox`. The desk, a laptop and a phone all attach
  to it.

## The way back without config

When the console runs in tmux, a jump to a host opens a tmux session of its
own on your laptop. It brings its own way back, and it needs none of your
tmux config:

- Tap `◂ console` in the status line (tmux 3.4 or newer).
- Press the home key on the phone.
- Press your prefix twice, then `m` or `d`. This needs the `send-prefix`
  binding on your prefix key.

Outside tmux, a jump takes the terminal. Press your prefix once, then `m`
or `d`.

Prefix `m` from any pane needs the way back that `matchblox setup` adds
to your tmux config. See [Remote hosts](remote.md#jump-into-a-session).

## Learn tmux

- [Getting Started](https://github.com/tmux/tmux/wiki/Getting-Started), on
  the tmux wiki.
- `man tmux`, the full reference.
