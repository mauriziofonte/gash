<!-- Generated from internal/prereq (go test ./internal/prereq -update): do not edit. -->

# Requirements

Gash is one static binary plus the Bash that loads it. The binary does
its work in process (search, hashing, JSON, databases, HTTP), so most of
Gash needs nothing else. What it does need is listed here, from the same
catalog that `install.sh` and `gash_doctor` read.

- **Required** tools are needed to install Gash and to load it: Bash 4.4
  or newer, git, curl or wget, tar, gzip, sha256sum or shasum, grep and
  sed. `install.sh --check` reports them, and `install.sh` offers to
  install the missing ones (`--assume-yes` installs them without asking).
- **Feature** tools are needed only by the functions listed with them.
  Without one, those functions stop with an error that names the tool and
  the command that installs it on your system.
- **Optional** tools add something to a function that works without them.

`gash_doctor --prereqs` shows what this system has and lacks, with the
install command of each missing tool. `gash_doctor --install` installs
the missing required and feature tools after asking (`--all` adds the
optional ones, or name the tools: `gash_doctor --install rsync zstd`).
It never installs anything for an AI agent (`GASH_HEADLESS=1`): agents get
the `prerequisite_missing` envelope described in [AGENTS.md](../AGENTS.md)
and ask the user. `gash_doctor --json` gives the same report as JSON.

In the tables, a dash means the system has no package for the tool (see
its notes), "built in" means macOS ships it, and "(EPEL)" means RHEL,
Rocky, AlmaLinux and Oracle Linux take it from EPEL, which `gash_doctor
--install` enables first. Amazon Linux has no EPEL.

## Install and update

- **bash** (required) runs Gash; version 4.4 or newer. Needed by `gash run` (--safe), `gash update`, `llm_exec`, `llm_lint` (.sh files) and `llm_wait` (--cmd-ok). On macOS, make the Homebrew bash your login shell: chsh -s "$(brew --prefix)/bin/bash".
- **curl** (required) downloads the Gash binary (wget works too). Any of: `curl`, `wget`.
- **tar** (required) unpacks the Gash release archive. Needed by `archive_extract` (tar archives).
- **gzip** (required) decompresses the Gash release archive and .gz files. Needed by `archive_extract` (.gz).
- **sha256sum** (required) verifies the downloaded binary against its pinned hash. Any of: `sha256sum`, `shasum`.
- **git** (required) installs and updates Gash, and runs the git functions. Needed by `agent_git_commit`, `gash update`, `gash_upgrade`, `git_add_tag`, `git_apply_patch`, `git_delete_tag`, `git_dump_revisions`, `git_list_tags`, `git_repair`, `git_sync`, `gl`, `gla`, `glf`, `glg`, `glo`, `gls`, `llm_git_diff`, `llm_git_log`, `llm_git_overview`, `llm_git_show` and `llm_git_status`. On macOS git comes with the Command Line Tools: xcode-select --install.
- **grep** (required) is used by the installer.
- **sed** (required) is used by the installer and gash_uninstall.

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| bash | `bash` | `bash` | `bash` | `bash` | `bash` | `bash` |
| curl | `curl` | `curl` | `curl` | `curl` | `curl` | built in |
| tar | `tar` | `tar` | `tar` | `tar` | `tar` | built in |
| gzip | `gzip` | `gzip` | `gzip` | `gzip` | `gzip` | built in |
| sha256sum | `coreutils` | `coreutils` | `coreutils` | `coreutils` | `coreutils` | built in |
| git | `git` | `git` | `git` | `git` | `git` | `git` |
| grep | `grep` | `grep` | `grep` | `grep` | `grep` | built in |
| sed | `sed` | `sed` | `sed` | `sed` | `sed` | built in |

## Remote hosts and SSH

- **ssh** (feature) connects to remote hosts. Needed by every `--host` call, `agent_put`, `gash_db_test` (a +ssh URL), `llm_db_databases` (a +ssh URL), `llm_db_explain` (a +ssh URL), `llm_db_query` (a +ssh URL), `llm_db_sample` (a +ssh URL), `llm_db_schema` (a +ssh URL), `llm_db_tables` (a +ssh URL), `llm_remote_run`, `llm_same` (a host side), `remote_exec`, `remote_sync_pull` and `remote_sync_push`.
- **scp** (feature) copies files to and from remote hosts. Needed by `remote_pull` and `remote_push`.
- **rsync** (feature) syncs directories with remote hosts. Needed by `remote_sync_pull` and `remote_sync_push`.
- **ssh-add** (feature) loads SSH keys into the agent. Needed by `gash_ssh_auto_unlock`.
- **ssh-agent** (feature) holds the unlocked SSH keys. Needed by `gash_ssh_auto_unlock`.

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| ssh | `openssh-client` | `openssh-clients` | `openssh` | `openssh-clients` | `openssh-client` | built in |
| scp | `openssh-client` | `openssh-clients` | `openssh` | `openssh-clients` | `openssh-client` | built in |
| rsync | `rsync` | `rsync` | `rsync` | `rsync` | `rsync` | built in |
| ssh-add | `openssh-client` | `openssh-clients` | `openssh` | `openssh-clients` | `openssh-client` | built in |
| ssh-agent | `openssh-client` | `openssh-clients` | `openssh` | `openssh-clients` | `openssh-client` | built in |

