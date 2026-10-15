# Upgrading from Gash 1.x

Gash 2.0 changes a few defaults that 1.x users rely on. None of them
silently rewrites your files: the first interactive shell after the upgrade
opens a guided migration that shows each change, asks what you want, shows
a diff, and backs up every file before writing it.

## Upgrade

```bash
gash_upgrade          # from a 1.x shell: fetches the latest release tag
exec bash             # open a fresh shell: the migration starts
```

A manual checkout works too: `git -C ~/.gash fetch --tags && git -C ~/.gash
checkout v2.0.0`, then open a new shell.

Gash 2.0 needs **Bash 4.4 or newer**. On an older Bash it says so and loads
nothing; on macOS install Bash with `brew install bash`.

## The guided migration

The migration starts by itself in the first interactive terminal after the
upgrade. It shows only the screens that concern your setup:

| Screen | What it detects | Recommended choice | The other choice |
|---|---|---|---|
| Loader | `~/.gashrc` is the 1.x file (exports `GASH_BASH`) | 2.0 loader; your own lines are kept at the end, checked with `bash -n` | keep the file as it is |
| Settings file | lines of `~/.gash_env` that 2.0 skips; a file readable by others | comment the lines out, `chmod 600` | leave the file alone |
| Command replacements | `htop`, `pydf`, `colordiff`, `most`, `multitail`, `mtr` or `phpX.Y` installed | the standard commands | `GASH_REPLACE_TOOLS=1` |
| umask | your system umask is not 022 | keep the system umask | `GASH_UMASK=022` |
| Your shell files | `~/.bashrc`, `~/.bash_profile`, `~/.bash_aliases`, `~/.bash_local` lines using changed commands | rewrite `dcup2 … -n` as `--dry-run` | leave the files alone |
| llm_exec | always, unless `GASH_LLM_EXEC` is set | read-only allowlist | `GASH_LLM_EXEC=unrestricted` |
| AI consent | `AI:` keys in `~/.gash_env` | ask before sending collected data | `GASH_AI_CONFIRM=1` (always ask) or `0` (never) |

On a terminal each screen lists its options with a selection line under
them (`↑`/`↓`, `j`/`k` or the option's number moves it, Enter confirms, `q`
stops). `--plain` uses numbered
prompts instead. Nothing is written until you confirm the final diff.

At startup the first screen also offers **Remind me next time** and
**Keep everything as it is and don't ask again**. Scripts and headless
shells (`GASH_HEADLESS=1`) never see a screen: they get one line on stderr,
at most once a day.

```bash
gash migrate              # run it again, any time
gash migrate --yes        # take every recommended choice, no questions
gash migrate --status     # schema version, pending migration, backups
gash migrate --rollback   # restore the files saved by the last migration
```

Backups live in `~/.local/state/gash/backups/<date>/` (under
`$XDG_STATE_HOME/gash` when that is set), with a manifest that also records
files that did not exist before, so a rollback removes them again.

## What changed

### Your configuration

- **Loader.** The 2.0 `~/.gashrc` finds Gash wherever it is installed
  (`GASH_DIR`) and no longer exports `GASH_BASH` and `GASH_COMPLETION`.
  Scripts that sourced `$GASH_BASH` should source `~/.gash/gash.sh`.
- **Command replacements are opt-in.** 1.x replaced `top`, `df`, `diff`,
  `less`, `tail`, `traceroute` and `tracepath` with their fancier cousins
  whenever those were installed, and `php`/`composer` with the newest
  `phpX.Y`. 2.0 leaves the standard commands alone unless you set
  `GASH_REPLACE_TOOLS=1`. The `php84`, `composer84` style aliases are always
  there.
- **umask is no longer forced to 022.** Set `GASH_UMASK` to choose one.
- **`~/.gash_env` has a strict grammar.** Malformed lines are skipped with
  a warning that gives the line number and never the value. Settings are
  `GASH_NAME=value` lines; see [configuration](configuration.md).

### Commands

- **`-n` means `--limit` everywhere** (`files_largest`, `dirs_*`, `llm_find`,
  `llm_grep`, `llm_git_log`, `history_grep`). `docker_compose_upgrade -n`
  no longer means dry run: use `--dry-run`.
- **Unknown options fail** with a message instead of being ignored or taken
  as a path.
- **`port_kill`** targets only processes listening on the port (TCP
  listeners and UDP sockets), lists them and asks first; `--force` skips
  the question.
- **`remote_*`** commands ask before touching any `.ssh` directory on the
  other side, and an rsync `--delete` into the remote home itself asks too.
- **`process_kill`** without a terminal (scripts, headless) needs `--force`,
  also for a single match.
- **`php_detect_version` and `node_detect_version`** take `--json`
  (`--llm` still works).
- **Every function answers `--help`** without side effects; a few 1.x
  functions acted on the word `--help`.

### Agents and scripts (`llm_*`)

- **`llm_exec` runs read-only commands only**, from an allowlist, in a child
  shell. Extend it with `GASH_LLM_EXEC_ALLOW`, sandbox it with
  `GASH_LLM_SANDBOX=1` (bwrap), or restore the 1.x behaviour with
  `GASH_LLM_EXEC=unrestricted`. These three settings are read from
  `~/.gash_env` only, never from the environment, so an agent cannot widen
  its own permissions.
- **`llm_db_*` run in a read-only database session**, not only behind a SQL
  regex.
- **`llm_git_status`** has a new `conflicted` array, and `staged` and
  `modified` follow git's index and worktree columns exactly.
- **Consistent errors:** `llm_git_diff` and `llm_git_log` return
  `not_a_git_repo` outside a repository, `llm_ports` returns
  `ports_tool_not_found` without `ss`/`netstat`, and `llm_docker_check`
  reports errors on stderr in the standard envelope.
- **`llm_exec` refuses forbidden paths** (`/proc`, `/sys`, `/dev`, `/root`,
  `/etc/shadow`, ...) in its arguments.
- **Error envelopes:** `recoverable` follows `action` (false only for
  `FATAL`). A value flag without a value returns `missing_value`.
- **`llm_tree`** marks a listing cut at 500 entries with `"truncated": true`.

See the [agent guide](../AGENTS.md) for the full contract.

### AI

- `ai_sysinfo` asks before sending the data it collected
  (`GASH_AI_CONFIRM=auto`, the default) and refuses without a terminal.
- Secrets in your text (private keys, URL passwords, `KEY=value` secrets,
  API tokens) are redacted before any request leaves the machine.
- `GASH_AI_MODEL_CLAUDE` and `GASH_AI_MODEL_GEMINI` choose the models.

The full list is in the [changelog](../CHANGELOG.md).

## Going back to 1.x

```bash
gash migrate --rollback                 # restore your 1.x files
git -C ~/.gash checkout v1.5.3          # the last 1.x release
```
