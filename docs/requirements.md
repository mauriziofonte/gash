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

- <a id="tool-bash"></a>**bash** (required) runs Gash; version 4.4 or newer. Needed by `gash run` (--safe), `gash update`, `llm_exec`, `llm_lint` (.sh files) and `llm_wait` (--cmd-ok). On macOS, make the Homebrew bash your login shell: chsh -s "$(brew --prefix)/bin/bash".
- <a id="tool-curl"></a>**curl** (required) downloads the Gash binary (wget works too). Any of: `curl`, `wget`.
- <a id="tool-tar"></a>**tar** (required) unpacks the Gash release archive. Needed by `archive_extract` (tar archives).
- <a id="tool-gzip"></a>**gzip** (required) decompresses the Gash release archive and .gz files. Needed by `archive_extract` (.gz).
- <a id="tool-sha256sum"></a>**sha256sum** (required) verifies the downloaded binary against its pinned hash. Any of: `sha256sum`, `shasum`.
- <a id="tool-git"></a>**git** (required) installs and updates Gash, and runs the git functions. Needed by `agent_git_commit`, `gash update`, `gash_upgrade`, `git_add_tag`, `git_apply_patch`, `git_delete_tag`, `git_dump_revisions`, `git_list_tags`, `git_repair`, `git_sync`, `gl`, `gla`, `glf`, `glg`, `glo`, `gls`, `llm_find` (--tracked), `llm_git_diff`, `llm_git_log`, `llm_git_overview`, `llm_git_show` and `llm_git_status`. On macOS git comes with the Command Line Tools: xcode-select --install.
- <a id="tool-grep"></a>**grep** (required) is used by the installer.
- <a id="tool-sed"></a>**sed** (required) is used by the installer and gash_uninstall. Needed by `gash_uninstall`.

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

- <a id="tool-ssh"></a>**ssh** (feature) connects to remote hosts. Needed by every `--host` call, `agent_put`, `llm_db_databases` (a +ssh URL), `llm_db_explain` (a +ssh URL), `llm_db_query` (a +ssh URL), `llm_db_sample` (a +ssh URL), `llm_db_schema` (a +ssh URL), `llm_db_tables` (a +ssh URL), `llm_remote_run`, `llm_same` (a host side), `remote_exec`, `remote_pull`, `remote_push`, `remote_sync_pull` and `remote_sync_push`.
- <a id="tool-scp"></a>**scp** (feature) copies files to and from remote hosts. Needed by `remote_pull` and `remote_push`.
- <a id="tool-rsync"></a>**rsync** (feature) syncs directories with remote hosts. Needed by `remote_sync_pull` and `remote_sync_push`.
- <a id="tool-ssh-add"></a>**ssh-add** (feature) loads SSH keys into the agent. Needed by `gash_ssh_auto_unlock`.
- <a id="tool-ssh-agent"></a>**ssh-agent** (feature) holds the unlocked SSH keys. Needed by `gash_ssh_auto_unlock`.
- <a id="tool-ssh-keygen"></a>**ssh-keygen** (optional) tells the SSH keys already in the agent, and checks SSH-signed release tags. Needed by `gash update` (GASH_REQUIRE_SIGNED=1), `gash_ssh_auto_unlock` (keys already loaded) and `gash_upgrade` (GASH_REQUIRE_SIGNED=1).

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| ssh | `openssh-client` | `openssh-clients` | `openssh` | `openssh-clients` | `openssh-client` | built in |
| scp | `openssh-client` | `openssh-clients` | `openssh` | `openssh-clients` | `openssh-client` | built in |
| rsync | `rsync` | `rsync` | `rsync` | `rsync` | `rsync` | built in |
| ssh-add | `openssh-client` | `openssh-clients` | `openssh` | `openssh-clients` | `openssh-client` | built in |
| ssh-agent | `openssh-client` | `openssh-clients` | `openssh` | `openssh-clients` | `openssh-client` | built in |
| ssh-keygen | `openssh-client` | `openssh-clients` | `openssh` | `openssh-clients` | `openssh-client` | built in |

