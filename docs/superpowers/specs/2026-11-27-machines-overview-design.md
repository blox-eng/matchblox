# An owner with agents on several hosts sees in one look which machine is in trouble, why, and which accounts it runs

Issue: #520.

## Who, and the moment

The owner runs agents on two or more hosts: a Linux workstation and one or more laptops. A host feels slow, or an agent on it stalls. The owner opens Machine in the console. They want to know, in one look: which machine is in trouble, what drives it, and which harness accounts run there.

Evidence: 2026-11-26, a laptop host felt slow. The network and ssh were fine. An agent job ran `xargs -P 16` over `gcloud storage cp`, and each copy started its own worker pool. The result was about 250 Python processes, a load average of 190 on 15 cores, and 77 processes waiting on I/O. The owner asked an agent "is there anything wrong with the machine or the ssh connection", and the answer took six ssh commands.

## Why it matters

- Today the Machine tab draws one long section for each host (`machinePanel`, `internal/app/dock.go`). The owner scrolls through each section to find the bad host. Measured: 2 hosts make about 2 screens of sections.
- A host that never answered is left out of the sections (`statedHosts`). It shows only in the "hosts · N" list at the bottom, with no last-seen time.
- USAGE groups accounts by name across hosts (`usageCells`). Nothing says which host runs which account.
- Nothing compares load with the core count. PSI is Linux-only, so a macOS host shows `io 0%` while 77 processes wait on I/O.
- The top processes list shows single processes. 250 workers at 7% each never show as one cause.

## The experience

At the top of Machine, under the view rows and above USAGE, a region `machines · N` shows one row for each host:

```
 MACHINES · 3
 ✗ box-3    not reached since 14:02 (2h ago)
     is not connected · run: matchblox connect box-3
 ! mac-1    up 9d   load 191 / 15 cores   cpu 94%   mem 84%   disk 76%   io wait 77   Claude Max · OpenAI Pro
     xargs → gcloud · 257 processes · 1,400% cpu · session fetch-laws
   ws-1     up 3d   load 4 / 32 cores     cpu 12%   mem 61%   disk 40%   io wait 0    Claude Max · Cursor
```

1. Each row of a host that answers shows: name, uptime, load (1 minute) against the core count, cpu, memory, disk, the count of processes waiting on I/O, and the harness accounts that its sessions run.
2. **Accounts** come from that host's `Snapshot.Limits`, one entry for each account. Each shows by its provider word from `providerNames` and its plan, never the account name or email. The plan is the part of the account label after ` · ` (`a@b.c · Max` → `Claude Max`). A label with no plan part shows the provider word alone (`Cursor`). Two accounts of one provider show twice (`Claude Max · Claude Pro`).
3. **Trouble.** A host is in trouble when it is not reached, or its 1-minute load is over its core count, or memory or the worst of its disks is at 90% or more. Memory gets the same 90% neg band that `useStyle` gives disk today. The mark `✗` means not reached. The mark `!` means a reached host in trouble. The mark and the trouble figure show in neg.
4. **Order.** Hosts marked `✗` come first (lost, never reached, silent, other version), then hosts over their cores, then hosts with memory or disk trouble, then the rest. Within each group, Hosts order.
5. **Cause.** Under a host whose load is over its core count, one line names the process tree with the highest summed cpu. The tree is cut at the first process under the pane's shell or agent, and an `sh -c` or `bash -c` wrapper is skipped. For a tree outside any pane, it is cut at the first process under init or launchd. The line shows that first command, the next command in the tree, the count of processes, the summed cpu, and the session whose pane holds the tree, or `no session`.
6. **Enter** on a row moves Machine to that host's section. With one host, it moves to the `MACHINE` lines. For a host that has no section (never reached, other version), it moves to the host's row in the hosts list at the bottom.
7. With one host, the region has one row. The cause line and the io-wait count matter on one host too. A single host that is not reached keeps today's blocked screen, and the region is not drawn.

### When it goes wrong

Each problem shows in the line under the row, with the fix in the words. For a host marked `✗`, the reason and the fix are the words that `blockedWords` gives the Sessions row today, word for word: one owner for each fact.

| What happened | The row | The line under it |
|---|---|---|
| The host is lost or stale | `✗ box-3  not reached since 14:02 (2h ago)`, and no figures, from the moment the connection is lost, not only after `staleAfter` | the `blockedWords` reason and fix |
| The host was never reached | `✗ box-3  never reached`, and no figures | the `blockedWords` reason and fix |
| The host is connected but silent (no state in 3 s, `silentAfter`) | `✗ box-3  connected, no state yet`, and no figures | the `blockedWords` reason and fix |
| The host runs another protocol version | `✗ box-3  runs another version`, and no figures | the `blockedWords` reason and fix |
| The host's serve is older and sends no uptime, io-wait or cause | each of those figures says `unknown` | `box-3 runs an older matchblox · on box-3: matchblox update` |
| A figure cannot be read on that OS | that figure says `unknown`, never `0` | the reason |
| No session on the host runs under a plan | `no plan sessions` in place of the accounts | — this is a normal state |

The new snapshot fields keep `proto.Version` at 1. An older serve leaves them out, and the console tells a missing field from a zero.

## Non-goals

