// Builds a session's view from the API's event stream.
import type { Effort, Message, Mode, Permission } from "@/lib/api"

export type Block =
  | { type: "text"; text: string }
  | { type: "tool_use"; id: string; name: string; input: unknown }
  | {
      type: "tool_result"
      tool_use_id: string
      content: unknown
      is_error?: boolean
    }
  | { type: "image"; media_type: string; data: string }
  | { type: "other" }

export type Entry = {
  uuid: string
  role: "user" | "assistant"
  blocks: Block[]
  /** Sent from this page and not yet in the transcript. */
  local?: boolean
  /** When the message was written, as an ISO 8601 time. */
  at?: string
  /** Marks where a compaction replaced the earlier history with a summary. The entry shows as a divider. */
  compacted?: boolean
}

/** What Claude is doing between a send and its result, apart from tools that run. */
export type Activity =
  | { kind: "starting" | "waiting" | "thinking" | "writing" }
  | { kind: "preparing"; tool: string }
  | { kind: "retrying"; attempt: number; max: number; status: number | null }

export type State = {
  entries: Entry[]
  draft: string
  pending: Permission[]
  running: boolean
  live: boolean
  /** True after the stream to agent-manager closed, until it reconnects. A send can't reach agent-manager. */
  lost?: boolean
  /** The model of the latest reply, as an API model ID. */
  model?: string
  /** The session's chosen effort. Absent means the model's default. */
  effort?: Effort
  /** Background tasks Claude is running. They go on after a turn ends. */
  background: number
  /** The session's permission mode, as chosen or as Claude last reported it. Absent until either. */
  mode?: Mode
  /** False when agent-manager has no session with this ID. */
  exists: boolean
  /** True once the first history page has arrived. */
  loaded: boolean
  /** True while the transcript has messages before the first loaded one. */
  more: boolean
  activity?: Activity
  /** When the current turn started, in milliseconds since the epoch. */
  since?: number
  /** A note under the transcript. A "connection" note clears when the stream reconnects. */
  notice?: { text: string; error: boolean; kind?: "connection" }
  /** True after a reply reported that Claude isn't logged in on the VM, until the next send. */
  loginNeeded?: boolean
  /** True between a compaction's boundary and its summary. The summary streams next. */
  compacting?: boolean
}

export const initial: State = {
  entries: [],
  draft: "",
  pending: [],
  running: false,
  live: false,
  background: 0,
  exists: true,
  loaded: false,
  more: false,
}

export type Event =
  | {
      name: "state"
      data: {
        live: boolean
        running?: boolean
        exists?: boolean
        /** When the running turn started, in milliseconds since the epoch. */
        since?: number
        effort?: Effort
        mode?: Mode
        background?: number
        pending: Permission[]
      }
    }
  | { name: "history"; data: { messages: Message[]; more?: boolean } }
  | { name: "earlier"; data: { messages: Message[]; more: boolean } }
  | { name: "batch"; events: Event[] }
  | { name: "effort"; effort?: Effort }
  | { name: "mode"; mode: Mode }
  | {
      name: "message"
      data: {
        message?: Message
        permission?: Permission
        answered?: string
        ended?: boolean
        error?: string
      }
    }
  | { name: "ended"; data: { error?: string } }
  | { name: "sent"; text: string; id: string; at: number }
  | { name: "unsent"; id: string; wasRunning: boolean }

function blocksOf(content: unknown): Block[] {
  if (typeof content === "string") return [{ type: "text", text: content }]
  if (!Array.isArray(content)) return []
  return content.map((block: { type?: unknown } | null) => {
    if (
      block?.type === "text" ||
      block?.type === "tool_use" ||
      block?.type === "tool_result"
    )
      return block as Block
    return imageOf(block) ?? { type: "other" }
  })
}

/** Image types the GUI shows. SVG stays out, because it can carry script. */
const imageTypes = new Set([
  "image/png",
  "image/jpeg",
  "image/gif",
  "image/webp",
])

