import { randomUUID } from "node:crypto"
import fs from "node:fs"
import os from "node:os"
import path from "node:path"
import type { Socket } from "node:net"
import readline from "node:readline"
import type {
  CanUseTool,
  EffortLevel,
  ModelInfo,
  PermissionMode,
  PermissionResult,
  Query,
  SDKMessage,
  SDKUserMessage,
} from "@anthropic-ai/claude-agent-sdk"

export type Sdk = Pick<
  typeof import("@anthropic-ai/claude-agent-sdk"),
  | "getSessionInfo"
  | "getSessionMessages"
  | "listSessions"
  | "query"
  | "renameSession"
>

/** Receives replies and events for one connection. */
export type Client = (line: object) => void

/** Permission modes a client may choose. Others, such as bypassPermissions, stay out of reach. */
export const MODES: PermissionMode[] = [
  "default",
  "auto",
  "acceptEdits",
  "plan",
]

/** Messages a page keeps before a linked message, for context. */
const FROM_CONTEXT = 10

export type Limits = {
  live: number
  idleMs: number
  modelsMs: number
  /**
   * A JSON file that keeps each session's effort and mode across agent-manager restarts. Absent keeps them in memory.
   */
  settingsFile?: string
}

type Request = {
  id?: unknown
  type?: string
  session?: string
  request?: string
  cwd?: string
  model?: string
  effort?: EffortLevel
  mode?: PermissionMode
  title?: string
  limit?: number
  before?: string
  from?: string
  content?: SDKUserMessage["message"]["content"]
  result?: PermissionResult
}

type Permission = {
  request: string
  tool: string
  input: Record<string, unknown>
  [option: string]: unknown
}

/** A queue that feeds user messages to a running query. */
class Inbox {
  private messages: SDKUserMessage[] = []
  private wake = () => {}

  push(message: SDKUserMessage) {
    this.messages.push(message)
    this.wake()
  }

  async *[Symbol.asyncIterator]() {
    for (;;) {
      const message = this.messages.shift()
      if (message) yield message
      else await new Promise<void>((resolve) => (this.wake = resolve))
    }
  }
}

/** A session with a running query. */
type Live = {
  id: string
  inbox: Inbox
  query?: Query
  turn: boolean
  /** When the current turn started, in milliseconds since the epoch. */
  since?: number
  backgroundTasks: number
  lastActive: number
  pending: Map<
    string,
    { permission: Permission; resolve: (r: PermissionResult) => void }
  >
}

/** A session with a turn, an approval or a background task in progress. */
const busy = (s: Live) => s.turn || s.pending.size > 0 || s.backgroundTasks > 0

/** Runs Claude sessions and relays their messages to subscribed clients. */
export class AgentManager {
  private live = new Map<string, Live>()
  /** Sessions created and not yet sent to, with the time they were created in milliseconds since the epoch. */
  private drafts = new Map<
    string,
    { cwd?: string; model?: string; at: number }
  >()
  /** Effort chosen for a session. Absent means the model's default. */
  private efforts = new Map<string, EffortLevel>()
  /** Each session's permission mode, as chosen or as Claude last reported it. Absent until either. */
  private modes = new Map<string, PermissionMode>()
  private models?: { list: ModelInfo[]; at: number }
  private subscribers = new Map<string, Set<Client>>()
  private sdk: Sdk
  private limits: Limits

  constructor(sdk: Sdk, limits: Limits) {
    this.sdk = sdk
    this.limits = limits
    if (!limits.settingsFile) return
    try {
      const saved = JSON.parse(
        fs.readFileSync(limits.settingsFile, "utf8"),
      ) as Record<string, { effort?: EffortLevel; mode?: PermissionMode }>
      for (const [id, { effort, mode }] of Object.entries(saved)) {
        if (effort) this.efforts.set(id, effort)
        if (mode) this.modes.set(id, mode)
      }
    } catch {
      // A missing or invalid file leaves the saved settings empty.
    }
  }

  /** Drops the saved settings of sessions that Claude doesn't list. */
  async prune() {
    const known = new Set(
      (await this.sdk.listSessions()).map((s) => s.sessionId),
    )
    let dropped = false
    for (const settings of [this.efforts, this.modes]) {
      for (const id of settings.keys()) {
        if (known.has(id) || this.drafts.has(id) || this.live.has(id)) continue
        settings.delete(id)
        dropped = true
      }
    }
    if (dropped) this.save()
  }

