import { useEffect } from "react"

import { useAllListings } from "@/hooks/queries"
import { APP_NAME } from "@/lib/app"

/**
 * Names the browser tab and history entry after the page, then the app. A dot leads while any session waits for
 * an approval.
 */
export function useTitle(...parts: (string | undefined)[]) {
  const { lists } = useAllListings()
  const waiting = Object.values(lists).some((l) =>
    Object.values(l.states).some((s) => s.pending > 0),
  )
  const text = [...parts, APP_NAME].filter(Boolean).join(" · ")
  const title = waiting ? `● ${text}` : text
  useEffect(() => {
    document.title = title
  }, [title])
}
