import assert from "node:assert/strict"
import { once } from "node:events"
import net from "node:net"
import os from "node:os"
import { test } from "node:test"
import type { CanUseTool, Options } from "@anthropic-ai/claude-agent-sdk"
import { AgentManager, type Client, type Sdk } from "./agent-manager.ts"

/** A directory that exists, for sessions that create checks. */
const work = os.tmpdir()

type Fake = {
  calls: { options: Options }[]
  emit: (message: object) => void
  fail: (error: Error) => void
  closed: () => number
  modelAsks: () => number
  applied: object[]
  modes: string[]
  sdk: Sdk
}

/** An SDK whose query yields what the test emits. */
function fakeSdk(info?: object): Fake {
  const calls: Fake["calls"] = []
  const queue: (object | Error)[] = []
  let wake = () => {}
  let closes = 0
  let modelAsks = 0
  const applied: object[] = []
  const modes: string[] = []
  const push = (item: object | Error) => {
    queue.push(item)
    wake()
  }
  const query = (params: { options: Options }) => {
    calls.push(params)
    return {
      close() {
        closes++
        push(new Error("closed"))
      },
      interrupt: () => Promise.resolve(),
      supportedModels: () => {
        modelAsks++
        return Promise.resolve([{ value: "haiku", displayName: "Haiku" }])
      },
      setPermissionMode: (mode: string) => {
        modes.push(mode)
        return Promise.resolve()
      },
      applyFlagSettings: (settings: object) => {
        applied.push(settings)
        return Promise.resolve()
      },
      async *[Symbol.asyncIterator]() {
        for (;;) {
          const item = queue.shift()
          if (item instanceof Error) {
            if (item.message === "closed") return
            throw item
          }
          if (item) yield item
          else await new Promise<void>((resolve) => (wake = resolve))
        }
      },
    }
  }
  return {
    calls,
    emit: push,
    fail: push,
    closed: () => closes,
    modelAsks: () => modelAsks,
    applied,
    modes,
    sdk: {
      query,
      getSessionInfo: () => Promise.resolve(info),
      getSessionMessages: () => Promise.resolve([]),
      listSessions: () => Promise.resolve([]),
      renameSession: () => Promise.resolve(),
    } as unknown as Sdk,
  }
}

/** A permission request as clients see it. */
type Permission = { request: string; tool: string; title?: string }

/** The fields the tests read from agent-manager's replies and events. */
type Line = {
  id?: number
  error?: string
  session?: string
  ended?: boolean
  answered?: string
  permission?: Permission
  pending?: Permission[]
  messages?: { uuid: string }[]
  more?: boolean
  models?: { value: string; displayName: string }[]
  effort?: string
  mode?: string
  since?: number
  exists?: boolean
  states?: Record<string, object>
  sessions?: { sessionId: string; summary: string; cwd: string }[]
  message?: {
    type?: string
    uuid?: string
    timestamp?: string
    message?: object
  }
}

function recorder() {
  const lines: Line[] = []
  const client: Client = (line) => lines.push(line)
  return { lines, client }
}

/** Returns the nth line from the end, asserting that it exists. */
function last(lines: Line[], n = 1): Line {
  const line = lines.at(-n)
  assert.ok(line, `line ${n} from the end`)
  return line
}

/** Calls the canUseTool callback of the fake's first query. */
function ask(
  fake: Fake,
  tool: string,
  input: Record<string, unknown>,
  options: { signal: AbortSignal; title?: string },
): ReturnType<CanUseTool> {
  const { canUseTool } = fake.calls[0].options
  assert.ok(canUseTool, "query has canUseTool")
  return canUseTool(tool, input, {
    ...options,
    toolUseID: "tool-use",
    requestId: "request",
  })
}

const limits = { live: 2, idleMs: 1000, modelsMs: 60_000 }
const tick = () => new Promise((resolve) => setTimeout(resolve, 5))

async function request(manager: AgentManager, client: Client, body: object) {
  await manager.serve(client, JSON.stringify({ id: 1, ...body }))
  await tick()
}

async function newSession(
  manager: AgentManager,
  client: Client,
  lines: Line[],
) {
  await request(manager, client, { type: "create", cwd: work })
  const { session } = last(lines)
  await request(manager, client, { type: "subscribe", session })
  return session as string
}