  /** Writes each session's effort and mode to the settings file. */
  private save() {
    const file = this.limits.settingsFile
    if (!file) return
    const saved: Record<
      string,
      { effort?: EffortLevel; mode?: PermissionMode }
    > = {}
    for (const [id, effort] of this.efforts) saved[id] = { effort }
    for (const [id, mode] of this.modes) saved[id] = { ...saved[id], mode }
    try {
      fs.mkdirSync(path.dirname(file), { recursive: true })
      fs.writeFileSync(`${file}.tmp`, JSON.stringify(saved))
      fs.renameSync(`${file}.tmp`, file)
    } catch (e) {
      console.error(`saving ${file}: ${messageOf(e)}`)
    }
  }

  /** Serves one connection. A connection error ends that connection. */
  connect(socket: Socket) {
    const client: Client = (line) => socket.write(JSON.stringify(line) + "\n")
    socket.on("error", () => socket.destroy())
    socket.on("close", () => this.disconnect(client))
    readline
      .createInterface({ input: socket })
      .on("error", () => socket.destroy())
      .on("line", (line) => void this.serve(client, line))
  }

  /** Runs one line of input and replies to the client. */
  async serve(client: Client, line: string) {
    let request: Request = {}
    try {
      request = parseRequest(line)
      client({ id: request.id, ...(await this.handle(client, request)) })
    } catch (error) {
      client({ id: request.id, error: messageOf(error) })
    }
  }

  /** Drops a client's subscriptions. Its sessions stay live until they go idle. */
  disconnect(client: Client) {
    for (const clients of this.subscribers.values()) clients.delete(client)
  }

  /** Closes sessions with nothing to do since the idle limit. */
  closeIdle(now: number) {
    for (const session of this.live.values()) {
      const quiet = now - session.lastActive >= this.limits.idleMs
      if (quiet && !busy(session)) session.query?.close()
    }
  }

  private async handle(client: Client, request: Request): Promise<object> {
    switch (request.type) {
      case "list":
        return {
          sessions: [
            ...[...this.drafts].map(([id, draft]) => ({
              sessionId: id,
              summary: "New session",
              cwd: draft.cwd,
              lastModified: draft.at,
            })),
            ...(await this.sdk.listSessions()),
          ],
          live: [...this.live.keys()],
          states: Object.fromEntries(
            [...this.live.values()].map((s) => [
              s.id,
              {
                running: s.turn,
                pending: s.pending.size,
                background: s.backgroundTasks,
              },
            ]),
          ),
        }
      case "create": {
        if (
          request.cwd &&
          !fs.statSync(request.cwd, { throwIfNoEntry: false })?.isDirectory()
        )
          throw new Error(`no directory ${request.cwd}`)
        const session = randomUUID()
        this.drafts.set(session, {
          cwd: request.cwd || os.homedir(),
          model: request.model,
          at: Date.now(),
        })
        if (request.effort) this.efforts.set(session, request.effort)
        if (request.mode) this.modes.set(session, checkMode(request.mode))
        if (request.effort || request.mode) this.save()
        return { session }
      }
      case "models":
        return { models: await this.listModels() }
      case "mode":
        await this.setMode(sessionOf(request), checkMode(request.mode))
        return {}
      case "effort":
        await this.setEffort(sessionOf(request), request.effort)
        return {}
      case "subscribe":
        return this.subscribe(client, sessionOf(request))
      case "messages":
        return this.page(sessionOf(request), request)
      case "rename":
        await this.sdk.renameSession(sessionOf(request), request.title ?? "")
        return {}
      case "send":
        this.send(sessionOf(request), request.content ?? "")
        return {}
      case "interrupt":
        await this.live.get(sessionOf(request))?.query?.interrupt()
        return {}
      case "answer":
        this.answer(sessionOf(request), request)
        return {}
      default:
        throw new Error(`unknown request ${request.type}`)
    }
  }

