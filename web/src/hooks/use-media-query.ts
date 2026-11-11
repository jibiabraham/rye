import { useSyncExternalStore } from "react"

/** True while the media query matches. It follows changes, such as a window resize. */
export function useMediaQuery(query: string) {
  return useSyncExternalStore(
    (onChange) => {
      const list = window.matchMedia(query)
      list.addEventListener("change", onChange)
      return () => list.removeEventListener("change", onChange)
    },
    () => window.matchMedia(query).matches,
  )
}

/** Matches a device with a mouse or trackpad. */
export const FINE_POINTER = "(hover: hover) and (pointer: fine)"

/** True on a device with a mouse or trackpad. Enter sends, and focus goes to the message box. */
export function useFinePointer() {
  return useMediaQuery(FINE_POINTER)
}
