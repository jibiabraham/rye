import { useQueryClient } from "@tanstack/react-query"
import { Link, useParams } from "@tanstack/react-router"
import { CircleAlert, Plus } from "lucide-react"
import { useEffect, useRef, useState } from "react"

import { ErrorNote } from "@/components/error-note"
import { Status } from "@/components/status"
import { buttonVariants } from "@/components/ui/button"
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemTitle,
} from "@/components/ui/item"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar"
import { Spinner } from "@/components/ui/spinner"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { useFrom } from "@/hooks/navigation"
import {
  retryAll,
  useAllListings,
  useModels,
  useVM,
  useVMs,
} from "@/hooks/queries"
import type { VM } from "@/lib/api"
import { APP_NAME } from "@/lib/app"
import { errorMessage } from "@/lib/errors"
import { type Listing, ordered, unseen } from "@/lib/sessions"
import { load, save } from "@/lib/storage"

const SEEN = "rye:seen"

/** Totals of a VM's waiting approvals, sessions running a turn or background tasks, and unseen sessions. */
function attention(
  vm: string,
  listing: Listing | undefined,
  seen: Record<string, number>,
) {
  const states = Object.values(listing?.states ?? {})
  return {
    pending: states.reduce((n, s) => n + s.pending, 0),
    running: states.filter((s) => s.running || (s.background ?? 0) > 0).length,
    unseen: (listing?.sessions ?? []).filter((s) =>
      unseen(s, seen[`${vm}/${s.sessionId}`], listing?.states[s.sessionId]),
    ).length,
  }
}

/** Marks a row as waiting on you, working, or live. */
function RowStatus(props: {
  pending: number
  running: boolean
  live: boolean
}) {
  if (props.pending > 0) {
    return (
      <CircleAlert
        role="img"
        aria-label="Waiting for you"
        className="size-3.5 shrink-0 text-attention"
      />
    )
  }
  if (props.running) {
    return (
      <Spinner
        role="img"
        aria-label="Working"
        className="size-3.5 shrink-0 text-muted-foreground"
      />
    )
  }
  if (props.live) {
    return (
      <span
        role="img"
        aria-label="Live"
        className="mx-1 size-1.5 shrink-0 rounded-full bg-success"
      />
    )
  }
  return null
}

