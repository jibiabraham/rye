import { CircleAlert } from "lucide-react"
import type { ReactNode } from "react"

import { Alert, AlertAction, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"

/**
 * The app's one style for errors. A failed read, such as a list or the session stream, gets Retry and a polite
 * announcement. A failed action gets neither.
 * The control that failed is the retry, and its error interrupts.
 */
export function ErrorNote(props: {
  children: ReactNode
  onRetry?: () => void
}) {
  return (
    <Alert
      variant="destructive"
      role={props.onRetry ? "status" : "alert"}
      className="border-destructive/30 bg-destructive/5"
    >
      <CircleAlert />
      <AlertDescription className="break-words">
        {props.children}
      </AlertDescription>
      {props.onRetry && (
        <AlertAction>
          <Button variant="outline" size="xs" onClick={props.onRetry}>
            Retry
          </Button>
        </AlertAction>
      )}
    </Alert>
  )
}
