/**
 * First messages of new sessions, kept for their session page. The page sends one once its stream has subscribed,
 * because a message sent before that streams past the page before the transcript stores it. They live in memory only. A
 * reload never sends one twice.
 */
const first = new Map<string, string>()

export function holdFirst(vm: string, session: string, content: string) {
  first.set(`${vm}/${session}`, content)
}

/** Returns the session's first message once, and forgets it. */
export function takeFirst(vm: string, session: string) {
  const key = `${vm}/${session}`
  const content = first.get(key)
  first.delete(key)
  return content
}
