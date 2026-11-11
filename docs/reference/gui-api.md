# GUI API

`ryed` serves a JSON API for the GUI on `127.0.0.1:8080` of the service VM. Errors are RFC 9457 problem details.

## Access
- The API has no login of its own. `ssh rye-gui` forwards to it and logs in with an SSH key that `make up`
  authorized. Requests must name `localhost` in `Host`, and a browser `Origin` must match it.
- After `make tailscale`, `tailscale serve` forwards to the same API on `127.0.0.1:8081`. On that port, `Host` must be
  the tailnet name. An `Origin`, when present, must be `https`. `Tailscale-User-Login` must be the owner's login.

## Endpoints

    GET    /api/vms                                             the list of VMs
    GET    /api/vms/{vm}/sessions                               agent-manager's `list` reply, with live `states`
    POST   /api/vms/{vm}/sessions  {cwd, model, effort, mode}   201 {session}
    GET    /api/vms/{vm}/models                                 models the VM's Claude account offers
    PATCH  /api/vms/{vm}/sessions/{id}           {title}        204
    PUT    /api/vms/{vm}/sessions/{id}/effort    {effort}       204; no effort means the model's default
    PUT    /api/vms/{vm}/sessions/{id}/mode      {mode}         204
    POST   /api/vms/{vm}/sessions/{id}/messages  {content}      202
    POST   /api/vms/{vm}/sessions/{id}/interrupt                202
    POST   /api/vms/{vm}/sessions/{id}/approvals/{request}      204; the body is the PermissionResult
    GET    /api/vms/{vm}/sessions/{id}/events                   server-sent events
    GET    /api/vms/{vm}/sessions/{id}/messages?before={uuid}   100 messages before `before`, {messages, more}
                                                 &from={uuid}   or 100 messages, extended to 10 before `from`
                                                 &all=1         or every earlier message

A refusal from agent-manager is a 400, and an unreachable agent-manager is a 502.

## Event stream
The stream starts with two events:

- `state` is agent-manager's `subscribe` reply, with pending approvals and whether the session exists.
- `history` is the transcript.

It then sends each of agent-manager's events as a `message` event. The stream ends with an `ended` event when that
connection closes. A `ping` event with `{}` arrives every 15 seconds. The GUI reconnects when a stream has been quiet
for 40 seconds. An `agent-manager-error` event carries agent-manager's error reply to `subscribe` or `messages`. The
GUI shows it as `Connection lost (<error>)`. After a reconnect, the stream sends `state` and `history` again. The GUI
ignores each message whose `uuid` it already has.
