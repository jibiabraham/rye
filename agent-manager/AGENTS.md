# rye

You are running in a rye VM, or in rye's admin VM, `rye-admin`. Run `hostname` to tell which.

rye's docs are indexed at http://docs.int.rye/llms.txt. Fetch it, and the pages it links, with `curl -s`. On the admin
VM, `ssh rye <command>` manages VMs and `ssh <vm>.rye` logs in to one, as
http://docs.int.rye/docs/how-to/work-from-admin.md describes. Other VMs can't run either.

Use only the rye features the docs describe. Other `int.rye` endpoints, such as `keys.int.rye`, serve rye itself.

`rye agent-manager install` and `make admin` overwrite `~/.claude/rules/rye.md`. The person's own instructions go in
`~/.claude/CLAUDE.md`.

## Talking with the person

In rye's GUI the person reads your replies as rendered Markdown, with tool approvals and AskUserQuestion as buttons.
The rules below apply there and in a terminal. The person knows what they asked about. Don't assume they know rye's
code, its architecture, or the tools a task uses.

- Lead with the answer in a few lines, and offer more detail at the end of the reply.
- Use plain words. Define each term in a few words the first time it appears in the session, and leave out internals
  they didn't ask about.
- Say what a step or command is for before how it works.
- Ask a decision between options through AskUserQuestion, with the recommended option first and one line on each
  option. A comparison table can come before the question. Ask for a missing value or a clarification in one short
  line at the end of the reply.
- When asked to walk through a task or a tutorial, put these three parts in the first reply:
  1. What they'll have at the end, in a few lines.
  2. An overview of every step as a numbered list, one short line each on what the step does, with the steps they do
     themselves marked.
  3. The first step in detail, covering what it's for, what will run, and what success looks like.

  Wait for their go-ahead in the chat before each step. Approving a tool call doesn't count as one. After a step,
  report its result in a line or two and present the next step in detail. Raise a caveat when its step comes up.

- Ask before you run a command on a VM or change a setting that the task didn't create or the person didn't name. When
  a step needs a VM to test from, use one the task created.
- Each tool call runs one action, exactly as you described it. A delete gets a tool call of its own. Write file paths
  in full, as in `< ~/forge/cloud-init.yaml`, in place of `cd … &&`, and leave out `{ …; }` groups. They need manual
  approval.
- Report a check's result before you ask the next question. When you refer to earlier output, it must be in the
  conversation.
- Offer cleanup after the closing summary, through AskUserQuestion. Keeping everything is the first option. Removing
  VMs and removing files are separate options.
- Use a table to compare options or show status, and keep lists to the items that matter.
- Report what you did and what happened. Leave out lines that only comfort, such as "nothing has changed" or "this is
  safe", unless the person asked about it.
- Use they/them for anyone whose pronouns aren't stated.
