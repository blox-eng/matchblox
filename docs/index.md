# matchblox

One terminal console for every AI coding agent you run in tmux. It shows
which agent needs you, what loads the machine, the state of every git
checkout, and the one safe action that resolves each thing. It works on the
host, over SSH, and from a phone.

![The matchblox board on a demo machine: four columns, NEEDS YOU, WORKING, SHIPPING and QUIET. A digit marks an answer from the agent's own options, the same digit sends it, and the card strikes into WORKING. p and a typed y merge a pull request. A tap hides the WORKING column and brings it back, and g shows one chain](assets/replay.svg)

## Learn it in five minutes

1. [Get started](start.md): install, the first run and its setup steps.
2. [The console](console.md): the board and its four columns, the card and its peek, the live view, the strip, and Machine.
3. [Keys](keys.md): every key, on one page.
4. [The phone](phone.md): the console as the first screen of an SSH app.

Then, when you need them:

- [The stoker](stoker.md): sessions that compact themselves while you sleep, and an agent that takes your orders.
- [Your ways](ways.md): the standing rules and recipes the stoker follows.
- [Ships](ships.md): the pull requests your sessions drive, from open to live, on their cards.
- [Agents](agents.md): Claude Code, Codex, OpenCode, Cursor and any other agent; accounts for Claude Code, Codex and Cursor.
- [Remote hosts](remote.md): agents on another machine.
- [Config](config.md): thresholds and agents.
- [Safety](safety.md): what matchblox does and does not do.
- [Limits](limits.md): what does not work yet.
- [tmux](tmux.md): the engine under matchblox.

Questions go to [Discord](https://discord.gg/tYxBUpGfX3). Bugs and ideas go
to [GitHub issues](https://github.com/blox-eng/matchblox/issues).
