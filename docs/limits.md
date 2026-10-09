# Limits

What does not work yet, and what to do until it does.

| Limit | What to do |
|---|---|
| No release binary yet | Install Go: the install script builds from source, here and on each host you connect. The first release: [#27](https://github.com/blox-eng/matchblox/issues/27). |
| The way back needs the console's own session | Run `matchblox` outside tmux. The tap on `◂ matchblox` also needs `set -g mouse on`. |
| The stoker compacts Claude Code only | Compact Codex and OpenCode sessions yourself: Sessions marks them `! compact`. |
| Codex context use needs Linux | On macOS, Codex shows "not measured". |
| Context use for Claude Code, Codex and OpenCode only | Other agents show "not measured", and still show in the queue and in Sessions. |
| Waits of agents other than Claude Code come from their pane | They reach the queue once their pane is idle and its last lines ask. |
| tmux only | Run your agents in tmux. |
| No native Windows | Use Linux or macOS. |
| One host on a screen | `esc` goes back to Hosts. |

## Soon

- **Start agents from the console**
  ([#10](https://github.com/blox-eng/matchblox/issues/10)).
