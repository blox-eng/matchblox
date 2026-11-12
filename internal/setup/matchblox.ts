// matchblox: tells matchblox what this OpenCode does, so its board puts
// the agent in the right lane. `matchblox setup` installs this file; delete
// it to stop. It writes <state dir>/matchblox/opencode/<pid>.json on each
// change: status is busy, waiting, retry or idle. It needs no package and
// never stops OpenCode: a failed write is skipped.
import { mkdirSync, renameSync, rmSync, writeFileSync } from "node:fs"
import { homedir } from "node:os"
import { join } from "node:path"

export const MatchbloxPlugin = async () => {
  const dir = join(process.env.XDG_STATE_HOME || join(homedir(), ".local", "state"), "matchblox", "opencode")
  const file = join(dir, `${process.pid}.json`)
  const startedAt = Date.now()
  const working = new Map() // session id -> its status while it is not idle
  const permissions = new Map() // id of a permission that waits for a reply -> its session id
  const questions = new Map() // id of a question that waits for a reply -> its session id
  let last = ""

  const state = () => {
    if (permissions.size > 0) return { status: "waiting", waitingFor: "permission" }
    if (questions.size > 0) return { status: "waiting", waitingFor: "question" }
    for (const s of working.values()) {
      if (s.type === "retry") return { status: "retry", attempt: s.attempt, next: s.next }
    }
    return { status: working.size > 0 ? "busy" : "idle" }
  }

  const write = () => {
    const now = state()
    const key = JSON.stringify(now)
    if (key === last) return
    const tmp = `${file}.tmp`
    mkdirSync(dir, { recursive: true })
    writeFileSync(tmp, JSON.stringify({ pid: process.pid, startedAt, ...now, statusUpdatedAt: Date.now() }))
    renameSync(tmp, file)
    last = key
  }

  // OpenCode drops a wait it cancels with no reply event: a session that
  // is idle or deleted waits for nothing.
  const forget = (session) => {
    working.delete(session)
    for (const waits of [permissions, questions]) {
      for (const [id, s] of waits) if (s === session) waits.delete(id)
    }
  }

  const apply = (event) => {
    const p = event?.properties ?? {}
    switch (event?.type) {
      case "session.status":
        if (p.status?.type === "idle") forget(p.sessionID)
        else if (p.status) working.set(p.sessionID, p.status)
        break
      case "session.deleted":
        forget(p.sessionID)
        break
      case "permission.asked":
        permissions.set(p.id, p.sessionID)
        break
      case "permission.replied":
        permissions.delete(p.requestID)
        break
      case "question.asked":
        questions.set(p.id, p.sessionID)
        break
      case "question.replied":
      case "question.rejected":
        questions.delete(p.requestID)
        break
    }
  }

  try {
    write()
  } catch {}
  return {
    event: async ({ event }) => {
      try {
        apply(event)
        write()
      } catch {}
    },
    dispose: async () => {
      try {
        rmSync(file, { force: true })
      } catch {}
    },
  }
}