## Processes and ports

- <a id="tool-ps"></a>**ps** (feature) lists the running processes. Needed by `agent_kill` (on macOS, or without /proc), `llm_procs` (on macOS, or without /proc), `llm_wait` (--gone, on macOS) and `process_find`.
- <a id="tool-pgrep"></a>**pgrep** (feature) finds processes by name. Needed by `process_kill`.
- <a id="tool-lsof"></a>**lsof** (feature) maps ports to the processes that hold them. Needed by `llm_procs` (--port) and `port_kill`.
- <a id="tool-ss"></a>**ss** (feature) lists the listening ports; netstat works too. Any of: `ss`, `netstat`. Needed by `ai_sysinfo` (questions), `llm_ports` and `sysinfo` (the listening ports).

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| ps | `procps` | `procps-ng` | `procps-ng` | `procps` | `procps` | built in |
| pgrep | `procps` | `procps-ng` | `procps-ng` | `procps` | `procps` | built in |
| lsof | `lsof` | `lsof` | `lsof` | `lsof` | `lsof` | built in |
| ss | `iproute2` | `iproute` | `iproute2` | `iproute2` | `iproute2` | built in |

## Docker

- <a id="tool-docker"></a>**docker** (feature) runs and inspects containers. Needed by `agent_docker_exec`, `docker_compose_check`, `docker_compose_upgrade`, `docker_prune_all`, `docker_start_all`, `docker_stop_all`, `llm_docker_check`, `llm_docker_exec`, `llm_docker_logs` and `llm_docker_ps`. Gash does not install it: see <https://docs.docker.com/engine/install/>.
- <a id="tool-docker-compose"></a>**docker-compose** (optional) runs compose projects; the docker compose plugin works too. Needed by `docker_compose_check`, `docker_compose_upgrade` and `llm_docker_check`. Gash does not install it: see <https://docs.docker.com/engine/install/>.

## Archives

- <a id="tool-unzip"></a>**unzip** (feature) extracts .zip archives. Needed by `archive_extract` (.zip) and `chrome_headless_update`.
- <a id="tool-bzip2"></a>**bzip2** (feature) extracts .bz2 archives. Any of: `bzip2`, `bunzip2`. Needed by `archive_extract` (.bz2).
- <a id="tool-xz"></a>**xz** (feature) extracts .xz archives, and lets archive_extract check a .tar.xz before extracting it. Needed by `archive_extract` (.xz).
- <a id="tool-zstd"></a>**zstd** (feature) extracts .zst archives. Needed by `archive_extract` (.zst).
- <a id="tool-7z"></a>**7z** (feature) extracts .7z archives. Any of: `7z`, `7zz`. Needed by `archive_extract` (.7z).
- <a id="tool-unrar"></a>**unrar** (feature) extracts .rar archives. Needed by `archive_extract` (.rar). Where no package exists: <https://www.rarlab.com/download.htm>. Its package is in the non-free repositories (Debian non-free, Ubuntu multiverse, openSUSE non-oss).
- <a id="tool-uncompress"></a>**uncompress** (feature) extracts .Z archives. Needed by `archive_extract` (.Z). Where no package exists: <https://github.com/vapier/ncompress>.

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| unzip | `unzip` | `unzip` | `unzip` | `unzip` | `unzip` | built in |
| bzip2 | `bzip2` | `bzip2` | `bzip2` | `bzip2` | `bzip2` | built in |
| xz | `xz-utils` | `xz` | `xz` | `xz` | `xz` | `xz` |
| zstd | `zstd` | `zstd` | `zstd` | `zstd` | `zstd` | `zstd` |
| 7z | `p7zip-full` | `p7zip-plugins` (EPEL) | `7zip` | `7zip` | `7zip` | `p7zip` |
| unrar | `unrar` (non-free) | — | `unrar` | `unrar` (non-free) | `unrar` | — |
| uncompress | `gzip` | `ncompress` | `ncompress` | `ncompress` | — | built in |

