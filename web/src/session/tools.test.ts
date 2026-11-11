import { describe, expect, it } from "vitest"

import {
  allowLabel,
  answersOf,
  changedLines,
  resultImages,
  resultText,
  toolSummary,
} from "@/session/tools"

describe("toolSummary", () => {
  it.each([
    ["Bash", { command: "set -e\ncurl a" }, "set -e ⏎ curl a"],
    ["Read", { file_path: "/a/b.go" }, "/a/b.go"],
    ["NotebookEdit", { notebook_path: "/n.ipynb" }, "/n.ipynb"],
    ["Grep", { pattern: "TODO", path: "src" }, "TODO in src"],
    ["Grep", { pattern: "TODO", glob: "*.go" }, "TODO in *.go"],
    ["Glob", { pattern: "**/*.ts" }, "**/*.ts"],
    ["WebFetch", { url: "https://x.dev" }, "https://x.dev"],
    ["WebSearch", { query: "rye" }, "rye"],
    [
      "AskUserQuestion",
      { questions: [{ question: "A?" }, { question: "B?" }] },
      "A? · B?",
    ],
    ["Agent", { description: "Find the bug" }, "Find the bug"],
    ["Unknown", { command: 1 }, ""],
  ])("summarizes a %s call with %j as %j", (name, input, want) => {
    expect(toolSummary(name, input)).toBe(want)
  })
})

describe("resultImages", () => {
  it("keeps png, jpeg, gif and webp images and leaves out svg", () => {
    const image = (media_type: string) => ({
      type: "image",
      source: { type: "base64", media_type, data: "AAAA" },
    })
    expect(
      resultImages([
        image("image/png"),
        { type: "text", text: "[Image: original 1568x3290]" },
        image("image/svg+xml"),
      ]),
    ).toEqual([{ type: "image", media_type: "image/png", data: "AAAA" }])
  })
})

describe("resultText", () => {
  it("returns a string result as it is", () => {
    expect(resultText("out")).toBe("out")
  })

  it("joins the text blocks and skips the rest", () => {
    expect(
      resultText([
        { type: "text", text: "a" },
        { type: "image" },
        { type: "text", text: "b" },
      ]),
    ).toBe("a\n\nb")
  })

  it("returns nothing for any other content", () => {
    expect(resultText(5)).toBe("")
  })
})

describe("answersOf", () => {
  const input = {
    questions: [{ question: "Which check?" }, { question: "Keep it?" }],
  }
  const answers = (content: string) =>
    Object.fromEntries(
      answersOf(input, { type: "tool_result", tool_use_id: "q", content }),
    )

  it("reads each question's answer", () => {
    expect(
      answers(
        'Your questions have been answered: "Which check?"="Use forge, then dev2", "Keep it?"="Yes". You can now continue.',
      ),
    ).toEqual({ "Which check?": "Use forge, then dev2", "Keep it?": "Yes" })
  })

  it("reads an answer at the end of the text", () => {
    expect(
      answers('Your questions have been answered: "Which check?"="Both"'),
    ).toEqual({ "Which check?": "Both" })
  })

  it("gives no answer for a question missing from the text", () => {
    expect(answers("The user denied this.")).toEqual({})
  })
})

describe("allowLabel", () => {
  const rule = (
    toolName: string,
    ruleContent?: string,
    destination = "localSettings",
  ) => ({
    type: "addRules",
    behavior: "allow",
    destination,
    rules: [{ toolName, ruleContent }],
  })

  it("names a lasting tool rule with its content", () => {
    expect(allowLabel([rule("Bash", "go test:*")])).toBe(
      "Always allow Bash(go test:*)",
    )
  })

  it("names whole tools and directories", () => {
    expect(
      allowLabel([
        rule("Edit"),
        {
          type: "addDirectories",
          directories: ["/srv"],
          destination: "localSettings",
        },
      ]),
    ).toBe("Always allow Edit, /srv")
  })

  it("says Accept edits allows all edits for this session", () => {
    expect(
      allowLabel([
        { type: "setMode", mode: "acceptEdits", destination: "session" },
      ]),
    ).toBe("Allow all edits for this session")
  })

  it("names a switch to another mode", () => {
    expect(
      allowLabel([
        rule("Read", undefined, "session"),
        { type: "setMode", mode: "plan", destination: "session" },
      ]),
    ).toBe("Allow Read for this session and switch to Plan")
  })
})

describe("changedLines", () => {
  it("marks only the lines that changed", () => {
    expect(changedLines("a\nb\nc\n", "a\nB\nc\n")).toEqual([
      { text: "a", kind: "default" },
      { text: "b", kind: "removed" },
      { text: "B", kind: "added" },
      { text: "c", kind: "default" },
    ])
  })

  it("marks every line of a new file added", () => {
    expect(changedLines("", "x\ny")).toEqual([
      { text: "x", kind: "added" },
      { text: "y", kind: "added" },
    ])
  })
})
