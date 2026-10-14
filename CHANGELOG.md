# Changelog

All notable changes to Gash are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Gash follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html): the public API is
the set of documented functions, the `--json` envelopes, the error contract
and the `GASH_*` settings (see [AGENTS.md](AGENTS.md)).

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

[2.0.0]: https://github.com/mauriziofonte/gash/compare/v1.5.3...v2.0.0
[1.5.3]: https://github.com/mauriziofonte/gash/compare/v1.5.2...v1.5.3
[1.5.2]: https://github.com/mauriziofonte/gash/compare/v1.5.1...v1.5.2
[1.5.1]: https://github.com/mauriziofonte/gash/compare/v1.5.0...v1.5.1
[1.5.0]: https://github.com/mauriziofonte/gash/compare/v1.4.9...v1.5.0
