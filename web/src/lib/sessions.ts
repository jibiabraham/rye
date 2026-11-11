import type { SessionInfo, SessionState, VM } from "@/lib/api"

/** One VM's sessions, with the turn and waiting approvals of each live one. */
export type Listing = {
  sessions: SessionInfo[]
  live: string[]
  states: Record<string, SessionState>
}

/** Most recent first, or in a frozen order while the pointer or focus is in the list. */
export function ordered(list: SessionInfo[], frozen: string[] | null) {
  const byActivity = [...list].sort((a, b) => b.lastModified - a.lastModified)
  if (!frozen) return byActivity
  const place = new Map(frozen.map((id, i) => [id, i]))
  const known = byActivity.filter((s) => place.has(s.sessionId))
  known.sort((a, b) => place.get(a.sessionId)! - place.get(b.sessionId)!)
  return [...known, ...byActivity.filter((s) => !place.has(s.sessionId))]
}

/** The storage key of the VM opened last. */
export const LAST_VM = "rye:last-vm"

/** The VM home opens. It is the last one used, else the first running one, else the first. */
export function homeVM(vms: VM[], last?: string) {
  if (last && vms.some((v) => v.name === last)) return last
  return (vms.find((v) => v.status === "running") ?? vms[0])?.name
}

/**
 * True for a session that changed since it was last open on this browser. A session never
 * opened here has no record and counts as seen.
 */
export function unseen(
  session: SessionInfo,
  seenAt: number | undefined,
  state?: SessionState,
) {
  if (seenAt === undefined || state?.running) return false
  return session.lastModified > seenAt
}

/** Where a session's unsent message waits while the tab is open. A new session's page uses "new" for the session. */
export const draftKeyOf = (vm: string, session: string) =>
  `rye:draft:${vm}/${session}`
