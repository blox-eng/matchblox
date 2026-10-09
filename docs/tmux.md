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

## Learn tmux

- [Getting Started](https://github.com/tmux/tmux/wiki/Getting-Started), on
  the tmux wiki.
- `man tmux`, the full reference.
