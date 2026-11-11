import { useNow } from "@/hooks/use-now"
import { duration } from "@/lib/time"

/** Seconds since a time in milliseconds, counting up once a second. */
export function Elapsed({ since }: { since: number }) {
  return <>{duration((useNow() - since) / 1000)}</>
}