## Processes and ports

- **ps** (feature) lists the running processes. Needed by `agent_kill` (on macOS, or without /proc), `llm_procs` (on macOS, or without /proc), `llm_wait` (--gone, on macOS) and `process_find`.
- **pgrep** (feature) finds processes by name. Needed by `llm_procs` (--name) and `process_kill`.
- **lsof** (feature) maps ports to the processes that hold them. Needed by `llm_procs` (--port) and `port_kill`.
- **ss** (feature) lists the listening ports; netstat works too. Any of: `ss`, `netstat`. Needed by `llm_ports`.

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| ps | `procps` | `procps-ng` | `procps-ng` | `procps` | `procps` | built in |
| pgrep | `procps` | `procps-ng` | `procps-ng` | `procps` | `procps` | built in |
| lsof | `lsof` | `lsof` | `lsof` | `lsof` | `lsof` | built in |
| ss | `iproute2` | `iproute` | `iproute2` | `iproute2` | `iproute2` | built in |

## Docker

- **docker** (feature) runs and inspects containers. Needed by `agent_docker_exec`, `docker_compose_check`, `docker_compose_upgrade`, `docker_prune_all`, `docker_start_all`, `docker_stop_all`, `llm_docker_check`, `llm_docker_exec`, `llm_docker_logs` and `llm_docker_ps`. Gash does not install it: see <https://docs.docker.com/engine/install/>.
- **docker-compose** (optional) runs compose projects; the docker compose plugin works too. Needed by `docker_compose_check`, `docker_compose_upgrade` and `llm_docker_check`. Gash does not install it: see <https://docs.docker.com/engine/install/>.

## Archives

- **unzip** (feature) extracts .zip archives. Needed by `archive_extract` (.zip) and `chrome_headless_update`.
- **bzip2** (feature) extracts .bz2 archives. Any of: `bzip2`, `bunzip2`. Needed by `archive_extract` (.bz2).
- **xz** (feature) extracts .xz archives, and lets archive_extract check a .tar.xz before extracting it. Needed by `archive_extract` (.xz).
- **zstd** (feature) extracts .zst archives. Needed by `archive_extract` (.zst).
- **7z** (feature) extracts .7z archives. Any of: `7z`, `7zz`. Needed by `archive_extract` (.7z).
- **unrar** (feature) extracts .rar archives. Needed by `archive_extract` (.rar). Where no package exists: <https://www.rarlab.com/download.htm>. Its package is in the non-free repositories (Debian non-free, Ubuntu multiverse, openSUSE non-oss).
- **uncompress** (feature) extracts .Z archives. Needed by `archive_extract` (.Z).

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| unzip | `unzip` | `unzip` | `unzip` | `unzip` | `unzip` | built in |
| bzip2 | `bzip2` | `bzip2` | `bzip2` | `bzip2` | `bzip2` | built in |
| xz | `xz-utils` | `xz` | `xz` | `xz` | `xz` | `xz` |
| zstd | `zstd` | `zstd` | `zstd` | `zstd` | `zstd` | `zstd` |
| 7z | `p7zip-full` | `p7zip-plugins` (EPEL) | `7zip` | `7zip` | `7zip` | `p7zip` |
| unrar | `unrar` (non-free) | — | `unrar` | `unrar` (non-free) | `unrar` | — |
| uncompress | `ncompress` | `ncompress` | `ncompress` | `ncompress` | `ncompress` | built in |

## Runtimes and linters

- **php** (feature) runs PHP code and lints .php files. Needed by `gash run` (--runtime php), `gash runtime` (php), `llm_lint` (.php files), `llm_runtime` (php) and `php_detect_version`.
- **composer** (optional) runs composer with a chosen PHP version. Needed by `gash run` (--runtime composer), `gash runtime` (composer) and `llm_runtime` (composer). Gash does not install it: see <https://getcomposer.org/download/>.
- **node** (feature) runs Node code and lints .js files. Needed by `gash run` (--runtime node), `gash runtime` (node), `llm_lint` (.js files), `llm_runtime` (node), `node_upgrade` and `nvm_prune_versions`. Gash does not install it: see <https://github.com/nvm-sh/nvm#installing-and-updating>. Gash works with the Node versions nvm installs.
- **python3** (feature) lints .py files. Needed by `llm_lint` (.py files).

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| php | `php-cli` | `php-cli` | `php` | `php8-cli` | `php83` | `php` |
| python3 | `python3` | `python3` | `python` | `python3` | `python3` | `python` |

