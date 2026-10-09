# The console

The header shows the host, `stoker` or `night → 07:00` while [the
stoker](stoker.md) runs, and the machine: CPU, load, pressure, memory and
temperature, each with its trend. Under it are seven tabs, `1 QUEUE` to
`7 PANES`. A tab that needs you shows it: `1 QUEUE 3` has three agents that
wait, and `4 PROCS !` has a problem.

The action line under the tabs shows the keys of the open tab, and how old
the sample is. On a phone the keyboard covers the bottom of the screen, so
the action line is at the top. See [Keys](keys.md).

## The match

Each agent row starts with a match:

| Mark | Word | It means |
|---|---|---|
| `✦` | busy | The agent works. The match burns. |
| `╿` | asks, waits, done, paused | The agent waits for you. The match is at rest. |
| `│` | its state | The context is full (at `compact_at`, 85% by default). The match is spent, and Sessions says `! compact`. |

## 1 Queue

The agents that wait for you, the oldest wait first:

- **asks**: the agent asks for a permission.
- **waits**: the agent asked a question, or waits for input.
- **paused**: the turn ended with its progress bar under 100%.
- **done**: the turn ended.

Each row shows the agent's last line and its open pull request (`#N`), if
its worktree has one. `Enter` jumps to the pane. `a` answers in one line.
After a jump, the console selects the next agent that waits, so the way
back lands on it.

Under the agents are the recommendations, ranked. Each one shows its
evidence and its exact command. The setup steps (`SET UP`) come first while
some are open. With nothing to do, the queue says "nothing waits for you".

## 2 Sessions

One row for each agent, by tmux pane: its state, how long it is idle, its
name and tmux place, its context use against the model's window, its
account (on a wide screen), its token burn over 30 minutes, the CPU of its
process tree, and when to compact (`! compact`) or clear (`▲ clear`).

- Context use comes from the agent's own files. An agent that matchblox
  cannot read shows "not measured". See [Agents](agents.md).
- The account is the provider's mark, then the email and plan, or
  "API key": `✻` Anthropic, `❋` OpenAI, `▣` OpenCode.
- An agent can draw a progress bar in its last reply:
  `Progress [████░░░░] <step>`. The console shows it as `▰▰▰▱▱ 75%`.
- An idle session is `idle`, then `cold` after one day, then `stale` after
  seven days. On a stale session, `x` and a typed `y` end the agent.
- Sessions sort busy first, then the ones that wait, then idle with the
  newest first.
- Agents that exited and wait for their parent are one faint line under the
  sessions. `e` lists them with the command that clears them.

## 3 Machine

CPU per core, load, pressure, memory and swap, temperature, CPU power
limits, GPUs, and the CPU share of the container groups you name in the
[config](config.md).

## 4 Procs

Detached busy loops: shells that keep burning CPU after their pane is gone,
named to the pane that started them. `x` and a typed `y` kill one.

## 5 Git

Every git checkout that your sessions use: dirty paths, the default branch
behind its remote, and worktrees that are merged and safe to remove.
`Space` marks one, `X` marks every safe one, and `x` with a typed `y`
removes the marked ones.

## 6 History

24 hours of CPU, load, pressure, temperature, memory and swap.

## 7 Panes

Every tmux pane, with its command. `Enter` goes to it.