test("a new session passes its generated ID and relays messages with a receive time", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  const session = await newSession(manager, client, lines)
  await request(manager, client, { type: "send", session, content: "hi" })
  assert.equal(fake.calls[0].options.sessionId, session)
  assert.equal(fake.calls[0].options.cwd, work)
  const message = { type: "assistant", uuid: "u" }
  fake.emit(message)
  await tick()
  assert.deepEqual(last(lines), {
    session,
    message: { ...message, timestamp: last(lines).message?.timestamp },
  })
  assert.match(last(lines).message?.timestamp ?? "", /^\d{4}-/)
  // A message with a time keeps it. Only user and assistant messages get one.
  fake.emit({ ...message, timestamp: "2026-01-01T00:00:00Z" })
  fake.emit({ type: "result", subtype: "success" })
  await tick()
  assert.equal(last(lines, 2).message?.timestamp, "2026-01-01T00:00:00Z")
  assert.equal(last(lines).message?.timestamp, undefined)
})

test("a second send reuses the running query", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  const session = await newSession(manager, client, lines)
  await request(manager, client, { type: "send", session, content: "one" })
  await request(manager, client, { type: "send", session, content: "two" })
  assert.equal(fake.calls.length, 1)
})

test("sending to a stored session resumes it in its directory", async () => {
  const fake = fakeSdk({ cwd: "/stored" })
  const manager = new AgentManager(fake.sdk, limits)
  const { client } = recorder()
  await request(manager, client, { type: "send", session: "s", content: "hi" })
  assert.equal(fake.calls[0].options.resume, "s")
  assert.equal(fake.calls[0].options.cwd, "/stored")
})

test("a session with no transcript ends with an error", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  await request(manager, client, { type: "subscribe", session: "s" })
  await request(manager, client, { type: "send", session: "s", content: "hi" })
  assert.equal(last(lines).ended, true)
  assert.match(last(lines).error ?? "", /no session s/)
})

test("a query that throws ends the session and allows a resume", async () => {
  const fake = fakeSdk({ cwd: "/work" })
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  const session = await newSession(manager, client, lines)
  await request(manager, client, { type: "send", session, content: "hi" })
  fake.fail(new Error("process exited"))
  await tick()
  assert.equal(last(lines).ended, true)
  await request(manager, client, { type: "send", session, content: "again" })
  assert.equal(fake.calls[1].options.resume, session)
})

test("an approval survives a reconnect and resolves once", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const first = recorder()
  const session = await newSession(manager, first.client, first.lines)
  await request(manager, first.client, { type: "send", session, content: "hi" })
  const decision = ask(
    fake,
    "Bash",
    { command: "ls" },
    { signal: new AbortController().signal, title: "Run ls" },
  )
  const { permission } = last(first.lines)
  assert.ok(permission)
  assert.equal(permission.tool, "Bash")
  assert.equal(permission.title, "Run ls")
  manager.disconnect(first.client)

  const second = recorder()
  await request(manager, second.client, { type: "subscribe", session })
  assert.deepEqual(last(second.lines).pending, [permission])

  const result = { behavior: "allow", updatedInput: { command: "ls" } }
  const body = { type: "answer", session, request: permission.request, result }
  await request(manager, second.client, body)
  assert.deepEqual(await decision, result)
  assert.equal(last(second.lines, 2).answered, permission.request)
  await request(manager, second.client, body)
  assert.match(last(second.lines).error ?? "", /no pending request/)
})

test("an aborted approval is denied and withdrawn", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  const session = await newSession(manager, client, lines)
  await request(manager, client, { type: "send", session, content: "hi" })
  const abort = new AbortController()
  const decision = ask(fake, "Bash", {}, { signal: abort.signal })
  const { permission } = last(lines)
  assert.ok(permission)
  abort.abort()
  assert.equal((await decision)?.behavior, "deny")
  assert.equal(last(lines).answered, permission.request)
})

test("at the live session limit a busy session refuses another", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, { ...limits, live: 1 })
  const { lines, client } = recorder()
  const one = await newSession(manager, client, lines)
  const two = await newSession(manager, client, lines)
  await request(manager, client, { type: "send", session: one, content: "hi" })
  await request(manager, client, { type: "send", session: two, content: "hi" })
  assert.match(last(lines).error ?? "", /all 1 live sessions are busy/)
  assert.equal(fake.closed(), 0)
})

