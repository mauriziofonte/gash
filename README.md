# Gash

**A Bash toolkit for people at a terminal and for the AI agents working next to them.**

[![CI](https://github.com/mauriziofonte/gash/actions/workflows/ci.yml/badge.svg)](https://github.com/mauriziofonte/gash/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/tag/mauriziofonte/gash?sort=semver&label=release)](CHANGELOG.md)
[![Bash 4.4+](https://img.shields.io/badge/bash-4.4%2B-4EAA25?logo=gnubash&logoColor=white)](#requirements)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

![Gash at a terminal: the reference card, files_largest, tree_stats, disk_usage and sysinfo](docs/assets/demo.gif)

Gash adds about eighty functions and a hundred aliases to Bash: find what
fills a disk, inspect a server, tag a release, upgrade a Compose stack, push
files to a host. Every function has `--help` with examples, and the ones an
agent needs speak JSON with a documented error contract.

## What it is, and what it is not

**It is**

- A set of Bash functions you load from `~/.bashrc`, with a prompt, history
  settings, aliases and completion that you can each turn off.
- A read-only toolbox for coding agents (Claude Code, Codex, Cursor, Gemini,
  Aider, local models): JSON envelopes, stable error codes with an `action`
  to branch on, no colors and no noise in headless mode.
- Reversible: `gash_unload` restores your shell as it was, `gash migrate
  --rollback` restores your files, `gash_uninstall` removes everything.

**It is not**

- A new shell or a framework: it is Bash, sourced once.
- A sandbox. The agent guardrails (read-only database sessions, a command
  allowlist, secret-file checks) are defence in depth for your own account,
  not a security boundary against a hostile user. See the
  [security model](docs/security.md).
- A replacement for your tools: `top`, `df` or `less` stay what they are
  unless you opt in to the replacements.

## Requirements

- **Bash 4.4 or newer.** Linux and WSL are supported; macOS works on a best
  effort basis with Homebrew Bash and GNU tools
  (`brew install bash coreutils findutils gnu-sed grep gawk`).
- `git` to install and upgrade.
- Optional: `jq` (JSON output of most `llm_*` functions), `ripgrep`, `docker`,
  `sqlite3`/`mysql`/`psql` for the database functions, `expect` for SSH key
  unlocking.

## Install

**With curl**

```bash
curl -fsSL https://raw.githubusercontent.com/mauriziofonte/gash/main/install.sh | bash
```

**With wget**

```bash
wget -qO- https://raw.githubusercontent.com/mauriziofonte/gash/main/install.sh | bash
```

**From a clone** (read the installer first, or install elsewhere with `GASH_DIR`)

```bash
git clone https://github.com/mauriziofonte/gash.git ~/.gash
bash ~/.gash/install.sh
```

The installer checks out the latest release tag into `~/.gash`, writes
`~/.gashrc` and adds one delimited block to your Bash profile. It asks before
changing anything; `--assume-yes` installs without a terminal (CI, Docker),
`GASH_REQUIRE_SIGNED=1` refuses an unsigned release. Open a new terminal when
it is done.

## Quickstart

```bash
gash                      # the reference card: every topic, function and alias
gash_help files_largest   # full help with examples (aliases work too: gash_help flf)
files_largest ~ -n 10     # the ten largest files in your home
sysinfo overview          # host, OS, load, memory and disk at a glance
gash_doctor               # check the installation and the active settings
```

`gash <topic>` (files, system, nav, git, docker, remote, dev, llm, ai, gash)
shows one topic; `gash_help --search WORD` searches every help page.

## What's inside

| Topic | Highlights |
|---|---|
| [files](docs/reference/files.md) | `files_largest`, `dirs_largest`, `dirs_find_large`, `dirs_list_empty`, `tree_stats`, `archive_extract`, `file_backup` |
| [system](docs/reference/system.md) | `disk_usage`, `sysinfo`, `port_kill`, `process_find`, `services_stop`, `history_grep`, WSL helpers |
| [nav](docs/reference/nav.md) | `..`, `...`, `ll`, `la` and friends |
| [git](docs/reference/git.md) | `git_add_tag`, `git_delete_tag`, `git_list_tags`, `git_dump_revisions`, and 44 short aliases |
| [docker](docs/reference/docker.md) | `docker_compose_check` and `docker_compose_upgrade` (image updates for a stack), `docker_stop_all`, `docker_prune_all` |
| [remote](docs/reference/remote.md) | `remote_exec`, `remote_push`, `remote_pull`, `remote_sync_push`: hosts named in `~/.gash_env`, with a deny list for system paths |
| [dev](docs/reference/dev.md) | `php_detect_version`, `node_detect_version` per project, `php84`-style aliases |
| [llm](docs/reference/llm.md) | Eighteen read-only JSON functions for agents |
| [ai](docs/reference/ai.md) | `ai_query` and `ai_sysinfo`: Claude or Gemini from the terminal, with secrets redacted |
| [gash](docs/reference/gash.md) | `gash_help`, `gash_doctor`, `gash_upgrade`, `gash_unload`, `gash migrate` |

The [function reference](docs/reference/README.md) is generated from the same
help text you see in the shell, so it always matches the code.

## For AI agents

![An agent's view: JSON from files_largest, a write query refused, a command outside the allowlist refused](docs/assets/agents.gif)

```bash
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; llm_git_status'
# {"branch":"main","ahead":0,"behind":0,"staged":[],"modified":["README.md"],"untracked":[],"conflicted":[]}
```

Headless mode loads only the functions: no prompt, no aliases, no profile
files, no colors, nothing printed while loading. Errors arrive on stderr as
one JSON line:

```json
{"error":"write_operation_blocked","details":"Statement starts with 'DELETE'","action":"FATAL","recoverable":false,"hint":"Only SELECT, WITH, VALUES, TABLE, SHOW, DESCRIBE, EXPLAIN and PRAGMA are allowed"}
```

`RETRY` means fix the arguments, `CONTINUE` skip this step, `STOP` ask the
user, `FATAL` report and stop. [AGENTS.md](AGENTS.md) is the full contract,
with drop-in snippets for `CLAUDE.md`, `AGENTS.md` and system prompts.

## Configuration

Settings are `GASH_*` lines in `~/.gash_env`, a private file that also holds
SSH keys to unlock, database connections, AI tokens and remote hosts
(`gash_env_init` creates it from the [template](.gash_env.template)):

```bash
GASH_PROMPT=0            # keep your own PS1
GASH_REPLACE_TOOLS=1     # top->htop, df->pydf, diff->colordiff... when installed
GASH_HISTORY_SYNC=1      # share history live between terminals
DB:default=pgsql://reader:secret@localhost:5432/app
```

A variable set in the environment wins over the file. Everything else,
including the load order and every setting, is in
[docs/configuration.md](docs/configuration.md).

## Upgrading

```bash
gash_upgrade
```

`gash_upgrade` moves to the highest release tag and refuses to overwrite local
changes. **Coming from 1.x?** The first new shell opens a guided migration
that shows each change, asks what you prefer, previews the diff and backs up
every file; see [docs/migration.md](docs/migration.md).

## Uninstall

```bash
gash_unload      # this shell only: back to how it was before Gash
gash_uninstall   # remove Gash, its profile block and ~/.gashrc (asks first)
```

## Documentation

- [Function reference](docs/reference/README.md), generated from the help registry
- [Configuration](docs/configuration.md): files, load order, every setting
- [Agent guide](AGENTS.md): headless mode, JSON shapes, error contract, safety model
- [Security model](docs/security.md): what the guardrails do and do not do
- [Troubleshooting](docs/troubleshooting.md)
- [Upgrading from 1.x](docs/migration.md)
- [Changelog](CHANGELOG.md)

## Contributing

Bug reports, ideas and pull requests are welcome: see
[CONTRIBUTING.md](CONTRIBUTING.md). The test suite runs on Bash 4.4 to 5.3,
gawk and mawk, Ubuntu and macOS: `bash tests/run.sh`.

Report security issues privately, as described in [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) © Maurizio Fonte
