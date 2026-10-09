# Agents

matchblox finds every agent that runs in a tmux pane. Start each agent in
a tmux session. Each command gives the console row under it.

**Claude Code**

```sh
tmux new -s app claude
```
```
✦ busy         app        app:1.1   ██████░░░░  62%
```

**Codex**

```sh
tmux new -s api codex
```
```
╿ asks   1m    codex      api:1.1   █████░░░░░  55%   ❋ you@example.com · Plus
```

**OpenCode**

```sh
tmux new -s web opencode
```
```
✦ busy         opencode   web:1.1   ███░░░░░░░  32%   ▣ API key
```

**Any other agent**, for example aider

```sh
tmux new -s docs aider
```
```
╿ done   4m    aider      docs:1.1  not measured
```

The first run writes the agents it finds to `agents` in the
[config](config.md). Add any other agent there by its command.

## Context use

Claude Code, Codex and OpenCode show their context use, read from the
agent's own files: the transcript, the rollout, the session storage.
Before the first turn a session is "fresh". An agent without readable files
shows "not measured".

The window comes from the model that the agent reports. To change it, set
`[sessions.windows]` in the config.

## The account

On a wide screen, Sessions shows the account of each session: the
provider's mark, then the email and plan, or "API key". Only those two
fields are read from the agent's auth files; no token reaches the console.

## How an agent tells that it waits

Claude Code tells its waits through hooks: the setup step "Add the queue
hooks" adds them. Every other agent is read from its pane: busy while the
pane changes, waiting when it is idle and its last lines ask.

An approval menu (Codex, OpenCode) shows as `asks`. Answer it in its pane,
where `Enter` takes the selected choice.

See [Limits](limits.md) for what is not measured yet.
