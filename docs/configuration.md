# Configuration

Gash reads its configuration from a few files in your home directory and from
environment variables. This page lists them, their load order and every setting.

## Files at a glance

| Path | Written by | Purpose |
|---|---|---|
| `~/.gashrc` | installer (once, never overwritten) | Loader: finds Gash and sources `gash.sh`, then completion |
| `~/.gash/` | installer | The Gash checkout. Another location works, see `GASH_DIR` below |
| `~/.gash_env` | you (`gash_env_init` creates it) | SSH keys, database connections, AI tokens, remote hosts, settings |
| `~/.bash_aliases`, `~/.bash_local` | you | Your own aliases and shell code, loaded after Gash |
| `~/.local/state/gash/` | `gash migrate` | Migration state and backups (`$XDG_STATE_HOME/gash` when that is set) |
| `~/.cache/gash/probes` | Gash, at startup | What the alias files learned about your tools (see [Aliases](#aliases)); safe to delete (`$XDG_CACHE_HOME/gash` when that is set) |

The installer adds a block between `# >>> GASH START >>>` and `# <<< GASH END <<<`
to `~/.bashrc` (or the profile it detects) that sources `~/.gashrc`.

## How Gash loads

`~/.gashrc` sources `gash.sh` from `${GASH_DIR:-$HOME/.gash}` (the installer
writes a custom location into that line). `gash.sh` then sets `GASH_DIR` to the
directory it lives in. It does nothing in a non-interactive shell unless
`GASH_HEADLESS=1` is set. There are two modes:

**Interactive** (the normal shell), in this order:

1. Core libraries (output, help, utils, validation, config).
2. Settings from `~/.gash_env` are applied (see [Precedence](#precedence)).
3. Terminal settings, history policy, prompt.
4. Every function module in `lib/modules/`.
5. Gash aliases (`lib/aliases/`), then the banner, then SSH key auto-unlock.
6. `~/.bash_aliases`, then `~/.bash_local`.
7. The Gash 1.x migration check. `~/.gashrc` then sources `bash_completion`.

**Headless** (`GASH_HEADLESS=1`): core libraries, function modules and the
migration check only. No prompt, history policy, aliases, banner, completion,
`~/.bash_aliases`, `~/.bash_local` or colors. Only the settings functions
need are applied from `~/.gash_env`; see [Precedence](#precedence).

`gash_doctor --timing` shows what each of these steps costs in a new shell.

### Aliases

A Gash alias never hides a program: when an alias name from `lib/aliases/`
is also a program on your `PATH` (`gp` and PARI/GP, `gc` and Graphviz, `dc`
and the calculator, `gs` and Ghostscript), the alias is not defined and the
program runs. The aliases that wrap the program of the same name on purpose
stay (`ls`, `cp`, `mv`, `rm`, `mkdir`, `grep`, `bc`, `reboot`, `shutdown`,
`sl`, and the `GASH_REPLACE_TOOLS` ones), and so does an alias you defined
before Gash loaded. The `hg` alias of `history_grep` follows the same rule
when Mercurial is installed. To get a dropped alias back, define it in
`~/.bash_aliases`.

Which options `ls`, `grep` and `rm` accept, `dircolors`' `LS_COLORS` and the
list of dropped aliases are worked out once and kept in
`~/.cache/gash/probes`. Gash works them out again after a day, or sooner
when Gash, `PATH`, `TERM`, `COLORTERM` or `~/.dircolors` changes or a
program is installed in a `PATH` directory.

## ~/.gash_env

### Create it

```bash
gash_env_init            # copies $GASH_DIR/.gash_env.template to ~/.gash_env, mode 600
gash_env_init --force    # overwrites an existing file
```

With `GASH_ENV_FILE` set, `gash_env_init` writes that file instead.

### Grammar

The file is read line by line. Leading and trailing whitespace and a trailing
carriage return are removed. Empty lines and lines starting with `#` are
skipped. Every other line must be one of these forms:

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
  included), so a password may contain `#`.
* `GASH_*` settings accept an inline comment: `GASH_BANNER=0  # quiet` sets
  `0`. A `#` counts as a comment only after whitespace; inside quotes it is
  part of the value. The template's lines work as they are once uncommented.
* When two entries share a name, the first one wins.
* A line that does not match is skipped with a warning on stderr that names
  the file, the line number and the kind of problem, never any part of the
  line, for example
  `Warning: Line 4 of ~/.gash_env: DB URL must start with mysql://, mariadb:// or pgsql:// (skipping)`.
  Headless mode prints these warnings only with `GASH_DEBUG=1`.

### SSH keys

`SSH:keypath=passphrase`. A leading `~` in the path is expanded. The key file
must exist, otherwise the entry is skipped with a warning. An empty passphrase
is allowed. In interactive shells `gash_ssh_auto_unlock` starts `ssh-agent`
when none is running and adds each key through `expect` (required), once per
session: it exports `GASH_SSH_AUTOUNLOCK_RAN=1`, so child shells skip it.

### Databases

`DB:name=driver://user:password@host:port/database`

* Drivers: `mysql`, `mariadb`, `pgsql`. Any other scheme is skipped.
* Port defaults to 3306 (mysql, mariadb) or 5432 (pgsql) when omitted.
* The database is optional. `llm_db_*` picks it from `-d NAME`, then from the
  URL, then from the first word of a `.target-database` file in the current
  directory.
* The connection named `default` is used when `-c` is not given.
* Only the password is percent-decoded. Encode `%` as `%25` and, to be safe,
  `@` as `%40`, `:` as `%3A`, `/` as `%2F`, `#` as `%23`.

`gash_db_list` masks passwords; `gash_db_test [NAME]` runs `SELECT 1` on one.

### AI providers

`AI:provider=token`, with provider `claude` or `gemini`. An empty token is
skipped. The first provider listed is the default for `ai_ask` (alias `ask`),
`ai_query` and `ai_sysinfo`. `gash_ai_list` shows only a token's first 8 characters.

### Remote hosts

`HOST:alias=user@host` (no spaces, exactly one `@`). The `remote_*` functions
accept the alias. For a non-standard port or key, define the host in
`~/.ssh/config` and use that name as `host`. In headless mode only registered
aliases are accepted. `remote_hosts` (alias `rh`) lists them.

### Settings

`GASH_NAME=value`, with a name from the [settings reference](#settings-reference).
One pair of matching quotes around the value is removed
(`GASH_GIT_EXCLUDE="/srv/a:/srv/b"`). Unknown names are skipped with a warning.

### Permissions

Keep the file private: `chmod 600 ~/.gash_env`. Whenever Gash reads it, it
warns when group or other users have any access (`Warning: ~/.gash_env is
accessible by other users (mode 644). Run: chmod 600 ~/.gash_env`) or when you
do not own it, then reads it anyway (headless: only with `GASH_DEBUG=1`). Symlinks are followed. `gash_doctor` checks the mode too.

## Precedence

For the settings in the reference below:

* **Interactive mode:** a variable that is already set when Gash loads (exported
  in the environment, or assigned in `~/.bashrc` before the Gash block) wins
  over `~/.gash_env`. `GASH_PROMPT=0 bash` gives a one-off shell with your own
  prompt.
* **Headless mode:** only the settings functions use are applied:
  `GASH_DB_TIMEOUT`, `GASH_AI_CONFIRM`, `GASH_AI_MODEL_CLAUDE` and
  `GASH_AI_MODEL_GEMINI`. The environment still wins over the file.
* **File only:** `GASH_LLM_EXEC`, `GASH_LLM_EXEC_ALLOW` and `GASH_LLM_SANDBOX`
  are read by `llm_exec` from `~/.gash_env` in both modes, never from the
  environment, so an agent cannot loosen `llm_exec` by exporting a variable
  (whoever controls `GASH_ENV_FILE` chooses the file, though). An interactive
  shell parses the file once: open a new shell after editing it.

Settings changed in `~/.bash_aliases` or `~/.bash_local` come too late for most
of them (the prompt, history and aliases are already set up). Use
`~/.gash_env` or set them before the Gash block in `~/.bashrc`.

## Settings reference

| Setting | Default | Effect |
|---|---|---|
| `GASH_PROMPT` | `1` | `0` keeps your own `PS1`. The prompt hook still runs for history and the last exit status. Names the prompt shows (git branch, virtualenv, `debian_chroot`, screen session) appear literally: `$(...)`, backticks or `${...}` in them are never run |
| `GASH_GIT_EXCLUDE` | empty | Colon-separated repository roots with no git segment in the prompt. Absolute paths, matched exactly (`~` is not expanded). `$HOME` as a root is always skipped |
| `GASH_BANNER` | `1` | `0` hides the startup banner and quote |
| `GASH_SHOW_INXI` | `0` | `1` prints a short `inxi` summary at startup when `inxi` is installed |
| `GASH_HISTORY` | `1` | Gash manages history: unlimited size, timestamps, `ignoreboth`, `histappend`, each command written at once. `0` leaves history alone |
| `GASH_HISTORY_SYNC` | `0` | `1` re-reads the history file before each prompt, to see commands from other terminals |
| `GASH_SAFETY_ALIASES` | `1` | `cp -iv`, `mv -iv`, `mkdir -pv`, and `rm -Iv --one-file-system --preserve-root` plus `--preserve-root` for `chown`, `chmod`, `chgrp` with GNU tools (`rm -iv` otherwise). `0` turns them off |
| `GASH_REPLACE_TOOLS` | `0` | `1` aliases `less` to `most` (and sets `PAGER`), `tail` to `multitail`, `df` to `pydf`, `top` to `htop`, `traceroute` and `tracepath` to `mtr`, `diff` to `colordiff` when installed, and `php`/`composer` to the newest `phpX.Y` |
| `GASH_UMASK` | empty | Octal mask such as `027`, applied at startup. Empty keeps the system umask |
| `GASH_PATH_ADD` | empty | Directories put first on `PATH`, in the order given, when they exist (`~` is expanded), e.g. `"~/.local/bin ~/.cargo/bin ~/.config/composer/vendor/bin"`. Re-sourcing never adds them twice. The same as `path_add` |
| `GASH_NVM` | `off` | `lazy` puts `node`, `npm`, `npx` and the global packages of nvm's default version on `PATH` at once and loads `nvm.sh` the first time `nvm` runs: it saves the 0.2 to 0.5 s that sourcing `nvm.sh` adds to every shell. `on` sources `nvm.sh` and its completion at startup. `off` leaves nvm to your own files. `NVM_DIR` defaults to `~/.nvm` |
| `GASH_DB_TIMEOUT` | `30` | PostgreSQL statement timeout for `llm_db_*`, in seconds. Not applied to MySQL, MariaDB or SQLite |
| `GASH_LLM_EXEC` | empty | `unrestricted` skips the `llm_exec` parser and allowlist. The dangerous-pattern blocklist still applies. File only |
| `GASH_LLM_EXEC_ALLOW` | empty | Extra commands for `llm_exec`, separated by spaces, commas or colons (`"make npm"`). They run with any arguments. File only |
| `GASH_LLM_SANDBOX` | `0` | `1` runs `llm_exec` inside `bwrap`: read-only filesystem, no network. Fails with an error when `bwrap` is missing. File only |
| `GASH_AI_CONFIRM` | `auto` | `auto` asks before sending data Gash collected (`ai_sysinfo`); `1` asks before every request; `0` never asks |
| `GASH_AI_MODEL_CLAUDE` | `claude-haiku-5-5` | Model for Claude requests |
| `GASH_AI_MODEL_GEMINI` | `gemini-2.5-flash` | Model for Gemini requests |

See [security.md](security.md) for what the `llm_exec` and AI settings change.

## Environment-only variables

These are not accepted in `~/.gash_env`.

| Variable | Effect |
|---|---|
| `GASH_HEADLESS=1` | Headless mode (see above). Only the value `1` counts |
| `NO_COLOR`, `GASH_NO_COLOR` | Turn ANSI colors off in Gash output and in the prompt (from the next prompt). `NO_COLOR`: any non-empty value; `GASH_NO_COLOR`: any value but empty or `0` |
| `GASH_DEBUG=1` | In headless mode, shows what modules and the `~/.gash_env` parser print on stderr while loading (hidden otherwise) |
| `GASH_TIMING=1` | When a shell starts: records how long each file and step takes, in the `__GASH_LOAD_TIMES` array (Bash 5.0+). `gash_doctor --timing` shows the same in a new shell |
| `XDG_CACHE_HOME` | Parent of the probe cache directory (default `~/.cache`) |
| `GASH_AI_DEBUG=1` | Prints AI request bodies and raw responses on stderr |
| `GASH_AWK` | awk program for the functions that need specific awk features. Default: `gawk` when installed, else `awk`. Read at load time |
| `GASH_TUI=plain` | Numbered prompts instead of arrow-key menus (`gash migrate`) |
| `GASH_ENV_FILE` | Use this file instead of `~/.gash_env` (loader, `llm_exec`, `gash_doctor`, `gash migrate`, `gash_env_init`, `gash_uninstall`) |
| `GASH_DIR` | Install location used by `~/.gashrc`, the installer, `gash_upgrade`, `gash_uninstall` and `gash_doctor` |
| `XDG_STATE_HOME` | Parent of the migration state directory (default `~/.local/state`) |
| `GASH_REQUIRE_SIGNED=1` | The installer and `gash_upgrade` refuse a release tag without a verified signature |
| `GASH_INSTALL_GIT_REPO`, `GASH_INSTALL_REF`, `PROFILE` | Installer only: repository to clone, ref to check out instead of the latest tag, profile file to edit (`/dev/null` for none) |
| `GASH_CHROME_ROOT`, `GASH_CHROME_LINK` | Where `chrome_headless_update` installs (default `/opt/chrome-headless-shell`) and the symlink it maintains (default `/usr/local/bin/chrome-headless-shell`) |
| `GASH_VERSION` | Set by Gash (not by you): the loaded version, for scripts |

## Your own shell files

In interactive mode, after everything else, Gash sources `~/.bash_aliases` and
then `~/.bash_local`, at top level, so they can override any Gash alias or
function. Many distribution `~/.bashrc` files already source `~/.bash_aliases`;
in that case it runs twice. `~/.gashrc` is created once by the installer and
not overwritten later; keep your own code in `~/.bash_local` rather than there.

## Inspecting and undoing

* `gash_doctor` (alias `gdoctor`): Bash version, files that failed to load,
  core files and modules, the `~/.gash_env` mode, settings currently set as
  shell variables, and whether `git`, `curl`, `jq`, `sqlite3` and `docker` are
  installed. `gash_doctor --timing` lists what each file and step costs when a
  shell starts, then times a whole new shell (Bash 5.0+).
* `gash_unload`: in an interactive shell, restores `PS1`, `PS2`, history
  variables, `EDITOR`, `PATH`, `histappend`, `checkwinsize` and the umask as
  they were before Gash loaded (so `GASH_PATH_ADD` and `GASH_NVM=lazy`
  entries go away), removes the prompt hook and every Gash function, alias
  and `GASH_*` variable. Nothing on disk changes; `source ~/.gashrc` loads Gash
  again. It does not work in headless mode.
* `gash_help NAME` and `gash` (the reference card) list every function. The
  generated [function reference](reference/README.md) has the same content.

## Upgrading from Gash 1.x

A Gash 1.x `~/.gashrc` (it exports `GASH_BASH`) starts a guided migration in
the first interactive shell after the upgrade. Run it again at any time with
`gash migrate`, check it with `gash migrate --status`, undo it with
`gash migrate --rollback`. See [migration.md](migration.md).
