# Changelog

All notable changes to Gash are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Gash follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html): the public API is
the set of documented functions, the `--json` envelopes, the error contract
and the `GASH_*` settings (see [AGENTS.md](AGENTS.md)).

## [Unreleased]

Every function now looks the same on a terminal: one palette, one set of
glyphs, one way to draw tables, titles, key/value blocks, lists and
questions. Piped and `NO_COLOR` output carries no escape code at all.

### Changed

- Colors mean the same thing everywhere: structure (rules, borders,
  labels) in the primary color, the names you look for (functions, mount
  points, PIDs, hosts) in the accent color, secondary data muted, green,
  yellow and red only for status. Before, each module had its own mix of
  cyan, green, yellow and bold white.
- Columns are aligned by one table renderer: `disk_usage`, `process_find`,
  `history_grep`, `docker_compose_check`, `remote_hosts`, `git_list_tags`,
  the scanners, `gash` and `gash_help` no longer pad by hand, so long values
  widen their column instead of breaking the row. Usage bars (`disk_usage`,
  `tree_stats`, `sysinfo overview`) take the status color of their fill.
- Key/value output (`ip_public`, `php_detect_version`,
  `node_detect_version`, `wsl_compact`, `sysinfo`, `gash_doctor` settings,
  `gash migrate --status`) goes through one renderer; plain text is the same.
- Questions look alike: `Question? [y/N]` (default No) or
  `Do you want to continue? [Y/n]` (default Yes), on stderr. Before,
  confirmations read `(y/N): `.
- Usage reminders after a wrong call are one `Usage: ...` line on stderr
  (before: `Error: Usage: ...`, or the whole help page on stdout for
  `git_apply_patch`).
- `ai_sysinfo` findings read `Error:`, `Warning:`, `OK:`, `Info:` when piped
  or saved (before: `[!]`, `[+]`, `[i]`); on a terminal they are glyphs.
- `gash migrate` lists the options of each screen and moves a selection
  line under them; with `NO_COLOR` it no longer clears the screen.
- Spinners and progress lines draw escape codes only when stdout and stderr
  are both color terminals; otherwise they redraw with carriage returns.
- `hgrep` groups and highlights only when colors are on (before: whenever
  stdout was a terminal), and `hgrep --pick` also works when its output is
  captured.
- `docker_compose_upgrade` marks each service with a status glyph on a
  terminal and `OK:` (upgrade) or `Info:` (pinned, skipped) when piped,
  instead of `[UPGRADE]` and `[SKIP]`; the dry-run notice reads
  `Dry run: ...` here and in `nvm_prune_versions` (before: `[DRY-RUN]`).
- `gl`, `gla`, `glo`, `glg`, `gls` color the log with the Gash palette:
  hash in the accent color, branches and tags in the primary color, date
  and author muted. Their plain text is unchanged, they print no colors
  with `NO_COLOR`, and graph lanes alternate primary and muted unless
  `log.graphColors` is configured.

### Fixed

- `llm_procs --name NAME` no longer lists the shell that runs it, its
  parent shells or their subshells (their command lines contain NAME):
  `llm_procs --name no-such-process` returns `[]`.
- Model answers, paths and command lines are printed verbatim: a `\n` or
  `\c` in them is no longer interpreted (`echo -e` and `printf %b` are gone).
- `gash_ssh_auto_unlock` no longer writes escape codes into a pipe.
- `process_find` with no match, `port_kill` with nothing listening,
  `wsl_compact`, `docker_compose_scan` on a missing directory and
  `docker_compose_check` without a local digest stopped early under
  `set -e` before printing their message.
- `gash_username` has a help page.

### Tests

- `tests/run.sh` skips the terminal rendering fixtures unless given
  `--slow` or the spec by name; CI runs them once, in their own job.

## [2.1.0] - 2026-10-14

Everyday tools from a long-lived `.gashrc`, commands that show what they
will do before they do it, a harder read-only boundary for agents, and a
quote collection where every line has a source.

### Added

- `node_upgrade` (alias `nvmc`): installs the newest patch of every
  installed Node major, moves global packages, removes the old patches.
- `chrome_headless_update` (alias `chri`): keeps a headless Chrome for
  Puppeteer and Playwright up to date under `GASH_CHROME_ROOT`, with
  rollback to the previous build and the missing system libraries listed.