test("at the live session limit the oldest idle session closes to make room", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, { ...limits, live: 1 })
  const { lines, client } = recorder()
  const one = await newSession(manager, client, lines)
  const two = await newSession(manager, client, lines)
  await request(manager, client, { type: "send", session: one, content: "hi" })
  fake.emit({ type: "result", subtype: "success", uuid: "r1" })
  await tick()
  await request(manager, client, { type: "send", session: two, content: "hi" })
  assert.equal(fake.closed(), 1)
  assert.equal(fake.calls.length, 2)
  assert.equal(fake.calls[1].options.sessionId, two)
  assert.ok(!lines.some((l) => l.error))
})

test("idle close skips turns, approvals and background tasks", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  const session = await newSession(manager, client, lines)
  await request(manager, client, { type: "send", session, content: "hi" })
  const later = () => Date.now() + limits.idleMs

  manager.closeIdle(later())
  assert.equal(fake.closed(), 0, "turn running")

  fake.emit({ type: "result" })
  fake.emit({
    type: "system",
    subtype: "background_tasks_changed",
    tasks: [{ task_id: "t" }],
  })
  await tick()
  manager.closeIdle(later())
  assert.equal(fake.closed(), 0, "background task running")

  fake.emit({
    type: "system",
    subtype: "background_tasks_changed",
    tasks: [{ task_id: "t", ambient: true }],
  })
  await tick()
  manager.closeIdle(Date.now())
  assert.equal(fake.closed(), 0, "recently active")
  manager.closeIdle(later())
  assert.equal(fake.closed(), 1)
  await tick()
  assert.deepEqual(last(lines), { session, ended: true, error: undefined })
})

test("a bad line gets an error reply", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  await manager.serve(client, "not json")
  assert.ok(last(lines).error)
  for (const line of ["null", "5", "[1]"]) {
    await manager.serve(client, line)
    assert.match(last(lines).error ?? "", /must be a JSON object/)
  }
  await request(manager, client, { type: "nope" })
  assert.match(last(lines).error ?? "", /unknown request/)
  await request(manager, client, { type: "messages" })
  assert.match(last(lines).error ?? "", /needs a session/)
})

test("a reset connection leaves agent-manager serving", async () => {
  const manager = new AgentManager(fakeSdk().sdk, limits)
  const server = net.createServer((socket) => manager.connect(socket))
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve))
  const { port } = server.address() as net.AddressInfo
  const dropped = net.connect(port, "127.0.0.1")
  await new Promise((resolve) => dropped.once("connect", resolve))
  dropped.resetAndDestroy()
  await tick()

  const next = net.connect(port, "127.0.0.1")
  next.write('{"id":7,"type":"create"}\n')
  const [reply] = (await once(next, "data")) as [Buffer]
  assert.equal((JSON.parse(String(reply)) as Line).id, 7)
  next.destroy()
  server.close()
})

test("messages pages back from the end", async () => {
  const fake = fakeSdk()
  const all = ["m1", "m2", "m3", "m4", "m5"].map((uuid) => ({ uuid }))
  fake.sdk.getSessionMessages = (() =>
    Promise.resolve(all)) as unknown as Sdk["getSessionMessages"]
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  await request(manager, client, { type: "messages", session: "s", limit: 2 })
  assert.deepEqual(last(lines), {
    id: 1,
    messages: [{ uuid: "m4" }, { uuid: "m5" }],
    more: true,
  })
  await request(manager, client, {
    type: "messages",
    session: "s",
    limit: 3,
    before: "m4",
  })
  assert.deepEqual(last(lines).messages, [
    { uuid: "m1" },
    { uuid: "m2" },
    { uuid: "m3" },
  ])
  assert.equal(last(lines).more, false)
  await request(manager, client, { type: "messages", session: "s" })
  assert.equal(last(lines).messages?.length, 5)
})

test("models come from Claude once, then from the cache", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  await request(manager, client, { type: "models" })
  await request(manager, client, { type: "models" })
  assert.deepEqual(last(lines).models, [
    { value: "haiku", displayName: "Haiku" },
  ])
  assert.equal(fake.modelAsks(), 1)
  assert.equal(fake.closed(), 1, "the process started for the list stops")
})

test("effort starts a session and changes a running one", async () => {
  const fake = fakeSdk({ cwd: "/work" })
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  await request(manager, client, { type: "create", effort: "high" })
  const { session } = last(lines)
  await request(manager, client, { type: "send", session, content: "hi" })
  assert.equal(fake.calls[0].options.effort, "high")
  await request(manager, client, { type: "effort", session, effort: "low" })
  assert.deepEqual(fake.applied, [{ effortLevel: "low" }])
  await request(manager, client, { type: "subscribe", session })
  assert.equal(last(lines).effort, "low")
  await request(manager, client, { type: "effort", session })
  assert.deepEqual(fake.applied.at(-1), { effortLevel: null })
})

