# Gash - Gash, Another SHell

> Oh Gash, was it _really_ necessary?

**Gash** is a no-fuss, colorful, and feature-rich replacement for your standard Bash configuration files. It packs everything you need to make your terminal experience faster, prettier, and more productive—all while keeping it simple and minimalistic.

## Why Gash?

* **Faster workflows**: Jump between directories, manage Git repos, and stop services in one-liners.
* **Colorful output**: Command-line information that stands out and helps you focus.
* **Smarter shell**: Aliases, functions, and an informative prompt that simplifies tasks.
* **Lightweight**: No bloat—just the tools you need.
* **Memorable commands**: Each function has a **long descriptive name** AND a **short alias** - use what suits you.

## Features At A Glance

* **Intelligent Prompt**: See everything you need (username, Git branch, jobs, etc.) at a glance.
* **Productivity Aliases**: Shortcuts for file operations, Git commands, and service management.
* **Convenient Functions**: One-liners to extract archives, list the largest files, or kill processes by port.
* **Dual Naming**: Every function has a descriptive LONG name and a memorable SHORT alias.
* **Bash Completion**: Tab-complete Gash functions (and Git tags for tag helpers).
* **Unload / Restore Session**: Turn off Gash in the current shell and restore the previous state.
* **Colorful Output**: Enhanced color schemes for better visibility (with `LS_COLORS`, Git status, etc.).
* **Cross-platform**: Works seamlessly on Linux, macOS, and Windows (WSL).

Check out the [full features list](#full-features-list) for a detailed breakdown.

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

### Bash completion

Gash ships with a dedicated completion script (`bash_completion`). Once sourced (see `~/.gashrc`), it provides context-aware tab completion for all public Gash functions:

* **Git tag functions** (`git_add_tag`, `git_delete_tag`, `git_list_tags`): suggests Git tags
* **Directory functions** (`files_largest`, `dirs_largest`, `docker_compose_check`, etc.): suggests directories
* **File functions** (`archive_extract`, `file_backup`, `git_dump_revisions`): suggests files
* **`gash`**: suggests reference card sections (`git`, `files`, `system`, `docker`, etc.)
* **AI functions** (`ai_ask`, `ai_query`): suggests providers (`claude`, `gemini`)
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
| `files_largest` | `flf` | Lists the top 100 largest files in a directory |
| `dirs_largest` | `dld` | Lists the top 100 largest directories |
| `dirs_find_large` | `dfl` | Finds directories larger than a specified size |
| `dirs_list_empty` | `dle` | Lists all empty directories in the specified path |
| `archive_extract` | `axe` | Extracts archive files (`.tar.gz`, `.zip`, `.7z`, etc.) |
| `file_backup` | `fbk` | Creates a backup of a file with a timestamp suffix |

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

Gash includes an AI module for interacting with Claude and Gemini directly from your terminal. Get instant help with bash commands, troubleshoot errors, and generate scripts.

| Function | Alias | Description |
|----------|-------|-------------|
| `ai_query [provider] "query"` | - | Non-interactive AI query |
| `ai_ask [provider]` | `ask` | Interactive AI chat session |
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

**Features:**

* **Smart response types** - Automatically detects intent and formats output appropriately
* **Pipe support** - Pipe error logs or command output for troubleshooting
* **Auto-truncation** - Piped content is truncated to 4KB to avoid token waste
* **Rich context** - Includes cwd, shell type, distro, package manager, git branch, and last exit code
* **Clear error messages** - Specific feedback for network issues, timeouts, and API errors

**Configuration:** See [AI Providers Configuration](#ai-providers-configuration) in the `~/.gash_env` section.

#### Gash Management

| Function | Description |
|----------|-------------|
| `gash` | Interactive reference card with sections (`gash all` for complete list) |
| `gash_help` | Displays a list of available Gash commands |
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

#### LLM Utilities (for AI Agents)

Gash includes a specialized module for LLM (Large Language Model) agents like Claude Code, GitHub Copilot, and similar AI coding assistants. These functions are designed to **minimize token usage** with machine-readable output (JSON/compact text).

**Key Features:**

* **No short aliases** - Only long names (`llm_tree`, not `lt`) since LLMs don't need quick typing
* **No bash history** - All commands are excluded from history to avoid pollution
* **Security hardened** - Dangerous commands are blocked, paths are sanitized, DB is read-only
* **JSON output** - Machine-parseable output minimizes token overhead

| Function | Description | Output |
|----------|-------------|--------|
| `llm_exec` | Execute command safely (validated, no history) | Command stdout |
| `llm_tree` | Compact directory tree | JSON structure |
| `llm_find` | Find files by pattern | Newline-separated paths |
| `llm_grep` | Search with structured output | file:line:content |
| `llm_db_query` | Read-only database queries (`-c CONNECTION` or `-f FILE`) | JSON array |
| `llm_db_tables` | List database tables (`-c CONNECTION` or `-f FILE`) | JSON array |
| `llm_db_schema` | Show table schema (`TABLE -c CONNECTION` or `-f FILE`) | JSON structure |
| `llm_db_sample` | Sample rows from table (`TABLE -c CONNECTION` or `-f FILE`) | JSON array |
| `llm_db_explain` | Query execution plan (`-c CONNECTION` or `-f FILE`) | JSON structure |
| `llm_project` | Detect project type and info | JSON |
| `llm_deps` | List project dependencies | JSON |
| `llm_config` | Read config files (no .env for security) | JSON |
| `llm_git_status` | Compact git status | JSON |
| `llm_git_diff` | Diff with stats | Text (stat format) |
| `llm_git_log` | Recent commit log | JSON |
| `llm_ports` | Ports in use | JSON |
| `llm_procs` | Processes by name/port | JSON |
| `llm_env` | Filtered env vars (no secrets) | JSON |
| `llm_docker_check` | Docker Compose update check | JSON |

**Security Protections:**

* Commands like `rm -rf /`, `dd`, `mkfs`, fork bombs are blocked
* Path traversal (`..`) and dangerous paths (`/root`, `/boot`) are rejected
* Secret files (`.env`, `*.pem`, `*_rsa`) cannot be accessed
* SQL injection patterns (DROP, DELETE, TRUNCATE) are blocked
* All database operations are read-only

**Example Usage:**

```bash
# Get project structure (ignores noise directories)
llm_tree

# Find all PHP files containing "Controller"
llm_grep "Controller" --ext php

# Query database safely (uses 'default' connection from ~/.gash_env)
llm_db_query "SELECT id, name FROM users WHERE active = 1"

# Query a specific connection
llm_db_query "SELECT * FROM orders LIMIT 10" -c legacy
llm_db_tables -c postgres

# SQLite databases (use -f instead of -c)
llm_db_query "SELECT * FROM users" -f ./data/app.db
llm_db_tables -f /path/to/database.sqlite

# Query execution plan
llm_db_explain "SELECT * FROM users WHERE id = 1" -c default

# Get compact git status
llm_git_status
# {"branch":"main","ahead":0,"behind":0,"staged":[],"modified":["README.md"],"untracked":[]}
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
* **Cross-Platform Commands**:
  * `explorer`, `taskmanager`: For Windows WSL users, opens Windows Explorer and Task Manager.
  * `wslrestart`, `wslshutdown`: Restarts or shuts down WSL.

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
