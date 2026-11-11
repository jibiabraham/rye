import { describe, expect, it, test } from "vitest"

import {
  initial,
  landing,
  visible,
  reduce,
  runningTools,
  toolResults,
  turnTexts,
  unanswered,
  type Entry,
  type Event,
  type State,
} from "@/session/state"

const run = (...events: Event[]) => events.reduce(reduce, initial)
const message = (data: object): Event => ({ name: "message", data })
const assistant = (uuid: string, text: string) => ({
  type: "assistant",
  uuid,
  message: { content: [{ type: "text", text }] },
})

test("history replaces local entries and keeps later live ones", () => {
  const state = run(
    { name: "sent", text: "hi", id: "local-1", at: 0 },
    message({ message: assistant("a2", "live") }),
    {
      name: "history",
      data: {
        messages: [
          { type: "user", uuid: "u1", message: { content: "hi" } },
          assistant("a1", "earlier"),
        ],
      },
    },
  )
  expect(state.entries.map((e) => e.uuid)).toEqual(["u1", "a1", "a2"])
})

test("a live message seen in history is not repeated", () => {
  const state = run(
    { name: "history", data: { messages: [assistant("a1", "x")] } },
    message({ message: assistant("a1", "x") }),
  )
  expect(state.entries).toHaveLength(1)
})

test("an echoed user message replaces the local copy", () => {
  const state = run(
    { name: "sent", text: "hi", id: "local-1", at: 0 },
    message({
      message: { type: "user", uuid: "u1", message: { content: "hi" } },
    }),
  )
  expect(state.entries.map((e) => e.uuid)).toEqual(["u1"])
})

test("text deltas build a draft that the assistant message replaces", () => {
  const delta = (text: string) =>
    message({
      message: {
        type: "stream_event",
        event: {
          type: "content_block_delta",
          delta: { type: "text_delta", text },
        },
      },
    })
  let state = run(delta("hel"), delta("lo"))
  expect(state.draft).toBe("hello")
  state = reduce(state, message({ message: assistant("a1", "hello") }))
  expect(state.draft).toBe("")
})

test("a turn runs from send until its result", () => {
  let state: State = run({ name: "sent", text: "go", id: "local-1", at: 0 })
  expect(state.running).toBe(true)
  state = reduce(
    state,
    message({ message: { type: "result", subtype: "error_during_execution" } }),
  )
  expect(state.running).toBe(false)
  expect(state.notice).toEqual({ text: "Stopped", error: false })
})

const failedSend = (wasRunning: boolean) => {
  let state: State = { ...initial, running: wasRunning, live: true }
  state = reduce(state, { name: "sent", text: "more", id: "local-1", at: 0 })
  return reduce(state, { name: "unsent", id: "local-1", wasRunning })
}

test("a failed send between turns takes back its message and leaves no turn running", () => {
  const state = failedSend(false)
  expect(state.entries).toEqual([])
  expect(state.running).toBe(false)
})

test("a failed send during a turn takes back its message and keeps the turn running", () => {
  const state = failedSend(true)
  expect(state.entries).toEqual([])
  expect(state.running).toBe(true)
})

test("approvals come and go by request", () => {
  const permission = { request: "r1", tool: "Bash", input: {} }
  let state = run(message({ permission }))
  expect(state.pending).toEqual([permission])
  state = reduce(state, message({ answered: "r1" }))
  expect(state.pending).toEqual([])
})

test("an ended session clears approvals and reports its error", () => {
  const state = run(
    message({ permission: { request: "r1", tool: "Bash", input: {} } }),
    message({ ended: true, error: "process exited" }),
  )
  expect(state.pending).toEqual([])
  expect(state.notice).toEqual({
    text: "Session ended (process exited)",
    error: true,
  })
})

test("tool results attach to their tool use", () => {
  const state = run(
    message({
      message: {
        type: "assistant",
        uuid: "a1",
        message: {
          content: [{ type: "tool_use", id: "t1", name: "Bash", input: {} }],
        },
      },
    }),
    message({
      message: {
        type: "user",
        uuid: "u1",
        message: {
          content: [{ type: "tool_result", tool_use_id: "t1", content: "ok" }],
        },
      },
    }),
  )
  expect(toolResults(state.entries).get("t1")?.content).toBe("ok")
})

