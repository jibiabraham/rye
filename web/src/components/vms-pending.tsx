import { useQueryClient } from "@tanstack/react-query"

import { ErrorNote } from "@/components/error-note"
import { PageHeader } from "@/components/page-header"
import { Status } from "@/components/status"
import { retryAll, useVMs } from "@/hooks/queries"
import { errorMessage } from "@/lib/errors"

/**
 * Stands in for a page that needs the VM list before the list has loaded. It shows a header, then "Loading VMs…" or
 * the list's error with Retry.
 */
export function VMsPending({ title }: { title: string }) {
  const client = useQueryClient()
  const { error } = useVMs()
  return (
    <section className="flex h-full min-h-0 flex-col">
      <PageHeader>
        <h2 tabIndex={-1} className="truncate px-1 font-medium outline-none">
          {title}
        </h2>
      </PageHeader>
      {error ? (
        <div className="m-6 max-w-md">
          <ErrorNote onRetry={() => retryAll(client)}>
            {errorMessage(error)}
          </ErrorNote>
        </div>
      ) : (
        <Status className="m-6">Loading VMs…</Status>
      )}
    </section>
  )
}
