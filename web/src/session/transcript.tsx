import { memo, useMemo } from "react"

import { CopyButton } from "@/components/copy-button"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Message, MessageContent, MessageFooter } from "@/components/ui/message"
import { shortTime } from "@/lib/time"
import {
  MessageScrollerContent,
  MessageScrollerItem,
} from "@/components/ui/message-scroller"
import { ImageBlock } from "@/session/image-block"
import { Markdown } from "@/session/markdown"
import { turnTexts, visible, type Block, type Entry } from "@/session/state"
import { ToolRow } from "@/session/tool-row"

type Results = Map<string, Extract<Block, { type: "tool_result" }>>

type EntryProps = {
  entry: Entry
  results: Results
  running: Set<string>
  /** Tool uses waiting for the person's approval or answer. */
  waiting: Set<string>
  /** When each tool's result arrived, by tool use ID. */
  ends: Map<string, string>
  highlighted: boolean
  /** The turn's text, on the turn's last reply. Its footer copies it. */
  turn?: string
  /** True for the last message. Its footer always shows. */
  last: boolean
}

/** Shows on hover or focus where the device can hover, and always on touch screens. */
const revealed =
  "[@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover/entry:opacity-100 [@media(hover:hover)]:group-focus-within/entry:opacity-100"

/**
 * A message's footer: Copy, the time and Copy link. Each of the person's messages has one, and each of Claude's turns
 * has one under its last reply. It shows on hover or focus, and always on the last message.
 */
function Footer(props: { entry: Entry; copy: string; last: boolean }) {
  const { entry } = props
  const at = entry.at ? new Date(entry.at) : undefined
  return (
    <MessageFooter
      className={`gap-1 px-0 ${props.last ? "" : revealed} ${entry.role === "user" ? "justify-end" : ""}`}
    >
      {props.copy && (
        <CopyButton
          label={entry.role === "user" ? "Copy message" : "Copy reply"}
          text={() => props.copy}
        />
      )}
      {at && (
        <time dateTime={entry.at} className="whitespace-nowrap tabular-nums">
          {shortTime(at)}
        </time>
      )}
      {!entry.local && (
        <CopyButton
          icon="link"
          label="Copy link to message"
          text={() => `${location.origin}${location.pathname}#${entry.uuid}`}
        />
      )}
    </MessageFooter>
  )
}

/** One message. It re-renders only when its own content, tool state or highlight changes. */
const EntryItem = memo(function EntryItem(props: EntryProps) {
  const { entry, results, running, waiting, ends, highlighted, turn } = props
  const ring = highlighted
    ? "ring-2 ring-ring ring-offset-4 ring-offset-background"
    : ""
  const user = entry.role === "user"
  const text = user
    ? entry.blocks.map((b) => (b.type === "text" ? b.text : "")).join("")
    : turn
  if (entry.compacted) {
    return (
      <MessageScrollerItem
        messageId={entry.uuid}
        id={entry.uuid}
        className="group/entry [content-visibility:visible]"
      >
        <div
          role="separator"
          className="flex items-center gap-3 text-xs text-muted-foreground before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border"
        >
          Conversation compacted
        </div>
      </MessageScrollerItem>
    )
  }
  const images = entry.blocks.flatMap((b) => (b.type === "image" ? [b] : []))
  return (
    <MessageScrollerItem
      messageId={entry.uuid}
      id={entry.uuid}
      // Landing on a linked message needs the real height of the messages above it.
      className="group/entry [content-visibility:visible]"
    >
      <Message align={user ? "end" : "start"}>
        <MessageContent>
          {user ? (
            <>
              {text && (
                <Bubble variant="secondary" align="end" className={ring}>
                  <BubbleContent className="rounded-2xl px-4 text-base whitespace-pre-wrap">
                    {text}
                  </BubbleContent>
                </Bubble>
              )}
              {images.map((image, i) => (
                <ImageBlock key={i} image={image} alt="Image" />
              ))}
            </>
          ) : (
            <div className={`flex flex-col gap-3 rounded-lg ${ring}`}>
              {entry.blocks.map((block, i) =>
                block.type === "tool_use" &&
                block.name === "AskUserQuestion" &&
                waiting.has(block.id) ? null : block.type === "text" ? (
                  <Markdown key={i} text={block.text} />
                ) : block.type === "image" ? (
                  <ImageBlock key={i} image={block} alt="Image" />
                ) : block.type === "tool_use" ? (
                  <ToolRow
                    key={i}
                    block={block}
                    result={results.get(block.id)}
                    running={running.has(block.id)}
                    waiting={waiting.has(block.id)}
                    started={entry.at}
                    ended={ends.get(block.id)}
                  />
                ) : null,
              )}
            </div>
          )}
          {text !== undefined && (
            <Footer entry={entry} copy={text} last={props.last} />
          )}
        </MessageContent>
      </Message>
    </MessageScrollerItem>
  )
}, sameEntry)

function sameEntry(a: EntryProps, b: EntryProps) {
  if (a.entry !== b.entry || a.highlighted !== b.highlighted) return false
  if (a.turn !== b.turn || a.last !== b.last) return false
  for (const block of a.entry.blocks) {
    if (block.type !== "tool_use") continue
    if (a.results.get(block.id) !== b.results.get(block.id)) return false
    if (a.running.has(block.id) !== b.running.has(block.id)) return false
    if (a.waiting.has(block.id) !== b.waiting.has(block.id)) return false
    if (a.ends.get(block.id) !== b.ends.get(block.id)) return false
  }
  return true
}

export type TranscriptProps = {
  entries: Entry[]
  results: Results
  running: Set<string>
  /** Tool uses waiting for the person's approval or answer. */
  waiting: Set<string>
  ends: Map<string, string>
  highlighted?: string
  /** True while Claude's turn runs. The turn's footer waits for its end. */
  turnRunning: boolean
}

/** The messages with something to show, as a log that screen readers announce as messages arrive. */
export function Transcript(props: TranscriptProps) {
  const turns = useMemo(() => turnTexts(props.entries), [props.entries])
  const shown = props.entries.filter(visible)
  return (
    <MessageScrollerContent aria-label="Messages" className="gap-3">
      {shown.map((entry, i) => (
        <EntryItem
          key={entry.uuid}
          entry={entry}
          results={props.results}
          running={props.running}
          waiting={props.waiting}
          ends={props.ends}
          highlighted={entry.uuid === props.highlighted}
          turn={
            props.turnRunning && i === shown.length - 1
              ? undefined
              : turns.get(entry.uuid)
          }
          last={i === shown.length - 1}
        />
      ))}
    </MessageScrollerContent>
  )
}
