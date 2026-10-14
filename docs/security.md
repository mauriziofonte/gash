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

`llm_exec "LINE"` runs in a child `bash --noprofile --norc` with `BASH_ENV` and
`ENV` cleared, never in the caller's shell, after three checks:

1. **Blocklist** (always on): the whitespace-collapsed line must not match
   `rm -rf /` and variants, `dd if=`, `mkfs.`, `> /dev/sd*`, `chmod 777 /`,
   `shutdown`, `reboot`, `halt`, fork bombs, `curl ... | sh`, `history -c`,
   `HISTFILE=`, `cat /etc/shadow`, `.ssh/id_`, `AWS_SECRET`, `PRIVATE_KEY`.
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
| `sed`, `yq` | `-i`, `--in-place` |
| `git` | global options other than `-C`, `--no-pager` and a few read-only ones (so no `-c`); subcommands outside a read-only list; `--output`, `--ext-diff`, `--textconv`; creating or deleting branches and tags; `remote` other than `show`/`get-url`; `config` without `--get`/`--list`; `stash` other than `list`/`show` |
| `docker` | subcommands other than `ps`, `images`, `logs`, `inspect`, `stats`, `top` and other read-only ones |
| `systemctl`, `journalctl` | anything but `status`, `show`, `cat`, `is-*`, `list-*`; `journalctl --vacuum*`, `--rotate`, `--flush` |
| `php`, `node`, `python`, `ruby`, `java`, `composer` | anything but `--version` |
| `crontab`, `date`, `hostname`, `ip` | anything but `crontab -l`; setting the date or hostname; `ip` other than show/list/get |

