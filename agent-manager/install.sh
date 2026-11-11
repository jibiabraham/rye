#!/bin/sh
# Installs agent-manager from ~/.local/share/agent-manager.new, where rye agent-manager install unpacks it. It installs
# the dependencies from the lockfile there, then moves the directory to ~/.local/share/agent-manager in place of the
# installed agent-manager, installs rye's instructions for Claude, restarts the user unit and waits until agent-manager
# listens. It stops the installed agent-manager only after the dependencies install. When the new one doesn't listen,
# it puts the old one back.
set -eu
A=~/.local/share/agent-manager
cd "$A.new"
# npx, which ships with Node, runs the pnpm that package.json names. The lockfile and pnpm-workspace.yaml decide what
# it installs.
npx --yes "$(node -p 'require("./package.json").packageManager')" install --frozen-lockfile --prod --loglevel=error
# The claude CLI that the Agent SDK bundles. rye agent-manager install links it into /usr/local/bin for every shell. The
# link is relative, because the directory moves.
ln -sfn "$(node --input-type=module -e '
import { createRequire } from "node:module"
import { relative } from "node:path"
const sdk = import.meta.resolve("@anthropic-ai/claude-agent-sdk")
console.log(relative(process.cwd(), createRequire(sdk).resolve("@anthropic-ai/claude-agent-sdk-linux-" + process.arch + "/claude")))
')" claude
systemctl --user stop agent-manager 2>/dev/null || true
rm -rf "$A.old"
if [ -e "$A" ]; then mv "$A" "$A.old"; fi
mv "$A.new" "$A"
cd "$A"
# Claude Code loads every file in ~/.claude/rules/ in each session on the VM.
# The package's files are read-only, and install gives each copy a writable mode for the next install.
install -D -m 644 AGENTS.md ~/.claude/rules/rye.md
install -D -m 644 agent-manager.service ~/.config/systemd/user/agent-manager.service
systemctl --user daemon-reload
systemctl --user enable --quiet agent-manager
systemctl --user restart agent-manager
for _ in $(seq 30); do
  if ss -Hltn 'sport = :7420' | grep -q 127.0.0.1; then
    rm -rf "$A.old"
    echo "agent-manager is listening on 127.0.0.1:7420"
    exit 0
  fi
  sleep 1
done
systemctl --user status --no-pager agent-manager >&2 || true
if [ -e "$A.old" ]; then
  systemctl --user stop agent-manager || true
  cd
  rm -rf "$A"
  mv "$A.old" "$A"
  install -D -m 644 "$A/AGENTS.md" ~/.claude/rules/rye.md
  install -D -m 644 "$A/agent-manager.service" ~/.config/systemd/user/agent-manager.service
  systemctl --user daemon-reload
  systemctl --user restart agent-manager
  echo "agent-manager did not start listening on 127.0.0.1:7420 within 30 s; the previous agent-manager runs again" >&2
  exit 1
fi
echo "agent-manager did not start listening on 127.0.0.1:7420 within 30 s" >&2
exit 1