- `git_sync` (alias `gsync`): add, commit and push in one step; refuses a
  detached HEAD or a merge or rebase in progress.
- `wsl_compact` (alias `wslc`): drops caches and compacts memory so WSL
  hands RAM back to Windows; `--status` only reports.
- `path_add [--append] DIR...` and the `GASH_PATH_ADD` setting: put
  directories on PATH once, only when they exist.
- `GASH_NVM=lazy`: node, npm and global packages of nvm's default version
  are on PATH at once and nvm.sh loads on the first `nvm` call. `on`
  sources nvm as its installer would; `off` (default) leaves nvm alone.
- `-y`/`--yes` and `--dry-run` on the commands that change things; they show
  a plan and ask "Do you want to continue? [Y/n]".
- `archive_extract` opens `.tar.xz`, `.txz`, `.tar.zst`, `.tzst`, `.xz`
  and `.zst`.
- `gash_inspiring_quote --source` shows where a quote comes from.
- Completion for short aliases (`flf<Tab>` completes like `files_largest`)
  and for the new functions.
- `llm_config --max-bytes N` (default 1 MiB).
- `gash_doctor --timing`: load time of every file, and of the whole shell
  against bare bash; `GASH_TIMING=1` records it while loading.

### Changed

- Interactive startup is about 40% faster (about 155 ms to 90 ms on the
  reference machine): capability probes are cached in
  `~/.cache/gash/probes` (or `$XDG_CACHE_HOME/gash`), lesspipe, tput, fold
  and the prompt's first git status no longer run while loading.
- A Gash alias no longer hides a program of the same name on PATH (`gp`,
  `gc`, `dc`, `gs`, `hg` when Mercurial is installed); the deliberate
  wrappers (`ls`, `rm`, `grep`...) stay.
- The banner quote wraps by characters, so accented text wraps right;
  `mesg n` runs only on a terminal.
- Quotes: 373 quotes from people who shaped open computing, each with its
  book, paper, talk or mailing list post. Misattributed, anonymous and
  off-topic lines were removed or corrected.
- `process_kill` always lists the matches and asks, even for one process.
- `services_stop` lists the running services and asks; the `quit` alias no
  longer passes `--force`. Without systemd it uses `service`. `redis-server`
  is in the list.
- `docker_prune_all` counts what it would remove before asking.
- `wsl_restart` and `wsl_shutdown` ask first and save history before
  stopping WSL; `wsl_explorer` converts the path with `wslpath`.
- Declining a confirmation returns 1 everywhere (was 0 in some commands).
  `--force` still works where it did.
- One ssh-agent per user on a fixed socket, reused by every shell, instead
  of a new agent per shell.
- AI defaults: `claude-haiku-5-5` (with low effort) and `gemini-2.5-flash`.
- AI redaction keeps ordinary words: "token expired" is sent as is; a long
  token after "token" or "secret" is still hidden.
- `llm_env` hides more names (PASSPHRASE, `*_PWD`, `*_PAT`, JWT, SESSION,
  COOKIE, DSN...) and masks tokens and private keys inside values.
- `llm_find PATTERN --contains RE` filters by PATTERN, searches hidden and
  gitignored files, and every extension without ripgrep.
- `llm_config` converts YAML with either yq (mikefarah or the Python one)
  and falls back to the raw text.
- `llm_procs` prints one compact JSON line.
- `--help`/`-h` work anywhere on an `llm_*` line (alone for `llm_exec`).

### Fixed

- `git_repair` works in worktrees and submodules, says when there is
  nothing to repair, and asks before changing the object store.
- Timings in comma-decimal locales (`de_DE`, `it_IT`): `query_time` and
  `scan_time` were wrong and each database query ran twice.
- MySQL output: `\\0` after an escaped backslash is no longer read as NUL.
- `llm_*` path checks work where `realpath -m` is missing (macOS).
- `nvm_prune_versions` no longer needs `grep -P`; `lts/<name>` aliases
  resolve through nvm, including `lts/krypton`.
- `llm_docker_check --help` shows help instead of scanning.
- `llm_exec` works when `POSIXLY_CORRECT` is exported (Bash before 5.1).
- No `local -` anywhere: on Bash 4.4 and 5.0 it makes an interactive shell
  crash when it exits.