Arguments naming a secret file (see below), also as `NAME=path`, are refused.
Unquoted globs are expanded first; a match that is a secret file or starts with
`-` is refused. In the child shell, functions shadowing allowed commands are
removed, `sed` runs as GNU `sed --sandbox`, `awk` as `gawk --sandbox` (both
refused where unavailable), and `git` as described in [Git](#git).

Settings, read from `~/.gash_env` only (never from the environment):

* `GASH_LLM_EXEC_ALLOW="make npm"`: extra commands with any arguments, without
  per-command rules; listing `sed`, `awk`, `gawk` or `git` drops its wrapper.
* `GASH_LLM_EXEC=unrestricted`: skips checks 2 and 3 (secret-file checks
  included) and the wrappers. Only the blocklist remains.
* `GASH_LLM_SANDBOX=1`: runs under `bwrap --ro-bind / / --dev /dev --proc /proc
  --unshare-all --die-with-parent`, so a read-only filesystem and no network;
  reading stays possible. Without a working `bwrap`: `sandbox_unavailable`.

Every argument is also checked against the [forbidden paths](#forbidden-paths):
option values (`--file=/proc/x`, `-f/proc/x`), `key=value` words, `~` and
`~user`, glob matches, and relative paths after a `cd` earlier in the line.
Not checked: `/dev/null`, the arguments of `echo`, `printf`, `basename` and
`dirname`, and the pattern operand of `grep`, `rg`, `sed`, `awk`, `jq` and
`yq`. An allowed command can still read any other file your user can read
whose name is not secret.

## llm_db_*

Read-only is enforced twice, so one parser mistake does not become a write.
First a **SQL guard** lexes the query with the target engine's quoting rules
(including PostgreSQL `E''` and dollar quotes), ignoring literals and comments.
It accepts one statement (a trailing `;` is fine) starting with `SELECT`,
`WITH`, `VALUES`, `TABLE`, `SHOW`, `DESCRIBE`, `DESC`, `EXPLAIN` or `PRAGMA`,
and refuses write keywords anywhere (`INSERT`, `UPDATE`, `DELETE`, `DROP`,
`CREATE`, `ALTER`, `INTO`, `OUTFILE`, `LOAD DATA`, `COPY`, ...), file and
program functions (`LOAD_FILE`, `PG_READ_FILE`, `LO_IMPORT`, `DBLINK`, ...),
`SLEEP`, `BENCHMARK`, client commands (a backslash outside a literal), MySQL
`/*! */` comments, SQLite dot-commands and lines holding only `go` or `/`.
Then the **session is read-only**:

| Engine | Session | Credentials |
|---|---|---|
| SQLite (`-f FILE`) | `sqlite3 -readonly -bail`, plus `-safe` on 3.37+; SQL on stdin | The file must pass the forbidden-path check and start with `SQLite format 3` |
| MySQL, MariaDB | `SET SESSION TRANSACTION READ ONLY`; rows capped with `sql_select_limit` | `--defaults-extra-file` on a process substitution, never argv |
| PostgreSQL | `default_transaction_read_only=on`, `statement_timeout` from `GASH_DB_TIMEOUT` (30 s), 10 s connect timeout, `psql -X`; SQL on stdin | `PGPASSWORD` in the environment of `psql` only |

Row caps are applied by the engine (outer `LIMIT` or `sql_select_limit`):
`llm_db_query -r N` (default 100), `llm_db_sample --limit N` (default 5). Table
names must match `[A-Za-z_][A-Za-z0-9_]*`. `llm_db_explain --analyze` runs the
query, in the same read-only session.

Nothing stops a query from reading what the database account can see: give
Gash an account with read-only grants.

## Secret files

A path is secret when its base name matches, ignoring case, one of:

| Kind | Patterns |
|---|---|
| Environment files | `.env` `.env.*` `*.env` (not `.env.example`, `.env.sample`, `.env.dist`, `.env.template`, `*.env.example`) |
| Keys and keystores | `*.pem` `*.key` `*.p12` `*.pfx` `*.jks` `*.keystore` `*.kdbx` `*.ppk` `id_rsa*` `id_dsa*` `id_ecdsa*` `id_ed25519*` `*_rsa` `*_dsa` `*_ecdsa` `*_ed25519` (not `*.pub`) |
| Credential files | `credentials` `credentials.*` `secrets` `secrets.*` `.gash_env` `.pgpass` `.netrc` `_netrc` `.git-credentials` `.npmrc` `.pypirc` `.htpasswd` `.my.cnf` `.boto` `.s3cfg` `.dockercfg` `.vault-token` `kubeconfig` `*.kubeconfig` `*.tfstate` `*.tfstate.*` `*.tfvars` |
| Directories | anything under `.ssh`, `.gnupg`, `.aws`, `.azure`, `.kube`, `.docker`, `.config/gcloud` |

A symlink is judged by its own name and by its target's.

| Function | Effect |
|---|---|
| `llm_exec` | Refuses secret arguments and glob matches (not in `unrestricted` mode) |
| `llm_config` | Refuses a secret file |
| `llm_grep`, `llm_find --contains` | Refuse a secret path; skip secret files and directories in recursive searches, with `rg` and with the `grep` fallback alike |
| `llm_tree`, `llm_find`, filesystem scanners | List names and sizes only, secret files included |

## Forbidden paths

`/proc`, `/sys`, `/dev`, `/boot`, `/root`, `/etc/shadow`, `/etc/gshadow`,
`/etc/passwd`, `/etc/sudoers`, `/etc/sudoers.d` and everything below them,
matched per path component (`/rootfs` is allowed). A `..` component is refused.
The path is checked as written, after `realpath -m` (symlinks resolved) and,
for macOS, without a `/private` prefix. Applies to `llm_tree`, `llm_find`,
`llm_grep`, `llm_config`, `llm_git_*`, `llm_project`, `llm_deps`, SQLite
files of `llm_db_*` and the arguments of `llm_exec`. The filesystem scanners (`files_largest`, `dirs_largest`,
`dirs_find_large`, `dirs_list_empty`, `tree_stats`) refuse `/proc`, `/sys`,
`/dev`, `/boot` and `/root` (as written and resolved), and `/` without
`--allow-root`.

## Git

A cloned repository can carry configuration that names programs: an
fsmonitor, hooks, an external diff, textconv drivers, a pager. `llm_git_*` and
the `git` of `llm_exec` run with `-c core.fsmonitor=false -c
core.hooksPath=/dev/null -c diff.external= -c core.pager=cat`, and diff or log
with `--no-ext-diff --no-textconv` (log also `--no-show-signature`). Clean and
smudge filters named in `.gitattributes` and defined in the repository's
configuration can still run during `status` and `diff`: inspect an untrusted
repository's `.git/config` before pointing an agent at it.

## AI module

* **Redaction.** The text sent (question, piped input, collected system data)
  is filtered first: private key blocks, URL passwords, values of names like
  `password`, `secret`, `token`, `api_key`, `access_key`, `private_key`,
  `credentials`, and `AKIA...`, `ghp_`/`github_pat_`, `sk-`/`sk-ant-`, `xox?-`,
  `AIza...` tokens. It is pattern based: a secret in another format is sent.
* **Transport.** The body goes through a temporary file created with umask 077
  and then deleted; the API key reaches `curl` through `--config` on a pipe,
  never argv. Control characters are stripped from replies.
* **Consent.** `GASH_AI_CONFIRM=auto` (default) asks before sending data Gash
  collected (`ai_sysinfo`) and sends what you typed or piped; `1` always asks;
  `0` never. Where consent is needed, no terminal or headless mode means
  refusal. Headless mode reads it from `~/.gash_env` too; the environment
  wins. `GASH_AI_DEBUG=1` prints the (redacted) request
  bodies and raw replies on stderr.

## remote_*

In headless mode only aliases registered as `HOST:alias=user@host` are
accepted; interactively a raw `user@host` works too. Remote paths of
`remote_push`, `remote_pull` and `remote_sync_*` are normalized (repeated `/`,
`.` segments, trailing `/`; `~/` becomes a path relative to the remote home,
`~root` becomes `/root` and `~user` becomes `/home/user`), `..` is refused, then:

| Tier | Paths | Interactive | Headless |
|---|---|---|---|
| Deny | `/`, `/etc/ssh`, `/etc/sudoers*`, `/etc/passwd`, `/etc/shadow*`, `/etc/gshadow*`, `/etc/pam.d`, `/boot`, `/sys`, `/proc`, `/dev`, `/bin`, `/sbin`, `/lib*`, `/usr/bin`, `/usr/sbin`, `/usr/lib`, `/usr/lib64` | Refused, no override | Refused, no override |
| Ask | `/etc`, `/var`, `/usr/local`, `/root`, and any path with a `.ssh` component (any home, any depth, any case) | y/N prompt, or `--force` | Refused unless `--force` |
| Allow | `/tmp`, `/var/tmp`, relative paths, everything else | Allowed | Allowed |

`--delete` on a sync runs without asking only below `/tmp`, `/var/tmp` or
`/home/<user>/`, and for relative paths inside the remote home. The home
itself, however it is written (`~`, `~user`, `/home/<user>`, `/Users/<user>`,
`/root`), and any `.ssh` path never count as safe; elsewhere it asks, or needs
`--force` in headless mode.
`remote_exec` runs any command on the remote host: the path tiers do not apply
to it.

## Confirmations

`needs_confirm_prompt` reads the answer from `/dev/tty`, accepts only `y` or
`Y`, and always refuses in headless mode or without a terminal.

| Function | Interactive | Headless |
|---|---|---|
| `port_kill PORT` | Lists the listening processes and asks; `--force` skips | Refused unless `--force` |
| `process_kill NAME` | Asks only when several processes match; `--force` skips | Refused unless `--force`, whatever the number of matches |
| `services_stop` | Asks; `--force` skips (the `quit` alias passes `--force`) | Nothing happens unless `--force` |
| `docker_prune_all` | Asks, no override | Nothing happens |
| `docker_compose_upgrade` | Shows the plan and asks; `--yes` skips | Refused unless `--yes` |
| `sudo_last` (no argument) | Shows the command and asks | Refused |
| `gash_uninstall` | Asks, then asks again before deleting `~/.gash_env` | Cancelled |

`gash_uninstall` also refuses a directory that resolves to `/` or your home,
or has no `gash.sh` and `lib/core`.

## Credentials in ~/.gash_env

The file holds passwords and tokens in plain text: keep it private (Gash warns
when group or other users can read it). Parse warnings give the line number
and the kind of problem, never any part of the line; headless mode prints them
only with `GASH_DEBUG=1`. Parsed values
live in unexported shell variables (not inherited by child processes) that
`gash_unload` removes. Secrets never go on a command line, where other users
could read them in `/proc/<pid>/cmdline`: MySQL credentials use
`--defaults-extra-file`, SSH passphrases reach `expect` on stdin, API keys
reach `curl` through `--config`.

## Install and upgrade

The installer and `gash_upgrade` check out the newest release tag. A signed tag
(PGP or SSH) is checked with `git verify-tag`; an unverifiable one gets a
warning. `GASH_REQUIRE_SIGNED=1` aborts on an unsigned or unverifiable tag.
`GASH_INSTALL_REF` skips this check. Both refuse a checkout with local changes.

## Reporting a vulnerability

Do not open a public issue. Follow [SECURITY.md](../SECURITY.md) at the
repository root, or use a GitHub private security advisory:
<https://github.com/mauriziofonte/gash/security/advisories/new>.
