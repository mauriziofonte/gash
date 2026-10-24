# Troubleshooting

Symptom, cause, fix. When in doubt, start with `gash_doctor` (alias `gdoctor`).

## "Gash requires Bash 4.4 or newer"

```
Gash requires Bash 4.4 or newer (this is 3.2.57(1)-release).
On macOS: brew install bash, then use /opt/homebrew/bin/bash.
```

**Cause:** the shell is Bash older than 4.4 (macOS `/bin/bash` is 3.2) or not
Bash at all (`this is not Bash`); Gash loads nothing. **Fix:** make a newer Bash
your login shell. On macOS: `brew install bash`, add `$(brew --prefix)/bin/bash`
to `/etc/shells`, then `chsh -s "$(brew --prefix)/bin/bash"`.

## Gash does nothing in a script

**Cause:** `gash.sh` returns at once in a non-interactive shell unless
`GASH_HEADLESS=1` is set. **Fix:** `GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; files_largest --json .'`

## Escape codes in output, or no colors

**Cause:** Gash colors messages only when stdout is a terminal and none of
`GASH_HEADLESS=1`, `NO_COLOR` (any non-empty value) or `GASH_NO_COLOR` (any
value but `0`) is set.
Messages go to stderr but follow stdout, so `cmd 2>log` at a terminal writes
colors into `log`. **Fix:** `NO_COLOR=1`, or `--no-color` where accepted
(`files_largest`, `tree_stats`, `gash_doctor`, `gash`, `disk_usage`, ...). The
prompt drops its colors too, from the next prompt; `GASH_PROMPT=0` keeps your
own `PS1`. Without colors no escape code is printed at all: no screen clear,
spinners redraw with carriage returns, and tables, bars and menus print as
plain text, with `Info:`, `OK:`, `Warning:` and `Error:` in place of the
status glyphs.

## Slow prompt in a large repository

**Cause:** in a git work tree each prompt runs `git status --porcelain=v2
--branch --untracked-files=no`, slow on huge repositories or network mounts.
`GASH_HISTORY_SYNC=1` adds a history re-read per prompt. **Fix:** add the
repository root to `GASH_GIT_EXCLUDE` (absolute paths joined by `:`, matched
exactly, no `~`) or set `GASH_PROMPT=0`. A repository at `$HOME` is skipped.

## New terminals open slowly

**Cause:** something in the startup takes long: a large `~/.bash_history`
(Gash keeps history unlimited), `GASH_NVM=on`, your own `~/.bash_local`, or a
slow filesystem. **Fix:** run `gash_doctor --timing`. It lists what each Gash
file and step costs in a new shell (5 ms or more stands out), then the time of
a whole new shell with your `~/.bashrc` next to bash alone, so you can tell
Gash from the rest.

## Warnings about ~/.gash_env

They repeat at every interactive shell start until the line is fixed. They
name the file, the line number and the problem, never the line's content.
Headless calls stay quiet unless `GASH_DEBUG=1`.

