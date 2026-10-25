# Gash

**A Bash toolkit for people at a terminal and for the AI agents working next to them.**

[![CI](https://github.com/mauriziofonte/gash/actions/workflows/ci.yml/badge.svg)](https://github.com/mauriziofonte/gash/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/tag/mauriziofonte/gash?sort=semver&label=release)](CHANGELOG.md)
[![Platforms](https://img.shields.io/badge/linux%20%C2%B7%20macOS%20%C2%B7%20WSL-amd64%20%7C%20arm64-4EAA25?logo=gnubash&logoColor=white)](#requirements)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

![Gash at a terminal: the reference card, files_largest, tree_stats and sysinfo overview](docs/assets/demo.gif)

Gash adds 98 functions and 163 aliases to Bash: find what
fills a disk, inspect a machine, tag a release, free a port, upgrade a
Compose stack, push files to a host, run local HTTPS sites. Every function
has `--help` with examples, and the ones an agent needs answer in JSON with
a documented error contract.

## What's new in 3.0

- **One static Go binary** does the work behind the same Bash functions.
  Names, flags, output, exit codes and `--json` envelopes stay as they were
  in 2.x, except for the changes listed in the [changelog](CHANGELOG.md).
  The prompt, history, aliases and completion are still plain Bash.
- **Fewer dependencies.** The `llm_*`, database and scanner functions no
  longer need `jq`, `sqlite3`, `mysql`, `psql`, `rg` or GNU `find`: SQLite,
  MySQL, MariaDB and PostgreSQL are spoken natively.
- **`gash hte` is built in**: local Apache and PHP-FPM sites with HTTPS,
  run as your own user. It replaces the separate hte-cli tool, and
  `gash hte migrate` takes over an existing hte-cli setup.
- **Verified upgrades.** The release binaries are pinned by SHA-256 in the
  tagged tree. `gash update` checks the new binary before it touches your
  install, tests the new version in a separate shell, and
  `gash update --rollback` goes back.
- **Audited.** The port reviewed every 2.x behaviour (443 items in the
  [ledger](docs/port/LEDGER.md)) and fixes the defects instead of carrying
  them over.

## Contents

[Install](#install) · [Quickstart](#quickstart) · [A tour](#a-tour) ·
[Local sites](#local-sites-with-gash-hte) · [For AI agents](#for-ai-agents) ·
[What's inside](#whats-inside) · [Configuration](#configuration) ·
[Upgrading](#upgrading) · [Documentation](#documentation)

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/mauriziofonte/gash/main/install.sh | bash
```

or `wget -qO- https://raw.githubusercontent.com/mauriziofonte/gash/main/install.sh | bash`.
To read the installer first, or to install somewhere else with `GASH_DIR`:

```bash
git clone https://github.com/mauriziofonte/gash.git ~/.gash
bash ~/.gash/install.sh
```

The installer checks out the latest release tag into `~/.gash`, downloads
the `gash` binary for your platform and checks its SHA-256 against the hash
the tag pins, writes `~/.gashrc` and adds one
delimited block to your Bash profile. It asks before changing anything.
Open a new terminal when it is done.

| Option | Effect |
|---|---|
| `--assume-yes` | Install without a terminal (CI, Docker) |
| `--repair` | Fetch and verify only the binary for the installed version; nothing else changes |
| `GASH_REQUIRE_SIGNED=1` | Refuse a release whose tag or Sigstore signature does not verify (needs `cosign`), and any binary the tag does not pin |

### Requirements

- **Linux** (amd64, arm64), **WSL2**, or **macOS** (Intel, Apple silicon).
- **Bash 4.4 or newer.** macOS ships Bash 3.2: `brew install bash` first
  ([details](docs/troubleshooting.md#gash-requires-bash-44-or-newer)).
- `git`, and `curl` or `wget`.
- Optional, for the functions that drive them: `docker`, `ssh`/`scp`/`rsync`
  (remote hosts), `expect` (SSH key unlocking), `fzf` (`hgrep --pick`),
  Apache and PHP-FPM (`gash hte`).

## Quickstart

```bash
gash                      # the reference card: every topic with its functions and aliases
gash git                  # one topic: functions, short aliases, usage
gash_help files_largest   # full help with examples (aliases work too: gash_help flf)
gash_help --search tag    # search every help page
files_largest ~ -n 10     # the ten largest files in your home
sysinfo overview          # host, OS, load, memory and disk at a glance
gash_doctor               # check the installation and the active settings
```

![Finding things: gash git, gash_help --search tag, gash_help on an alias](docs/assets/help.gif)

## A tour

### Files and disks

`files_largest`, `dirs_largest`, `dirs_find_large`, `tree_stats` and
`dirs_list_empty` find what takes the space. `archive_extract` unpacks any
common format, and refuses an archive whose members would land outside the
target folder before it extracts anything. `file_backup` makes a
timestamped copy before you edit.

![dirs_largest, dirs_list_empty, archive_extract refusing an unsafe tar, file_backup](docs/assets/files.gif)

### Git

`gl` and its siblings draw a compact, colored log. `git_sync` stages,
commits and pushes, `git_add_tag` creates an annotated tag and pushes it:
both show what they are about to do and ask first (`--yes` skips the
question, `--dry-run` only shows). Around them sit short aliases (`gst`,
`gcm`, `gpf`, `gswc`...) and `git_repair` for a repository with corrupted
objects.

![gl, git_sync and git_add_tag with their plan and question, git_list_tags](docs/assets/git.gif)

### Processes, ports, history

`process_find` and `port_kill` answer "what is holding port 8080" and free
it, after asking. `hgrep` searches your history with timestamps and
de-duplication. `sysinfo` reports the machine section by section, and on
WSL `wsl_compact`, `wsl_restart` and friends handle the Windows side.

![process_find, port_kill with its question, hgrep](docs/assets/system.gif)

## Local sites with `gash hte`

`gash hte` runs local Apache and PHP-FPM sites, with HTTPS and HTTP/2, as
your own user. Install needs `sudo` once, for a few system steps it lists
before running them: a small helper that may edit only the gash block of
`/etc/hosts`, the sudoers line for that helper, and the sysctl that lets
your user bind ports 80 and 443. After that no command runs as root. It
uses the Apache and PHP-FPM your system already has, one pool per PHP
version, and `gash hte uninstall` puts every system file back as it was.

```bash
gash hte install                                  # one sudo, then the stack runs as you
gash hte site create --domain shop.test --docroot public
curl -k https://shop.test/                        # self-signed, 10-year certificate per site
gash hte doctor                                   # checks everything, names the fix for each failure
gash hte migrate                                  # coming from hte-cli: a dry-run plan
```

![gash hte install, site create, curl https://shop.test, site list, service status](docs/assets/hte.gif)

It works on Debian, Ubuntu, Fedora, Arch, WSL2 and macOS with Homebrew
(through a `pf` redirect for ports 80 and 443). The [guide](docs/hte.md)
covers every command, the hte-cli command table and the WSL notes.

## For AI agents

Coding agents (Claude Code, Codex, Cursor, Gemini, Aider, local models) load
Gash headless and get read-only tools that answer in JSON: the project's
type and tree, code search, git state, database schemas and queries,
processes and ports. Writes, secret files and commands outside an allowlist
are refused with an error the agent can act on.

![An agent's view: llm_project, llm_git_status and a SELECT as JSON; a DELETE, a .env read and rm -rf refused](docs/assets/agents.gif)

```bash
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; llm_db_query "SELECT id, email FROM users LIMIT 5" -c default'
```

Headless mode loads only the functions: no prompt, no aliases, no colors,
nothing printed while loading. Errors arrive on stderr as one JSON line:

```json
{"error":"write_operation_blocked","details":"Statement starts with 'DELETE'","action":"FATAL","recoverable":false,"hint":"Only SELECT, WITH, VALUES, TABLE, SHOW, DESCRIBE, EXPLAIN and PRAGMA are allowed"}
```

`RETRY` means fix the arguments, `CONTINUE` skip this step, `STOP` ask the
user, `FATAL` report and stop. [AGENTS.md](AGENTS.md) is the full contract,
with JSON shapes, the safety model and drop-in snippets for `CLAUDE.md`,
`AGENTS.md` and system prompts.

The guardrails are defence in depth for your own account, not a sandbox
against a hostile user: see the [security model](docs/security.md).

## What's inside

| Topic | Highlights |
|---|---|
| [files](docs/reference/files.md) | `files_largest`, `dirs_largest`, `dirs_find_large`, `dirs_list_empty`, `tree_stats`, `archive_extract`, `file_backup` |
| [system](docs/reference/system.md) | `disk_usage`, `sysinfo`, `port_kill`, `process_find`, `services_stop`, `hgrep`, `wsl_compact` and other WSL helpers |
| [nav](docs/reference/nav.md) | `..`, `...`, `ll`, `la` and friends |
| [git](docs/reference/git.md) | `git_sync`, `git_repair`, `git_add_tag`, `git_list_tags`, `git_dump_revisions`, `git_apply_patch`, 38 short aliases and 6 log helpers |
| [docker](docs/reference/docker.md) | `docker_compose_check` and `docker_compose_upgrade` (image updates for a stack), `docker_stop_all`, `docker_prune_all` |
| [remote](docs/reference/remote.md) | `remote_exec`, `remote_push`, `remote_pull`, `remote_sync_push`: hosts named in `~/.gash_env`, with a deny list for system paths |
| [dev](docs/reference/dev.md) | `node_upgrade`, `chrome_headless_update` (Chrome for Puppeteer and Playwright), `php_detect_version`, `node_detect_version`, `php84`-style aliases |
| [llm](docs/reference/llm.md) | 30 read-only JSON functions for agents |
| [agent](docs/reference/agent.md) | Write tools for agents (`agent_replace`, ...): explicit targets, `--dry-run`, all or nothing; kept apart so `llm_*` stays read-only |
| [ai](docs/reference/ai.md) | `ai_query` and `ai_sysinfo`: Claude or Gemini from the terminal, with secrets redacted |
| [gash](docs/reference/gash.md) | `gash_help`, `gash_doctor`, `gash_upgrade`, `gash_unload` |
| [hte](docs/hte.md) | `gash hte install`, `site`, `php`, `service`, `cert`, `hosts`, `doctor`, `migrate` |

The [function reference](docs/reference/README.md) is generated from the
same help text you see in the shell, and a test keeps them equal.

## Configuration

Settings are `GASH_*` lines in `~/.gash_env`, a private file that also holds
SSH keys to unlock, database connections, AI tokens and remote hosts
(`gash_env_init` creates it from the [template](.gash_env.template)):

```bash
GASH_PROMPT=0            # keep your own PS1
GASH_REPLACE_TOOLS=1     # top->htop, df->pydf, diff->colordiff... when installed
GASH_HISTORY_SYNC=1      # share history live between terminals
GASH_PATH_ADD="~/.local/bin ~/.cargo/bin"   # put these on PATH if they exist
GASH_NVM=lazy            # node on PATH now, nvm.sh loaded on first use
DB:default=pgsql://reader:secret@localhost:5432/app
```

A variable set in the environment wins over the file. The load order, every
setting and how to undo each one are in
[docs/configuration.md](docs/configuration.md).

## Upgrading

```bash
gash update               # or gash_upgrade: the newest release, verified
gash update --rollback    # back to the version you had before
```

`gash update` moves to the highest release tag, never to an older one, and
refuses to overwrite local changes. It downloads and verifies the new
binary before it touches the checkout, then loads the new version in a test
shell and checks out the previous one again if that fails. The previous
version and its binary are kept for `--rollback`.

- **From 2.x:** run `gash_upgrade` as usual; it moves you to 3.0. To go
  back, `git -C ~/.gash checkout v2.1.1` and open a new terminal.
- **From 1.x:** move to 2.1.0 first and run its guided migration, then
  upgrade; [docs/migration.md](docs/migration.md) has the steps.

## Uninstall

```bash
gash_unload      # this shell only: back to how it was before Gash
gash_uninstall   # remove Gash, its profile block and ~/.gashrc (asks first)
```

If you installed `gash hte`, run `gash hte uninstall --yes` first: it stops
the stack and restores the system files it changed.

## Documentation

- [Function reference](docs/reference/README.md), generated from the help registry
- [Local sites (`gash hte`)](docs/hte.md)
- [Agent guide](AGENTS.md): headless mode, JSON shapes, error contract, safety model
- [Configuration](docs/configuration.md): files, load order, every setting
- [Security model](docs/security.md): what the guardrails do and do not do
- [Troubleshooting](docs/troubleshooting.md)
- [Upgrading from 1.x](docs/migration.md)
- [Changelog](CHANGELOG.md)

## Contributing

Bug reports, ideas and pull requests are welcome: see
[CONTRIBUTING.md](CONTRIBUTING.md). `go test ./...` runs the suite; CI runs
it on Linux and macOS, checks the generated Bash on Bash 4.4 to 5.3, and
runs `gash hte` end to end on Debian, Ubuntu, Fedora, Arch and macOS. The GIFs above
are recorded from [docs/tapes](docs/tapes/README.md).

Report security issues privately, as described in [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) © Maurizio Fonte
