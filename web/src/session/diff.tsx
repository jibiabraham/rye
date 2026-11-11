import {
  CodeBlock,
  CodeBlockContent,
  CodeBlockHeader,
  CodeBlockLine,
  CodeBlockTitle,
} from "@/components/ui/code-block"
import { changedLines } from "@/session/tools"

/**
 * A file change as a line diff. Unchanged lines show plain. `path` names the file above the lines. `full` shows every
 * line with no scroll box, for an approval.
 */
export function Diff(props: {
  before: string
  after: string
  path?: string
  full?: boolean
}) {
  return (
    <CodeBlock size="sm">
      {props.path && (
        <CodeBlockHeader>
          <CodeBlockTitle className="font-mono">{props.path}</CodeBlockTitle>
        </CodeBlockHeader>
      )}
      <CodeBlockContent className={props.full ? undefined : "max-h-80"}>
        {changedLines(props.before, props.after).map((line, i) => (
          <CodeBlockLine
            key={i}
            variant={line.kind}
            className={
              line.kind === "added"
                ? "bg-success/10 after:text-success"
                : undefined
            }
          >
            {line.text}
          </CodeBlockLine>
        ))}
      </CodeBlockContent>
    </CodeBlock>
  )
}