/** An API image block with base64 data of a type the GUI shows, as a Block. */
export function imageOf(
  block: unknown,
): Extract<Block, { type: "image" }> | undefined {
  const b = block as {
    type?: unknown
    source?: { type?: unknown; media_type?: unknown; data?: unknown }
  } | null
  const source = b?.type === "image" ? b.source : undefined
  if (
    source?.type !== "base64" ||
    typeof source.media_type !== "string" ||
    !imageTypes.has(source.media_type) ||
    typeof source.data !== "string"
  )
    return undefined
  return { type: "image", media_type: source.media_type, data: source.data }
}

/**
 * The entry for a user or assistant message, or undefined for one the transcript leaves out. Subagent messages,
 * background task notifications, local commands and their output stay out. A compaction summary becomes a divider.
 */
function entryOf(message: Message): Entry | undefined {
  if (
    (message.type !== "user" && message.type !== "assistant") ||
    !message.uuid ||
    message.parent_tool_use_id ||
    message.origin?.kind === "task-notification" ||
    message.isReplay ||
    message.isCompletedLocalCommand ||
    message.isSynthetic
  ) {
    return undefined
  }
  const entry: Entry = {
    uuid: message.uuid,
    role: message.type,
    blocks: blocksOf(message.message?.content),
    at: message.timestamp,
  }
  if (message.isCompactSummary) return { ...entry, blocks: [], compacted: true }
  return entry
}

const textOf = (entry: Entry) =>
  entry.blocks.map((b) => (b.type === "text" ? b.text : "")).join("")

function addEntry(entries: Entry[], entry: Entry) {
  if (entries.some((e) => e.uuid === entry.uuid)) return entries
  const local =
    entry.role === "user"
      ? entries.findIndex((e) => e.local && textOf(e) === textOf(entry))
      : -1
  const rest = local < 0 ? entries : entries.filter((_, i) => i !== local)
  return [...rest, entry]
}

export function reduce(state: State, event: Event): State {
  switch (event.name) {
    case "state": {
      const kept = {
        ...state,
        notice: state.notice?.kind === "connection" ? undefined : state.notice,
      }
      return {
        ...kept,
        lost: false,
        pending: event.data.pending,
        running: event.data.running ?? false,
        live: event.data.live,
        exists: event.data.exists ?? true,
        effort: event.data.effort,
        mode: event.data.mode ?? state.mode,
        background: event.data.background ?? 0,
        since: event.data.running
          ? (event.data.since ?? state.since)
          : undefined,
        activity: event.data.running ? { kind: "waiting" } : undefined,
        // A stream that reconnects in the middle of a reply gets the rest of it as the assistant message.
        draft: "",
      }
    }
    case "history": {
      // A reconnect sends the latest page again. Earlier pages already loaded stay in front of it.
      const history = event.data.messages.flatMap((m) => entryOf(m) ?? [])
      const known = new Set(history.map((e) => e.uuid))
      const first = history.length
        ? state.entries.findIndex((e) => e.uuid === history[0].uuid)
        : -1
      const before = first > 0 ? state.entries.slice(0, first) : []
      const later = state.entries.filter(
        (e, i) => !e.local && !known.has(e.uuid) && (first < 0 || i > first),
      )
      const entries = [...before, ...history, ...later]
      return {
        ...state,
        loaded: true,
        entries,
        model: latestModel(event.data.messages) ?? state.model,
        more: before.length > 0 ? state.more : (event.data.more ?? false),
        loginNeeded: loginFailedLast(event.data.messages) || state.loginNeeded,
      }
    }
    case "earlier": {
      const known = new Set(state.entries.map((e) => e.uuid))
      const earlier = event.data.messages
        .flatMap((m) => entryOf(m) ?? [])
        .filter((e) => !known.has(e.uuid))
      return {
        ...state,
        entries: [...earlier, ...state.entries],
        more: event.data.more,
      }
    }
    case "batch":
      return event.events.reduce(reduce, state)
    case "effort":
      return { ...state, effort: event.effort }
    case "mode":
      return { ...state, mode: event.mode }
    case "sent":
      return {
        ...state,
        running: true,
        activity: { kind: state.live ? "waiting" : "starting" },
        since: event.at,
        notice: undefined,
        loginNeeded: false,
        entries: [
          ...state.entries,
          {
            uuid: event.id,
            role: "user",
            local: true,
            at: new Date(event.at).toISOString(),
            blocks: [{ type: "text", text: event.text }],
          },
        ],
      }
    // A send that fails takes back its message. A turn that was already running goes on.
    case "unsent":
      return {
        ...state,
        running: event.wasRunning,
        activity: event.wasRunning ? state.activity : undefined,
        entries: state.entries.filter((e) => e.uuid !== event.id),
      }
    case "ended":
      return {
        ...state,
        lost: true,
        running: false,
        activity: undefined,
        draft: "",
        notice: event.data.error
          ? {
              text: `Connection lost (${event.data.error})`,
              error: true,
              kind: "connection",
            }
          : state.notice,
      }
    case "message":
      return reduceAgentManagerEvent(state, event.data)
  }
}

