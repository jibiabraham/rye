import { type Effort, type Mode, type Model, modeLabel, modes } from "@/lib/api"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { effortsOf } from "@/lib/models"

const effortLabel = (level: Effort) =>
  level === "xhigh"
    ? "Extra high effort"
    : `${level[0].toUpperCase()}${level.slice(1)} effort`

/** The select's value for the model's own default effort. */
const DEFAULT = "default" as const

/**
 * The app's one dropdown. Every select uses it. `label` names the control for screen readers. An item's `short`
 * label shows in the closed select on a narrow screen.
 */
export function OptionSelect<T extends string>(props: {
  label: string
  items: { value: T; label: string; short?: string }[]
  value: T
  onChange: (value: T) => void
  disabled?: boolean
  size?: "sm" | "default"
  className?: string
  id?: string
}) {
  return (
    <Select
      items={props.items}
      value={props.value}
      disabled={props.disabled}
      onValueChange={(value) => {
        if (value !== null) props.onChange(value)
      }}
    >
      <SelectTrigger
        id={props.id}
        aria-label={props.label}
        size={props.size}
        className={props.className}
      >
        <SelectValue>
          {(value: T) => {
            const item = props.items.find((i) => i.value === value)
            if (!item?.short) return item?.label
            return (
              <>
                <span className="sm:hidden">{item.short}</span>
                <span className="max-sm:hidden">{item.label}</span>
              </>
            )
          }}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        {props.items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

/** The model. Without a list, it shows "Loading models…" while one loads, and "Default" otherwise. */
export function ModelSelect(props: {
  models?: Model[]
  loading: boolean
  value: string
  onChange: (value: string) => void
  size?: "sm" | "default"
  className?: string
}) {
  return (
    <OptionSelect
      label="Model"
      items={
        props.models?.map((m) => ({
          value: m.value,
          label: m.displayName,
          // "Default (recommended)" shows as "Default".
          short: m.displayName.replace(/\s*\(.*\)$/, ""),
        })) ?? [
          {
            value: props.value,
            label: props.loading ? "Loading models…" : "Default",
          },
        ]
      }
      value={props.value}
      disabled={!props.models}
      onChange={props.onChange}
      size={props.size}
      className={props.className}
    />
  )
}

/** The permission mode. A session with no stored mode runs in "default", which is Manual. */
export function ModeSelect(props: {
  value?: Mode
  onChange: (value: Mode) => void
  size?: "sm" | "default"
  className?: string
}) {
  return (
    <OptionSelect
      label="Permission mode"
      items={modes.map((mode) => ({ value: mode, label: modeLabel(mode) }))}
      value={props.value ?? "default"}
      onChange={props.onChange}
      size={props.size}
      className={props.className}
    />
  )
}

/** Effort levels the model supports, with Default for the model's own default. */
export function EffortSelect(props: {
  model?: Model
  value?: Effort
  onChange: (value?: Effort) => void
  size?: "sm" | "default"
  className?: string
}) {
  const levels = effortsOf(props.model)
  if (levels.length === 0) return null
  const items: {
    value: Effort | typeof DEFAULT
    label: string
    short: string
  }[] = [
    { value: DEFAULT, label: "Default effort", short: "Effort" },
    ...levels.map((level) => ({
      value: level,
      label: effortLabel(level),
      short: effortLabel(level).replace(/ effort$/, ""),
    })),
  ]
  return (
    <OptionSelect
      label="Effort"
      items={items}
      value={props.value ?? DEFAULT}
      onChange={(value) =>
        props.onChange(value === DEFAULT ? undefined : value)
      }
      size={props.size}
      className={props.className}
    />
  )
}