test("activity follows the stream from start to the running tool", () => {
  const block = (type: string, name?: string) =>
    message({
      message: {
        type: "stream_event",
        event: { type: "content_block_start", content_block: { type, name } },
      },
    })
  let state = run({ name: "sent", text: "go", id: "local-1", at: 5 })
  expect(state.activity).toEqual({ kind: "starting" })
  expect(state.since).toBe(5)
  state = reduce(
    state,
    message({ message: { type: "system", subtype: "init" } }),
  )
  expect(state.activity).toEqual({ kind: "waiting" })
  state = reduce(state, block("thinking"))
  expect(state.activity).toEqual({ kind: "thinking" })
  state = reduce(state, block("tool_use", "Glob"))
  expect(state.activity).toEqual({ kind: "preparing", tool: "Glob" })
  state = reduce(
    state,
    message({
      message: {
        type: "assistant",
        uuid: "a1",
        message: {
          content: [{ type: "tool_use", id: "t1", name: "Glob", input: {} }],
        },
      },
    }),
  )
  expect(state.activity).toBeUndefined()
  expect(runningTools(state).map((t) => t.id)).toEqual(["t1"])
  state = reduce(
    state,
    message({
      message: {
        type: "user",
        uuid: "u1",
        message: {
          content: [{ type: "tool_result", tool_use_id: "t1", content: "" }],
        },
      },
    }),
  )
  expect(runningTools(state)).toEqual([])
  expect(state.activity).toEqual({ kind: "waiting" })
})

test("a reconnect keeps earlier pages in front of the latest page", () => {
  const user = (uuid: string) => ({
    type: "user",
    uuid,
    message: { content: uuid },
  })
  let state = run({
    name: "history",
    data: { messages: [user("m3"), user("m4")], more: true },
  })
  expect(state.more).toBe(true)
  state = reduce(state, {
    name: "earlier",
    data: { messages: [user("m1"), user("m2")], more: false },
  })
  expect(state.entries.map((e) => e.uuid)).toEqual(["m1", "m2", "m3", "m4"])
  state = reduce(state, {
    name: "history",
    data: { messages: [user("m3"), user("m4"), user("m5")], more: true },
  })
  expect(state.entries.map((e) => e.uuid)).toEqual([
    "m1",
    "m2",
    "m3",
    "m4",
    "m5",
  ])
  expect(state.more).toBe(false)
})

test("a batch applies its events in order", () => {
  const delta = (text: string): Event => ({
    name: "message",
    data: {
      message: {
        type: "stream_event",
        event: {
          type: "content_block_delta",
          delta: { type: "text_delta", text },
        },
      },
    },
  })
  const state = run({ name: "batch", events: [delta("a"), delta("b")] })
  expect(state.draft).toBe("ab")
})

test("the session model comes from the latest reply and effort from state", () => {
  let state = run(
    { name: "state", data: { live: true, effort: "low", pending: [] } },
    {
      name: "history",
      data: {
        messages: [
          {
            type: "assistant",
            uuid: "a1",
            message: { content: [], model: "claude-haiku-5-5" },
          },
        ],
      },
    },
  )
  expect(state.model).toBe("claude-haiku-5-5")
  expect(state.effort).toBe("low")
  state = reduce(state, { name: "effort", effort: "max" })
  expect(state.effort).toBe("max")
})

test("a link to a hidden entry lands on the next visible one", () => {
  const state = run({
    name: "history",
    data: {
      messages: [
        { type: "user", uuid: "u1", message: { content: "hi" } },
        {
          type: "assistant",
          uuid: "a1",
          message: { content: [{ type: "thinking", thinking: "" }] },
        },
        {
          type: "assistant",
          uuid: "a2",
          message: { content: [{ type: "text", text: "hello" }] },
        },
      ],
    },
  })
  expect(state.entries.filter(visible).map((e) => e.uuid)).toEqual(["u1", "a2"])
  expect(landing(state.entries, "a1")).toBe("a2")
  expect(landing(state.entries, "nope")).toBeUndefined()
})

const historyEndingWith = (error?: string) =>
  run({
    name: "history",
    data: {
      messages: [
        { type: "user", uuid: "u1", message: { content: "hi" } },
        { type: "assistant", uuid: "a1", error, message: { content: [] } },
      ],
    },
  })

test("history that ends in a failed login asks for a login", () => {
  expect(historyEndingWith("authentication_failed").loginNeeded).toBe(true)
})

test("history that ends in an ordinary reply asks for no login", () => {
  expect(historyEndingWith().loginNeeded).toBeFalsy()
})

test("a reply that reports no Claude login asks for one until the next send", () => {
  const failed = reduce(initial, {
    name: "message",
    data: {
      message: {
        type: "assistant",
        uuid: "a1",
        error: "authentication_failed",
        message: {
          content: [
            { type: "text", text: "Not logged in · Please run /login" },
          ],
        },
      },
    },
  })
  expect(failed.loginNeeded).toBe(true)
  const sent = reduce(failed, { name: "sent", text: "hi", id: "u2", at: 1 })
  expect(sent.loginNeeded).toBe(false)
})

