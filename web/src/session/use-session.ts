import { useCallback, useEffect, useReducer, useRef, useState } from "react"

import { api, type Effort, type Mode } from "@/lib/api"
import { initial, reduce, type Event } from "@/session/state"

/** How long a stream may go without an event, ping included, before the GUI reconnects. */
const QUIET_MS = 40_000
/** How long a dropped stream may take to reconnect before the GUI says so. */
const DOWN_MS = 5_000

/**
 * Follows a session's event stream. EventSource reconnects after the stream ends, and history dedupes.
 * Events queue until the next animation frame and render as one batch.
 */
export function useSession(vm: string, session: string) {
  const [state, dispatch] = useReducer(reduce, initial)
  const [loadingEarlier, setLoadingEarlier] = useState(false)
  /** Bumped by retry, which opens a new stream. */
  const [generation, setGeneration] = useState(0)
  /** True while the stream is down or has gone quiet past QUIET_MS. */
  const [stale, setStale] = useState(false)
  const oldest = useRef<string | undefined>(undefined)
  useEffect(() => {
    oldest.current = state.entries.find((e) => !e.local)?.uuid
  }, [state.entries])

  useEffect(() => {
    const source = new EventSource(api.eventsURL(vm, session))
    let heard = Date.now()
    /** When the stream dropped, if it hasn't delivered an event since. */
    let dropped: number | undefined
    const alive = () => {
      heard = Date.now()
      dropped = undefined
      setStale(false)
    }
    // ryed sends pings. A stream down past DOWN_MS shows as reconnecting, and one quiet past QUIET_MS
    // is replaced.
    const watch = setInterval(() => {
      const now = Date.now()
      if (dropped !== undefined && now - dropped >= DOWN_MS) setStale(true)
      if (now - heard < QUIET_MS) return
      setStale(true)
      setGeneration((n) => n + 1)
    }, 1000)
    source.addEventListener("ping", alive)
    let queue: Event[] = []
    let frame = 0
    const flush = () => {
      frame = 0
      const events = queue
      queue = []
      dispatch({ name: "batch", events })
    }
    const on = (name: "state" | "history" | "message" | "ended") =>
      source.addEventListener(name, (e) => {
        alive()
        const data: unknown = JSON.parse(e.data as string)
        queue.push({ name, data } as Event)
        frame ||= requestAnimationFrame(flush)
      })
    on("state")
    on("history")
    on("message")
    on("ended")
    // agent-manager answered subscribe or messages with an error. Report it as the session's end.
    source.addEventListener("agent-manager-error", (e) => {
      alive()
      const data = JSON.parse(e.data as string) as { error?: string }
      queue.push({ name: "ended", data: { error: data.error } })
      frame ||= requestAnimationFrame(flush)
    })
    source.onerror = () => {
      if (source.readyState === EventSource.CONNECTING) dropped ??= Date.now()
      if (source.readyState === EventSource.CLOSED) {
        dispatch({
          name: "ended",
          data: { error: "agent-manager in this VM is unreachable" },
        })
      }
    }
    return () => {
      clearInterval(watch)
      cancelAnimationFrame(frame)
      source.close()
    }
  }, [vm, session, generation])

  /** Loads the page before the oldest loaded message, back to `from`, or with `all` everything. */
  const loadEarlier = useCallback(
    async (options: { from?: string; all?: boolean } = {}) => {
      const before = oldest.current
      if (!before) return
      setLoadingEarlier(true)
      try {
        dispatch({
          name: "earlier",

          data: await api.earlier(vm, session, before, options),
        })
      } finally {
        setLoadingEarlier(false)
      }
    },
    [vm, session],
  )

  return {
    state,
    stale,
    loadingEarlier,
    loadEarlier,
    /** Takes back a sent message that agent-manager didn't take. */
    unsent: (id: string, wasRunning: boolean) =>
      dispatch({ name: "unsent", id, wasRunning }),
    retry: () => setGeneration((n) => n + 1),
    setEffort: (effort?: Effort) => dispatch({ name: "effort", effort }),
    setMode: (mode: Mode) => dispatch({ name: "mode", mode }),
    /** Shows a message as sent, until the transcript has it. Returns its local ID. */
    sent: (text: string) => {
      const id = `local-${crypto.randomUUID()}`
      dispatch({ name: "sent", text, id, at: Date.now() })
      return id
    },
  }
}