## Runtimes and linters

- <a id="tool-php"></a>**php** (feature) runs PHP code and lints .php files. Needed by `gash run` (--runtime php), `gash runtime` (php), `llm_lint` (.php files), `llm_runtime` (php) and `php_detect_version`.
- <a id="tool-composer"></a>**composer** (optional) runs composer with a chosen PHP version. Needed by `gash run` (--runtime composer), `gash runtime` (composer) and `llm_runtime` (composer). Gash does not install it: see <https://getcomposer.org/download/>.
- <a id="tool-node"></a>**node** (feature) runs Node code and lints .js files. Needed by `gash run` (--runtime node), `gash runtime` (node), `llm_lint` (.js files), `llm_runtime` (node), `node_upgrade` and `nvm_prune_versions`. Gash does not install it: see <https://github.com/nvm-sh/nvm#installing-and-updating>. Gash works with the Node versions nvm installs.
- <a id="tool-python3"></a>**python3** (feature) lints .py files. Needed by `llm_lint` (.py files).

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| php | `php-cli` | `php-cli` | `php` | `php8-cli` | `php83` | `php` |
| python3 | `python3` | `python3` | `python` | `python3` | `python3` | `python` |

## Agent command runner (llm_exec)

- <a id="tool-gawk"></a>**gawk** (optional) lets llm_exec run awk, through gawk --sandbox. Needed by `llm_exec` (awk).
- <a id="tool-gnu-sed"></a>**gnu-sed** (optional) lets llm_exec run sed, through GNU sed --sandbox. Any of: `sed`, `gsed`. Needed by `llm_exec` (sed). On macOS, Homebrew installs it as gsed, which Gash finds.
- <a id="tool-diffutils"></a>**diffutils** (optional) lets llm_exec run diff -r, through GNU diff --no-dereference. Needed by `llm_exec` (diff -r).
- <a id="tool-bwrap"></a>**bwrap** (optional) sandboxes llm_exec when GASH_LLM_SANDBOX=1. Only on Linux. Needed by `llm_exec` (GASH_LLM_SANDBOX=1).

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| gawk | `gawk` | `gawk` | `gawk` | `gawk` | `gawk` | `gawk` |
| gnu-sed | `sed` | `sed` | `sed` | `sed` | `sed` | `gnu-sed` |
| diffutils | `diffutils` | `diffutils` | `diffutils` | `diffutils` | `diffutils` | `diffutils` |
| bwrap | `bubblewrap` | `bubblewrap` | `bubblewrap` | `bubblewrap` | `bubblewrap` | — |

## System

- <a id="tool-sudo"></a>**sudo** (feature) runs the privileged steps of some functions. Needed by `ai_sysinfo` (the privileged sections), `gash hte`, `gash logs` (clean --sudo), `services_stop`, `sudo_last`, `sysinfo` (the privileged sections) and `wsl_compact`.
- <a id="tool-systemctl"></a>**systemctl** (feature) reads and controls systemd services. Only on Linux. Needed by `ai_sysinfo` (questions), `llm_service` and `services_stop`. It comes with systemd, part of the operating system: Gash cannot install it. Under WSL, enable systemd in /etc/wsl.conf.
- <a id="tool-journalctl"></a>**journalctl** (optional) adds the service journal to llm_service --log. Only on Linux. Needed by `ai_sysinfo` (questions) and `llm_service` (--log). It comes with systemd.
- <a id="tool-fzf"></a>**fzf** (optional) picks an entry from a list. Needed by `hgrep` (--pick).
- <a id="tool-truncate"></a>**truncate** (optional) empties root-owned live logs for gash logs clean --sudo --truncate. Needed by `gash logs` (clean --sudo --truncate).
- <a id="tool-ldd"></a>**ldd** (feature) lists the shared libraries chrome-headless-shell needs. Only on Linux. Needed by `chrome_headless_update`.
- <a id="tool-bc"></a>**bc** (optional) is the calculator behind the calc alias. Needed by `gash init` (the calc alias).

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| sudo | `sudo` | `sudo` | `sudo` | `sudo` | `sudo` | built in |
| fzf | `fzf` | `fzf` (EPEL) | `fzf` | `fzf` | `fzf` | `fzf` |
| truncate | `coreutils` | `coreutils` | `coreutils` | `coreutils` | `coreutils` | `truncate` |
| ldd | `libc-bin` | `glibc-common` | `glibc` | `glibc` | `musl-utils` | — |
| bc | `bc` | `bc` | `bc` | `bc` | `bc` | built in |

