# Docker

These functions act on every container of a host at once (start them all,
stop them all, or remove everything Docker holds), and keep Compose stacks
current: find the stacks on a host, check which of their images have a newer
version on the registry, and upgrade the services that follow a moving tag.

Platforms: Linux, macOS and WSL, wherever the `docker` command works (Docker
Engine, or Docker Desktop and its WSL integration). The Compose functions
need Compose v2 (`docker compose`) or `docker-compose`. Finding stacks with
`docker_compose_scan` needs neither.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [docker](../requirements.md#tool-docker) (feature), for `docker_compose_check`, `docker_compose_upgrade`, `docker_prune_all`, `docker_start_all`, `docker_stop_all` and `llm_docker_check`
- [docker-compose](../requirements.md#tool-docker-compose) (optional), for `docker_compose_check`, `docker_compose_upgrade` and `llm_docker_check`
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).

The functions run `docker` as your user: you need the rights `docker ps`
needs (membership of the `docker` group, or rootless Docker). Apart from the
scan, their output depends on your containers and on the registries, so this
page shows the commands and describes what they print.

## Start or stop every container

[`docker_stop_all`](reference.md#docker_stop_all) (`dsa`) stops every running
container, and [`docker_start_all`](reference.md#docker_start_all) (`daa`)
starts every stopped one (exited or just created):

```bash
dsa             # list the running containers, ask, stop them
daa -y          # start them again without the question
dsa && daa      # restart everything
```

Each prints `Will stop N container(s): NAMES` (or `start`) and asks
`Do you want to continue? [Y/n]`: Enter goes ahead. `-y` or `--yes` skips
the question and is required without a terminal. With nothing to do they say
so (`No running containers to stop.`) and return 0.

## Remove everything Docker holds

[`docker_prune_all`](reference.md#docker_prune_all) (`dpa`) returns a host to
an empty Docker: it stops and removes every container, removes every image,
every volume and every network you created, then runs
`docker system prune --volumes -a --force` for the build cache. Volume data
is lost, and nothing comes back. Count first:

```bash
dpa --dry-run   # count what would go, change nothing
dpa             # the same counts, a warning, then a question
```

`--dry-run` prints
`Info: To remove: N container(s), N image(s), N volume(s), N network(s), plus the build cache.`
The question defaults to no, and `-y` skips it. Without a terminal it refuses
unless you pass `-y`. Docker's own networks (`bridge`, `host`, `none`) stay.
To free space and keep your volumes, Docker's own `docker system prune`
(without `--volumes`) or the `dirm` alias (`docker image prune -a`) are the
gentler choices.

## Find the Compose projects on a host

[`docker_compose_scan`](reference.md#docker_compose_scan) (`dcscan`) looks for
`compose.yaml`, `compose.yml`, `docker-compose.yml` and `docker-compose.yaml`
below a directory, three levels deep unless you pass `--depth`, and lists
each project with its services and their images:

<!-- example
mkdir -p srv/shop srv/blog/deploy srv/notes
printf 'services:\n  db:\n    image: postgres:16.4\n  web:\n    image: nginx:latest\n' >srv/shop/compose.yaml
printf 'services:\n  app:\n    image: ghost:5-alpine\n' >srv/blog/deploy/docker-compose.yml
-->
```console
$ dcscan srv

Scanning for Docker Compose files in ~/srv (depth: 3)

~/srv/blog/deploy
  • app: ghost:5-alpine

~/srv/shop
  • db: postgres:16.4
  • web: nginx:latest

Found 2 compose file(s)
$ dcscan srv --json
{"base_path":"~/srv","depth":3,"compose_files":[{"path":"~/srv/blog/deploy","file":"docker-compose.yml","services":["app"]},{"path":"~/srv/shop","file":"compose.yaml","services":["db","web"]}],"total_found":2}
```

It reads the files and starts no container. With `docker compose` installed
it asks Compose to read each file, so variables in image names resolve as
Compose resolves them; without it, it reads the services and images from the
file itself.

## Check a stack for newer images

[`docker_compose_check`](reference.md#docker_compose_check) (`dcc`) compares,
for each service of a Compose project, the image you have with the one on its
registry:

```bash
dcc                 # the project in the current directory
dcc /opt/shop       # another project
dcc /opt/shop --json
```

It prints one row per service with its image and a status:

- `CURRENT`: the image you have is the registry's.
- `UPDATE`: the registry has a newer image under the same tag.
- `NOT_PULLED`: the image is not on this host yet.
- `PINNED`: a digest, or a full version such as `nginx:1.25.3`, which the
  upgrade leaves alone.
- `ERROR`: the registry could not answer for this image; with `--json` the
  `error` field says why (`rate_limited`, `unauthorized`, `dns_error` and the
  others the [reference](reference.md#docker_compose_check) lists).

It asks each image's registry the way an anonymous `docker pull` would: it
never reads the credentials of `docker login`. For GHCR, an exported
`GITHUB_TOKEN` is exchanged for a read token, so private GHCR images work;
other private images show as `ERROR` with `unauthorized`. Image names may use
`${VARIABLES}`: they are filled from your exported environment and the
project's `.env`, as Compose does. The status is 0 whenever the report
prints, even with `ERROR` rows. AI agents call
[`llm_docker_check`](reference.md#llm_docker_check), the same check as JSON,
in which only the project's `.env` and a short list of common variables
(`HOME`, `USER` and the like) can fill an image name.

## Upgrade a stack

[`docker_compose_upgrade`](reference.md#docker_compose_upgrade) (`dcup2`)
pulls new images for the services whose tag moves, and recreates their
containers:

```bash
dcup2 --dry-run     # the plan and the commands it would run
dcup2               # the plan, what it will do, then a [y/N] question
dcup2 /opt/shop --yes
dcup2 --force       # pinned versions too
```

A tag counts as pinned when it is a full version, such as `1.25.3` or
`v2.4.0-alpine`, and so does an image pinned by digest: the upgrade skips
those unless you pass `--force`. Every other tag moves, whether it names a
channel (`latest`, `main`, `nightly`) or part of a version (`16`, `8.2`,
`5-alpine`), and its service is upgraded.

After you agree, it runs, in the project directory, `pull`, `stop` and
`up -d --remove-orphans` for those services, then `docker image prune -f`,
and shows `docker compose ps`. Know two side effects before you agree:
`--remove-orphans` removes the project's containers whose service is no
longer in its files, and `docker image prune -f` removes the dangling images
of the whole host, not only this project's. Compose reads the project's files
itself, so `docker-compose.override.yml` and `COMPOSE_FILE` apply.

## The docker aliases

With `docker` installed, short aliases cover the everyday commands: `dcls`
lists all containers, `dexec` is `docker exec -it`, `dlogs` follows a
container's logs, `dstopall`, `dstartall` and `dpruneall` are the functions
above. The `dc` family (`dcup`, `dcdown`, `dclogs`, `dcps`...) runs the
`docker-compose` program and exists only when that program is installed;
with Compose v2 only, type `docker compose`. `gash docker` lists the ones your
shell defines, and the [docker pack](reference.md#the-docker-pack) table has
them all. An alias is not defined when a program of the same name is
installed: `dc`, for example, is also the name of a calculator.

## When something goes wrong

| Message | Meaning and what to do |
|---|---|
| `Error: Docker is not installed.` | `docker` is not on `PATH` ([requirements](../requirements.md#tool-docker)) |
| `Error: Cannot list Docker objects: MESSAGE` | `docker` itself failed; MESSAGE is its first line. Usually the daemon is not running, or your user may not use its socket |
| `Error: Docker Compose not found. Install docker-compose or Docker Desktop.` | Neither `docker compose` nor `docker-compose` works |
| `Error: No docker-compose.yml found in DIR` | DIR holds none of the four Compose file names; pass the project directory |
| `Error: Cannot resolve path: 'PATH'` | The path does not exist |
| `Error: No services with images found in FILE` | Every service of the file is built locally (`build:` without `image:`): there is nothing to check on a registry |
| `Error: Cannot read DIR/.env: ...` | A quoted value in `.env` never closes, and Compose would refuse the whole file. Close the quote on the line the hint names |
| `Error: Invalid service name 'NAME' in FILE` | A service name starts with `-`; rename it |
| `ERROR` rows with `rate_limited` | Docker Hub limits anonymous requests; wait, or check fewer projects at a time |
| `Error: Refusing to upgrade in headless mode without --yes (pull + restart is destructive).` | Check the plan with `--dry-run`, then add `--yes` |
| `Info: Upgrade cancelled.` or `Info: Aborted.` | You answered no; nothing changed. Status 1 |
| `Error: No terminal to ask for confirmation: pass -y to go ahead.` | `docker_start_all`, `docker_stop_all` or `docker_prune_all` ran without a terminal |
| `Error: Failed to pull images. Aborting upgrade.` | A pull failed (registry, credentials, a missing tag) before anything was stopped; Compose's message above says which |
| `Error: Failed to start containers. Check logs with: ...` | The new images were pulled and the old containers stopped, but `up` failed: run the logs command it prints |
| `Error: -n is not --dry-run anymore; use --dry-run` | `docker_compose_upgrade` has only the long form |

## See also

- [reference.md](reference.md): every function and alias of this area, with all options.
- [Tools for AI agents](../agents/README.md): `llm_docker_ps`, `llm_docker_logs`, `llm_docker_exec` and `agent_docker_exec`.
- [System](../system/README.md): `services_stop`, which also stops the Docker service.
