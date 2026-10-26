# Configuration

Gash reads its configuration from a few files in your home directory and
from environment variables. This page lists the files, the order in which
an interactive shell loads them, the grammar of `~/.gash_env` and every
`GASH_*` setting.

Platforms: Linux, macOS and WSL; the files and settings are the same on
each.

## Files

| Path | Written by | Purpose |
|---|---|---|
| `~/.gashrc` | the installer, once; never overwritten | The loader: sources `gash.sh` from the install directory, then the completion in interactive shells |
| `~/.gash/` | the installer, `gash update` | The install: a git checkout of a release. Another location works: see `GASH_DIR` below |
| `~/.gash_env` | you (`gash_env_init` creates it) | SSH keys to unlock, database connections, AI tokens, remote hosts and settings |
| `~/.bash_aliases`, `~/.bash_local` | you | Your own aliases and shell code, loaded after Gash |
| `~/.local/share/gash/versions/VERSION/` | the installer, the first shell, `gash update` | The `gash` binary of each version (and `gash-hosts-helper`) |
| `~/.local/state/gash/` | Gash | `update` (the version `gash update --rollback` returns to), `schema` and `notice` (the Gash 1.x reminder), `hte/` (the rendered `gash hte` configuration and its logs) |
| `~/.cache/gash/` | Gash | `init-probes` (what the shell start learned about `ls`, `grep`, `rm` and `dircolors`, reused for a day), `runtimes.json` (`gash runtime` answers). A `probes` file there is a Gash 2.x leftover, safe to delete |
| `~/.config/gash/hte/` | `gash hte` | The sites, settings and overlay files of the [local web stack](hte/README.md) |

`XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME` and `XDG_CONFIG_HOME`
move the last four, as their names say.

The installer adds this block to `~/.bashrc` (or the profile it finds),
and `gash_uninstall` removes it:

```bash
# >>> GASH START >>>
# Load Gash Bash - Do not edit this block, it is managed by Gash installer
if [ -f "$HOME/.gashrc" ]; then
  source "$HOME/.gashrc"
fi
# <<< GASH END <<<
```

## How Gash loads

`~/.gashrc` sources `gash.sh` from `${GASH_DIR:-$HOME/.gash}` (the
installer writes a custom location into that line). `gash.sh` sets
`GASH_DIR` to the directory it lives in and `GASH_VERSION` to its version.
It refuses a Bash older than 4.4, and in a non-interactive shell it does
nothing unless `GASH_HEADLESS=1` is set. There are two modes.

**Interactive** (a terminal), in this order:

1. `gash.sh` records the shell state that `gash_unload` restores.
2. The core libraries, every function, and the short aliases of the
   modules (`flf`, `gsync`, `dcc`, ...).