- `wsl_restart`, `wsl_shutdown` no longer fail where history is off.

### Security

- `llm_db_*`: `-d` and `.target-database` must be plain database names
  (`invalid_database_name`); a crafted name could send the stored password
  to another host or reopen a writable session.
- `llm_exec` runs in a minimal environment (`env -i` plus PATH, HOME,
  locale and a few others), so tokens in the caller's environment do not
  reach it.
- `llm_exec` refuses walks from `/` or above protected trees, walks that
  follow symlinks, `ps e`, `getent shadow`, yq `load()`, and git commands
  that read arbitrary objects (`--no-index`, `cat-file --batch`, blob ids,
  `REV:PATH` of a secret file); grep, rg, diff and git skip secret files.
- git runs with `GIT_OPTIONAL_LOCKS=0`, so `git status` no longer writes
  the index.

### Agents (contract)

- New error codes, all RETRY: `invalid_database_name`, `invalid_type`,
  `file_too_large`.
- A second PATH argument fails with `unexpected_argument` (`llm_tree`,
  `llm_find`, `llm_grep`, `llm_git_*`, `llm_project`, `llm_deps`,
  `llm_config`); `llm_git_status`, `llm_project` and `llm_config` reject
  unknown options; `llm_find` without a pattern fails with
  `missing_pattern`; `llm_project` on a missing path fails with
  `path_not_found`; numbers have at most 9 digits.

## [2.0.0] - 2026-10-14

A rebuild for quality: a quieter and reversible bootstrap, read-only
guarantees for agents that hold under adversarial input, a reference that
cannot drift from the code, and terminal output designed for people.
Upgrading from 1.x is guided: see [docs/migration.md](docs/migration.md).

### Breaking

- Bash 4.4 or newer is required; older shells get a message and load nothing.
- Command replacements are opt-in: `top`→`htop`, `df`→`pydf`,
  `diff`→`colordiff`, `less`→`most`, `tail`→`multitail`,
  `traceroute`/`tracepath`→`mtr` and `php`/`composer`→`phpX.Y` need
  `GASH_REPLACE_TOOLS=1`.
- umask is no longer forced to 022 (`GASH_UMASK` sets one).
- `~/.gashrc` no longer exports `GASH_BASH` and `GASH_COMPLETION`; Gash
  finds itself through `GASH_DIR`.
- `~/.gash_env` is parsed with a strict grammar: malformed lines are
  skipped with a warning that names the line, never the value.
- `llm_exec` runs only read-only commands from an allowlist, in a child
  shell. `GASH_LLM_EXEC=unrestricted` restores the 1.x behaviour;
  `GASH_LLM_EXEC_ALLOW` extends the list; `GASH_LLM_SANDBOX=1` runs it in
  bwrap. These are read from `~/.gash_env` only.
- `-n` means `--limit` everywhere; `docker_compose_upgrade -n` is no longer
  a dry run (`--dry-run`).
- Unknown options fail instead of being ignored or taken as arguments.
- `port_kill` targets only listening processes on the port and asks first
  (`--force` skips the question). `process_kill` without a terminal needs
  `--force`, also for a single match.
- `llm_git_diff` and `llm_git_log` return `not_a_git_repo` outside a
  repository; `llm_ports` returns `ports_tool_not_found` instead of `[]`;
  `llm_docker_check` reports errors on stderr in the standard envelope (the
  path moved to `details`) and unknown digests as `null`.
- `llm_exec` refuses arguments under the forbidden paths (`forbidden_path`),
  which also cover root's home where it is not `/root` (macOS: `/var/root`).
- `GASH_NO_COLOR=0` no longer turns colors off.
- Error envelopes: `recoverable` follows `action` (false only for `FATAL`).
- `php_detect_version` and `node_detect_version` take `--json` (`--llm`
  still works).

### Added

- Guided migration from 1.x (`gash migrate`): one screen per change that
  concerns the user, arrow-key menus with a plain fallback, a diff preview,
  backups with a manifest, `--rollback`, `--status`, `--yes`.
