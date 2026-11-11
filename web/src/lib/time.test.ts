import { describe, expect, it } from "vitest"

import { duration, shortTime } from "@/lib/time"

describe("shortTime", () => {
  const now = new Date(2026, 10, 7, 15, 0)

  it("shows only the time for today", () => {
    expect(shortTime(new Date(2026, 10, 7, 9, 5), now)).not.toMatch(/Nov/)
  })

  it("adds the date for another day", () => {
    expect(shortTime(new Date(2026, 10, 6, 9, 5), now)).toMatch(/Nov/)
  })

  it("adds the year for another year", () => {
    expect(shortTime(new Date(2025, 10, 6, 9, 5), now)).toMatch(/2025/)
  })
})

describe("duration", () => {
  it.each([
    [0, "0s"],
    [45.9, "45s"],
    [60, "1m 0s"],
    [200, "3m 20s"],
    [-3, "0s"],
  ])("formats %d seconds as %s", (seconds, want) => {
    expect(duration(seconds)).toBe(want)
  })
})
