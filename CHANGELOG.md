# Changelog

All notable changes to Gash are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Gash follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html): the public API is
the set of documented functions, the `--json` envelopes, the error contract
and the `GASH_*` settings (see [AGENTS.md](AGENTS.md)).

## [3.4.0] - 2026-10-26

Gash becomes the toolkit an AI agent reaches for instead of raw shell
pipelines: 36 new functions and new options on most of the 3.0 ones, built
from how agents actually use a shell, each answering in one call what took a
pipeline or several round trips, with compact JSON (or TSV) and output that
stays inside a budget; most of them also run on a registered ssh host. Every read stays
in the read-only `llm_*` family; every change is a separate `agent_*` write
tool. Gash also learns to work with Windows from inside WSL, and to reclaim
the disk space that log files quietly eat on any host. Reading Windows is
automatic; changing it is human-only and goes through a UAC prompt.

### Added

- **Shared output options** for the agent functions (AGENTS.md 3.6):
  `--format json|jsonl|tsv|table` (TSV with a header and `\N` for null,
  metadata as one JSON line on stderr), an output budget (`--width`,
  `--max-lines`, `--head`, `--tail`, with a `[... K lines omitted]` marker),
  `--relative`/`--absolute`, `--count [--by KEY]`, and `-j N`/`--timeout S`
  on every multi-target function (results keep the input order; network
  probes default to 32 at a time).
- **Files and search**: `llm_find --count/--sort/--newer/--since/--tracked`,
  `llm_tree --count` and `--top N` (disk usage, a parallel walk),
  `llm_grep --exclude/--only-matching/--distinct/--count/--files-only`.
- **Text reads**: `llm_stat` (metadata of many paths), `llm_count` (many
  patterns, one pass per file), `llm_assert`, `llm_read` (ranges or the
  lines around a match), `llm_log` (read from the end, by pattern, time and
  level), `llm_hash`, `llm_textstats`, `llm_textcheck`.
- **Data**: `--format` and `--scalar` on every `llm_db_*` function,
  `llm_db_tables --counts`, `llm_db_schema --all`, `llm_db_databases`;
  `llm_json` (a value, the keys or the shape), `llm_csv`, `llm_config
  --path` for YAML, TOML, INI and JSON, `llm_dotenv`, `llm_date`.
- **Git**: `llm_git_overview` (the state of a repository in one small
  object), `llm_git_show`, `llm_git_status --summary/--files/--repos/
  --fetch`, `llm_git_diff --summary/--files/--numstat/--patch` and
  `REV..REV`, `llm_git_log` filters and `--fields`.
- **Network probes** (read-only): `llm_http` (status, timings, headers, a
  JSON value; credentials only through `--auth-env`), `llm_dns` (a built-in
  resolver client), `llm_tls`, `llm_tcp`, `llm_ports --port`.
- **Processes**: `llm_wait` (one call instead of a sleep and poll loop: a
  log line, a file, a process gone, an HTTP answer, a port, a command),
  `llm_procs --pid/--alive/--tree`, `llm_service`, `llm_docker_ps`,
  `llm_docker_logs`, `llm_docker_exec` (read-only), `llm_sysinfo`.
- **Runtimes, lint and tests**: `llm_tools`, `llm_runtime` and `gash runtime`
  (a PHP or Node version resolved to its binary without loading nvm.sh),
  `llm_lint` (php, sh, js, json, yaml, toml, py in parallel), `agent_test`
  (a test suite summarized: counts and each failure's file:line; a write
  tool, since a suite runs the project's code), `gash run` (a command with
  its output shaped and its exit status kept).
- **Remote hosts** (AGENTS.md 3.12): `--host ALIAS` (repeatable) on 31 read
  functions runs the same call on a registered `HOST:` alias over ssh in one
  round trip, several hosts in parallel; Gash ships its own binary once to a
  host of the same OS and architecture (`~/.cache/gash/remote/`), and
  `llm_find`, `llm_grep`, `llm_tree` and `llm_config` fall back to POSIX sh
  elsewhere. `llm_remote_run` (one command on several hosts, or several
  commands on one host in one round trip, each checked by the `llm_exec`
  rules first), `llm_same` (is the deployed copy the local one: short
  sha256 per file, one round trip per target, optionally inside a
  container), and `llm_db_*` through a `+ssh` connection
  (`mysql+ssh://USER:PASS@ALIAS/DB`, `pgsql+ssh://`, `sqlite+ssh://`,
  optionally `+docker:CTR`), which runs the engine's client on the host.
- **Write tools, the `agent_*` family** (AGENTS.md section 8): `agent_replace`
  (exact replacements in explicit files, all or nothing), `agent_git_commit`
  (stage explicit paths, commit, push, never forced), `agent_kill`,
  `agent_docker_exec`, `agent_put` (upload one file to a host: size and
  sha256 checks, optional backup, atomic rename), `agent_test` (a test
  suite, and nothing else, run and summarized), and `agent_replace
  --host`. Each takes `--dry-run`, names its targets explicitly
  and keeps the secret-file and protected-path rails; system paths are
  refused and sensitive ones need `--force`. Its own card topic, `gash
  agent`.
- **Routing**: `gash agents-hint` prints the "raw command -> Gash function"
  table for an agent's instructions (also AGENTS.md section 7), each help
  page opens with the shapes it replaces, `gash agent-functions` lists the
  agent functions for wrappers, and every agent function runs as `gash FN`
  without sourcing.
