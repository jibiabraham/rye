# Open the GUI from your phone

rye uses [Tailscale](https://tailscale.com/), which connects your own devices in a private network called a tailnet.
`make tailscale` adds the service VM to your tailnet and serves the GUI over HTTPS to devices signed in with your
login.

You need a tailnet with MagicDNS and HTTPS certificates turned on. Both settings are on the DNS page of the Tailscale
admin console. In steps 1 and 2, set `LOGIN` to your login as the Users page of the admin console shows it, such as
`you@github`.

## 1. Merge the policy
Print the policy with your login filled in:

    LOGIN=you@github make tailscale-policy

Open Access controls in the Tailscale admin console and merge its entries into the policy. Remove the default grant, the
entry with `"src": ["*"]` and `"dst": ["*"]`, which lets every source reach every destination. The policy's first grant
replaces it for your own devices. The policy includes tests, and the console rejects a policy that fails them.

## 2. Join the service VM

    LOGIN=you@github make tailscale

`make tailscale` prints a login link and a QR code. Open the link and approve the node.

`make tailscale` uses `tailscale serve` to publish a port of the service VM to the tailnet. If the Serve feature is
off on your tailnet, `make tailscale` prints a link to turn it on and waits until it is on.

It ends with the GUI's address and a policy check:

    GUI: https://rye-service.<tailnet>.ts.net, for you@github
    policy: only you@github reaches this node, on port 443

If the check finds a problem, it prints the policy with your login filled in. Merge it and run `make tailscale` again.
Later runs take `LOGIN` from the previous run when it isn't set.

## 3. Open the GUI
Install the Tailscale app on your phone and sign in as `LOGIN`. Open the `https://rye-service.<tailnet>.ts.net` address
from step 2.
