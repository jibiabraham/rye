import { diffLines } from "diff"

import { modeLabel } from "@/lib/api"

import { imageOf, type Block } from "@/session/state"

type ToolResult = Extract<Block, { type: "tool_result" }>
export type Input = Record<string, unknown>

export const str = (value: unknown) => (typeof value === "string" ? value : "")

export const resultText = (content: unknown): string =>
  typeof content === "string"
    ? content
    : Array.isArray(content)
      ? content
          .map((c: { type?: unknown; text?: unknown } | null) =>
            c?.type === "text" ? str(c.text) : "",
          )
          .join("\n")
      : ""

/** The images in a tool result's content, of the types the GUI shows. */
export const resultImages = (content: unknown) =>
  Array.isArray(content)
    ? content.flatMap((c: unknown) => imageOf(c) ?? [])
    : []

/** Tools whose input the summary line already shows in full. */
export const summarized = new Set([
  "Read",
  "Grep",
  "Glob",
  "WebFetch",
  "WebSearch",
])

/** A one-line summary of a tool call from its input. */
export function toolSummary(name: string, input: Input) {
  switch (name) {
    case "Bash":
      return str(input.command).replace(/\n+/g, " ⏎ ")
    case "AskUserQuestion":
      return questionsOf(input)
        .map((q) => q.question)
        .join(" · ")
    case "Read":
    case "Write":
    case "Edit":
    case "MultiEdit":
    case "NotebookEdit":
      return str(input.file_path) || str(input.notebook_path)
    case "Grep":
      return [str(input.pattern), str(input.path) || str(input.glob)]
        .filter(Boolean)
        .join(" in ")
    case "Glob":
      return str(input.pattern)
    case "WebFetch":
      return str(input.url)
    case "WebSearch":
      return str(input.query)
    default:
      return str(input.description)
  }
}

type Question = { question: string }

export const questionsOf = (input: Input): Question[] =>
  Array.isArray(input.questions)
    ? (input.questions as Question[]).filter(
        (q) => typeof q?.question === "string",
      )
    : []

/**
 * The answer to each question, from the result Claude Code records:
 * `Your questions have been answered: "<question>"="<answer>", ... You can now continue ...`.
 */
export function answersOf(input: Input, result: ToolResult) {
  const text = resultText(result.content)
  const answers = new Map<string, string>()
  for (const { question } of questionsOf(input)) {
    const start = text.indexOf(`"${question}"="`)
    if (start < 0) continue
    const from = start + question.length + 4
    const end = text.slice(from).search(/"(, "|\. |\.?$)/)
    answers.set(
      question,
      end < 0 ? text.slice(from) : text.slice(from, from + end),
    )
  }
  return answers
}

type Rule = { toolName?: unknown; ruleContent?: unknown }
type Suggestion = {
  type?: unknown
  behavior?: unknown
  rules?: Rule[]
  directories?: unknown[]
  mode?: unknown
  destination?: unknown
}

/** What a suggestion allows: tool rules such as `Bash(go test:*)`, directories, and all edits for Accept edits. */
function allowed(s: Suggestion) {
  if (s.type === "addRules" && s.behavior === "allow") {
    return (s.rules ?? []).flatMap((r) =>
      typeof r.toolName !== "string"
        ? []
        : typeof r.ruleContent === "string" && r.ruleContent
          ? [`${r.toolName}(${r.ruleContent})`]
          : [r.toolName],
    )
  }
  if (s.type === "addDirectories") {
    return (s.directories ?? []).filter((d) => typeof d === "string")
  }
  if (s.type === "setMode" && s.mode === "acceptEdits") return ["all edits"]
  return []
}

/**
 * The label of the button that approves a call with the permission rules Claude suggests. It names what the rules
 * allow, and says "for this session" when none of them outlasts the session.
 */
export function allowLabel(suggestions: unknown[] = []) {
  const list = suggestions as Suggestion[]
  const what = list.flatMap(allowed).join(", ")
  const lasting = list.some(
    (s) => s.destination !== "session" && s.destination !== "cliArg",
  )
  const mode = list.find((s) => s.type === "setMode")?.mode
  const switches =
    typeof mode === "string" && mode !== "acceptEdits"
      ? ` and switch to ${modeLabel(mode)}`
      : ""
  return lasting
    ? `Always allow${what ? ` ${what}` : ""}${switches}`
    : `Allow${what ? ` ${what}` : ""} for this session${switches}`
}

/** The lines of a change, each marked added, removed or unchanged. */
export function changedLines(before: string, after: string) {
  return diffLines(before, after).flatMap((part) =>
    part.value
      .replace(/\n$/, "")
      .split("\n")
      .map((text) => ({
        text,
        kind: part.added
          ? ("added" as const)
          : part.removed
            ? ("removed" as const)
            : ("default" as const),
      })),
  )
}
