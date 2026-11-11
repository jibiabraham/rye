# Run a forge with CI

This tutorial runs Forgejo and a Forgejo Actions runner on a VM called `forge`. A second VM, `dev2`, calls Forgejo
through an integration. The integration's credential is a bot user's token.

On `dev2` you clone a repository, push a branch, open a pull request and read its CI result. You run the steps by
hand, then repeat them through Claude.

This tutorial continues from [First run](first-run.md). Open the browser and run the `ssh rye` and `ssh <vm>.rye`
commands on the Incus host, from the root of a clone of this repository.

## 1. Create the forge VM
Where: the Incus host.

    ssh rye new forge --cloud-init < deployments/forge-vm/cloud-init.yaml

`deployments/forge-vm/cloud-init.yaml` installs Docker CE and starts two containers with Docker Compose:

| Container | Runs |
|---|---|
| `forgejo` | Forgejo 17, listening on `127.0.0.1:3000` |
| `runner` | Forgejo runner 13, which runs each CI job in a container on the VM's Docker |

First boot also registers the runner with Forgejo. `rye new` prints `Ready: forge in <duration>`.

Check the containers:

    ssh forge.rye "sudo docker compose -f /srv/forge/compose.yaml ps --format '{{.Service}} {{.Status}}'"

It prints one line each for `forgejo` and `runner`. Each status starts with `Up`.

## 2. Create the admin user
Where: the Incus host, then the forge VM's shell.

Log in to the forge VM:

    ssh forge.rye

Define `fj`, a shell function that runs the Forgejo command line in its container. Then create the admin user `owner`:

    fj() { sudo docker compose -f /srv/forge/compose.yaml exec -T -u git forgejo forgejo "$@"; }
    PASSWORD=$(openssl rand -hex 12); echo "$PASSWORD"
    fj admin user create --admin --username owner --password "$PASSWORD" --email owner@example.com \
      --must-change-password=false

It prints `New user 'owner' has been successfully created!`. Save the printed password for the browser login in
step 3. Keep this shell open. Steps 4 to 6 run in this shell and use `fj`, `$PASSWORD` and the variables that step 4
sets.

## 3. Open Forgejo in a browser
Where: a second terminal on the Incus host.

Forward port 3000:

    ssh -N -L 3000:127.0.0.1:3000 forge.rye

The command prints nothing and keeps running. Leave it open until step 12.

Open <http://localhost:3000> and log in as `owner`. The page <http://localhost:3000/admin/actions/runners> lists the
runner from step 1, with the label `docker`.

## 4. Create a repository with CI
Where: the forge VM's shell.

Set two variables for the API calls:

    AUTH="owner:$PASSWORD"
    API=http://127.0.0.1:3000/api/v1

Create `owner/demo` with a first commit on `main`:

    curl -fsS -u "$AUTH" -H 'Content-Type: application/json' "$API/user/repos" \
      -d '{"name":"demo","auto_init":true,"default_branch":"main"}' | jq -r .full_name

It prints `owner/demo`.

Write a workflow that checks out the code and lists its files on every push and pull request:

    cat > ci.yaml <<'EOF'
    name: ci
    on: [push, pull_request]
    jobs:
      test:
        runs-on: docker
        steps:
          - uses: actions/checkout@v4
          - run: ls -la
    EOF

`runs-on: docker` selects the runner by its label. The contents API adds the file as `.forgejo/workflows/ci.yaml`:

    jq -n --arg content "$(base64 -w0 ci.yaml)" '{content: $content, message: "Add CI"}' \
      | curl -fsS -u "$AUTH" -H 'Content-Type: application/json' -d @- \
          "$API/repos/owner/demo/contents/.forgejo/workflows/ci.yaml" | jq -r .commit.sha

The commit starts a CI run. Open <http://localhost:3000/owner/demo/actions> to see its
result.

## 5. Create the bot user and its token
Where: the forge VM's shell.

Create `bot` and give it write access to `owner/demo`:

    fj admin user create --username bot --password "$(openssl rand -hex 12)" --email bot@example.com \
      --must-change-password=false
    curl -fsS -u "$AUTH" -X PUT -H 'Content-Type: application/json' "$API/repos/owner/demo/collaborators/bot" \
      -d '{"permission":"write"}'

Create a token that lets `bot` push, open pull requests and read CI results:

    fj admin user generate-access-token --username bot --token-name dev2 --raw \
      --scopes write:repository,write:issue,read:user

Copy the printed token for step 7.

## 6. Protect main
Where: the forge VM's shell.

