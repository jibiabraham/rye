import { useEffect, useId, useRef, useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Questionnaire,
  QuestionnaireActions,
  QuestionnaireChoice,
  QuestionnaireChoiceDescription,
  QuestionnaireChoices,
  QuestionnaireDescription,
  QuestionnaireInput,
  QuestionnaireItem,
  QuestionnaireNext,
  QuestionnairePrevious,
  QuestionnaireProgress,
  QuestionnaireSubmit,
  QuestionnaireTitle,
} from "@/components/ui/questionnaire"
import { useFinePointer } from "@/hooks/use-media-query"
import { api, type Permission, type PermissionResult } from "@/lib/api"
import { ToolDetail } from "@/session/tool-row"
import { allowLabel, summarized, toolSummary } from "@/session/tools"

type Question = {
  question: string
  header?: string
  multiSelect?: boolean
  options: { label: string; description?: string }[]
}

/** Both cards share this frame, because both mean Claude is waiting for the user. */
const frame = "border-attention/50 bg-attention/5"

/**
 * A tool call waiting for an answer. AskUserQuestion gets its questions one at a time, each with its options and a
 * free-text answer.
 */
export function Approval(props: {
  vm: string
  session: string
  permission: Permission
  /** Runs the answer, and resolves to whether it succeeded. */
  act: (action: Promise<unknown>) => Promise<boolean>
}) {
  const { vm, session, permission, act } = props
  const [busy, setBusy] = useState(false)
  const title = useId()
  const answer = (result: PermissionResult) => {
    setBusy(true)
    void act(api.answer(vm, session, permission.request, result)).then((ok) => {
      if (!ok) setBusy(false)
    })
  }
  const deny = () =>
    answer({ behavior: "deny", message: "The user denied this." })

  if (permission.tool === "AskUserQuestion") {
    return (
      <Card role="group" aria-labelledby={title} className={frame}>
        <CardHeader>
          <CardTitle id={title}>Claude has a question</CardTitle>
        </CardHeader>
        <CardContent>
          <Questions
            permission={permission}
            busy={busy}
            answer={answer}
            deny={deny}
          />
        </CardContent>
      </Card>
    )
  }
  const purpose = permission.input.description
  return (
    <Card
      role="group"
      aria-labelledby={title}
      className={frame}
      onKeyDown={(e) => {
        if (e.key === "Escape" && !busy) {
          e.preventDefault()
          deny()
        }
      }}
    >
      <CardHeader>
        <CardTitle id={title}>
          {permission.title ?? `Claude wants to use ${permission.tool}`}
        </CardTitle>
        {[
          typeof purpose === "string" ? purpose : undefined,
          permission.description,
          permission.decisionReason,
        ]
          .filter(Boolean)
          .map((line) => (
            <CardDescription key={line}>{line}</CardDescription>
          ))}
      </CardHeader>
      <CardContent>
        {summarized.has(permission.tool) ? (
          <pre className="code">
            {toolSummary(permission.tool, permission.input)}
          </pre>
        ) : (
          <ToolDetail
            titled
            block={{
              type: "tool_use",
              id: permission.request,
              name: permission.tool,
              input: permission.input,
            }}
          />
        )}
      </CardContent>
      <CardFooter className="flex-wrap gap-2">
        <Button
          disabled={busy}
          onClick={() =>
            answer({ behavior: "allow", updatedInput: permission.input })
          }
        >
          Allow
        </Button>
        {permission.suggestions && permission.suggestions.length > 0 && (
          <Button
            variant="outline"
            disabled={busy}
            onClick={() =>
              answer({
                behavior: "allow",
                updatedInput: permission.input,
                updatedPermissions: permission.suggestions,
              })
            }
          >
            {allowLabel(permission.suggestions)}
          </Button>
        )}
        <Button
          variant="outline"
          disabled={busy}
          onClick={deny}
          aria-keyshortcuts="Escape"
        >
          Deny
        </Button>
      </CardFooter>
    </Card>
  )
}

/** The field name of question i in the form. */
const field = (i: number) => `q${i}`

function Questions(props: {
  permission: Permission
  busy: boolean
  answer: (r: PermissionResult) => void
  deny: () => void
}) {
  const questions = (props.permission.input.questions ?? []) as Question[]
  const fine = useFinePointer()
  const form = useRef<HTMLFormElement>(null)
  // With a mouse, the first option takes focus as the card arrives, unless the person is typing a message.
  // The options render a frame after the card.
  useEffect(() => {
    if (!fine) return
    const frame = requestAnimationFrame(() => {
      const box = document.querySelector<HTMLTextAreaElement>("[data-arrival]")
      if (box?.value) return
      const option = form.current?.querySelector<HTMLElement>(
        "input[type=radio], input[type=checkbox]",
      )
      option?.focus({ preventScroll: true })
    })
    return () => cancelAnimationFrame(frame)
  }, [fine])
  return (
    <Questionnaire
      ref={form}
      // Number keys pick options where there is a keyboard.
      shortcuts={fine ? "numbers" : undefined}
      onSubmit={(e) => {
        e.preventDefault()
        const data = new FormData(e.currentTarget)
        // Claude reads each answer as text. Several choices, or a choice and typed text, join with commas.
        const answers = Object.fromEntries(
          questions.map((q, i) => [
            q.question,
            data
              .getAll(field(i))
              .map(String)
              .filter((v) => v.trim())
              .join(", "),
          ]),
        )
        props.answer({
          behavior: "allow",
          updatedInput: { ...props.permission.input, answers },
        })
      }}
    >
      {questions.length > 1 && <QuestionnaireProgress />}
      {questions.map((q, i) => (
        <QuestionnaireItem
          key={q.question}
          name={field(i)}
          multiple={q.multiSelect}
          required
        >
          <QuestionnaireTitle>{q.question}</QuestionnaireTitle>
          {q.header && (
            <QuestionnaireDescription>{q.header}</QuestionnaireDescription>
          )}
          <QuestionnaireChoices>
            {q.options.map((o) => (
              <QuestionnaireChoice key={o.label} value={o.label}>
                {o.label}
                {o.description && (
                  <QuestionnaireChoiceDescription>
                    {o.description}
                  </QuestionnaireChoiceDescription>
                )}
              </QuestionnaireChoice>
            ))}
          </QuestionnaireChoices>
          <QuestionnaireInput aria-label="Other answer" placeholder="Other" />
        </QuestionnaireItem>
      ))}
      <QuestionnaireActions>
        <QuestionnairePrevious />
        <QuestionnaireNext />
        <QuestionnaireSubmit disabled={props.busy}>Answer</QuestionnaireSubmit>
        <Button
          type="button"
          variant="outline"
          disabled={props.busy}
          onClick={props.deny}
          // The grid cell where the kit puts its own Skip for one question.
          className="col-start-2 row-start-1 justify-self-end"
        >
          Skip questions
        </Button>
      </QuestionnaireActions>
    </Questionnaire>
  )
}