  /**
   * The last `limit` messages before the message `before`, or before the end. With `from`, the page
   * reaches back to a few messages before that message, for a link to an old message. A `from` the
   * transcript no longer reaches, such as one before a compaction, starts the page at its first message.
   */
  private async page(id: string, { limit, before, from }: Request) {
    const all = await this.sdk.getSessionMessages(id)
    const find = (uuid: string) => {
      const i = all.findIndex((m) => m.uuid === uuid)
      if (i < 0) throw new Error(`no message ${uuid} in session ${id}`)
      return i
    }
    const end = before ? find(before) : all.length
    let start = limit ? Math.max(0, end - limit) : 0
    if (from) {
      const at = all.findIndex((m) => m.uuid === from)
      start = at < 0 ? 0 : Math.min(start, Math.max(0, at - FROM_CONTEXT))
    }
    return { messages: all.slice(start, end), more: start > 0 }
  }

  /** Subscribes a client. `exists` is false for an ID with no session, live, new or stored. */
  private async subscribe(client: Client, id: string) {
    const clients = this.subscribers.get(id) ?? new Set()
    this.subscribers.set(id, clients.add(client))
    const session = this.live.get(id)
    const exists =
      session !== undefined ||
      this.drafts.has(id) ||
      (await this.sdk.getSessionInfo(id)) !== undefined
    return {
      exists,
      live: session !== undefined,
      running: session?.turn ?? false,
      since: session?.turn ? session.since : undefined,
      background: session?.backgroundTasks ?? 0,
      effort: this.efforts.get(id),
      mode: this.modes.get(id),
      pending: [...(session?.pending.values() ?? [])].map((p) => p.permission),
    }
  }

  private send(id: string, content: NonNullable<Request["content"]>) {
    const session = this.live.get(id) ?? this.start(id)
    if (!session.turn) session.since = Date.now()
    session.turn = true
    session.lastActive = Date.now()
    const message: SDKUserMessage = {
      type: "user",
      uuid: randomUUID(),
      session_id: id,
      message: { role: "user", content },
      parent_tool_use_id: null,
    }
    session.inbox.push(message)
    // The SDK doesn't echo user messages. Every subscriber gets this one from here, with the uuid the transcript
    // records. History dedupes on that uuid.
    this.publish(id, { session: id, message: stamped(message) })
  }

  /**
   * Registers the session at once, because a second send must not start another query. At the limit it closes the
   * least recently active idle session. That session resumes from its transcript on its next message.
   */
  private start(id: string) {
    if (this.live.size >= this.limits.live) {
      const idle = [...this.live.values()]
        .filter((s) => !busy(s))
        .sort((a, b) => a.lastActive - b.lastActive)[0]
      if (!idle) {
        throw new Error(
          `all ${this.limits.live} live sessions are busy; wait for one to finish`,
        )
      }
      this.live.delete(idle.id)
      idle.query?.close()
    }
    const session: Live = {
      id,
      inbox: new Inbox(),
      turn: false,
      backgroundTasks: 0,
      lastActive: Date.now(),
      pending: new Map(),
    }
    this.live.set(id, session)
    void this.run(session)
    return session
  }

  /** Ends in exactly one `ended` event. That event closes any turn with no `result`. */
  private async run(session: Live) {
    let error: string | undefined
    try {
      session.query = this.sdk.query({
        prompt: session.inbox,
        options: {
          ...(await this.startOptions(session.id)),
          // install.sh writes rye's instructions to ~/.claude/rules/rye.md, and sessions load it.
          systemPrompt: { type: "preset", preset: "claude_code" },
          includePartialMessages: true,
          canUseTool: this.askClients(session),
        },
      })
      for await (const message of session.query) {
        this.observe(session, message)
        this.publish(session.id, {
          session: session.id,
          message: stamped(message),
        })
      }
    } catch (e) {
      error = messageOf(e)
    }
    // A session closed to make room is already gone, and its ID may be live again.
    if (this.live.get(session.id) === session) this.live.delete(session.id)
    for (const { resolve } of session.pending.values()) {
      resolve({ behavior: "deny", message: "session ended" })
    }
    this.publish(session.id, { session: session.id, ended: true, error })
  }

  /** The models Claude Code offers this VM's account, cached for modelsMs. */
  private async listModels() {
    if (this.models && Date.now() - this.models.at < this.limits.modelsMs) {
      return this.models.list
    }
    const running = [...this.live.values()].find((s) => s.query)?.query
    const list = running
      ? await running.supportedModels()
      : await this.askNewProcess()
    this.models = { list, at: Date.now() }
    return list
  }