- No new CLI command. `matchblox status` JSON carries the new sampled fields. `--text` does not change.
- No change to USAGE, to the per-host sections, or to the bottom hosts list.
- No alerts, push notices or stoker triggers on trouble.
- No act on a row: no kill, no jump, no renice. Enter only moves within Machine.
- No load history or chart in the region.
- No change to the iOS app or to the desk bar of #494.

## Anti-goals (failure even if it works)

- A host that is not reached shows figures that look live.
- A figure that is not known shows as `0`.
- The cause line names a session that did not start the tree.
- A host in trouble sits below a healthy host.
- Sampling the process tree makes serve itself a load: a host at load 190 stops answering the console, or serve's cpu on an idle host rises by 1 percentage point or more.
- An account name or email shows in the region, or anywhere USAGE does not show it today: a log, the demo, the landing page, a test fixture with a real address.

## Surfaces

- Personal data — account names are often email addresses. The region shows the provider and plan only, never the name. Its anti-goal is above.
- No other sensitive surface touched. Checked: authentication, money, uploads, webhooks, secrets, CI, an agent's tools.

## Rulings

- 2026-11-27 — Build way 1: an overview region at the top of Machine, one row for each host, trouble first, a cause line under a host over its cores, and Enter moves to that host's section. The other ways were 2: add the same facts to each host section (smallest change, but the owner still scrolls N sections), and 3: way 1 plus a `matchblox machines` CLI (agents can read it, but it adds a second surface to build and test). — because the region answers "which machine and why" in the first lines, and it reuses the sampled data that each host sends today. — the owner.
- 2026-11-27 — Build on `origin/main`, with the new code in its own file. — because PR #504 makes Machine a sheet but keeps its content, and #504 has not moved since 2026-11-25. — proposed, not confirmed; the approval of this spec confirms it.
- 2026-11-27 — Show provider and plan, never the account name. — because the region is a glance view and the name adds personal data without adding a decision. — proposed, not confirmed; the approval of this spec confirms it.

- 2026-11-27 — The 13 findings of the second cold read are settled by the builder's recommendations, as now written in "The experience", the failure table, the anti-goals and "Done when": a 3-minute load window, trees ranked by summed cpu with `sh -c` wrappers skipped, the worst disk, `providerNames` from `Snapshot.Limits`, no release string, `blockedWords` as the one owner of the reason and fix, "not reached" from the moment of loss, a 1-point serve cpu budget, every `✗` host first, a 90% memory band, `!` and `✗` defined, and no region for a single unreached host. — because each one reuses a fact the code already owns, or makes a done-when line runnable. — the owner.
- 2026-11-27: spec approved. Decided by: the owner.

## Left to the builder

- How serve counts the processes waiting on I/O on Linux and on macOS, and how it groups the process trees.
- The names and the shape of the new snapshot fields (uptime, io-wait count, cause), as long as an older console ignores them and a missing field is not read as zero.
- The exact columns, widths and how a narrow terminal folds the row.

## Calls to check

- Is "1-minute load over the core count" the trouble rule for load? Recommendation: yes — because it is the plain saturation rule and the incident had 190 on 15. Cost if wrong: false alarms on a host with a short burst; the 1-minute figure recovers in minutes.
- Does the region show with one host? Recommendation: yes — because the cause line and the io-wait count help on one host too. Cost if wrong: one extra line for an owner with one host.

## Done when

- [ ] With two hosts, Machine opens with `machines · 2` above USAGE, one row for each host, in the order of "The experience" step 4.
- [ ] On a host where a person starts `xargs -P <cores+4>` over a busy loop from a session, the row turns neg within 3 minutes, shows `load N / C cores`, and the cause line names `xargs`, the process count and that session.
- [ ] With the same busy tree started outside any tmux pane, the cause line says `no session`.
- [ ] With a peer's serve stopped, its row says `not reached since HH:MM` and `run: matchblox connect <host>`, and it shows no figures.
- [ ] A peer that runs an older serve shows `io wait unknown` and `on <host>: matchblox update`, never `io wait 0`.
- [ ] A lost peer's row loses its figures at once, and its reason and fix match its Sessions row word for word.
- [ ] Serve's cpu on an idle host, measured for 5 minutes before and after the change, rises by less than 1 percentage point.
- [ ] The io-wait count is right on Linux and on macOS: a test reads a fixture with processes in the waiting state and counts them.
- [ ] Each row lists the providers and plans that the sessions on that host run, and no account name. A host with none says `no plan sessions`.
- [ ] Enter on a row moves Machine to that host's section, or to its hosts-list row when it has no section.
- [ ] `docs/console.md`, `docs/phone.md` and `docs/remote.md` name the machines region where they describe Machine.

## Cold read

Cold read: 2026-11-27, 1 fresh reader. Blocker 0, major 6, nit 8. Fixed 13, contested 0, for the person 1, declined 0. Second read: 13 left (major 8, nit 5), each given to the owner with a recommendation.

- Changed after the read: The experience (the example rows now follow the approved rules: `providerNames`, the `blockedWords` line, the tree root's Comm, and the upper-case region head that `region()` draws).
- For the person: ruling 2 (build on `origin/main`) and the two calls to check stay open until the owner approves the spec.
