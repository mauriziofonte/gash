# Gash - Gash, Another SHell

> Oh Gash, was it _really_ necessary?
>
> **Dual-mode by design**: fluent for humans, machine-readable for LLM agents.

**Gash** is a no-fuss, colorful, and feature-rich replacement for your standard
Bash configuration files. It packs everything you need to make your terminal
experience faster, prettier, and more productive — while also exposing a
**first-class contract for LLM coding agents** (Claude Code, OpenAI Codex, Kimi,
Gemini, Cursor, Windsurf, Aider, local LLMs…): JSON envelopes, zero-ANSI output
in headless/pipe mode, read-only safeguards, path/command validation, and
structured error messages with behavioral actions.

## Why Gash?

* **Faster workflows**: Jump between directories, manage Git repos, stop services in one-liners.
* **LLM-friendly**: A dedicated `llm_*` API + `--json` envelopes + auto-off ANSI when stdout is a pipe. See [AGENTS.md](AGENTS.md) for the agent integration guide.
* **Safe by default**: Blocks destructive commands (`rm -rf /`, `dd`, `mkfs`, fork bombs), secret files (`.env`, `*.pem`, `*_rsa`), protected paths (`/proc`, `/sys`, `/dev`, `/boot`, `/root`), and DDL/DML on databases. Read-only DB queries, validated command exec.
* **Colorful output for humans, plain text for pipes**: Hybrid color policy honors `NO_COLOR`, `GASH_NO_COLOR`, `GASH_HEADLESS`, and automatic TTY detection.
* **Smarter shell**: Aliases, functions, and an informative prompt that simplifies tasks.
* **Lightweight**: No bloat — just the tools you need.
* **Memorable commands**: Each function has a **long descriptive name** AND a **short alias**.

## Features At A Glance

* **LLM / Agent integration**: `llm_*` API (exec, tree, find, grep, db_*, git_*, ports, procs, env, project, deps, docker_check), `--json` envelopes on all filesystem scanners, `GASH_HEADLESS=1` one-liner auto-config, structured error contract. See [AGENTS.md](AGENTS.md).
* **Filesystem audit (v1.5+)**: `files_largest`, `dirs_largest`, `dirs_find_large`, `dirs_list_empty`, `tree_stats` — all with `--json`, `--null`, `--xdev`, `--exclude`, `--min-size`, `--depth`, and O(N) single-walk aggregation.
* **AI Chat**: Ask questions, troubleshoot errors, or pipe configuration files to Claude/Gemini from your terminal (`ai_query`, `ai_ask`).
* **System Intelligence**: AI-powered server analysis with deep drill-downs and interactive Q&A (`ai_sysinfo`).
* **Intelligent Prompt**: Username, Git branch, jobs, last exit code — at a glance.
* **Productivity Aliases**: Shortcuts for file operations, Git commands, and service management.
* **Built-in Help System**: `gash_help function_name` for rich, example-driven help.
* **Dual Naming**: Every function has a descriptive LONG name and a memorable SHORT alias.
* **Bash Completion**: Context-aware tab completion (git tags, directories, flags, AI providers, help topics).
* **Unload / Restore Session**: Turn off Gash in the current shell and restore the previous state.
* **Hybrid color policy**: ANSI in interactive terminals, plain text in pipes/headless/`NO_COLOR` — no parser breakage for agents.
* **Cross-platform**: Works seamlessly on Linux, macOS, and Windows (WSL).

