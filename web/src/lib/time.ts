/** A duration in seconds as "45s" or "3m 20s". */
export function duration(seconds: number) {
  const s = Math.max(0, Math.floor(seconds))
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`
}

/** A message time. Today's messages show the time of day, and other days' show the date before it. */
export function shortTime(at: Date, now = new Date()) {
  const time = at.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
  if (at.toDateString() === now.toDateString()) return time
  const date = at.toLocaleDateString([], {
    month: "short",
    day: "numeric",
    year: at.getFullYear() === now.getFullYear() ? undefined : "numeric",
  })
  return `${date}, ${time}`
}