/** The VM list, and the open VM's sessions. On narrow screens it opens as a sheet. */
export function AppSidebar() {
  const client = useQueryClient()
  const { vm, session } = useParams({ strict: false })
  const from = useFrom()
  const { isMobile, setOpenMobile } = useSidebar()
  const vmsQuery = useVMs()
  const { lists, errors } = useAllListings()
  const { vm: vmInfo, stopped, noAgentManager, ready } = useVM(vm ?? "")
  const modelsQuery = useModels(ready ? vmInfo?.name : undefined)
  /** The list's order when the pointer or focus entered it. Rows hold still meanwhile. */
  const [frozen, setFrozen] = useState<string[] | null>(null)
  /** When each session was last open, as its lastModified then, by VM and session. */
  const [seen, setSeen] = useState(() => load<Record<string, number>>(SEEN, {}))
  const selectedRow = useRef<HTMLAnchorElement>(null)

  const listing = vm ? lists[vm] : undefined
  const failure =
    (vm ? errors[vm] : undefined) ?? modelsQuery.error ?? vmsQuery.error
  const sessions = ordered(listing?.sessions ?? [], frozen)
  const current = sessions.find((s) => s.sessionId === session)
  // A link in the sheet closes it.
  const close = () => {
    if (isMobile) setOpenMobile(false)
  }

  // The open session counts as seen up to its latest change.
  const openKey = vm && session ? `${vm}/${session}` : undefined
  const openModified = current?.lastModified
  useEffect(() => {
    if (!openKey || openModified === undefined) return
    const mark = setTimeout(() =>
      setSeen((s) => {
        if (s[openKey] === openModified) return s
        const next = { ...s, [openKey]: openModified }
        save(SEEN, next)
        return next
      }),
    )
    return () => clearTimeout(mark)
  }, [openKey, openModified])

  // The open session's row scrolls into view after a link or a reload.
  const listed = listing !== undefined
  useEffect(() => {
    selectedRow.current?.scrollIntoView({ block: "nearest" })
  }, [session, listed])

  const vmRow = (v: VM) => {
    const at = attention(v.name, lists[v.name], seen)
    const up = v.status === "running"
    return (
      <SidebarMenuItem key={v.name}>
        <SidebarMenuButton
          isActive={v.name === vm}
          className={at.unseen > 0 ? "font-semibold" : ""}
          render={
            <Link
              to="/vms/$vm"
              params={{ vm: v.name }}
              state={from}
              activeOptions={{ exact: true }}
              onClick={close}
            />
          }
        >
          <span className="truncate">{v.name}</span>
          <span className="ml-auto flex items-center gap-1.5 text-xs font-normal">
            <RowStatus
              pending={at.pending}
              running={at.running > 0}
              live={false}
            />
            <span
              aria-hidden
              className={`size-1.5 rounded-full ${up ? "bg-success" : "bg-muted-foreground/40"}`}
            />
            <span className={up ? "text-foreground" : "text-muted-foreground"}>
              {up ? "Running" : "Stopped"}
            </span>
          </span>
        </SidebarMenuButton>
      </SidebarMenuItem>
    )
  }

  const freeze = () => setFrozen(sessions.map((s) => s.sessionId))
  return (
    <Sidebar>
      <nav
        aria-label="VMs and sessions"
        className="flex min-h-0 flex-1 flex-col"
      >
        <SidebarHeader>
          <h1 className="px-2 font-heading font-semibold">{APP_NAME}</h1>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>VMs</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu aria-label="VMs">
                {vmsQuery.data?.map(vmRow)}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
          {vmInfo && (
            <SidebarGroup className="min-h-0 flex-1">
              <SidebarGroupLabel>Sessions on {vmInfo.name}</SidebarGroupLabel>
              <SidebarGroupContent className="min-h-0 flex-1">
                <Link
                  to="/vms/$vm"
                  params={{ vm: vmInfo.name }}
                  state={from}
                  onClick={close}
                  className={buttonVariants({
                    variant: "outline",
                    className: "mb-2 w-full",
                  })}
                >
                  <Plus />
                  New session
                </Link>
                {!listing && !failure && !stopped && !noAgentManager && (
                  <Status className="px-2">Loading sessions…</Status>
                )}
                <ul
                  aria-label="Sessions"
                  className="flex flex-col gap-1"
                  onPointerEnter={freeze}
                  onPointerLeave={() => setFrozen(null)}
                  onFocus={() => frozen || freeze()}
                  onBlur={(e) => {
                    if (!e.currentTarget.contains(e.relatedTarget))
                      setFrozen(null)
                  }}
                >
                  {sessions.map((s) => {
                    const selected = s.sessionId === session
                    const state = listing?.states[s.sessionId]
                    const fresh =
                      !selected &&
                      unseen(s, seen[`${vmInfo.name}/${s.sessionId}`], state)
                    return (
                      <li key={s.sessionId}>
                        {/* The full working directory shows on hover or focus of the row. */}
                        <Tooltip>
                          <TooltipTrigger
                            render={
                              <Item
                                size="xs"
                                variant={selected ? "muted" : "default"}
                                render={
                                  <Link
                                    ref={selected ? selectedRow : undefined}
                                    to="/vms/$vm/sessions/$session"
                                    params={{
                                      vm: vmInfo.name,
                                      session: s.sessionId,
                                    }}
                                    state={from}
                                    onClick={close}
                                  />
                                }
                              />
                            }
                          >
                            <ItemContent className="min-w-0">
                              <ItemTitle
                                className={`line-clamp-2 wrap-anywhere ${fresh ? "font-semibold" : selected ? "" : "font-normal"}`}
                              >
                                {s.customTitle ?? s.summary}
                                {fresh && (
                                  <span className="sr-only"> (new)</span>
                                )}
                              </ItemTitle>
                              <ItemDescription className="truncate text-xs">
                                {s.cwd}
                              </ItemDescription>
                            </ItemContent>
                            <ItemActions className="self-start">
                              <RowStatus
                                pending={state?.pending ?? 0}
                                running={
                                  (state?.running ?? false) ||
                                  (state?.background ?? 0) > 0
                                }
                                live={
                                  listing?.live.includes(s.sessionId) ?? false
                                }
                              />
                            </ItemActions>
                          </TooltipTrigger>
                          <TooltipContent side="right">{s.cwd}</TooltipContent>
                        </Tooltip>
                      </li>
                    )
                  })}
                </ul>
              </SidebarGroupContent>
            </SidebarGroup>
          )}
          {failure && (
            <div className="px-2">
              <ErrorNote onRetry={() => retryAll(client)}>
                {errorMessage(failure)}
              </ErrorNote>
            </div>
          )}
        </SidebarContent>
      </nav>
    </Sidebar>
  )
}
