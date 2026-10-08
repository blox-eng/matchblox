# tmux, the engine

matchblox stands on [tmux](https://github.com/tmux/tmux). tmux is a terminal
multiplexer: it runs many terminals from one screen. You can detach them,
they keep running in the background, and you can attach them again to a
different terminal. tmux is free software under the ISC license, for
OpenBSD, FreeBSD, NetBSD, Linux, macOS and Solaris.

Your agents run in tmux panes. matchblox finds them there, jumps to them
with `tmux switch-client`, and answers them with `tmux send-keys`. Without
tmux, matchblox has nothing to show.

## What matchblox takes from tmux

- **A server that keeps running, and clients that come and go.** The tmux
  server keeps your sessions when no terminal is attached. The matchblox
  service keeps sampling the machine when no console is open, and every
  console is a client of it.
- **Attach from any terminal.** A tmux session follows you from the desk to
  a laptop to a phone. The console does too: it lives in the tmux session
  `matchblox`, and every connection attaches to it.
- **One reference.** tmux has one manual page. matchblox has these docs,
  and a test holds every key and command in them to the binary.
- **A small example config.** tmux ships `example_tmux.conf`. matchblox
  ships [config.example.toml](config.md), and works without a config.

## Learn tmux

- [Getting Started](https://github.com/tmux/tmux/wiki/Getting-Started), on
  the tmux wiki.
- `man tmux`, the full reference.
