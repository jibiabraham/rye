import {
  type ErrorComponentProps,
  Navigate,
  Outlet,
  useNavigate,
  useParams,
} from "@tanstack/react-router"
import { useEffect } from "react"

import { AppSidebar } from "@/components/app-sidebar"
import { useFrom } from "@/hooks/navigation"
import { useVM } from "@/hooks/queries"
import { ErrorNote } from "@/components/error-note"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import { TooltipProvider } from "@/components/ui/tooltip"
import { errorMessage } from "@/lib/errors"
import { LAST_VM } from "@/lib/sessions"
import { save } from "@/lib/storage"

/** The VM and session list beside the open page, and a link that skips to the page. */
export function RootLayout() {
  useNewSessionKey()
  useLastVM()
  return (
    <TooltipProvider>
      <a
        href="#main"
        className="sr-only z-50 rounded-md bg-background px-3 py-2 focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:ring-3 focus:ring-ring/50"
      >
        Skip to page
      </a>
      <SidebarProvider className="h-dvh min-h-0">
        <AppSidebar />
        <SidebarInset id="main" tabIndex={-1} className="min-h-0 outline-none">
          <Outlet />
        </SidebarInset>
      </SidebarProvider>
    </TooltipProvider>
  )
}

/** Any page that throws while rendering shows its error here, with Retry. */
export function RootError({ error, reset }: ErrorComponentProps) {
  return (
    <div className="m-6 max-w-md">
      <ErrorNote onRetry={reset}>{errorMessage(error)}</ErrorNote>
    </div>
  )
}

/** A path the app has no page for opens the home page. */
export function RootNotFound() {
  return <Navigate to="/" replace />
}

/** Ctrl+Shift+O, or Cmd+Shift+O on a Mac, opens a new session on the open VM. */
function useNewSessionKey() {
  const { vm } = useParams({ strict: false })
  const navigate = useNavigate()
  const { from } = useFrom()
  useEffect(() => {
    if (!vm) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key.toLowerCase() !== "o" || !e.shiftKey) return
      if (!(e.metaKey || e.ctrlKey) || e.altKey) return
      e.preventDefault()
      void navigate({ to: "/vms/$vm", params: { vm }, state: { from } })
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [vm, navigate, from])
}

/** Saves the open VM as the one the home page opens. */
function useLastVM() {
  const { vm } = useParams({ strict: false })
  const { vm: info } = useVM(vm ?? "")
  useEffect(() => {
    if (info) save(LAST_VM, info.name)
  }, [info])
}