3. `gash init bash`, from the binary: the settings of `~/.gash_env` (see
   [Precedence](#precedence)), terminal settings and the umask, history,
   the prompt, `GASH_PATH_ADD` and `GASH_NVM`, the alias packs, the
   banner, the unlock of the SSH keys of `~/.gash_env`, the Gash 1.x
   reminder, and the `gash hte` start when its autostart is `shell`.
4. `~/.bash_aliases` (unless your `~/.bashrc` already sources it), then
   `~/.bash_local`.
5. `~/.gashrc` sources the completion.

When the binary of this version is missing, the first interactive shell
downloads it and prints one line saying so.

**Headless** (`GASH_HEADLESS=1`): the core libraries and the functions
only, and from `~/.gash_env` only the settings functions need (see
[Precedence](#precedence)). No prompt, history settings, aliases, banner,
completion, `~/.bash_aliases`, `~/.bash_local` or colors, and nothing is
printed while loading. A missing binary is downloaded at the first call
instead, without a word unless it fails.

`gash_doctor --timing` shows what each of these steps costs in a new shell.

### Aliases

A Gash alias never hides a program: when an alias name of the alias packs
or a module short alias (`dcc`, `pk`, `wr`, ...) is also a program on your
`PATH` (`gp` and PARI/GP, `gc` and Graphviz, `dc` and the calculator, `gs`
and Ghostscript, `sl` and the steam locomotive), the alias is not defined
and the program runs. The aliases that wrap the program of the same name on
purpose stay (`ls`, `cp`, `mv`, `rm`, `mkdir`, `grep`, `bc`, `reboot`,
`shutdown`, and the `GASH_REPLACE_TOOLS` ones), and so does an alias you
defined before Gash loaded. The `hg` alias of `history_grep` follows the
same rule when Mercurial is installed. To get a dropped alias back, define
it in `~/.bash_aliases`.

Which options `ls`, `grep` and `rm` accept, the `LS_COLORS` of `dircolors`
and the list of dropped aliases are worked out when a shell starts. The
answers are kept in `~/.cache/gash/init-probes` for a day, and worked out
again sooner when `PATH`, `TERM`, the locale or the `dircolors` database
change. The [shell guide](shell/README.md) describes the alias packs.

## ~/.gash_env

### Create it

```bash
gash_env_init            # copies the template to ~/.gash_env, mode 600
gash_env_init --force    # overwrites an existing file
```

<!-- example -->
```console
$ gash_env_init
OK: Created ~/.gash_env
Info: Edit with your SSH keys and database credentials
$ stat -c '%a %n' ~/.gash_env
600 ~/.gash_env
```

With `GASH_ENV_FILE` set, `gash_env_init` writes that file instead.

### Grammar

The file is read line by line. Spaces and tabs around a line (ASCII
whitespace, in every locale) and a trailing carriage return are ignored; a
line padded with a non-ASCII space such as U+00A0 is not recognized. Empty
lines and lines starting with `#` are skipped. Every other line must be one
of these forms:

| Form | Example |
|---|---|
| `SSH:keypath=passphrase` | `SSH:~/.ssh/id_ed25519=my passphrase` |
| `DB:name=driver://user:password@host:port/database` | `DB:default=mysql://app:s3cret@localhost:3306/shop` |
| `AI:provider=token` | `AI:claude=sk-ant-api03-...` |
| `HOST:alias=user@host` | `HOST:web=deploy@web.example.com` |
| `GASH_NAME=value` | `GASH_PROMPT=0` |

Rules that apply to every form:

* The name ends at the first `=`. For `SSH:`, `DB:`, `AI:` and `HOST:`
  entries everything after it is the value, taken literally (spaces and `#`
  included), so a password may contain `#`. Whitespace at the start of a
  line and at the end of a `DB:`, `AI:` or `HOST:` value is dropped; an
  `SSH:` passphrase keeps it.
* `GASH_*` settings accept an inline comment: `GASH_BANNER=0  # quiet` sets
  `0`. A `#` counts as a comment only after whitespace; inside quotes it is
  part of the value. One pair of matching quotes around the value is
  removed (`GASH_GIT_EXCLUDE="/srv/a:/srv/b"`). The template's lines work as
  they are once uncommented.
* When two entries share a name, the first one wins. A later `SSH:`, `DB:`
  or `HOST:` entry with the same name is ignored with a warning, for example
  `Warning: Line 7 of ~/.gash_env: duplicate DB:default, ignored (first is at line 2)`.
* A line that does not match is skipped with a warning on stderr that names
  the file, the line number and the kind of problem, never any part of the
  line (only the duplicate warning names the entry, never its value):

<!-- example
printf 'GASH_BANNER=0\nGASH_PROMTP=0\nDB:x=postgres://app@localhost/shop\n' >.gash_env
chmod 600 .gash_env
-->
```console
$ gash_db_list
Warning: Line 2 of ~/.gash_env: unknown GASH_ setting, skipping
Warning: Line 3 of ~/.gash_env: DB URL must start with mysql://, mariadb:// or pgsql:// (skipping)
Info: No database connections configured in ~/.gash_env
Info: Run 'gash_env_init' to create a template configuration
```

The warnings appear once per shell: the first time a function reads the
file (`gash_db_list`, `remote_*`, `llm_db_*`, `ai_*`), or when the shell
starts if the file has `SSH:` keys. Headless mode prints them only with
`GASH_DEBUG=1`. [troubleshooting.md](troubleshooting.md#warnings-about-gash_env)
lists each warning and its fix.

### SSH keys

`SSH:keypath=passphrase`. A leading `~/` (or `~` alone) stands for your home
directory; a `~user` path is skipped with a warning (write the full path). The
key file must exist, otherwise the entry is skipped with a warning. An empty
passphrase is allowed. The passphrase is everything after the `=` up to the
end of the line (a Windows line ending excepted), trailing spaces and tabs
included; such a line gets
`Warning: Line N of ~/.gash_env: passphrase ends with whitespace`, so an
accidental space is visible. In interactive shells `gash_ssh_auto_unlock`
starts `ssh-agent` when none is running and adds each key through `expect`
(required), once per session: it exports `GASH_SSH_AUTOUNLOCK_RAN=1`, so
child shells skip it. See the [remote hosts guide](remote/README.md).

### Databases

`DB:name=driver://user:password@host:port/database`

* Drivers: `mysql`, `mariadb`, `pgsql`. Any other scheme is skipped.
* Port defaults to 3306 (mysql, mariadb) or 5432 (pgsql) when omitted.
* The database is optional. `llm_db_*` picks it from `-d NAME`, then from the
  URL, then from the first word of a `.target-database` file in the current
  directory.
* The connection named `default` is used when `-c` is not given.
* Only the password is percent-decoded. Write `%` as `%25`; other
  characters may be written raw or URL-encoded (`p@ss` and `p%40ss` are the
  same password). The user name is used as written (not decoded). The
  decoded password is used whole, trailing newlines included; one that holds
  `%00` (a NUL byte) is refused with `invalid_db_url`.
* An IPv6 host goes in brackets: `[::1]:5432`, or `[::1]` for the default
  port. A bracket that is not closed is refused with `invalid_db_url`.

`gash_db_list` masks passwords; `gash_db_test [NAME]` runs `SELECT 1` on one.
No `mysql`, `mariadb` or `psql` client is needed and none is run: Gash
connects through its own drivers, so client option files (`~/.my.cnf`,
`/etc/mysql/*.cnf`) and `~/.pgpass` are not read. A MySQL `localhost` host
connects through the first existing of `/run/mysqld/mysqld.sock`,
`/var/run/mysqld/mysqld.sock` and `/tmp/mysql.sock`, else over TCP. The test
gives up after `GASH_DB_TIMEOUT` seconds (default 30), and a failed test
prints the driver's error as a `Hint:` line after `Error: Connection failed`.

A scheme ending in `+ssh` reaches a database on a registered host:
`mysql+ssh://USER:PASS@ALIAS/DB`, `mariadb+ssh://...`,
`pgsql+ssh://USER:PASS@ALIAS[:PORT]/DB`, `sqlite+ssh://ALIAS/relative.db`
or `sqlite+ssh://ALIAS//absolute.db`, any of them with `+docker:CONTAINER`
at the end. ALIAS is a `HOST:` alias. The `llm_db_*` functions then run the
engine's own client on the host (`mysql`, `psql` 12 or newer, `sqlite3`
with `-json`), one ssh round trip per query, after the same SQL guard; the
password goes on ssh's stdin, never in an argument, and one holding a
newline is refused. `gash_db_test` does not test these connections. See
the [databases guide](databases/README.md).

### AI providers

`AI:provider=token`, with provider `claude` or `gemini`. An empty token is
skipped. The first provider listed is the default for `ai_ask` (alias `ask`),
`ai_query` and `ai_sysinfo`. `gash_ai_list` shows only a token's first 8
characters. See the [AI guide](ai/README.md).

### Remote hosts

`HOST:alias=user@host`. The user is letters, digits, `.`, `_` and `-` (not
first); the host is a name of letters, digits, `.` and `-` starting with a
letter or digit, or an IPv6 address in brackets (`deploy@[2001:db8::1]`). Any
other target, such as `u@h:2222` or `-oOption@h`, is skipped at load with a
warning. For a non-standard port or key, define the host in
`~/.ssh/config` and use that name as `host`. The `remote_*` functions
accept the alias, and so do `--host ALIAS` on the `llm_*` reads,
`llm_remote_run`, `llm_same` and `agent_put`. In headless mode only
registered aliases are accepted. `remote_hosts` (alias `rh`) lists them.
See the [remote hosts guide](remote/README.md).

### Settings

`GASH_NAME=value`, with a name from the [settings reference](#settings-reference).
Unknown names are skipped with a warning.

### Permissions

Keep the file private: `chmod 600 ~/.gash_env`. Whenever Gash reads it, it
warns when group or other users have any access (`Warning: ~/.gash_env is
accessible by other users (mode 644). Run: chmod 600 ~/.gash_env`) or when
you do not own it, then reads it anyway (headless: only with
`GASH_DEBUG=1`). Symlinks are followed. `gash_doctor` checks the mode too.

## Precedence

For the settings in the reference below:

* **Interactive mode:** a variable that is already set when Gash loads
  (exported in the environment, or assigned in `~/.bashrc` before the Gash
  block) wins over `~/.gash_env`. `GASH_PROMPT=0 bash` gives a one-off
  shell with your own prompt.
* **Headless mode:** only the settings functions use are applied:
  `GASH_DB_TIMEOUT`, `GASH_AI_CONFIRM`, `GASH_AI_MODEL_CLAUDE` and
  `GASH_AI_MODEL_GEMINI`. The environment still wins over the file.
* **File only:** `GASH_LLM_EXEC`, `GASH_LLM_EXEC_ALLOW` and
  `GASH_LLM_SANDBOX` are read from `~/.gash_env` at each call, in both
  modes, never from the environment, so an agent cannot loosen `llm_exec`
  by exporting a variable (whoever controls `GASH_ENV_FILE` chooses the
  file, though). A change to them applies at once.

Settings changed in `~/.bash_aliases` or `~/.bash_local` come too late for
most of them (the prompt, history and aliases are already set up). Use
`~/.gash_env`, or set them before the Gash block in `~/.bashrc`, then open
a new terminal.

## Settings reference

These are the settings `~/.gash_env` accepts. Any of them may also be set
in the environment (but the three file-only ones).

| Setting | Default | Effect |
|---|---|---|
| `GASH_PROMPT` | `1` | `0` keeps your own `PS1`. The prompt hook still runs for history and the last exit status. Names the prompt shows (git branch, virtualenv, `debian_chroot`, screen session) appear literally: `$(...)`, backticks or `${...}` in them are never run. Control characters in the terminal title (the directory, user and host names) are shown as `?` |
| `GASH_GIT_EXCLUDE` | empty | Colon-separated repository roots with no git segment in the prompt. Absolute paths, matched exactly (`~` is not expanded). `$HOME` as a root is always skipped |
| `GASH_BANNER` | `1` | `0` hides the startup banner and quote |
| `GASH_SHOW_INXI` | `0` | `1` prints a short `inxi` summary at startup when `inxi` is installed |
| `GASH_HISTORY` | `1` | Gash manages history: unlimited size, timestamps, `histappend`, each command written at once, and `ignoreboth` added to `HISTCONTROL` unless you set your own (`ignorespace`, `ignoredups` or `ignoreboth`; another value such as `erasedups` gets `:ignoreboth` appended). `0` leaves history alone |
| `GASH_HISTORY_SYNC` | `0` | `1` re-reads the history file before each prompt, to see commands from other terminals |
| `GASH_SAFETY_ALIASES` | `1` | `cp -iv`, `mv -iv`, `mkdir -pv`, and `rm -Iv --one-file-system --preserve-root` plus `--preserve-root` for `chown`, `chmod`, `chgrp` with GNU tools (`rm -iv` otherwise). `0` turns them off |
| `GASH_REPLACE_TOOLS` | `0` | `1` aliases `less` to `most` (and sets `PAGER`), `tail` to `multitail`, `df` to `pydf`, `top` to `htop`, `traceroute` and `tracepath` to `mtr`, `diff` to `colordiff` when installed, and `php`/`composer` to the newest `phpX.Y` |
| `GASH_UMASK` | empty | An octal mask of 3 or 4 digits, such as `027`, applied at startup. Empty keeps the system umask; any other value is ignored with a warning |
| `GASH_PATH_ADD` | empty | Directories put first on `PATH`, in the order given, when they exist (`~` is expanded), e.g. `"~/.local/bin ~/.cargo/bin ~/.config/composer/vendor/bin"`. Separated by spaces; no wildcards (a `*` is taken literally). Directories only, no options: a word starting with `-` is a directory name. Re-sourcing never adds them twice |
| `GASH_NVM` | `off` | `lazy` puts `node`, `npm`, `npx` and the global packages of nvm's default version on `PATH` at once and loads `nvm.sh` the first time `nvm` runs: it saves the time that sourcing `nvm.sh` adds to every shell. `on` (or `1`) sources `nvm.sh` and its completion at startup. `off` leaves nvm to your own files. `NVM_DIR` defaults to `~/.nvm` |
| `GASH_DB_TIMEOUT` | `30` | PostgreSQL statement timeout of `llm_db_*`, and how long `gash_db_test` waits, in seconds. Not applied to MySQL, MariaDB or SQLite queries |
| `GASH_LLM_EXEC` | empty | `unrestricted` skips the `llm_exec` parser and allowlist. The blocklist of destructive patterns still applies. File only |
| `GASH_LLM_EXEC_ALLOW` | empty | Extra commands for `llm_exec`, separated by spaces, commas or colons (`"make npm"`). They run with any arguments. File only |
| `GASH_LLM_SANDBOX` | `0` | `1` runs `llm_exec` inside `bwrap`: read-only filesystem, no network, and under WSL no Windows programs. Fails with `sandbox_unavailable` when `bwrap` is missing. File only |
| `GASH_AI_CONFIRM` | `auto` | `auto` asks before sending data Gash collected (`ai_sysinfo`); `1` asks before every request; `0` never asks. The value is case-sensitive: any other value (`off`, `no`, `AUTO`) asks before every request, with a warning, so an unattended send always needs an explicit `0` |
| `GASH_AI_MODEL_CLAUDE` | `claude-haiku-5-5` | Model for Claude requests. Letters, digits, `.`, `_` and `-` only, starting with a letter or digit; any other value is an error and nothing is sent |
| `GASH_AI_MODEL_GEMINI` | `gemini-2.5-flash` | Model for Gemini requests (it is part of the request URL). Same rule as `GASH_AI_MODEL_CLAUDE` |

The `GASH_LLM_*` settings also apply to `llm_wait --cmd-ok`,
`llm_docker_exec`, `gash run --safe` and `llm_remote_run` (the sandbox
aside). See [security.md](security.md) for what the `llm_exec` and AI
settings change, and the [agents guide](agents/README.md#decide-what-agents-may-run)
for how they bound an agent.

## Environment-only variables

These are read from the environment and are not accepted in
`~/.gash_env`.

| Variable | Effect |
|---|---|
| `GASH_HEADLESS=1` | Headless mode (see above). Only the value `1` counts |
| `NO_COLOR`, `GASH_NO_COLOR` | Turn ANSI colors off in Gash output and in the prompt (from the next prompt). `NO_COLOR`: any non-empty value; `GASH_NO_COLOR`: any value but empty or `0` |
| `GASH_DEBUG=1` | Shows what files print while a headless shell loads, the `~/.gash_env` warnings in headless mode, the `Debug:` lines of functions, and a stack trace with an `internal_error` |
| `GASH_TIMING=1` | When a shell starts: records how long each file and step takes, in the `__GASH_LOAD_TIMES` array (Bash 5.0+). `gash_doctor --timing` shows the same in a new shell |
| `GASH_AI_DEBUG=1` | Prints the (redacted) AI request bodies and raw responses on stderr |
| `GASH_AWK` | awk program for the functions that need specific awk features. Default: `gawk` when installed, else `awk`. Read when Gash loads |
| `GASH_TUI=plain` | Numbered prompts instead of arrow-key menus |
| `GASH_ENV_FILE` | Use this file instead of `~/.gash_env` (the loader, every function that reads it, `llm_exec`, `gash_doctor`, `gash_env_init`, `gash_uninstall`) |
| `GASH_DIR` | Install location used by `~/.gashrc`, the installer, `gash_upgrade` (`gash update`), `gash_uninstall` and `gash_doctor`. `gash.sh` sets it to its own directory |
| `GASH_REQUIRE_SIGNED=1` | The installer and `gash_upgrade` (`gash update`) refuse a release unless its tag signature, or the Sigstore signature of its release, verifies (the latter needs `cosign` on `PATH`), and every binary download accepts only a binary the tagged tree pins in `release/SHA256SUMS` |
| `GASH_RELEASE_URL` | Base URL of the release assets for every binary download (the shell, the installer, `gash update`); default the GitHub release of the version; `file://` works |
| `GASH_BIN` | A gash binary to use as it is, instead of the one under `versions/` (development) |
| `GASH_SERVER=0` | Turns off the per-shell server. By default (Linux) the first call of a served function (the `llm_*` reads, the file and disk reports, `gash_help` and the like) starts `gash __serve`, one process per shell that runs that call and the later ones without starting the binary again, with the shell's environment, directory and standard streams, and that ends with the shell. A call it cannot run the same way (in a subshell or `$(...)`, with a stream it cannot share, after `TZ` changed) starts the binary as usual. Read at each call |
| `GASH_TELEMETRY_FILE` | A file path: every `llm_*` and `agent_*` call appends one JSON line, `{"ts","fn","ms","out_bytes","rc","truncated"}`, to it (created 0600). Off when unset. Local only: never an argument, a path or a value, never the network; a write failure never fails the call. Export it so the calls see it |
| `GASH_WIN_TIMEOUT` | WSL only. How long one `gash win` call waits for the Windows side before it fails with `interop_timeout`. A whole number of seconds or a Go duration (`8s`, `500ms`); default `10s`, capped at `120s`. Read at each call |
| `GASH_CHROME_ROOT`, `GASH_CHROME_LINK` | Where `chrome_headless_update` installs (default `/opt/chrome-headless-shell`) and the symlink it maintains (default `/usr/local/bin/chrome-headless-shell`) |
| `GASH_INSTALL_GIT_REPO`, `GASH_INSTALL_REF`, `PROFILE` | Installer only: the repository to clone, a ref to check out instead of the latest release tag (no signature check then), the profile file to edit (`/dev/null` for none) |
| `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`, `XDG_CONFIG_HOME` | Parents of the Gash directories listed under [Files](#files) |
| `GASH_VERSION` | Set by Gash, not by you: the loaded version, for scripts |
| `GASH_SSH_AUTOUNLOCK_RAN` | Set by Gash, not by you: the SSH keys were unlocked in this session, so child shells skip it |

## Your own shell files

In interactive mode, after everything else, Gash sources `~/.bash_aliases`
and then `~/.bash_local`, at top level, so they can override any Gash alias
or function. Many distribution `~/.bashrc` files already source
`~/.bash_aliases` (a line such as `. ~/.bash_aliases`); Gash then does not
source it again, so it runs once, and its aliases still win because Gash
never replaces an alias you defined before it loaded. `~/.gashrc` is
created once by the installer and not overwritten later; keep your own code
in `~/.bash_local` rather than there.

## Check and undo

* `gash_doctor` lists the settings set in the shell and checks the mode of
  `~/.gash_env`; see the [Gash guide](gash/README.md#check-the-installation).
* `gash_unload` removes Gash from the current shell and restores what it
  changed there, settings included; nothing on disk changes. See the
  [Gash guide](gash/README.md#unload-gash-from-this-shell).
* `gash_uninstall` removes Gash, its profile block and `~/.gashrc`; see the
  [Gash guide](gash/README.md#remove-gash).
