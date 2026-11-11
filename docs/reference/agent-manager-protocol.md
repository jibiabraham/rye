# agent-manager protocol

`agent-manager` listens on `127.0.0.1:7420` inside each VM where it is installed. It runs Claude sessions with the Agent
SDK and reads the Claude login from `~/.claude`.

## Requests
The connection is plain TCP. Each message is one JSON object on its own line. Requests have an `id` and a `type`, and
the reply has the same `id`. The reply to a failed request is `{"id":<id>,"error":"<message>"}`.

    {"id":1,"type":"create","cwd":"/home/dev","model":"haiku"}     -> {"id":1,"session":"<uuid>"}
    {"id":2,"type":"subscribe","session":"<uuid>"}                 -> {"id":2,"exists":true,"live":false,...}
    {"id":3,"type":"send","session":"<uuid>","content":"hello"}    -> {"id":3}

| Type | Fields | Reply |
|---|---|---|
| `list` | none | `sessions`, `live` and `states`. `sessions` lists the stored sessions and the sessions that have had no `send` yet. `live` lists the IDs of live sessions. `states` gives each live session's `running` flag, `pending` count and `background` task count. |
| `create` | `model`, optional `cwd`, `effort` and `mode` | `session`, a new ID. The session starts on its first `send`, in `cwd` or the home directory. |
| `subscribe` | `session` | `exists`, `live`, `running`, `since`, `background`, `effort`, `mode` and `pending`. `since` is the start of the running turn, in milliseconds since the epoch. `pending` lists the tool calls waiting for an answer. The connection then receives the session's events. |
| `send` | `session`, `content` | Empty. Starts or resumes the session and queues the message. |
| `messages` | `session`, optional `limit`, `before`, `from` | `messages` and `more`. The reply holds up to `limit` messages before `before`. Without `before`, it holds the last `limit` messages. Without `limit`, it starts at the first message. With `from`, it starts 10 messages before `from`, or at the first message when the transcript no longer has `from`. `more` is true when earlier messages exist. |
| `rename` | `session`, `title` | Empty. |
| `interrupt` | `session` | Empty. Interrupts the running turn. |
| `answer` | `session`, `request`, `result` | Empty. `result` is a `PermissionResult` from the Agent SDK. |
| `models` | none | `models`, the models the VM's Claude account offers. |
| `effort` | `session`, optional `effort` | Empty. No `effort` restores the model's default. |
| `mode` | `session`, `mode` | Empty. Sets the permission mode, one of `default`, `acceptEdits`, `plan` and `auto`. A live session switches at once. |

## Events
Subscribers receive these events:

    {"session":"<uuid>","message":{...}}      every SDK message
    {"session":"<uuid>","permission":{...}}   a tool call waiting for an answer
    {"session":"<uuid>","answered":"<id>"}    the permission request was answered or withdrawn
    {"session":"<uuid>","ended":true}         the Agent SDK query that runs the session stopped

Each turn ends in a `result` message or an `ended` event. An `ended` event after a failure has an `error` field. The
next `send` after an `ended` event resumes the session.

## Lifetime
- A session closes after 30 idle minutes. A session with a running turn, a pending approval or a background task stays
  open.
- At most 4 sessions are live at once. A `send` that would start a fifth closes the least recently active idle session.
  When all 4 are busy, it fails with `all 4 live sessions are busy; wait for one to finish`.
- agent-manager runs as the systemd user unit `agent-manager` of the VM's user, `dev`, or `admin` on the admin VM.
  Systemd linger for that user starts it at boot.
- `rye agent-manager install` adds NodeSource's `node_26.x` apt repository, and unattended-upgrades updates it.
  agent-manager runs on that Node.js.
- `/usr/local/bin/claude` links to the `claude` CLI that the Agent SDK bundles, at agent-manager's version.
