import { fireEvent, render, screen } from "@testing-library/react"
import { expect, test, vi } from "vitest"

import { ErrorNote } from "@/components/error-note"

test("a failed read shows politely with Retry", () => {
  const retry = vi.fn()
  render(<ErrorNote onRetry={retry}>dev1 is stopped</ErrorNote>)
  expect(screen.getByRole("status")).toHaveTextContent("dev1 is stopped")
  fireEvent.click(screen.getByRole("button", { name: "Retry" }))
  expect(retry).toHaveBeenCalled()
})

test("a failed action interrupts, with no Retry", () => {
  render(<ErrorNote>rename failed</ErrorNote>)
  expect(screen.getByRole("alert")).toHaveTextContent("rename failed")
  expect(screen.queryByRole("button")).toBeNull()
})
