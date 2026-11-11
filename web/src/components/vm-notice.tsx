import { Command } from "@/components/command"
import { Status } from "@/components/status"

/** Why a VM can't run sessions, with the command that fixes it. It renders nothing for a VM that can. */
export function VMNotice(props: {
  vm: string
  stopped: boolean
  noAgentManager: boolean
  className?: string
}) {
  const { vm } = props
  if (props.stopped) {
    return (
      <Status className={props.className}>
        {vm} is stopped. Start it with{" "}
        <Command>{`ssh rye start ${vm}`}</Command>.
      </Status>
    )
  }
  if (props.noAgentManager) {
    return (
      <Status className={props.className}>
        {vm} has no agent-manager. Install it with{" "}
        <Command>{`ssh rye agent-manager install ${vm}`}</Command>.
      </Status>
    )
  }
  return null
}
