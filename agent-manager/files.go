// Package agentfiles holds agent-manager's source and install files. rye agent-manager install unpacks them in a VM,
// where install.sh installs the dependencies from the lockfile.
package agentfiles

import "embed"

// FS holds the files that rye agent-manager install unpacks into ~/.local/share/agent-manager.
//
//go:embed AGENTS.md agent-manager.ts main.ts install.sh package.json
//go:embed agent-manager.service pnpm-lock.yaml pnpm-workspace.yaml
var FS embed.FS
