import { type ComponentProps, useRef } from "react"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"

import { CopyButton } from "@/components/copy-button"

/** Claude's reply as Markdown. Raw HTML in the reply shows as text. */
export function Markdown({ text }: { text: string }) {
  return (
    <div className="markdown">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{ pre: CodeBlock, a: Link, table: Table }}
      >
        {text}
      </ReactMarkdown>
    </div>
  )
}

function Link({ children, ...props }: ComponentProps<"a">) {
  return (
    <a {...props} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  )
}

/** Wide tables scroll inside a box, because `display: block` on a table drops its table role. */
function Table(props: ComponentProps<"table">) {
  return (
    <div className="max-w-full overflow-x-auto">
      <table {...props} />
    </div>
  )
}

function CodeBlock(props: ComponentProps<"pre">) {
  const pre = useRef<HTMLPreElement>(null)
  return (
    <div className="group/code relative">
      <pre ref={pre} {...props} />
      <CopyButton
        label="Copy code"
        className="absolute top-1.5 right-1.5 bg-muted [@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover/code:opacity-100 [@media(hover:hover)]:focus-visible:opacity-100"
        text={() => pre.current?.innerText ?? ""}
      />
    </div>
  )
}