| Warning | Fix |
|---|---|
| `~/.gash_env is accessible by other users (mode 644). Run: chmod 600 ~/.gash_env` | `chmod 600 ~/.gash_env` (any mode without group or other bits is fine) |
| `~/.gash_env is owned by uid N, not by you` | `chown "$USER" ~/.gash_env` |
| `Line N ...: SSH key file not found (skipping)` | Fix the path, or remove the line |
| `Line N ...: DB URL must start with mysql://, mariadb:// or pgsql://` | Use one of those schemes (`pgsql`, not `postgres`) |
| `Line N ...: unknown AI provider (supported: claude, gemini)` | Only `claude` and `gemini` exist |
| `Line N ...: AI provider X has an empty token` | Add the token after `=` |
| `Line N ...: HOST target must be user@host` | One `@`, no spaces |
| `Line N ...: unknown GASH_ setting` | Check the name in [configuration.md](configuration.md#settings-reference) |
| `Line N ...: unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=)` | The line has none of those prefixes |
| `Line N ...: SSH: entry has no '='` | Use `SSH:path=passphrase` (same for `DB:`, `AI:`, `HOST:`) |

## A setting has no effect

1. **Typo or quotes.** `gash_doctor` lists the settings Gash sees; an
   unknown name gets a warning at startup.
2. **Environment wins.** A variable already set when Gash loads overrides the
   file. `gash_doctor` lists the settings set in the shell under "Configuration".
3. **Headless mode** applies only `GASH_DB_TIMEOUT`, `GASH_AI_CONFIRM` and
   `GASH_AI_MODEL_*` from the file (and `llm_exec` reads its three
   `GASH_LLM_*`); the prompt, history and alias settings mean nothing there.
4. **Set too late.** A setting assigned in `~/.bash_local` comes after the
   prompt, history and aliases are set up. Use `~/.gash_env`.

## awk differences (mawk, BSD awk)

**Cause:** Gash uses `gawk` when it is installed, otherwise the system `awk`.
`hgrep --json` needs `strftime` (gawk; mawk 1.3.4 has it, older mawk and BSD
awk do not). `llm_exec` refuses `awk` unless `gawk --sandbox` works, and
refuses `sed` unless GNU `sed --sandbox` works.
**Fix:** install `gawk`. To force a specific program, set `GASH_AWK` (for
example `GASH_AWK=gawk`) before Gash loads.

## macOS

macOS is best effort. Besides Homebrew Bash, Gash expects GNU tools:

| Missing GNU feature | Affected |
|---|---|
| `find -printf` | `files_largest`, `tree_stats`, `dirs_find_large --with-mtime`, `llm_tree` (JSON) |
| `realpath -m` | path normalization in `llm_*` |
| `sed --sandbox` | `sed` in `llm_exec` |
| `df -T` | `disk_usage` shows every `/dev` device, without a filesystem type |

**Fix:** `brew install bash coreutils findutils gawk gnu-sed`, then put the GNU
names first in `PATH` before the Gash block in `~/.bashrc`:

```bash
for p in coreutils findutils gnu-sed; do
    PATH="$(brew --prefix)/opt/$p/libexec/gnubin:$PATH"
done
```

## Parsing headless output

* Data goes to stdout, diagnostics to stderr. Capture them separately:
  `out=$(llm_git_status 2>err.json)`.
* `llm_*` errors are one JSON object per line on stderr:
  `{"error":"...","details":"...","action":"STOP|RETRY|CONTINUE|FATAL","recoverable":true,"hint":"..."}`.
  Branch on `.action`.
* The filesystem scanners emit JSON errors only with `--json` or `--null`.
  Without them they print `Error: TYPE: DETAILS` and `Hint: ...`.
* Other functions print plain `Info:`, `OK:`, `Warning:`, `Error:` lines; a
  malformed `~/.gash_env` adds `Warning:` lines. Fix the file.

## Load errors and gash_doctor

`gash_doctor` reports the Bash version, then `failed to load FILE: exit N` (or
`FILE: missing`) for every file that did not load, then core files, modules,
the `~/.gash_env` mode and settings, and the external tools `git`, `curl`,
`jq`, `sqlite3`, `docker` (missing tools are warnings only). It ends with
`All checks passed.` or `N issue(s) found.`

Headless mode hides what a failing file prints (`GASH_DEBUG=1` shows it). A
missing file means a damaged checkout: `git -C ~/.gash status`, then reinstall.

## Completion does not work

**Cause:** `~/.gashrc` loads `bash_completion` from the install directory, in
interactive shells only; sourcing `gash.sh` directly skips it. **Fix:** source
`~/.gashrc` (a 1.x one is replaced by `gash migrate`). `complete -p
files_largest` should print `complete -o default -F __gash_complete files_largest`.

## "gash: your configuration is from Gash 1.x"

```
gash: your configuration is from Gash 1.x; open a terminal and run "gash migrate" to review the changes in 2.0
```

**Cause:** `~/.gashrc` is the 1.x loader (it mentions `GASH_BASH`) and no
migration is recorded. In a script or headless shell Gash prints this line on
stderr at most once a day; in an interactive terminal it starts the guided
migration instead.
**Fix:** run `gash migrate` in a terminal (`gash migrate --yes` takes the
recommended choice everywhere without asking). In the menu that opens by
itself, "Keep everything as it is and don't ask again" also stops it. State
lives in `~/.local/state/gash` (`$XDG_STATE_HOME/gash`); `gash migrate --status`
shows it. See [migration.md](migration.md).

## Removing Gash

* For the current shell only: `gash_unload` (interactive shells).
* For good: `gash_uninstall` in an interactive terminal (it refuses in headless
  mode). After confirmation it removes the Gash block from `~/.bashrc`,
  `~/.bash_profile` and `~/.profile`, deletes `~/.gashrc` and the install
  directory, and asks separately before deleting `~/.gash_env`. A profile that
  is a symlink (dotfiles) stays one: the file it points to is edited, with its
  mode kept.
* Backups it keeps, and lists at the end (`Backups kept: ...`): each edited
  profile as it was before the edit (`~/.bashrc.gash-uninstall.bak`,
  `~/.bash_profile.gash-uninstall.bak`, `~/.profile.gash-uninstall.bak`), and
  `~/.gashrc.gash-uninstall.bak` when your `~/.gashrc` held lines of your own
  (a `~/.gashrc` exactly as an installer wrote it is simply deleted).
* Left behind: `~/.local/state/gash` (migration backups), a block added to a
  profile chosen with `PROFILE=` at install time, and a file named by
  `GASH_ENV_FILE`. Remove them by hand. Open a new terminal afterwards.
