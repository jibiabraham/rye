// Per-browser conveniences. Without storage, a read returns the fallback and a write does nothing. "local" keeps a
// value across tabs and restarts, and "session" keeps it for one tab.

type Area = "local" | "session"

// Reading window.localStorage throws where the browser blocks site storage.
const storeOf = (area: Area) =>
  area === "local" ? localStorage : sessionStorage

export function load<T>(key: string, fallback: T, area: Area = "local"): T {
  try {
    const raw = storeOf(area).getItem(key)
    return raw === null ? fallback : (JSON.parse(raw) as T)
  } catch {
    return fallback
  }
}

/** Saves a value. Undefined removes it. */
export function save(key: string, value: unknown, area: Area = "local") {
  try {
    if (value === undefined) storeOf(area).removeItem(key)
    else storeOf(area).setItem(key, JSON.stringify(value))
  } catch {
    // Storage is off or full, and the value isn't kept.
  }
}
