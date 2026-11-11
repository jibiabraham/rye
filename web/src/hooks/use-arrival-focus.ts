import { type RefObject, useEffect } from "react"

import { FINE_POINTER } from "@/hooks/use-media-query"

/** False until the first page has shown. */
let arrived = false

/**
 * Moves focus when a page mounts. On a device with a mouse, focus goes to the page's message box, marked
 * `data-arrival`. Otherwise it goes to the first h2 inside `ref`, after the first page. Call it from the component
 * that holds each page's heading.
 */
export function useArrivalFocus(ref: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const first = !arrived
    arrived = true
    const box = window.matchMedia(FINE_POINTER).matches
      ? document.querySelector<HTMLElement>("[data-arrival]")
      : null
    if (box) {
      box.focus({ preventScroll: true })
      return
    }
    if (first) return
    ref.current
      ?.querySelector<HTMLElement>("h2")
      ?.focus({ preventScroll: true })
  }, [ref])
}
