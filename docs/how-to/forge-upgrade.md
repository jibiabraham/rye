# Upgrade the forge to a new Forgejo release

The forge VM from [Run a forge with CI](../tutorial/forge.md) pins a major version of Forgejo and of the runner. The
image tags in `/srv/forge/compose.yaml` set both.

The daily `forge-update.timer` pulls new images for those tags. It restarts each container with a changed image. To
move to a new major version, change the tag.

Read the release notes at <https://forgejo.org/releases/> and <https://code.forgejo.org/forgejo/runner/releases>
before upgrading.

Log in to the forge VM from the Incus host. The commands in the next four sections run there, in `/srv/forge`:

    ssh forge.rye
    cd /srv/forge

## Back up Forgejo's data
Stop Forgejo and archive its data directory:

    sudo docker compose stop forgejo
    sudo tar -czf /srv/forgejo-backup.tar.gz -C /srv/forge forgejo

## Change Forgejo's tag
Change the `forgejo` service's image tag, for example from 17 to 18. Then pull the image and start Forgejo:

    sudo sed -i 's|forgejo/forgejo:17|forgejo/forgejo:18|' compose.yaml
    sudo docker compose pull forgejo
    sudo docker compose up -d forgejo

Forgejo migrates its database on start. Follow its log:

    sudo docker compose logs -f forgejo

Press Ctrl-C when the log prints `Listen: http://0.0.0.0:3000`.

## Change the runner's tag
Change the `runner` service's image tag, for example from 13 to 14. Then pull the image and start the runner:

    sudo sed -i 's|forgejo/runner:13|forgejo/runner:14|' compose.yaml
    sudo docker compose pull runner
    sudo docker compose up -d runner

## Roll back Forgejo
An older Forgejo cannot read a database that a newer Forgejo has migrated. To go back, restore the archive and set the
old tag. The restore discards everything written after the backup:

    sudo docker compose stop forgejo
    sudo rm -rf /srv/forge/forgejo
    sudo tar -xzf /srv/forgejo-backup.tar.gz -C /srv/forge
    sudo sed -i 's|forgejo/forgejo:18|forgejo/forgejo:17|' compose.yaml
    sudo docker compose up -d forgejo
