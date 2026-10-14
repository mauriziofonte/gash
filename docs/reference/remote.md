# remote

Hosts from ~/.gash_env: exec, push, pull, sync.

_Generated from the help registry by `tools/gen-docs.sh`; do not edit by hand._

| Function | Alias | Description |
|---|---|---|
| [`gash_ssh_auto_unlock`](#gash-ssh-auto-unlock) |  | Auto-unlock SSH keys from ~/.gash_env on shell startup |
| [`remote_exec`](#remote-exec) | `rex` | Run a command or script on a remote host via multiplexed SSH |
| [`remote_hosts`](#remote-hosts) | `rh` | List configured remote host aliases |
| [`remote_pull`](#remote-pull) | `rpull` | Copy a remote file/dir to the local machine via scp |
| [`remote_push`](#remote-push) | `rpush` | Copy a local file/dir to a remote host via scp |
| [`remote_sync_pull`](#remote-sync-pull) | `rspull` | Sync a remote directory to the local machine via rsync |
| [`remote_sync_push`](#remote-sync-push) | `rspush` | Sync a local directory to a remote host via rsync |

## gash_ssh_auto_unlock

Auto-unlock SSH keys from ~/.gash_env on shell startup

#### Usage

```text
gash_ssh_auto_unlock
```

#### Examples

```bash
# Runs automatically on shell startup if SSH keys are configured.
# Manual invocation:
gash_ssh_auto_unlock
```

#### Configuration

```text
Add SSH keys to ~/.gash_env:
  SSH:~/.ssh/id_ed25519=your_passphrase
  SSH:~/.ssh/work_rsa=work-passphrase
```

#### Requirements

```text
- expect (for non-interactive passphrase entry)
- ssh-agent (auto-started if not running)
```

#### Notes

```text
Runs once per session (tracked by GASH_SSH_AUTOUNLOCK_RAN).
Only unlocks keys that are not already loaded in the agent.
```

See also: `gash_env_init`

## remote_exec

Run a command or script on a remote host via multiplexed SSH

Alias: `rex`

#### Usage

```text
remote_exec <alias|user@host> "command"
remote_exec <alias|user@host> --script FILE [-- arg1 arg2 ...]
remote_exec <alias|user@host> --script -
```

#### Examples

```bash
# Run a single command
rex hnas "ls -la /root"

# Run a local script on the remote (no quoting hell)
rex myserver --script /tmp/provision.sh -- --verbose
```

#### Notes

```text
Connections are multiplexed (ControlMaster, 60s persist): repeated calls to
the same host reuse one session. Under GASH_HEADLESS=1 only a registered
alias (~/.gash_env HOST:) is accepted, never a raw user@host target.
```

See also: `remote_hosts`, `remote_push`, `remote_pull`

## remote_hosts

List configured remote host aliases

Alias: `rh`

#### Usage

```text
remote_hosts
```

#### Examples

```bash
# See which aliases are registered
rh
```

#### Notes

```text
Reads HOST:alias=user@host entries from ~/.gash_env.
```

See also: `remote_exec`, `remote_push`, `remote_pull`

## remote_pull

Copy a remote file/dir to the local machine via scp

Alias: `rpull`

#### Usage

```text
remote_pull <alias|user@host> <remote-src> <local-dst> [-r] [-p] [--no-progress] [--force]
```

#### Examples

```bash
# Pull a single file
rpull viper /tmp/backup.tar.gz ./backup.tar.gz
```

See also: `remote_push`, `remote_sync_pull`

## remote_push

Copy a local file/dir to a remote host via scp

Alias: `rpush`

#### Usage

```text
remote_push <alias|user@host> <local-src> <remote-dst> [-r] [-p] [--no-progress] [--force]
```

#### Examples

```bash
# Push a config file
rpush hnas ./nginx.conf /tmp/nginx.conf

# Push a directory
rpush hnas ./dist /tmp/dist -r
```

See also: `remote_pull`, `remote_sync_push`

## remote_sync_pull

Sync a remote directory to the local machine via rsync

Alias: `rspull`

#### Usage

```text
remote_sync_pull <alias|user@host> <remote-dir> <local-dir> [--delete] [--exclude=PAT] [--dry-run] [--no-progress] [--force]
```

#### Examples

```bash
# Sync a remote project directory locally
rspull viper /home/maurizio/projects/foo/ ./foo/
```

See also: `remote_sync_push`, `remote_pull`

## remote_sync_push

Sync a local directory to a remote host via rsync

Alias: `rspush`

#### Usage

```text
remote_sync_push <alias|user@host> <local-dir> <remote-dir> [--delete] [--exclude=PAT] [--dry-run] [--no-progress] [--force]
```

#### Examples

```bash
# Sync a build directory
rspush hnas ./build/ /tmp/build/ --delete --exclude='*.log'
```

See also: `remote_sync_pull`, `remote_push`

