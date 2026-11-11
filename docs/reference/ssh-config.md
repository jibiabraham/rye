# SSH config

`dist/ryectl ssh-config` on the Incus host writes rye's aliases to `~/.ssh/config.d/rye`. If `~/.ssh/config` lacks the
line `Include config.d/rye`, it prints that line to add at the top. `ssh rye ssh-config` prints the same blocks. `rye`
and `rye-gui` set `HostName` to the service VM's address, and `rye-admin` to the admin VM's, .11 of the same /24.
`make up` and `make admin` write the two VMs' host keys to `~/.ssh/known_hosts.d/rye`, and the last block keeps every
alias's host keys there, unhashed.

    Host rye
      HostName 198.18.0.10
      User rye
      RequestTTY no

    Host rye-gui
      HostName 198.18.0.10
      User rye
      LocalForward 8080 127.0.0.1:8080
      SessionType none

    Host rye-admin
      HostName 198.18.0.11
      User admin

    Host *.rye
      User dev
      ProxyJump rye

    Host rye rye-gui rye-admin *.rye
      UserKnownHostsFile ~/.ssh/known_hosts.d/rye
      HashKnownHosts no

| Alias | Use |
|---|---|
| `rye` | `ssh rye <command>` runs a [rye command](rye.md). |
| `rye-gui` | `ssh rye-gui` forwards `localhost:8080` to the GUI and API. |
| `rye-admin` | `ssh rye-admin` logs in to the [admin VM](../how-to/work-from-admin.md) as `admin`. |
| `<vm>.rye` | `ssh <vm>.rye` logs in to a VM as `dev`, through the service VM. |

- The aliases work on the Incus host and on the admin VM. The service VM accepts SSH from those two.
- SSH uses the first value set for each option. The `Include` line comes first in `~/.ssh/config`, ahead of any
  `Host *` block.
- Running `dist/ryectl ssh-config` again rewrites `~/.ssh/config.d/rye`.
- If another program uses local port 8080, forward a different local port with
  `ssh -N -L 8081:127.0.0.1:8080 rye`.