- Settings in `~/.gash_env`, documented in `.gash_env.template`:
  `GASH_PROMPT`, `GASH_BANNER`, `GASH_HISTORY`, `GASH_HISTORY_SYNC`,
  `GASH_SAFETY_ALIASES`, `GASH_REPLACE_TOOLS`, `GASH_UMASK`,
  `GASH_DB_TIMEOUT`, `GASH_AI_CONFIRM`, `GASH_AI_MODEL_CLAUDE`,
  `GASH_AI_MODEL_GEMINI`; `GASH_GIT_EXCLUDE` and `GASH_SHOW_INXI` can live
  there too. The environment wins over the file.
- `gash_unload` restores a snapshot of everything Gash changed.
- `gash_doctor` reports the Bash version, files that failed to load and the
  active settings.
- `gash` prints a reference card generated from the help registry, by topic.
- `sysinfo` opens with an overview card (host, OS, kernel, uptime, load,
  CPU, memory, swap and disk with usage bars, address); `sysinfo overview`
  shows it alone.
- On a terminal, `files_largest`, `dirs_largest` and `dirs_find_large` draw
  a ranked list with share bars and a footer; `tree_stats` a summary grid
  with top extensions; `disk_usage` usage bars that turn yellow and red.
- `llm_git_status` reports merge conflicts in a `conflicted` array.
- `llm_tree` marks a listing cut at 500 entries with `"truncated": true`.
- `llm_ports --listen` filters listening sockets and reports the address.
- `llm_deps` reads `go.mod`.
- `llm_db_query` and `llm_db_sample` take `-n` as the row limit.
- `GASH_*` settings in `~/.gash_env` accept inline comments, so the
  template's lines work as soon as they are uncommented.
- Headless mode applies `GASH_DB_TIMEOUT`, `GASH_AI_CONFIRM` and
  `GASH_AI_MODEL_*` from `~/.gash_env`.
- The prompt follows `NO_COLOR` and `GASH_NO_COLOR`.
- AI requests retry 429/5xx/529 up to three times, honouring `Retry-After`.
- Installer: refuses to overwrite local changes, reports tag signatures
  (`GASH_REQUIRE_SIGNED=1` enforces them), `GASH_INSTALL_REF` installs a
  branch or commit, custom `GASH_DIR`, `--assume-yes` for non-interactive
  installs.
- Documentation: a function reference generated from the help registry
  (`docs/reference/`), configuration, security model, troubleshooting and
  migration guides; a test fails when the reference or any cited function
  or setting drifts from the code.
- CI on Ubuntu 22.04 and 24.04 with gawk and mawk, Bash 4.4, 5.0, 5.1 and
  5.3, and macOS (best effort); shellcheck and shfmt gates.

### Changed

- Startup is faster: about 130 ms interactive (1.5.3: about 325 ms) and
  60 ms headless (about 205 ms). The prompt runs one `git status` per prompt
  inside a repository and no other subshell for git.
- The prompt is prepended to `PROMPT_COMMAND` (string or array form) and
  keeps `$?`, so direnv, VTE and bash-preexec keep working.
- `llm.sh` and `ai.sh` are split into submodules under `lib/modules/llm/`
  and `lib/modules/ai/`.
- Every public function answers `--help` without side effects.
- `docker_compose_check` and `docker_compose_upgrade` read services through
  `docker compose config` (anchors, `extends`, `include`, `.env`), with a
  YAML-subset fallback at any indentation; multi-arch images compare the
  right digest.
- `gash_upgrade` and the installer pick the highest version tag.

### Fixed

- `llm_ports --listen` was ignored; `llm_project` reported tests for every
  Go module; `llm_env --filter` took the flag as the pattern.
- `llm_tree --stats` understated sizes at the depth limit.
- JSON arrays lost all but the first element on jq 1.6 (Ubuntu 22.04).
- `--null` output was truncated with mawk; `hgrep --limit`/`--reverse`
  needed gawk.
- `query_time` read `0µs` on Bash 4.4.
- `git_add_tag`/`git_delete_tag` matched tags by prefix (`v1.0` found
  `v1.0.1`).
- RETURN traps stayed armed in the user's shell after a function returned.
- Re-sourcing Gash baked the `cp -iv`/`rm -Iv` aliases into function bodies.
- A value flag at the end of the command line looped forever; it now
  returns `missing_value`.
- `docker_compose_upgrade` no longer changes the caller's directory.
- Installer aborted under `set -e` in a repository without tags.
- Functions with a `--no-color` flag (`disk_usage`, `gash`, `sysinfo`,
  `docker_compose_*`) never printed colors, even on a terminal.
