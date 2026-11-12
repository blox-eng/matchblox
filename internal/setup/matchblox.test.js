// The OpenCode plugin that `matchblox setup` installs, run as OpenCode runs
// it: one call for the plugin, then its event hook for each event, in the
// shapes OpenCode 1.19.3 sends (session.status, permission.*, question.*).
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

test("TheOpencodePluginTracksPendingByID: each wait clears only by its own id", async (t) => {
  const state = mkdtempSync(join(tmpdir(), "matchblox-state-"));
  t.after(() => rmSync(state, { recursive: true, force: true }));
  process.env.XDG_STATE_HOME = state;
  const { MatchbloxPlugin } = await import("./matchblox.ts");
  const hooks = await MatchbloxPlugin({ directory: "/work/wick-mobile" });
  const file = join(state, "matchblox", "opencode", `${process.pid}.json`);
  const report = () => {
    const { pid, startedAt, statusUpdatedAt, ...rest } = JSON.parse(readFileSync(file, "utf8"));
    assert.equal(pid, process.pid);
    assert.ok(startedAt > 0 && statusUpdatedAt >= startedAt, "times in ms");
    return rest;
  };
  const send = (type, properties) => hooks.event({ event: { id: "evt", type, properties } });

  assert.deepEqual(report(), { status: "idle" });
  await send("session.status", { sessionID: "ses_wick", status: { type: "busy" } });
  assert.deepEqual(report(), { status: "busy" });
  await send("permission.asked", { id: "per_A", sessionID: "ses_wick", permission: "bash", patterns: ["rm -rf build"] });
  await send("question.asked", { id: "que_B", sessionID: "ses_wick", questions: [] });
  assert.deepEqual(report(), { status: "waiting", waitingFor: "permission" });
  await send("permission.replied", { sessionID: "ses_wick", requestID: "per_A", reply: "once" });
  assert.deepEqual(report(), { status: "waiting", waitingFor: "question" }, "B still waits");
  await send("permission.replied", { sessionID: "ses_wick", requestID: "per_other", reply: "once" });
  assert.deepEqual(report(), { status: "waiting", waitingFor: "question" }, "another id clears nothing");
  await send("question.replied", { sessionID: "ses_wick", requestID: "que_B", answers: [] });
  assert.deepEqual(report(), { status: "busy" });
  await send("session.status", { sessionID: "ses_wick", status: { type: "retry", attempt: 2, message: "overloaded", next: 1790003700000 } });
  assert.deepEqual(report(), { status: "retry", attempt: 2, next: 1790003700000 });
  await send("session.status", { sessionID: "ses_wick", status: { type: "idle" } });
  assert.deepEqual(report(), { status: "idle" });

  await hooks.event({ event: undefined }); // never throws into OpenCode
  await hooks.dispose();
  assert.equal(existsSync(file), false, "an OpenCode that ends takes its report with it");
});
