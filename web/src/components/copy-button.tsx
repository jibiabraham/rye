import { Check, Copy, Link } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { useCopied } from "@/hooks/use-copied"

/**
 * The app's one copy button: an icon that turns into a check after a copy. Use it for every copy action. Its
 * `label` names what it copies, such as "Copy link to message".
 */
export function CopyButton(props: {
  label: string
  /** The text to copy, read at the click. */
  text: () => string
  icon?: "copy" | "link"
  className?: string
}) {
  const { copied, copy } = useCopied()
  const Icon = copied ? Check : props.icon === "link" ? Link : Copy
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={copied ? "Copied" : props.label}
            className={props.className}
            onClick={() => copy(props.text())}
          />
        }
      >
        <Icon />
      </TooltipTrigger>
      <TooltipContent>{copied ? "Copied" : props.label}</TooltipContent>
    </Tooltip>
  )
}
