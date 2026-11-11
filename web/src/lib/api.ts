// Calls ryed's GUI API. Errors are RFC 9457 problem details.

/** A VM. agentManager is true once rye agent-manager install has run on it. */
export type VM = {
  name: string
  status: string
  agentManager?: boolean
}

/** A live session's turn and waiting approvals. */
export type SessionState = {
  running: boolean
  pending: number
  background?: number
}

export type SessionInfo = {
  sessionId: string
  summary: string
  customTitle?: string
  cwd?: string
  lastModified: number
}

/** A message in a session's transcript, as Claude's SDK reports it. */
export type Message = {
  type: string
  subtype?: string
  /** Set on system init and status messages. */
  permissionMode?: Mode
  /** Set on background_tasks_changed messages. Ambient tasks don't count. */
  tasks?: { ambient?: boolean }[]
  uuid?: string
  timestamp?: string
  /** Set on a synthetic assistant message, e.g. "authentication_failed" when Claude isn't logged in. */
  error?: string
  model?: string
  message?: { content?: unknown; model?: string }
  /** Set on a message from a subagent, to the tool use that started it. */
  parent_tool_use_id?: string | null
  /** Set on the summary that replaces a session's history in a compaction, in the stored transcript. */
  isCompactSummary?: boolean
  /** Set on a user message Claude Code wrote, such as the compaction summary as it streams. */
  isSynthetic?: boolean
  /** Set on a local command's output as it streams. */
  isReplay?: boolean
  /** Set on a local command and its output in the stored transcript. */
  isCompletedLocalCommand?: boolean
  /** Where a user message came from, such as a background task's notification. */
  origin?: { kind?: string }
  event?: {
    type: string
    delta?: { type: string; text?: string }
    content_block?: { type: string; name?: string }
  }
  errors?: string[]
  /** Set on api_retry messages: the retry number, the retry limit and the HTTP status (null without a response). */
  attempt?: number
  max_retries?: number
  error_status?: number | null
}

export type PermissionResult =
  | {
      behavior: "allow"
      updatedInput?: Record<string, unknown>
      updatedPermissions?: unknown[]
    }
  | { behavior: "deny"; message: string }

export type Permission = {
  request: string
  /** The tool use the request is for. Its transcript row shows as waiting. */
  toolUseID?: string
  tool: string
  input: Record<string, unknown>
  title?: string
  description?: string
  decisionReason?: string
  suggestions?: unknown[]
}

export type Effort = "low" | "medium" | "high" | "xhigh" | "max"

export const efforts: Effort[] = ["low", "medium", "high", "xhigh", "max"]

/** Permission modes the GUI offers. agent-manager refuses others. */
export type Mode = "default" | "auto" | "acceptEdits" | "plan"

export const modes: Mode[] = ["default", "acceptEdits", "plan", "auto"]

/** The names Claude's own apps give the modes. */
const MODE_LABELS: Record<Mode, string> = {
  default: "Manual",
  acceptEdits: "Accept edits",
  plan: "Plan",
  auto: "Auto",
}

/** The name Claude's own apps give a mode, or the mode itself for one the GUI doesn't offer. */
export const modeLabel = (mode: string) =>
  (MODE_LABELS as Record<string, string>)[mode] ?? mode

/** A model as Claude Code offers it to the VM's account. */
export type Model = {
  value: string
  resolvedModel?: string
  displayName: string
  description: string
  supportedEffortLevels?: Effort[]
}

export class ApiError extends Error {}

async function call<T>(method: string, path: string, body?: unknown) {
  const response = await fetch(`/api${path}`, {
    method,
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!response.ok) {
    const problem = (await response.json().catch(() => ({}))) as {
      detail?: string
    }
    throw new ApiError(
      problem.detail ?? `${method} ${path}: ${response.status}`,
    )
  }
  return (
    response.status === 200 || response.status === 201
      ? await response.json()
      : undefined
  ) as T
}

const vmPath = (vm: string) => `/vms/${encodeURIComponent(vm)}`
const sessionPath = (vm: string, session: string) =>
  `${vmPath(vm)}/sessions/${encodeURIComponent(session)}`

export const api = {
  vms: () => call<VM[]>("GET", "/vms"),
  sessions: (vm: string) =>
    call<{
      sessions: SessionInfo[]
      live: string[]
      states?: Record<string, SessionState>
    }>("GET", `${vmPath(vm)}/sessions`),
  models: (vm: string) =>
    call<{ models: Model[] }>("GET", `${vmPath(vm)}/models`),
  create: (
    vm: string,
    start: { cwd?: string; model: string; effort?: Effort; mode?: Mode },
  ) => call<{ session: string }>("POST", `${vmPath(vm)}/sessions`, start),
  mode: (vm: string, session: string, mode: Mode) =>
    call<void>("PUT", `${sessionPath(vm, session)}/mode`, { mode }),
  effort: (vm: string, session: string, effort?: Effort) =>
    call<void>(
      "PUT",
      `${sessionPath(vm, session)}/effort`,
      effort ? { effort } : {},
    ),
  rename: (vm: string, session: string, title: string) =>
    call<void>("PATCH", sessionPath(vm, session), { title }),
  send: (vm: string, session: string, content: string) =>
    call<void>("POST", `${sessionPath(vm, session)}/messages`, { content }),
  interrupt: (vm: string, session: string) =>
    call<void>("POST", `${sessionPath(vm, session)}/interrupt`),
  answer: (
    vm: string,
    session: string,
    request: string,
    result: PermissionResult,
  ) =>
    call<void>(
      "POST",
      `${sessionPath(vm, session)}/approvals/${encodeURIComponent(request)}`,
      result,
    ),
  /** The page before `before`, every message back to `from`, or with `all` every earlier message. */
  earlier: (
    vm: string,
    session: string,
    before: string,
    { from, all }: { from?: string; all?: boolean } = {},
  ) => {
    const query = new URLSearchParams({ before })
    if (from) query.set("from", from)
    if (all) query.set("all", "1")
    return call<{ messages: Message[]; more: boolean }>(
      "GET",
      `${sessionPath(vm, session)}/messages?${query}`,
    )
  },
  eventsURL: (vm: string, session: string) =>
    `/api${sessionPath(vm, session)}/events`,
}