- `GASH_TELEMETRY_FILE`: an opt-in local JSONL line per agent call
  (function, time, output size, status, truncation; never arguments).
- The agent benchmark kit (`bench/agent/`): the raw shell shapes against the
  Gash functions, with relative acceptance rules.

- `gash wsl`: report and repair the WSL environment. `info` summarises the
  detection (version, distro, interop, drives, networking) and exits 0 off
  WSL; `doctor` checks interop, mounts and configuration; `path` converts a
  path between its Linux and Windows forms (`--windows`, `--mixed`,
  `--linux`); `config` shows the effective `wsl.conf` and `.wslconfig`;
  `interop --export` prints the `export WSL_INTEROP=…` line that repairs a
  stale socket after a reattached `tmux` or `screen`.
- `gash win`: inspect the Windows host over interop, read-only, with
  eye-candy for a human and `--json` for an agent — `info`, `disk`, `net`,
  `updates`, `defender`, `policy`, `hosts`, `doctor`, `services`, `procs`,
  `apps`, `reg`, `events`, `ports` and `file`. Windows programs are found by
  absolute path under `System32` (so interop works with
  `appendWindowsPath=false`), the system drive is discovered from the mounts
  (not hard-wired to `/mnt/c`), and every call is bounded by
  `GASH_WIN_TIMEOUT` (default 10s, max 120s). PowerShell runs fixed,
  read-only scripts; arguments reach them as data, never as code.
- `gash win fix`: the one path that changes Windows, deliberately narrow. It
  is human-only (a headless caller is refused with `elevation_required`),
  always goes through a UAC prompt, prints the exact PowerShell command and
  asks first, and acts only from a fixed allowlist (`flush-dns`,
  `restart-service`, `hosts-add`, `hosts-remove`) with every argument
  validated. `--dry-run` prints the command without running it, so an agent
  can hand it to a human.
- `gash logs`: find and reclaim disk space from log files. `scan` reports
  the logs eating disk, grouped by category (`*.log`, log-shaped `*.txt`,
  rotated and compressed copies, and the patterns Apache, nginx, PHP, Node,
  Python, package managers and databases write); `clean` deletes rotated and
  compressed logs and, with `--truncate`, empties live logs in place so a
  running daemon keeps its open file. `--sudo` reclaims root-owned logs
  through an explicit `sudo`. See [docs/logs/README.md](docs/logs/README.md).
- `GASH_WIN_TIMEOUT` bounds one `gash win` call (default `10s`, cap `120s`).
- **Prerequisites**: one catalog of every external program Gash runs, what
  it is for, which functions need it and its package on apt, dnf (with
  EPEL where needed), yum, microdnf, pacman, zypper, apk and Homebrew
  ([docs/requirements.md](docs/requirements.md)). A function that needs a
  missing tool says so in one way: `prerequisite_missing` (`STOP`), or the
  tool's own code, with `missing` and `install` members (and `host`, or
  `container`, for a tool missing on a remote host); a person gets the
  explanation and the install command of this system. `gash_doctor
  --prereqs` lists every tool by area, `--json` for scripts, and
  `gash_doctor --install [--all] [TOOL...]` installs the missing ones after
  asking; it never installs for an agent (`install_needs_confirmation`).
- **Installer**: `install.sh --check` reports what Gash needs (Bash 4.4+,
  git, curl or wget, tar, gzip, sha256sum or shasum, grep, sed) and the
  command that installs it; an install offers to install it first
  (`--assume-yes` installs those required tools only). A Bash older than
  4.4 is refused with a clear message, and a failed binary fetch ends with
  the `install.sh --repair` line instead of "Enjoy using Gash".
- **Supported systems**: CI on arm64 (unit, black-box and smoke), an
  install smoke test on Debian, Ubuntu, Rocky Linux 8 and 9 (minimal),
  AlmaLinux 9 and 10, Oracle Linux 9, Amazon Linux 2023, Fedora, openSUSE
  Leap and Tumbleweed, Alpine and Arch, on amd64 and arm64 (Arch on amd64
  only; each installs every package of the catalog and checks it provides
  its tool), and the
  local web stack end to end on AlmaLinux 9 too. The table is in
  [docs/install.md](docs/install.md).
- **Documentation**: rewritten. One guide per area with a generated
  reference of its functions ([docs/](docs/README.md)), an install page,
  the requirements, configuration, security and troubleshooting pages, and
  a new README and AGENTS.md. The examples are run by the tests and their
  output checked; links, function and setting names, and the generated
  parts fail CI when they go stale.
- `llm_date` carries its own time zone database, so `--tz` works where the
  system has none.