test("a resumed session keeps its chosen effort", async () => {
  const fake = fakeSdk({ cwd: "/work" })
  const manager = new AgentManager(fake.sdk, limits)
  const { client } = recorder()
  await request(manager, client, {
    type: "effort",
    session: "s",
    effort: "max",
  })
  await request(manager, client, { type: "send", session: "s", content: "hi" })
  assert.equal(fake.calls[0].options.resume, "s")
  assert.equal(fake.calls[0].options.effort, "max")
})

test("subscribe reports when the running turn started", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  const session = await newSession(manager, client, lines)
  const before = Date.now()
  await request(manager, client, { type: "send", session, content: "hi" })
  await request(manager, client, { type: "subscribe", session })
  assert.ok((last(lines).since ?? 0) >= before)
  fake.emit({ type: "result" })
  await tick()
  await request(manager, client, { type: "subscribe", session })
  assert.equal(last(lines).since, undefined)
})

test("messages from a linked message reach back past the page size", async () => {
  const fake = fakeSdk()
  const all = Array.from({ length: 50 }, (_, i) => ({ uuid: `m${i}` }))
  fake.sdk.getSessionMessages = (() =>
    Promise.resolve(all)) as unknown as Sdk["getSessionMessages"]
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  await request(manager, client, {
    type: "messages",
    session: "s",
    limit: 5,
    before: "m45",
    from: "m20",
  })
  const { messages, more } = last(lines)
  assert.ok(messages)
  assert.equal(messages[0].uuid, "m10")
  assert.equal(messages.at(-1)?.uuid, "m44")
  assert.equal(more, true)
  await request(manager, client, {
    type: "messages",
    session: "s",
    from: "nope",
  })
  assert.equal(last(lines).messages?.[0]?.uuid, "m0")
})

test("subscribe says whether the session exists", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  await request(manager, client, { type: "subscribe", session: "nope" })
  assert.equal(last(lines).exists, false)
  const session = await newSession(manager, client, lines)
  await request(manager, client, { type: "subscribe", session })
  assert.equal(last(lines).exists, true)
  const stored = new AgentManager(fakeSdk({ cwd: "/work" }).sdk, limits)
  await request(stored, client, { type: "subscribe", session: "old" })
  assert.equal(last(lines).exists, true)
})

test("list reports each live session's turn and waiting approvals", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  const session = await newSession(manager, client, lines)
  await request(manager, client, { type: "send", session, content: "hi" })
  void ask(
    fake,
    "Bash",
    {},
    {
      signal: new AbortController().signal,
    },
  )
  await request(manager, client, { type: "list" })
  assert.deepEqual(last(lines).states, {
    [session]: { running: true, pending: 1, background: 0 },
  })
})

test("list includes sessions created and not yet sent to", async () => {
  const manager = new AgentManager(fakeSdk().sdk, limits)
  const { lines, client } = recorder()
  await request(manager, client, { type: "create", cwd: work })
  const session = last(lines).session
  await request(manager, client, { type: "list" })
  assert.deepEqual(
    last(lines).sessions?.map(
      (s: { sessionId: string; summary: string; cwd: string }) => [
        s.sessionId,
        s.summary,
        s.cwd,
      ],
    ),
    [[session, "New session", work]],
  )
})

test("create refuses a directory that doesn't exist", async () => {
  const manager = new AgentManager(fakeSdk().sdk, limits)
  const { lines, client } = recorder()
  await request(manager, client, { type: "create", cwd: "/nonexistent/dir" })
  assert.equal(last(lines).error, "no directory /nonexistent/dir")
})

test("a session created without a directory starts in the home directory", async () => {
  const manager = new AgentManager(fakeSdk().sdk, limits)
  const { lines, client } = recorder()
  await request(manager, client, { type: "create" })
  await request(manager, client, { type: "list" })
  assert.equal(last(lines).sessions?.[0]?.cwd, os.homedir())
})

test("a session with no chosen mode starts in Manual", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  await request(manager, client, { type: "create", cwd: work })
  const { session } = last(lines)
  await request(manager, client, { type: "send", session, content: "hi" })
  assert.equal(fake.calls[0].options.permissionMode, "default")
})