function reduceAgentManagerEvent(
  state: State,
  data: Extract<Event, { name: "message" }>["data"],
): State {
  if (data.permission) {
    return { ...state, pending: [...state.pending, data.permission] }
  }
  if (data.answered) {
    return {
      ...state,
      pending: state.pending.filter((p) => p.request !== data.answered),
    }
  }
  if (data.ended) {
    return {
      ...state,
      running: false,
      live: false,
      activity: undefined,
      draft: "",
      pending: [],
      notice: data.error
        ? { text: `Session ended (${data.error})`, error: true }
        : state.notice,
    }
  }
  const message = data.message
  if (!message || message.parent_tool_use_id) return state
  if (message.type === "system" && message.subtype === "compact_boundary") {
    return { ...state, compacting: true }
  }
  if (state.compacting && message.type === "user" && message.isSynthetic) {
    // The summary streams with the uuid it keeps in the stored transcript, where it shows as the divider too.
    const divider: Entry = {
      uuid: message.uuid ?? "",
      role: "user",
      blocks: [],
      at: message.timestamp,
      compacted: true,
    }
    return message.uuid
      ? {
          ...state,
          compacting: false,
          entries: addEntry(state.entries, divider),
        }
      : { ...state, compacting: false }
  }
  if (message.type === "system" && message.subtype === "api_retry") {
    return {
      ...state,
      activity: {
        kind: "retrying",
        attempt: message.attempt ?? 0,
        max: message.max_retries ?? 0,
        status: message.error_status ?? null,
      },
      loginNeeded:
        message.error === "authentication_failed" || state.loginNeeded,
    }
  }
  if (message.type === "system" && message.subtype === "init") {
    // Every turn starts with init, including a queued message's turn and one a finished background task starts.
    return {
      ...state,
      running: true,
      since: state.since ?? Date.now(),
      live: true,
      model: message.model ?? state.model,
      mode: message.permissionMode ?? state.mode,
      activity: { kind: "waiting" },
    }
  }
  if (
    message.type === "system" &&
    message.subtype === "background_tasks_changed"
  ) {
    return {
      ...state,
      background: (message.tasks ?? []).filter((t) => !t.ambient).length,
    }
  }
  if (message.type === "system" && message.subtype === "status") {
    return message.permissionMode
      ? { ...state, mode: message.permissionMode }
      : state
  }
  if (message.type === "stream_event") {
    const event = message.event
    if (event?.type === "message_start") return { ...state, draft: "" }
    if (event?.type === "content_block_start") {
      const block = event.content_block
      const activity: Activity =
        block?.type === "tool_use"
          ? { kind: "preparing", tool: block.name ?? "a tool" }
          : { kind: block?.type === "thinking" ? "thinking" : "writing" }
      return { ...state, activity }
    }
    const delta = event?.delta
    if (delta?.type === "text_delta") {
      return { ...state, draft: state.draft + (delta.text ?? "") }
    }
    return state
  }
  if (message.type === "result") {
    return {
      ...state,
      running: false,
      activity: undefined,
      draft: "",
      notice: resultNotice(message.subtype),
    }
  }
  const entry = entryOf(message)
  if (!entry) return state
  const toolResult = entry.blocks.some((b) => b.type === "tool_result")
  const toolUse = entry.blocks.some((b) => b.type === "tool_use")
  return {
    ...state,
    loginNeeded:
      message.error === "authentication_failed" ? true : state.loginNeeded,
    draft: entry.role === "assistant" ? "" : state.draft,
    model: latestModel([message]) ?? state.model,
    activity: toolUse
      ? undefined
      : toolResult && state.running
        ? { kind: "waiting" }
        : state.activity,
    entries: addEntry(state.entries, entry),
  }
}

