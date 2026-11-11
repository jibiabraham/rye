import { CopyButton } from "@/components/copy-button"

/** A command for the person to run, in code type, with a button that copies it. Use it inside a sentence. */
export function Command({ children }: { children: string }) {
  return (
    <span className="inline-flex items-center gap-0.5 align-middle">
      <code className="rounded bg-muted px-1 py-0.5 font-mono text-[0.85em]">
        {children}
      </code>
      <CopyButton label="Copy command" text={() => children} />
    </span>
  )
}
