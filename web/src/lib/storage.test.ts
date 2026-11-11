import { afterEach, expect, test, vi } from "vitest"

import { load, save } from "@/lib/storage"

afterEach(() => {
  vi.restoreAllMocks()
  localStorage.clear()
})

test("a saved value loads back, and undefined removes it", () => {
  save("k", { a: 1 })
  expect(load("k", null)).toEqual({ a: 1 })
  save("k", undefined)
  expect(load("k", "none")).toBe("none")
})

test("blocked storage loads the fallback and saves nothing", () => {
  vi.spyOn(window, "localStorage", "get").mockImplementation(() => {
    throw new DOMException("blocked", "SecurityError")
  })
  expect(load("k", "fallback")).toBe("fallback")
  expect(() => save("k", 1)).not.toThrow()
})
