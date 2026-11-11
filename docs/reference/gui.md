# GUI

`ryed` serves the GUI on `127.0.0.1:8080` of the service VM. `ssh rye-gui` forwards it to <http://localhost:8080>,
and `make tailscale` serves it at `https://rye-service.<tailnet>.ts.net`.

## Pages

| URL | Page |
|---|---|
| `/`, `/vms` | The page of the VM you opened last |
| `/vms/{vm}` | A VM and its sessions, with a message box for a new session |
| `/vms/{vm}/sessions/{id}` | One session |
| `/vms/{vm}/sessions/{id}#{message-id}` | One session, scrolled to the message, with the message highlighted |

Session and message IDs are the IDs in the VM's transcripts. If a linked message isn't loaded, the page loads earlier
messages until it is.

## Layout
- The VM and session list sits beside the page. The button at the top left of each page hides or shows it. On a
  narrow screen the list opens over the page and closes when you pick a row.
- Opening a page with a message box puts keyboard focus in the box, on a device with a mouse. Other pages, and every
  page on a touch screen, put focus on the heading. A Skip to page link comes first in the tab order.
- With reduced motion set in the system, spinners and transitions stand still.

## Keys
| Key | Does |
|---|---|
| Enter | Sends the message. On a touch screen, Return starts a new line and Send sends. |
| Shift+Enter | Starts a new line. |
| Esc in the message box | Stops Claude's turn. |
| Esc in an approval card | Denies the call. |
| 1 to 9 in a question card | Picks an option. The first option takes focus as the card appears, unless the message box holds text. |
| Ctrl+Shift+O, or Cmd+Shift+O on a Mac | Opens a new session on the open VM. |
| Ctrl+B, or Cmd+B on a Mac | Hides or shows the VM and session list. |

## Sessions
- A running VM with agent-manager installed lists its sessions.
- The page of a running VM without agent-manager, and of its sessions, shows
  `<vm> has no agent-manager. Install it with ssh rye agent-manager install <vm>.`
- The page of a stopped VM, and of its sessions, shows `<vm> is stopped. Start it with ssh rye start <vm>.`
- A command the GUI suggests has a button that copies it.
- A new session starts with its first message. Under the message box are the model, the effort and the permission
  mode, with the working directory below the box.
- The session page has controls for approvals, Claude's questions, Stop and Rename.
- An approval for an edit or a write shows the file and a line diff of the change.
- A tool call waiting for approval says so in its row.
- Claude's questions show one at a time. Each has its options and a field for an answer of your own.
- The second approval button approves the call and applies the permission rules that Claude suggests for it. Its
  label names what the rules allow, such as `Always allow Bash(go test:*)`, or `Allow all edits for this session` for
  rules that end with the session.
- A command that writes outside Claude's allowed paths needs approval each time.
- Stop shows Stopping until the turn ends. Send is off while the connection to agent-manager is lost.
- Each of your messages, and each of Claude's turns, has a footer with Copy, the time and Copy link. A turn's Copy
  copies the text of all its replies. Footers show on hover, and on touch screens and the last message all the time.
- The page restores the reading position when you return to a session in the same tab.
- On a VM without a Claude login, Claude replies to a message with `Not logged in · Please run /login`. Below that
  reply, the page shows `Claude isn't logged in on <vm>. Log in to the VM over SSH, run claude and type /login.` The
  note clears on the next send.

## Markers
- The session list marks a session waiting for you, with an approval or a question, one with a turn running, and one
  that changed since you last opened it in this browser.
- Each VM row shows the markers of its sessions.
- The tab title starts with ● while any VM has a session waiting for you.
