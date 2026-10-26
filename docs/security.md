# Security

What Gash protects against, what it does not, and which checks each part enforces.

## Threat model

### What Gash protects against

* **An LLM agent that errs or is prompt-injected** (by a file, web page or
  database row) and reaches the machine only through Gash functions in headless
  mode, mainly `llm_*`: no writes via `llm_exec`, read-only database sessions,
  no secret files or system paths, no unregistered remote hosts, and
  destructive helpers that need `--force` or `--yes` ([Confirmations](#confirmations)).
* **A human typing a destructive command by accident**: interactive `cp`, `mv`
  and `rm` aliases, confirmations before killing processes or pruning Docker.

### What Gash does not protect against

* **A malicious local user, or anyone with a shell as your user**: they can
  read `~/.gash_env`, edit the Gash checkout or set the environment Gash trusts
  (`GASH_ENV_FILE`, `PATH`).
* **An agent that can run arbitrary Bash.** With a general shell tool it can
  run `rm`, read `~/.gash_env` or write `GASH_LLM_EXEC=unrestricted` into it
  without going through Gash. Gash does not contain such an agent.
* **Root, compromised tools** (`git`, `jq`, `curl`, DB clients), or data a permitted command may read.

### Defense in depth, not a sandbox

Gash puts checks in front of ordinary programs running as your user. Each narrows
what a mistake can do; none is an isolation boundary (except the optional `bwrap`
mode of `llm_exec`). To contain an agent, use a container or VM.

## llm_exec

`llm_exec "LINE"` runs in a child `bash --noprofile --norc`, never in the
caller's shell, after three checks:

1. **Blocklist** (always on): the whitespace-collapsed line must not match
   `rm -rf /` and variants, `dd if=`, `mkfs.`, `> /dev/sd*`, `chmod 777 /`,
   `shutdown`, `reboot`, `halt`, fork bombs, `curl ... | sh`, `history -c`,
   `HISTFILE=`, `cat /etc/shadow`, `.ssh/id_`, `AWS_SECRET`, `PRIVATE_KEY`.
   It reads the line as text, quotes included, so `echo 'please shutdown'`
   is refused too.
2. **Quote-aware parse.** Only plain words joined by `|`, `&&`, `||`, `;` or a
   newline. Refused: `$` and backticks (also inside double quotes), `(` `)`,
   brace expansion, background `&`, `|&`, `VAR=value` prefixes, commands given
   by path or glob, and every redirection except `>/dev/null`, `2>/dev/null`,
   `&>/dev/null`, `2>&1` and `>&2`.
3. **Read-only allowlist.** Each command must be on the list (`ls`, `cat`,
   `grep`, `rg`, `find`, `jq`, `ps`, `ss`, `git`, `docker`, `systemctl`, and
   similar; see `gash_help llm_exec`). Commands that also have writing options
   get per-command rules, for example:

| Command | Refused |
|---|---|
| `find`, `fd`, `rg` | `-exec`, `-execdir`, `-ok`, `-delete`, `-fprint*`, `-fls`; `fd -x/-X`; `rg --pre` |
| `sort`, `tree` | `-o`, `--output`, `--compress-program` |
| `sed`, `yq` | `-i`, `--in-place`; `yq` also `-s`/`--split-exp` (writes files) and `load()` (reads any file) |
| `git` | global options other than `-C`, `--no-pager` and a few read-only ones (so no `-c`, no `--literal-pathspecs`); subcommands outside a read-only list; `--output`, `--ext-diff`, `--textconv`, `--no-index`, `cat-file --batch*`; blobs named by id (`show SHA`, `cat-file -p SHA`); `REV:PATH` naming a secret file; creating or deleting branches and tags; `remote` other than `show -n`/`get-url`; `config` without `--get`/`--list`; `stash` other than `list`/`show` (summary options only; no patch options on `list`) |
| `ps`, `getent` | BSD `ps e` (other processes' environments); `getent shadow`, `getent gshadow` |
| `diff` | `-r` where `diff` has no `--no-dereference` |
| `docker` | subcommands other than `ps`, `images`, `logs`, `inspect`, `stats`, `top` and other read-only ones |
| `systemctl`, `journalctl` | anything but `status`, `show`, `cat`, `is-*`, `list-*`; `journalctl --vacuum*`, `--rotate`, `--flush` |
| `php`, `node`, `python`, `ruby`, `java`, `composer` | anything but `--version` |
| `crontab`, `date`, `hostname`, `ip` | anything but `crontab -l`; setting the date or hostname; `ip` other than show/list/get |

Arguments naming a secret file (see below), also as `NAME=path` or as a value
attached to a short option or option cluster (`-f.env`, `-ivf.env`), are
refused.
Unquoted globs are expanded first, by Gash, in byte order whatever the locale
(bash does no globbing of its own); a match that is a secret file or starts with
`-` is refused. In the child shell, functions shadowing allowed commands are
removed, `sed` runs as GNU `sed --sandbox`, `awk` as `gawk --sandbox` (both
refused where unavailable), and `git` as described in [Git](#git). `grep`,
`egrep`, `fgrep`, `rg` and `diff` get exclusions for the secret-file patterns
(GNU grep: after the caller's own `--include`, so they win; `diff -r` also
`--no-dereference`); `git diff`, `show`, `grep`, `diff-*` and `log`/`reflog`
with a patch option get `:(exclude)` pathspecs for them (not with `--follow`
or `-L`, where git takes no extra pathspecs).

The child starts from `env -i` plus an allowlist: `PATH`, `HOME`, `USER`,
`LOGNAME`, `LANG`, `LANGUAGE`, `LC_*`, `TERM`, `TZ`, `TMPDIR`, `NO_COLOR`,
`XDG_RUNTIME_DIR`, `DOCKER_HOST`, `DOCKER_CONTEXT` and `PWD`, with
`GIT_OPTIONAL_LOCKS=0` and `GIT_TERMINAL_PROMPT=0`. Tokens, `GIT_*`,
`LD_*`, `BASH_ENV` and exported functions do not reach it. In `unrestricted`
mode it inherits the caller's environment with `BASH_ENV` and `ENV` empty.

Settings, read from `~/.gash_env` only (never from the environment):

* `GASH_LLM_EXEC_ALLOW="make npm"`: extra commands with any options, without
  per-command option rules; the secret-file, forbidden-path and walk rules
  (no symlink-following walk, no walk at `/` or above a protected tree)
  still apply; listing `sed`, `awk`, `gawk`, `grep`, `rg`, `diff` or
  `git` drops its wrapper.
* `GASH_LLM_EXEC=unrestricted`: skips checks 2 and 3 (secret-file checks
  included) and the wrappers. Only the blocklist remains.
* `GASH_LLM_SANDBOX=1`: runs under `bwrap --ro-bind / / --dev /dev --proc /proc
  --unshare-all --die-with-parent`, so a read-only filesystem and no network;
  reading stays possible. Under WSL the sandbox also mounts an empty
  `/run/WSL` (`--tmpfs /run/WSL`) and never passes `WSL_INTEROP`: a Windows
  program would run on Windows, outside the sandbox, so the interop sockets
  are out of its reach and a `.exe` cannot start from it. Without a working
  `bwrap`: `sandbox_unavailable`.

Every argument is also checked against the [forbidden paths](#forbidden-paths):
option values (`--file=/proc/x`, `-f/proc/x`, `-vf/proc/x`), `key=value` words, `~` and
`~user`, glob matches, and relative paths after a `cd` earlier in the line.
Not checked: `/dev/null`, the arguments of `echo`, `printf`, `basename` and
`dirname`, and the pattern operand of `grep`, `rg`, `sed`, `awk`, `jq` and
`yq` (a pattern that looks like a secret file name, `grep id_rsa`, is still
refused by the secret-file check). An allowed command can still read any other file your user can read
whose name is not secret.

Recursive commands (`grep -r`, `rg`, `find`, `fd`, `tree`, `du`, `ls -R`,
`diff -r`) may not start at `/` or at an ancestor of `/proc`, `/sys`, `/dev`,
`/boot`, `/root` or root's home (`forbidden_path`); content searches (`grep`,
`rg`, `diff`) also not above `/etc`. Walks that follow symlinks are refused
(`option_not_allowed`): `grep -R`, `--dereference-recursive`, `rg -L`,
`find -L`/`-follow`, `fd -L`, `tree -l`, `du -L`, `ls -RL`, and starting
points read from a file (`find -files0-from`, `--files0-from`).

## llm_db_*

Read-only is enforced twice, so one parser mistake does not become a write.
First a **SQL guard** lexes the query with the target engine's quoting rules
(including PostgreSQL `E''` strings, where the `E` starts a token, and dollar
quotes), ignoring literals and comments.
It accepts one statement (a trailing `;` is fine) starting with `SELECT`,
`WITH`, `VALUES`, `TABLE`, `SHOW`, `DESCRIBE`, `DESC`, `EXPLAIN` or `PRAGMA`,
and refuses write keywords anywhere (`INSERT`, `UPDATE`, `DELETE`, `DROP`,
`CREATE`, `ALTER`, `INTO`, `OUTFILE`, `LOAD DATA`, `COPY`, ...), file and
program functions (`LOAD_FILE`, `PG_READ_FILE`, `LO_IMPORT`, `DBLINK`, ...),
`SET`, `SLEEP`, `BENCHMARK`, client commands (a backslash outside a literal,
or at the start of any line, literals and comments included), MySQL
`/*! */` comments, SQLite dot-commands and lines holding only `go` or `/`.
A `PRAGMA` passes only as a read: `PRAGMA name` for a pragma that only
reports a value, or `PRAGMA table_info(t)` and the other schema lookups; a
value (`= v`, `(v)`) and the pragmas that act when run (`optimize`,
`wal_checkpoint`, ...) are refused.
Then the **session is read-only**:

| Engine | Session | Credentials |
|---|---|---|
| SQLite (`-f FILE`) | Opened read-only (`mode=ro`), defensive and query-only, untrusted schemas (`trusted_schema=0`) | The file must pass the forbidden-path check and start with `SQLite format 3` |
| MySQL, MariaDB | `SET SESSION TRANSACTION READ ONLY`; rows capped with `sql_select_limit` | Passed to the built-in driver in memory, never on a command line |
| PostgreSQL | `default_transaction_read_only=on`, `statement_timeout` from `GASH_DB_TIMEOUT` (30 s), 10 s connect timeout | Passed to the built-in driver in memory, never on a command line |

All three engines use drivers built into Gash: no `sqlite3`, `mysql` or
`psql` client runs, and their error text is the driver's (`details` of
`sqlite_error`, `mysql_error`, `pgsql_error`).

Row caps are applied by the engine (outer `LIMIT` or `sql_select_limit`):
`llm_db_query -r N` (default 100), `llm_db_sample --limit N` (default 5). Table
names must match `[A-Za-z_][A-Za-z0-9_]*`; database names given with `-d` or
read from `.target-database` must match `[A-Za-z0-9_][A-Za-z0-9_$.-]{0,63}`
(`invalid_database_name`). `llm_db_explain --analyze` runs the query, in the
same read-only session.

Nothing stops a query from reading what the database account can see: give
Gash an account with read-only grants.

## Secret files

A path is secret when its base name matches, ignoring case, one of:

| Kind | Patterns |
|---|---|
| Environment files | `.env` `.env.*` `*.env` (not `.env.example`, `.env.sample`, `.env.dist`, `.env.template`, `*.env.example`) |
| Keys and keystores | `*.pem` `*.key` `*.p12` `*.pfx` `*.jks` `*.keystore` `*.kdbx` `*.ppk` `id_rsa*` `id_dsa*` `id_ecdsa*` `id_ed25519*` `*_rsa` `*_dsa` `*_ecdsa` `*_ed25519` (not `*.pub`) |
| Credential files | `credentials` `credentials.*` `credentials_*` `credentials-*` `secrets` `secrets.*` `.gash_env` `.pgpass` `.netrc` `_netrc` `.git-credentials` `.npmrc` `.pypirc` `.htpasswd` `.my.cnf` `.boto` `.s3cfg` `.dockercfg` `.vault-token` `kubeconfig` `*.kubeconfig` `*.tfstate` `*.tfstate.*` `*.tfvars` `*.tfvars.json` `.envrc` |
| Host keys | `ssh_host_*_key` (not `*.pub`) |
| Windows (WSL) | `ntuser.dat` `ntuser.dat.*` `usrclass.dat` `usrclass.dat.*` `*.rdp` `unattend.xml` `autounattend.xml` `login data` `logins.json` `key4.db` `key3.db` — reachable over `/mnt` |
| Directories | anything under `.ssh`, `.gnupg`, `.aws`, `.azure`, `.kube`, `.docker`, `.config/gcloud` |

A symlink is judged by its own name and by its target's.

| Function | Effect |
|---|---|
| `llm_exec` | Refuses secret arguments, glob matches and `git` `REV:PATH`; `grep`, `rg`, `diff` and `git` diff/show/grep/patch logs skip secret files (not in `unrestricted` mode, nor for commands in `GASH_LLM_EXEC_ALLOW`) but search the `.env` templates and the `id_*` public keys (`id_rsa.pub`); an exception name that also matches another pattern (`credentials.pub`, `secrets.env.example`) stays skipped there |
| `llm_config` | Refuses a secret file |
| `llm_grep`, `llm_find --contains` | Refuse a secret path; skip secret files and directories in recursive searches, but search the exceptions (`.env.example`, `*.pub`, ...) there, dotfiles included |
| `llm_tree`, `llm_find`, filesystem scanners | List names and sizes only, secret files included |

## Forbidden paths

`/proc`, `/sys`, `/dev`, `/boot`, `/root`, `/etc/shadow`, `/etc/gshadow`,
`/etc/passwd`, `/etc/sudoers`, `/etc/sudoers.d`, `/etc/shadow-`,
`/etc/gshadow-`, `/etc/security/opasswd` and everything below them,
matched per path component (`/rootfs` is allowed), plus root's home where it
is not `/root` (macOS: `/var/root`). A `..` component is refused.
The path is checked as written, after resolving it as `realpath -m` does
(symlinks resolved) and, for macOS, without a `/private` prefix. Applies to
`llm_tree`, `llm_find`, `llm_grep`, `llm_config`, `llm_git_*`, `llm_project`,
`llm_deps`, `llm_docker_check`, SQLite files of `llm_db_*` and the
arguments of `llm_exec`.
The walks of `llm_tree`, `llm_find` and `llm_grep` may not start at `/` or
above a protected tree (content searches, `llm_grep` and
`llm_find --contains`, also not above an `/etc` entry): `forbidden_path`,
hint `Search a subdirectory`. Below the root they skip every entry at or
below a protected path, and they do not follow symlinks. The filesystem scanners (`files_largest`, `dirs_largest`,
`dirs_find_large`, `dirs_list_empty`, `tree_stats`) refuse `/proc`, `/sys`,
`/dev`, `/boot` and `/root` (as written and resolved), also when reached
through a symlink or from `/` (a walk skips them), and `/` without
`--allow-root`.

## Git

A cloned repository can carry configuration that names programs: an
fsmonitor, hooks, an external diff, textconv drivers, a pager. `llm_git_*` and
the `git` of `llm_exec` run with `-c core.fsmonitor=false -c
core.hooksPath=/dev/null -c diff.external= -c core.pager=cat` and
`GIT_OPTIONAL_LOCKS=0` (no index refresh written by `status`), and diff or log
with `--no-ext-diff --no-textconv` (log also `--no-show-signature`). In
`llm_exec`, `GIT_TERMINAL_PROMPT=0` keeps git from asking for credentials,
and `git remote show` needs `-n` (no network). Clean and
smudge filters named in `.gitattributes` and defined in the repository's
configuration can still run during `status` and `diff`: inspect an untrusted
repository's `.git/config` before pointing an agent at it.

The git module functions (`git_sync`, `git_add_tag`, `git_delete_tag`,
`git_apply_patch`, `git_repair`, `git_dump_revisions`, `git_list_tags`) are
your own repository operations and get none of this hardening: they run your
hooks (`pre-commit`, `commit-msg`, ...), signing (`commit.gpgSign`,
`tag.gpgSign`), credential helpers, `core.sshCommand` and `insteadOf` rules,
as a plain `git` would. Do not run them in a repository whose configuration
you do not trust.

## AI module

* **Redaction.** The text sent (question, piped input, collected system data)
  is filtered first: private key blocks, URL passwords, values of names like
  `password`, `secret`, `token`, `api_key`, `access_key`, `private_key`,
  `credentials`, and `AKIA...`, `ghp_`/`github_pat_`, `sk-`/`sk-ant-`, `xox?-`,
  `AIza...` tokens, `Authorization:`/`Proxy-Authorization:` bearer, basic and
  token credentials, the `SSH:KEY=PASSPHRASE` lines of `~/.gash_env`, and
  crypt(3) password hashes (`$1$`, `$2a$`/`$2b$`/`$2y$`, `$5$`, `$6$`, `$7$`,
  `$y$`, `$gy$`). A name followed by `=` or `:` loses its value; a name
  followed only by a space loses the next word only when it looks like a
  token (16 or more characters), so "token expired" stays readable. It is
  pattern based: a secret in another format, or in free text ("the password
  is hunter2"), is sent. The piped input and the question are redacted
  separately, so an unclosed private key block in the pipe never swallows
  the question.
* **WireGuard keys.** The `sysinfo` drill-downs, and the AI collectors that
  run them, print the value of every WireGuard `PrivateKey` and
  `PresharedKey` line as `[REDACTED]`, whatever its case and spacing.
* **Transport.** The request is made inside the gash process over HTTPS: the
  body is never written to a file, and the API key is a request header set
  in the process, never on a command line. Control characters are stripped
  from replies.
* **Consent.** `GASH_AI_CONFIRM=auto` (default) asks before sending data Gash
  collected (`ai_sysinfo`) and sends what you typed or piped; `1` always asks;
  `0` never; any other value (case-sensitive: `off`, `AUTO`) always asks and
  warns once. Where consent is needed, no terminal or headless mode means
  refusal. Headless mode reads it from `~/.gash_env` too; the environment
  wins. `GASH_AI_DEBUG=1` prints the (redacted) request
  bodies and raw replies on stderr.
* **`ai_sysinfo` questions.** A path named in a `?` question passes the same
  guard as the `llm_*` readers: a `..` component is refused, symlinks are
  resolved, and a [forbidden path](#forbidden-paths) or a
  [secret file](#secret-files) (lexical or resolved, base names
  case-insensitive) contributes only `=== FILE: PATH === (skipped: protected
  path)`, never its content; directory listings leave secret entries out.
  The data a question collects is confirmed per question (as `data`, so the
  default `auto` asks too); without a terminal nothing is sent.

## remote_*

In headless mode only aliases registered as `HOST:alias=user@host` are
accepted; interactively a raw `user@host` works too. A target, registered or
raw, is strict: the user is letters, digits, `.`, `_` and `-` (not first), the
host a name of letters, digits, `.` and `-` starting with a letter or digit, or
a bracketed IPv6 literal. So no part reads as an ssh option, and a port
(`u@h:22`, which ssh, scp and the tier check would read differently) is
refused: put it in a `Host` entry of `~/.ssh/config`. ssh gets `--` before the
target, scp and rsync before their operands, and rsync runs with `-s`
(`--protect-args`), so the remote shell does not split or expand the remote
path. openrsync (macOS `/usr/bin/rsync`) has no `-s`: it gets the remote path
inside single quotes instead (a leading `~`, `~/` or `~NAME/` left out of
them), which the remote shell reads as one word; the characters that could
end the quotes or be expanded are already refused. Unknown options and extra arguments are refused. Remote paths of
`remote_push`, `remote_pull` and `remote_sync_*` are normalized (repeated `/`,
`.` segments, trailing `/`; `~/` becomes a path relative to the remote home,
`~root` becomes `/root` and `~user` becomes `/home/user`; `/.` is `/`), `..`
is refused, then:

| Tier | Paths | Interactive | Headless |
|---|---|---|---|
| Deny | `/` itself; `/etc/ssh`, `/etc/sudoers`, `/etc/sudoers.d`, `/etc/passwd`, `/etc/shadow`, `/etc/shadow-`, `/etc/gshadow`, `/etc/gshadow-`, `/etc/pam.d`, `/boot`, `/sys`, `/proc`, `/dev`, `/bin`, `/sbin`, `/lib`, `/lib64`, `/lib32`, `/libx32`, `/usr/bin`, `/usr/sbin`, `/usr/lib`, `/usr/lib64` and everything below each | Refused, no override | Refused, no override |
| Deny | A path the remote side may expand: `*` `?` `[` `]` `{` `}` `$` a backquote `;` `&` `\|` `<` `>` `(` `)` `!` a quote, a backslash, a newline, or a `~` past the start (`deny:metacharacters`) | Refused, no override | Refused, no override |
| Ask | `/etc`, `/var`, `/usr/local`, `/root`, and any path with a `.ssh` component (any home, any depth, any case) | y/N prompt, or `--force` | Refused unless `--force` |
| Allow | `/tmp`, `/var/tmp`, relative paths, everything else | Allowed | Allowed |

`--delete` on a sync runs without asking only below `/tmp`, `/var/tmp` or
`/home/<user>/`, and for relative paths inside the remote home. The home
itself, however it is written (`~`, `~user`, `/home/<user>`, `/Users/<user>`,
`/root`), and any `.ssh` path never count as safe; elsewhere it asks, or needs
`--force` in headless mode. For `remote_sync_pull` the same rule applies to
the local destination (a relative path is inside the current directory, never
your home itself), and the messages say so.
`remote_exec` runs any command on the remote host: the path tiers do not apply
to it.

## Agent toolkit (3.4)

The 3.4 functions keep every rail above and split reads from writes.
[AGENTS.md](../AGENTS.md) sections 6.6 and 8 hold the details.

* **`llm_*` only read**, with no exception. `llm_http` sends GET, HEAD or
  OPTIONS without a body; a credential reaches it only through
  `--auth-env VAR` (never argv, never printed, never sent to another host
  on a redirect). `llm_test` runs a test suite and nothing else;
  `llm_docker_exec` and `llm_wait --cmd-ok` run under the `llm_exec` rules;
  `llm_procs` has no kill.
* **`agent_*` are the write tools**: `agent_replace`, `agent_git_commit`,
  `agent_kill`, `agent_docker_exec`, `agent_put`. Each takes `--dry-run`,
  names every target (no glob is expanded, no implicit "all"), checks every
  target before the first write, and keeps the secret-file and
  forbidden-path rules. The deny tier of `remote_*` applies to local targets
  too (`write_path_denied`, even with `--force`); the ask tier needs
  `--force` (`sensitive_path`), except below your HOME or the project you
  work in. On macOS a target resolved into `/private/etc` or
  `/private/var` is judged as `/etc` or `/var`. `agent_git_commit` never force-pushes and has no `--amend` or
  `--no-verify`; `agent_kill` signals only your own processes, never PID 1,
  the caller or an ancestor; `agent_docker_exec` refuses commands that wipe
  or format a root, stop the machine or pipe a download into a shell.
* **Remote hosts.** `--host`, `llm_remote_run`, `llm_same` and `agent_put`
  take registered `HOST:` aliases only. Remote paths pass the local rules
  and the deny tier before anything connects, and again on the host after
  links resolve; a secret file never leaves the host and the local stdin is
  never sent. `llm_remote_run` checks each command with the `llm_exec`
  rules first. Over a `+ssh` database connection the SQL guard runs before
  ssh and the password goes on ssh's stdin. The gash binary Gash copies to a
  host is size-checked, test-run and renamed into a 0700 cache directory.
* **Telemetry** (`GASH_TELEMETRY_FILE`) is off by default and local: the
  function, time, output size, status and truncation of each call, never an
  argument, a path or a value.

## Confirmations

`needs_confirm_prompt` reads the answer from `/dev/tty`, accepts only `y` or
`Y`, and always refuses in headless mode or without a terminal. The 2.1
commands that show a plan first ask "Do you want to continue? [Y/n]" the same
way. Answering no returns 1. `port_kill`, `process_kill`, `services_stop`
and `node_upgrade` still accept the 2.0 `--force` as a synonym of `-y`.

| Function | Interactive | Headless |
|---|---|---|
| `port_kill PORT` | Lists the listening processes and asks; `-y` skips | Refused unless `-y` |
| `process_kill NAME` | Lists the matches and asks, even for one; `-y` skips | Refused unless `-y` |
| `services_stop` (alias `quit`) | Lists the running services and asks; `-y` skips | Refused unless `-y` |
| `docker_prune_all` | Counts what would go and asks; `-y` skips, `--dry-run` only shows | Refused unless `-y` |
| `docker_stop_all`, `docker_start_all` | List the containers (`Will stop N container(s): NAMES`) and ask; `-y` skips | Refused unless `-y` |
| `git_repair` | Shows the empty object files and the plan, asks; `-y` skips | Refused unless `-y` |
| `git_add_tag`, `git_delete_tag` | Show the tag and the origin URL (`Will push refs/tags/T to origin (URL)`) and ask; `-y`/`--yes` skips, `--dry-run` only shows | Refused unless `--yes` |
| `git_sync` | Shows what it commits and where it pushes (`Will commit N file(s) on B (M untracked) and push to U`) and asks; `-y`/`--yes` skips, `--dry-run` only shows | Refused unless `--yes` |
| `git_apply_patch` | Shows the checkout, pull and apply plan and asks; `-y`/`--yes` skips, `--dry-run` only shows | Refused unless `--yes` |
| `node_upgrade`, `chrome_headless_update` | Show the plan (versions, download size) and ask; `-y` skips | Refused unless `-y` |
| `wsl_restart`, `wsl_shutdown` | Ask before stopping every WSL distribution; `-y` skips | Refused unless `-y` |
| `wsl_compact` | Runs without asking (it changes no data) | Refused unless `-y` |
| `docker_compose_upgrade` | Shows the plan and asks; `--yes` skips | Refused unless `--yes` |
| `sudo_last` (no argument) | Shows the command and asks | Refused |
| `gash_uninstall` | Asks, then asks again before deleting `~/.gash_env` | Cancelled |

`gash_uninstall` also refuses a directory that resolves to `/` or your home,
or has no `gash.sh` and `lib/core`.

`services_stop`, `wsl_compact` and `chrome_headless_update` run sudo with `-n`
when headless or without a terminal, after a `sudo -n true` check: a sudo
that needs a password stops the command with `Error: sudo needs a password:
run interactively or configure NOPASSWD.` instead of waiting at a prompt
nobody sees. `chrome_headless_update` uses no sudo at all when its root and
link directories are writable.

## Credentials in ~/.gash_env

The file holds passwords and tokens in plain text: keep it private (Gash warns
when group or other users can read it). Parse warnings give the line number
and the kind of problem, never any part of the line; headless mode prints them
only with `GASH_DEBUG=1`. Parsed values
live in unexported shell variables (not inherited by child processes) that
`gash_unload` removes. Secrets never go on a command line, where other users
could read them in `/proc/<pid>/cmdline`: MySQL credentials use
`--defaults-extra-file`, SSH passphrases are typed to `ssh-add` on a
terminal Gash drives, and AI API keys stay inside the gash process.

## The per-shell server

On Linux the first call of a served function starts `gash __serve`, a
child of the shell that runs the shell's later calls of those functions
(`GASH_SERVER=0` turns it off). It holds no more than the exec'd binary
would: it runs as the shell's user, its private directory (the request and
reply files) is created `0700` under `XDG_RUNTIME_DIR`, `/dev/shm` or
`TMPDIR`, and it ends when the shell does. A request carries what the
binary would have received (the arguments, the exported variables, the
variables `__gash_export_state` passes, the umask); the server takes the
shell's working directory from `/proc/<pid>/cwd` and its standard streams
with `pidfd_getfd`, or reopens them through `/proc` when the kernel's
ptrace policy refuses that, and declines the call (the binary runs it, as
before) when a stream cannot be shared exactly. Ctrl-C reaches it with the
shell's process group and ends the call, and the server restarts when the
binary on disk changes.

## Install and upgrade

The installer and `gash_upgrade` check out the newest release tag. A signed tag
(PGP or SSH) is checked with `git verify-tag`; an unverifiable one gets a
warning. A tag counts as signed only when it is an annotated tag whose object
ends with a signature block: a lightweight tag (even on a signed commit) and a
message that only quotes the marker are unsigned.

The releases (from 3.1.0) are signed by the release workflow, not by a
person's key: an unsigned tag is checked through the Sigstore signature of its
release (`checksums.txt.sigstore.json`) when `cosign` is on `PATH`. The
signature's certificate names the workflow (`.github/workflows/release.yml`),
the tag and the commit, and cosign must accept it for the commit the tag
points at in the clone, so a verified signature covers the tree of the tag,
and with it `release/SHA256SUMS`. A signature that does not verify is a
warning. `GASH_REQUIRE_SIGNED=1` aborts unless the tag signature or the
release signature verifies (without `cosign` it says how to install it).
A new install is cloned and checked in a temporary directory next to the
install location and moved into place only when it passes, so a refusal leaves
nothing behind. `GASH_INSTALL_REF` skips this check. Both refuse a checkout with local changes.

The binary (`gash`, and `gash-hosts-helper`, which `gash hte install` makes a
root-owned sudo target) is verified before it is installed: against the
SHA-256 the tagged tree pins in `release/SHA256SUMS`, so a signed tag covers
the binaries too, else against the release's `checksums.txt`. With
`GASH_REQUIRE_SIGNED=1` only a pinned binary is accepted, and a helper the tree
does not pin is not installed. `gash_upgrade` (`gash update`) downloads and
verifies the new binary before it checks out the new tag, checks that it
answers its version, then loads the new tree in a test shell and checks out
the previous tag again when that fails.

## Verifying a release

Every release from 3.1.0 is built by `.github/workflows/release.yml` from its
tag, and ships with:

- `checksums.txt`, the SHA-256 of every archive and SBOM, and
  `checksums.txt.sigstore.json`, its keyless Sigstore signature: the
  certificate is issued to the workflow for that tag and commit, and the
  signature is recorded in the public Rekor transparency log;
- one SPDX SBOM per archive (`gash_<version>_<os>_<arch>.tar.gz.spdx.json`);
- SLSA build provenance for every archive and SBOM, stored by GitHub.

The binaries are also reproducible: `go run ./internal/tools/releasesums -check`
on the tagged tree rebuilds them and compares them with the hashes the tree
pins in `release/SHA256SUMS`.

To verify a download by hand (here 3.1.0 on linux/amd64):

```bash
v=3.1.0
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/mauriziofonte/gash/.github/workflows/release.yml@refs/tags/v$v" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum --ignore-missing -c checksums.txt
gh attestation verify "gash_${v}_linux_amd64.tar.gz" --repo mauriziofonte/gash
```

The installer and `gash update` do the first check themselves (see Install
and upgrade), adding the commit the tag points at.

## gash hte

`gash hte` runs Apache and PHP-FPM as your user. Root is used in two
places only.

**At install, uninstall and migrate**, through one `sudo` call that runs
the steps the command printed first, each recorded in a receipt that
uninstall plays back:

* the hosts helper, copied to `/usr/local/libexec/gash/gash-hosts-helper`,
  root-owned, mode 0755, after its SHA-256 is checked against the one the
  printed plan names;
* `/etc/sudoers.d/gash-hte`, checked with `visudo -c -f` before it is put
  in place, which lets your user run exactly `gash-hosts-helper apply` and
  `gash-hosts-helper check` without a password, and nothing else;
* `net.ipv4.ip_unprivileged_port_start = 80` in
  `/etc/sysctl.d/50-gash-hte.conf`, so that the stack can listen on ports
  80 and 443 without root. **Every local user** can then bind ports 80 to
  1023 while HTE is installed; on a shared machine, install with
  `--ports high` (8080 and 8443), which skips this step. On macOS a `pf`
  anchor redirects loopback ports 80 and 443 to the stack's ports instead,
  checked with `pfctl -n` and loaded at boot by a LaunchDaemon;
* the autostart unit, when one is asked for, and stopping the system Apache
  when `migrate --apply` takes its sites over.

**At runtime, the hosts helper.** It takes no path and ignores its
environment: the file it writes is `/etc/hosts`, a constant of the binary,
and it edits only the lines between `# >>> gash hte >>>` and
`# <<< gash hte <<<`. It accepts only `127.0.0.1` and `::1` as addresses and
only lowercase hostnames of at least two labels, with no wildcard and no
numeric last label (which a resolver would read as an address). Input over
its size limits, or a hosts file whose markers are broken, is refused and
nothing is written.

The rest runs as you:

* Apache listens on the loopback addresses only (`127.0.0.1`, and `[::1]`
  where the machine has IPv6 loopback): the sites are not reachable from the
  network.
* Configuration, certificates and logs live in your XDG directories, never
  inside a document root.
* Each site has a self-signed ECDSA P-256 certificate; its directory is
  mode 0700 and its key 0600.
* Configuration that gash writes is regenerated from its state; your own
  edits go in overlay files it never rewrites.

## Reporting a vulnerability

Do not open a public issue. Follow [SECURITY.md](../SECURITY.md) at the
repository root, or use a GitHub private security advisory:
<https://github.com/mauriziofonte/gash/security/advisories/new>.
