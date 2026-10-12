# Limits

What does not work yet, and what to do until it does.

| Limit | What to do |
|---|---|
| The way back needs the console's own session | Run `matchblox` outside tmux. The tap on `◂ matchblox` also needs the tmux mouse: press `m` in the way back's preview. |
| The stoker compacts Claude Code only | Compact Codex and OpenCode sessions yourself: Sessions marks them `! compact`. |
| Codex context use needs Linux | On macOS, Codex shows "not measured". |
| Context use for Claude Code, Codex and OpenCode only | Other agents show "not measured", and still show in Sessions. |
| Waits of agents other than Claude Code come from their pane | They wait for you in Sessions once their pane is idle and its last lines ask. |
| Plan limits for Claude Code and Codex only | OpenCode's usage shows "not measured". |
| Ships reads GitHub only | A pull request on another forge does not show. |
| No week of Opus or Sonnet in usage | Claude Code's status line gives the 5-hour window and the week only. |
| tmux only | Run your agents in tmux. |
| No native Windows | Use Linux or macOS. |
| One host on a screen | `esc` goes back to Hosts. |

## Soon

- **Start agents from the console**
  ([#10](https://github.com/blox-eng/matchblox/issues/10)).
