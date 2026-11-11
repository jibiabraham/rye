import { useQueryClient } from "@tanstack/react-query"
import { getRouteApi, useLocation } from "@tanstack/react-router"
import { useCallback, useState } from "react"

import { NotFound } from "@/components/not-found"
import { VMsPending } from "@/components/vms-pending"
import { PageHeader } from "@/components/page-header"
import { VMNotice } from "@/components/vm-notice"
import { useUp } from "@/hooks/navigation"
import { refreshLists, useListings, useModels, useVM } from "@/hooks/queries"
import { useTitle } from "@/hooks/use-title"
import { SessionView } from "@/session/session-view"

const route = getRouteApi("/vms/$vm/sessions/$session")

/** One session. The URL's hash names a message to land on. */
export function SessionPage() {
  const { vm, session } = route.useParams()
  const hash = useLocation({ select: (l) => l.hash })
  const startModel = useLocation({ select: (l) => l.state.startModel })
  const client = useQueryClient()
  const up = useUp()
  const {
    vm: vmInfo,
    missing: vmMissing,
    stopped,
    noAgentManager,
    ready,
  } = useVM(vm)
  const { lists } = useListings(ready ? [vm] : [])
  const { data: models } = useModels(ready ? vm : undefined)
  /** Set when agent-manager reports no session with this ID. */
  const [missing, setMissing] = useState(false)
  const listing = lists[vm]
  const current = listing?.sessions.find((s) => s.sessionId === session)
  const title = current?.customTitle ?? current?.summary
  const onChange = useCallback(() => refreshLists(client), [client])
  const onMissing = useCallback(() => setMissing(true), [])
  useTitle(missing ? "Not found" : title, vmMissing ? "Not found" : vm)

  if (vmMissing) {
    return (
      <NotFound back="All VMs" onBack={() => up({ to: "/" })}>
        There is no VM named {vm}.
      </NotFound>
    )
  }
  if (missing) {
    return (
      <NotFound
        back={`Sessions on ${vm}`}
        onBack={() => up({ to: "/vms/$vm", params: { vm } })}
      >
        This session isn't on {vm}. It was deleted, or the VM was made again
        under the same name.
      </NotFound>
    )
  }
  if (!vmInfo) return <VMsPending title={vm} />
  if (stopped || noAgentManager) {
    return (
      <section className="flex h-full min-h-0 flex-col">
        <PageHeader>
          <h2 tabIndex={-1} className="truncate px-1 font-medium outline-none">
            Session on {vm}
          </h2>
        </PageHeader>
        <VMNotice
          vm={vm}
          stopped={stopped}
          noAgentManager={noAgentManager}
          className="mx-auto w-full max-w-3xl p-4"
        />
      </section>
    )
  }
  return (
    <SessionView
      key={`${vm}/${session}`}
      vm={vm}
      session={session}
      message={hash || undefined}
      title={title ?? (listing ? "New session" : "")}
      cwd={current?.cwd}
      onChange={onChange}
      onMissing={onMissing}
      models={models}
      startModel={startModel}
    />
  )
}
