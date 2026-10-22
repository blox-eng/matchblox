# Tailscale SSH gate bypass (#185) — design

## Intent

Paired laptop and phone keys use OpenSSH `restrict` + ForcedCommand (`matchblox gate …`). When Tailscale SSH stays on (`RunSSH: true`), connections to the host’s MagicDNS name or Tailscale IP on port 22 are answered by tailscaled, which authenticates by tailnet identity and ignores `authorized_keys`. The gate never runs on that path. #184 only stops `matchblox pair` before the QR; permanent keys remain bypassed.

Builders must be told plainly that on the tailnet `:22` path paired keys are not gated, and how to keep the gate meaningful.

## Goals

- Detect host-wide Tailscale SSH (`RunSSH`) by reusing `#184` paircheck probes.
- Warn on surfaces builders already use: `matchblox serve`, `matchblox status`, and `matchblox setup` (no `doctor` command exists).
- Document honest setups: `tailscale set --ssh=false`, or `--address`/`--port` that reaches OpenSSH; note that Tailscale ACLs can limit Tailscale SSH but do not restore ForcedCommand.
- Tests with fixtures/fakes for prefs (same pattern as paircheck).
- CHANGELOG Unreleased; PR closes #185.

## Non-goals / anti-goals

- Do not invent a new pairing design or weaken OpenSSH ForcedCommand.
- Do not reopen pair-QR preflight unless a clear gap remains (#184 stays as-is).
- Do not claim matchblox can see or gate sessions Tailscale SSH answers (gate never runs).
- Do not add a `doctor` command solely for this warning.

## Approach (warn + document)

1. **Detection:** Export a small helper that returns whether Tailscale SSH is on for this host (`LookPath("tailscale")` + prefs JSON `RunSSH: true`). Skip (false) when the tool or prefs cannot run — same spirit as paircheck. Do not require MagicDNS/IP match; this is host-wide permanent-key risk, not pair-address refusal.
2. **Message:** One shared string naming the bypass and the fix (`tailscale set --ssh=false`, or reach OpenSSH on another address/port). Distinct from `ErrTailscaleSSH` (pair refusal).
3. **Surfaces:** Non-fatal stderr (or text stdout for `status --text`) on `serve` and `status`; print the same line from `setup` before the door list. Do not set `pairDoor.Problem` (that blocks opening; pair already refuses via #184).
4. **Optional connect warn:** Skip. Connections Tailscale SSH answers never enter `matchblox gate`, so there is no honest connect hook for the bypass path.
5. **Docs:** Extend `docs/phone.md` and `docs/remote.md` with the permanent-key / gate story after the pair-preflight note.

## Calls left to the builder

- Exact warning copy (must include `tailscale set --ssh=false` and that the gate is skipped on the MagicDNS/:22 path).
- Whether console gets a flash: prefer serve/status/setup only unless a one-line flash is trivial; not required for done.