/** Maps tool use IDs to their results. Results arrive in later user entries. */
export function toolResults(entries: Entry[]) {
  const results = new Map<string, Extract<Block, { type: "tool_result" }>>()
  for (const entry of entries) {
    for (const block of entry.blocks) {
      if (block.type === "tool_result") results.set(block.tool_use_id, block)
    }
  }
  return results
}

/** When each tool's result arrived, by tool use ID, as an ISO 8601 time. */
export function toolEnds(entries: Entry[]) {
  const ends = new Map<string, string>()
  for (const entry of entries) {
    if (!entry.at) continue
    for (const block of entry.blocks) {
      if (block.type === "tool_result") ends.set(block.tool_use_id, entry.at)
    }
  }
  return ends
}

/** True for user entries that hold only tool results. Those show under their tool use. */
export function onlyToolResults(entry: Entry) {
  return (
    entry.role === "user" && entry.blocks.every((b) => b.type === "tool_result")
  )
}

/** Tool uses with no result yet, while a turn runs. */
export function runningTools(state: State) {
  if (!state.running) return []
  const results = toolResults(state.entries)
  return state.entries.flatMap((e) =>
    e.blocks.flatMap((b) =>
      b.type === "tool_use" && !results.has(b.id) ? [b] : [],
    ),
  )
}

/** The model named by the latest assistant message in a list. Error replies carry the placeholder "<synthetic>". */
function latestModel(messages: Message[]) {
  for (let i = messages.length - 1; i >= 0; i--) {
    const model = messages[i].type === "assistant" && messages[i].message?.model
    if (model && model !== "<synthetic>") return model
  }
  return undefined
}

/** True when the last user or assistant message is a reply that says Claude isn't logged in. */
function loginFailedLast(messages: Message[]) {
  const last = messages.findLast(
    (m) => m.type === "user" || m.type === "assistant",
  )
  return last?.type === "assistant" && last.error === "authentication_failed"
}

/** A stopped turn ends with error_during_execution. Other non-success results are errors. */
function resultNotice(subtype?: string): State["notice"] {
  if (subtype === "success") return undefined
  if (subtype === "error_during_execution")
    return { text: "Stopped", error: false }
  return { text: `Turn ended (${subtype})`, error: true }
}

/** True for entries with something to show. Thinking-only replies and tool results alone have none. */
// Claude Code's text for a stopped turn, which it stores as the person's message, with " for tool use]" during a tool.
const interrupted = "[Request interrupted by user"

/**
 * True when no turn runs and the person's stored message is the last thing shown. The turn after it ended without a
 * reply, as when agent-manager restarts mid-turn, whether or not a page was open.
 */
export function unanswered(state: State) {
  const last = state.entries.filter(visible).at(-1)
  return (
    state.loaded &&
    !state.running &&
    last?.role === "user" &&
    !last.local &&
    !last.compacted &&
    !textOf(last).startsWith(interrupted)
  )
}

export function visible(entry: Entry) {
  if (entry.compacted) return true
  if (onlyToolResults(entry)) return false
  return entry.blocks.some(
    (b) => b.type === "text" || b.type === "tool_use" || b.type === "image",
  )
}

/** The entry a link to `id` lands on. That is the entry itself, or the next one with something to show. */
export function landing(entries: Entry[], id: string) {
  const at = entries.findIndex((e) => e.uuid === id)
  if (at < 0) return undefined
  return entries.slice(at).find(visible)?.uuid
}

/**
 * The text of each turn's replies, by the ID of the turn's last reply. A turn is Claude's replies after one of the
 * person's messages. Its text is every text block, without the tool calls.
 */
export function turnTexts(entries: Entry[]) {
  const texts = new Map<string, string>()
  let parts: string[] = []
  let last: string | undefined
  const end = () => {
    if (last) texts.set(last, parts.join("\n\n"))
    parts = []
    last = undefined
  }
  for (const entry of entries.filter(visible)) {
    if (entry.role === "user") {
      end()
      continue
    }
    last = entry.uuid
    for (const b of entry.blocks) {
      if (b.type === "text" && b.text.trim()) parts.push(b.text)
    }
  }
  end()
  return texts
}