- **Protected paths of your own**: `PROTECT:content=PATH` and
  `PROTECT:all=PATH` lines in `~/.gash_env`, for this machine and every
  host, or for one host with `PROTECT:all@ALIAS=PATH`. Under a `content`
  path the agent functions see names, metadata and digests, never the
  content (`secret_file_blocked`); an `all` path is refused as a whole
  (`forbidden_path`) and left out of every listing, search and git output.
  Every `llm_*` and `agent_*` call holds them after resolving links, on
  this machine and on a host (as written before connecting, then on the
  host). See [docs/configuration.md](docs/configuration.md#protected-paths).
- `GASH_REMOTE_NO_BINARY` (in `~/.gash_env` only): host aliases, or `*`,
  that never get the gash binary. `--host` calls there answer through the
  POSIX fallbacks or with `remote_binary_unavailable`, and nothing is
  written on the host.
- The version `project` for `--runtime` (`gash run --runtime php:project`,
  `node:project`) and `llm_runtime`: the version the project pins
  (`.nvmrc`, `.node-version`, `package.json` engines; `.target-php-version`,
  `composer.json` `require.php`). A project that pins nothing is
  `runtime_not_pinned` (`STOP`) instead of a silent default.
- `gash run --plain` drops escape sequences (colors, cursor moves, titles)
  and keeps the last state of a line redrawn with carriage returns
  (progress bars), before the budget and `--test-summary` read the output.
- `agent_test --runtime NAME:VERSION` (repeatable) puts that runtime first
  on the suite's PATH, as `gash run --runtime` does.

### Changed

- **A missing tool is reported, never misread.** Among the cases that
  blamed something else or failed silently: the `git_*` functions said "Not
  in a git repository" and the `gl*` family exited 127 without a word;
  `process_find`, `process_kill` and `services_stop` blamed the process or
  went on without `sudo`; `remote_exec` without ssh exited 127 silently and
  every remote call without a local ssh was `host_unreachable` (`RETRY`);
  a missing docker for a `+docker` connection was `container_unavailable`,
  and a psql older than 12 or a sqlite3 older than 3.33 on a `+ssh` host
  was an SQL error; `gash_upgrade` blamed the network for a missing git;
  verifying a signed tag without gpg or ssh-keygen blamed the keyring;
  `hgrep --pick` without fzf, and `archive_extract`, had messages of their
  own. On macOS the Xcode stub of git or python3 counts as missing.
- **Fewer tools needed.** `jq` is no longer used (`llm_deps` reads
  `requirements.txt` and `go.mod` itself); `llm_procs` lists processes
  from `/proc` without ps, and on macOS; `llm_ports` reads BSD netstat on
  macOS; `llm_exec` uses Homebrew's `gsed`; `sysinfo` and `ai_sysinfo`
  fall back to netstat, `ps aux` and `df -h`, read host, kernel, cores,
  disk and address in-process, and print a note for each block whose tool
  is missing or that needs a sudo it cannot get; `gash_unload`,
  `gash_username` and the `~/.gash_env` permission check work without awk,
  grep, id or stat.
- `llm_service` adds a `log_note` when the journal cannot be read, and
  `llm_procs` without a filter lists the first 20 processes by PID with
  user, cpu and mem.
- `gash_ssh_auto_unlock` returns 1 when the keys cannot be unlocked, and
  `gash_uninstall` stops, removing nothing, when it cannot edit the profile.
- `gash_db_test` names the query to run for a `sqlite`, `+ssh` or
  `+docker` connection (it said "Unknown driver"), and the warning for a
  `DB:` URL with an unknown scheme names the `+ssh` forms.
- `glf FILE` takes `git log` options after the file; `gl`, `gla`, `glg`
  and `gls` end their output with a newline; a git action refused for lack
  of a terminal no longer says "headless mode".
- The `sysinfo` overview shows the disk share as `df` does (used of used
  plus available), so the space reserved for root, or outside a
  container's allowance, no longer hides a full disk.
- `llm_exec` refusing a command that changes files (`rm`, `mv`, `chmod`,
  ...) points at the `agent_*` functions instead of the allowlist.
- `gash_doctor --prereqs` finds Apache and PHP-FPM where `gash hte` does
  (`/usr/sbin`, `php-fpm8.3`), and installs `mod_ssl` and `mod_http2` with
  `httpd` on the Fedora family.

- The per-shell server runs the new read functions, so a sourced shell calls
  them without starting a process.
- WSL is detected from `wsl.conf`, the `WSLInterop` binfmt handler, or the
  `microsoft` marker in `/proc/version`, so a distribution without a
  `wsl.conf` is still recognised. The Windows PowerShell path the HTE and
  system helpers use is resolved from the mounted system drive rather than
  assuming `/mnt/c`.

### Fixed

- `llm_lint` passed PHP files it never checked: before PHP 8.3, `php -l`
  checks only the first file it is given, so a syntax error in any other
  file of a batch went unreported. Each file now gets its own verdict: PHP
  8.3 and later still lint in batches, older versions run once per file.

### Security

- `GASH_LLM_EXEC=unrestricted` applies to `llm_exec` alone. `llm_remote_run`,
  `llm_docker_exec`, `llm_wait --cmd-ok` and `gash run --safe` always check
  their command with the `llm_exec` rules (parser, allowlist, secret files,
  forbidden paths); with the setting on, they ran any command line.
  `GASH_LLM_EXEC_ALLOW` still applies to all of them.
- `llm_*` stays read-only with no exception: writes live only in `agent_*`;
  `llm_http` refuses any method but GET, HEAD and OPTIONS; `llm_procs` has
  no kill. Running a test suite executes the project's code, so it is
  `agent_test`, which runs a test suite and nothing else (`gash run` runs
  any other command).
- Remote calls take registered `HOST:` aliases only. Remote paths pass the
  local rails and the transfer deny list before anything connects and are
  checked again on the host after links resolve; a secret file never
  leaves the host and the local stdin is never sent. Over `+ssh` the SQL
  guard runs before ssh and the password goes on ssh's stdin, never in an
  argument. The shipped binary is size-checked, test-run and renamed into a
  0700 cache directory.
- The path guards treat the Windows secrets reachable over `/mnt` as secret
  files: the user registry hives (`NTUSER.DAT`, `UsrClass.dat`), saved RDP
  credentials (`*.rdp`), answer files (`unattend.xml`, `autounattend.xml`)
  and the browser credential stores (`Login Data`, `logins.json`, `key4.db`,
  `key3.db`). See [docs/security.md](docs/security.md).
- The remote secret-file and `.ssh` rails no longer need `tr` on the host,
  and a symlink on a host with neither realpath nor readlink is refused
  instead of judged by its own name: without them both failed open.
- `archive_extract` refuses a `.tar.xz` when xz is missing (macOS
  extracted it unchecked), and a tar whose member headers it cannot read
  (tar would skip the header and extract what follows unchecked).

### Removed

- The `jq_not_found` error code: no function needs jq any more.

## [3.2.0] - 2026-10-25

Gash gets out of the way: a shell loads in half the time, the prompt no
longer forks when there is no git repository, and a loaded shell runs the
read-only functions in a fraction of a millisecond instead of starting the
binary for each call. Behaviour is unchanged (each change is proven against
the path it replaces).

### Added

- A per-shell server (Linux): the first call of a served function (the
  `llm_*` read-only functions, the file and disk reports, `gash_help`,
  `gash_username` and the like) starts `gash __serve`, which runs that call
  and the later ones with the shell's environment, directory and standard
  streams, Ctrl-C and broken pipes included, and ends with the shell. A call
  it cannot run exactly as the binary would (in a subshell or `$(...)`, with
  a stream it cannot share, after `TZ` changed) runs the binary as before;
  `GASH_SERVER=0` turns it off. See docs/security.md, "The per-shell
  server".

### Changed

- Loading is faster: a function longer than a few hundred bytes is a
  one-line stand-in that reads its definition from `lib/compat/cold/` at its
  first call (a load parsed some 80 KB of functions a shell mostly never
  calls), the core shims no longer source the bootstrap again when gash.sh
  just did, and `gash_doctor` checks the cold definitions with the shims.
- `gash init bash` keeps the answers of its probes (`dircolors`,
  `grep --color`, `ls --color`, `rm --version`) in
  `${XDG_CACHE_HOME:-~/.cache}/gash/init-probes`, as 2.x did, and reuses
  them while TERM, the locale, PATH and the dircolors database are the same,
  no PATH directory changed and the file is less than a day old.
- The prompt counts the running and stopped jobs with `\j` instead of a
  subshell: no fork per prompt outside a git repository.
- `llm_grep` skips a file or line that lacks the pattern's literal prefix
  before the regexp runs, and the ignore globs of the form `**/NAME` (all the
  secret excludes) are matched without a regexp; compiled globs are kept for
  the process.
- Regular expressions and templates of the binary are compiled at their
  first use instead of at every start, the `gash init bash` probes run side
  by side, and `gash_username` reads `/etc/passwd` when the name service
  takes it from files.

## [3.1.0] - 2026-10-25

Releases are now signed and verifiable without any key to keep, and the
guardrails are fuzzed, mutation-tested and held to coverage floors. Two of
the fuzzers found real gaps, closed here (see Security).

### Added

- Signed releases without a signing key: the release workflow signs
  `checksums.txt` with Sigstore (keyless, `checksums.txt.sigstore.json`),
  attaches an SPDX SBOM per archive and SLSA build provenance
  (`gh attestation verify`). docs/security.md, "Verifying a release",
  shows how to check a download by hand.
- `GASH_REQUIRE_SIGNED=1` accepts an unsigned release tag when `cosign`
  verifies the Sigstore signature of its release for the release workflow,
  the tag and the commit the tag points at (the installer, `gash update`
  and `gash_upgrade`); without `cosign` the refusal says how to install it.
  With `cosign` on `PATH` the signature is checked and reported without
  `GASH_REQUIRE_SIGNED` too.
- A **Tag release** workflow that checks the commit (CI green,
  `release/SHA256SUMS` current, a CHANGELOG entry), tags `v<VERSION>` and
  starts the release; the release notes are the CHANGELOG entry.

### Changed

- The lint gate adds unused, gosec, errorlint, bodyclose, noctx,
  copyloopvar, durationcheck and makezero; the dead code they found (the
  unported grep helpers of the dev module, test helpers of the removed 2.x
  comparisons) is gone, and errors are matched with `errors.Is`/`errors.As`.
- CI holds the guardrail packages (`internal/sqlguard`,
  `internal/llmexec`, `internal/pathguard`, `internal/hostshelper`) to
  statement coverage floors, and a weekly **Mutation** workflow runs
  go-mutesting on them against score floors; both floors only go up.
- `SECURITY.md`: 2.1.x gets critical and high security fixes until
  2027-04-30; a fix for a critical or high report ships within 30 days.

### Security

- `llm_db_*`: a `PRAGMA` passes the SQL guard only as a read (`PRAGMA name`
  for a pragma that reports a value, or `PRAGMA table_info(t)` and the other
  schema lookups). `PRAGMA user_version = 5`, `PRAGMA journal_mode(WAL)` and
  the pragmas that act when run (`optimize`, `wal_checkpoint`,
  `incremental_vacuum`, ...) are refused with `write_operation_blocked`
  (2.x and 3.0 accepted them; the read-only SQLite session still stopped
  the write). Found by fuzzing.
- `llm_exec` refuses a command holding a NUL byte (`construct_not_allowed`)
  instead of rendering a word bash would cut at it. Found by fuzzing.
- `gash hte install` refuses to install the hosts helper, the sudoers
  drop-in or the macOS pf rules below a directory that a user other than
  root can write (from which the helper, which sudo runs as root, could be
  swapped), and creates its temporary files exclusively, so a planted
  symlink is refused instead of followed as root.
- Seven fuzz targets cover the guardrails: the SQL guard (with an oracle
  that runs every accepted query on a writable SQLite database and checks
  nothing changed), the `llm_exec` renderer (every rendered word is a
  quoted literal and re-parses to the validated words), the path guard
  (against the kernel's own symlink resolution), the hosts helper's block
  grammar and splicing, and the `~/.gash_env` parser (no warning shows
  content). CI fuzzes each for 15 seconds on every change and 5 minutes
  weekly.
- The workflows pin every action by commit SHA, keep no checkout
  credentials, use no module cache in the publishing jobs and grant each
  job only the permissions it needs; actionlint and zizmor check them in CI.
  Dependabot updates the Go modules and the actions weekly. OpenSSF
  Scorecard and CodeQL run on every change and weekly, and the CI runs
  weekly too, so a new advisory or a distro change shows without a commit.

## [3.0.0] - 2026-10-24

Gash is now one static Go binary with thin generated Bash around it, for
Linux, macOS and WSL on amd64 and arm64.

- **Same functions.** Every 2.x function keeps its name, arguments, output,
  exit codes and `--json` envelopes, except for the changes below. The
  prompt, history, aliases, completion and toolchains still come from Bash
  (`gash init bash`).
- **`gash hte` is built in.** HTE, formerly the separate hte-cli tool, runs a
  user-owned Apache and PHP-FPM with no sudo at runtime, and
  `gash hte migrate` takes over an hte-cli setup
  ([docs/hte/README.md](docs/hte/README.md)).
- **Fewer dependencies.** The `llm_*`, database and scanner functions run
  inside the binary: no `jq`, `sqlite3`, `mysql`, `psql`, `rg` or GNU `find`.
- **Verified upgrades.** Release binaries are pinned by SHA-256 in the
  tagged tree; `gash update` verifies before it changes anything and
  `gash update --rollback` goes back.
- **Audited.** The port reviewed every 2.x behaviour (the census is
  [dev/port/LEDGER.md](dev/port/LEDGER.md), 443 items) and fixes the
  defects it found instead of carrying them over. The changes a user or a
  script can notice are listed below.

To upgrade, run `gash_upgrade` from 2.x; `git checkout v2.1.1` in the
install directory goes back. From 1.x, go through 2.1.0 first (see
Breaking).

### Breaking

- Upgrading from 1.x goes through 2.1.0: `gash_migrate` prints that route
  (check out `v2.1.0`, run `gash_migrate`, then `gash_upgrade`) instead of
  migrating.
- The `hte`, `hte-create`, `hte-remove` and `hte-details` aliases are gone:
  use `gash hte`. There is no hte-cli compatibility command; `gash hte
  migrate` takes over an hte-cli setup.
- The `gp`, `gc`, `dc` and `gs` aliases are not defined when a program of
  that name is on PATH, module short aliases never hide a program, and a
  user alias always wins over a module alias.
- `process_kill` always asks before killing; `quit` no longer passes
  `--force`. `docker_stop_all` and `docker_start_all` list the containers
  and ask; `git_add_tag`, `git_delete_tag`, `git_sync` and
  `git_apply_patch` show a plan and ask. Without a terminal each of them
  needs `-y`/`--yes`, as `wsl_compact` now does too.
- Arguments are checked strictly: a second PATH fails with
  `unexpected_argument`, unknown options are refused everywhere, and the
  self-management commands exit 2 on a bad option or argument.
- `llm_*` contract changes: a missing PATH is `path_not_found` (before: exit
  0 with no output); an empty pattern is `missing_pattern`; `~` and `~/`
  expand and `~user` is `invalid_path`; a refused query reports the guard
  verdict of the connection's engine; allow-listed `llm_exec` commands keep
  the walk-root and symlink checks.
- `hgrep -n N` always shows the N most recent matches, with `--reverse` too.
- `SSH:` passphrases in `~/.gash_env` keep trailing whitespace (only a final
  `\r` is removed), with a warning.
- `archive_extract` refuses archives with absolute, `..` or outside-pointing
  link members before extracting anything.
- New error codes in the agent contract: `invalid_db_url`,
  `invalid_manifest`, `invalid_path`, `invalid_service_name`.

### Added

- `gash version [--json]` and `gash init bash`, which prints the
  interactive Bash layer that `gash.sh` evaluates.
- `gash hte`: install, sites, certificates, PHP-FPM pools per PHP version,
  `doctor`, `migrate` from hte-cli and a clean `uninstall`; see
  [docs/hte/README.md](docs/hte/README.md). Ports 80 and 443 are made bindable by the
  user through `net.ipv4.ip_unprivileged_port_start`, asked for and undone
  by uninstall.
- `gash update` installs and verifies the new release's binary before it
  touches the checkout, loads the new version in a test shell and goes back
  to the previous version when that fails; `gash update --rollback` returns
  to the previous version, whose binary is kept. `gash_upgrade` runs the
  same verified steps with the 2.x output lines and exit codes.
- `install.sh --repair` fetches and verifies only the binary for the
  checked-out `VERSION` (no git, no profile or `~/.gashrc` edit), works
  offline from a `file://` release, and exits 1 on failure, keeping the
  binary in place. `GASH_BIN` and `GASH_RELEASE_URL` point Gash at another
  binary or release server.
- `gash hte`: with `autostart = "shell"` in `hte.toml`, an interactive shell
  that loads Gash starts the stack in the background when it is down,
  silently and without ever failing the shell (the reason of a failure goes
  to `~/.local/state/gash/hte/logs/autostart.log`).
- Release binaries, `gash-hosts-helper` included, are pinned in the tagged
  tree (`release/SHA256SUMS`); the bootstrap, `gash update` and the installer
  install a binary only once it matches its pin:
  with `GASH_REQUIRE_SIGNED=1`, a signed tag covers the binary too.
  `gash_doctor` checks the binary and the generated shims.
- `ip_public --raw`; `--dry-run` on the git tag and patch commands;
  `--exclude=PAT` on `remote_sync_*`; `nvm_prune_versions -y/--yes`.
- `gash <Tab>` completes card topics and commands; the docker, git, remote
  and sysinfo completions offer the real flags.
- `GASH_DB_TIMEOUT` also bounds `gash_db_test`, which prints the driver's
  error as a hint when the connection fails.
- `gash hte site create|modify --http2` turns HTTP/2 back on after
  `--no-http2`; every `gash hte` command has its own `--help`, and
  [docs/hte/README.md](docs/hte/README.md) has a WSL section.

### Changed

- The `llm_*`, database, scanner and search functions need no `mysql`,
  `mariadb`, `psql`, `sqlite3`, `jq`, `rg`, `numfmt` or `find`: they run
  inside the binary. Client option files (`~/.my.cnf`) and `~/.pgpass` are
  not read.
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
  `node_detect_version`, `wsl_compact`, `sysinfo`, `gash_doctor` settings)
  goes through one renderer; plain text is the same.
- Questions look alike: `Question? [y/N]` (default No) or
  `Do you want to continue? [Y/n]` (default Yes), on stderr. Before,
  confirmations read `(y/N): `.
- Usage reminders after a wrong call are one `Usage: ...` line on stderr
  (before: `Error: Usage: ...`, or the whole help page on stdout for
  `git_apply_patch`).
- `ai_sysinfo` findings read `Error:`, `Warning:`, `OK:`, `Info:` when piped
  or saved (before: `[!]`, `[+]`, `[i]`); on a terminal they are glyphs.
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
- Scanners (`files_largest`, `dirs_largest`, `dirs_list_empty`,
  `dirs_find_large`, `tree_stats`): paths with a tab or newline are left out
  the same way everywhere; `--exclude GLOB` has one meaning (a name, or a
  path below PATH); numbers take at most 9 digits and are never evaluated;
  size units are strict; `--min-depth` filters only the output; an
  unreadable root is `path_unreadable` and unreadable entries count in
  `errors`; `-L` detects directory loops; JSON escapes every control
  character; no truncated paths without GNU `du`.
- `archive_extract`: `.Z`, `.tar.Z` and `.taz` extract instead of emptying the
  target; upper-case suffixes work; single-file formats replace the target
  only on success; OUTPUT_DIR is created only after the format, tool and
  members are checked, and may start with `-`; `7z` reads the archive name
  literally and `unzip`/`unrar` names with `* ? [` are refused; headless or
  without a terminal the tools never prompt or overwrite.
- `file_backup` reserves its name atomically (`_1`, `_2`), never writes
  through an existing file, link or directory, and removes a partial copy
  when `cp` fails.
- `llm_*`: every JSON path escapes control characters and turns invalid
  UTF-8 into U+FFFD; `llm_ports` gives one record per socket, sorted;
  `llm_procs --port` is numeric and wins over `--name`; `llm_git_log` keeps
  commits with control bytes; `llm_deps` reads `composer.json` and
  `package.json` itself and reports `invalid_manifest`; `llm_grep` and
  `llm_find --contains` search the secret-exception files (`.env.example`,
  `*.pub`); `llm_tree --text` lists in byte order, with symlinks as
  `name -> target`; `llm_project` detects manifests correctly;
  `query_time` measures the query only; a SQLite `EXPLAIN` returns an
  envelope.
- `llm_docker_check` follows the `llm_*` grammar and path rules, and
  exported secrets no longer reach Compose interpolation.
- Docker: `docker_compose_upgrade` lets Compose find its files (override
  files and `COMPOSE_FILE` honoured, `--remove-orphans` no longer removes
  override-only services); `image:tag@sha256` keeps its pin; Docker Hub
  names are normalized; interpolation follows Compose (defaults, required
  and alternate values, `$$`, `.env` precedence); `docker_compose_scan`
  always prints valid JSON; daemon errors are reported and prune counts
  are honest; a declined upgrade returns 1.
- Git: `git_list_tags` lists unpeeled tags and fails when `ls-remote` fails;
  an unreachable origin is an error; tags starting with `-` are refused;
  `git_dump_revisions` writes atomically and fails loudly;
  `git_apply_patch` returns to the starting branch on failure;
  `git_sync` pushes to the upstream; `git_repair` clones before deleting and
  deletes only empty objects.
- Remote: hosts must be `user@host` (a port goes in `~/.ssh/config`, with a
  hint) and are passed after `--`; remote paths with expandable characters
  are refused; `-e` quoting survives a space in `$HOME`; `~/.ssh` is created
  0700; `/.` is treated as `/` and denied.
- System: `process_kill` and `process_find` match names literally and
  report permission errors; `port_kill` kills only sockets bound to the
  port; `history_grep` and `hgrep` refuse an invalid regex, emit valid JSON,
  accept `--`, ignore `HISTTIMEFORMAT` and deduplicate; `mkdir_cd` takes
  exactly one literal name and ignores `CDPATH`; `sudo_last` refuses an
  empty history; `ip_public` validates the address with a 10 s timeout;
  `disk_usage` keeps the other rows when one mount fails; `sysinfo` asks for
  sudo only where needed and scopes its headers; `wsl_compact` reports an
  unknown `dmesg`.
- `ai_*`: a provider name alone selects the provider; `ai_ask` needs a
  terminal (use `ai_query`) and `exit`/`quit` cancel it; silent piped stdin
  is ignored after 2 s with a warning; the report file is written 0600;
  `GASH_AI_MODEL_*` and `GASH_AI_CONFIRM` values are validated, with a
  warning; `-h` works after the provider.
- `chrome_headless_update` resolves `GASH_CHROME_ROOT`, checks the version
  exactly, replaces an unverified copy, uses sudo only when the target is
  not writable and `sudo -n` when unattended.
- PHP and Node: OR constraints resolve to the highest installed version,
  major-only constraints count as X.0, `.target-php-version` is read from
  its first line, a detected PHP that is not installed warns, and only
  `vX.Y.Z` directories count as installed Node; `node_upgrade` and
  `nvm_prune_versions` exit 1 when an install or removal fails and ignore
  io.js entries; a missing `project_dir` warns.
- `~/.gash_env`: DB URLs accept bracketed IPv6 hosts; decoded passwords
  keep newlines, and a NUL is refused with `invalid_db_url`; a repeated
  `SSH:`, `DB:` or `HOST:` name is ignored with a warning naming both lines;
  messages name the file `GASH_ENV_FILE` points at; SSH key paths expand
  only `~/`.
- Shell: the user's `HISTCONTROL` is kept; `~/.bash_aliases` is sourced
  once; the `grep --color` alias is back; a load leaves `BASH_REMATCH` and
  readonly names alone; a symlinked `gash.sh` finds its tree; headless
  sourcing with a 1.x `~/.gashrc` is silent; control characters are
  replaced in the terminal title; `needs_confirm_prompt` without a terminal
  no longer prints the shell's `/dev/tty` error.
- Self-management: `gash_upgrade` never moves a checkout to an older
  release; `gash_uninstall` keeps a user-edited `~/.gashrc` and the profile
  backups and edits a symlinked profile through its link; `gash_unload`
  restores completion specs, exports and the user's `GASH_*` exactly; a tag
  counts as signed only when it ends with a signature block; a new install
  is verified in a temporary clone; the card lists only the aliases the
  shell defines.
- HTE: `gash hte doctor` lists the certificate a site kept after
  `site modify --no-https`, and `--https` reuses it; changing a site's
  aliases issues a certificate for exactly the names it serves.
- Help pages, `AGENTS.md` and `docs/reference` were corrected wherever they
  described 2.x behaviour wrongly (size grammar, envelope keys, error
  vocabulary, exclusions, which user configuration the git and archive
  tools honour).
- `llm_find` keeps its results when the root path holds a newline;
  `llm_find` and `llm_tree` list entries in byte order, so `-n N` and the
  500-entry cap cut at the same place.
- Inside `llm_exec`, grep, rg, diff and git search `.env` templates
  (`.env.example`, ...) and `id_*.pub` keys again; real secrets stay
  excluded.
- `remote_sync_push` and `remote_sync_pull` work with openrsync, the
  `/usr/bin/rsync` of macOS.
- `docker_compose_check` and `docker_compose_upgrade` refuse a `DIR/.env`
  with an unterminated quoted value, as Compose does (`invalid_env_file`),
  instead of reading it differently from Compose.
- `ai_sysinfo` prints the Containers and SSL private keys headings only
  with content, and Virtualmin certificates under their own heading.
- The scanners' `--exclude` hint shows `'node_modules'`, which matches at
  any depth.
- `gash hte install` works on Fedora and Arch without a running service
  manager (it failed because `service` is missing), accepts macOS's
  `#includedir /private/etc/sudoers.d` line (it refused every Mac), and no
  longer counts server configuration files without a `<VirtualHost>`
  (Fedora's `conf.d/welcome.conf`, Arch's `conf/extra`) as blocking vhosts.
- `gash hte migrate --apply --exclude <stock vhost> --yes` switches onto
  ports 80 and 443 when the excluded stock vhosts were all that blocked
  install and there is nothing to import (it reported "nothing to migrate");
  `gash hte uninstall` starts the system Apache again.
- `gash hte doctor` no longer reports `/etc/sudoers.d/gash-hte` missing where
  `/etc/sudoers.d` is not readable by the user (Fedora, Arch): it asks
  `sudo -n -l` instead.
- PHP discovery on macOS no longer reports a duplicate when Homebrew's `php`
  and `php@X.Y` links point to the same installation.
- A Ctrl-C that ends the child process before Gash records the signal exits
  130 (128+N), not 1.
- `gash <Tab>` completes the `gash hte` command tree.
- Gash no longer registers a completion for `nvm`, which replaced the user's
  own nvm completion and was then removed by `gash_unload`.
- `source ~/.gashrc` in a shell that ran 2.x loads the whole 3.x interactive
  layer (it stopped part-way with a syntax error), and `gash_unload` then
  keeps the user's exported `GASH_*` settings.
- The installer no longer reports "Installed gash" after a failed binary
  download, and `install.sh --help` lists `GASH_RELEASE_URL` and
  `GASH_REQUIRE_SIGNED`.

### Security

- The four 2.1.1 fixes are in 3.0 by design: the SQL guard (the PostgreSQL
  client is not used at all), the prompt that never runs text from a branch
  or environment name, `llm_grep --context` and `tree_stats` reading digits
  only.
- `llm_tree`, `llm_find` and `llm_grep` refuse walks from `/` or above a
  protected tree and skip protected entries below; `-L` never enters
  `/proc`, `/sys`, `/dev`, `/boot` or root's home; more files count as
  secrets (`/etc/shadow-`, `/etc/gshadow-`, `/etc/security/opasswd`,
  `credentials_*`, `*.tfvars.json`, `ssh_host_*_key`, `.envrc`).
- `llm_exec` checks attached short-option values against the protected
  paths and secret files, refuses patch options on `git stash list`, and
  handles `||`/`&&` across a line continuation.
- `ai_*` redaction also covers `Authorization` headers, indented or not
  `SSH:` passphrase lines and crypt(3) hashes; `ai_sysinfo` paths pass the
  `llm_*` path guard and each question's data is confirmed before it is
  sent. `llm_env` masks every URL password; the password of a DB URL with an
  empty user is masked too; WireGuard keys are always masked.
- `GITHUB_TOKEN` is exchanged for a ghcr.io token and never sent raw; other
  registries get anonymous Bearer tokens.
- The SSH agent search never changes permissions and refuses group- or
  world-writable directories; a stale `SSH_AUTH_SOCK` is recovered.

### Removed

- The Bash implementation of the modules and the 2.x test runner
  (`tests/run.sh`); the files left under `lib/` are generated shims.
- The 1.x migration (see Breaking) and the `hte*` aliases.

## [2.1.1] - 2026-10-18

A security release. Four ways untrusted text could reach code execution are
closed; ordinary use is unchanged.

### Security

- The read-only SQL guard could be bypassed on PostgreSQL. A quote written
  right after a word was mistaken for the start of an escape string, so text
  the guard then ignored, including a client command line, slipped past the
  keyword checks; on the `psql` client such a line can run a shell command.
  The guard now opens an escape string only for a standalone `e`/`E`, rejects
  any statement line that begins with a backslash, and treats `SET` as a
  blocked keyword.
- The interactive prompt no longer runs text from the current git branch
  name, the Python virtual-env path, the `debian_chroot` name or the GNU
  `screen` session as shell code. Each is now shown literally, so entering a
  repository, virtual-env, chroot or screen whose name contains shell syntax
  is safe. Ordinary names look exactly as before.
- `llm_grep --context` is validated as a plain integer instead of being
  evaluated as shell arithmetic. A non-numeric or crafted value is refused
  with `invalid_context` and no longer runs code or aborts a `set -u` caller.
- `tree_stats` no longer evaluates text taken from a scanned file name as
  shell arithmetic. A tab inside a file extension could shift that text into
  the per-extension count and size fields, which the terminal view then used
  in an arithmetic expression; scanning a directory someone else could write
  to (a cloned repository, an extracted archive, a shared folder) could run a
  command. Those fields are now read as digits only, so any other value counts
  as zero and is never evaluated; `--json` keeps numeric fields too.

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
Upgrading from 1.x is guided: see [the 2.1.0 migration guide](https://github.com/mauriziofonte/gash/blob/v2.1.0/docs/migration.md).

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

[3.4.0]: https://github.com/mauriziofonte/gash/compare/v3.2.0...v3.4.0
[3.2.0]: https://github.com/mauriziofonte/gash/compare/v3.1.0...v3.2.0
[3.1.0]: https://github.com/mauriziofonte/gash/compare/v3.0.0...v3.1.0
[3.0.0]: https://github.com/mauriziofonte/gash/compare/v2.1.1...v3.0.0
[2.1.1]: https://github.com/mauriziofonte/gash/compare/v2.1.0...v2.1.1
[2.1.0]: https://github.com/mauriziofonte/gash/compare/v2.0.0...v2.1.0
[2.0.0]: https://github.com/mauriziofonte/gash/compare/v1.5.3...v2.0.0
[1.5.3]: https://github.com/mauriziofonte/gash/compare/v1.5.2...v1.5.3
[1.5.2]: https://github.com/mauriziofonte/gash/compare/v1.5.1...v1.5.2
[1.5.1]: https://github.com/mauriziofonte/gash/compare/v1.5.0...v1.5.1
[1.5.0]: https://github.com/mauriziofonte/gash/compare/v1.4.9...v1.5.0