## Local web stack (gash hte)

- <a id="tool-apache"></a>**apache** (feature) serves the gash hte sites. Any of: `apache2`, `httpd`. Needed by `gash hte`. Gash hte runs on Debian, Ubuntu, Fedora, the RHEL family, Arch and macOS with Homebrew.
- <a id="tool-php-fpm"></a>**php-fpm** (feature) runs PHP for the gash hte sites: one master per PHP version, one pool per site. Needed by `gash hte` (PHP sites). Gash hte runs on Debian, Ubuntu, Fedora, the RHEL family, Arch and macOS with Homebrew.
- <a id="tool-openssl"></a>**openssl** (feature) makes the self-signed certificate of each gash hte site. Needed by `gash hte`.

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| apache | `apache2` | `httpd mod_ssl mod_http2` | `apache` | — | — | `httpd` |
| php-fpm | `php-fpm` | `php-fpm` | `php-fpm` | — | — | `php` |
| openssl | `openssl` | `openssl` | `openssl` | `openssl` | `openssl` | `openssl@3` |

## Windows (WSL)

- <a id="tool-powershell.exe"></a>**powershell.exe** (feature) answers the gash win queries and edits the Windows hosts file. Only on WSL. Needed by `gash win`. It comes with Windows: check that WSL interop is enabled ([interop] enabled=true in /etc/wsl.conf).
- <a id="tool-cmd.exe"></a>**cmd.exe** (feature) restarts and shuts down WSL from inside it, and starts the Task Manager. Only on WSL. Needed by `wsl_restart`, `wsl_shutdown` and `wsl_taskmanager`. It comes with Windows: check that WSL interop is enabled ([interop] enabled=true in /etc/wsl.conf).
- <a id="tool-explorer.exe"></a>**explorer.exe** (feature) opens Windows Explorer from WSL. Only on WSL. Needed by `wsl_explorer`. It comes with Windows: check that WSL interop is enabled ([interop] enabled=true in /etc/wsl.conf).

## Release signatures

- <a id="tool-cosign"></a>**cosign** (optional) checks the Sigstore signature of a release. Needed by `gash update` (GASH_REQUIRE_SIGNED=1) and `gash_upgrade` (GASH_REQUIRE_SIGNED=1). Where no package exists: <https://docs.sigstore.dev/cosign/system_config/installation/>.
- <a id="tool-gpg"></a>**gpg** (optional) checks signed release tags. Needed by `gash update` (GASH_REQUIRE_SIGNED=1) and `gash_upgrade` (GASH_REQUIRE_SIGNED=1).

| Tool | Debian, Ubuntu (apt) | Fedora, RHEL family, Amazon Linux (dnf) | Arch (pacman) | openSUSE (zypper) | Alpine (apk) | macOS (Homebrew) |
| --- | --- | --- | --- | --- | --- | --- |
| cosign | — | — | `cosign` | — | `cosign` | `cosign` |
| gpg | `gnupg` | `gnupg2` | `gnupg` | `gpg2` | `gnupg` | `gnupg` |
