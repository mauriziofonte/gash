# Gash - Gash, Another SHell 🚀

> Oh Gash, was it _really_ necessary?

**Gash** is a no-fuss, colorful, and feature-rich replacement for your standard Bash configuration files. It packs everything you need to make your terminal experience faster, prettier, and more productive—all while keeping it simple and minimalistic.

## Why Gash?

* **🚀 Faster workflows**: Jump between directories, manage Git repos, and stop services in one-liners.
* **🎨 Colorful output**: Command-line information that stands out and helps you focus.
* **💡 Smarter shell**: Aliases, functions, and an informative prompt that simplifies tasks.
* **⚡ Lightweight**: No bloat—just the tools you need.

## Features At A Glance 👀

* **Intelligent Prompt**: See everything you need (username, Git branch, jobs, etc.) at a glance.
* **Productivity Aliases**: Shortcuts for file operations, Git commands, and service management.
* **Convenient Functions**: One-liners to extract archives, list the largest files, or kill processes by port.
* **Colorful Output**: Enhanced color schemes for better visibility (with `LS_COLORS`, Git status, etc.).
* **Cross-platform**: Works seamlessly on Linux, macOS, and Windows (WSL).

Check out the [full features list](#full-features-list) for a detailed breakdown.

## Quickstart 🌟

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

### 🖥️ Custom Command Prompt

Your new prompt shows:

* **Username** (colored by user type)
* **Current directory** (shortened to `~` for home)
* **Git branch & status** if you're inside a Git repo (shows unstaged changes, ahead/behind status)
* **Background jobs** and **last command exit code** for easy debugging.

```sh
# Example: 
[maurizio@server]:~/projects (main*)[j2] $  # Git branch, jobs, exit code
```

### ⚙️ Built-in Aliases

Gash improves everyday commands:

* **`..`**: Go up one directory, **`...`** for two, etc.
* **`ll`**, **`la`**, **`lash`**: Enhanced directory listings.
* **`g`**: Shortcut for `git`, with **`ga`**, **`gst`**, **`gl`** for common actions.
* **`hgrep`**: Search Bash history with color-coded output.

And [many more](#aliases)!

### 📂 Useful Functions

Save time with these built-in utilities:

* **`mkcd dir_name`**: Make a directory and `cd` into it.
* **`extract file.tar.gz [output_dir]`**: Extract almost any archive to a directory.
* **`largest_files [path]`**: Show the largest files in a directory.
* **`pskill process_name`**: Kill all processes by name.
* **`portkill port_number`**: Kill processes running on a specific port.
* **`stop_services`**: Stop well-known services like Apache, MySQL, Redis, Docker, etc.

And [many more](#helpers-functions)!

## Power Up with Additional Tools 💪

Gash works out of the box, but it shines when you install these optional tools:

| Original Command | Replacement | Description                              |
|------------------|-------------|------------------------------------------|
| `df`             | `pydf`      | Enhanced disk usage output               |
| `less`           | `most`      | Better paging for long files             |
| `top`            | `htop`      | Enhanced process viewer                  |
| `traceroute`     | `mtr`       | Interactive network diagnostics          |


### Install recommended tools:

#### Debian/Ubuntu:

```sh
sudo apt install most multitail pydf mtr htop colordiff
```

#### macOS (with Homebrew):

```sh
brew install most multitail pydf mtr htop colordiff
```

## Customization 🛠️

Gash is fully customizable. Want to add your own aliases or functions? Easy!

### Create a `~/.bash_local` file:

```sh
# ~/.bash_local example: 
alias cls='clear'  # Custom alias  
greet() {   echo "Hello, $USER!" }
```

Gash will load your custom settings automatically.

## Uninstalling Gash 🔧

Gash comes with an automated uninstaller.

To remove Gash and restore your original configuration:

```sh
uninstall_gash
```

Then, restart your terminal.

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
  * `glog`: Detailed Git log with a graph and decorations.
* **Docker**:
  * `dcls`, `dclsr`: Lists all or running Docker containers.
  * `dils`: Lists all Docker images.
  * `dcrm`: Prunes stopped containers.
  * `dirm`: Prunes Docker images.
  * `dstop`, `dstart`: Stops or starts Docker containers.
  * `dexec`: Executes commands inside Docker containers.
  * `drm`, `drmi`: Removes Docker containers or images.
  * `dlogs`: Follows logs of a Docker container.
  * `dinspect`: Inspects Docker containers or images.
  * `dnetls`, `dnetrm`: Lists or prunes Docker networks.
  * `dvolrm`: Prunes Docker volumes.
  * `dcup`, `dcdown`, `dclogs`, `dcb`, `dcrestart`, `dcps`, `dcpull`: Docker Compose commands for managing services.
* **System Commands**:
  * `cls`: Clears the terminal screen.
  * `quit`: Stops well-known services (`apache2`, `nginx`, `mysql`, etc.) with the `--force` flag.
  * `ports`: Lists open network ports.
  * `explorer`, `taskmanager`: For Windows WSL users, opens Windows Explorer and Task Manager.
  * `wslrestart`, `wslshutdown`: Restarts or shuts down WSL.

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

## License 📝

Gash is open-source and distributed under the **Apache License 2.0**.

## Contributing 🤝

We welcome contributions! Fork the repo, open issues, or submit pull requests to help improve Gash.
