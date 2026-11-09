# Gash

**A Bash toolkit for people at a terminal and for the AI agents working next to them.**

[![CI](https://github.com/mauriziofonte/gash/actions/workflows/ci.yml/badge.svg)](https://github.com/mauriziofonte/gash/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/tag/mauriziofonte/gash?sort=semver&label=release)](CHANGELOG.md)
[![Platforms](https://img.shields.io/badge/linux%20%C2%B7%20macOS%20%C2%B7%20WSL-amd64%20%7C%20arm64-4EAA25?logo=gnubash&logoColor=white)](#supported-systems)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

![Gash at a terminal: the overview, gash files largest, gash files stats and gash system info overview](docs/assets/demo.gif)

Gash adds 121 commands and 155 aliases to Bash, for the jobs that
otherwise take a pipeline or a search: find what fills a disk, see what
holds a port, tag a release, upgrade a Compose stack, copy files to a
server, query a database read-only, run local HTTPS sites, look after WSL
and Windows. Every command has `--help` with examples. For AI coding
agents it adds read-only `llm_*` functions and guarded `agent_*` write
functions that answer in JSON, with a documented error contract.

Gash is one static Go binary plus the Bash that loads it, installed in
your home directory for your user. It keeps your shell: the prompt,
aliases and safety defaults can each be turned off.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/mauriziofonte/gash/main/install.sh | bash
```

The installer checks that the system has what Gash needs (Bash 4.4 or
newer, git, curl or wget, tar, gzip, sha256sum or shasum, grep and sed),
offers to install what is missing with the system's package manager,
clones the newest release into `~/.gash`, downloads the binary of that
release and checks its SHA-256 against the hash the release pins, and adds
one block to your Bash profile. It asks before it changes anything;
`--assume-yes` installs without a terminal, `--check` only reports. Then
open a new terminal.

[docs/install.md](docs/install.md) covers the options, installs in CI and
containers, signature checks, updates, removal and what to do when an
install fails.

## Supported systems

CI installs Gash on each of these systems at every change, from the bare
image, and checks every package Gash can install there:

<!-- BEGIN generated: systems (go test ./internal/docgen -update) -->
| System | Version | amd64 | arm64 | Local sites (`gash localweb`) |
|---|---|---|---|---|
| Rocky Linux | 8 | tested | tested | not tested |
| Rocky Linux | 9 (minimal image) | tested | tested | not tested |
| AlmaLinux | 9 | tested | tested | tested |
| AlmaLinux | 10 | tested | tested | not tested |
| Oracle Linux | 9 | tested | tested | not tested |
| Amazon Linux | 2023 | tested | tested | not tested |
| Fedora | 44 | tested | tested | tested |
| Debian | 12 | tested | tested | tested |
| Ubuntu | 24.04 | tested | tested | tested |
| Alpine Linux | 3.22 | tested | tested | not tested |
| openSUSE Leap | 15.6 | tested | tested | not tested |
| openSUSE Tumbleweed | rolling | tested | tested | not tested |
| Arch Linux | rolling | tested | no image | tested |
| macOS (Homebrew Bash) | current | built, not tested | tested | tested (Homebrew httpd and PHP) |
<!-- END generated: systems -->

WSL 2 runs the Linux build. macOS needs a current Bash from Homebrew
(`brew install bash`): the system Bash 3.2 is too old.

## Quick start

Every command is `gash NAMESPACE COMMAND`, and Tab completes each word,
with a description, and the values they take (hosts, databases, tags,
containers):

```bash
gash                          # the overview: every namespace and its commands
gash files                    # one namespace: its commands, usage and shortcuts
gash help files largest       # full help with examples (shortcuts work too: gash help flf)
gash help --search tag        # search every help page
gash files largest ~ -n 10    # the ten largest files in your home
gash system info overview     # host, OS, load, memory and disks at a glance
gash doctor                   # check the install, and what this system lacks
```

Shortcuts save typing in an interactive shell: `flf` is
`gash files largest`, `gst` is `git status`, and `gash help shortcuts`
lists them all (`GASH_SHORTCUTS` picks the packs you want). The flat names
of Gash 3.x (`files_largest`) are gone from a new shell since 4.0:
`GASH_LEGACY_NAMES=1` brings them back, `gash doctor --legacy` finds the
ones your files still call, and [migration](docs/migration-4.0.md) has the
table.

![Finding things: gash git, Tab completion with descriptions, gash help --search tag, gash help on a shortcut](docs/assets/help.gif)

## What Gash does

### Files and disks

`gash files largest`, `gash files largest-dirs`, `gash files big-dirs`, `gash files stats` and
`gash files empty-dirs` find what takes the space. `gash files extract` unpacks
every common archive format and refuses, before extracting anything, an
archive whose members would land outside the target folder.
`gash files backup` makes a timestamped copy before you edit.
[Guide](docs/files/README.md)

![gash files largest-dirs, gash files empty-dirs, gash files extract refusing an unsafe tar, gash files backup](docs/assets/files.gif)

### Git

`gl` and its siblings draw a compact log. `gash git sync` stages, commits and
pushes; `gash git tag add` creates an annotated tag and pushes it; both show
what they will do and ask first (`--yes` skips the question, `--dry-run`
only shows). Around them sit 37 short aliases and 6 log helpers (`gst`,
`gcm`, `gpf`, `gswc`...) and `gash git repair` for a repository with damaged
objects. [Guide](docs/git/README.md)

![gl, gash git sync and gat with their plan and question, gash git tag list](docs/assets/git.gif)

### Processes, ports and the system

`gash system ps` and `gash system port-kill` answer "what holds port 8080" and free it,
after asking. `gash system info` reports the machine section by section,
`gash system disks` the mounts, `gash shell history-smart` searches your history with timestamps.
[Guide](docs/system/README.md)

![gash system ps, gash system port-kill with its question, gash shell history-smart](docs/assets/system.gif)

### Docker, remote hosts and databases

`gash docker compose check` tells which images of a Compose stack have a newer
version, and `gash docker compose upgrade` pulls them and restarts the stack.
`gash remote exec`, `gash remote push` and `gash remote sync push` work with the SSH
hosts you name in `~/.gash_env`, and refuse system paths on the other
side. The `DB:` connections of the same file are queried read-only by
`llm_db_query` and its siblings, for MySQL, MariaDB, PostgreSQL and SQLite,
locally or on a remote host. Guides: [Docker](docs/docker/README.md),
[remote hosts](docs/remote/README.md), [databases](docs/databases/README.md)

### PHP, Node.js and local sites

`gash runtime detect php` and `gash runtime detect node` read which version a project wants; `gash run --runtime node:project -- npm test` runs a command under it. `gash localweb` runs local Apache and PHP-FPM sites with HTTPS, one pool per PHP version, as your own user; installing it needs `sudo` once, for a few system steps it lists first. It was `gash hte` until 4.0 (HTE, *homelab test environment*, is the name of hte-cli, the standalone PHP tool it began as): `gash hte` is deprecated, still runs it through 4.x and goes away in 5.0, and Gash 4.1 moves the files of an existing install to their localweb names. Guides: [runtimes](docs/runtimes/README.md), [local sites](docs/localweb/README.md)

![gash localweb install, site create, curl https://shop.test, site list, service status](docs/assets/hte.gif)

### Windows and WSL, logs

Inside WSL, `gash wsl` checks and repairs the WSL setup and `gash win`
reads the Windows host (services, ports, updates, events, Defender);
`gash wsl compact` gives the disk space of the virtual disk back to Windows.
`gash logs` finds the log files that fill a disk and reclaims the space
without breaking the programs that write them.
Guides: [Windows and WSL](docs/windows/README.md), [logs](docs/logs/README.md)

### AI from the terminal

`ask` puts one question to Claude or Gemini from the terminal, with piped
files as context; `gash ai sysinfo` sends a report of this system for a
diagnosis, after asking. Secrets are redacted before anything is sent.
[Guide](docs/ai/README.md)

## For AI agents

Coding agents (Claude Code, Codex, Cursor, Gemini CLI, Aider, local
models) call Gash instead of composing pipelines. Gash has
48 read-only JSON functions for agents (`llm_*`: code search, project and git state, logs,
data files, databases, processes, services, containers, HTTP, DNS and TLS
probes, remote hosts) and a separate family of write functions
(`agent_*`: edit, upload, commit, run tests, stop a process) that need an
explicit target, offer `--dry-run`, and never touch secret files or system
paths.

```bash
~/.gash/bin/gash llm_git_overview     # the launcher: no shell setup
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; llm_db_query "SELECT id, email FROM users LIMIT 5"'
```

![An agent's view: llm_project, llm_git_status and a SELECT as JSON; a DELETE, a .env read and rm -rf refused](docs/assets/agents.gif)

Errors arrive on stderr as one JSON line, with an action the agent follows:
`RETRY` fix the arguments, `CONTINUE` skip this step, `STOP` ask the user,
`FATAL` report and stop.

```json
{"error":"write_operation_blocked","details":"Statement starts with 'DELETE'","action":"FATAL","recoverable":false,"hint":"Only SELECT, WITH, VALUES, TABLE, SHOW, DESCRIBE, EXPLAIN and PRAGMA are allowed"}
```

[AGENTS.md](AGENTS.md) is the contract: invocation, every function, the
output shapes, the error codes, the safety model, and snippets for
`CLAUDE.md`, `AGENTS.md` and system prompts. `gash agents-hint` prints a
routing table to paste into an agent's instructions.
[docs/agents/README.md](docs/agents/README.md) is the guide for the person
who sets the agent up.

## When a tool is missing

Most of Gash needs nothing beyond the install. A function that drives
another program (rsync, Docker, expect, a database client on a remote
host) checks for it first; when it is missing, the function stops with an
error that names the program and the command that installs it on your
system. `gash doctor` lists everything this system lacks, and
`gash doctor --install` installs it after asking:

```bash
gash doctor --prereqs           # what is missing, by area, with the install command
gash doctor --install rsync     # install one tool (asks first)
```

[docs/requirements.md](docs/requirements.md) lists every tool with its
package on each system.

## Configuration

Settings are `GASH_*` lines in `~/.gash_env`, a private file that also holds
the SSH keys to unlock, the database connections, AI tokens and remote
hosts (`gash config init` creates it from the [template](.gash_env.template)):

```bash
GASH_PROMPT=0            # keep your own PS1
GASH_SAFETY_ALIASES=0    # no cp -i, mv -i, rm -I
GASH_HISTORY_SYNC=1      # share history between terminals as you type
GASH_PATH_ADD="~/.local/bin ~/.cargo/bin"   # put these on PATH if they exist
DB:default=pgsql://reader:secret@localhost:5432/app
HOST:web=deploy@web.example.com
```

A variable set in the environment wins over the file.
[docs/configuration.md](docs/configuration.md) has the load order, every
setting and how to undo each one.

## Security

Gash runs as you and changes nothing as root, except the steps of
`gash localweb install` that it lists before asking for `sudo`. Release
binaries are pinned by SHA-256 in the release tag, and the release
workflow signs each release with Sigstore. The guardrails of the agent functions (read-only
database sessions, secret files, protected paths, an allowlist for
`llm_exec`) are defence in depth for your own account, not a sandbox
against a hostile user: [docs/security.md](docs/security.md) says what
they do and what they do not. Report vulnerabilities privately, as
[SECURITY.md](SECURITY.md) describes.

## Documentation

Each area has a guide and a reference generated from the help you see in
the shell:

<!-- BEGIN generated: areas (go test ./internal/docgen -update) -->
| Area | What it covers | Reference |
|---|---|---|
| [Shell](docs/shell/README.md) | The prompt, navigation, history search, shortcuts, colors, Tab completion and the overview of the commands that Gash adds to an interactive Bash. | [reference](docs/shell/reference.md) |
| [Files and archives](docs/files/README.md) | Find what fills a disk, summarize a tree, extract any archive and back up a file. | [reference](docs/files/reference.md) |
| [System](docs/system/README.md) | Processes, ports, services, disks, sudo history and a system report. | [reference](docs/system/reference.md) |
| [Git](docs/git/README.md) | Sync, tags, patches, repository repair and the gl* log shortcuts. | [reference](docs/git/reference.md) |
| [Docker](docs/docker/README.md) | Start, stop and prune containers, and check and upgrade Compose stacks. | [reference](docs/docker/reference.md) |
| [Remote hosts](docs/remote/README.md) | Run commands on, and copy and sync files with, the SSH hosts named in ~/.gash_env, and unlock SSH keys at login. | [reference](docs/remote/reference.md) |
| [Databases](docs/databases/README.md) | The MySQL, MariaDB, PostgreSQL and SQLite connections of ~/.gash_env: list, test and query them read-only. | [reference](docs/databases/reference.md) |
| [PHP, Node.js and Chrome](docs/runtimes/README.md) | Pick PHP and Node.js versions per project, run commands under a version, and keep headless Chrome current. | [reference](docs/runtimes/reference.md) |
| [Local web stack (localweb)](docs/localweb/README.md) | gash localweb: a userland Apache and PHP-FPM for local sites with HTTPS, one PHP-FPM pool per site. | [reference](docs/localweb/reference.md) |
| [Windows and WSL](docs/windows/README.md) | Inspect and repair WSL, and read the Windows host from inside it. | [reference](docs/windows/reference.md) |
| [Logs](docs/logs/README.md) | gash logs: find the log files that fill a disk and reclaim the space safely. | [reference](docs/logs/reference.md) |
| [AI assistants](docs/ai/README.md) | Ask Claude or Gemini from the terminal, with the context of this system. | [reference](docs/ai/reference.md) |
| [Tools for AI agents](docs/agents/README.md) | The read-only llm_* functions and the agent_* write tools that AI coding agents call: one JSON answer per call, with the safety rails of AGENTS.md. | [reference](docs/agents/reference.md) |
| [Gash itself](docs/gash/README.md) | Help, the doctor, prerequisites, settings, updates and removal. | [reference](docs/gash/reference.md) |
<!-- END generated: areas -->

Also: [install, update, remove](docs/install.md) ·
[requirements](docs/requirements.md) ·
[configuration](docs/configuration.md) · [security](docs/security.md) ·
[troubleshooting](docs/troubleshooting.md) · [AGENTS.md](AGENTS.md) ·
[changelog](CHANGELOG.md)

## Contributing

Bug reports, ideas and pull requests are welcome: see
[CONTRIBUTING.md](CONTRIBUTING.md). `go test ./...` runs the suite. CI runs
it on Linux (amd64 and arm64) and macOS, checks the generated Bash on Bash
4.4 to 5.3, installs Gash on the systems above, and runs `gash localweb` end to
end. The GIFs are recorded from [dev/tapes](dev/tapes/README.md).

## License

[MIT](LICENSE) © Maurizio Fonte