  /** Starts Claude with no prompt to ask for its models, then stops it. */
  private async askNewProcess() {
    // A prompt stream that never yields keeps Claude waiting until the close.
    const idle: AsyncIterable<SDKUserMessage> = {
      [Symbol.asyncIterator]: () => ({ next: () => new Promise(() => {}) }),
    }
    const query = this.sdk.query({ prompt: idle })
    try {
      return await query.supportedModels()
    } finally {
      query.close()
    }
  }

  /** Sets a session's effort. A running query takes it from its next request on. */
  private async setEffort(id: string, effort: EffortLevel | undefined) {
    if (effort) this.efforts.set(id, effort)
    else this.efforts.delete(id)
    this.save()
    await this.live
      .get(id)
      ?.query?.applyFlagSettings({ effortLevel: effort ?? null })
  }

  /** Sets a session's permission mode. A running query switches at once. */
  private async setMode(id: string, mode: PermissionMode) {
    this.modes.set(id, mode)
    this.save()
    await this.live.get(id)?.query?.setPermissionMode(mode)
  }

  private async startOptions(id: string) {
    const effort = this.efforts.get(id)
    const mode = this.modes.get(id)
    const chosen = {
      ...(effort ? { effort } : {}),
      permissionMode: mode ?? ("default" as const),
    }
    const draft = this.drafts.get(id)
    if (draft) {
      this.drafts.delete(id)
      return { sessionId: id, ...draft, ...chosen }
    }
    const info = await this.sdk.getSessionInfo(id)
    if (!info) throw new Error(`no session ${id}`)
    return { resume: id, cwd: info.cwd, ...chosen }
  }

  private observe(session: Live, message: SDKMessage) {
    session.lastActive = Date.now()
    if (
      message.type === "system" &&
      (message.subtype === "init" || message.subtype === "status") &&
      message.permissionMode
    ) {
      if (this.modes.get(session.id) !== message.permissionMode) {
        this.modes.set(session.id, message.permissionMode)
        this.save()
      }
    }
    // Every turn starts with init, including a queued message's turn and one a finished background task starts.
    if (message.type === "system" && message.subtype === "init") {
      if (!session.turn) session.since = Date.now()
      session.turn = true
    }
    if (message.type === "result") session.turn = false
    if (
      message.type === "system" &&
      message.subtype === "background_tasks_changed"
    ) {
      session.backgroundTasks = message.tasks.filter((t) => !t.ambient).length
    }
  }

  /** Holds a tool call until a client answers. It waits across disconnects. */
  private askClients(session: Live): CanUseTool {
    return (tool, input, { signal, ...options }) =>
      new Promise((resolve) => {
        const request = randomUUID()
        const permission = { ...options, request, tool, input }
        session.pending.set(request, { permission, resolve })
        signal.addEventListener("abort", () => {
          if (!session.pending.delete(request)) return
          resolve({ behavior: "deny", message: "interrupted" })
          this.publish(session.id, { session: session.id, answered: request })
        })
        this.publish(session.id, { session: session.id, permission })
      })
  }

  private answer(id: string, { request, result }: Request) {
    const session = this.live.get(id)
    const pending = session?.pending.get(request ?? "")
    if (!session || !pending || !result) {
      throw new Error(`no pending request ${request}`)
    }
    session.pending.delete(pending.permission.request)
    session.lastActive = Date.now()
    pending.resolve(result)
    this.publish(id, { session: id, answered: pending.permission.request })
  }

  private publish(id: string, event: object) {
    for (const client of this.subscribers.get(id) ?? []) client(event)
  }
}

/** Parses one input line. A line that isn't a JSON object is an error. */
function parseRequest(line: string): Request {
  const value: unknown = JSON.parse(line)
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error("a request must be a JSON object")
  }
  return value
}

function sessionOf(request: Request) {
  if (!request.session) throw new Error(`${request.type} needs a session`)
  return request.session
}

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

/**
 * Adds the receive time to a user or assistant message that has none. Stored transcript messages
 * carry a timestamp, and the GUI shows one for every message.
 */
function stamped(message: SDKMessage) {
  if (message.type !== "user" && message.type !== "assistant") return message
  if ("timestamp" in message && message.timestamp) return message
  return { ...message, timestamp: new Date().toISOString() }
}

function checkMode(mode: unknown): PermissionMode {
  if (!MODES.includes(mode as PermissionMode)) {
    throw new Error(`mode must be one of ${MODES.join(", ")}`)
  }
  return mode as PermissionMode
}
