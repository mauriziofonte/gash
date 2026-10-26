# Remote hosts

Run commands on, and copy and sync files with, the SSH hosts named in ~/.gash_env, and unlock SSH keys at login.

You give each server a short name once, in `~/.gash_env`, and then run a
command on it, push or pull a file, or mirror a directory with that name.
Every call goes through your own `ssh`, `scp` and `rsync`, never asks for a
password, and refuses remote paths that would touch the system (such as
`/etc/ssh` or `/boot`). Gash can also load your SSH keys into an agent when a
shell starts, so the keys are ready for these calls.

Platforms: Linux, macOS and WSL. The functions run the `ssh`, `scp` and
`rsync` of the system; the `rsync` that macOS ships (openrsync) works too.
SSH keys are unlocked in interactive shells only.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [ssh](../requirements.md#tool-ssh) (feature), for `remote_exec`, `remote_pull`, `remote_push`, `remote_sync_pull` and `remote_sync_push`
- [scp](../requirements.md#tool-scp) (feature), for `remote_pull` and `remote_push`
- [rsync](../requirements.md#tool-rsync) (feature), for `remote_sync_pull` and `remote_sync_push`
- [ssh-add](../requirements.md#tool-ssh-add) (feature), for `gash_ssh_auto_unlock`
- [ssh-agent](../requirements.md#tool-ssh-agent) (feature), for `gash_ssh_auto_unlock`
- [ssh-keygen](../requirements.md#tool-ssh-keygen) (optional), for `gash_ssh_auto_unlock` (keys already loaded)
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).

## Name your hosts

Add one `HOST:` line per server to `~/.gash_env` (`gash_env_init` creates the
file from a template):

```text
HOST:web=deploy@web.example.com
HOST:nas=admin@192.168.1.20
```

The format is `HOST:alias=user@host`. The user is letters, digits, `.`, `_`
and `-` (not first); the host is a name or an IPv4 address, or an IPv6
address in brackets (`deploy@[2001:db8::1]`). There is no place for a port
or a key file: put them in a `Host` entry of `~/.ssh/config` and use that
entry's name as the host. A line that breaks these rules is skipped with a
warning, and the first entry wins when two share an alias. The full grammar
of the file is in [configuration](../configuration.md#remote-hosts).

`remote_hosts` (alias `rh`) lists the aliases in effect. The functions read
the file at each call, so a new line works at once; the warnings about a bad
line appear the first time a shell reads the file:

<!-- example
printf 'HOST:web=deploy@web.example.com\nHOST:nas=admin@192.168.1.20\nHOST:old=deploy@old.example.com:2222\n' >.gash_env
chmod 600 .gash_env
-->
```console
$ remote_hosts
Warning: Line 3 of ~/.gash_env: HOST target must be user@host (skipping)
Hint: put the port in a Host entry of ~/.ssh/config
Info: Configured remote hosts:

  web             deploy@web.example.com
  nas             admin@192.168.1.20
```

Every `remote_*` function takes an alias or, in an interactive shell, a raw
`user@host`. In headless mode (`GASH_HEADLESS=1`, the mode agents use) only
the aliases are accepted, and `remote_exec`, `remote_push`, `remote_pull`
and the `remote_sync_*` functions answer a refusal with one JSON error line
on stderr, as the agent functions do (`unknown_host`, `host_unreachable`,
`forbidden_path`, `write_path_denied`, `sensitive_path`, `invalid_path`,
`missing_value` and the like; see the error contract in
[AGENTS.md](../../AGENTS.md)); in a terminal they print the `Error:` lines
below. `GASH_ENV_FILE` points Gash at another file than `~/.gash_env`.

## Run a command on a host

`remote_exec` (alias `rex`) runs one command line on the host and passes its
output and exit status through:

```bash
rex web 'df -h / && uptime'
rex web 'tail -n 50 /var/log/nginx/error.log | grep -i timeout'
```

Pass the whole remote command as one quoted string: the remote shell reads
it, so pipes and `&&` work there. Extra words are refused rather than
dropped. For anything longer, run a local script on the host with
`--script`; the words after `--` become its arguments, and `--script -`
reads the script from standard input:

```bash
rex web --script ./provision.sh -- --verbose
printf 'hostname\nuptime\n' | rex web --script -
```

What to know about the connection:

- `ssh` runs with `BatchMode=yes`: it never asks for a password, a
  passphrase or a new host key. The key must be loaded in an agent (see
  [Unlock SSH keys](#unlock-ssh-keys-when-a-shell-starts)) or have no
  passphrase, and the host key must already be in `~/.ssh/known_hosts`
  (connect once with plain `ssh deploy@web.example.com` to accept it).
- Connections are multiplexed: the first call opens one connection, later
  calls to the same host reuse it, and it closes 60 seconds after the last
  one. The control socket lives in `~/.ssh` (created with mode 0700 when it
  is missing).
- The connect timeout is 10 seconds.

See [`remote_exec`](reference.md#remote_exec) for the exit status rules.

## Copy a file or a directory

`remote_push` (alias `rpush`) copies a local file to the host, and
`remote_pull` (alias `rpull`) copies a remote file here, both with `scp`.
The arguments are always the alias, the source, then the destination:

```bash
rpush web ./nginx.conf /tmp/nginx.conf
rpull web /srv/backups/db.sql.gz ./db.sql.gz
rpull web /srv/app/config ./config -r -p   # a directory, keeping times and modes
```

`-r` copies a directory, `-p` keeps modification times and modes, and
`--no-progress` silences the progress meter. `--force` answers yes to the
question about a sensitive path (below). A remote path that does not start
with `/` is relative to the remote home. See
[`remote_push`](reference.md#remote_push) and
[`remote_pull`](reference.md#remote_pull).

## Keep a directory in sync

`remote_sync_push` (alias `rspush`) and `remote_sync_pull` (alias `rspull`)
mirror a directory with `rsync` (archive mode, compressed), over the same
multiplexed connection as `remote_exec`. They copy only what changed, so they
suit deploys and backups:

```bash
rspush web ./dist/ /srv/www/app/
rspush web ./dist/ /srv/www/app/ --delete --exclude '*.map'
rspull web /srv/www/app/storage/ ./storage-backup/
```

The trailing `/` keeps its rsync meaning: `./dist/` copies the contents of
`dist`, `./dist` copies the directory itself. `--dry-run` passes rsync's own
`--dry-run`: nothing is copied or deleted. `--exclude PATTERN` (repeatable)
leaves files out, and `--no-progress` silences the progress line.
`--delete` removes files from the destination that the source does not
have, so it has its own check: it runs without asking only when the
destination is below `/tmp`, `/var/tmp` or `/home/<user>/`, or is a
relative path (inside the remote home for a push, inside the current
directory for a pull). A home directory itself and any `.ssh` path never
count as safe. Anywhere else Gash asks first, as it does for the second
command above, or, in headless mode, needs `--force`. See [`remote_sync_push`](reference.md#remote_sync_push) and
[`remote_sync_pull`](reference.md#remote_sync_pull).

## Which remote paths are allowed

Before `scp` or `rsync` starts, the remote path goes through three tiers:

| Tier | Which paths | What happens |
|---|---|---|
| Deny | `/` itself, the system directories (such as `/boot`, `/proc`, `/usr/bin`) and the files that control logins (such as `/etc/ssh`, `/etc/shadow`, `/etc/sudoers`); any path with `..`; any path with a character the remote shell could expand | Refused, even with `--force` |
| Ask | `/etc`, `/var`, `/usr/local`, `/root`, and any path with a `.ssh` component | A y/N question; `--force` skips it; headless, refused without `--force` |
| Allow | Everything else, `/tmp` and `/var/tmp` included | Runs |

The full lists are in [security](../security.md). The characters that are
refused are the glob and shell characters (`*`, `?`, `[`, `$`, a quote, `;`,
`|` and the like) and a `~` past the start: name each file in full instead
of using a pattern. The tiers apply to transfers only; `remote_exec` runs
whatever command you give it.

<!-- example
printf 'HOST:web=deploy@web.example.com\n' >.gash_env
chmod 600 .gash_env
printf 'server {}\n' >nginx.conf
-->
```console
$ remote_push web ./nginx.conf /etc/ssh/sshd_config
Error: Remote path '/etc/ssh/sshd_config' is in the DENY list (matched: /etc/ssh). This is never allowed, not even with --force. Refusing push.
$ remote_pull web '/var/log/nginx/*.log' ./logs/
Error: Remote path '/var/log/nginx/*.log' contains characters the remote side may expand. Refusing pull (source path access).
```

Both refusals happen before anything connects.

## Unlock SSH keys when a shell starts

`BatchMode=yes` means a key with a passphrase works only once it is in an
agent. Gash can load it for you: add one `SSH:` line per key to
`~/.gash_env`, with the passphrase after the `=`:

```text
SSH:~/.ssh/id_ed25519=my passphrase
SSH:~/.ssh/work_rsa=another passphrase
```

The passphrase is everything after the first `=`, spaces included (a
trailing space gets a warning, in case it is an accident); an empty one is
allowed. `~/` stands for your home. The key file must exist, or the line is
skipped with a warning. Keep the file private (`chmod 600 ~/.gash_env`):
Gash warns at each read when others can open it. What storing passphrases in
a file means is described in [security](../security.md).

When an interactive shell starts and the file has `SSH:` lines,
[`gash_ssh_auto_unlock`](reference.md#gash_ssh_auto_unlock) runs:

1. It uses the agent `SSH_AUTH_SOCK` names when one answers. Otherwise it
   looks for Gash's own agent, on the socket `gash-ssh-agent.sock` in
   `$XDG_RUNTIME_DIR` (when that directory is set and private) or in
   `~/.ssh`, starts `ssh-agent` there when none answers, and exports
   `SSH_AUTH_SOCK`. So every terminal you open shares one agent.
2. It skips the keys the agent already holds.
3. It adds each other key with `ssh-add`, typing the passphrase into it on a
   pseudo-terminal of its own, and prints the result as lines that start
   with `SSH:` (for example `SSH: Identity added: ...`).

It runs once per session: it exports `GASH_SSH_AUTOUNLOCK_RAN=1`, so new
shells started from this one skip it. `expect` is not needed. To load a key
you have just added to `~/.gash_env` without opening a new terminal:

```bash
unset GASH_SSH_AUTOUNLOCK_RAN
gash_ssh_auto_unlock
```

## Use a host from the agent functions

The read-only `llm_*` functions take `--host ALIAS` to run the same read on
a registered host: `llm_read /etc/nginx/nginx.conf --host web`, or
`llm_grep timeout /var/log/nginx --host web`. Three functions exist only
for hosts: [`llm_remote_run`](../agents/reference.md#llm_remote_run) runs a
read-only command on several hosts in parallel,
[`llm_same`](../agents/reference.md#llm_same) checks that a deployed copy
matches the local one, and [`agent_put`](../agents/reference.md#agent_put)
uploads a file atomically. They all use the `HOST:` aliases of this page and
the same multiplexed connection, and they accept an alias only, never a raw
`user@host`. Remote paths pass the deny tier above before anything connects,
and the paths you protect for the host with `PROTECT:` lines are kept from
them on the host too. `GASH_REMOTE_NO_BINARY` names the hosts that must
never receive the gash binary: there only the POSIX fallbacks answer. See
[configuration.md](../configuration.md#protected-paths).
They answer in JSON, for agents first; the
[agents guide](../agents/README.md) explains them, and
[AGENTS.md](../../AGENTS.md) holds the rules an agent follows.

## When something goes wrong

| Message | What it means and what to do |
|---|---|
| `Error: Unknown host alias 'web' and not a valid user@host target.` | No `HOST:web=` line is in effect: check `rh`, and add the line. |
| `{"error":"unknown_host","details":"Unknown host alias 'web'. In headless mode only aliases registered in ~/.gash_env (HOST:alias=user@host) are accepted.",...}` | The same in headless mode, where a raw `user@host` is never accepted: add a `HOST:` line. |
| `Warning: Line N of ~/.gash_env: HOST target must be user@host (skipping)` | The target has a port, an option or another character the rule refuses. Move the port or the key to `~/.ssh/config` and use that `Host` name. |
| `Error: Failed to connect to deploy@web.example.com. Verify SSH connectivity and key configuration.` | `ssh` could not connect or log in (status 1). Try `ssh deploy@web.example.com` by hand: it shows the reason, lets you accept a new host key, and tells you whether the key is missing from the agent (`ssh-add -l` lists the loaded keys). |
| `Error: Remote command exited 255 (killed by a signal or explicit exit 255); the SSH connection is healthy.` | The connection works; the command itself ended with status 255. |
| `Error: Pass the remote command as a SINGLE quoted string: ...` | `rex web ls -la` has three words: write `rex web 'ls -la'`. |
| `Error: Remote path '...' is in the DENY list (matched: ...)` | The path is in the deny tier; no option overrides it. Use a path below `/tmp` or your home, then move the file on the host with `rex`. |
| `Error: Remote path '...' contains characters the remote side may expand.` | Remove the glob or shell characters; name the file in full, or sync the whole directory. |
| `Remote path '...' is sensitive (matched: /etc). Proceed with push? [y/N]` | The ask tier. Answer `y`, or pass `--force`. Headless, the call is refused unless `--force` is given. |
| `rsync --delete targeting '...' can remove files on the remote. Proceed? [y/N]` | `--delete` outside the safe directories. Check the destination, then answer, or pass `--force`. |
| `Error: Unknown option: -x (usage: ...)` or `Error: Unexpected argument: ...` | Only the options of the usage line are accepted, after the three positional arguments. |
| `Error: scp is not installed.` / `Error: rsync is not installed.` | Install it: [requirements](../requirements.md#tool-scp), [rsync](../requirements.md#tool-rsync). |
| `SSH: Bad passphrase, try again for ...` | The passphrase in the `SSH:` line is wrong. Fix the line, then re-run the unlock as shown above. |
| `Error: ssh-add is not available; cannot auto-unlock SSH keys.` | Install the OpenSSH client ([ssh-add](../requirements.md#tool-ssh-add)). |
| `Error: ssh-agent is not running and could not be started automatically.` | `ssh-agent` is missing or could not start: install it ([ssh-agent](../requirements.md#tool-ssh-agent)), or start an agent yourself and open a new shell. |

## See also

- [reference.md](reference.md): every function of this area with all its options.
- [configuration](../configuration.md#remote-hosts): the `HOST:` and [`SSH:`](../configuration.md#ssh-keys) lines of `~/.gash_env`.
- [security](../security.md): the remote path tiers in full, and how passphrases in `~/.gash_env` are protected.
- [Databases](../databases/README.md): a `DB:` connection can reach a database on a registered host (`mysql+ssh://`).
- [Tools for AI agents](../agents/README.md): `--host`, `llm_remote_run`, `llm_same` and `agent_put`.
