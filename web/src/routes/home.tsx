import { useNavigate } from "@tanstack/react-router"
import { useEffect } from "react"

import { Command } from "@/components/command"
import { PageHeader } from "@/components/page-header"
import { VMsPending } from "@/components/vms-pending"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { useVMs } from "@/hooks/queries"
import { useTitle } from "@/hooks/use-title"
import { homeVM, LAST_VM } from "@/lib/sessions"
import { load } from "@/lib/storage"

/** Opens the VM used last, else the first running one, else the first. */
export function HomePage() {
  const navigate = useNavigate()
  const { data: vms } = useVMs()
  useTitle()
  useEffect(() => {
    if (!vms) return
    const home = homeVM(vms, load<string | undefined>(LAST_VM, undefined))
    if (home) {
      void navigate({ to: "/vms/$vm", params: { vm: home }, replace: true })
    }
  }, [vms, navigate])
  if (!vms) return <VMsPending title="VMs" />
  return (
    <section className="flex h-full min-h-0 flex-col">
      <PageHeader>
        <h2 tabIndex={-1} className="truncate px-1 font-medium outline-none">
          VMs
        </h2>
      </PageHeader>
      {vms.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>No VMs yet</EmptyTitle>
            <EmptyDescription>
              Create one with <Command>ssh rye new &lt;vm&gt;</Command>.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : null}
    </section>
  )
}