A write collaborator can also push to `main` and merge pull requests. This rule restricts pushes to `main` and merges
to `owner`:

    curl -fsS -u "$AUTH" -H 'Content-Type: application/json' -d @- "$API/repos/owner/demo/branch_protections" <<'EOF' \
      | jq -r .rule_name
    {
      "rule_name": "main",
      "enable_push": true,
      "enable_push_whitelist": true,
      "push_whitelist_usernames": ["owner"],
      "enable_merge_whitelist": true,
      "merge_whitelist_usernames": ["owner"]
    }
    EOF

It prints `main`.

Type `exit` to leave the forge VM.

## 7. Add the integration and create dev2
Where: the Incus host.

Paste the token from step 5 at the `read` prompt, which shows nothing, and press Enter. Then add the integration:

    read -rs TOKEN
    printf 'token %s' "$TOKEN" | ssh rye integration add forge --url http://forge.rye:3000

The URL host `forge.rye` is the forge VM. [Architecture](../explanation/architecture.md) describes how the service VM
connects to it.

Create `dev2` with the integration attached:

    ssh rye new dev2 --integration forge

## 8. Check the bot's access and clone the repository
Where: the Incus host, then `dev2`'s shell.

Log in to `dev2`:

    ssh dev2.rye

Install git and jq. Then query Forgejo's `/user` endpoint. It returns the user that owns the token:

    sudo apt-get install -y git jq
    curl -fsS http://forge.int.rye/api/v1/user | jq -r .login

It prints `bot`. The service VM adds the token to each request.

Clone the repository through the same integration:

    git clone http://forge.int.rye/owner/demo.git
    cd demo
    git config user.name bot
    git config user.email bot@example.com

A VM without the integration gets a `403` response. To see it, open a third terminal on the Incus host and send the
request from `forge`, or from any other VM without the integration:

    ssh forge.rye curl -sS http://forge.int.rye/api/v1/user

It prints `forbidden`, the body of the `403` response.

## 9. Push a branch and open a pull request
Where: `dev2`'s shell, in the clone.

    git switch -c bot/hello
    echo hello > hello.txt
    git add hello.txt
    git commit -m 'Add hello'
    git push origin bot/hello
    curl -fsS -H 'Content-Type: application/json' http://forge.int.rye/api/v1/repos/owner/demo/pulls \
      -d '{"head":"bot/hello","base":"main","title":"Add hello"}' | jq -r .number

It prints `1`, the pull request number. The push and the pull request each start a CI run.

## 10. Read the CI result
Where: `dev2`'s shell, in the clone.

Read the combined commit status, one state for the whole commit:

    sha=$(git rev-parse HEAD)
    curl -fsS http://forge.int.rye/api/v1/repos/owner/demo/commits/$sha/status | jq -r .state

It prints `pending` while the jobs run, `success` when they pass and `failure` when a job fails.

Each entry in `statuses` is one job. This command prints each job's context, status and log page path:

    curl -fsS http://forge.int.rye/api/v1/repos/owner/demo/commits/$sha/status \
      | jq -r '.statuses[] | "\(.context) \(.status) \(.target_url)"'

It prints one line for the push run and one for the pull request run, such as
`ci / test (push) success /owner/demo/actions/runs/<run>/jobs/0`.

`actions/runs` lists the repository's runs, newest first:

    curl -fsS http://forge.int.rye/api/v1/repos/owner/demo/actions/runs \
      | jq -r '.workflow_runs[0] | "\(.status) \(.prettyref)"'

It prints the newest run's status and branch, `success bot/hello`.

## 11. Run the same work through Claude
Where: the Incus host, then `dev2`'s shell.

On the Incus host, install agent-manager on `dev2`:

    ssh rye agent-manager install dev2

In `dev2`'s shell, log in to Claude as in [step 6 of First run](first-run.md#6-create-a-vm-and-log-in-to-claude).
Then start `claude` in the clone and send this prompt:

    A rye integration sends requests to http://forge.int.rye as the Forgejo user bot. Don't look for a token on
    this VM. Do these steps:
    1. Create the branch bot/readme from main.
    2. Add a README.md with one line about this repo, then commit and push it.
    3. Open a pull request against main with the Forgejo API at http://forge.int.rye/api/v1.
    4. Poll the commit status until it leaves pending, then tell me the result.

Claude requests approval before each command. The push and the API calls use the integration. With the bot's token,
Claude can also change the workflow file, and the runner then runs it on the forge VM.

`dev2` also appears in the GUI from [step 7 of First run](first-run.md#7-open-the-gui). The same prompt works in a new
session there.

## 12. Merge as the owner
Where: the browser on the Incus host.

Open <http://localhost:3000/owner/demo/pulls> as `owner`. Each pull request page has its CI result and a merge button.
The branch protection from step 6 blocks `bot` from merging.

## Next steps
- [Upgrade the forge to a new Forgejo release](../how-to/forge-upgrade.md)
- [Give a VM access to an API](../how-to/integrations.md)
