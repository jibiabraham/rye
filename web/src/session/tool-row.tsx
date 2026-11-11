import { Check, CircleAlert, X } from "lucide-react"
import { Fragment } from "react"

import { Spinner } from "@/components/ui/spinner"
import {
  Trace,
  TraceCollapsible,
  TraceCollapsibleContent,
  TraceCollapsibleTrigger,
  TraceDescription,
  TraceIcon,
  TraceLine,
  TraceTitle,
} from "@/components/ui/trace"
import { duration } from "@/lib/time"
import { Diff } from "@/session/diff"
import { Elapsed } from "@/session/elapsed"
import { ImageBlock } from "@/session/image-block"
import type { Block } from "@/session/state"
import {
  answersOf,
  type Input,
  questionsOf,
  resultImages,
  resultText,
  str,
  summarized,
  toolSummary,
} from "@/session/tools"

type ToolUse = Extract<Block, { type: "tool_use" }>
type ToolResult = Extract<Block, { type: "tool_result" }>

const lineCount = (text: string) =>
  text ? text.trimEnd().split("\n").length : 0

/** A question the person skipped comes back as a denied tool call. */
const skipped = (name: string, result: ToolResult) =>
  name === "AskUserQuestion" && !!result.is_error

function resultNote(name: string, input: Input, result: ToolResult) {
  const text = resultText(result.content)
  if (skipped(name, result)) return "skipped"
  if (result.is_error) return "failed"
  if (name === "AskUserQuestion")
    return [...answersOf(input, result).values()].join(" · ")
  // An image's result has the CLI's note about its size in place of lines.
  if (name === "Read")
    return resultImages(result.content).length ? "" : `${lineCount(text)} lines`
  if (name === "Glob" || name === "Grep") return `${lineCount(text)} results`
  return ""
}

/** A tool call's input in full, such as a diff or a command, and its output once it has one. */
export function ToolDetail({
  block,
  result,
  titled = false,
}: {
  block: ToolUse
  result?: ToolResult
  /**
   * For an approval. Names the file above a change, because no row above names it, and shows the whole input with no
   * scroll box.
   */
  titled?: boolean
}) {
  const input = (block.input ?? {}) as Input
  const output = result ? resultText(result.content) : ""
  const outputClass = "code max-h-80 overflow-auto"
  const inputClass = titled ? "code overflow-auto" : outputClass
  const path = titled ? str(input.file_path) : undefined
  switch (block.name) {
    case "AskUserQuestion": {
      const answers =
        result && !result.is_error ? answersOf(input, result) : undefined
      return (
        <dl className="flex flex-col gap-2">
          {questionsOf(input).map(({ question }) => (
            <div key={question}>
              <dt className="text-muted-foreground">{question}</dt>
              <dd>
                {result
                  ? (answers?.get(question) ?? "Skipped")
                  : "Waiting for an answer"}
              </dd>
            </div>
          ))}
        </dl>
      )
    }
    case "Bash":
      return (
        <pre className={inputClass}>
          <span className="text-muted-foreground">$ </span>
          {str(input.command)}
          {output && `\n${output}`}
        </pre>
      )
    case "Edit":
      return (
        <div className="flex flex-col gap-2">
          <Diff
            path={path}
            before={str(input.old_string)}
            after={str(input.new_string)}
            full={titled}
          />
          {input.replace_all === true && (
            <p>Replaces every occurrence in the file.</p>
          )}
        </div>
      )
    case "MultiEdit":
      return (
        <div className="flex flex-col gap-2">
          {((input.edits ?? []) as Input[]).map((edit, i) => (
            <Fragment key={i}>
              <Diff
                path={i === 0 ? path : undefined}
                before={str(edit.old_string)}
                after={str(edit.new_string)}
                full={titled}
              />
              {edit.replace_all === true && (
                <p>Replaces every occurrence in the file.</p>
              )}
            </Fragment>
          ))}
        </div>
      )
    case "Write":
      return (
        <Diff path={path} before="" after={str(input.content)} full={titled} />
      )
    default:
      return (
        <div className="flex flex-col gap-2">
          {!summarized.has(block.name) && (
            <pre className={inputClass}>{JSON.stringify(input, null, 2)}</pre>
          )}
          {output && <pre className={outputClass}>{output}</pre>}
        </div>
      )
  }
}

/** A tool call, collapsed to one line. It spins while the tool runs. */
export function ToolRow({
  block,
  result,
  running,
  waiting = false,
  started,
  ended,
}: {
  block: ToolUse
  result?: ToolResult
  running: boolean
  /** Waiting for the person's approval. */
  waiting?: boolean
  /** When Claude called the tool and when its result arrived, as ISO 8601 times. */
  started?: string
  ended?: string
}) {
  const input = (block.input ?? {}) as Input
  const note = result ? resultNote(block.name, input, result) : ""
  const images = result ? resultImages(result.content) : []
  const failed = !!result?.is_error && !skipped(block.name, result)
  const done = !!result && !failed && !skipped(block.name, result)
  // Claude gives some calls a description, such as Bash commands. The row shows it in place of the summary.
  const description = str(input.description)
  const start = started ? Date.parse(started) : NaN
  const took = started && ended ? (Date.parse(ended) - start) / 1000 : undefined
  const status = waiting
    ? "Waiting for approval"
    : running
      ? "Running"
      : failed
        ? "Failed"
        : done
          ? "Done"
          : ""
  return (
    <Trace>
      <TraceCollapsible>
        <TraceCollapsibleTrigger>
          <TraceLine
            status={running ? "running" : failed ? "error" : undefined}
          >
            <TraceIcon>
              {waiting ? (
                <CircleAlert className="text-attention" />
              ) : running ? (
                <Spinner />
              ) : failed ? (
                <X />
              ) : done ? (
                <Check className="text-success" />
              ) : null}
            </TraceIcon>
            <TraceTitle>
              <span className="font-medium text-foreground">{block.name}</span>{" "}
              {description || (
                <span className="font-mono text-xs">
                  {toolSummary(block.name, input)}
                </span>
              )}
            </TraceTitle>
            {/* The title truncates its text but not this absolutely placed span. Inside the title, it would land past
                the cut-off text and widen the transcript. */}
            {status && <span className="sr-only">, {status}</span>}
            {note && (
              <TraceDescription className="max-w-2/5 truncate">
                {note}
              </TraceDescription>
            )}
            {waiting && (
              <TraceDescription aria-hidden>
                Waiting for approval
              </TraceDescription>
            )}
            {running && !Number.isNaN(start) && (
              <TraceDescription>
                <Elapsed since={start} />
              </TraceDescription>
            )}
            {!running && took !== undefined && took >= 1 && (
              <TraceDescription>{duration(took)}</TraceDescription>
            )}
          </TraceLine>
        </TraceCollapsibleTrigger>
        <TraceCollapsibleContent>
          <ToolDetail block={block} result={result} />
        </TraceCollapsibleContent>
      </TraceCollapsible>
      {images.map((image, i) => (
        <ImageBlock key={i} image={image} alt={`Image from ${block.name}`} />
      ))}
    </Trace>
  )
}
