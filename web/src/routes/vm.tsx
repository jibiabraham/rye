import { useQueryClient } from "@tanstack/react-query"
import { getRouteApi, useNavigate } from "@tanstack/react-router"
import { useEffect, useId, useState } from "react"

import { ErrorNote } from "@/components/error-note"
import { VMNotice } from "@/components/vm-notice"
import { EffortSelect, ModelSelect, ModeSelect } from "@/components/selects"
import { NotFound } from "@/components/not-found"
import { VMsPending } from "@/components/vms-pending"
import { PageHeader } from "@/components/page-header"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useFrom, useUp } from "@/hooks/navigation"
import {
  addCreated,
  refreshLists,
  retryAll,
  useAllListings,
  useModels,
  useVM,
} from "@/hooks/queries"
import { useTitle } from "@/hooks/use-title"
import { api, type Effort, type Mode } from "@/lib/api"
import { errorMessage } from "@/lib/errors"
import { findModel } from "@/lib/models"
import { draftKeyOf } from "@/lib/sessions"
import { load, save } from "@/lib/storage"
import { Composer, composerSelect } from "@/session/composer"
import { holdFirst } from "@/session/first-message"

const vmRoute = getRouteApi("/vms/$vm")
/** A VM's page, which starts a new session. */
export function VMPage() {
  const { vm } = vmRoute.useParams()
  return <NewSession key={vm} vm={vm} />
}

function NewSession({ vm }: { vm: string }) {
  const client = useQueryClient()
  const navigate = useNavigate()
  const up = useUp()
  const from = useFrom()
  const { vm: vmInfo, missing, stopped, noAgentManager, ready } = useVM(vm)
  const { errors } = useAllListings()
  const { data: models, isLoading: modelsLoading } = useModels(
    ready ? vm : undefined,
  )
  const draftKey = draftKeyOf(vm, "new")
  const [text, setText] = useState(() => load(draftKey, "", "session"))
  useEffect(
    () => save(draftKey, text || undefined, "session"),
    [draftKey, text],
  )
  const [cwd, setCwd] = useState("")
  const [model, setModel] = useState("default")
  const [effort, setEffort] = useState<Effort>()
  const [mode, setMode] = useState<Mode>("default")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()
  const cwdInput = useId()
  useTitle(undefined, missing ? "Not found" : vm)

  if (missing) {
    return (
      <NotFound back="All VMs" onBack={() => up({ to: "/" })}>
        There is no VM named {vm}.
      </NotFound>
    )
  }
  if (!vmInfo) return <VMsPending title={vm} />

  const chosenModel = models?.some((m) => m.value === model) ? model : "default"
  const listError = errors[vm]
  /** Creates the session and opens it with the first message, which its page sends. */
  const start = async () => {
    const content = text.trim()
    if (!content || busy) return
    setBusy(true)
    const session = await api
      .create(vm, {
        cwd: cwd.trim() || undefined,
        model: chosenModel,
        effort,
        mode,
      })
      .then(
        (r) => r.session,
        (e: unknown) => {
          setError(errorMessage(e))
          setBusy(false)
        },
      )
    if (!session) return
    addCreated(client, vm, {
      sessionId: session,
      summary: "New session",
      cwd: cwd.trim() || undefined,
      lastModified: Date.now(),
    })
    // The session page sends the message once it has subscribed. The message waits in its box until then.
    holdFirst(vm, session, content)
    save(draftKeyOf(vm, session), content, "session")
    // The page goes before an effect could save the empty box.
    save(draftKey, undefined, "session")
    void navigate({
      to: "/vms/$vm/sessions/$session",
      params: { vm, session },
      state: { ...from, startModel: chosenModel },
    })
    refreshLists(client)
  }

  return (
    <section className="flex h-full min-h-0 flex-col">
      <PageHeader>
        <h2 tabIndex={-1} className="truncate px-1 font-medium outline-none">
          New session on {vm}
        </h2>
      </PageHeader>
      <div className="flex flex-1 items-start justify-center overflow-y-auto p-4 md:items-center">
        <div className="flex w-full max-w-3xl flex-col gap-3">
          <Composer
            text={text}
            onText={setText}
            onSend={() => void start()}
            blocked={busy || stopped || noAgentManager}
          >
            <ModelSelect
              size="sm"
              className={composerSelect}
              models={models}
              loading={modelsLoading}
              value={chosenModel}
              onChange={(value) => {
                setModel(value)
                setEffort(undefined)
              }}
            />
            <EffortSelect
              size="sm"
              className={composerSelect}
              model={findModel(models ?? [], chosenModel)}
              value={effort}
              onChange={setEffort}
            />
            <ModeSelect
              size="sm"
              className={composerSelect}
              value={mode}
              onChange={setMode}
            />
          </Composer>
          <Field orientation="horizontal" className="px-1">
            <FieldLabel
              htmlFor={cwdInput}
              className="shrink-0 whitespace-nowrap text-muted-foreground"
            >
              Working directory
            </FieldLabel>
            <Input
              id={cwdInput}
              value={cwd}
              placeholder="Home directory"
              onChange={(e) => setCwd(e.target.value)}
              className="h-8 font-mono text-sm"
            />
          </Field>
          <VMNotice vm={vm} stopped={stopped} noAgentManager={noAgentManager} />
          {listError && (
            // The sidebar shows the error beside the list. A phone hides the sidebar.
            <div className="md:hidden">
              <ErrorNote onRetry={() => retryAll(client)}>
                {errorMessage(listError)}
              </ErrorNote>
            </div>
          )}
          {error && <ErrorNote>{error}</ErrorNote>}
        </div>
      </div>
    </section>
  )
}
