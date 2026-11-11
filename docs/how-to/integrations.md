# Give a VM access to an API

An integration lets a VM call an API, such as GitHub's, without holding the token. Claude can read every file in a
VM, which is why the service VM stores the token. The VM sends requests to `http://<name>.int.rye/`, and the service
VM forwards them to the API with the token in a header. Claude can use every right the token grants.

## Add an integration
`integration add` reads the credential from stdin and sends it in the `Authorization` header. This adds an API token
as `Authorization: Bearer <token>`:

    printf 'Bearer %s' "$TOKEN" | ssh rye integration add example --url https://api.example.com

`--header` selects a different header, such as `--header X-Api-Key`.

[rye commands](../reference/rye.md#integrations) lists the rules for integration names and the reserved names.

`ssh rye integration ls` lists each integration with its URL, header and attached VMs.

## Attach it to a VM

    ssh rye integration attach example dev1

Or attach integrations when you create the VM, as a comma-separated list:

    ssh rye new dev1 --integration example,other

## Use it from the VM

    curl http://example.int.rye/v1/status

The path and query are appended to the integration's URL. The service VM removes any `Authorization` header the VM sends
and sets the integration's header. Requests from a VM without the attachment return `403 forbidden`.

## Reach a service on another VM
An integration URL whose host is `<vm>.rye` targets that VM. The service VM connects over SSH to `127.0.0.1:<port>`
on that VM. The port is the URL's port, or 80 for `http` and 443 for `https`. The service must listen on `127.0.0.1`.

This adds a Forgejo token as the credential for Forgejo on port 3000 of the VM `forge`:

    printf 'token %s' "$TOKEN" | ssh rye integration add forge --url http://forge.rye:3000

Requests to `http://forge.int.rye/` return `502` when `forge` is stopped. [Run a forge with CI](../tutorial/forge.md)
sets up this integration.

## Detach or remove it

    ssh rye integration detach example dev1
    ssh rye integration rm example