## Agent command runner (llm_exec)

- **gawk** (optional) lets llm_exec run awk, through gawk --sandbox. Needed by `llm_exec` (awk).
- **gnu-sed** (optional) lets llm_exec run sed, through GNU sed --sandbox. Any of: `sed`, `gsed`. Needed by `llm_exec` (sed). Homebrew installs it as gsed; Gash finds it there.
- **diffutils** (optional) lets llm_exec run diff -r, through GNU diff --no-dereference. Needed by `llm_exec` (diff -r).
- **bwrap** (optional) sandboxes llm_exec when GASH_LLM_SANDBOX=1. Only on Linux. Needed by `llm_exec` (GASH_LLM_SANDBOX=1).

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| gawk | `gawk` | `gawk` | `gawk` | `gawk` | `gawk` | `gawk` |
| gnu-sed | `sed` | `sed` | `sed` | `sed` | `sed` | `gnu-sed` |
| diffutils | `diffutils` | `diffutils` | `diffutils` | `diffutils` | `diffutils` | `diffutils` |
| bwrap | `bubblewrap` | `bubblewrap` | `bubblewrap` | `bubblewrap` | `bubblewrap` | — |

## System

- **sudo** (feature) runs the privileged steps of some functions. Needed by `ai_sysinfo` (the privileged sections), `gash hte`, `gash logs` (clean --sudo), `services_stop`, `sudo_last`, `sysinfo` (the privileged sections) and `wsl_compact`.
- **systemctl** (feature) reads and controls systemd services. Only on Linux. Needed by `llm_service` and `services_stop`. It comes with systemd, part of the operating system: Gash cannot install it. Under WSL, enable systemd in /etc/wsl.conf.
- **journalctl** (optional) adds the service journal to llm_service --log. Only on Linux. Needed by `llm_service` (--log). It comes with systemd.
- **fzf** (optional) picks an entry from a list. Needed by `hgrep` (--pick) and `history_grep` (--pick).
- **bc** (optional) is the calculator behind the calc alias. Needed by `gash init` (the calc alias).

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| sudo | `sudo` | `sudo` | `sudo` | `sudo` | `sudo` | built in |
| fzf | `fzf` | `fzf` (EPEL) | `fzf` | `fzf` | `fzf` | `fzf` |
| bc | `bc` | `bc` | `bc` | `bc` | `bc` | built in |

## Local web stack (gash hte)

- **apache** (feature) serves the gash hte sites. Any of: `apache2`, `httpd`. Needed by `gash hte`.
- **php-fpm** (feature) runs PHP for the gash hte sites, one pool per PHP version. Needed by `gash hte` (PHP sites).
- **openssl** (feature) makes the gash hte local certificate authority and site certificates. Needed by `gash hte`.

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| apache | `apache2` | `httpd` | `apache` | `apache2` | `apache2` | `httpd` |
| php-fpm | `php-fpm` | `php-fpm` | `php-fpm` | `php8-fpm` | `php83-fpm` | `php` |
| openssl | `openssl` | `openssl` | `openssl` | `openssl` | `openssl` | `openssl@3` |

## Windows (WSL)

- **powershell.exe** (feature) answers the gash win queries and edits the Windows hosts file. Only on WSL. Needed by `gash win`. It comes with Windows: check that WSL interop is enabled ([interop] enabled=true in /etc/wsl.conf).
- **cmd.exe** (feature) restarts and shuts down WSL from inside it. Only on WSL. Needed by `wsl_restart`, `wsl_shutdown` and `wsl_taskmanager`. It comes with Windows: check that WSL interop is enabled ([interop] enabled=true in /etc/wsl.conf).

## Release signatures

- **cosign** (optional) checks the Sigstore signature of a release. Needed by `gash update` (GASH_REQUIRE_SIGNED=1) and `gash_upgrade` (GASH_REQUIRE_SIGNED=1). Where no package exists: <https://docs.sigstore.dev/cosign/system_config/installation/>.
- **gpg** (optional) checks signed release tags. Needed by `gash_upgrade` (GASH_REQUIRE_SIGNED=1).

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| cosign | — | — | `cosign` | — | `cosign` | `cosign` |
| gpg | `gnupg` | `gnupg2` | `gnupg` | `gpg2` | `gnupg` | `gnupg` |
