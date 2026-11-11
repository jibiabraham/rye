import { useEffect, useState } from "react"

/** The time in milliseconds, updated once a second while `ticking` is true. */
export function useNow(ticking = true) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!ticking) return
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [ticking])
  return now
}