- The prompt counted a finished job after any function whose cleanup trap ran
  a program (Bash before 5.3).
- `files_largest`, `dirs_largest` and `dirs_find_large` printed
  `sort: write failed: Broken pipe` where SIGPIPE is ignored.
- `gash_ssh_auto_unlock` re-added keys the agent already held.
- `llm_procs` ignored unknown options.
- The `~/.gash_env` permission check warned about mode 700; warnings named
  `~/.gash_env` even when `GASH_ENV_FILE` pointed elsewhere, and
  `gash_env_init` and `gash_uninstall` ignored `GASH_ENV_FILE`.

### Security

- `llm_db_*` validate SQL with a lexer that follows each engine's quoting
  rules, refuse client commands, executable comments, second statements and
  file functions, then run in a read-only session (SQLite `-readonly
  -safe`, MySQL and PostgreSQL read-only transactions with row caps and a
  statement timeout). Passwords travel in a pipe, not on the command line.
- Secret files are matched case-insensitively and through symlinks
  (`.ssh`, `.aws`, key and credential names), and are excluded from
  `llm_grep` and `llm_find` content searches.
- Forbidden paths are checked as written and as resolved (macOS
  `/private/etc`), by whole path component, and now also on every argument
  of `llm_exec`.
- `llm_git_*` and the `git` of `llm_exec` ignore repository configuration
  that runs programs (fsmonitor, hooks, external diff, textconv, pager).
- `psql` reads the SQL from stdin, like the other clients: queries no longer
  appear in `ps`.
- `~/.gash_env` warnings never print any part of the offending line.
- Without ripgrep, the `grep` fallback of `llm_grep` and `llm_find
  --contains` skips nested secret directories such as `.config/gcloud`.
- `remote_*` normalizes paths before its deny and ask lists (`~root` is
  `/root`, `~user` is `/home/user`), asks before touching any `.ssh`
  directory on the other side, and never treats an rsync `--delete` into the
  remote home itself as safe.
- AI requests redact private keys, URL passwords, `KEY=value` secrets and
  API tokens; the body and the API key never appear in `ps`; terminal
  control characters are stripped from replies; `ai_sysinfo` asks before
  sending collected data (`GASH_AI_CONFIRM`).
- `gash_uninstall` removes only a directory that holds Gash, never `/` or
  `$HOME`.

### Removed

- The hand-written function card (`gash` builds it from the help registry).
- The forced `umask 022` and the automatic command replacements (see
  Breaking).

## [1.5.3] - 2026-07-11

### Changed

- `hgrep` rewritten as a single gawk pass, with grouping and an fzf picker.
- Color gating centralized in the AI, system and prompt modules.

## [1.5.2] - 2026-07-11

### Added

- Native remote host operations over SSH, scp and rsync (`rex`, `rpush`,
  `rpull`, `rspush`, `rspull`).
- PHP and Node project version detection.

## [1.5.1] - 2026-06-10

### Fixed

- Headless hardening: errexit-safe pipelines and collectors, JSON built with
  jq, secrets kept off argv, a SQL `LIMIT` guard, locale-safe numbers,
  diagnostics on stderr, and a spec for the headless contract.

## [1.5.0] - 2026-04-21

### Added

- `tree_stats`: totals, top extensions, depth and empty directories in one
  walk.
- `llm_tree` nested JSON with `--depth N` and `--stats`.
- Color gating for agents and pipelines: `GASH_HEADLESS`, `NO_COLOR`,
  `GASH_NO_COLOR`, `--no-color`, and no colors when stdout is not a TTY.

### Changed

- Filesystem scanners rewritten: JSON envelopes and a single O(N) walk.

[2.1.0]: https://github.com/mauriziofonte/gash/compare/v2.0.0...v2.1.0
[2.0.0]: https://github.com/mauriziofonte/gash/compare/v1.5.3...v2.0.0
[1.5.3]: https://github.com/mauriziofonte/gash/compare/v1.5.2...v1.5.3
[1.5.2]: https://github.com/mauriziofonte/gash/compare/v1.5.1...v1.5.2
[1.5.1]: https://github.com/mauriziofonte/gash/compare/v1.5.0...v1.5.1
[1.5.0]: https://github.com/mauriziofonte/gash/compare/v1.4.9...v1.5.0
