import { fireEvent, render, screen } from "@testing-library/react"
import { expect, test } from "vitest"

import { ToolRow } from "@/session/tool-row"

const bash = {
  type: "tool_use" as const,
  id: "t1",
  name: "Bash",
  input: {
    command:
      "ssh rye new forge --cloud-init < deployments/forge-vm/cloud-init.yaml",
    description: "Create the forge VM",
  },
}

test("a tool row shows Claude's description, and the command when opened", () => {
  render(<ToolRow block={bash} running={false} />)
  expect(screen.getByText("Create the forge VM")).toBeInTheDocument()
  expect(screen.queryByText(/ssh rye new forge/)).toBeNull()
  fireEvent.click(screen.getByRole("button", { name: /Create the forge VM/ }))
  expect(screen.getByText(/ssh rye new forge/)).toBeVisible()
})

test("a finished tool row shows its duration and a done mark, a failed one a failed mark", () => {
  const { rerender } = render(
    <ToolRow
      block={bash}
      running={false}
      result={{ type: "tool_result", tool_use_id: "t1", content: "Ready" }}
      started="2026-11-08T10:00:00Z"
      ended="2026-11-08T10:01:05Z"
    />,
  )
  expect(screen.getByText("1m 5s")).toBeInTheDocument()
  expect(screen.getByText(", Done")).toBeInTheDocument()
  rerender(
    <ToolRow
      block={bash}
      running={false}
      result={{
        type: "tool_result",
        tool_use_id: "t1",
        content: "boom",
        is_error: true,
      }}
    />,
  )
  expect(screen.getByText(", Failed")).toBeInTheDocument()
})

const ask = {
  type: "tool_use" as const,
  id: "q1",
  name: "AskUserQuestion",
  input: {
    questions: [
      { question: "Which check?", options: [] },
      { question: "Keep the file?", options: [] },
    ],
  },
}

test("an answered question row shows each question and its answer", () => {
  render(
    <ToolRow
      block={ask}
      running={false}
      result={{
        type: "tool_result",
        tool_use_id: "q1",
        content:
          'Your questions have been answered: "Which check?"="Use forge, then dev2", "Keep the file?"="Yes". You can now continue with these answers in mind.',
      }}
    />,
  )
  expect(screen.getByText("Use forge, then dev2 · Yes")).toBeInTheDocument()
  expect(screen.getByText(", Done")).toBeInTheDocument()
  fireEvent.click(screen.getByRole("button", { name: /Which check\?/ }))
  expect(screen.getByText("Keep the file?")).toBeVisible()
})

test("a skipped question row says skipped, without a failed mark", () => {
  render(
    <ToolRow
      block={ask}
      running={false}
      result={{
        type: "tool_result",
        tool_use_id: "q1",
        content: "The user denied this.",
        is_error: true,
      }}
    />,
  )
  expect(screen.getByText("skipped")).toBeInTheDocument()
  expect(screen.queryByText(", Failed")).toBeNull()
})
