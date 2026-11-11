import { Pencil } from "lucide-react"
import { Link, useNavigate } from "@tanstack/react-router"
import { useEffect, useEffectEvent, useMemo, useState } from "react"

import { api, type Model } from "@/lib/api"
import { errorMessage } from "@/lib/errors"
import { draftKeyOf } from "@/lib/sessions"
import { load, save } from "@/lib/storage"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ErrorNote } from "@/components/error-note"
import { Status } from "@/components/status"
import { PageHeader } from "@/components/page-header"
import { useFrom } from "@/hooks/navigation"
import { findModel } from "@/lib/models"
import { EffortSelect, ModeSelect } from "@/components/selects"
import { ActivityLine } from "@/session/activity"
import { Approval } from "@/session/approval"
import { Composer, composerSelect } from "@/session/composer"
import { takeFirst } from "@/session/first-message"
import {
  runningTools,
  toolEnds,
  toolResults,
  unanswered,
} from "@/session/state"
import { TranscriptPane } from "@/session/transcript-pane"
import { useSession } from "@/session/use-session"

export function SessionView(props: {
  vm: string
  session: string
  title: string
  /**
   * Called when the session's row in the list may have changed: after a send or a rename, and when the stream
   * reports a new or answered approval, a turn's start or end, or a change in background tasks.
   */
  onChange: () => void
  /** The session's working directory, from the list. */
  cwd?: string
  /** Called when agent-manager reports no session with this ID. */
  onMissing: () => void
  /** A message to scroll to and highlight, from the URL's hash. */
  message?: string
  models?: Model[]
  /** The model value a new session started with, shown until a reply names the model. */
  startModel?: string
}) {
  const { vm, session, onChange } = props
  const {
    state,
    sent,
    unsent,
    retry,
    loadingEarlier,
    loadEarlier,
    setEffort,
    setMode,
    stale,
  } = useSession(vm, session)
  const { onMissing } = props
  const navigate = useNavigate()
  const from = useFrom()
  useEffect(() => {
    if (!state.exists) onMissing()
  }, [state.exists, onMissing])
  const model = findModel(props.models ?? [], state.model ?? props.startModel)
  const [renaming, setRenaming] = useState(false)
  const draftKey = draftKeyOf(vm, session)
  // The unsent message lasts while the tab is open.
  const [text, setText] = useState(() => load(draftKey, "", "session"))
  useEffect(
    () => save(draftKey, text || undefined, "session"),
    [draftKey, text],
  )
  const [error, setError] = useState<string>()
  const [sends, setSends] = useState(0)
  const report = (e: unknown) => setError(errorMessage(e))
  /** Runs an action. A failure's error shows until an action succeeds. Resolves to whether this one did. */
  const act = (action: Promise<unknown>) =>
    action.then(
      () => {
        setError(undefined)
        return true
      },
      (e: unknown) => {
        report(e)
        return false
      },
    )

  const results = useMemo(() => toolResults(state.entries), [state.entries])
  const ends = useMemo(() => toolEnds(state.entries), [state.entries])
  const waiting = useMemo(
    () => new Set(state.pending.flatMap((p) => p.toolUseID ?? [])),
    [state.pending],
  )
  // A tool waiting for approval hasn't started.
  const running = useMemo(
    () =>
      new Set(
        runningTools(state)
          .filter((t) => !waiting.has(t.id))
          .map((t) => t.id),
      ),
    [state, waiting],
  )

  const target = props.message
  /** Drops the message from the URL. A reload then opens the latest messages. */
  const leaveMessage = () => {
    if (!target) return
    void navigate({
      to: "/vms/$vm/sessions/$session",
      params: { vm, session },
      replace: true,
    })
  }

  const listed = `${state.live}/${state.running}/${state.pending.length}/${state.background}`
  useEffect(() => {
    if (state.loaded) onChange()
  }, [state.loaded, listed, onChange])

  const send = () => sendText(text.trim())
  const sendText = async (content: string) => {
    if (!content) return
    setText("")
    setError(undefined)
    setSends((n) => n + 1)
    leaveMessage()
    const wasRunning = state.running
    const id = sent(content)
    try {
      await api.send(vm, session, content)
      onChange()
    } catch (e) {
      // The message goes back in the box, ahead of anything typed since.
      unsent(id, wasRunning)
      setText((now) => (now ? `${content}\n\n${now}` : content))
      report(e)
    }
  }
  const sendFirst = useEffectEvent((content: string) => void sendText(content))
  // The first message's events reach the page because history arrives after agent-manager has subscribed it.
  useEffect(() => {
    if (!state.loaded) return
    const first = takeFirst(vm, session)
    if (first) sendFirst(first)
  }, [state.loaded, vm, session])
  const rename = async (title: string) => {
    setRenaming(false)
    if (!title.trim() || title === props.title) return
    if (await act(api.rename(vm, session, title.trim()))) onChange()
  }
  /** True from a Stop until the turn ends. */
  const [stopping, setStopping] = useState(false)
  if (stopping && !state.running) setStopping(false)
  const stop = () => {
    if (stopping) return
    setStopping(true)
    void act(api.interrupt(vm, session)).then((ok) => {
      if (!ok) setStopping(false)
    })
  }

  return (
    <section className="flex h-full min-h-0 flex-col">
      <PageHeader>
        {renaming ? (
          <Input
            aria-label="Session title"
            // Focus moves here because the Rename button it replaces is gone.
            ref={(el) => el?.focus()}
            defaultValue={props.title}
            className="h-8 max-w-sm"
            onBlur={(e) => void rename(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void rename(e.currentTarget.value)
              if (e.key === "Escape") setRenaming(false)
            }}
          />
        ) : (
          <div className="flex min-w-0 flex-col px-1">
            <h2
              tabIndex={-1}
              className="truncate leading-5 font-medium outline-none"
            >
              {props.title}
            </h2>
            <p className="truncate text-xs leading-4 text-muted-foreground">
              <Link
                to="/vms/$vm"
                params={{ vm }}
                state={from}
                className="hover:text-foreground hover:underline"
              >
                {vm}
              </Link>
              {props.cwd && <span> · {props.cwd}</span>}
            </p>
          </div>
        )}
        {!renaming && (
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Rename"
            onClick={() => setRenaming(true)}
          >
            <Pencil />
          </Button>
        )}
      </PageHeader>
      <TranscriptPane
        place={`${vm}/${session}`}
        state={state}
        transcript={{
          entries: state.entries,
          results,
          running,
          waiting,
          ends,
          turnRunning: state.running,
        }}
        loadingEarlier={loadingEarlier}
        loadEarlier={loadEarlier}
        target={target}
        sends={sends}
        onLatest={leaveMessage}
        onError={report}
      >
        {state.pending.map((p) => (
          <Approval
            key={p.request}
            vm={vm}
            session={session}
            permission={p}
            act={act}
          />
        ))}
        {state.loginNeeded && (
          <ErrorNote>
            Claude isn't logged in on {vm}. Log in to the VM over SSH, run
            claude and type /login.
          </ErrorNote>
        )}
        {unanswered(state) && <ErrorNote>Turn ended without a reply</ErrorNote>}
        {state.notice &&
          (state.notice.error ? (
            <ErrorNote onRetry={state.live ? undefined : retry}>
              {state.notice.text}
            </ErrorNote>
          ) : (
            <Status>{state.notice.text}</Status>
          ))}
        {error && <ErrorNote>{error}</ErrorNote>}
      </TranscriptPane>
      {/* The same column as the messages above. */}
      <div className="px-4 pb-4">
        <div className="mx-auto max-w-3xl">
          <div className="mb-1.5 px-1">
            <ActivityLine state={state} stale={stale} stopping={stopping} />
          </div>
          <Composer
            text={text}
            onText={setText}
            onSend={() => void send()}
            onStop={state.running ? stop : undefined}
            stopping={stopping}
            blocked={state.lost}
          >
            <ModeSelect
              size="sm"
              className={composerSelect}
              value={state.mode}
              onChange={(mode) => {
                const was = state.mode ?? "default"
                setMode(mode)
                void act(api.mode(vm, session, mode)).then((ok) => {
                  if (!ok) setMode(was)
                })
              }}
            />
            <EffortSelect
              size="sm"
              className={composerSelect}
              model={model}
              value={state.effort}
              onChange={(effort) => {
                const was = state.effort
                setEffort(effort)
                void act(api.effort(vm, session, effort)).then((ok) => {
                  if (!ok) setEffort(was)
                })
              }}
            />
            <span className="truncate text-xs text-muted-foreground max-sm:hidden">
              {model?.displayName ?? state.model}
            </span>
          </Composer>
        </div>
      </div>
    </section>
  )
}
