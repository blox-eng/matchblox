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

Prefix `m` from any pane goes to the console. The console binds it on the
tmux server when it starts in tmux, in any session, in place of the tmux key
that marks a pane, and the way back that `matchblox setup` adds to your tmux
config binds it too. On a pane that runs a console, in any session, you stay on the pane. On any other pane it goes to
the session `matchblox` and the console's pane there, also from another
window of that session. With no `matchblox` session, it says `No matchblox
console here: run matchblox in a shell to open one`. A `bind m` of your own wins again when you source
your config, until the console starts again. From a jump, prefix `m` goes
back to the console and ends the jump.

tmux tells a console's pane by its command name, `matchblox`. A binary under
another name is not one to tmux: prefix `m` on its pane goes to the session
`matchblox`, and its key line says `tap back`, as a tap on the hint is the way
that works there. See [Remote hosts](remote.md#jump-into-a-session).

## Learn tmux

- [Getting Started](https://github.com/tmux/tmux/wiki/Getting-Started), on
  the tmux wiki.
- `man tmux`, the full reference.
