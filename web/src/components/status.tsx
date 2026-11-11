import type { ReactNode } from "react"

import { cn } from "cn"

/**
 * A line of muted text that says what the page is doing or why something is missing, such as "Loading sessions…".
 * Screen readers announce it when it changes.
 */
export function Status(props: { children: ReactNode; className?: string }) {
  return (
    <p
      role="status"
      className={cn("text-sm text-muted-foreground", props.className)}
    >
      {props.children}
    </p>
  )
}
