import { CircleAlert, WifiOff } from "lucide-react"
import { useState } from "react"

import { Marker, MarkerContent, MarkerIcon } from "@/components/ui/marker"
import { Spinner } from "@/components/ui/spinner"
import { useNow } from "@/hooks/use-now"
import { duration } from "@/lib/time"
import { runningTools, type State } from "@/session/state"

/** What Claude is doing now. Tool rows in the transcript say which tool and why. */
function label(state: State) {
  const tools = runningTools(state)
  if (tools.length > 1) return `Running ${tools.length} tools`
  if (tools.length === 1) return "Running"
  switch (state.activity?.kind) {
    case "starting":
      return "Starting Claude"
    case "thinking":
      return "Thinking"
    case "preparing":
      return `Preparing ${state.activity.tool}`
    case "writing":
      return "Writing"
    case "retrying": {
      const { attempt, max, status } = state.activity
      const cause =
        status === null
          ? "No connection to the API."
          : `The API refused the request (${status}).`
      return `${cause} Retry ${attempt} of ${max}.`
    }
    default:
      return "Working"
  }
}

/**
 * One line above the message box while Claude works. It shows what Claude is doing, how long that step has run, and
 * the turn's total. It also shows waiting on the person, background tasks after a turn, and a lost connection.
 */
export function ActivityLine({
  state,
  stale,
  stopping,
}: {
  state: State
  /** True while the stream from agent-manager is down or has gone quiet. */
  stale?: boolean
  /** True from a Stop until the turn ends. */
  stopping?: boolean
}) {
  const waiting = state.pending.length > 0
  const busy = state.running || state.background > 0
  const now = useNow(busy && !waiting)
  const text = !state.running ? "" : stopping ? "Stopping" : label(state)
  // The current step starts when its label changes.
  const [step, setStep] = useState(() => ({ text, at: Date.now() }))
  if (step.text !== text) setStep({ text, at: now })

  if (stale) {
    return (
      <Marker role="status">
        <MarkerIcon>
          <WifiOff />
        </MarkerIcon>
        <MarkerContent>Reconnecting to agent-manager…</MarkerContent>
      </Marker>
    )
  }
  if (state.running && waiting && !stopping) {
    const question = state.pending.every((p) => p.tool === "AskUserQuestion")
    return (
      <Marker role="status">
        <MarkerIcon>
          <CircleAlert className="text-attention" />
        </MarkerIcon>
        <MarkerContent>
          {question ? "Waiting for your answer" : "Waiting for your approval"}
        </MarkerContent>
      </Marker>
    )
  }
  if (state.running) {
    const stepSeconds = (now - step.at) / 1000
    const turnSeconds = state.since ? (now - state.since) / 1000 : undefined
    return (
      <Marker role="status">
        <MarkerIcon>
          <Spinner />
        </MarkerIcon>
        <MarkerContent className="truncate">{text}</MarkerContent>
        {/* The counters are hidden from screen readers because they change every second. */}
        <span aria-hidden className="shrink-0 tabular-nums">
          · {duration(stepSeconds)}
        </span>
        {turnSeconds !== undefined && turnSeconds - stepSeconds >= 1 && (
          <span aria-hidden className="ml-auto shrink-0 tabular-nums">
            turn {duration(turnSeconds)}
          </span>
        )}
      </Marker>
    )
  }
  if (state.background > 0) {
    return (
      <Marker role="status">
        <MarkerIcon>
          <Spinner />
        </MarkerIcon>
        <MarkerContent>
          {state.background === 1
            ? "1 background task running"
            : `${state.background} background tasks running`}
        </MarkerContent>
      </Marker>
    )
  }
  return null
}
