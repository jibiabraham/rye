import { ChevronLeft } from "lucide-react"
import { type ReactNode, useRef } from "react"

import { Button } from "@/components/ui/button"
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { useArrivalFocus } from "@/hooks/use-arrival-focus"

/** A page for a VM or session that isn't there, with a way back up. */
export function NotFound(props: {
  children: ReactNode
  back: string
  onBack: () => void
}) {
  const page = useRef<HTMLDivElement>(null)
  useArrivalFocus(page)
  return (
    <Empty ref={page}>
      <EmptyHeader>
        <EmptyTitle>
          <h2 tabIndex={-1} className="outline-none">
            Not found
          </h2>
        </EmptyTitle>
        <EmptyDescription>{props.children}</EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button variant="outline" size="sm" onClick={props.onBack}>
          <ChevronLeft />
          {props.back}
        </Button>
      </EmptyContent>
    </Empty>
  )
}