Check out the [full features list](#full-features-list) for a detailed breakdown, or jump to [LLM / Agent Integration](#for-llm-agents).

## Quickstart

Get Gash up and running in **under 60 seconds**:

### Install with cURL

```sh
curl -fsSL https://raw.githubusercontent.com/mauriziofonte/gash/refs/heads/main/install.sh | bash
```

### Install with Wget

```sh
wget -qO- https://raw.githubusercontent.com/mauriziofonte/gash/refs/heads/main/install.sh | bash
```

Once installed, type `gash_help` to explore all available functions, or `gash_help function_name` for detailed help with examples on any specific function.

The automated installer will add a new section to your shell profile (`.bashrc` or `.bash_profile`) with:

```sh
# >>> GASH START >>>
# Load Gash Bash - Do not edit this block, it is managed by Gash installer
if [ -f "$HOME/.gashrc" ]; then
  source "$HOME/.gashrc"
fi
# <<< GASH END <<<
```

If no shell profile is found, the installer will print instructions for you to add the sourcing block manually.

## For LLM Agents

> Target: Claude Code, OpenAI Codex, Kimi, Gemini, Cursor, Windsurf, Aider, local LLMs.

Gash was built to coexist cleanly with LLM coding agents. It ships with a
stable `llm_*` API, JSON envelopes for every structured output, automatic
ANSI-off in non-interactive contexts, and a documented error contract.

### Drop-in auto-configuration

Any agent can get the full Gash toolkit with **one line**:

```bash
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; <function> [args]'
```

That invocation guarantees:

* ✅ All `llm_*` functions loaded (exec, tree, find, grep, db_*, git_*, ports, procs, env, project, deps, docker_check)
* ✅ All v1.5 filesystem scanners with `--json` envelopes
* ✅ **Zero ANSI color pollution** on stdout and stderr — no parser breakage
* ✅ **No Bash history pollution** — `__gash_no_history` wraps every call path
* ✅ **No user-profile mutation** — skips aliases, PS1, `~/.bash_aliases`, `~/.bash_local`

### Example agent invocations

```bash
# Structured filesystem audit (JSON envelope)
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; files_largest --json --min-size 50M /var/log'
# {"data":[{"size":...,"size_human":"...","mtime":"...","path":"..."}],"count":N,"total_size":N,...}

# One-shot project stats (top extensions by size/count, depth, empty dirs)
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; tree_stats --json .'

# Read-only database query with auto-EXPLAIN on slow queries
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; llm_db_query "SELECT id, name FROM users WHERE active=1" -c default'

# Compact git context
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; llm_git_status; llm_git_diff --staged'

# Nested directory tree with per-node size + children_count
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; llm_tree --depth 3 --stats src/'
```

### Error contract (all `llm_*` + filesystem scanners)

Errors arrive on **stderr** as JSON:

```json
{"error":"<type>","action":"STOP|RETRY|CONTINUE|FATAL","recoverable":true,"details":"...","hint":"..."}
```

Branch on `.action`: `STOP` → ask user; `RETRY` → fix input, retry once; `CONTINUE` → proceed; `FATAL` → surface `hint`, stop.

### Safety rails (can't be bypassed)

* **Blocked commands**: `rm -rf /`, `dd`, `mkfs`, fork bombs, `curl|sh`, `wget|bash`, `chmod 777 /`, shutdown/reboot.
* **Blocked paths**: `/proc`, `/sys`, `/dev`, `/boot`, `/root`, `/etc/shadow`, `/etc/passwd`, `/etc/sudoers`; `..` traversal. Scanning `/` requires `--allow-root`.
* **Blocked secret files**: `.env*`, `*.pem`, `*.key`, `*_rsa`, `id_rsa*`, `id_ed25519*`, `credentials*`, `secrets*`, `.gash_env`.
* **Database**: read-only, DDL/DML blocked at parse time, multi-statement blocked, table-name allowlist.

### Full reference

See **[AGENTS.md](AGENTS.md)** for:

* Complete function catalog with argument shapes
* JSON envelope schemas for every structured output
* Error action semantics with branching examples
* Copy-paste drop-ins for `CLAUDE.md`, `AGENTS.md`, Kimi/Gemini system prompts
* Version compatibility notes

Agents and projects that want Gash-aware behavior should embed the relevant
section of `AGENTS.md` into their own configuration file (`CLAUDE.md`,
`AGENTS.md`, `.cursorrules`, system prompt, etc.).

## Features Breakdown

### Custom Command Prompt

Your new prompt shows:

* **Username** (colored by user type)
* **Current directory** (shortened to `~` for home)
* **Git branch & status** if you're inside a Git repo (shows `*` for dirty state)
* **Background jobs** and **last command exit code** for easy debugging.

```sh
# Example:
maurizio:~/projects (main*) $  # Git branch with dirty indicator
```

**Performance Optimizations:**

* Uses `git symbolic-ref` instead of `git name-rev` (faster branch detection)
* Limits git detection depth (avoids slowdown when home directory is a git repo)
* Uses `GIT_OPTIONAL_LOCKS=0` to prevent lock file creation
* Supports `GASH_GIT_EXCLUDE` environment variable to exclude specific repositories from prompt (set in `~/.bash_local`)

### Built-in Aliases

Gash improves everyday commands:

* **`..`**: Go up one directory, **`...`** for two, etc.
* **`ll`**, **`la`**, **`lash`**: Enhanced directory listings.
* **`gl`**: Professional git log with graph visualization (run `gl --help` for all variants).
* **`ga`**, **`gst`**, **`gc`**, **`gp`**: Common git operations.
* **`hg`** (or `history_grep`): Search Bash history with color-coded output.

And [many more](#aliases)!

### Useful Functions

Save time with these built-in utilities. Each has a **long** descriptive name AND a **short** memorable alias:

| Long Name | Short | Description |
|-----------|-------|-------------|
| `mkdir_cd` | `mkcd` | Make a directory and `cd` into it |
| `archive_extract` | `axe` | Extract almost any archive to a directory |
| `files_largest` | `flf` | Show the largest files in a directory |
| `process_kill` | `pk` | Kill all processes by name |
| `port_kill` | `ptk` | Kill processes running on a specific port |
| `services_stop` | `svs` | Stop well-known services like Apache, MySQL, Redis, Docker |
| `gash_unload` | - | Restore your shell state and remove Gash (current session only) |

And [many more](#helpers-functions)!

### Help System

Gash includes a rich, example-driven help system. Every public function has detailed help with usage patterns, real-world examples, and cross-references.

```bash
# See all available functions grouped by module
gash_help

# Get detailed help for any function (with examples)
gash_help ai_query
gash_help files_largest

# Works with aliases too - automatically resolves to the parent function
gash_help flf        # Shows help for files_largest
gash_help dsa        # Shows help for docker_stop_all

# Search across all functions by keyword
gash_help --search docker
gash_help --search pipe

# List all functions with short descriptions
gash_help --list

# Get a one-line description (useful for scripting)
gash_help --short files_largest

# Access bash builtin help
gash_help --bash cd
```

### Bash completion

Gash ships with a dedicated completion script (`bash_completion`). Once sourced (see `~/.gashrc`), it provides context-aware tab completion for all public Gash functions:

* **Git tag functions** (`git_add_tag`, `git_delete_tag`, `git_list_tags`): suggests Git tags
* **Directory functions** (`files_largest`, `dirs_largest`, `docker_compose_check`, etc.): suggests directories
* **File functions** (`archive_extract`, `file_backup`, `git_dump_revisions`): suggests files
* **`gash`**: suggests reference card sections (`git`, `files`, `system`, `docker`, `ai`, etc.)
* **AI functions** (`ai_ask`, `ai_query`): suggests providers (`claude`, `gemini`)
* **System intelligence** (`ai_sysinfo`): suggests providers (`claude`, `gemini`) and `--raw` flag
* **System enumeration** (`sysinfo`): suggests sections (`identity`, `storage`, `services`, etc.) and `--llm` flag
* **Help system** (`gash_help`): suggests function names, aliases, and flags (`--list`, `--search`, `--short`, `--bash`)
* **All other functions**: falls back to default file completion

**Note:** Completion is registered for function names only (e.g., `docker_stop_all`), not short aliases (e.g., `dsa`). This is a Bash limitation: Bash expands aliases before consulting completion specs, so `complete -F handler alias_name` is silently ignored.

### Unload Gash (session-only)

If you want to temporarily disable Gash in your current terminal (without uninstalling anything), run:

```sh
gash_unload
```

This performs a best-effort restore of the previous shell state (prompt variables, history settings, plus aliases/functions introduced by Gash).
To re-enable Gash in the same terminal:

```sh
source ~/.gashrc
```

## Power Up with Additional Tools

Gash works out of the box, but it shines when you install these optional tools:

| Original Command | Replacement | Description                              |
|------------------|-------------|------------------------------------------|
| `df`             | `pydf`      | Enhanced disk usage output               |
| `less`           | `most`      | Better paging for long files             |
| `top`            | `htop`      | Enhanced process viewer                  |
| `traceroute`     | `mtr`       | Interactive network diagnostics          |

### Install recommended tools

#### Debian/Ubuntu

```sh
sudo apt install most multitail pydf mtr htop colordiff
```

#### macOS (with Homebrew)

```sh
brew install most multitail pydf mtr htop colordiff
```

## Color policy

Gash follows a **hybrid color gating model** (v1.5+) designed so output is
always clean when consumed by scripts, LLM agents, or anything that isn't an
interactive terminal.

### When colors are emitted

Gash emits ANSI color codes **only if ALL of these are true**:

1. `GASH_HEADLESS` is **not** set to `1`.
2. `NO_COLOR` environment variable is **not** set (honors [no-color.org](https://no-color.org) standard).
3. `GASH_NO_COLOR` environment variable is **not** set (Gash-specific override).
4. `stdout` is a TTY (automatic off in pipes / redirects / captured output).

If any of those conditions is false, Gash emits plain text — including
`Error:`, `Warning:`, help pages, `gash_doctor`, `docker_compose_check`,
`sysinfo`, the entire `files_largest` / `dirs_largest` / `tree_stats` family,
and every inline colored print across the codebase.

### Per-command override

Several functions accept an explicit `--no-color` flag to force plain output
for a single invocation (useful when you want colors globally on but a
specific command to be script-friendly):

```text
files_largest --no-color         dirs_largest --no-color
dirs_find_large --no-color       dirs_list_empty --json
tree_stats --no-color            disk_usage --no-color
ip_public --no-color             gash_doctor --no-color
gash_help --no-color             docker_compose_check --no-color
docker_compose_upgrade --no-color  docker_compose_scan --no-color
sysinfo --no-color               sysinfo --llm      # --llm implies --no-color
ai_sysinfo --no-color            ai_sysinfo --raw   # --raw implies --no-color
```

### LLM / scripting use

If you integrate Gash in an LLM pipeline or automation script, prefer either
`GASH_HEADLESS=1` (skips prompt/alias loading too) or `GASH_NO_COLOR=1` (only
disables colors, keeps the rest of the shell experience). Both guarantee zero
ANSI escape sequences in stdout and stderr.

```bash
# Typical Claude Code / agent invocation
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; files_largest /var/log'

# Keep interactive extras, disable colors only
export GASH_NO_COLOR=1
```

## Customization

Gash is fully customizable. Want to add your own aliases or functions? Easy!

### Configuration files you can create

Gash is meant to be sourced from your shell startup files and will optionally load a few user-owned config files.
These files are the recommended way to customize behavior without editing the Gash repo.

#### `~/.gashrc` (entry point)

This is the file your `.bashrc`/`.bash_profile` sources. The installer creates it if missing.
It defines where Gash lives and loads both the main script and completion:

```sh
# ~/.gashrc
export GASH_BASH="$HOME/.gash/gash.sh"
export GASH_COMPLETION="$HOME/.gash/bash_completion"

if [ -f "$GASH_BASH" ]; then
  source "$GASH_BASH" # This loads Gash
  [ -f "$GASH_COMPLETION" ] && source "$GASH_COMPLETION" # This loads Gash completion
fi
```

Developer tip (working on a local checkout): you can point `GASH_BASH`/`GASH_COMPLETION` to a repo clone, or symlink your clone to `~/.gash`.

#### `~/.bash_aliases` (your personal aliases)

If present, Gash loads it near the end of startup. This is a good place for aliases that are specific to your machine/team.

```sh
# ~/.bash_aliases
alias k=kubectl
alias tf=terraform
alias dcu='docker compose up -d'
```

#### `~/.bash_local` (your local environment + tooling)

If present, Gash loads it after `~/.bash_aliases`. Use it for environment variables, PATH changes, and tool initialization.
This is ideal for development tooling that must be available in every terminal.

Example: enable `nvm` (Node.js version manager) reliably in interactive shells:

```sh
# ~/.bash_local
export NVM_DIR="$HOME/.nvm"
if [ -s "$NVM_DIR/nvm.sh" ]; then
  # shellcheck disable=SC1091
  . "$NVM_DIR/nvm.sh"
fi

# Optional: load bash completion for nvm (if you want it)
if [ -s "$NVM_DIR/bash_completion" ]; then
  # shellcheck disable=SC1091
  . "$NVM_DIR/bash_completion"
fi
```

Example: common Cloud/dev defaults (AWS, GCP, Kubernetes):

```sh
# ~/.bash_local

# AWS
export AWS_PROFILE="dev"
export AWS_REGION="eu-west-1"

# GCP (example: pick a default project)
export CLOUDSDK_CORE_PROJECT="my-dev-project"

# kubectl: default namespace/context helpers (optional)
alias kctx='kubectl config current-context'
alias kns='kubectl config set-context --current --namespace'
```

Note: prefer keeping secrets out of `~/.bash_local` (use your OS keychain/credential helpers where possible).

#### `~/.gash_env` (optional: SSH keys and database credentials)

This is the **unified configuration file** for sensitive credentials. It supports:

* SSH key auto-unlock (replaces the old `~/.gash_ssh_credentials`)
* Database connection strings (for LLM utilities)

##### Quick Setup

```sh
# Create from template
gash_env_init

# Or manually
cp ~/.gash/.gash_env.template ~/.gash_env
chmod 600 ~/.gash_env
```

##### SSH Keys Configuration

SSH key entries follow the format `SSH:keypath=passphrase`:

```sh
# ~/.gash_env
SSH:~/.ssh/id_ed25519=correct horse battery staple
SSH:~/.ssh/work_rsa=my-work-passphrase
```

SSH auto-unlock requires:

* `expect` installed (used to provide passphrases non-interactively)

Gash will automatically start `ssh-agent` if it's not running and SSH keys are configured in `~/.gash_env`.

##### Database Connections Configuration

Database entries follow URL-style format `DB:name=driver://user:password@host:port/database`:

```sh
# ~/.gash_env
DB:default=mysql://root:password@localhost:3306/myapp
DB:legacy=mysql://admin:oldpass@localhost:3307/legacy_db
DB:postgres=pgsql://pguser:secret@localhost:5432/analytics
DB:remote=mariadb://deploy:s3cr3t@192.168.1.100:3306/production
```

Supported drivers: `mysql`, `mariadb`, `pgsql`

**Note:** SQLite databases don't require configuration in `~/.gash_env`. Use the `-f` flag directly:

```bash
llm_db_query "SELECT * FROM users" -f /path/to/database.sqlite
llm_db_tables -f ./app.db
```

##### AI Providers Configuration

AI provider entries follow the format `AI:provider=API_KEY`:

```sh
# ~/.gash_env
AI:claude=sk-ant-api03-xxxxxxxxxxxxx
AI:gemini=AIzaSyxxxxxxxxxxxxx
```

Supported providers: `claude` (Anthropic), `gemini` (Google)

The first configured provider is used by default. You can specify a provider explicitly:

```bash
ai_query "how to list files?"           # Uses first available provider
ai_query claude "explain kubernetes"     # Uses Claude specifically
ai_query gemini "write a backup script"  # Uses Gemini specifically
```

##### PS1 Prompt Configuration

You can exclude specific git repositories from the prompt (useful for large repos or when your home directory is a git repo).

Add this to your `~/.bash_local` file (not `~/.gash_env`):

```sh
# ~/.bash_local
# Colon-separated list of repository roots to exclude from PS1 prompt
export GASH_GIT_EXCLUDE=/home/user:/path/to/huge/repo
```

**Password URL encoding:** If your password contains special characters, URL-encode them:

| Character | Encoded |
|-----------|---------|
| `@`       | `%40`   |
| `:`       | `%3A`   |
| `/`       | `%2F`   |
| `#`       | `%23`   |
| `%`       | `%25`   |

Example: password `p@ss:word` becomes `p%40ss%3Aword`

##### Database Helper Commands

```sh
# List all configured connections
gash_db_list

# Test a connection
gash_db_test default
gash_db_test postgres

# Create config from template
gash_env_init
```

##### Security Note

This file contains sensitive data in plain text. **Always set restrictive permissions:**

```sh
chmod 600 ~/.gash_env
```

Gash will warn you at startup if the permissions are too open.

You can override the config file location:

```sh
export GASH_ENV_FILE="$HOME/.config/gash/env"
```

### Create a `~/.bash_local` file

```sh
# ~/.bash_local example:
alias cls='clear'  # Custom alias
greet() {   echo "Hello, $USER!" }
```

Gash will load your custom settings automatically.

## Uninstalling Gash

Gash comes with an automated uninstaller.

To remove Gash and restore your original configuration:

```sh
gash_uninstall
```

Then, restart your terminal.

## Upgrading Gash

To upgrade Gash to the latest version, run the dedicated _Gash Helper_ function

```sh
gash_upgrade
```

This will fetch the latest version from the repository and update your local configuration.

Remember to restart your terminal to apply the changes.

## Full Features List

### Helpers (Functions)

All functions have a **long descriptive name** and a **short alias**. Use whichever you prefer!

#### File and Directory Operations

| Long Name | Short | Description |
|-----------|-------|-------------|
| `files_largest` | `flf` | List the largest files with size/exclude/json/depth filters |
| `dirs_largest` | `dld` | List the largest directories with depth and filter control |
| `dirs_find_large` | `dfl` | Find directories exceeding a size threshold (single-walk aggregation) |
| `dirs_list_empty` | `dle` | List empty directories with min-depth / exclude / count / null |
| `tree_stats` | `tls` | Compact filesystem stats: totals, top extensions, depth, empty dirs |
| `archive_extract` | `axe` | Extracts archive files (`.tar.gz`, `.zip`, `.7z`, etc.) |
| `file_backup` | `fbk` | Creates a backup of a file with a timestamp suffix |

**Disk usage tools (v1.5+):** the four scanners share a common flag surface —
`--limit N`, `--min-size SIZE`, `--depth N`, `--exclude GLOB`, `--xdev`,
`--allow-root`, `--json`, `--null`, `--human`, `--no-color`. They auto-prune
`node_modules`, `vendor`, `.git`, `__pycache__`, `.cache` (disable with
`--no-ignore`), refuse to scan `/` without `--allow-root`, reject forbidden
prefixes (`/proc`, `/sys`, `/dev`, `/boot`, `/root`), and emit a JSON envelope
`{data, count, total_size, total_size_human, scan_time, errors}` in `--json`
mode. `dirs_find_large` was rewritten in v1.5 to use a **single `du -k` pass +
awk filtering** (O(N) instead of O(N²) for nested trees), removed the hard
dependency on `numfmt` (pure-bash IEC fallback), and added `--with-mtime` for
per-directory newest-file timestamps. `tree_stats` is a new one-shot audit:
file/dir counts, cumulative size, top extensions by count and by size, max
depth, empty-dir count, all from a single `find -printf` walk.

```bash
# Top 20 files larger than 50MB, JSON envelope for scripting
files_largest --limit 20 --min-size 50M --xdev --json /var/log | jq '.data'

# Drill two levels deep into projects, ignoring vendored dirs
dirs_largest --depth 2 --min-size 100M ~/projects

# Find 1GB+ directories with newest-file mtime, capped at depth 6
dirs_find_large --size 1G --depth 6 --with-mtime /var

# Count empty directories, excluding hidden ones
dirs_list_empty --ignore-dotfiles --count /opt/app

# One-shot stats on a codebase (top-10 extensions by size + count)
tree_stats --top 10 ~/projects

# NUL-delimited paths for safe xargs pipelines
files_largest --min-size 100M --null /tmp | xargs -0 rm -i
```

#### System Operations

| Long Name | Short | Description |
|-----------|-------|-------------|
| `disk_usage` | `du2` | Displays disk usage for specific filesystem types |
| `history_grep` | `hg` | Searches Bash history for a pattern (removes duplicates) |
| `hgrep` | - | Smart history search with timestamps, deduplication, and options (`-n`, `-j`, `-E`, `-c`, etc.) |
| `ip_public` | `myip` | Displays your public IP address |
| `process_find` | `pf` | Searches for processes by name |
| `process_kill` | `pk` | Kills all processes matching a given name |
| `port_kill` | `ptk` | Kills processes running on a specified port |
| `services_stop` | `svs` | Stops well-known services (Apache, MySQL, Redis, Docker, etc.) |
| `sudo_last` | `plz` | Runs the last command or a given command with `sudo` |
| `mkdir_cd` | `mkcd` | Creates a directory and changes into it |

#### WSL (Windows Subsystem for Linux)

| Long Name | Short | Description |
|-----------|-------|-------------|
| `wsl_restart` | `wr` | Restart WSL (saves bash history, runs wsl --shutdown) |
| `wsl_shutdown` | `wsd` | Shutdown WSL (saves bash history, runs wsl --shutdown) |
| `wsl_explorer` | `wex` | Open Windows Explorer in current or specified directory |
| `wsl_taskmanager` | `wtm` | Open Windows Task Manager from WSL |

#### Git Operations

| Long Name | Short | Description |
|-----------|-------|-------------|
| `git_list_tags` | `glt` | Lists all local and remote tags |
| `git_add_tag` | `gat` | Creates an annotated tag and pushes it to remote |
| `git_delete_tag` | `gdt` | Deletes a tag both locally and on remote |
| `git_dump_revisions` | `gdr` | Dumps all revisions of a Git-tracked file into separate files |
| `git_apply_patch` | `gpatch` | Creates and applies patches from a feature branch to main |

#### Git Log Functions

Professional git log visualization with multiple variants. Run `gl --help` for full documentation.

| Function | Description |
|----------|-------------|
| `gl` | Compact log with graph (current branch) |
| `gla` | All branches with graph |
| `glo` | Ultra-compact oneline format |
| `glg` | Graph focused (first-parent only) |
| `gls` | Log with file statistics |
| `glf FILE` | File history with patches |

#### Docker Operations

| Long Name | Short | Description |
|-----------|-------|-------------|
| `docker_stop_all` | `dsa` | Stops all running Docker containers |
| `docker_start_all` | `daa` | Starts all stopped Docker containers |
| `docker_prune_all` | `dpa` | Removes all Docker containers, images, volumes, and networks |

**Docker Compose Smart Upgrade:**

| Long Name | Short | Description |
|-----------|-------|-------------|
| `docker_compose_check` | `dcc` | Check for available updates (compares local vs registry) |
| `docker_compose_upgrade` | `dcup2` | Upgrade services with mutable tags (latest, main, dev) |
| `docker_compose_scan` | `dcscan` | Scan directories for docker-compose files |

The smart upgrade system queries Docker Hub and GHCR registries to compare local image digests with remote ones. It only upgrades services with mutable tags (like `latest`, `main`, `dev`) and skips pinned versions (like `nginx:1.25.3`). Use `--force` to override this behavior.

```bash
# Check for updates in current directory
docker_compose_check

# Preview what would be upgraded (dry-run)
docker_compose_upgrade --dry-run

# Scan for all compose files in a directory tree
docker_compose_scan /path/to/projects --depth 3
```

#### AI Chat Integration

Gash includes an AI module for interacting with Claude and Gemini directly from your terminal. Get instant help with bash commands, troubleshoot errors, pipe configuration files for analysis, and generate scripts.

| Function | Alias | Description |
|----------|-------|-------------|
| `ai_query [provider] "query"` | - | Non-interactive AI query |
| `ai_ask [provider]` | `ask` | Interactive AI chat session |
| `ai_sysinfo [provider] [--raw]` | `sysinfo_ai` | AI-powered system analysis with interactive drill-downs |
| `sysinfo [section] [--llm]` | `si` | System enumeration (10 sections, verbose or LLM-compact output) |
| `gash_ai_list` | - | List configured AI providers |

**Response Types:**

The AI module automatically detects the type of request and formats output accordingly:

| Type | Trigger | Output Format |
|------|---------|---------------|
| `command` | "how to...", "how do I..." | `Command:` + `Explanation:` |
| `explanation` | "what is...", "explain..." | `Explanation:` only |
| `code` | "write a script...", "write code..." | Description + code block |
| `troubleshoot` | Pipe input detected | `Issue:` + `Suggestion:` |
| `fallback` | Everything else (greetings, etc.) | Direct text, no labels |

**Examples:**

```bash
# Get a command suggestion
ai_query "how to find files larger than 100MB?"
# Command: `find . -type f -size +100M`
# Explanation: Finds all files larger than 100MB in current directory.

# Get an explanation
ai_query "what is systemd?"
# Explanation: systemd is a system and service manager for Linux...

# Generate code
ai_query "write a script to backup my home directory"
# Creates a compressed backup of your home directory.
# # bash
# #!/bin/bash
# tar -czf ~/backup-$(date +%Y%m%d).tar.gz ~/

# Troubleshoot errors (pipe input)
tail -50 /var/log/apache2/error.log | ai_query "what's wrong?"
# Issue: PHP Fatal error - undefined function mysql_connect()
# Suggestion: mysql_connect() was removed in PHP 7. Use mysqli_connect() or PDO.

# Interactive mode
ask
# claude: how to list files by size?
# Command: `ls -lhS`
# Explanation: Lists files sorted by size in descending order.

# Use specific provider
ai_query gemini "explain kubernetes"
```

**Pipe Support for `ai_ask` and `ai_query`:**

Both `ai_ask` (alias `ask`) and `ai_query` accept piped input via stdin. When input is piped, the AI automatically enters **troubleshoot mode**: it receives the piped content as context and uses your question to diagnose issues, review configurations, or explain what's happening. Piped content is auto-truncated to 4KB to avoid token waste.

This is especially powerful for sending configuration files, log snippets, or command output directly to the AI for analysis:

```bash
# Review an Apache vhost configuration
cat /etc/apache2/sites-enabled/mysite.conf | ai_query "is this vhost correct? any security issues?"

# Analyze a PHP-FPM pool configuration
cat /etc/php/8.2/fpm/pool.d/www.conf | ai_query "optimize this pool for a server with 8GB RAM"

# Debug a failing systemd service
systemctl status my-app.service | ai_query "why is this service failing?"

# Review a Docker Compose file
cat docker-compose.yml | ai_query "any best practice violations?"

# Analyze Nginx configuration
cat /etc/nginx/sites-enabled/default | ai_query "add rate limiting and security headers"

# Troubleshoot MySQL slow queries
cat /var/log/mysql/slow-query.log | ai_query "what queries need optimization?"

# Review SSH hardening
cat /etc/ssh/sshd_config | ai_query "is this SSH config secure for a production server?"

# Analyze a crontab
crontab -l | ai_query "are there any overlapping or problematic schedules?"

# Inspect firewall rules
sudo iptables -L -n | ai_query "is this firewall properly configured?"

# Review Postfix configuration
cat /etc/postfix/main.cf | ai_query "is this mail relay configured correctly for outbound-only?"

# Pipe multiple files for comparison
diff /etc/php/8.1/fpm/pool.d/www.conf /etc/php/8.2/fpm/pool.d/www.conf | ai_query "what changed between PHP 8.1 and 8.2 pool configs?"

# Analyze a Bash script
cat my-backup-script.sh | ai_query "review this script for bugs and improvements"
```

**Features:**

* **Smart response types** - Automatically detects intent and formats output appropriately
* **Pipe support** - Pipe config files, error logs, or command output for troubleshooting and review
* **Auto-truncation** - Piped content is truncated to 4KB to avoid token waste
* **Rich context** - Includes cwd, shell type, distro, package manager, git branch, and last exit code
* **Clear error messages** - Specific feedback for network issues, timeouts, and API errors

**Tip:** Run `gash_help ai_query` for more examples, including multi-source piping patterns.

**Configuration:** See [AI Providers Configuration](#ai-providers-configuration) in the `~/.gash_env` section.

#### AI System Intelligence (`ai_sysinfo`)

`ai_sysinfo` is an AI-powered system analysis tool for headless Debian/Ubuntu servers. It collects comprehensive system data (with optional `sudo` for full visibility), sends it to Claude or Gemini for structured analysis, and provides an interactive drill-down interface.

**What it does:**

1. **Collects** system data across 10 categories (identity, storage, services, auth, network, security, webstack, mail, infrastructure, system config)
2. **Analyzes** the data via AI, producing a structured report with severity-coded findings (critical/warning/info/ok)
3. **Drills down** interactively into 6 detail sections (Services, Security, Network, Storage, Performance, Maintenance) using deep collectors that read full configuration files
4. **Answers questions** with smart context collection: asks about a specific service, file, or port, and the tool automatically gathers targeted data before querying the AI
5. **Exports** the full analysis (including all drill-downs and Q&A) as a Markdown report

```bash
# Run AI-powered system analysis
ai_sysinfo

# Use a specific provider
ai_sysinfo claude
ai_sysinfo gemini

# Dump raw collected data (no API call, useful for debugging)
ai_sysinfo --raw
```

**Interactive drill-down menu:**

After the initial analysis, an interactive menu lets you explore in detail:

```text
Drill down for details:
  1) Services    2) Security    3) Network
  4) Storage     5) Performance 6) Maintenance
  ?) Ask a question              s) Save report  q) Quit
```

* **Options 1-6**: Runs deep collectors that read full config files, service status, runtime data. The AI then provides per-component analysis with config highlights and issues.
* **Option `?`** (Ask a question): Type any question about your system. The tool intelligently detects entities in your question and collects targeted data before sending to the AI:
  * Systemd units (e.g., "analyze viper-backup.service") - reads unit file, status, and journal
  * File paths (e.g., "what's in /etc/postfix/main.cf?") - reads the file contents
  * Port numbers (e.g., "what's on port 3306?") - reads socket listeners
  * Known services (e.g., "tell me about apache") - reads service-specific configs
  * Custom service names (e.g., "explain viper-backup") - detects and collects even without `.service` suffix
* **Option `s`** (Save): Exports the full session (initial analysis + all drill-downs + all Q&A) to a timestamped Markdown file (`gash-ai-sysinfo-YYYYMMDD-HHMMSS.md`)

**Knowledge base:**

The system prompt includes an embedded knowledge base covering 40+ service families: Apache, Nginx, PHP-FPM, MySQL/MariaDB, PostgreSQL, Redis, Postfix, Dovecot, BIND, fail2ban, CrowdSec, UFW, Docker, OpenVPN, WireGuard, Samba, ZFS, Sanoid, LVM, RAID, Let's Encrypt, Collectd, Monit, Webmin, Virtualmin, and many more. It knows about Debian/Ubuntu file layout conventions (`.d/` directories, `available/enabled` symlink patterns, `.dpkg-old`/`.dpkg-dist` markers).

**Standalone system enumeration (`sysinfo`):**

The underlying data collection is also available as a standalone command, independent from the AI analysis:

```bash
# Full system enumeration (colored verbose output)
sysinfo

# Specific section only
sysinfo services
sysinfo security
sysinfo webstack

# LLM-compact output (token-efficient, for piping to AI tools)
sysinfo --llm
sysinfo services --llm

# Available sections:
# identity storage services auth network security webstack mail infra system all
```

#### Gash Management

| Function | Description |
|----------|-------------|
| `gash` | Interactive reference card with sections (`gash all` for complete list) |
| `gash_help` | Rich help system: `gash_help func`, `--search`, `--list`, `--short` |
| `gash_doctor` / `gdoctor` | Run health checks on Gash installation (core files, modules, config, tools) |
| `gash_upgrade` | Upgrades Gash to the latest version |
| `gash_uninstall` | Uninstalls Gash and cleans up configurations |
| `gash_unload` | Unloads Gash from the current shell session (best-effort restore) |
| `gash_inspiring_quote` | Displays an inspiring quote |
| `gash_env_init` | Creates `~/.gash_env` from template |
| `gash_db_list` | Lists all configured database connections |
| `gash_db_test` | Tests a database connection |
| `gash_ai_list` | Lists configured AI providers (claude, gemini) |
| `gash_ssh_auto_unlock` | Auto-unlock SSH keys configured in `~/.gash_env` |
| `sysinfo` / `si` | System enumeration for Debian/Ubuntu servers (10 sections, `--llm` for compact output) |

#### LLM Utilities (for AI Agents)

Gash exposes a first-class API for LLM coding agents — **Claude Code**,
**OpenAI Codex**, **Kimi**, **Gemini**, Cursor, Windsurf, Aider, local models.
All functions are designed to **minimize token usage** (machine-readable
output) and **eliminate integration friction** (no ANSI pollution, no history
pollution, no profile mutation).

> 📘 **Full agent integration guide: [AGENTS.md](AGENTS.md)** — function
> catalog, JSON envelope schemas, error contract, version compatibility, and
> copy-paste drop-ins for `CLAUDE.md`, `AGENTS.md`, and generic system prompts.

**Key features:**

* **Single-line auto-config**: `GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; <fn>'`
* **Stable contract**: `llm_*` names and JSON envelope shapes are considered public API; changes require a minor bump.
* **Zero ANSI pollution**: Auto-off on pipe / `NO_COLOR` / `GASH_NO_COLOR` / `GASH_HEADLESS`.
* **No short aliases** — Only long names; LLMs don't need quick typing.
* **No bash history**: Every call path wraps through `__gash_no_history`.
* **Security hardened**: Validated commands, sanitized paths, read-only DB, rejected secret files.
* **Machine-parseable output**: JSON envelopes with `{data, count, query_time|scan_time, errors, ...}`.

##### Filesystem & tree (v1.5+)

| Function | Description | Output |
|----------|-------------|--------|
| `files_largest --json [PATH]` | Largest files by size | Size-list envelope |
| `dirs_largest --json [PATH]` | Largest dirs by cumulative size, `--depth N` to drill | Size-list envelope |
| `dirs_find_large --json [PATH]` | Dirs over `--size SIZE` threshold, single-walk O(N) | Size-list envelope |
| `dirs_list_empty --json [PATH]` | Empty directories with `--count`, `--null`, `--ignore-dotfiles` | `{data:[], count, path, errors}` |
| `tree_stats --json [PATH]` | One-shot audit: totals, top extensions, depth, empty dirs | Nested `data` object |
| `llm_tree --depth N --stats` | Nested directory tree with per-node size + children_count | Nested JSON tree |

##### Code & project exploration

| Function | Description | Output |
|----------|-------------|--------|
| `llm_exec "<cmd>"` | Execute command safely (validated, no history) | Command stdout |
| `llm_find <pattern> [PATH] [--contains REGEX]` | Find files by name or content | Newline-separated paths |
| `llm_grep <pattern> [PATH] [--ext a,b,c]` | Search with structured output | `file:line:content` |
| `llm_config <FILE>` | Read config files (blocks secrets) | Raw / YAML→JSON via `yq` |
| `llm_project [PATH]` | Detect project type (node/php/python/go/rust/...) | JSON |
| `llm_deps [PATH] [--dev]` | List project dependencies from package files | JSON |
| `llm_docker_check [PATH]` | Docker Compose image-update check | JSON |

##### Database (read-only, enveloped)

All return `{"data": ..., "rows": N, "query_time": "Xms"}`. Drivers: `mysql`, `mariadb`, `pgsql`, SQLite via `-f`.

| Function | Description |
|----------|-------------|
| `llm_db_query "<SQL>" [-c CONN \| -f FILE]` | Read-only SELECT with auto-EXPLAIN on slow queries (≥100ms) |
| `llm_db_tables [-c CONN \| -f FILE]` | List tables |
| `llm_db_schema <TABLE> [-c CONN \| -f FILE]` | Column info |
| `llm_db_sample <TABLE> [--limit N] [-c CONN \| -f FILE]` | Sample rows |
| `llm_db_explain "<SQL>" [--analyze] [-c CONN \| -f FILE]` | Execution plan |

##### Git / system

| Function | Description | Output |
|----------|-------------|--------|
| `llm_git_status [PATH]` | Branch, ahead/behind, staged/modified/untracked | JSON |
| `llm_git_diff [--staged] [PATH]` | Diff with stats | Text (stat format) |
| `llm_git_log [--limit N] [PATH]` | Recent commit log | JSON |
| `llm_ports [--listen]` | Ports in use (via `ss` or `netstat`) | JSON |
| `llm_procs [--name X \| --port N]` | Processes by name/port | JSON |
| `llm_env [--filter PATTERN]` | Filtered env vars (secrets redacted) | JSON |

##### Security protections (always enforced)

* **Commands blocked**: `rm -rf /`, `rm -rf ~`, `dd if=`, `mkfs.*`, fork bombs, `curl|sh`, `wget|bash`, `chmod 777 /`, shutdown/reboot, `/etc/shadow`/`/etc/passwd` reads.
* **Path traversal** (`..`) rejected; **protected prefixes** (`/proc`, `/sys`, `/dev`, `/boot`, `/root`, `/etc/shadow`, `/etc/sudoers`) blocked.
* **Scanning `/`** requires explicit `--allow-root`.
* **Secret files** (`.env*`, `*.pem`, `*.key`, `*_rsa`, `id_*`, `credentials*`, `secrets*`, `.gash_env`) cannot be accessed.
* **SQL**: write keywords (`INSERT/UPDATE/DELETE/DROP/CREATE/ALTER/TRUNCATE/GRANT/REVOKE`) blocked at statement start; multi-statement (`;`) blocked; table-name allowlist `[A-Za-z0-9_]+`.

##### Error contract (stderr JSON)

```json
{"error":"<type>","action":"STOP|RETRY|CONTINUE|FATAL","recoverable":true,"details":"...","hint":"..."}
```

Branch on `.action`: `STOP` → ask user; `RETRY` → fix input, retry once; `CONTINUE` → proceed; `FATAL` → surface `hint`, stop. See [AGENTS.md §5](AGENTS.md) for exhaustive semantics.

##### Example usage

```bash
# Always prefix with GASH_HEADLESS=1 for agents
export GASH_HEADLESS=1
source ~/.gash/gash.sh

# Project structure (ignores noise dirs: node_modules, vendor, .git, __pycache__, .cache)
llm_tree --depth 3 --stats

# Find all PHP files containing "Controller"
llm_grep "Controller" --ext php

# Database: safe read-only, with auto-EXPLAIN on slow queries
llm_db_query "SELECT id, name FROM users WHERE active = 1" -c default
# {"data":[{"id":1,"name":"alice"},...],"rows":1,"query_time":"3ms"}

# SQLite
llm_db_query "SELECT * FROM users" -f ./data/app.db

# Compact git status (branch + ahead/behind + change lists)
llm_git_status
# {"branch":"main","ahead":0,"behind":0,"staged":[],"modified":["README.md"],"untracked":[]}

# Filesystem audit — largest files under /var/log, >50MB, JSON envelope
files_largest --json --min-size 50M /var/log | jq '.data[:5]'

# One-shot stats on the current codebase
tree_stats --json | jq '.data | {files_total, size_total_human, top_by_size}'
```

### Aliases

* **Directory Navigation**:
  * `..`, `...`, `....`, `.....`: Quickly move up multiple directory levels.
  * `cd..`, `.4`, `.5`: Alternate ways to move up multiple directory levels.
* **File Operations**:
  * `cp`, `mv`, `rm`: Aliased with safer interactive options (`-iv`, `-I`) to prevent accidental overwrites or deletions.
  * `mkdir`: Aliased with `-pv` to create parent directories and show verbose output.
  * `bc`: Launches `bc` with the `-l` option for floating point calculations.
* **Git Shortcuts** (run `gl --help` for full list):
  * **Log**: `gl` (graph), `gla` (all branches), `glo` (oneline), `glg` (first-parent), `gls` (with stats), `glf` (file history)
  * **Status**: `gst` (full), `gs` (short with branch)
  * **Add**: `ga`, `gaa` (all), `gap` (interactive patch)
  * **Commit**: `gc`, `gcm` (with message), `gca` (amend), `gcan` (amend no-edit)
  * **Push/Pull**: `gp`, `gpf` (force-with-lease), `gpl`, `gplr` (rebase)
  * **Branch**: `gb`, `gba` (all), `gcb` (create+switch), `gbd`/`gbD` (delete)
  * **Checkout/Switch**: `gco`, `gsw`, `gswc` (create)
  * **Diff**: `gd`, `gds` (staged), `gdw` (word-diff)
  * **Stash**: `gsh`, `gshp` (pop), `gshl` (list), `gsha` (apply)
  * **Remote**: `gf` (fetch), `gfa` (fetch all+prune), `gr` (remote -v)
  * **Reset**: `grh`, `grh1` (undo last commit), `grhh` (hard)
  * **Rebase**: `grb`, `grbc` (continue), `grba` (abort)
* **Network Utilities**:
  * `traceroute`, `tracepath`: Replaced with `mtr` (if installed) for interactive network diagnostics.
* **System Monitoring**:
  * `df` → `pydf`: Replaced with color-coded disk usage (if installed).
  * `top` → `htop`: Replaced with interactive process viewer (if installed).
* **Docker**:
  * `dcls`, `dclsr`: Lists all or running Docker containers.
  * `dils`: Lists all Docker images.
  * `dcrm`: Removes a Docker container. `dirm`: Prunes all Docker images.
  * `dstop`, `dstart`: Stops or starts a Docker container.
  * `dexec`, `drm`, `drmi`: Executes a command, removes a container, or an image.
  * `dlogs`, `dinspect`, `dnetls`: Shows logs, inspects an object, or lists networks.
* **Miscellaneous**:
  * `ll`, `la`, `lash`: Enhanced directory listings with color-coded output.
  * `ports`: Lists open network ports.
  * `all_colors`: Function that prints all available terminal colors with ANSI escape codes.
* **Cross-Platform Commands** (WSL):
  * `wsl_explorer` (`wex`), `wsl_taskmanager` (`wtm`): For Windows WSL users, opens Windows Explorer and Task Manager.
  * `wsl_restart` (`wr`), `wsl_shutdown` (`wsd`): Restarts or shuts down WSL. Saves bash history first.

### Swap-ins (Command Replacements)

* **Command Replacements**:
  * **less** → `most`: Provides a more feature-rich pager with better handling for non-text input files.
  * **tail** → `multitail`: Displays multiple log files in real-time.
  * **df** → `pydf`: Shows disk usage with color-coded output.
  * **top** → `htop`: An interactive system-monitoring tool with a more user-friendly interface.
  * **traceroute**/**tracepath** → `mtr`: A network diagnostic tool combining the functionality of traceroute and ping.
  * **diff** → `colordiff`: Colorizes the output of `diff` for easier readability.
* **PHP & Composer Versions**:
  * `php85`, `composer85`, `php84`, `composer84`, `php83`, `composer83`, `php82`, `composer82`, etc.: Aliases for specific versions of PHP (5.6-8.5) and Composer, allowing easy switching between different environments. Uses memory limits and `allow_url_fopen` enabled for Composer.
* **Git Enhancements**:
  * **git log** → `gl`: Professional log with graph, colors, and variants (`gl --help` for all options).
  * **git status** → `gst`/`gs`: Full or compact status view.
  * See [Git Shortcuts](#aliases) for the complete list of 40+ git aliases.

## License

Gash is open-source and distributed under the **Apache License 2.0**.

## Contributing

We welcome contributions! Fork the repo, open issues, or submit pull requests to help improve Gash.
