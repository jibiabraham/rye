import { render, screen } from "@testing-library/react"
import { expect, test } from "vitest"

import { ActivityLine } from "@/session/activity"
import { initial } from "@/session/state"

test("a running tool reads Running, with no repeat of its command", () => {
  render(
    <ActivityLine
      state={{
        ...initial,
        running: true,
        since: Date.now(),
        entries: [
          {
            uuid: "a",
            role: "assistant",
            blocks: [
              {
                type: "tool_use",
                id: "t1",
                name: "Bash",
                input: { command: "make gate" },
              },
            ],
          },
        ],
      }}
    />,
  )
  expect(screen.getByRole("status")).toHaveTextContent(/^Running\s*· 0s$/)
})

test("background tasks keep the line after the turn ends", () => {
  render(<ActivityLine state={{ ...initial, background: 2 }} />)
  expect(screen.getByRole("status")).toHaveTextContent(
    "2 background tasks running",
  )
})

test("a lost stream says it's reconnecting", () => {
  render(<ActivityLine state={{ ...initial, running: true }} stale />)
  expect(screen.getByRole("status")).toHaveTextContent(
    "Reconnecting to agent-manager…",
  )
})