test("a session starts in its chosen mode and switches live", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  await request(manager, client, { type: "create", cwd: work, mode: "plan" })
  const { session } = last(lines)
  await request(manager, client, { type: "send", session, content: "hi" })
  assert.equal(fake.calls[0].options.permissionMode, "plan")
  await request(manager, client, { type: "mode", session, mode: "auto" })
  assert.deepEqual(fake.modes, ["auto"])
  await request(manager, client, { type: "subscribe", session })
  assert.equal(last(lines).mode, "auto")
})

test("a mode outside the allowed ones is refused", async () => {
  const manager = new AgentManager(fakeSdk().sdk, limits)
  const { lines, client } = recorder()
  await request(manager, client, { type: "create", cwd: work })
  const { session } = last(lines)
  await request(manager, client, {
    type: "mode",
    session,
    mode: "bypassPermissions",
  })
  assert.match(last(lines).error ?? "", /mode must be one of/)
})

test("the mode Claude reports becomes the session's mode", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  const session = await newSession(manager, client, lines)
  await request(manager, client, { type: "send", session, content: "hi" })
  fake.emit({
    type: "system",
    subtype: "status",
    status: null,
    permissionMode: "acceptEdits",
    uuid: "s1",
  })
  await tick()
  await request(manager, client, { type: "subscribe", session })
  assert.equal(last(lines).mode, "acceptEdits")
})

test("a restarted agent-manager keeps each session's effort and mode", async () => {
  const dir = await import("node:fs").then((fs) =>
    fs.mkdtempSync("/tmp/agent-manager-test-"),
  )
  const settingsFile = `${dir}/sessions.json`
  const first = new AgentManager(fakeSdk().sdk, { ...limits, settingsFile })
  const { lines, client } = recorder()
  await request(first, client, { type: "create", cwd: work, mode: "plan" })
  const { session } = last(lines)
  await request(first, client, { type: "effort", session, effort: "low" })
  await request(first, client, { type: "mode", session, mode: "auto" })

  const fake = fakeSdk({ cwd: "/w" })
  const second = new AgentManager(fake.sdk, { ...limits, settingsFile })
  await request(second, client, { type: "subscribe", session })
  assert.equal(last(lines).mode, "auto")
  assert.equal(last(lines).effort, "low")
  await request(second, client, { type: "send", session, content: "hi" })
  assert.equal(fake.calls[0].options.permissionMode, "auto")
  assert.equal(fake.calls[0].options.effort, "low")
})

test("sessions get Claude Code's own system prompt, and every settings source", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const { lines, client } = recorder()
  const session = await newSession(manager, client, lines)
  await request(manager, client, { type: "send", session, content: "hi" })
  assert.deepEqual(fake.calls[0].options.systemPrompt, {
    type: "preset",
    preset: "claude_code",
  })
  // settingSources left out loads ~/.claude/rules/rye.md, rye's instructions.
  assert.equal(fake.calls[0].options.settingSources, undefined)
})

test("a sent message reaches every subscriber with the uuid Claude gets", async () => {
  const fake = fakeSdk()
  const manager = new AgentManager(fake.sdk, limits)
  const one = recorder()
  const two = recorder()
  const session = await newSession(manager, one.client, one.lines)
  await request(manager, two.client, { type: "subscribe", session })
  await request(manager, one.client, { type: "send", session, content: "hi" })
  const echoed = two.lines.find((l) => l.message?.type === "user")
  assert.ok(echoed, "the second viewer got the message")
  assert.deepEqual(echoed.message?.message, { role: "user", content: "hi" })
  assert.ok(echoed.message?.uuid)
})

test("prune drops the saved settings of sessions Claude doesn't list", async () => {
  const fs = await import("node:fs")
  const dir = fs.mkdtempSync("/tmp/agent-manager-test-")
  const settingsFile = `${dir}/sessions.json`
  fs.writeFileSync(
    settingsFile,
    JSON.stringify({ kept: { effort: "low" }, gone: { mode: "plan" } }),
  )
  const fake = fakeSdk()
  fake.sdk.listSessions = () =>
    Promise.resolve([{ sessionId: "kept" }]) as ReturnType<Sdk["listSessions"]>
  await new AgentManager(fake.sdk, { ...limits, settingsFile }).prune()
  assert.deepEqual(JSON.parse(fs.readFileSync(settingsFile, "utf8")), {
    kept: { effort: "low" },
  })
})
