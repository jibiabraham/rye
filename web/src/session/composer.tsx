import { ArrowUp, Square } from "lucide-react"
import type { ReactNode } from "react"

import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group"
import { useFinePointer } from "@/hooks/use-media-query"

/** The class each select under the message box takes. On a narrow screen the selects show short labels. */
export const composerSelect =
  "w-auto min-w-0 border-none bg-transparent px-1.5 hover:bg-muted dark:bg-transparent dark:hover:bg-muted text-muted-foreground shadow-none *:data-[slot=select-value]:block *:data-[slot=select-value]:truncate"

/**
 * The message box, with its selects and Send below the text. Enter sends where there's a keyboard and a mouse. A
 * touch screen's Return starts a new line, and Send sends.
 */
export function Composer(props: {
  text: string
  onText: (text: string) => void
  onSend: () => void
  /** Stops Claude's turn. Stop shows beside Send while it's set, and Esc in the box calls it. */
  onStop?: () => void
  /** True from a Stop until the turn ends. Stop is disabled meanwhile. */
  stopping?: boolean
  /** Disables Send whatever the text. */
  blocked?: boolean
  /** The selects under the text. */
  children: ReactNode
}) {
  const { onStop } = props
  const enterSends = useFinePointer()
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        props.onSend()
      }}
    >
      <InputGroup
        aria-label="Message Claude"
        // A disabled Send, with an empty box, leaves the box at full strength.
        className="rounded-2xl has-disabled:bg-transparent has-disabled:opacity-100"
      >
        <InputGroupTextarea
          aria-label="Message"
          // The page puts focus here on arrival, on a device with a mouse.
          data-arrival
          placeholder="Message Claude"
          rows={1}
          className="max-h-60 min-h-9 px-3"
          value={props.text}
          onChange={(e) => props.onText(e.target.value)}
          onKeyDown={(e) => {
            if (e.nativeEvent.isComposing) return
            if (e.key === "Enter" && !e.shiftKey && enterSends) {
              e.preventDefault()
              if (!props.blocked) props.onSend()
            }
            if (e.key === "Escape" && onStop) {
              e.preventDefault()
              onStop()
            }
          }}
        />
        <InputGroupAddon align="block-end">
          {props.children}
          <span className="ml-auto flex shrink-0 gap-1.5">
            {onStop && (
              <InputGroupButton
                variant="outline"
                size="icon-sm"
                aria-label="Stop"
                aria-keyshortcuts="Escape"
                className="rounded-full"
                disabled={props.stopping}
                onClick={onStop}
              >
                <Square className="fill-current" />
              </InputGroupButton>
            )}
            <InputGroupButton
              type="submit"
              variant="default"
              size="icon-sm"
              aria-label="Send"
              className="rounded-full"
              disabled={props.blocked || !props.text.trim()}
            >
              <ArrowUp />
            </InputGroupButton>
          </span>
        </InputGroupAddon>
      </InputGroup>
    </form>
  )
}
