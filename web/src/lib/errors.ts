/** The text of a thrown value, for an ErrorNote. */
export function errorMessage(e: unknown) {
  return e instanceof Error ? e.message : String(e)
}
