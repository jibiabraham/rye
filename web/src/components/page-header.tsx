import { type ReactNode, useRef } from "react"

import { SidebarTrigger } from "@/components/ui/sidebar"
import { useArrivalFocus } from "@/hooks/use-arrival-focus"

/**
 * The bar at the top of a page. It starts with the button that shows or hides the VM and session list. Its h2,
 * with tabIndex -1, takes focus when a page without a message box opens, or on a touch screen.
 */
export function PageHeader({ children }: { children: ReactNode }) {
  const header = useRef<HTMLElement>(null)
  useArrivalFocus(header)
  return (
    <header
      ref={header}
      className="flex h-12 shrink-0 items-center gap-1 border-b px-2 md:px-4"
    >
      <SidebarTrigger aria-label="VMs and sessions" />
      {children}
    </header>
  )
}