test("an error reply's <synthetic> model doesn't replace the session's model", () => {
  const state = run(
    message({
      message: {
        type: "assistant",
        uuid: "a1",
        message: { model: "claude-haiku-5-5", content: [] },
      },
    }),
    message({
      message: {
        type: "assistant",
        uuid: "a2",
        message: { model: "<synthetic>", content: [] },
      },
    }),
  )
  expect(state.model).toBe("claude-haiku-5-5")
})

test("the mode follows the subscribe state, Claude's init and status messages, and local changes", () => {
  const subscribed = reduce(initial, {
    name: "state",
    data: { live: false, pending: [], mode: "plan" },
  })
  expect(subscribed.mode).toBe("plan")
  const started = run(
    message({
      message: {
        type: "system",
        subtype: "init",
        uuid: "i",
        permissionMode: "default",
      },
    }),
  )
  expect(started.mode).toBe("default")
  const chosen = reduce(started, { name: "mode", mode: "auto" })
  expect(chosen.mode).toBe("auto")
  const left = reduce(
    chosen,
    message({
      message: {
        type: "system",
        subtype: "status",
        uuid: "s",
        permissionMode: "acceptEdits",
      },
    }),
  )
  expect(left.mode).toBe("acceptEdits")
  const quiet = reduce(
    left,
    message({ message: { type: "system", subtype: "status", uuid: "t" } }),
  )
  expect(quiet.mode).toBe("acceptEdits")
})

test("background tasks come from the subscribe state and Claude's task messages", () => {
  const subscribed = reduce(initial, {
    name: "state",
    data: { live: true, pending: [], background: 1 },
  })
  expect(subscribed.background).toBe(1)
  const changed = reduce(
    subscribed,
    message({
      message: {
        type: "system",
        subtype: "background_tasks_changed",
        uuid: "b",
        tasks: [{ ambient: false }, { ambient: true }, {}],
      },
    }),
  )
  expect(changed.background).toBe(2)
})

test("tool ends are the times their results arrived", async () => {
  const { toolEnds } = await import("@/session/state")
  const ends = toolEnds([
    {
      uuid: "a",
      role: "assistant",
      at: "2026-11-08T10:00:00Z",
      blocks: [{ type: "tool_use", id: "t1", name: "Bash" }],
    },
    {
      uuid: "u",
      role: "user",
      at: "2026-11-08T10:01:05Z",
      blocks: [{ type: "tool_result", tool_use_id: "t1", content: "ok" }],
    },
  ] as never)
  expect(ends.get("t1")).toBe("2026-11-08T10:01:05Z")
})

describe("turnTexts", () => {
  const user = (uuid: string): Entry => ({
    uuid,
    role: "user",
    blocks: [{ type: "text", text: "go" }],
  })
  const reply = (uuid: string, text: string): Entry => ({
    uuid,
    role: "assistant",
    blocks: [{ type: "text", text }],
  })
  const tool = (uuid: string): Entry => ({
    uuid,
    role: "assistant",
    blocks: [{ type: "tool_use", id: `t-${uuid}`, name: "Bash", input: {} }],
  })

  it("joins a turn's replies under its last reply, without the tool calls", () => {
    const turns = turnTexts([
      user("u1"),
      reply("a1", "Looking."),
      tool("a2"),
      reply("a3", "Done."),
    ])
    expect([...turns]).toEqual([["a3", "Looking.\n\nDone."]])
  })

  it("keeps each turn apart", () => {
    const turns = turnTexts([
      user("u1"),
      reply("a1", "A"),
      user("u2"),
      reply("a2", "B"),
    ])
    expect([...turns]).toEqual([
      ["a1", "A"],
      ["a2", "B"],
    ])
  })

  it("gives a turn of tool calls alone empty text", () => {
    expect([...turnTexts([user("u1"), tool("a1")])]).toEqual([["a1", ""]])
  })
})

test("a lost stream blocks sends until agent-manager's state arrives again", () => {
  const lost = run({
    name: "ended",
    data: { error: "agent-manager in this VM is unreachable" },
  })
  expect(lost.lost).toBe(true)
  expect(
    reduce(lost, { name: "state", data: { live: false, pending: [] } }).lost,
  ).toBe(false)
})

test("a subagent's messages stay out of the transcript and the draft", () => {
  const state = run(
    message({
      message: {
        ...assistant("a1", "from a subagent"),
        parent_tool_use_id: "t1",
      },
    }),
    message({
      message: {
        type: "stream_event",
        parent_tool_use_id: "t1",
        event: {
          type: "content_block_delta",
          delta: { type: "text_delta", text: "partial" },
        },
      },
    }),
  )
  expect(state.entries).toEqual([])
  expect(state.draft).toBe("")
})

