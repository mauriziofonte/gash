# docker

Containers and Compose stacks.

_Generated from the help registry by `tools/gen-docs.sh`; do not edit by hand._

| Function | Alias | Description |
|---|---|---|
| [`docker_compose_check`](#docker-compose-check) | `dcc` | Check for available Docker Compose image updates |
| [`docker_compose_scan`](#docker-compose-scan) | `dcscan` | Scan directories for Docker Compose projects |
| [`docker_compose_upgrade`](#docker-compose-upgrade) | `dcup2` | Upgrade Docker Compose services to latest images |
| [`docker_prune_all`](#docker-prune-all) | `dpa` | Remove ALL Docker containers, images, volumes, and networks |
| [`docker_start_all`](#docker-start-all) | `daa` | Start all stopped Docker containers |
| [`docker_stop_all`](#docker-stop-all) | `dsa` | Stop all running Docker containers |
| [`llm_docker_check`](#llm-docker-check) |  | Docker Compose update check (JSON output for AI agents) |

## docker_compose_check

Check for available Docker Compose image updates

Alias: `dcc`

#### Usage

```text
docker_compose_check [PATH] [--json]
```

#### Examples

```bash
# Check for updates in current directory
dcc

# Check a specific project
dcc /opt/myapp

# JSON output for scripting
dcc --json
```

#### Status codes

```text
UPDATE     New image available on registry
UP-TO-DATE Image matches remote digest
PINNED     Version is pinned (e.g. nginx:1.25.3), skipped
ERROR      Could not reach registry
```

#### Notes

```text
Queries Docker Hub and GHCR registries to compare
local image digests with remote ones.
```

See also: `docker_compose_upgrade`, `docker_compose_scan`

## docker_compose_scan

Scan directories for Docker Compose projects

Alias: `dcscan`

#### Usage

```text
docker_compose_scan [BASEDIR] [--depth N] [--json]
```

#### Examples

```bash
# Scan current directory for compose files
dcscan

# Scan /opt with limited depth
dcscan /opt --depth 2

# JSON output for scripting
dcscan /home/deploy --json

# Full audit: scan then check each project
dcscan /opt --depth 3
```

See also: `docker_compose_check`, `docker_compose_upgrade`

## docker_compose_upgrade

Upgrade Docker Compose services to latest images

Alias: `dcup2`

#### Usage

```text
docker_compose_upgrade [PATH] [--dry-run] [--force]
```

#### Examples

```bash
# Upgrade services with mutable tags (latest, main, dev)
dcup2

# Preview what would be upgraded without doing it
dcup2 --dry-run

# Force upgrade even for pinned versions
dcup2 --force

# Upgrade a specific project
dcup2 /opt/myapp
```

#### Notes

```text
Only upgrades services with mutable tags (latest, main, dev, etc.).
Pinned versions (e.g. nginx:1.25.3) are skipped unless --force is used.
Pulls new images, recreates affected containers, removes old images.
```

See also: `docker_compose_check`, `docker_compose_scan`

## docker_prune_all

Remove ALL Docker containers, images, volumes, and networks

Alias: `dpa`

#### Usage

```text
docker_prune_all
```

#### Examples

```bash
# Nuclear cleanup (interactive confirmation)
dpa
```

#### Notes

```text
WARNING: This is destructive. It will:
  1. Stop all running containers
  2. Remove all containers
  3. Remove all images
  4. Remove all volumes (DATA LOSS)
  5. Remove all networks
  6. Run docker system prune
You will be asked for confirmation before proceeding.
```

See also: `docker_stop_all`

## docker_start_all

Start all stopped Docker containers

Alias: `daa`

#### Usage

```text
docker_start_all
```

#### Examples

```bash
# Start all stopped containers
daa

# Restart everything
dsa && daa
```

See also: `docker_stop_all`

## docker_stop_all

Stop all running Docker containers

Alias: `dsa`

#### Usage

```text
docker_stop_all
```

#### Examples

```bash
# Stop every running container
dsa

# Restart all containers: stop then start
dsa && daa
```

See also: `docker_start_all`, `docker_prune_all`

## llm_docker_check

Docker Compose update check (JSON output for AI agents)

#### Usage

```text
llm_docker_check [PATH]
```

#### Examples

```bash
# Check for updates (JSON output)
llm_docker_check

# Check a specific path
llm_docker_check /opt/myapp
```

#### Notes

```text
Wrapper around docker_compose_check --json.
Designed for LLM/AI agent consumption.
```

See also: `docker_compose_check`

## Aliases

| Alias | Runs |
|---|---|
| `dcls` | `docker container ls -a` |
| `dclsr` | `docker container ls` |
| `dils` | `docker image ls` |
| `dirm` | `docker image prune -a` |
| `dstop` | `docker stop` |
| `dstart` | `docker start` |
| `dexec` | `docker exec -it` |
| `dcrm` | `docker container rm` |
| `drm` | `docker rm` |
| `drmi` | `docker rmi` |
| `dlogs` | `docker logs -f` |
| `dinspect` | `docker inspect` |
| `dnetls` | `docker network ls` |
| `dstopall` | `docker_stop_all` |
| `dstartall` | `docker_start_all` |
| `dpruneall` | `docker_prune_all` |
| `dc` | `docker-compose` |
| `dcup` | `docker-compose up -d` |
| `dcdown` | `docker-compose down` |
| `dclogs` | `docker-compose logs -f` |
| `dcb` | `docker-compose build` |
| `dcrestart` | `docker-compose restart` |
| `dcps` | `docker-compose ps` |
| `dcpull` | `docker-compose pull` |
| `dcc` | `docker_compose_check` |
| `dcup2` | `docker_compose_upgrade` |
| `dcscan` | `docker_compose_scan` |

