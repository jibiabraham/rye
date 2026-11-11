import { useEffect, useState } from "react"

/** How long a copy button shows that it copied. */
const SHOWN_MS = 1500

/**
 * Copies text to the clipboard. `copied` is true for a moment after a copy. A refused clipboard
 * leaves it false.
 */
export function useCopied() {
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) return
    const timer = setTimeout(() => setCopied(false), SHOWN_MS)
    return () => clearTimeout(timer)
  }, [copied])
  const copy = (text: string) => {
    navigator.clipboard.writeText(text).then(
      () => setCopied(true),
      () => setCopied(false),
    )
  }
  return { copied, copy }
}
