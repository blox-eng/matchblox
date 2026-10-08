# Get started

You need Linux or macOS, and tmux. The first run can install tmux for you.

## 1. Install

```sh
curl -fsSL https://matchblox.sh | sh
```

Read the script first at [matchblox.sh](https://matchblox.sh/install.sh).
Until the first release, the script builds matchblox from source, and that
needs Go. The same build with Go:

```sh
go install github.com/blox-eng/matchblox/cmd/matchblox@latest
```

Check that it runs: `matchblox version`.

## 2. Run it

```sh
matchblox
```

Outside tmux, `matchblox` opens the console in its own tmux session, named
`matchblox`. A second terminal that runs `matchblox` joins the same session.

The console starts the service in the background if it does not run. The
service samples the machine once for every console that watches. It keeps
the history when no console is open. `matchblox serve` runs the service
in the foreground, for a log or a service manager.

## 3. Answer the first run

The first run asks one question: **Where do your agents run?**

- **this machine**: the console of this machine opens.
- **another machine**: pick a host from `~/.ssh/config`, or type one. See
  [Remote hosts](remote.md).

The first run also writes `~/.config/matchblox/config.toml` with goals for
this machine, and the agents it finds on `PATH`.

## 4. Open the setup steps

The queue shows the setup steps above everything else, under `SET UP`. Each
step says why it helps. `Enter` shows the exact change before it runs. A
typed `y` makes the change. `x` closes a step that you do not want.

| Step | What it does |
|---|---|
| Install tmux | Installs tmux with your package manager, when it is not there. |
| Add the queue hooks | Adds the matchblox hooks to `~/.claude/settings.json`, and keeps a backup. Claude Code then tells the queue when it waits. |
| Add the way back | Adds `prefix m` and a tap target `◂ matchblox` to your tmux config (`~/.tmux.conf`), and keeps a backup. From any pane, one key or one tap comes back to the console. `prefix m` was the tmux key that marks a pane. |
| Open a guide session | Starts Claude Code with a prompt that reads this page and walks you through it. It uses your tokens. |

A step that is done folds away. `matchblox setup` shows every step again,
done and closed ones too.

## 5. Start your agents in tmux

```sh
tmux new -s app claude
```

matchblox finds each agent that runs in a tmux pane. When an agent waits
for you, it comes to the top of the queue. `Enter` jumps to its pane. The
way back (`prefix m`) returns to the console.

Next: [the console](console.md).
