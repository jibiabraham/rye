import net from "node:net"
import os from "node:os"
import path from "node:path"
import * as sdk from "@anthropic-ai/claude-agent-sdk"
import { AgentManager } from "./agent-manager.ts"

const PORT = 7420
const LIVE_SESSIONS = 4
const IDLE_MS = 30 * 60 * 1000
const IDLE_CHECK_MS = 60 * 1000
const MODELS_MS = 10 * 60 * 1000

const manager = new AgentManager(sdk, {
  live: LIVE_SESSIONS,
  idleMs: IDLE_MS,
  modelsMs: MODELS_MS,
  // Outside ~/.local/share/agent-manager, which each install replaces.
  settingsFile: path.join(
    os.homedir(),
    ".local/state/agent-manager/sessions.json",
  ),
})
manager
  .prune()
  .catch((e: unknown) => console.error("pruning saved settings:", e))
setInterval(() => manager.closeIdle(Date.now()), IDLE_CHECK_MS)

net.createServer((socket) => manager.connect(socket)).listen(PORT, "127.0.0.1")
