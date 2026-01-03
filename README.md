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

The automated installer will add a new section to your `.bash_profile` with:

```sh
...
# Load the Gash Environment
if [ -f ~/.gashrc ]; then
    source ~/.gashrc
fi
...
```

If no `.bash_profile` is found, the installer will create one for you.

## Features Breakdown

### Custom Command Prompt

Your new prompt shows:

* **Username** (colored by user type)
* **Current directory** (shortened to `~` for home)
* **Git branch & status** if you're inside a Git repo (shows unstaged changes, ahead/behind status)
* **Background jobs** and **last command exit code** for easy debugging.

```sh
# Example:
[maurizio@server]:~/projects (main*)[j2] $  # Git branch, jobs, exit code
```

### Built-in Aliases

Gash improves everyday commands:

* **`..`**: Go up one directory, **`...`** for two, etc.
* **`ll`**, **`la`**, **`lash`**: Enhanced directory listings.
* **`g`**: Shortcut for `git`, with **`ga`**, **`gst`**, **`gl`** for common actions.
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

Gash ships with a dedicated completion script (`bash_completion`). Once sourced (see `~/.gashrc`), it:

* Suggests all public Gash functions (excluding internal helpers)
* For `git_add_tag` / `git_delete_tag`, suggests Git tags when you are inside a Git repository

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
  # Loads Gash (functions, aliases, prompt)
  source "$GASH_BASH"
  # Loads bash completion for Gash functions
  source "$GASH_COMPLETION"
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
| `git_apply_patch` | `gap` | Creates and applies patches from a feature branch to main |

#### Docker Operations

| Long Name | Short | Description |
|-----------|-------|-------------|
| `docker_stop_all` | `dsa` | Stops all running Docker containers |
| `docker_start_all` | `daa` | Starts all stopped Docker containers |
| `docker_prune_all` | `dpa` | Removes all Docker containers, images, volumes, and networks |

#### Gash Management

| Function | Description |
|----------|-------------|
| `gash_help` | Displays a list of available Gash commands |
| `gash_upgrade` | Upgrades Gash to the latest version |
| `gash_uninstall` | Uninstalls Gash and cleans up configurations |
| `gash_unload` | Unloads Gash from the current shell session (best-effort restore) |
| `gash_inspiring_quote` | Displays an inspiring quote |

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
| `llm_structure` | Project structure (ignores node_modules, vendor, .git) | Tree-like text |
| `llm_recent` | Recently modified files | Path + timestamp |
| `llm_big` | Largest files in directory | Size + path |
| `llm_def` | Find function/class definitions | file:line:signature |
| `llm_refs` | Find references/usages | file:line:context |
| `llm_imports` | Analyze imports/require statements | JSON dependencies |
| `llm_db_query` | Read-only database queries (`-c CONNECTION`) | JSON array |
| `llm_db_tables` | List database tables (`-c CONNECTION`) | JSON array |
| `llm_db_schema` | Show table schema (`-c CONNECTION`) | JSON structure |
| `llm_db_sample` | Sample rows from table (`-c CONNECTION`) | JSON array |
| `llm_project` | Detect project type and info | JSON |
| `llm_deps` | List project dependencies | JSON |
| `llm_config` | Read config files (no .env for security) | JSON |
| `llm_routes` | Extract routes (Laravel/Express) | JSON |
| `llm_git_status` | Compact git status | JSON |
| `llm_git_diff` | Diff with stats | JSON or unified |
| `llm_git_log` | Recent commit log | JSON |
| `llm_git_blame` | Blame on line range | JSON |
| `llm_ports` | Ports in use | JSON |
| `llm_procs` | Processes by name/port | JSON |
| `llm_env` | Filtered env vars (no secrets) | JSON |

**Security Protections:**

* Commands like `rm -rf /`, `dd`, `mkfs`, fork bombs are blocked
* Path traversal (`..`) and dangerous paths (`/root`, `/boot`) are rejected
* Secret files (`.env`, `*.pem`, `*_rsa`) cannot be accessed
* SQL injection patterns (DROP, DELETE, TRUNCATE) are blocked
* All database operations are read-only

**Example Usage:**

```bash
# Get project structure (ignores noise directories)
llm_structure

# Find all PHP files containing "Controller"
llm_grep "Controller" --ext php

# Query database safely (uses 'default' connection from ~/.gash_env)
llm_db_query "SELECT id, name FROM users WHERE active = 1"

# Query a specific connection
llm_db_query "SELECT * FROM orders LIMIT 10" -c legacy
llm_db_tables -c postgres

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
* **Git Shortcuts**:
  * `g`, `ga`, `gst`, `gco`, `gb`, `gd`, `gl`, `gcm`, `gp`: Common Git commands, shortened for convenience.
  * `gl`, `glog`: A more visual and color-enhanced log of commits.
  * `gst`, `gstatus`: Colorized and compact view of the repository's status.
* **Network Utilities**:
  * `ping`, `traceroute`, `tracepath`: Aliased with `-c 5` for a limited number of packets.
  * `mtr`: Launches `mtr` with the `-c 5` option for a limited number of packets.
* **System Monitoring**:
  * `df`, `du`, `free`, `ps`, `top`: Aliased with human-readable output and color-coded columns.
  * `htop`: Launches `htop` with color-coded output for better process monitoring.
  * `pydf`: Displays disk usage with color-coded output.
* **Docker**:
  * `dcls`, `dclsr`: Lists all or running Docker containers.
  * `dils`: Lists all Docker images.
  * `dcrm`, `dirm`: Removes all Docker containers or images.
  * `dstop`, `dstart`: Stops or starts a Docker container.
  * `dexec`, `drm`, `drmi`: Executes a command, removes a container, or an image.
  * `dlogs`, `dinspect`, `dnetls`: Shows logs, inspects an object, or lists networks.
* **Miscellaneous**:
  * `ll`, `la`, `lash`: Enhanced directory listings with color-coded output.
  * `ports`: Lists open network ports.
  * `all_colors`: Prints all available terminal colors with ANSI escape codes.
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
  * `php83`, `composer83`, `php82`, `composer82`, etc.: Aliases for specific versions of PHP and Composer, allowing easy switching between different environments. Uses memory limits and `allow_url_fopen` enabled for Composer.
* **Git Enhancements**:
  * **git log** → `gl`: A more visual and color-enhanced log of commits.
  * **git status** → `gst`: Colorized and compact view of the repository's status.
  * **git add** → `ga`: Quickly adds files to staging.
  * **git commit** → `gc`: Shortcut for committing changes.

## License

Gash is open-source and distributed under the **Apache License 2.0**.

## Contributing

We welcome contributions! Fork the repo, open issues, or submit pull requests to help improve Gash.