test("a stored compaction shows as one divider", () => {
  const state = run({
    name: "history",
    data: {
      messages: [
        {
          type: "user",
          uuid: "s1",
          isCompactSummary: true,
          message: { content: "This session is being continued" },
        },
        {
          type: "user",
          uuid: "c1",
          isCompletedLocalCommand: true,
          message: {
            content:
              "<command-name>/compact</command-name>\n<command-args></command-args>",
          },
        },
        {
          type: "user",
          uuid: "o1",
          isCompletedLocalCommand: true,
          message: {
            content: "<local-command-stdout>Compacted </local-command-stdout>",
          },
        },
        {
          type: "user",
          uuid: "n1",
          origin: { kind: "task-notification" },
          message: { content: "<task-notification></task-notification>" },
        },
      ],
    },
  })
  expect(
    state.entries.map((e) => [e.uuid, e.compacted ?? false, e.blocks]),
  ).toEqual([["s1", true, []]])
})

test("a live compaction shows its summary as a divider under the stored uuid", () => {
  const state = run(
    message({
      message: { type: "system", subtype: "compact_boundary", uuid: "b1" },
    }),
    message({
      message: {
        type: "user",
        uuid: "s1",
        isSynthetic: true,
        message: { content: "This session is being continued" },
      },
    }),
    message({
      message: {
        type: "user",
        uuid: "o1",
        isReplay: true,
        message: {
          content: "<local-command-stdout>Compacted </local-command-stdout>",
        },
      },
    }),
  )
  expect(state.entries.map((e) => [e.uuid, e.compacted])).toEqual([
    ["s1", true],
  ])
  expect(state.compacting).toBe(false)
})

test("an image block in a message is kept", () => {
  const state = run(
    message({
      message: {
        type: "user",
        uuid: "u1",
        message: {
          content: [
            {
              type: "image",
              source: { type: "base64", media_type: "image/png", data: "AAAA" },
            },
          ],
        },
      },
    }),
  )
  expect(state.entries[0].blocks).toEqual([
    { type: "image", media_type: "image/png", data: "AAAA" },
  ])
  expect(visible(state.entries[0])).toBe(true)
})

test("an API retry shows as the activity, and an auth failure asks for a login", () => {
  const state = run(
    { name: "sent", text: "hi", id: "local-1", at: 0 },
    message({
      message: {
        type: "system",
        subtype: "api_retry",
        attempt: 3,
        max_retries: 10,
        error_status: 401,
        error: "authentication_failed",
      },
    }),
  )
  expect(state.activity).toEqual({
    kind: "retrying",
    attempt: 3,
    max: 10,
    status: 401,
  })
  expect(state.loginNeeded).toBe(true)
})

test("a turn that starts without a send runs from its init", () => {
  const state = run(message({ message: { type: "system", subtype: "init" } }))
  expect(state.running).toBe(true)
  expect(state.since).toBeTypeOf("number")
})

test("a reconnect clears a lost connection's note and the partial reply", () => {
  const state = run(
    { name: "sent", text: "hi", id: "local-1", at: 0 },
    message({
      message: {
        type: "stream_event",
        event: {
          type: "content_block_delta",
          delta: { type: "text_delta", text: "par" },
        },
      },
    }),
    { name: "ended", data: { error: "agent-manager closed the connection" } },
    { name: "state", data: { live: true, running: true, pending: [] } },
  )
  expect(state.notice).toBeUndefined()
  expect(state.draft).toBe("")
})

test("a turn is unanswered when the person's stored message is the last entry and nothing runs", () => {
  const user = { type: "user", uuid: "u1", message: { content: "hi" } }
  const loaded = (messages: object[], ...after: Event[]) =>
    run({ name: "history", data: { messages: messages as never } }, ...after)
  expect(unanswered(loaded([user]))).toBe(true)
  expect(unanswered(loaded([user, assistant("a1", "done")]))).toBe(false)
  // A stopped turn ends with Claude Code's own user message.
  for (const text of [
    "[Request interrupted by user]",
    "[Request interrupted by user for tool use]",
  ]) {
    const stop = {
      type: "user",
      uuid: "u2",
      message: { content: [{ type: "text", text }] },
    }
    expect(unanswered(loaded([user, stop]))).toBe(false)
  }
  // A send shows the message as local and the turn as running until its result.
  expect(
    unanswered(loaded([], { name: "sent", text: "hi", id: "local-1", at: 0 })),
  ).toBe(false)
  expect(
    unanswered(
      loaded([user], {
        name: "state",
        data: { live: true, running: true, pending: [] },
      }),
    ),
  ).toBe(false)
})
