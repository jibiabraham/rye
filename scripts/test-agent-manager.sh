#!/bin/bash
# Tests agent-manager install against a deployed service VM. Creates and removes a real VM. Run make up first.
set -euo pipefail
# shellcheck source=scripts/lib.sh
. "$(dirname "$0")/lib.sh"
VM=rye-test-agent-manager
PORT=7420

trap 'rye rm "$VM" --yes </dev/null || true' EXIT
vm() { ssh_vm "$VM.rye" "$@" </dev/null; }
# Sends one request through ssh -W, the path ryed uses, and prints the reply line as soon as it arrives.
ask() {
  local reply=""
  coproc AGENT_MANAGER { ssh_vm -W "127.0.0.1:$PORT" "$VM.rye"; }
  echo "$1" >&"${AGENT_MANAGER[1]}"
  IFS= read -r -t 30 reply <&"${AGENT_MANAGER[0]}" || true
  kill "$AGENT_MANAGER_PID" 2>/dev/null || true
  wait "$AGENT_MANAGER_PID" 2>/dev/null || true
  echo "$reply"
}
wait_for_agent_manager() {
  for _ in $(seq 1 30); do
    [ "$(ask '{"id":1,"type":"list"}' | jq -r .id)" = 1 ] && return
    sleep 2
  done
  echo "agent-manager did not answer" >&2
  return 1
}

step "new and agent-manager install"
rye new "$VM"
wait_for_vm "$VM"
rye agent-manager install "$VM"
wait_for_agent_manager
vm 'node --version' | grep -q "^v26\."
vm 'claude --version' >/dev/null
vm 'test -s ~/.claude/rules/rye.md'
ask '{"id":2,"type":"create"}' | jq -e '.id == 2 and (.session | length) == 36' >/dev/null
ask '{"id":3,"type":"nope"}' | jq -e '.error | contains("unknown request")' >/dev/null

step "a second install"
rye agent-manager install "$VM"
wait_for_agent_manager
vm 'claude --version' >/dev/null
vm '[ ! -e ~/.local/share/agent-manager.new ] && [ ! -e ~/.local/share/agent-manager.old ]'

step "stop and start"
rye stop "$VM"
# start installs agent-manager in a VM that has it.
rye start "$VM" | grep -q "agent-manager is listening"
wait_for_agent_manager
vm 'claude --version' >/dev/null

step "a reused name"
# A reused name has a new host key.
rye rm "$VM" --yes
rye new "$VM"
wait_for_vm "$VM"
rye agent-manager install "$VM"
wait_for_agent_manager
echo "test-agent-manager passed in ${SECONDS}s"
