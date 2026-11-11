import { ArrowDown } from "lucide-react"
import { type ReactNode, useEffect, useRef, useState } from "react"

import { Button } from "@/components/ui/button"
import { Status } from "@/components/status"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerProvider,
  MessageScrollerViewport,
  useMessageScroller,
  useMessageScrollerScrollable,
  useMessageScrollerVisibility,
} from "@/components/ui/message-scroller"
import { Markdown } from "@/session/markdown"
import { landing, type State } from "@/session/state"
import { Transcript, type TranscriptProps } from "@/session/transcript"

/**
 * Where the reader was in each session this page has shown, by VM and session. An entry holds the message in view,
 * or marks that the reader followed the latest output.
 */
const places = new Map<string, { following: boolean; uuid?: string }>()

const missingMessage = "The linked message isn't in this session."

type PaneProps = {
  /** The session's VM and ID, which key the reader's place. */
  place: string
  state: State
  transcript: Omit<TranscriptProps, "highlighted">
  loadingEarlier: boolean
  loadEarlier: (options?: { from?: string; all?: boolean }) => Promise<void>
  /** A message to land on and highlight, from the URL's hash. */
  target?: string
  /** Counts sends. Each one returns the view to the latest output. */
  sends: number
  /** Called when the reader leaves a linked message for the latest output. */
  onLatest: () => void
  onError: (e: unknown) => void
  /** Approvals, notes and errors under the transcript. */
  children: ReactNode
}

/**
 * The scrolling transcript. It follows the latest output, keeps the reader's place while earlier pages load above,
 * lands on a linked message, and returns to where the reader was on a later visit.
 */
export function TranscriptPane(props: PaneProps) {
  const [saved] = useState(() => places.get(props.place))
  return (
    <MessageScrollerProvider autoScroll defaultScrollPosition="end">
      <Pane {...props} saved={props.target ? undefined : saved} />
    </MessageScrollerProvider>
  )
}

function Pane(
  props: PaneProps & { saved?: { following: boolean; uuid?: string } },
) {
  const { state, loadingEarlier, loadEarlier, target, saved, onError } = props
  const { scrollToEnd, scrollToMessage } = useMessageScroller()
  const scrollable = useMessageScrollerScrollable()
  const { visibleMessageIds } = useMessageScrollerVisibility()
  const firstVisible = visibleMessageIds.at(0)
  const viewport = useRef<HTMLDivElement>(null)
  const top = useRef<HTMLDivElement>(null)
  const [highlighted, setHighlighted] = useState<string>()
  const [linkNote, setLinkNote] = useState<string>()
  /** True once the view has scrolled to its first place. The top of the transcript shows before then. */
  const [scrolled, setScrolled] = useState(false)

  // The place a later visit returns to.
  const place = useRef({
    following: true,
    uuid: undefined as string | undefined,
  })
  useEffect(() => {
    place.current = {
      following: !scrollable.end,
      uuid: firstVisible,
    }
  }, [scrollable.end, firstVisible])
  const key = props.place
  useEffect(() => () => void places.set(key, place.current), [key])

  // A send returns the view to the latest output.
  const sends = props.sends
  useEffect(() => {
    if (sends > 0) scrollToEnd()
  }, [sends, scrollToEnd])

  // Earlier messages load when the top of the transcript comes near.
  useEffect(() => {
    const target = top.current
    const root = viewport.current
    if (!target || !root || !scrolled || !state.more || loadingEarlier) return
    // The first scroll arms it. A first page that doesn't fill the view never scrolls, and loads more only by the
    // button.
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) loadEarlier().catch(onError)
      },
      { root, rootMargin: "600px 0px 0px 0px" },
    )
    observer.observe(target)
    return () => observer.disconnect()
  }, [scrolled, state.more, loadingEarlier, loadEarlier, onError])

  // A linked message, or the message of an earlier visit, loads if it is older than the loaded pages, then scrolls
  // into view once. Later events leave the reader's scroll alone.
  const goal = target ?? (saved && !saved.following ? saved.uuid : undefined)
  const handled = useRef<string | undefined>(undefined)
  // The jump and the highlight's end are timers that outlive a rerun of the effect. A streaming session reruns it
  // every few frames. The timers stop only when the pane goes.
  const timers = useRef({ jump: 0, hide: 0, note: 0 })
  useEffect(() => {
    const t = timers.current
    return () => {
      cancelAnimationFrame(t.jump)
      clearTimeout(t.hide)
      clearTimeout(t.note)
    }
  }, [])
  useEffect(() => {
    const t = timers.current
    if (!goal || handled.current === goal || t.jump) return
    if (!state.loaded || loadingEarlier) return
    const land = landing(state.entries, goal)
    if (land) {
      // The scroller keeps the reader's place when a page loads above. The jump waits a frame, until it has.
      t.jump = requestAnimationFrame(() => {
        t.jump = 0
        handled.current = goal
        scrollToMessage(land, {
          align: target ? "center" : "start",
          behavior: "instant",
        })
        if (!target) return
        setHighlighted(land)
        t.hide = window.setTimeout(() => setHighlighted(undefined), 2500)
      })
      return
    }
    if (state.more) {
      loadEarlier({ from: goal }).catch(() => {
        handled.current = goal
        if (target) setLinkNote(missingMessage)
      })
      return
    }
    handled.current = goal
    if (target) t.note = window.setTimeout(() => setLinkNote(missingMessage))
  }, [
    goal,
    target,
    state.loaded,
    state.entries,
    state.more,
    loadingEarlier,
    loadEarlier,
    scrollToMessage,
  ])

  return (
    <MessageScroller className="min-h-0 flex-1">
      <MessageScrollerViewport
        ref={viewport}
        preserveScrollOnPrepend
        onScroll={() => setScrolled(true)}
        className="px-4 pt-6 pb-14"
      >
        <div className="mx-auto flex max-w-3xl flex-col gap-4">
          {state.more && (
            <>
              <Button
                variant="ghost"
                size="sm"
                className="self-center text-muted-foreground"
                disabled={loadingEarlier}
                onClick={() => {
                  loadEarlier({ all: true }).catch(onError)
                }}
              >
                Load all earlier messages
              </Button>
              <div ref={top} className="h-px" />
            </>
          )}
          {!state.loaded && <Status>Loading messages…</Status>}
          {loadingEarlier && (
            <Status className="text-center">Loading earlier messages…</Status>
          )}
          <Transcript {...props.transcript} highlighted={highlighted} />
          {state.draft && <Markdown text={state.draft} />}
          {linkNote && target && <Status>{linkNote}</Status>}
          {props.children}
        </div>
      </MessageScrollerViewport>
      <MessageScrollerButton
        variant="outline"
        size="sm"
        className="rounded-full shadow-sm dark:bg-background"
        onClick={props.onLatest}
      >
        <ArrowDown />
        Jump to latest
      </MessageScrollerButton>
    </MessageScroller>
  )
}
