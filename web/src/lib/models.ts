import { efforts, type Model } from "@/lib/api"

/** The alias row for a model ID from a transcript, such as "Haiku 5.5" for claude-haiku-5-5. */
export function findModel(models: Model[], id?: string) {
  if (!id) return undefined
  return (
    models.find((m) => m.value === id) ??
    models.find((m) => m.value !== "default" && m.resolvedModel === id)
  )
}

export const effortsOf = (model?: Model) =>
  model ? (model.supportedEffortLevels ?? []) : efforts
