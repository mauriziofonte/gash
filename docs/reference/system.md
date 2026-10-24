# system

Processes, ports, services, disks, history, WSL.

_Written from the help registry (`gash_help NAME`); `go test ./internal/registry` checks it against the help text._

| Function | Alias | Description |
|---|---|---|
| [`all_colors`](#all-colors) |  | Display all 256 terminal colors |
| [`disk_usage`](#disk-usage) | `du2` | Display disk usage for real filesystem types |
| [`hgrep`](#hgrep) |  | Smart history search with timestamps and deduplication |
| [`history_grep`](#history-grep) | `hg` | Search command history with colored output |
| [`ip_public`](#ip-public) | `myip` | Get your public IP address |
| [`mkdir_cd`](#mkdir-cd) | `mkcd` | Create a directory and cd into it |
| [`path_add`](#path-add) |  | Add directories to PATH once each |
| [`port_kill`](#port-kill) | `ptk` | Kill processes listening on a specific port |
| [`process_find`](#process-find) | `pf` | Search for a process by name |
| [`process_kill`](#process-kill) | `pk` | Kill all processes matching a name |
| [`services_stop`](#services-stop) | `svs` | Stop well-known services (Apache, MySQL, Redis, Docker, etc.) |
| [`sudo_last`](#sudo-last) | `plz` | Re-run last command with sudo |
| [`sysinfo`](#sysinfo) | `si` | Comprehensive system information enumeration |
| [`wsl_compact`](#wsl-compact) | `wslc` | Fix Windows interop on a WSL2 VM with fragmented memory |
| [`wsl_explorer`](#wsl-explorer) | `wex` | Open Windows Explorer from WSL |
| [`wsl_restart`](#wsl-restart) | `wr` | Restart WSL (saves history, runs wsl --shutdown) |
| [`wsl_shutdown`](#wsl-shutdown) | `wsd` | Shutdown WSL (saves history, runs wsl --shutdown) |
| [`wsl_taskmanager`](#wsl-taskmanager) | `wtm` | Open Windows Task Manager from WSL |

## all_colors

Display all 256 terminal colors

#### Usage

```text
all_colors
```

#### Examples

```bash
# Test your terminal's color support
all_colors
```

#### Notes

```text
Colors only on a color terminal: piped, or with NO_COLOR set, it prints
the color names alone.
```

## disk_usage

Display disk usage for real filesystem types

Alias: `du2`

#### Usage

```text
disk_usage [--no-color]
```

#### Examples

```bash
# Show disk usage for ext4, xfs, btrfs, zfs, etc.
du2
```

#### Notes

```text
Filters out virtual filesystems (tmpfs, devtmpfs, overlay, fuse).
Windows drives under WSL (/mnt/c, ...) are shown.
The bar turns yellow at 75% and red at 90%.
On macOS (no df -T) every /dev device is listed, without a type.
```

See also: `dirs_largest`, `dirs_find_large`

## hgrep

Smart history search with timestamps and deduplication

#### Usage

```text
hgrep PATTERN [OPTIONS]
```

#### Options

```text
-n, --limit N       Show only the N most recent matches (0: all)
-a, --all           Include hgrep/history_grep commands in results
-r, --reverse       Newest first (default: oldest first)
-j, --json          Output as JSON array
-H, --no-highlight  Disable highlighting / force plain output
-E, --regex         PATTERN is a Go regular expression (RE2 syntax),
                    case-insensitive; an invalid regex is an error
-c, --count         Only show the count of unique matching commands
-g, --group         Collapse similar commands under a shared chain prefix
    --pick          Fuzzy-select a matching command via fzf and print it
    --no-color      Disable colored output for this invocation
-h, --help          Show this help (anywhere before --)
--                  End of options: the next word is PATTERN
```

#### Examples

```bash
# Search for git push commands
hgrep "git push"

# Last 10 docker commands
hgrep docker -n 10

# Export history as JSON
hgrep deploy -j

# Count how many times you ran a command
hgrep "npm install" -c

# Regex: git push or pull at the start
hgrep -E "^git (push|pull)"

# Group "cd X && ..." variants under one prefix
hgrep cd -g

# Fuzzy-pick a past git command (needs fzf)
hgrep git --pick

# Newest first
hgrep kubectl -r

# A pattern that starts with "-"
hgrep -- --force
```

#### Notes

```text
Matching is case-insensitive; PATTERN is a fixed string unless -E.
Commands are deduplicated (the last run wins) and listed oldest first.
-n N keeps the N most recent matches, with or without -r.
```

#### Output format

```text
[2024-01-15 14:52:01] git push origin main
[2024-01-15 15:10:22] git commit -m "fix"

With --json:
[{"timestamp":"2024-01-15 14:52:01","epoch":1705330321,"count":1,"command":"git push origin main"},...]
```

See also: `history_grep`

## history_grep

Search command history with colored output

Alias: `hg`

#### Usage

```text
history_grep PATTERN...
```

#### Examples

```bash
# Find all git commands in history
hg git

# Find docker-compose commands
hg "docker compose"

# Find SSH connections
hg "ssh "
```

#### Notes

```text
Simple and fast. For advanced features (timestamps,
deduplication, JSON output), use hgrep instead.
The words are joined with spaces into one pattern (a GNU grep basic
regex, case-insensitive) matched against each command. Column 2 is a
command's first two words, column 3 the rest; duplicates and the
history_grep/hg lines themselves are left out.
```

See also: `hgrep`

## ip_public

Get your public IP address

Alias: `myip`

#### Usage

```text
ip_public [--no-color] [--raw]
```

#### Options

```text
--no-color  Plain output
--raw       Print the bare address, no label
```

#### Examples

```bash
# Show your public IP
myip

# Use in a script
MY_IP=$(myip --raw) && echo "Server IP: $MY_IP"
```

#### Notes

```text
The request gives up after 10 seconds. An answer that is not an IP
address (a captive portal or rate-limit page) is an error, rc 1.
```

## mkdir_cd

Create a directory and cd into it

Alias: `mkcd`

#### Usage

```text
mkdir_cd DIRECTORY
```

#### Examples

```bash
# Create and enter a new project directory
mkcd my-new-project

# Create nested directories and enter
mkcd ~/projects/client/frontend
```

## path_add

Add directories to PATH once each

#### Usage

```text
path_add [--append] DIR...
```

#### Examples

```bash
# Toolchains first, in this order, only those that exist
path_add ~/.local/bin ~/.cargo/bin ~/.config/composer/vendor/bin

# At the end of PATH instead
path_add --append /opt/tools/bin
```

#### Notes

```text
Directories that do not exist are skipped and one already in PATH stays
where it is, so running it again (or re-sourcing ~/.gashrc) never makes
PATH grow. GASH_PATH_ADD in ~/.gash_env does this for every new shell.
```

## port_kill

Kill processes listening on a specific port

Alias: `ptk`

#### Usage

```text
port_kill [-y] PORT
```

#### Examples

```bash
# Stop whatever listens on port 3000 (lists it and asks first)
ptk 3000

# No confirmation (scripts, agents)
ptk -y 3000
```

#### Notes

```text
--force is the same as -y. Requires lsof. Only listeners are targeted: a browser or ssh session
connected to that port elsewhere is left alone. SIGTERM first,
SIGKILL for processes still alive after about 3 seconds.
Returns 1 when a process could not be signalled (permission denied).
```

See also: `process_find`, `process_kill`

## process_find

Search for a process by name

Alias: `pf`

#### Usage

```text
process_find PROCESS_NAME
```

#### Examples

```bash
# Find nginx processes
pf nginx

# Find node processes
pf node

# Check if a service is running
pf mysql
```

#### Notes

```text
NAME is matched literally, ignoring case, against each command line
(not the user or other columns). Returns 1 when nothing matches.
```

See also: `process_kill`, `port_kill`

## process_kill

Kill all processes matching a name

Alias: `pk`

#### Usage

```text
process_kill [-y] PROCESS_NAME
```

#### Examples

```bash
# Kill all node processes (lists them and asks first)
pk node

# No confirmation (scripts, agents)
pk -y webpack-dev-server
```

#### Notes

```text
Matches the executable name exactly, ignoring case, lists the matches
with their command lines and asks. Without a terminal to ask
(GASH_HEADLESS=1, no /dev/tty) nothing is killed unless -y is given.
--force is the same as -y. SIGTERM first, SIGKILL for processes still
alive after about 3 seconds. Declining returns 1, and so does a
process that could not be signalled (permission denied).
NAME is a name, not a pattern: regex characters match themselves, and
a name longer than 15 bytes is matched against the full program name.
```

See also: `process_find`, `port_kill`

## services_stop

Stop well-known services (Apache, MySQL, Redis, Docker, etc.)

Alias: `svs`

#### Usage

```text
services_stop [-y] [--dry-run]
```

#### Examples

```bash
# List the running ones and ask before stopping them
svs

# Only list them
svs --dry-run

# Skip confirmation
svs -y
```

#### Services stopped

```text
apache2, nginx, mysql, mariadb, postgresql,
mongodb, redis, redis-server, memcached, docker
```

#### Notes

```text
Only stops services that are currently active, through systemd when it
runs, else through `service` (WSL without systemd, containers).
--force is the same as -y. Returns 1 when declined or when a stop
fails. Headless or without a terminal it runs sudo -n, and stops
before anything when sudo needs a password.
```

See also: `process_kill`

## sudo_last

Re-run last command with sudo

Alias: `plz`

#### Usage

```text
sudo_last [command]
```

#### Examples

```bash
# Oops, forgot sudo? Re-run with sudo
apt update        # -> Permission denied
plz               # -> sudo apt update

# Run a specific command with sudo
plz systemctl restart nginx
```

#### Notes

```text
Without COMMAND it shows the previous command and asks first;
declining (or no terminal to ask on) returns 1, and so does a shell
with no previous command in its history.
```

## sysinfo

Comprehensive system information enumeration

Alias: `si`

#### Usage

```text
sysinfo [section] [--llm] [--no-color]
```

#### Sections

```text
overview   Host, load, CPU, memory, swap, root disk and address at a glance
identity   OS, kernel, hostname, uptime, virtualization, container, WSL
storage    fstab, crypttab, LVM, mdadm, ZFS, smartd, sanoid, hdparm, NFS
services   Detected/running services, timers, listening ports, custom units
auth       Users, sudoers, sshd_config (PAM details via deep drill-down)
network    resolv.conf, hosts, interfaces, netplan, dhcpcd
security   fail2ban, crowdsec, ufw; firewalld, iptables, AppArmor presence
webstack   Apache, Nginx, PHP and FPM pools, MySQL/MariaDB, PostgreSQL
mail       Postfix, Dovecot, Exim4, SpamAssassin, OpenDKIM, BIND, Unbound
infra      FTP, Samba, OpenMediaVault, VPN, Docker, containers, monitoring
system     Cron, Let's Encrypt, SSL keys, kernel tuning, GRUB, logrotate
all        Overview, then every section (default)
```

#### Examples

```bash
# Full system enumeration (colored verbose output)
sysinfo

# Check just network configuration
sysinfo network

# Audit the web stack
sysinfo webstack

# LLM-compact output (token-efficient, pipe to AI)
sysinfo --llm
sysinfo services --llm

# Combine with AI for instant analysis
sysinfo webstack --llm | ai_query "audit this web stack"
```

#### Notes

```text
On a terminal a normal user is asked for sudo only by the sections
that read root-only files (identity, storage, services, auth,
security, system, all); overview, network, webstack, mail and infra
never ask.
One section per call: a second one is an error. -h/--help anywhere
shows this help.
```

See also: `ai_sysinfo`

## wsl_compact

Fix Windows interop on a WSL2 VM with fragmented memory

Alias: `wslc`

#### Usage

```text
wsl_compact [--status] [-y]
```

#### Examples

```bash
# code . or explorer.exe fail with "UtilAcceptVsock ... accept4 failed 110"
wslc

# Only show the failures and the free large blocks, change nothing
wslc --status
```

#### Notes

```text
Every Windows program started from WSL needs a contiguous 512 KiB block
of guest memory for its hv_sock channel. When memory is fragmented the
allocation fails (dmesg: "page allocation failure: order:7") and the
launch times out. wsl_compact drops the page cache and slab, compacts
memory (sudo), prints the free blocks before and after, and checks that
cmd.exe starts. If it still fails, run `wsl --shutdown` from Windows.
On a terminal it runs without asking; headless it needs -y (--yes).
Unattended, sudo must not need a password (sudo -n is checked first).
When dmesg is not readable the failure count is "unknown".
```

See also: `wsl_restart`, `wsl_shutdown`

## wsl_explorer

Open Windows Explorer from WSL

Alias: `wex`

#### Usage

```text
wsl_explorer [PATH]
```

#### Examples

```bash
# Open Explorer in current directory
wex

# Open Explorer in a specific path
wex /home/user/projects

# Open Explorer in Windows user folder
wex /mnt/c/Users
```

#### Notes

```text
Defaults to current directory if no path specified.
Only works inside WSL (Windows Subsystem for Linux).
```

See also: `wsl_taskmanager`

## wsl_restart

Restart WSL (saves history, runs wsl --shutdown)

Alias: `wr`

#### Usage

```text
wsl_restart [-y]
```

#### Examples

```bash
# Restart WSL (asks, saves bash history first)
wr

# Without the question
wr -y
```

#### Notes

```text
Runs `wsl --shutdown`: every distro stops, Docker Desktop's included,
with the servers running in them. WSL starts again with the next
terminal. Asks first; -y skips the question. Same as wsl_shutdown.
```

See also: `wsl_shutdown`

## wsl_shutdown

Shutdown WSL (saves history, runs wsl --shutdown)

Alias: `wsd`

#### Usage

```text
wsl_shutdown [-y]
```

#### Examples

```bash
# Shut down WSL (asks, saves bash history first)
wsd
```

#### Notes

```text
Runs `wsl --shutdown`: every distro stops, Docker Desktop's included.
Asks first; -y skips the question.
```

See also: `wsl_restart`

## wsl_taskmanager

Open Windows Task Manager from WSL

Alias: `wtm`

#### Usage

```text
wsl_taskmanager
```

#### Examples

```bash
# Open Windows Task Manager
wtm
```

#### Notes

```text
Launches Task Manager in the background.
Does not change your current working directory.
Only works inside WSL (Windows Subsystem for Linux).
```

See also: `wsl_explorer`, `process_find`

## Aliases

| Alias | Runs |
|---|---|
| `cp` | `cp -iv` |
| `mv` | `mv -iv` |
| `mkdir` | `mkdir -pv` |
| `rm` | `rm -Iv --one-file-system --preserve-root` |
| `chown` | `chown --preserve-root` |
| `chmod` | `chmod --preserve-root` |
| `chgrp` | `chgrp --preserve-root` |
| `bc` | `bc -l` |
| `ports` | `ss -tulanp` |
| `reboot` | `sudo /sbin/reboot` |
| `shutdown` | `sudo /sbin/shutdown` |
| `grep` | `grep --color=auto` |
| `quit` | `services_stop` |
| `less` | `most` |

