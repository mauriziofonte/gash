# Troubleshooting

Symptoms, their causes and the fixes, for installing and running Gash.
When in doubt, start with `gash_doctor` (alias `gdoctor`): it checks the
installation and every tool Gash uses, and names what is missing.

Platforms: Linux, macOS and WSL. The [macOS](#macos) section covers what
differs there; the WSL helpers have their own
[guide](windows/README.md), and `gash hte` its
[own list of errors](hte/README.md#when-something-goes-wrong).

## The installer stops

`install.sh` checks what Gash needs before it changes anything. To run
only that check, and print the command that installs what is missing:

```bash
bash install.sh --check
```

| Message | Cause and fix |
|---|---|
| `Gash requires Bash 4.4 or newer (found X)` | No Bash 4.4 or newer on this system (macOS ships 3.2). On macOS: `brew install bash`, then run the installer again. When a newer Bash exists elsewhere on `PATH`, the installer only warns and names it: make it your login shell with `chsh -s PATH`. |
| `Gash needs these tools, which are not installed: git, curl.` | Gash needs git, curl or wget, tar, gzip, sha256sum or shasum, grep and sed. The next line, `To install them: ...`, gives the command for this system; the installer offers to run it, and `--assume-yes` runs it without asking. On macOS without a package manager, install the Xcode Command Line Tools (`xcode-select --install`) or Homebrew. |
| `Installing packages needs root, and sudo is not installed` | Run the install command it printed as root, then the installer again. |
| `No terminal to ask on: run that command, or the installer with --assume-yes.` | The installer runs without a terminal (piped in a script) and a tool is missing: run the command yourself, or add `--assume-yes`. |
| `Still not installed after the install command: ...` | The package manager ran but the tool is still not on `PATH`; install it by hand. |
| `No terminal to ask for confirmation. Re-run with --assume-yes to install non-interactively.` | The installer asks before it edits your profile; without a terminal, pass `--assume-yes`. |
| `DIR exists and is not a Gash checkout. Move it away or set GASH_DIR.` | The install directory (`~/.gash` by default) holds something else. Move it, or install elsewhere with `GASH_DIR=/path`. |
| `DIR has local modifications.` | An update would overwrite your edits: keep them with `git -C DIR stash` or discard them with `git -C DIR checkout -- .`, then run the installer again. |
| `Failed to clone REPO.`, `Failed to update Gash, run 'git fetch' in DIR yourself.` | No network access to GitHub, or a proxy that git does not use. Check `git ls-remote https://github.com/mauriziofonte/gash.git`. |
| `GASH_REQUIRE_SIGNED=1 is set; refusing to install TAG.` | No signature of the release could be verified: usually `cosign` is missing. Install it (the warning above the error links the instructions), or install without `GASH_REQUIRE_SIGNED=1`. Nothing was installed. |
| `Cannot write to your home directory (HOME).` | Fix the ownership or the mode of your home directory. |
| `Profile not found. Tried ~/.bashrc, ~/.bash_profile, and ~/.profile.` | Not an error: create `~/.bashrc` and run the installer again, or add the lines it prints to your profile yourself. |
| `Error: run the Gash installer with bash (curl ... \| bash).` | The installer was started with `sh`, `zsh` or another shell: pipe it to `bash`. |

After the install, the installer downloads the gash binary. When that
fails it prints a warning and goes on: the next section has the fix.

## "gash VERSION is not installed and could not be downloaded"

Every function then fails with this message, and an agent gets the
`binary_not_found` error.

**Cause:** the gash binary of the installed version is not under
`~/.local/share/gash/versions/` and could not be fetched. The reason is in
parentheses:

| Reason | Fix |
|---|---|
| `neither curl nor wget is installed`, `tar is not installed`, `gzip is not installed`, `neither sha256sum nor shasum is installed` | Install the tool: the hint gives the command for this system |
| `download failed: URL` | No network access to the GitHub release. Check the URL, a proxy, or set `GASH_RELEASE_URL` to a mirror |
| `checksum mismatch for ARCHIVE`, `the binary does not match release/SHA256SUMS` | The download is not the released file. Retry; if it persists, report it as described in [SECURITY.md](../SECURITY.md) |
| `this tree pins no binary for OS/ARCH (GASH_REQUIRE_SIGNED=1)` | `GASH_REQUIRE_SIGNED=1` accepts only a binary the release pins, and this one is not pinned |
| `no release for OS/ARCH` | No binary is built for this system |
| `another download holds LOCK` | Another shell is downloading the same version; wait and open a new terminal |

**Fix:** after fixing the cause, run `bash ~/.gash/install.sh --repair`: it
downloads and verifies the binary of the installed version and changes
nothing else.

## A function says a program is not installed

The message starts `Error: NAME needs TOOL, which is not installed.`, where
NAME is the function and TOOL the program it runs.

**Cause:** the function runs an external program that this system lacks.
**Fix:** the lines after the error give the package and the command that
installs it on this system; run it, or `gash_doctor --install TOOL`. A few
functions print the shorter `Error: Command 'TOOL' is not installed or not
in PATH.`: install the tool the same way. `gash_doctor --prereqs` lists
every tool and what needs it, and [requirements.md](requirements.md) the
package on each system. The [Gash guide](gash/README.md#install-a-missing-tool)
explains the message and `gash_doctor --install`.

## "Gash requires Bash 4.4 or newer"

The message gives the version found, and a second line with the Homebrew
command for macOS.

**Cause:** the shell is Bash older than 4.4 (macOS `/bin/bash` is 3.2) or
not Bash at all (`this is not Bash`); Gash loads nothing. **Fix:** make a
newer Bash your login shell. On macOS: `brew install bash`, add
`$(brew --prefix)/bin/bash` to `/etc/shells`, then
`chsh -s "$(brew --prefix)/bin/bash"`.

## Gash does nothing in a script

**Cause:** `gash.sh` returns at once in a non-interactive shell unless
`GASH_HEADLESS=1` is set. **Fix:**
`GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; files_largest --json .'`,
or call the binary directly: `~/.gash/bin/gash llm_tree`.

## "the Gash tree changed since this shell loaded it"

The message names the function you called and ends
`Run: source ~/.gashrc`.

**Cause:** Gash was updated while this shell was open, and the shell still
holds the old version. **Fix:** `source ~/.gashrc`, or open a new terminal.

## Escape codes in output, or no colors

**Cause:** Gash colors messages only when stdout is a terminal and none of
`GASH_HEADLESS=1`, `NO_COLOR` (any non-empty value) or `GASH_NO_COLOR` (any
value but `0`) is set. Messages go to stderr but follow stdout, so
`cmd 2>log` at a terminal writes colors into `log`. **Fix:** `NO_COLOR=1`,
or `--no-color` where accepted (`files_largest`, `tree_stats`,
`gash_doctor`, `gash`, `disk_usage`, ...). The prompt drops its colors too,
from the next prompt; `GASH_PROMPT=0` keeps your own `PS1`. Without colors
no escape code is printed at all: no screen clear, spinners redraw with
carriage returns, and tables, bars and menus print as plain text, with
`Info:`, `OK:`, `Warning:` and `Error:` in place of the status glyphs.

## Slow prompt in a large repository

**Cause:** in a git work tree each prompt runs `git status --porcelain=v2
--branch --untracked-files=no`, slow on huge repositories or network
mounts. `GASH_HISTORY_SYNC=1` adds a history re-read per prompt. **Fix:**
add the repository root to `GASH_GIT_EXCLUDE` (absolute paths joined by
`:`, matched exactly, no `~`) or set `GASH_PROMPT=0`. A repository at
`$HOME` is skipped.

## New terminals open slowly

**Cause:** something in the startup takes long: a large `~/.bash_history`
(Gash keeps history unlimited), `GASH_NVM=on`, your own `~/.bash_local`, or
a slow filesystem. **Fix:** run `gash_doctor --timing`. It lists what each
Gash file and step costs in a new shell (5 ms or more stands out), then the
time of a whole new shell with your `~/.bashrc` next to bash alone, so you
can tell Gash from the rest. `GASH_NVM=lazy` avoids sourcing `nvm.sh` at
every start.

## Warnings about ~/.gash_env

They appear once per shell, the first time a function reads the file
(`gash_db_list`, `remote_*`, `llm_db_*`, `ai_*`), or when the shell starts
if the file has `SSH:` keys, until the line is fixed. They name the file,
the line number and the problem, never the line's content. Headless calls
stay quiet unless `GASH_DEBUG=1`.

| Warning | Fix |
|---|---|
| `~/.gash_env is accessible by other users (mode 644). Run: chmod 600 ~/.gash_env` | `chmod 600 ~/.gash_env` (any mode without group or other bits is fine) |
| `~/.gash_env is owned by uid N, not by you (uid M). Gash will still read it.` | `chown "$USER" ~/.gash_env` |
| `Line N ...: SSH key file not found (skipping)` | Fix the path, or remove the line |
| `Line N ...: SSH key path starts with ~user; write the full path (skipping)` | Write the path in full, or with `~/` for your own home |
| `Line N ...: passphrase ends with whitespace` | The spaces or tabs are part of the passphrase; remove them unless the passphrase really ends with them |
| `Line N ...: DB URL must start with mysql://, mariadb://, pgsql:// or a +ssh form` | Use one of those schemes (`pgsql`, not `postgres`), or a `+ssh` form of the [databases](configuration.md#databases) |
| `Line N ...: unknown AI provider (supported: claude, gemini)` | Only `claude` and `gemini` exist |
| `Line N ...: AI provider X has an empty token` | Add the token after `=` |
| `Line N ...: HOST target must be user@host` | One `@`; a user of letters, digits, `.`, `_`, `-`; a host name or `[IPv6]`; no port (the `Hint:` line says to put it in a `Host` entry of `~/.ssh/config`) |
| `Line N ...: duplicate DB:NAME, ignored (first is at line M)` | Two entries share a name and the first wins: rename or remove one |
| `Line N ...: unknown GASH_ setting` | Check the name in [configuration.md](configuration.md#settings-reference) |
| `Line N ...: unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=)` | The line has none of those prefixes |
| `Line N ...: SSH: entry has no '='` | Use `SSH:path=passphrase` (the same for `DB:`, `AI:`, `HOST:`) |

## A setting has no effect

1. **Typo or quotes.** An unknown name gets a warning when the file is
   read; `gash_doctor` lists, under "Configuration", the settings set in
   the shell.
2. **Environment wins.** A variable already set when Gash loads overrides
   the file.
3. **Headless mode** applies only `GASH_DB_TIMEOUT`, `GASH_AI_CONFIRM` and
   `GASH_AI_MODEL_*` from the file; the prompt, history and alias settings
   mean nothing there. `GASH_LLM_EXEC`, `GASH_LLM_EXEC_ALLOW` and
   `GASH_LLM_SANDBOX` are read from the file only, never from the
   environment.
4. **Set too late.** A setting assigned in `~/.bash_local` comes after the
   prompt, history and aliases are set up. Use `~/.gash_env`, and open a
   new terminal.

## awk and sed in llm_exec

**Cause:** `llm_exec` refuses `awk` unless `gawk --sandbox` works, and
refuses `sed` unless GNU `sed --sandbox` works (mawk, BSD awk and BSD sed
have no sandbox). `hgrep` runs no awk: its matching, timestamps and JSON
are the same with every awk. **Fix:** install `gawk` and GNU sed
(`gash_doctor --install gawk gnu-sed`). To force a specific awk program
for the other functions, set `GASH_AWK` (for example `GASH_AWK=gawk`)
before Gash loads.

## macOS

Gash needs the Homebrew Bash on macOS. The scanners (`files_largest`,
`tree_stats`, `dirs_*`, `llm_tree`, `llm_find`) and the path checks of the
`llm_*` functions are Go code and work the same as on Linux; a few
functions still call GNU tools:

| Missing GNU feature | Affected |
|---|---|
| `sed --sandbox`, `gawk --sandbox` | `sed` and `awk` in `llm_exec` are refused |
| `df -T` | `disk_usage` shows every `/dev` device, without a filesystem type |

**Fix:** `brew install gawk gnu-sed` is enough for `llm_exec`: Gash
finds them under their Homebrew names (`gawk`, `gsed`). For the filesystem
type of `disk_usage`, also `brew install coreutils` and put its GNU names
first in `PATH`, before the Gash block in `~/.bashrc`:

```bash
PATH="$(brew --prefix)/opt/coreutils/libexec/gnubin:$PATH"
```

## Parsing headless output

* Data goes to stdout, diagnostics to stderr. Capture them separately:
  `out=$(llm_git_status 2>err.json)`.
* `llm_*` and `agent_*` errors are one JSON object per line on stderr:
  `{"error":"...","details":"...","action":"STOP|RETRY|CONTINUE|FATAL","recoverable":true,"hint":"..."}`.
  Branch on `.action`. A missing program adds `missing` and `install`.
* The filesystem scanners emit JSON errors only with `--json` or `--null`.
  Without them they print `Error: TYPE: DETAILS` and `Hint: ...`.
* Other functions print plain `Info:`, `OK:`, `Warning:`, `Error:` lines.

## Load errors and gash_doctor

`gash_doctor` reports the Bash version and the binary, then
`failed to load FILE: exit N` (or `FILE: missing`) for every file that did
not load, then the core files, the modules, the binary and the generated
files against the release, the `~/.gash_env` mode and settings, and every
external tool. It ends with `All checks passed.` or `N issue(s) found.`; a
missing tool that only some functions use is a warning, not an issue.

Headless mode hides what a failing file prints (`GASH_DEBUG=1` shows it).
A missing file means a damaged install: `git -C ~/.gash status`, then
`git -C ~/.gash checkout -- .` or reinstall. A binary that does not match
its release: `bash ~/.gash/install.sh --repair`.

## Completion does not work

**Cause:** `~/.gashrc` loads `bash_completion` from the install directory,
in interactive shells only; sourcing `gash.sh` directly skips it.
**Fix:** source `~/.gashrc`. `complete -p files_largest` should print
`complete -o default -F __gash_complete files_largest`.

## "gash: your configuration is from Gash 1.x"

The line goes on to say that Gash 3 does not migrate it, and to run
`gash migrate` for the steps.

**Cause:** `~/.gashrc` is the 1.x loader (it mentions `GASH_BASH`) and no
migration is recorded. An interactive shell prints this line on stderr at
most once a day; headless shells print nothing.
**Fix:** Gash 3 does not migrate a 1.x configuration. `gash migrate` prints
the route: check out v2.1.0 in your Gash directory, open a new terminal,
run `gash_migrate` there (the 2.x guided review), then `gash_upgrade`.
State lives in `~/.local/state/gash` (`$XDG_STATE_HOME/gash`). See
[migration.md](https://github.com/mauriziofonte/gash/blob/v2.1.0/docs/migration.md).

## Removing Gash

`gash_unload` removes Gash from the current shell; `gash_uninstall`
removes it for good, and asks first (without a terminal it prints
`Info: Uninstall cancelled.` and removes nothing). Run
`gash hte uninstall --yes` before it if you installed `gash hte`. What
each removes, and what it leaves behind, is in the
[Gash guide](gash/README.md#remove-gash).
