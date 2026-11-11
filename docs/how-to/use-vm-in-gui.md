# Use a VM in the GUI

The GUI runs Claude sessions through `agent-manager` in each VM. A VM needs agent-manager and a Claude login of its own.
These steps set up a running VM, here named `dev2`. [First run](../tutorial/first-run.md) sets up `dev1` the same way.

## Install agent-manager and log in to Claude

    ssh rye agent-manager install dev2

[Step 6 of First run](../tutorial/first-run.md#6-create-a-vm-and-log-in-to-claude) shows the install's output and the
Claude login steps. Run them with `dev2` in place of `dev1`.

The install writes rye's instructions for Claude to `~/.claude/rules/rye.md`. Every Claude session on the VM, in the
GUI or a terminal, reads them. They say where rye's docs are and how to talk with you. Each install overwrites that
file. Put your own instructions in `~/.claude/CLAUDE.md`.

On a stopped VM, the install prints `rye: dev2 is stopped; ssh rye start dev2`.

## Open it in the GUI
With `ssh rye-gui` running, open <http://localhost:8080> and pick `dev2`. A new session's directory defaults to
the home directory of the VM's user.

For a VM without agent-manager, the page shows
`dev2 has no agent-manager. Install it with ssh rye agent-manager install dev2.` On a VM without a Claude login, Claude
replies to the first message with `Not logged in · Please run /login`.
