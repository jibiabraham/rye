import { describe, expect, it } from "vitest"

import type { SessionState } from "@/lib/api"
import { homeVM, ordered, unseen } from "@/lib/sessions"

const at = (id: string, lastModified: number) => ({
  sessionId: id,
  summary: id,
  lastModified,
})
const ids = (rows: { sessionId: string }[]) => rows.map((s) => s.sessionId)

describe("ordered", () => {
  const list = [at("a", 1), at("b", 3), at("c", 2)]

  it("puts the most recently changed session first", () => {
    expect(ids(ordered(list, null))).toEqual(["b", "c", "a"])
  })

  it("keeps a frozen order and adds new sessions last", () => {
    expect(ids(ordered([...list, at("d", 9)], ["a", "b", "c"]))).toEqual([
      "a",
      "b",
      "c",
      "d",
    ])
  })
})

describe("homeVM", () => {
  const vms = [
    { name: "a", status: "stopped" },
    { name: "b", status: "running" },
  ]

  it("returns the last VM opened", () => {
    expect(homeVM(vms, "a")).toBe("a")
  })

  it("falls back to the first running VM when the last one is gone", () => {
    expect(homeVM(vms, "gone")).toBe("b")
  })

  it("returns the first running VM when none was opened", () => {
    expect(homeVM(vms, undefined)).toBe("b")
  })

  it("returns nothing when there are no VMs", () => {
    expect(homeVM([], undefined)).toBeUndefined()
  })
})

describe("unseen", () => {
  const session = { sessionId: "s", summary: "", cwd: "/", lastModified: 20 }
  const running: SessionState = { running: true, pending: 0 }

  it("is false for a session never opened here", () => {
    expect(unseen(session, undefined)).toBe(false)
  })

  it("is false for a session unchanged since it was opened", () => {
    expect(unseen(session, 20)).toBe(false)
  })

  it("is true for a session changed since it was opened", () => {
    expect(unseen(session, 10)).toBe(true)
  })

  it("is false while the session runs a turn", () => {
    expect(unseen(session, 10, running)).toBe(false)
  })
})
