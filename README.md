# Gash - Gash, Another SHell

> Oh Gash, was it _really_ necessary?

**Gash** is a no-fuss, colorful, and feature-rich replacement for your standard Bash configuration files. It packs everything you need to make your terminal experience faster, prettier, and more productive—all while keeping it simple and minimalistic.

## Why Gash?

* **Faster workflows**: Jump between directories, manage Git repos, and stop services in one-liners.
* **Colorful output**: Command-line information that stands out and helps you focus.
* **Smarter shell**: Aliases, functions, and an informative prompt that simplifies tasks.
* **Lightweight**: No bloat—just the tools you need.

## Features At A Glance

* **Intelligent Prompt**: See everything you need (username, Git branch, jobs, etc.) at a glance.
* **Productivity Aliases**: Shortcuts for file operations, Git commands, and service management.
* **Convenient Functions**: One-liners to extract archives, list the largest files, or kill processes by port.
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
* **`hgrep`**: Search Bash history with color-coded output.

And [many more](#aliases)!

### Useful Functions

Save time with these built-in utilities:

* **`mkcd dir_name`**: Make a directory and `cd` into it.
* **`extract file.tar.gz [output_dir]`**: Extract almost any archive to a directory.
* **`largest_files [path]`**: Show the largest files in a directory.
* **`pskill process_name`**: Kill all processes by name.
* **`portkill port_number`**: Kill processes running on a specific port.
* **`stop_services`**: Stop well-known services like Apache, MySQL, Redis, Docker, etc.
* **`gash_unload`**: Restore your shell state and remove Gash (current session only).

And [many more](#helpers-functions)!

### Bash completion

Gash ships with a dedicated completion script (`bash_completion`). Once sourced (see `~/.gashrc`), it:

* Suggests all public Gash functions (excluding internal helpers)
* For `gadd_tag` / `gdel_tag`, suggests Git tags when you are inside a Git repository

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

#### `~/.gash_ssh_credentials` (optional: SSH key auto-unlock)

If this file exists, Gash may attempt to add SSH keys to your running `ssh-agent` at shell startup.
This feature is implemented by `gash_ssh_auto_unlock` and requires:

* `ssh-agent` running and reachable (`ssh-add -l` must work)
* `expect` installed (used to provide passphrases non-interactively)

By default, Gash will NOT start `ssh-agent` for you (to avoid spawning extra agents unexpectedly).
If you want Gash to auto-start an agent when this file exists, enable it explicitly:

```sh
export GASH_SSH_AUTO_START_AGENT=1
```

Format: one key per line as `PATH_TO_PRIVATE_KEY:PASS_PHRASE`.
Empty lines and lines starting with `#` are ignored.

```sh
# ~/.gash_ssh_credentials
# Format: /path/to/key:passphrase
~/.ssh/id_ed25519:correct horse battery staple
~/.ssh/work_id_rsa:my-work-passphrase
```

You can also change the location of this file:

```sh
# ~/.bash_local
export GASH_SSH_CREDENTIALS_FILE="$HOME/.config/gash/ssh_credentials"
```

Security note: this file contains sensitive data in plain text.
If you use it, strongly consider setting permissions to owner-read only (e.g. `chmod 600 ~/.gash_ssh_credentials`) and prefer agent/keychain-based flows when available.

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
  * `gadd_tag`, `gdel_tag`, `gtags`: Tag management commands.
  * `ga`, `gadd`: Quickly adds files to staging.
  * `gc`, `gcommit`: Shortcut for committing changes.
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
  * `docker_stop_all`, `dstopall`, `dstartall`: Stops or starts all Docker containers.
  * `dpruneall`, `docker_prune_all`: Removes all Docker containers, images, volumes, and networks.
* **Miscellaneous**:
  * `ll`, `la`, `lash`: Enhanced directory listings with color-coded output.
  * `hgrep`: Searches Bash history for a pattern and removes duplicates.
  * `myip`: Displays your public IP address.
  * `stop_services`: Stops well-known services like Apache, MySQL, Redis, Docker, etc.
* **Cross-Platform Commands**:
  * `explorer`, `taskmanager`: For Windows WSL users, opens Windows Explorer and Task Manager.
  * `wslrestart`, `wslshutdown`: Restarts or shuts down WSL.
* **System Commands**:
  * `cls`: Clears the terminal screen.
  * `quit`: Stops well-known services (`apache2`, `nginx`, `mysql`, etc.) with the `--force` flag.
  * `ports`: Lists open network ports.
  * `all_colors`: Prints all available terminal colors with ANSI escape codes.
* **Gash Helper Functions**:
  * `gash_help`: Displays a list of available Gash commands. (Also, help is shown if you type `help`, at the end of the prompt.)
  * `gash_unload`: Unloads Gash from the current shell session and restores the previous shell state (best-effort).
  * `gash_upgrade`: Upgrades Gash to the latest version.
  * `gash_inspiring_quote`: Displays an inspiring quote.
  * `gash_uninstall`: Uninstalls Gash and cleans up configurations.

### Helpers (Functions)

* **File and Directory Management**:
  * `mkcd`: Creates a directory and changes into it.
  * `extract`: Extracts archive files (`.tar.gz`, `.zip`, `.7z`, etc.) into the current or specified output directory.
  * `backup_file`: Creates a backup of a file with a timestamp suffix.
  * `list_empty_dirs`: Lists all empty directories in the specified path.
* **System Monitoring**:
  * `largest_files`: Lists the top 100 largest files in a directory.
  * `largest_dirs`: Lists the top 100 largest directories.
  * `find_large_dirs`: Finds directories larger than a specified size and lists their largest file's modification time.
  * `disk_usage_fs`: Displays disk usage for specific filesystem types, formatted for readability.
  * `all_colors`: Prints all available terminal colors with ANSI escape codes.
* **Process Management**:
  * `pskill`: Kills all processes matching a given name.
  * `portkill`: Kills processes running on a specified port.
  * `psgrep`: Searches for processes by name and displays details with color-coded output.
* **Git Utilities**:
  * `git_dump_revisions`: Dumps all revisions of a Git-tracked file into separate files.
  * `git_apply_feature_patch`: Creates and applies patches from a feature branch to the main branch.
* **History and Permissions**:
  * `hgrep`: Searches Bash history for a pattern and removes duplicates, only showing the last occurrence.
  * `please`: Runs the last command or a given command with `sudo`.

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
