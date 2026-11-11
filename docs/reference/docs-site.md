# Docs served to VMs

`ryed` serves rye's docs and code at the deployed commit on `http://docs.int.rye/`, on the service VM's port 80.
Every VM and the admin VM can reach it.

| URL | Returns |
|---|---|
| `/` and `/llms.txt` | An [llms.txt](https://llmstxt.org/) index. It has the README's first paragraph, the commit, and links to the README and to each Markdown file directly in `docs/tutorial`, `docs/how-to`, `docs/reference` and `docs/explanation`, grouped by directory. Each link's text is the page's first `# ` heading. |
| `/llms-full.txt` | Every page that the index lists, in its order. A line `<!-- <path> -->` precedes each page. |
| `/<path>` | The repo's file at that path, such as `/docs/how-to/manage-vms.md` or `/deployments/forge-vm/cloud-init.yaml`. |

Every response is `text/plain`. A path that isn't a regular file in the deployed commit returns `404`, and a method
other than `GET` or `HEAD` returns `405`. `GET` and `HEAD` return `503` while the service VM has no deployed commit.

`make up` publishes the clone's `HEAD` commit, without uncommitted changes.

[Let Claude manage your VMs](../how-to/work-from-admin.md) pipes a file from the site to `ssh rye new`.
