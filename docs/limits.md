# Limits

matchblox says what it does not do yet. Each limit has its issue.

| Limit | Why, and what to do |
|---|---|
| No release binary yet | The install script builds from source, and needs Go, on this machine and on each host you connect. The first release comes with [#27](https://github.com/blox-eng/matchblox/issues/27). |
| The way back needs the console's own session | Run `matchblox` outside tmux. Inside tmux the console runs in place, and `prefix m` does not reach it. The tap on `◂ matchblox` also needs `set -g mouse on`. |
| Codex context use needs Linux | matchblox finds a Codex session through the files that the process holds open. macOS cannot list them, so Codex shows "not measured" there. |
| Other agents show "not measured" | Context use needs an adapter for the agent's files. Claude Code, Codex and OpenCode have one. Every agent in tmux still shows in the queue and in Sessions. |
| Waits of agents other than Claude Code can come late | They are read from the pane, not told through hooks. An agent is in the queue when its pane is idle and its last lines ask. |
| tmux only | matchblox finds agents in tmux panes. Other multiplexers are not supported. |
| No native Windows | Use Linux or macOS. |
| One host on a screen | The console shows one host at a time. `esc` goes back to Hosts. |

## Soon

- **Night mode** ([#24](https://github.com/blox-eng/matchblox/issues/24)):
  sessions that fill their context overnight compact themselves and leave a
  RESUME line for the morning.
- **Start agents from the console**
  ([#10](https://github.com/blox-eng/matchblox/issues/10)).
