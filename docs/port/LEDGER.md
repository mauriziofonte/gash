<!-- Gash 3.0 port ledger. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# LEDGER

Every defect, forced divergence, operator decision and doc drift found in Gash 2.1.0, and every intentional
difference between `gash hte` and hte-cli, with the exact fix recipe and the test that proves it. rc.A
reproduces every Defect (pure port, PRD 4.1); Forced divergences are the only places where rc.A differs from
2.x; the operator triages every item before 3.0.

## How to triage

* Each item ends with `| Status | open |`. Replace `open` with `accept`, `reject` or `defer`, optionally
  followed by a short note (`accept: but keep the old message`). Items left `open` count as `defer`.
* `accept` on a Defect: 3.0 applies the fix recipe and its proving test. On a Forced divergence: the 3.0 fix
  recipe (the rc.A behavior is already the divergence). On a Decision: the option written first in the fix
  recipe, unless the note names another. On Doc drift: the side the item says is right. On an HTE design delta:
  the design as written (`reject` asks for the hte-cli behavior instead).
* Severity: critical = code execution or secret disclosure reachable without consent; high = a safety rail
  bypassed, a destructive action without consent, or data loss; medium = wrong or invalid output, or a silent
  wrong result; low = cosmetic or edge case. Public-contract impact = yes when the fix changes a documented
  output, envelope, exit status, flag or error code.
* Reproductions are harmless (empty marker files in a temporary directory). Where a working payload would be
  a usable exploit of the released 2.1.0, the item describes the input shape instead and the reproduction is
  kept outside the repository (decision D-7 in `go-impl/design/README.md`).
* rc.A keeps every `--help` body byte for byte: a Doc drift item about help text is fixed in 3.0, while the
  prose docs rewritten for 3.x (`AGENTS.md`, `docs/`) describe what rc.A does.
* `TEST` items are defects of the Bash test harness, fixed on the branch before the specs run against Go
  (no Gash behavior and no spec expectation changes); triage confirms them.
* HTE design delta items marked "Decision for the operator" in their Section field need an answer before the
  HTE phase starts.
* Ids: the prefix names the area (`LLM` llm_* agent functions, `DB` databases, `EXEC` llm_exec, `SYS` system, sysinfo, ssh, chrome, `DOCK` docker and compose, `GIT` git module, `REM` remote, `DEV` php and node, `AI` ai, `FS` file scanners, `FILE` archive_extract and file_backup, `SHELL` shell layer, `SELF` self-management, `CORE` lib/core, `TEST` test harness, `HTE` HTE (design delta vs hte-cli)).

## Conventions per area

Every reproduction runs on a copy of the 2.1.0 tree, never on a working checkout or a real home directory
(`GASH_SRC` below is such a copy, for example `git archive v2.1.0 | tar -x -C "$GASH_SRC"`;
`HTE_CLI_SRC` is a checkout of hte-cli at v1.1.1). Paths of the form `work/...` are S1 scratch helpers
that are not in the repository; the item text says what they do.

### llm_* agent functions, llm_exec, databases (LLM, EXEC, DB)

Scope: `lib/modules/llm.sh`, `lib/modules/llm/{core,fs,git,project,system,exec,db}.sh`,
`llm_docker_check` in `lib/modules/docker-compose.sh`, shared helpers in
`lib/core/utils.sh`. IDs: `LLM-*` (llm-core), `EXEC-*` (llm_exec), `DB-*` (databases).

### File scanners (FS)

Scope: the five scanners of `lib/modules/files.sh` with their `__gash_fs_*` helpers and the core
helpers they call (`lib/core/utils.sh`, `lib/core/output.sh`). Source: the S1 audit notes (files-scanners)
(seeds 1-4, findings N1-N21). IDs: `FS-*`. Line numbers: main a4fbc8b (lib/ identical on
go-refactor). Documented path (lead decision): GNU find, GNU du, numfmt and gawk present, C locale;
in Go the walks, sizes and empty-directory tests are native (architecture.md section 7).

### archive_extract and file_backup (FILE)

Scope: `archive_extract` (alias `axe`) and `file_backup` (alias `fbk`) in `lib/modules/files.sh:1886-1991`, their help registrations (`files.sh:2220-2264`) and the core helpers they call. Source: the S1 audit notes (files-archive). Line numbers re-checked on `go-refactor` 9126836 (no difference from main a4fbc8b or from the dossier's cb16bc1 under `lib/`, `tests/`, `docs/`).

Conventions:

- **Reproduction**: `R '...'` runs headless Gash with a temporary HOME, in a fresh temporary directory, `TZ=Etc/UTC`, `LANG=C.UTF-8` (`RLANG`, `RPATH`, `RTZ`, `REXTRA` override). Every file a reproduction truncates or rewrites is a throwaway file inside that directory. `$W/fx1` held throwaway fixture archives built with the tools named in the item.

  ```bash
  R() { local d; d=$(mktemp -d) && (cd "$d" && exec env -i GASH_SRC="$GASH_SRC" HOME="$d/home" TMPDIR="$d" PATH="${RPATH:-/usr/local/bin:/usr/bin:/bin}" TERM=xterm-256color LANG="${RLANG:-C.UTF-8}" TZ="${RTZ:-Etc/UTC}" $REXTRA bash -c 'GASH_HEADLESS=1; source "$GASH_SRC/gash.sh"; '"$1"); }
  ```
- **rc.A** (lead decision): Go runs the same extraction tools with the same argv (tar, gunzip, bunzip2, xz, zstd, uncompress, unzip, 7z, unrar) and `mkdir -p`; for single-file formats it opens `OUTPUT_DIR/<name>` with `O_WRONLY|O_CREAT|O_TRUNC`, mode 0666, following symlinks, before starting the tool, so the shell's file effects reproduce. `file_backup` runs `cp -v -- FILE BACKUP`.
- **Proving tests** live in `internal/files` (`layout.md`); `TestLedger_FILEnn_2x` proves rc.A reproduces 2.x, `TestLedger_FILEnn_fix` proves the 3.0 fix and replaces it (`testing.md` 3).

### System, sysinfo, SSH, Chrome (SYS)

Source: the S1 audit notes (system) (seed 1, findings N1-N37) plus new items marked "new". Reproductions run from
the root of a 2.1.0 checkout; they touch only temporary directories, test stubs and processes they start
themselves, and never use real sudo, the internet or a real secret. Go packages follow
`design/layout.md`: `internal/system`, `internal/sysinfo`, `internal/ssh`, `internal/dev`, `internal/compatgen`.

### Docker, Compose, git, remote (DOCK, GIT, REM)

Every reproduction runs from a fresh temporary directory with these helpers (no network unless stated, no daemon writes, nothing outside `$T`):

```bash
T=$(mktemp -d)
g()  { env -i HOME="$T" PATH="$T/bin:/usr/local/bin:/usr/bin:/bin" LANG=C.UTF-8 T="$T" bash -c "GASH_HEADLESS=1; source $GASH_SRC/gash.sh; $1"; }
gm() { env -i HOME="$T" PATH="$GASH_SRC/tests/mocks/bin:$T/bin:/usr/local/bin:/usr/bin:/bin" LANG=C.UTF-8 T="$T" bash -c "GASH_HEADLESS=1; source $GASH_SRC/gash.sh; $1"; }
fake() { mkdir -p "$T/bin"; for n; do printf '#!/bin/sh\necho "%s $*" >>"%s/argv"\n' "$n" "$T" >"$T/bin/$n"; chmod +x "$T/bin/$n"; done; }
```

`g` is headless, `gm` puts the spec mocks (`docker`, `curl`) first, `fake ssh scp rsync docker` installs stubs that only append their argv to `$T/argv`. Remote items use `printf 'HOST:box=deploy@box.example\n' >$T/.gash_env && chmod 600 $T/.gash_env`. Git items use scratch repositories under `$T` only. Dossier ids N1-N32 are those of the dossier's "New findings" section (its per-function text uses some older, offset numbers).

### PHP, Node, AI (DEV, AI)

Source dossier: the S1 audit notes (dev-ai). Items are grouped by section; IDs run DEV-01.. and AI-01.. in their own
sequence. Reproductions assume a temporary HOME as in `BRIEF.md`; the mock `nvm` functions are inline.

### Shell layer, self-management, lib/core (SHELL, SELF, CORE)

Source: the S1 audit notes (shell-self) (seeds 1-7, findings N1-N20) plus a reading of `lib/core/*.sh`, `lib/modules/gash.sh`, `lib/modules/migrate.sh`, `install.sh`, `bash_completion` at go-refactor (VERSION 2.1.0). Prefixes: SHELL- (gash.sh, prompt, history, terminal, toolchains, banner, alias packs, completion), SELF- (self-management functions, install.sh, gen-docs), CORE- (lib/core).

Conventions:

- **Reproduction**: run against a private copy of the tree, never the real checkout (`git -C $GASH_SRC archive go-refactor | tar -x -C $C`), with `$E = env -i HOME=$(mktemp -d) PATH=/usr/local/bin:/usr/bin:/bin TERM=xterm-256color LANG=C.UTF-8`; interactive loads are `$E bash --noprofile --norc -i -c 'source $C/gash.sh; ...' </dev/null`. Self-management reproductions (upgrade, uninstall, install, migrate) use a scratch clone or a fake install directory inside the temporary HOME.
- **rc.A layer**: "init" means the Bash emitted by `gash init bash` (`internal/shellinit`, `go-impl/design/shell.md`); "shim" means the generated `gash.sh`, the 7 harness paths and `bash_completion` (`internal/compatgen`, `compat.md`).
- Proving tests named `testdata/script/*.txtar` run with the generated shims and a temporary HOME.

### HTE design delta (HTE)

Every intentional difference between `gash hte` (design: `go-impl/design/hte.md`) and hte-cli 1.1.1, plus the 1.0.x behaviors `gash hte migrate` must handle. Source: the S1 audit notes (hte) (sections A to I).

Conventions for this section:

- **hte-cli location**: paths in `$HTE_CLI_SRC` at `8e18711` (tag v1.1.1), or `REV:path:line` for history.
- **Reproduction**: commands of the S1 sandbox harness, which ran the real hte-cli 1.1.1 code (and historical phars) with system paths mapped into a temporary directory and `systemctl`/`a2ensite`/`a2dissite` faked. The harness is not in the repository (it carries hte-cli and its vendored dependencies); the hte-cli location of each item is the evidence, and the legacy fixtures are in `go-impl/testdata/hte/legacy/`. "by design" marks pure design choices.
- **Severity**: for a PRD 6.7 defect or a dossier finding, the severity of the hte-cli behavior the delta removes (brief scale); for a design choice, how much an hte-cli user is affected by the change.
- **Public-contract impact**: yes when users or scripts of hte-cli would notice (commands, flags, files, URLs, ports, redirects, log locations, pool settings).
- **Fix recipe (3.0)**: "none: the delta is the design" unless the operator has to choose; then the options are listed, the first one being what the design says today.
- Items flagged **Decision for the operator** in the Section field need an answer before the HTE phase starts; the design follows the recommended option until then.
- The design gaps these items found ("the design does not say ...", "design text to amend") were closed in `go-impl/design/hte.md` at S1 with the recommended option (HTE-08, 19, 27, 29, 38, 47, 55, 59, 64, 66, 67); the items stay for triage.

## Summary

| Section | critical | high | medium | low | Total | Contract impact |
|---|---|---|---|---|---|---|
| Defects | 1 | 25 | 80 | 77 | 183 | 118 |
| Forced divergences | 3 | 4 | 26 | 72 | 105 | 47 |
| Decisions for the operator | 0 | 1 | 5 | 26 | 32 | 23 |
| Doc drift | 0 | 0 | 1 | 47 | 48 | 11 |
| HTE design delta | 0 | 6 | 23 | 46 | 75 | 58 |
| **All** | 4 | 36 | 135 | 268 | **443** | 257 |

Critical and high items:

| Id | Title | Section | Severity |
|---|---|---|---|
| [LLM-01](#llm-01) | llm_grep --context is evaluated as Bash arithmetic | Forced divergences | critical |
| [DB-01](#db-01) | PostgreSQL E-string mislex lets a backslash line slip past the SQL guard | Defects | critical |
| [SHELL-01](#shell-01) | The prompt re-expands the git branch name, VIRTUAL_ENV, debian_chroot and STY as code | Forced divergences | critical |
| [FS-50](#fs-50) | tree_stats evaluates text from a scanned file name as Bash arithmetic | Forced divergences | critical |
| [LLM-02](#llm-02) | llm_env URL password masking fails on @ or / in the password, or a non-leading URL | Defects | high |
| [LLM-09](#llm-09) | llm_grep / llm_find / llm_tree walks enter protected trees | Defects | high |
| [LLM-10](#llm-10) | forbidden-path and secret-file lists have gaps | Defects | high |
| [LLM-20](#llm-20) | llm_docker_check applies no path guard and interpolates .env and caller-env secrets into output | Defects | high |
| [EXEC-01](#exec-01) | attached short-option values and clusters skip the secret-file and forbidden-path checks | Defects | high |
| [EXEC-02](#exec-02) | `git stash list -p` leaks secret files | Defects | high |
| [EXEC-07](#exec-07) | A trailing backslash makes llm_exec check one word and run another | Forced divergences | high |
| [SYS-01](#sys-01) | process_kill takes NAME as a pgrep regex: `.*` matches every process | Defects | high |
| [SYS-35](#sys-35) | gash_ssh_auto_unlock glob-expands each key/passphrase row: keys dropped, passphrase printed | Forced divergences | high |
| [DOCK-02](#dock-02) | A realpath without -m (BSD, macOS, BusyBox) makes the compose functions act on the current directory's stack | Forced divergences | high |
| [DOCK-04](#dock-04) | dcup2 ignores override files and COMPOSE_FILE, and --remove-orphans removes override-only services | Defects | high |
| [GIT-01](#git-01) | Tag push, tag deletion, git_sync and git_apply_patch act without confirmation or a headless --yes | Defects | high |
| [REM-01](#rem-01) | Unknown flags and extra positionals are silently ignored, also with --delete | Defects | high |
| [REM-02](#rem-02) | A remote target can be an ssh option (no `--` before the target) | Defects | high |
| [REM-03](#rem-03) | Path tiers are checked on strings the remote side may expand | Defects | high |
| [AI-01](#ai-01) | ai_sysinfo `?` reads and sends any named file after the first consent | Defects | high |
| [FS-02](#fs-02) | -L / --follow-symlinks walks into the forbidden trees | Defects | high |
| [FS-03](#fs-03) | --allow-root / walks /proc /sys /dev /boot /root | Defects | high |
| [FS-20](#fs-20) | A newline inside an --exclude value turns it into extra find predicates | Forced divergences | high |
| [FILE-01](#file-01) | `.Z` archives are emptied by archive_extract | Defects | high |
| [FILE-02](#file-02) | Upper- and mixed-case suffixes (`.GZ`, `.XZ`, `.Bz2`, `.ZST`) empty the archive; a file named `.gz` empties itself | Defects | high |
| [FILE-03](#file-03) | Single-file formats truncate the output before the tool runs: a corrupt archive or a missing tool destroys an existing file | Defects | high |
| [FILE-04](#file-04) | file_backup: two backups in the same second (or in a DST fall-back hour) overwrite each other | Defects | high |
| [FILE-05](#file-05) | file_backup writes through an existing symlink or hard link, or copies into a directory, at the backup name | Defects | high |
| [FILE-06](#file-06) | file_backup ignores cp's exit status | Defects | high |
| [FILE-22](#file-22) | Hostile archive members: protection depends on the installed tool version | Decisions for the operator | high |
| [SELF-02](#self-02) | gash_uninstall deletes ~/.gashrc without a backup and then deletes its own profile backups | Defects | high |
| [SELF-07](#self-07) | The installer leaves an unreleased checkout behind when GASH_REQUIRE_SIGNED refuses | Defects | high |
| [CORE-03](#core-03) | gash_db_list shows the password when the URL has an empty user name | Defects | high |
| [TEST-04](#test-04) | Specs fake `kill` with a fixed PID, a hazard once the code under test is a binary | Defects | high |
| [HTE-04](#hte-04) | `remove` and `modify` act on site #1 when `--domain` is a typo | HTE design delta | high |
| [HTE-05](#hte-05) | Site identity comes from the FPM socket name; the pool is found by substring | HTE design delta | high |
| [HTE-06](#hte-06) | Certificate generation runs `rm -rf <certdir>/<domain>.*` | HTE design delta | high |
| [HTE-08](#hte-08) | The document root is interpolated unquoted into the vhost and the pool | HTE design delta | high |
| [HTE-09](#hte-09) | Apache restart without a config test, and no rollback | HTE design delta | high |
| [HTE-18](#hte-18) | The pool's `user`/`group` follow the invoker; `modify` as root writes `user = root` | HTE design delta | high |

## Defects

| Id | Title | Area | Severity | Contract |
|---|---|---|---|---|
| [LLM-02](#llm-02) | llm_env URL password masking fails on @ or / in the password, or a non-leading URL | Security | high | yes |
| [LLM-05](#llm-05) | llm_deps leaks a raw jq error with exit 5 on an invalid manifest | Contract | medium | yes |
| [LLM-06](#llm-06) | awk JSON escapers leave C0 control characters raw (invalid JSON) | Correctness | medium | yes |
| [LLM-08](#llm-08) | llm_tree root path with quote, backslash, tab or newline gives invalid JSON and an empty listing | Correctness | medium | yes |
| [LLM-09](#llm-09) | llm_grep / llm_find / llm_tree walks enter protected trees | Security | high | yes |
| [LLM-10](#llm-10) | forbidden-path and secret-file lists have gaps | Security | high | yes |
| [LLM-11](#llm-11) | llm_ports collapses distinct sockets on the same port and protocol | Correctness | medium | yes |
| [LLM-12](#llm-12) | llm_procs --port: leading zeros produce an invalid JSON number and there is no range check | Correctness | medium | yes |
| [LLM-13](#llm-13) | llm_git_log drops commits whose subject or author contains 0x1e/0x1f | Correctness | medium | yes |
| [LLM-14](#llm-14) | llm_deps go.mod and array manifests produce wrong data silently | Correctness | medium | yes |
| [LLM-15](#llm-15) | secret exceptions are not honored by walks; the grep fallback refuses an explicit secret-exception file | Security | medium | yes |
| [LLM-19](#llm-19) | llm_tree --text and llm_find name search quirks | Correctness | low | yes |
| [LLM-20](#llm-20) | llm_docker_check applies no path guard and interpolates .env and caller-env secrets into output | Security | high | yes |
| [LLM-23](#llm-23) | non-UTF-8 bytes pass raw into JSON (two behaviors) | Correctness | medium | yes |
| [LLM-27](#llm-27) | llm_project detection quirks (poetry always, case-sensitive flask, framework grep anywhere) | Correctness | low | yes |
| [LLM-34](#llm-34) | llm_procs --name can list its own exiting helper subshell, with an empty name | Correctness | low | no |
| [DB-01](#db-01) | PostgreSQL E-string mislex lets a backslash line slip past the SQL guard | Security | critical | yes |
| [DB-06](#db-06) | the slow-query threshold includes client spawn and connect time | Correctness | medium | yes |
| [DB-14](#db-14) | llm_db_query with EXPLAIN on SQLite prints a raw jq parse error and no envelope | Contract | yes | Reproduced (`internal/db` envelope): the same message, exit 5, no stdout. |
| [EXEC-01](#exec-01) | attached short-option values and clusters skip the secret-file and forbidden-path checks | Security | high | yes |
| [EXEC-02](#exec-02) | `git stash list -p` leaks secret files | Security | high | yes |
| [EXEC-08](#exec-08) | A line continuation between two `\|` is checked as two pipes, run as `\|\|` | Correctness | low | no |
| [SYS-01](#sys-01) | process_kill takes NAME as a pgrep regex: `.*` matches every process | Security | high | yes |
| [SYS-02](#sys-02) | process_kill and port_kill report a process they may not signal as "terminated" | Correctness | medium | yes |
| [SYS-03](#sys-03) | port_kill also targets UDP clients whose remote port is PORT | Correctness | medium | yes |
| [SYS-04](#sys-04) | process_find returns 0 when nothing matches and matches the whole ps line | Contract | medium | yes |
| [SYS-05](#sys-05) | hgrep -E with an invalid regex reports success | Contract | medium | yes |
| [SYS-06](#sys-06) | hgrep --json emits invalid JSON for control characters; CRLF history entries are lost | Contract | medium | yes |
| [SYS-07](#sys-07) | hgrep cannot search a pattern that starts with `-`; `-h` is help only as the first word | Contract | low | yes |
| [SYS-08](#sys-08) | history_grep passes extra words to grep as FILE operands | Correctness | medium | yes |
| [SYS-09](#sys-09) | history_grep duplicates text in column 3; HISTTIMEFORMAT breaks columns and dedup; `hg` self-matches | Correctness | medium | yes |
| [SYS-10](#sys-10) | Extra positional arguments are silently accepted | Contract | low | yes |
| [SYS-11](#sys-11) | mkdir_cd follows CDPATH and passes option-like names to mkdir and cd | Correctness | medium | yes |
| [SYS-12](#sys-12) | sudo_last returns 0 when declined and asks even with no previous command | Contract | low | yes |
| [SYS-13](#sys-13) | ip_public prints whatever the endpoint returns, with no timeout | Correctness | medium | yes |
| [SYS-14](#sys-14) | disk_usage fails entirely when df reports one unreadable mount | Correctness | medium | yes |
| [SYS-15](#sys-15) | sysinfo asks for the sudo password for every section, even those that never use sudo | Correctness | low | no |
| [SYS-16](#sys-16) | sysinfo prints `none` twice for Virtualization on bare metal | Correctness | low | yes |
| [SYS-17](#sys-17) | sysinfo shows a stale sub-header above unrelated lines | Correctness | medium | yes |
| [SYS-18](#sys-18) | sysinfo identity misreads quoted ID and unquoted VERSION_ID in /etc/os-release | Portability | medium | yes |
| [SYS-19](#sys-19) | sysinfo expands globs for sudo commands as the unprivileged user | Correctness | low | no |
| [SYS-20](#sys-20) | sysinfo silently keeps the last of several sections; `--help` after another word is an error | Contract | low | yes |
| [SYS-21](#sys-21) | gash_ssh_auto_unlock does not recover from a stale SSH_AUTH_SOCK | Correctness | medium | no |
| [SYS-22](#sys-22) | The agent search chmods an existing directory to 700; the expect hint goes to stdout | Correctness | low | no |
| [SYS-23](#sys-23) | chrome_headless_update loses the installed version under a symlinked or relative GASH_CHROME_ROOT | Correctness | medium | yes |
| [SYS-24](#sys-24) | chrome_headless_update: version check is a substring, a pre-existing target is linked unverified | Security | medium | no |
| [SYS-25](#sys-25) | services_stop, wsl_compact and chrome_headless_update call sudo without -n in unattended runs | Contract | medium | yes |
| [SYS-26](#sys-26) | sysinfo completion does not offer `overview` | Completion | low | no |
| [SYS-27](#sys-27) | No completion for the flags of hgrep, port_kill, process_kill, services_stop, sudo_last and the wsl stop functions | Completion | low | no |
| [SYS-50](#sys-50) | deep_services WireGuard redaction misses PrivateKey without spaces and every PresharedKey | Security | medium | yes |
| [SYS-51](#sys-51) | deep_performance I/O-scheduler line reports the queue directory, not the block device | Correctness | medium | no |
| [SYS-52](#sys-52) | deep collectors glue the next section header onto a file with no final newline | Correctness | low | no |
| [DOCK-01](#dock-01) | Compose functions act on the current directory when the path argument is empty | Correctness | medium | yes |
| [DOCK-03](#dock-03) | Completion: dcup2 lacks --yes; docker_prune_all, git_repair, git_sync -m and remote_* incomplete | Completion | low | no |
| [DOCK-04](#dock-04) | dcup2 ignores override files and COMPOSE_FILE, and --remove-orphans removes override-only services | Correctness | high | yes |
| [DOCK-05](#dock-05) | dcup2 on a compose file without image services runs compose with an empty service argument | Correctness | medium | yes |
| [DOCK-06](#dock-06) | `image:tag@sha256:...` loses its digest pin | Correctness | medium | yes |
| [DOCK-07](#dock-07) | `docker.io/<name>` and `index.docker.io/...` references are queried wrongly | Correctness | medium | yes |
| [DOCK-09](#dock-09) | docker_compose_scan emits invalid or misleading JSON | Correctness | medium | yes |
| [DOCK-10](#dock-10) | docker_compose_scan ignores unknown options, also --help after a path | Contract | low | yes |
| [DOCK-13](#dock-13) | Image interpolation rules differ from Compose's (default operators, precedence, `$VAR`) | Correctness | medium | yes |
| [DOCK-14](#dock-14) | GITHUB_TOKEN is sent raw as the ghcr.io bearer and blocks anonymous access | Correctness | medium | no |
| [DOCK-15](#dock-15) | Other registries: no auth challenge, https only, no credential store | Correctness | low | no |
| [DOCK-16](#dock-16) | Local digests come from `docker inspect` without `--type image` | Correctness | low | no |
| [DOCK-18](#dock-18) | docker_compose_upgrade returns 0 when the upgrade is declined | Contract | low | yes |
| [DOCK-20](#dock-20) | docker_stop_all and docker_start_all hide daemon errors and ignore arguments; prune counts read 0 on a failing daemon | Correctness | medium | yes |
| [DOCK-21](#dock-21) | docker_prune_all refuses headless without saying that -y is needed | Contract | low | yes |
| [DOCK-28](#dock-28) | The YAML-subset reader cuts ` #...` inside quoted image values | Correctness | low | no |
| [DOCK-29](#dock-29) | Compose file precedence is the reverse of Compose's | Correctness | low | yes |
| [DOCK-30](#dock-30) | docker_compose_check table: contradictory width limits misalign rows | Correctness | low | no |
| [DOCK-31](#dock-31) | Option-like image references and service names reach the docker CLI without `--` | Security | low | no |
| [GIT-01](#git-01) | Tag push, tag deletion, git_sync and git_apply_patch act without confirmation or a headless --yes | Contract | high | yes |
| [GIT-02](#git-02) | git_list_tags shows peeled tags and returns 0 when ls-remote fails | Correctness | low | yes |
| [GIT-03](#git-03) | An unreachable origin counts as "tag absent" | Correctness | medium | yes |
| [GIT-04](#git-04) | Option-like tag names reach `git tag` | Security | low | yes |
| [GIT-05](#git-05) | git_dump_revisions: file names parsed as revisions, absolute paths give empty files | Correctness | medium | yes |
| [GIT-06](#git-06) | git_apply_patch: slash branch names break the patch file, failures leave another branch checked out | Correctness | medium | yes |
| [GIT-07](#git-07) | git_sync --dry-run promises a push that git push refuses | Correctness | medium | yes |
| [GIT-08](#git-08) | git_repair: origin URL parsed as a clone option; empty-file deletion too broad and before the clone | Security | medium | no |
| [REM-01](#rem-01) | Unknown flags and extra positionals are silently ignored, also with --delete | Contract | high | yes |
| [REM-02](#rem-02) | A remote target can be an ssh option (no `--` before the target) | Security | high | yes |
| [REM-03](#rem-03) | Path tiers are checked on strings the remote side may expand | Security | high | yes |
| [REM-05](#rem-05) | The rsync -e string breaks when $HOME contains a space | Portability | low | no |
| [REM-06](#rem-06) | remote_sync_pull --delete checks the local destination with remote wording | Contract | low | yes |
| [REM-07](#rem-07) | remote_exec creates ~/.ssh with the default umask | Security | low | no |
| [REM-08](#rem-08) | A HOST target `u@h:22` is accepted; ssh, scp and the tier check read it differently | Correctness | low | yes |
| [REM-10](#rem-10) | A `/.` (or `//.`) root target normalizes to `.` and classifies as allow, not deny:/ | Security | medium | yes |
| [DEV-01](#dev-01) | php_detect_version reports a PHP version that is not installed | Correctness | medium | yes |
| [DEV-02](#dev-02) | OR constraints resolve to their first X.Y (`^7.4\\|^8.0` gives 7.4) | Correctness | medium | yes |
| [DEV-03](#dev-03) | Major-only constraints are ignored and give the 8.4 default | Correctness | medium | yes |
| [DEV-04](#dev-04) | Two X.Y values on line 1 of .target-php-version give a multi-line version | Contract | medium | yes |
| [DEV-05](#dev-05) | node_detect_version counts every entry of versions/node as installed | Correctness | medium | yes |
| [DEV-06](#dev-06) | node_upgrade exits 0 when installing a new patch failed | Contract | medium | yes |
| [DEV-07](#dev-07) | nvm_prune_versions --force reports success when an uninstall fails | Contract | medium | yes |
| [DEV-08](#dev-08) | nvm_prune_versions reads io.js entries as Node versions | Correctness | low | no |
| [AI-01](#ai-01) | ai_sysinfo `?` reads and sends any named file after the first consent | Security | high | yes |
| [AI-02](#ai-02) | `ai_query claude` sends the word "claude" as the question | Correctness | medium | yes |
| [AI-03](#ai-03) | ai_ask asks one question per call; a typed `exit` is sent as a question | Correctness | low | yes |
| [AI-04](#ai-04) | ai_ask without a terminal sends stdin for auto-analysis without asking | Security | medium | yes |
| [AI-05](#ai-05) | ai_query and ai_ask block on any open non-terminal stdin | Correctness | medium | yes |
| [AI-06](#ai-06) | An unclosed PRIVATE KEY block swallows the rest of the text, the question included | Correctness | medium | no |
| [AI-07](#ai-07) | Redaction misses bearer headers, Gash's own SSH: entries and password hashes | Security | medium | no |
| [AI-08](#ai-08) | The port pattern of ai_sysinfo questions takes clock times for ports | Correctness | low | no |
| [AI-09](#ai-09) | Saving the ai_sysinfo report: a failed write is reported as saved | Correctness | medium | yes |
| [AI-10](#ai-10) | An analysis reply without `.sections` prints four jq errors and empty counts | Correctness | low | yes |
| [AI-11](#ai-11) | Context distro detection misses quoted IDs and Linux Mint | Correctness | low | no |
| [AI-12](#ai-12) | GASH_AI_MODEL_GEMINI goes into the request URL unvalidated | Security | low | yes |
| [FS-02](#fs-02) | -L / --follow-symlinks walks into the forbidden trees | Security | high | yes |
| [FS-03](#fs-03) | --allow-root / walks /proc /sys /dev /boot /root | Security | high | yes |
| [FS-04](#fs-04) | A second PATH silently wins; words after `-- PATH` are ignored | Contract | medium | yes |
| [FS-05](#fs-05) | Integer flags accept digit strings of any length | Contract | medium | yes |
| [FS-06](#fs-06) | dirs_largest keeps paths containing a tab | Contract | medium | yes |
| [FS-07](#fs-07) | dirs_list_empty keeps paths containing a tab | Contract | low | yes |
| [FS-08](#fs-08) | tree_stats counts tab paths, and its empty-dir count counts newline directories | Correctness | medium | yes |
| [FS-09](#fs-09) | dirs_find_large cuts a path at its first tab and can name another directory | Correctness | medium | yes |
| [FS-10](#fs-10) | JSON carries raw C0 control characters (invalid JSON) | Contract | medium | yes |
| [FS-11](#fs-11) | The size parser's fallback joins all digits and accepts stray units | Correctness | low | yes |
| [FS-12](#fs-12) | --exclude means three different things across the scanners | Correctness | medium | yes |
| [FS-13](#fs-13) | Default prunes apply to the scan root itself; --min-depth disables them | Correctness | medium | yes |
| [FS-14](#fs-14) | Walker failures on the GNU path are reported as an empty success | Contract | medium | yes |
| [FS-15](#fs-15) | errors counts walker stderr lines, not unreadable entries | Contract | low | yes |
| [FS-16](#fs-16) | tree_stats empty_dirs counts pruned and excluded entries | Correctness | medium | yes |
| [FS-17](#fs-17) | tree_stats extension containing a tab breaks the JSON and the text | Contract | medium | yes |
| [FS-18](#fs-18) | dirs_list_empty prints one blank line when nothing is found | Contract | low | yes |
| [FS-19](#fs-19) | The --json pre-scan also matches a flag value | Contract | low | yes |
| [FILE-01](#file-01) | `.Z` archives are emptied by archive_extract | Correctness | high | yes |
| [FILE-02](#file-02) | Upper- and mixed-case suffixes (`.GZ`, `.XZ`, `.Bz2`, `.ZST`) empty the archive; a file named `.gz` empties itself | Correctness | high | yes |
| [FILE-03](#file-03) | Single-file formats truncate the output before the tool runs: a corrupt archive or a missing tool destroys an existing file | Correctness | high | yes |
| [FILE-04](#file-04) | file_backup: two backups in the same second (or in a DST fall-back hour) overwrite each other | Correctness | high | yes |
| [FILE-05](#file-05) | file_backup writes through an existing symlink or hard link, or copies into a directory, at the backup name | Correctness | high | yes |
| [FILE-06](#file-06) | file_backup ignores cp's exit status | Correctness | high | yes |
| [FILE-07](#file-07) | OUTPUT_DIR is created before the format and its tool are checked | Correctness | low | no |
| [FILE-08](#file-08) | An option-like OUTPUT_DIR reaches `mkdir` without `--` | Correctness | low | no |
| [FILE-09](#file-09) | unrar reads a dash-named OUTPUT_DIR as a switch | Correctness | medium | no |
| [FILE-10](#file-10) | unzip, 7z and unrar expand wildcards in the archive name | Correctness | medium | yes |
| [FILE-11](#file-11) | unzip, unrar and 7z prompt and read stdin in headless mode | Contract | medium | yes |
| [FILE-12](#file-12) | "File 'X' does not exist." for existing non-regular paths (both functions) | Correctness | low | yes |
| [FILE-13](#file-13) | `.tar.Z` is decompressed, not unpacked; `.taz`, `.tar.lz`, `.tlz`, `.tar.lzma`, `.lz4`, `.tbz` are unsupported | Correctness | low | yes |
| [FILE-14](#file-14) | `uncompress F -c` puts `-c` after the operand | Portability | low | no |
| [FILE-15](#file-15) | A trailing newline in the archive name is dropped from the output name | Correctness | low | no |
| [SHELL-02](#shell-02) | The terminal title writes the raw working directory, USER and HOSTNAME into an OSC sequence | Security | medium | no |
| [SHELL-03](#shell-03) | Completion offers `gash help`, which fails | Completion | low | no |
| [SHELL-04](#shell-04) | GASH_HISTORY overwrites the user's HISTCONTROL, while the migration says it is respected | Correctness | medium | yes |
| [SHELL-06](#shell-06) | The `grep` color alias is never defined | Correctness | low | yes |
| [SHELL-08](#shell-08) | The first word of GASH_PATH_ADD is taken as a path_add option | Correctness | low | no |
| [SHELL-09](#shell-09) | Gash overwrites aliases defined before it, and unload does not restore them | Correctness | medium | yes |
| [SHELL-11](#shell-11) | Sourcing gash.sh through a symlinked file breaks GASH_DIR | Correctness | low | no |
| [SHELL-12](#shell-12) | Module aliases are outside the shadow rule | Correctness | low | no |
| [SELF-01](#self-01) | gash_upgrade compares for equality only and can downgrade | Self-management | medium | yes |
| [SELF-02](#self-02) | gash_uninstall deletes ~/.gashrc without a backup and then deletes its own profile backups | Self-management | high | yes |
| [SELF-03](#self-03) | gash_unload leaves completion specs, exports and an empty PROMPT_COMMAND | Correctness | medium | no |
| [SELF-04](#self-04) | gash_uninstall replaces a symlinked profile with a regular file | Self-management | medium | no |
| [SELF-05](#self-05) | gash_unload removes the user's own GASH_* settings | Correctness | low | no |
| [SELF-06](#self-06) | gash_doctor always returns 0, ignores unknown options and checks incomplete lists | Self-management | medium | yes |
| [SELF-07](#self-07) | The installer leaves an unreleased checkout behind when GASH_REQUIRE_SIGNED refuses | Security | high | no |
| [SELF-08](#self-08) | Migration backups are world-readable, keep the old mode of ~/.gash_env and are never pruned | Security | medium | no |
| [SELF-09](#self-09) | The migration does not mention the 1.x history behavior it drops | Self-management | low | no |
| [SELF-10](#self-10) | Tag signatures are detected by a text match | Self-management | low | no |
| [SELF-12](#self-12) | The `gash` card counts and lists aliases that are not defined | Correctness | low | no |
| [SELF-13](#self-13) | Small message defects: positional reported as an option, wrong line count in a comment | Correctness | low | no |
| [SELF-14](#self-14) | A 1.x shell reading the 2.x/3.x quote file can print a comment line | Self-management | low | no |
| [SELF-15](#self-15) | Arguments are ignored, and gash_doctor --timing runs the user's ~/.bashrc three times | Self-management | low | yes |
| [CORE-03](#core-03) | gash_db_list shows the password when the URL has an empty user name | Security | high | no |
| [CORE-04](#core-04) | gash_db_list lists every duplicate name, while only the first is used | Correctness | low | no |
| [CORE-06](#core-06) | Messages name ~/.gash_env even when GASH_ENV_FILE points elsewhere | Correctness | low | yes |
| [CORE-11](#core-11) | A bracketed IPv6 host in a DB URL is split at its first colon | Correctness | low | no |
| [CORE-13](#core-13) | DB passwords lose trailing newlines and `%00` when decoded | Correctness | low | no |
| [CORE-14](#core-14) | HOME is matched as a glob when a path is shown with `~` | Correctness | low | yes |
| [CORE-15](#core-15) | SSH key paths: `~user/` is not a home directory, and `&` in HOME is rewritten on Bash 5.2 | Correctness | low | no |
| [CORE-19](#core-19) | A located download URL that 404s with a Content-Length is treated as present | Correctness | medium | no |
| [TEST-01](#test-01) | The test runner leaks the caller's NO_COLOR into the hermetic environment | Test harness | low | no |
| [TEST-02](#test-02) | `tests/run.sh -j1` is refused | Test harness | low | no |
| [TEST-03](#test-03) | 88 conditional skips print no reason | Test harness | low | no |
| [TEST-04](#test-04) | Specs fake `kill` with a fixed PID, a hazard once the code under test is a binary | Test harness | high | no |
| [TEST-05](#test-05) | 52 render fixtures fake external commands as Bash functions | Test harness | medium | no |
| [TEST-06](#test-06) | `llm_spec.sh` uses the repository's own `lib/` as a fixture tree | Test harness | low | no |
| [TEST-07](#test-07) | 18 specs source Gash at top level in the runner process | Test harness | low | no |
| [TEST-09](#test-09) | `tests/run.sh` can end in FAIL with 0 failed: a reaped job is waited for again | Test harness | low | no |
| [TEST-10](#test-10) | Mock-client DB specs depend on the runner's speed: the slow-query rule runs the mock twice | Test harness | low | no |
| [TEST-11](#test-11) | Specs fake curl per test, but the binary speaks HTTP itself and never runs those fakes | Test harness | low | no |
| [TEST-12](#test-12) | The settings leak check also scans the random temp path, which can hold a checked word | Test harness | low | no |
| [TEST-13](#test-13) | Specs call 2.x private helpers that rc.A does not have | Test harness | no | The tests stay as they are and run in Bash mode; Go mode reports them under TEST-13. Their rules are pinned by the Go unit tests of `internal/llmexec`, `internal/pathguard`, `internal/sqlguard` and `internal/llm`, which reuse their vectors. |
| [TEST-14](#test-14) | Mock-client DB specs: the native drivers never run a mock mysql or psql | Test harness | no | The tests run in Bash mode; Go mode reports them under TEST-14. What they check is pinned in `internal/db`: `TestMySQLMatches2x` (read-only session, row cap, the database from `-d` and `.target-database`, the password never in argv or environment) and `TestPgsqlMatches2x` (read-only, standard strings and timeout settings, `current_database()`), plus the byte-for-byte comparison against real psql on a private PostgreSQL 16. |

### llm_* agent functions

<a id="llm-02"></a>
#### LLM-02 · llm_env URL password masking fails on @ or / in the password, or a non-leading URL

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "llm_env URL password masking fails with @ or / in the password" (CONFIRMED) |
| Area | Security |
| Bash location | `lib/modules/llm/system.sh:250` |
| Observed behavior | The redaction regex `^([A-Za-z][A-Za-z0-9+.-]*://[^/:@]*):[^/@]*@(.*)$` is anchored to the start and the password class excludes `/` and `@`, so a password containing `@` leaks its suffix, a password with `/` is not masked at all, and a URL not at the start of the value is never masked. |
| Reproduction | `MYURL_A='mysql://user:p@ss@host/db' llm_env --filter MYURL` -> `mysql://user:***@ss@host/db` (suffix leaks) |
| Severity | high |
| Public-contract impact | yes (documented redaction output changes) |
| rc.A behavior | Reproduced: rc.A masks exactly as the 2.x anchored regex does (partial leaks preserved) so the redaction output matches byte for byte. |
| Fix recipe (3.0) | Split each whitespace token and apply `^(.*[^A-Za-z0-9+.-] | )([A-Za-z][A-Za-z0-9+.-]*://[^/:@]*):.*@([^@]*)$` -> `\1\2:***@\3` (password runs greedily to the last `@`), in `internal/ai`/`internal/ui` redaction; rejoin on the original separators. |
| Proving test | `TestLedger_LLM02` asserts `mysql://u:p@ss@h/d` -> `mysql://u:***@h/d`, `mysql://u:pa/ss@h/d` -> `mysql://u:***@h/d`, `x https://u:pw@h/` -> `x https://u:***@h/`. |
| Status | accept: done 4b5e9b1 |

<a id="llm-05"></a>
#### LLM-05 · llm_deps leaks a raw jq error with exit 5 on an invalid manifest

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "invalid manifest leaks a raw jq error with exit 5" (PARTIAL: llm_deps yes, llm_project no) |
| Area | Contract |
| Bash location | `lib/modules/llm/project.sh:151-154` |
| Observed behavior | `jq -c … FILE` runs with no error handling: a malformed or non-object composer.json/package.json prints jq's raw parse error on stderr and returns exit 5; an empty manifest prints nothing and returns 0. `llm_project` only greps and is not affected. |
| Reproduction | broken `package.json` then `llm_deps DIR; echo rc=$?` -> jq parse error, `rc=5` (temp dir) |
| Severity | medium |
| Public-contract impact | yes (uncontracted exit 5 and non-envelope stderr) |
| rc.A behavior | Reproduced: rc.A returns exit 5 and a jq-equivalent raw error line on an invalid manifest (byte parity on the exit code; message text matches jq's). |
| Fix recipe (3.0) | Native manifest parse in the `llm_deps` handler; on parse failure emit a new `invalid_manifest`/RETRY, details the path, hint `The manifest is not a JSON object; fix it or use llm_config to read it`; empty manifest -> same code; add `invalid_manifest` to AGENTS.md section 5. |
| Proving test | `TestLedger_LLM05` asserts broken/array/empty `package.json` -> exit 1 with `.error=="invalid_manifest"` and one JSON line on stderr. |
| Status | accept: done e864421 |

<a id="llm-06"></a>
#### LLM-06 · awk JSON escapers leave C0 control characters raw (invalid JSON)

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "awk JSON escapers miss U+0001-U+001F besides tab, newline, CR" (CONFIRMED) |
| Area | Correctness |
| Bash location | `lib/modules/llm/fs.sh:148-156` (llm_tree), `system.sh:62` (llm_ports), `system.sh:158-163` (llm_procs), `core.sh:627-663` (`__llm_tsv_to_json`) |
| Observed behavior | The four awk/inline escapers handle only `\ " \n \r \t` (some only `\ "`), so a name or value containing U+0001..U+001F (e.g. `\x01`, `\x1b`) is emitted raw, producing JSON that jq and strict parsers reject. |
| Reproduction | `touch "$(printf 'a\x01b')"; llm_tree . | jq .` -> jq: control characters must be escaped (temp dir) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced at the byte level: on the paths that used the awk or inline escapers (llm_tree names, llm_ports, llm_procs), the contract encoder writes C0 controls other than tab, newline and CR raw, exactly as 2.x; the jq paths keep jq's `\u00xx` escapes. |
| Fix recipe (3.0) | Drop the raw mode of `internal/contract` for these paths: escape `\b \f \n \r \t` and `\u00xx` (lowercase hex, as jq) for every other C0 control on every path. |
| Proving test | `TestLedger_LLM06` feeds names/values with `\x01`, `\x1b`, `\x7f` through llm_tree, llm_ports, llm_procs and `__llm_tsv_to_json` equivalents and asserts `jq -e .` succeeds and values round-trip. |
| Status | accept: done 80f82cb |

<a id="llm-08"></a>
#### LLM-08 · llm_tree root path with quote, backslash, tab or newline gives invalid JSON and an empty listing

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N1) |
| Area | Correctness |
| Bash location | `lib/modules/llm/fs.sh:130,140,143` |
| Observed behavior | The root `path` is escaped once and then re-interpreted by `awk -v root=`, so `"`/`\`/control bytes land raw in `path`; and the `! -path '*\t*'`/`'*\n*'` filters match the root prefix, so when the root name contains a tab or newline every child is excluded and `children` is `[]` although files exist. |
| Reproduction | `mkdir 'q"uote'; touch 'q"uote/f1'; llm_tree 'q"uote' | jq .` -> invalid JSON (temp dir) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Partly reproduced: the native encoder escapes `"`/`\`/controls correctly so the path is valid JSON (cannot emit the raw bytes, see LLM-06), but rc.A reproduces the empty-children behavior when the root name itself contains a tab or newline (the tab/newline exclusion applied to the root, tied to LLM-29). |
| Fix recipe (3.0) | In `internal/fsscan` carry the root as bytes through the encoder (no re-interpretation) and apply the tab/newline exclusion by entry name, not by a path-prefix match that catches the root. |
| Proving test | `TestLedger_LLM08` asserts a root named `q"uote` with one child emits valid JSON with `children` length 1. |
| Status | accept: done 80338c9 |

<a id="llm-09"></a>
#### LLM-09 · llm_grep / llm_find / llm_tree walks enter protected trees

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N2) |
| Area | Security |
| Bash location | `lib/modules/llm/fs.sh:76,306,419`; `core.sh:179-191` (`__llm_path_above_forbidden`, used only by llm_exec) |
| Observed behavior | Only the root PATH is validated; the walk itself is never checked, so `llm_grep '^root:' /etc` reads `/etc/shadow`/`/etc/gshadow`, and `llm_tree --depth 2 /` and `llm_find '*' /` list `/proc`, `/root`, `/sys`. llm_exec refuses the same walks and AGENTS.md section 6.1 says the trees are protected for every llm_* path argument. |
| Reproduction | `llm_find '*' / -n 100000 | grep -c '^/proc/'` returns a large count (read-only, temp HOME) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the native scanner walks into the protected trees exactly as 2.x does (only the root is guarded). |
| Fix recipe (3.0) | In `internal/fsscan`, skip any path at or below a `__LLM_FORBIDDEN_PATHS` entry during the walk (reuse `pathguard.AboveForbidden`); refuse a root that is a forbidden tree or its ancestor with `forbidden_path` FATAL, hint `Search a subdirectory`. |
| Proving test | `TestLedger_LLM09` asserts `llm_grep root /etc` output has no `/etc/shadow` line and `llm_tree --depth 2 /` has no children under `root`/`proc`/`sys`. |
| Status | accept: done 7b87da7 |

<a id="llm-10"></a>
#### LLM-10 · forbidden-path and secret-file lists have gaps

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N3) |
| Area | Security |
| Bash location | `lib/modules/llm/core.sh:71-82,92-100` |
| Observed behavior | `/etc/shadow-`, `/etc/gshadow-` and `/etc/security/opasswd` are not in the forbidden list, so `llm_config /etc/shadow-` prints the shadow backup; `ssh_host_*_key`, `.envrc`, `*.tfvars.json`, `auth.json` and `credentials_old` are not matched as secrets. |
| Reproduction | `llm_config /etc/shadow- | head -1` as root prints a shadow line (read-only) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A uses the identical path/secret tables, so the same gaps exist. |
| Fix recipe (3.0) | In `internal/pathguard`, add `/etc/shadow-`, `/etc/gshadow-`, `/etc/security/opasswd` to the forbidden paths and `ssh_host_*_key`, `.envrc`, `*.tfvars.json`, `credentials*` to the secret patterns (the tables are shared with docs generation). |
| Proving test | `TestLedger_LLM10` extends the secret/forbidden matrix with these names and asserts refusal. |
| Status | accept: done 7b87da7 (credentials_* and credentials-* rather than credentials*) |

<a id="llm-11"></a>
#### LLM-11 · llm_ports collapses distinct sockets on the same port and protocol

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N4) |
| Area | Correctness |
| Bash location | `lib/modules/llm/system.sh:61` |
| Observed behavior | `sort -t $'\t' -k1,1n -k2,2 -u` dedups on (port, proto) only, so two listeners on the same port (e.g. `0.0.0.0:22` and `[::]:22`, or a LISTEN and an ESTAB on one port) collapse to a single record; the comment at `system.sh:34` claims one record per proto/address/port/state. |
| Reproduction | a fake `ss` emitting an IPv4 and an IPv6 listener on 22 yields one port-22 record (temp dir) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A dedups on (port, proto) and loses the other rows. |
| Fix recipe (3.0) | In the `llm_ports` handler dedupe whole records (port, proto, state, address) and sort by port then proto; keep `UNCONN`->`LISTEN` and the address-without-port rule. |
| Proving test | `TestLedger_LLM11` feeds IPv4+IPv6 listeners on 22 and asserts two records. |
| Status | accept: done b1f9d1c |

<a id="llm-12"></a>
#### LLM-12 · llm_procs --port: leading zeros produce an invalid JSON number and there is no range check

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N5) |
| Area | Correctness |
| Bash location | `lib/modules/llm/system.sh:109,121` |
| Observed behavior | `--port` is validated only as a uint and emitted verbatim as `"port":$port`, so `--port 018080` yields `"port":018080` (an invalid JSON number that strict parsers reject); values above 65535 (e.g. 99999999) are accepted and simply match nothing. |
| Reproduction | `llm_procs --port 018080` with a listener on 18080 -> `"port":018080` (temp HOME) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A emits the port as the caller typed it (source-text number token, matching jq 1.7's lenient acceptance) and applies no range check. |
| Fix recipe (3.0) | In the `llm_procs` handler normalize with base-10 parse before emitting and reject `port>65535` with `invalid_port`/RETRY; document that `--name` is ignored when `--port` is given. |
| Proving test | `TestLedger_LLM12` asserts `llm_procs --port 0080` output parses as strict JSON and `--port 99999` -> `invalid_port`. |
| Status | accept: done 77625d1 |

<a id="llm-13"></a>
#### LLM-13 · llm_git_log drops commits whose subject or author contains 0x1e/0x1f

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N6) |
| Area | Correctness |
| Bash location | `lib/modules/llm/git.sh:187-188` |
| Observed behavior | The log is split on US (0x1f) and RS (0x1e) with `jq -cRs … select(length==4)`, so any commit whose subject or author contains those bytes parses to the wrong field count and is silently dropped from the array. |
| Reproduction | three commits, one subject containing 0x1e: `llm_git_log | jq length` -> 1 (temp repo) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A splits on 0x1f/0x1e and drops the same records so the array matches 2.x. |
| Fix recipe (3.0) | In the `llm_git_log` handler use NUL-delimited `git log -z --format='%h%x00%s%x00%an%x00%ci'` (NUL cannot occur in a header), read 4 fields per record and render with the native encoder. |
| Proving test | `TestLedger_LLM13` asserts a commit whose subject has a control char still appears in the array. |
| Status | accept: done 253f6f7 |

<a id="llm-14"></a>
#### LLM-14 · llm_deps go.mod and array manifests produce wrong data silently

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N7) |
| Area | Correctness |
| Bash location | `lib/modules/llm/project.sh:152-154,159-163` |
| Observed behavior | A go.mod require block with a `// comment` line yields a `"//"` dependency; a JSON-array `dependencies` value yields numeric indices (`[0,1]`) via jq `keys`; an empty package.json yields no output and exit 0. |
| Reproduction | go.mod require block with a `// pinned` line -> `"dependencies":["//",…]` (temp dir) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A matches the awk/jq output including the `"//"` entry and index lists. |
| Fix recipe (3.0) | In the `llm_deps` handler skip go.mod lines matching `^//`; treat a non-object `dependencies` as empty; empty manifest -> `invalid_manifest` (LLM-05). |
| Proving test | `TestLedger_LLM14` asserts a commented go.mod and an array-valued package.json produce clean dependency lists. |
| Status | accept: done e864421 |

<a id="llm-15"></a>
#### LLM-15 · secret exceptions are not honored by walks; the grep fallback refuses an explicit secret-exception file

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N8) |
| Area | Security |
| Bash location | `lib/modules/llm/core.sh:380-398`; `fs.sh:450,466` |
| Observed behavior | `__llm_secret_excludes` applies the secret patterns but not the exceptions, so `.env.example`, `*.pub` and `*.env.example` are excluded from llm_grep / llm_find --contains walks even though they are documented as allowed; and GNU `--exclude` makes the grep fallback refuse such a file even when it is named explicitly (prints nothing, exit 0). |
| Reproduction | dir with `.env.example`, `x.env.example`: `llm_grep hit .` omits both; `llm_grep hit .env.example` finds nothing without rg (temp dir) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the native engine applies patterns without exceptions in walks, matching 2.x. |
| Fix recipe (3.0) | In `internal/fsscan`, apply the secret exceptions after the patterns (later-wins) so exception files are searched, and never exclude a file named explicitly as the single PATH. |
| Proving test | `TestLedger_LLM15` asserts `.env.example` is searched in a tree walk and as an explicit file. |
| Status | accept: done b1779e2 |

<a id="llm-19"></a>
#### LLM-19 · llm_tree --text and llm_find name search quirks

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N12) |
| Area | Correctness |
| Bash location | `lib/modules/llm/fs.sh:85-91,95,341-346` |
| Observed behavior | A root directory literally named `vendor`/`node_modules`/`.git`/`__pycache__` (text mode also `.cache`) prints nothing because the prunes run with no `-mindepth 1`; the unquoted `${item#$safe_path}` glob mis-indents a root like `a[1]`; text mode omits symlinks (JSON lists them), splits newline names and prints `/` as `//`; ordering is locale `sort`. |
| Reproduction | `mkdir vendor; llm_tree --text vendor` prints nothing (temp dir) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A matches the text-mode layout, root-prune blank output and symlink omission. |
| Fix recipe (3.0) | In the `llm_tree` text renderer prune only below the root, quote the prefix strip, include symlinks, exclude newline names as JSON mode does, and sort with C collation. |
| Proving test | `TestLedger_LLM19` asserts a directory named `vendor` lists its contents in text mode and a symlink appears. |
| Status | accept: done 7a6bb46 |

<a id="llm-20"></a>
#### LLM-20 · llm_docker_check applies no path guard and interpolates .env and caller-env secrets into output

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N13) |
| Area | Security |
| Bash location | `lib/modules/docker-compose.sh:1137-1138,511-525`; `:591,111-156` |
| Observed behavior | `llm_docker_check` calls `docker_compose_check "${1:-.}" --json` with no `__llm_validate_path`, so `..` and `/root` are accepted and extra args are ignored; image refs are interpolated from `PATH/.env` (a secret file) and from arbitrary caller environment variables via `${!var}`, so a secret value appears verbatim in the `image` field of the JSON. |
| Reproduction | a compose `image: "${ZSECRET}/x@sha256:…"` with `ZSECRET` exported -> the value appears in `.services[].image` (temp dir) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A skips the path guard and performs the same `.env`/env interpolation, matching the 2.x JSON (caller-variable clobber is handled separately as a forced divergence, LLM-21). |
| Fix recipe (3.0) | Give `llm_docker_check` the llm_* grammar (`__llm_needs_help`, path-arg, `pathguard` validation); interpolate only from the compose `.env` and compose-defined variables, never arbitrary caller env; the `.env` read is itself a secret-file access to gate. |
| Proving test | `TestLedger_LLM20` asserts `llm_docker_check ../x` is refused and an exported secret never reaches the `image` field. |
| Status | accept: done 4f80853 |

<a id="llm-23"></a>
#### LLM-23 · non-UTF-8 bytes pass raw into JSON (two behaviors)

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N15) |
| Area | Correctness |
| Bash location | `lib/core/utils.sh:173-195`; `lib/modules/llm/fs.sh:148-156` |
| Observed behavior | The bash and awk escapers pass invalid UTF-8 bytes through raw (llm_tree names, llm_env values), producing JSON that strict parsers reject, while the jq-based outputs (llm_git_status, llm_git_log, llm_deps) replace them with U+FFFD: two different behaviors for the same input class. |
| Reproduction | `touch "$(printf 'bad\xff\xfe')"; llm_tree .` emits raw bytes (temp dir) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced at the byte level: the contract encoder writes raw bytes where 2.x let them through and U+FFFD where jq replaced them (`jsontext.AllowInvalidUTF8`), preserving both paths. |
| Fix recipe (3.0) | Decide one rule (recommended U+FFFD everywhere, as jq does) in `internal/contract`; `strings.ToValidUTF8(s,"�")` before encoding on the former raw paths. |
| Proving test | `TestLedger_LLM23` asserts the chosen rule holds for llm_tree, llm_env and the jq-path functions alike. |
| Status | accept: done 80f82cb |

<a id="llm-27"></a>
#### LLM-27 · llm_project detection quirks (poetry always, case-sensitive flask, framework grep anywhere)

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N19) |
| Area | Correctness |
| Bash location | `lib/modules/llm/project.sh:63-82` and the python branch |
| Observed behavior | `package_manager` is `poetry` whenever `pyproject.toml` exists (even for a setuptools/pip project); flask is detected by a case-sensitive lowercase substring; the JS framework is chosen by `grep -q` anywhere in package.json (so `"next"` in a description wins). |
| Reproduction | a pyproject.toml with no poetry section -> `"package_manager":"poetry"` (temp dir) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A matches the first-match and grep-anywhere detection exactly. |
| Fix recipe (3.0) | In the `llm_project` handler parse the manifest (poetry only when `[tool.poetry]` present; framework keys only under `dependencies`/`devDependencies`); keep the documented precedence order. |
| Proving test | `TestLedger_LLM27` asserts a non-poetry pyproject reports `pip` and a `"next"` mention in a description does not set the framework. |
| Status | accept: done 7cd22ff |

### databases

<a id="llm-34"></a>
#### LLM-34 · llm_procs --name can list its own exiting helper subshell, with an empty name

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (rc.A gate run, intermittent) |
| Area | Correctness |
| Bash location | `lib/modules/llm/system.sh:125-149` |
| Observed behavior | The self-exclusion skips the calling shell chain and the subshells whose command line equals their parent's. A helper subshell of llm_procs itself (for example the one that ran `pgrep`) can be caught while it exits: its command line no longer matches its parent's, so it is not skipped, and it is gone before `ps -o comm=` reads its name. The result then holds an entry with an empty name, e.g. for a NAME nothing else matches. Seen once under load (`llm_flags_spec.sh` "leaves out the calling shell"). |
| Reproduction | `llm_procs --name` with a NAME no process has, repeated on a loaded machine; rarely prints one `{"pid":N,"name":""}` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Does not occur: the binary forks no helper shells, and it leaves out its own process and the calling shell chain as 2.x meant to. The Bash reference spec can still fail intermittently on this until cutover. |
| Fix recipe (3.0) | Same as rc.A. |
| Proving test | `TestLedger_LLM34` in `internal/llm`: a NAME that only the caller's chain matches gives `[]` across repeated runs. |
| Status | accept: done (rc.A); TestLedger_LLM34 |

<a id="db-01"></a>
#### DB-01 · PostgreSQL E-string mislex lets a backslash line slip past the SQL guard

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "PostgreSQL E-string mislex -> guard bypass; SET not forbidden; psql meta-commands reachable" (CONFIRMED, severe) |
| Area | Security |
| Bash location | `lib/modules/llm/db.sh:56-58` (the `[Ee]` test), amplified by `:135-136`; `:25` (`SET` absent from the forbidden words); transport `:418-420` |
| Observed behavior | The pgsql lexer treats a `'` as an E-string opener whenever the preceding character is `e` or `E`, even when that `e`/`E` is part of an identifier. Inside a (mislexed) E-string a backslash escapes the next character, so a backslash placed after such an identifier keeps the guard's string state open past a `'` that PostgreSQL (with the forced `standard_conforming_strings=on`) treats as a normal closing quote. Text that the guard then blanks out of its `code` buffer is never keyword-checked, so forbidden content, and a client backslash line, can pass the guard. Because the psql client processes a line beginning with a backslash as a client meta-command before any SQL error, this reaches shell execution on the client host. `SET` is additionally absent from the forbidden-words list, so a mislexed `SET` is not flagged (it is only blocked as a leading keyword). The working payload is kept outside the repository. |
| Reproduction | The guard-bypass and end-to-end mechanism are described above; a harmless proving vector (no meta-command) is `__llm_sql_guard pgsql` on an identifier-then-quote-then-backslash-then-backslash-line shape, which should return `client_command_blocked`. The exploit string and its live-cluster reproduction are kept outside the repository. |
| Severity | critical |
| Public-contract impact | yes |
| rc.A behavior | The lexer quirk is reproduced in the guard verdicts for parity (rc.A); but with the native pgx driver there is no psql client, so no client meta-command can run — the shell-execution impact disappears even while a guard verdict can still mis-pass. (The `\!`/meta-command path is unreachable; see DB-09.) |
| Fix recipe (3.0) | In `internal/sqlguard`, treat `'` as an E-string opener only when the `e`/`E` is a standalone token, i.e. the character before it is not an identifier character `[A-Za-z0-9_$]` (mirror the dollar-quote guard at db.sh:74); additionally reject a backslash at the start of any physical line of the query before lexing (defense in depth); add `SET` to the forbidden words. The native pgx driver removes meta-commands entirely. |
| Proving test | `TestLedger_DB01_2x` asserts the lexer vectors reproduce the 2.x verdicts; `TestLedger_DB01_fix` asserts the identifier-boundary rule makes the identifier-then-quote-then-backslash-line shape return `client_command_blocked`, that `SET …` is `write_operation_blocked`, and that a pgx connection never runs a client meta-command. |
| Status | accept: done 8ec419c |

<a id="db-06"></a>
#### DB-06 · the slow-query threshold includes client spawn and connect time

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "slow-query threshold includes client spawn and connect time" (CONFIRMED) |
| Area | Correctness |
| Bash location | `lib/modules/llm/core.sh:557-604` (sample at :560, test at :577) |
| Observed behavior | `__llm_db_envelope` samples the clock before calling the impl, which spawns the client and connects, so the >=100000 us test that triggers `slow_query_explain` (and `query_time`) includes spawn + TCP connect + auth, not just the query. A slow connection with a trivial query triggers a second client invocation (the auto-EXPLAIN) and a misleading field. |
| Reproduction | a mock client that sleeps during connect then returns fast yields `query_time` >100ms and a `slow_query_explain` (temp HOME) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A keeps a measurement window equivalent to 2.x (connection time included), so `query_time` and the slow-query trigger match. |
| Fix recipe (3.0) | Measure only server-side execution (driver query duration after the connection is established, or PostgreSQL `EXPLAIN (ANALYZE)` actual time) in `internal/db`; the >=100ms trigger then reflects the query alone. |
| Proving test | `TestLedger_DB06_2x` asserts a connect-slow/query-fast mock still trips the threshold (rc.A); `TestLedger_DB06_fix` asserts it does not once timing is query-only. |
| Status | accept: done 103ab0d |

<a id="db-14"></a>
#### DB-14 · llm_db_query with EXPLAIN on SQLite prints a raw jq parse error and no envelope

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (rc.A port of llm_* and internal/db) |
| Area | Contract |
| Bash location | `lib/modules/llm/db.sh` SQLite path into `__llm_db_envelope` (`lib/modules/llm/core.sh`) |
| Observed behavior | `EXPLAIN …` and `EXPLAIN QUERY PLAN …` make the sqlite3 shell print a text table, not JSON, so the envelope's jq step fails: `jq: parse error: Invalid numeric literal at line 1, column N` on stderr, exit 5, nothing on stdout. |
| Reproduction | `llm_db_query "EXPLAIN SELECT 1" -f db.sqlite` (temp SQLite) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced (`internal/db` envelope): the same message, exit 5, no stdout. |
| Fix recipe (3.0) | Wrap the plan text as `[{"plan": TEXT}]` the way `llm_db_explain` does, or refuse EXPLAIN with a hint naming `llm_db_explain`. |
| Proving test | `TestLedger_DB14_2x` in `internal/db` (rc.A bytes); `TestLedger_DB14_fix` (3.0). |
| Status | accept: done af24d8f |

### llm_exec

<a id="exec-01"></a>
#### EXEC-01 · attached short-option values and clusters skip the secret-file and forbidden-path checks

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "attached short-option values / clusters skip secret-file and forbidden-path checks" (CONFIRMED); also NF3 |
| Area | Security |
| Bash location | `lib/modules/llm/exec.sh:649-668` |
| Observed behavior | For a word shaped `-<letters><value>` the `-?*` branch strips exactly two characters and runs only the forbidden-path check (never the secret-file check) on the remainder, and a cluster like `-uf<value>` leaves the real option value unexamined. So a secret file or a protected path reached through an attached option value (e.g. a `-f`-style option that reads a file as data) is read: it bypasses both the secret-file refusal and, for a cluster, the forbidden-path refusal. The same hole lets a pattern-file option feed a secret file to grep/rg/sed. |
| Reproduction | with a secret marker file in a temp cwd, an allowed walker command plus `-f<that file>` reads it instead of being refused (temp HOME; the exploiting argument shapes are described, not a working line) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A ports the `-?*` two-character strip and the forbidden-only check, so the same attached-value bypass exists. |
| Fix recipe (3.0) | In `internal/llmexec` argument checking, split option letters from an attached value using the per-command value-taking-option tables (the walk tables already encode which short options take a value), then run BOTH the secret-file check (`secret_file_blocked`/FATAL) and the forbidden-path check (`forbidden_path`/FATAL) on the extracted value; drop the fixed 2-character assumption. |
| Proving test | `TestLedger_EXEC01` asserts an attached-value secret file -> `secret_file_blocked` and an attached-value protected path -> `forbidden_path`, added alongside the S12 vectors. |
| Status | accept: done 697f422 |

<a id="exec-02"></a>
#### EXEC-02 · `git stash list -p` leaks secret files

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "git stash list -p leaks secret files" (CONFIRMED); also NF2 |
| Area | Security |
| Bash location | `lib/modules/llm/exec.sh:555-573`; child wrapper `exec.sh:1012-1036` |
| Observed behavior | `git stash` is allow-listed and the per-subcommand `stash` rule only constrains `stash show`, placing no option rule on `stash list`; the child git wrapper `__llm_cx_git` has no `stash` branch, so it adds no `:(exclude)` secret pathspecs. `git stash list -p` therefore prints the full patch of stashed changes, including `.env` and other secret files (effectively an unconstrained `git log -p` over the stash reflog). |
| Reproduction | a repo with a stashed change touching a secret file, then `llm_exec "git stash list -p"` prints the secret (temp repo) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A allows `git stash list -p` and applies no exclude pathspecs, leaking the same files. |
| Fix recipe (3.0) | In `internal/llmexec` treat `git stash list` like `log`: when a patch option is present, apply the `:(exclude,icase,glob)**/PATTERN` and `**/DIR/**` secret pathspecs and `--no-ext-diff --no-textconv`; or refuse patch options on `stash list`. Also gate other `stash list` options (NF2) the same way `log` is gated. |
| Proving test | `TestLedger_EXEC02` asserts `git stash list -p` shows the non-secret line and never the secret file, in `llm_exec_walk_spec.sh`'s Go form. |
| Status | accept: done e581039 |

<a id="exec-08"></a>
#### EXEC-08 · A line continuation between two `|` is checked as two pipes, run as `||`

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (rc.A port of the llm_exec renderer) |
| Area | Correctness |
| Bash location | `lib/modules/llm/exec.sh:847-852` |
| Observed behavior | The 2.x parser looks at the next character before it handles a backslash-newline, so `\|`, a line continuation, then `\|` is checked as two pipes around an empty command, while bash joins the continuation and reads `\|\|`. The words checked are the same either way; only the operator differs. |
| Reproduction | two allowed commands joined by `\|`, backslash-newline, `\|`: 2.x checks a pipeline and bash runs an or-list (temp cwd) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: the canonical script keeps the continuation, so bash runs what 2.x ran. |
| Fix recipe (3.0) | In `internal/llmexec`, consume backslash-newline before looking at the next character, so the checked operator is the one bash reads. |
| Proving test | `TestLedger_EXEC08` pins the rc.A output against 2.x for this line. |
| Status | accept: done 30410b6 |

### system, sysinfo, ssh, chrome

<a id="sys-01"></a>
#### SYS-01 · process_kill takes NAME as a pgrep regex: `.*` matches every process

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N13); lead decision: reproduce in rc.A |
| Area | Security |
| Bash location | `lib/modules/system.sh:643` (`pgrep -ix "$process_name"`); help example `lib/modules/system.sh:1218`, `docs/reference/system.md:312` |
| Observed behavior | NAME is an anchored, case-insensitive ERE on the 15-byte `comm`, so `.*`, `a.*` or `node\|npm` select unrelated processes, PID 1 included (with `-y` as root: the whole machine). Names over 15 bytes never match, so the documented `pk -y webpack-dev-server` finds nothing (pgrep's warning is discarded by `2>/dev/null`). |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; process_kill ".*"'` (headless refuses: `Error: 83 process(es) match '.*' (PID 1 2 3 ...)`; never add `-y`) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A runs `pgrep -ix NAME` exactly as 2.x and keeps the headless refusal and the prompt. Only the binary's own PID (a process 2.x did not have) is removed from the list. |
| Fix recipe (3.0) | `internal/system` `processKill`: escape every ERE metacharacter of NAME (`\ . [ ] ( ) * + ? { } \| ^ $`) and run `pgrep -ix -- ESCAPED`; when NAME is longer than 15 bytes, run `pgrep -ix -- ESCAPED15` on its first 15 bytes and keep only PIDs whose `/proc/PID/cmdline` argv[0] basename equals NAME ignoring case. No new error code: no match keeps `Error: No process found with exact name 'NAME'. Use process_find (pf) to inspect candidates.` rc 1. Help NOTES gain "NAME is a name, not a pattern"; regenerate `docs/reference/system.md`. |
| Proving test | `TestLedger_SYS01` in `internal/system` (fake runner: `.*` yields argv `pgrep -ix -- \.\*`; a 19-byte NAME is matched through a cmdline fixture) and `testdata/script/ledger_sys01.txtar` (headless `process_kill '.*'` prints the not-found error, rc 1) |
| Status | accept: done 44a54b7 |

<a id="sys-02"></a>
#### SYS-02 · process_kill and port_kill report a process they may not signal as "terminated"

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N14) |
| Area | Correctness |
| Bash location | `lib/modules/system.sh:681-701` (process_kill), `lib/modules/system.sh:763-781` (port_kill) |
| Observed behavior | The liveness poll is `kill -0 PID`, whose EPERM failure counts as "dead": an unprivileged `pk -y NAME` on a root-owned process prints `Info: Process P terminated.` and returns 0 while the process keeps running. |
| Reproduction | `runuser -u nobody -- env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; process_kill -y gashtestslp'` after root started `$tmp/gashtestslp 300` (a copy of sleep in a temporary directory) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced with kill(2): EPERM from the TERM or the 0-signal probe is treated as "terminated", rc 0. |
| Fix recipe (3.0) | `internal/system` shared `terminate(pids)`: on EPERM print `Error: Process P: permission denied.` and continue; return 1 when any PID was refused. ESRCH keeps meaning terminated. Help NOTES of both functions: "Returns 1 when a process could not be signalled". |
| Proving test | `TestLedger_SYS02` in `internal/system` (fake kill returning EPERM → message and rc 1) |
| Status | accept: done ed9ec21 |

<a id="sys-03"></a>
#### SYS-03 · port_kill also targets UDP clients whose remote port is PORT

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N16) |
| Area | Correctness |
| Bash location | `lib/modules/system.sh:737` (`lsof -t -nP -iUDP:PORT`); help NOTES `lib/modules/system.sh:1245-1246` |
| Observed behavior | `-iUDP:PORT` matches a socket whose local **or remote** port is PORT, so a connected UDP client of a remote service on that port is listed and, after the question or with `-y`, killed. The help promises "Only listeners are targeted". |
| Reproduction | `python3 -c 'import socket,time; s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.connect(("127.0.0.1",18053)); time.sleep(30)' & env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; port_kill 18053'` (lists the client, then `Info: Aborted.`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A runs the same two lsof queries and merges them with `sort -un` semantics. |
| Fix recipe (3.0) | `internal/system` `portKill`: keep the TCP query; for UDP run `lsof -nP -iUDP:PORT -F pn` and keep only PIDs whose name field has no `->` (unconnected) and whose local port is PORT. Help NOTES unchanged (they become true). |
| Proving test | `TestLedger_SYS03` in `internal/system` (lsof `-F` fixture with a `127.0.0.1:40000->127.0.0.1:18053` socket → not targeted) |
| Status | accept: done 6d329e0 |

<a id="sys-04"></a>
#### SYS-04 · process_find returns 0 when nothing matches and matches the whole ps line

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N12) |
| Area | Contract |
| Bash location | `lib/modules/system.sh:602-612` |
| Observed behavior | No match prints `Error: No process found with name 'NAME'.` and returns 0. The match runs on the whole `ps aux` line: `pf PID` lists the header as a process, a calling `bash -c` whose script contains NAME is listed, and any line containing `grep` is dropped (`pf grep`, `pf ripgrep` find nothing). |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; process_find zz""zz; echo rc=$?'` (`rc=0`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: `ps aux` filtered as `grep -i NAME \| grep -v grep` (dialect: SYS-38), table of `$2 $1 $3 $11`, rc 0 on no match. 2.x also listed its own pipeline subshells (forks whose command line contained NAME); they do not exist in rc.A, and the binary drops its own row. |
| Fix recipe (3.0) | `internal/system` `processFind`: match NAME (case-insensitive, literal) against the COMMAND field only (`ps -eo pid,user,pcpu,args`), never the header; exclude only the gash process; no match returns 1 after the same error line. |
| Proving test | `TestLedger_SYS04` in `internal/system` (ps fixture: header and a `grep`-named process; no-match rc 1) |
| Status | accept: done 3750b94 |

<a id="sys-05"></a>
#### SYS-05 · hgrep -E with an invalid regex reports success

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N3) |
| Area | Contract |
| Bash location | `lib/modules/system.sh:304-329` (`\|\| return 1` sees only the last pipeline stage) |
| Observed behavior | awk dies on the regex (`gawk: ... fatal: invalid regexp: Unmatched ( or \(`; mawk: `regular expression compile failed`), stdout is `[]` with `-j`, `0` with `-c`, nothing otherwise, and the status is 0. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; printf "#1700000000\ngit a\n" >"$HOME/h"; HISTFILE="$HOME/h" hgrep -E "(" -j; echo rc=$?'` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: empty result (`[]`, `0` or nothing) and rc 0. The awk diagnostic itself cannot be kept (its text depended on the awk, SYS-32): rc.A prints one stderr line `Error: Invalid regex: PATTERN` in its place. |
| Fix recipe (3.0) | `internal/system` `hgrep`: compile the pattern before reading the file; on failure print `Error: Invalid regex: PATTERN` and return 1 (all output modes). Help OPTIONS for `-E`: "an invalid regex is an error". |
| Proving test | `TestLedger_SYS05` in `internal/system` and `testdata/script/ledger_sys05.txtar` (`hgrep -E '(' -j` → rc 1, empty stdout) |
| Status | accept: done 105f468 |

<a id="sys-06"></a>
#### SYS-06 · hgrep --json emits invalid JSON for control characters; CRLF history entries are lost

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N5) |
| Area | Contract |
| Bash location | `lib/modules/system.sh:351-355` (`jesc`), `lib/modules/system.sh:308` (`/^#[0-9]+$/`) |
| Observed behavior | `jesc` escapes only `\`, `"`, newline and tab, so a command containing ESC, CR or another control byte is written raw and the array does not parse. In a CRLF history file `#123\r` does not match the record regex and every entry is skipped; a lone `\r` at the end of a command line stays in the command. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; printf "#1700000000\necho \033[31mred\n" >"$HOME/h"; HISTFILE="$HOME/h" hgrep echo -j \| jq .'` (`jq: parse error: Invalid string: control characters ...`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced byte for byte: the contract encoder writes the 2.x `jesc` escaping (raw control bytes), and CRLF records are skipped. |
| Fix recipe (3.0) | `internal/system` `hgrep`: encode with the jq-compatible encoder of `internal/contract` (`\u001b`, `\r`, other controls as `\u00xx`); strip one trailing `\r` from both the `#epoch` line and the command line before matching. |
| Proving test | `TestLedger_SYS06` in `internal/system` (ESC and CR commands → output parses and round-trips; CRLF file yields its records) |
| Status | accept: done 105f468 |

<a id="sys-07"></a>
#### SYS-07 · hgrep cannot search a pattern that starts with `-`; `-h` is help only as the first word

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N8) |
| Area | Contract |
| Bash location | `lib/modules/system.sh:163`, `lib/modules/system.sh:264-267` |
| Observed behavior | Every word starting with `-` is an option, `--` included: `hgrep -- --force` fails with `Error: Unknown option: --. Use -h for help.` rc 1, so `git push --force` cannot be looked up. `hgrep git -h` is `Unknown option: -h`. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; : >"$HOME/h"; HISTFILE="$HOME/h" hgrep -- --force; echo rc=$?'` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced by the 2.x grammar of `internal/cli/args` (no `--` terminator, help only as `$1`). |
| Fix recipe (3.0) | `internal/cli/args` hgrep grammar: `--` ends options (next word is PATTERN even if it starts with `-`); `-h`/`--help` anywhere prints help. Help OPTIONS gain `--  end of options`. |
| Proving test | `TestLedger_SYS07` in `internal/cli/args` (vectors `-- --force` → pattern `--force`; `git -h` → help) |
| Status | accept: done 105f468 |

<a id="sys-08"></a>
#### SYS-08 · history_grep passes extra words to grep as FILE operands

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N10) |
| Area | Correctness |
| Bash location | `lib/modules/system.sh:125` (`grep -i -- "$@"`) |
| Observed behavior | Only the first word is the pattern; every further word is a file that grep reads instead of the history. `history_grep git push` in a directory holding a file `push` prints that file's matching lines, and `history_grep -- -v` greps for `--` in a file `-v` (`grep: -v: No such file or directory`, rc 0). |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; cd "$HOME"; printf "1 git from-a-file\n" >push; set -o history; history -s "git push origin"; history_grep git push'` (prints `git from-a-file`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: with more than one word the binary greps the named files (relative to the caller's cwd, grep's `name:` prefix with two or more files, GNU grep's missing-file message), ignoring the piped history; rc 0. |
| Fix recipe (3.0) | `internal/system` `historyGrep`: join all words with single spaces into one pattern and always read the piped history. Help USAGE becomes `history_grep PATTERN...` with the note "words are joined with spaces". |
| Proving test | `TestLedger_SYS08` in `internal/system` (words `git push` match the history entry `git push origin`, a file named `push` is never opened) |
| Status | accept: done 4734075 |

<a id="sys-09"></a>
#### SYS-09 · history_grep duplicates text in column 3; HISTTIMEFORMAT breaks columns and dedup; `hg` self-matches

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N11) |
| Area | Correctness |
| Bash location | `lib/modules/system.sh:125-127`; Gash's own `HISTTIMEFORMAT` at `lib/shell/history.sh:19` |
| Observed behavior | Column 3 is `substr($0, index($0,$3))`, which restarts at the first occurrence of word 3: `git push` shows `git push  git push`, `cp a a` shows `cp a  a a`. With `HISTTIMEFORMAT='%F %T '` (set by Gash in interactive shells) column 2 is the date and time and identical commands are never deduplicated. The self-exclusion is the literal `history_grep`, so `hg git` lines are listed. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; set -o history; history -s "cp a a"; history_grep cp'` (`1     cp a                 a a`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the binary applies the same two awk programs (field rebuild with single spaces, first-seen dedup on the rebuilt line, `NR`, `$1" "$2`, `index`-based remainder) to the text the shim pipes from `history`. |
| Fix recipe (3.0) | `internal/system` `historyGrep`: the shim pipes `HISTTIMEFORMAT= history` so no timestamp prefix reaches the binary; parse each line as `NUM CMD`; dedup on CMD; column 2 = first two words, column 3 = the rest by position; exclude lines whose first word is `history_grep` or `hg`. |
| Proving test | `TestLedger_SYS09` in `internal/system` (`git push`, `cp a a`, `ls ls ls`, two timestamped `git push origin` → one row each, exact columns) |
| Status | accept: done b6d161e |

<a id="sys-10"></a>
#### SYS-10 · Extra positional arguments are silently accepted

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N17) |
| Area | Contract |
| Bash location | `lib/modules/system.sh:633` (process_kill, last wins), `lib/modules/system.sh:721` (port_kill, last wins), `lib/modules/system.sh:886` (mkdir_cd, first wins); same pattern in process_find, wsl_explorer, sysinfo sections (SYS-20) |
| Observed behavior | `port_kill 80 81` reports `Nothing is listening on port 81.` (80 dropped), `process_kill a b` acts on `b`, `mkdir_cd a b` creates and enters `a`, with no warning. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; port_kill 80 81'` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced by the per-function 2.x grammars (last or first positional wins as in 2.x). |
| Fix recipe (3.0) | `internal/cli/args`: these grammars accept exactly one positional; a second one prints `Error: Unexpected argument: X` (the human form of the llm family's `unexpected_argument`) and returns 1. |
| Proving test | `TestLedger_SYS10` in `internal/cli/args` (one vector per function: two positionals → error, rc 1) |
| Status | accept: done b6d161e |

<a id="sys-11"></a>
#### SYS-11 · mkdir_cd follows CDPATH and passes option-like names to mkdir and cd

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N18) |
| Area | Correctness |
| Bash location | `lib/modules/system.sh:890` (`mkdir -p "$dir" && cd "$dir"`) |
| Observed behavior | With CDPATH set and `$CDPATH/foo` existing, `mkdir_cd foo` creates `./foo` but enters `$CDPATH/foo` and prints that path on stdout. `mkdir_cd -` creates `./-` and then `cd -` jumps to OLDPWD; `mkdir_cd -p` fails with `mkdir: missing operand`. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; cd "$HOME"; mkdir -p cdp/foo work; cd work; CDPATH="$HOME/cdp" mkdir_cd foo; echo "pwd=$PWD"'` (pwd is `$HOME/cdp/foo`, `$HOME/work/foo` was created) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the generated shim keeps `mkdir -p "$dir" && cd "$dir" \|\| return 1` verbatim in the caller's shell. |
| Fix recipe (3.0) | `internal/compatgen` mkdir_cd shim: `mkdir -p -- "$dir" && CDPATH= cd -- "$dir"`, with `./-` substituted for a literal `-`; no stdout. |
| Proving test | `TestLedger_SYS11` in `internal/compatgen` plus `testdata/script/ledger_sys11.txtar` (CDPATH set → PWD is `./foo`, stdout empty; `mkdir_cd -p` creates `./-p`) |
| Status | accept: done b6d161e |

<a id="sys-12"></a>
#### SYS-12 · sudo_last returns 0 when declined and asks even with no previous command

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N19) |
| Area | Contract |
| Bash location | `lib/modules/system.sh:868-877` |
| Observed behavior | Declined, headless or without a TTY: `Info: Aborted.` and status 0, unlike process_kill, port_kill and services_stop (1). Without history (non-interactive shells) the command is empty and it still prints `Warning: About to run as root: ` and asks. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; sudo_last; echo rc=$?'` (`rc=0`) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the shim reads `fc -ln -1`, the binary warns and asks, decline returns 0. |
| Fix recipe (3.0) | `internal/system` `sudoLast` (and its shim): empty recalled command → `Error: No previous command in history.` rc 1 before any warning; decline → `Info: Aborted.` rc 1. Help NOTES: "Declining returns 1". |
| Proving test | `TestLedger_SYS12` in `internal/system` and `testdata/script/ledger_sys12.txtar` (headless `plz` → rc 1) |
| Status | accept: done 2661b44 |

<a id="sys-13"></a>
#### SYS-13 · ip_public prints whatever the endpoint returns, with no timeout

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N20) |
| Area | Correctness |
| Bash location | `lib/modules/system.sh:567-583`; help example `lib/modules/system.sh:1181`, `docs/reference/system.md:181` |
| Observed behavior | The body is never validated: a captive portal or rate-limit page answered with 200 (or any error body on the curl path, which lacks `-f`) is printed as `Public IP: <html>...`. There is no timeout. The documented `MY_IP=$(myip)` captures the whole `Public IP: X` line, not the address. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; wget() { printf "<html>Rate limit</html>\n"; }; ip_public; echo rc=$?'` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced on the wget path (SYS-36): a 2xx body, trailing newlines stripped as `$(...)` did, is printed unvalidated with rc 0; no client timeout (Ctrl-C cancels). |
| Fix recipe (3.0) | `internal/system` `ipPublic`: 10 s timeout; accept only a body that `net/netip.ParseAddr` parses after trimming whitespace, else `Error: Failed to retrieve public IP address.` rc 1. Add `--raw` (address only) and change the help and docs example to `MY_IP=$(myip --raw)`. |
| Proving test | `TestLedger_SYS13` in `internal/system` (local HTTPS fake: HTML body → rc 1; slow server → timeout error; `--raw` prints the bare address) |
| Status | accept: done 168324c |

<a id="sys-14"></a>
#### SYS-14 · disk_usage fails entirely when df reports one unreadable mount

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N21) |
| Area | Correctness |
| Bash location | `lib/modules/system.sh:46-54` (`raw="$(df -PTk)" \|\| raw=""`) |
| Observed behavior | GNU df exits 1 when any mount fails (a dead sshfs or NFS mount) while still printing every other row; disk_usage discards the whole output, retries `df -Pk` (same failure) and prints `Error: disk_usage: df failed` rc 1. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; df() { printf "Filesystem Type 1024-blocks Used Available Capacity Mounted on\n/dev/vda ext4 100 50 50 50%% /\n"; return 1; }; disk_usage; echo rc=$?'` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A runs `df -PTk`, discards its stdout on a non-zero exit, falls back to `df -Pk` the same way, then fails with the 2.x message. |
| Fix recipe (3.0) | `internal/system` `diskUsage`: keep df's stdout whatever its exit status; treat only an empty stdout (or one without data rows) as failure; fall back to `-Pk` only when `-PTk` printed nothing. |
| Proving test | `TestLedger_SYS14` in `internal/system` (fake df: one ext4 row, exit 1 → row shown, rc 0) |
| Status | accept: done ef16f49 |

<a id="sys-15"></a>
#### SYS-15 · sysinfo asks for the sudo password for every section, even those that never use sudo

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N22) |
| Area | Correctness |
| Bash location | `lib/modules/sysinfo.sh:1844` (`__sysinfo_ensure_sudo` before the dispatch), `lib/modules/sysinfo.sh:37-95` |
| Observed behavior | On a terminal, a non-root user without cached sudo is asked for the password (`sudo -v`) and a keep-alive loop starts even for `sysinfo overview` and `sysinfo network`, which never call `__sysinfo_sudo`. |
| Reproduction | As a non-root user on a terminal, with a `sudo` stub first on PATH that appends its arguments to `$tmp/sudo.log` and exits 1: `sysinfo overview >/dev/null`; the log shows `-n true` then `-v` (by reading; root and headless runs never ask) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: rc.A calls the same sudo sequence (`sudo -n true`, then `sudo -v` on a TTY, keep-alive) before every section. |
| Fix recipe (3.0) | `internal/sysinfo`: each section declares `NeedsSudo` (identity, storage, services, auth, security, system; `all` when any does); acquire sudo only for those. |
| Proving test | `TestLedger_SYS15` in `internal/sysinfo` (fake runner on a fake TTY: `overview` and `network` issue no sudo call, `auth` issues `sudo -n true`) |
| Status | accept: done a80886c |

<a id="sys-16"></a>
#### SYS-16 · sysinfo prints `none` twice for Virtualization on bare metal

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N23) |
| Area | Correctness |
| Bash location | `lib/modules/sysinfo.sh:361` (`virt=$(systemd-detect-virt 2>/dev/null \|\| echo "none")`) |
| Observed behavior | systemd-detect-virt prints `none` and exits 1 when nothing is detected, so the value is `none\nnone`: llm mode prints `Virtualization=none` followed by a stray line `none`. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; systemd-detect-virt() { echo none; return 1; }; sysinfo identity --llm \| grep -A1 Virtualization'` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: stdout of the tool plus `none` on a non-zero exit, joined by a newline, as the kv value. |
| Fix recipe (3.0) | `internal/sysinfo` identity: value = trimmed stdout of `systemd-detect-virt`, `none` only when that is empty. |
| Proving test | `TestLedger_SYS16` in `internal/sysinfo` (fake tool printing `none`, exit 1 → exactly `Virtualization=none`) |
| Status | accept: done 835332a |

<a id="sys-17"></a>
#### SYS-17 · sysinfo shows a stale sub-header above unrelated lines

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N24) |
| Area | Correctness |
| Bash location | `lib/modules/sysinfo.sh:133-141` (lazy header flush), `lib/modules/sysinfo.sh:860-866` (Docker block), `lib/modules/sysinfo.sh:910-925` (cron items, /etc/ssl/private listing) |
| Observed behavior | A pending sub-header is printed by the next kv or item even when that line belongs to another block: with `/etc/docker` (no daemon.json) and `/etc/containerd`, `sysinfo infra --llm` prints `--- Docker` then `containerd=present`; `sysinfo system --llm` prints the /etc/ssl/private key names under the `cron.monthly:` item, which has no header of its own. |
| Reproduction | On a host with `/etc/docker` without `daemon.json` and `/etc/containerd`: `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; sysinfo infra --llm'` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the in-memory header state follows the 2.x rule (a pending sub-header is flushed by the next kv, item or raw line, whatever block it comes from). |
| Fix recipe (3.0) | `internal/sysinfo`: a block opens its sub-header and closes it at the block end (a pending header is dropped, never inherited); the /etc/ssl/private listing gets its own sub-header `SSL private keys`. |
| Proving test | `TestLedger_SYS17` in `internal/sysinfo` (sysroot with `/etc/docker` and `/etc/containerd` only → no `Docker` heading, `containerd=present` under `Containers`) |
| Status | accept: done c22cd6e |

<a id="sys-18"></a>
#### SYS-18 · sysinfo identity misreads quoted ID and unquoted VERSION_ID in /etc/os-release

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N25) |
| Area | Portability |
| Bash location | `lib/modules/sysinfo.sh:343-345` (`grep -oP '^ID=\K\w+'`, `'^VERSION_ID="\K[^"]+'`) |
| Observed behavior | `ID="rhel"` and `ID="opensuse-leap"` give an empty ID, `ID=opensuse-tumbleweed` gives `opensuse`; `VERSION_ID=39` (Fedora) and `VERSION_ID=3.19.1` (Alpine) give an empty version. Without GNU grep `-P` (BusyBox, BSD) all three are empty. |
| Reproduction | `printf 'ID="rhel"\nVERSION_ID=39\n' \| grep -oP '^ID=\K\w+'; printf 'VERSION_ID=39\n' \| grep -oP '^VERSION_ID="\K[^"]+'` (both print nothing: the exact 2.x expressions) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced on the GNU grep `-P` path: the same three expressions with Go regexp (`\w` ASCII as in PCRE without UCP); the no-`-P` path (all empty) is not reproduced. |
| Fix recipe (3.0) | `internal/sysinfo` identity: use the os-release parser of `internal/platform` (KEY=VALUE, optional single or double quotes, backslash escapes per os-release(5)) for ID, PRETTY_NAME and VERSION_ID. |
| Proving test | `TestLedger_SYS18` in `internal/sysinfo` (table of Debian, Ubuntu, RHEL, Fedora, openSUSE, Alpine os-release fixtures) |
| Status | accept: done 4b4b9a8 |

<a id="sys-19"></a>
#### SYS-19 · sysinfo expands globs for sudo commands as the unprivileged user

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N26) |
| Area | Correctness |
| Bash location | `lib/modules/sysinfo.sh:925` (`__sysinfo_sudo ls /etc/ssl/private/*.pem ...`) |
| Observed behavior | `/etc/ssl/private` is 0710 root:ssl-cert, so for a normal user the globs stay literal and the root `ls` fails on the literal names (stderr discarded): the key names are never listed although sudo was granted. |
| Reproduction | As a non-root user with sudo rights: `sysinfo system --llm \| grep /etc/ssl/private` prints nothing while `sudo ls /etc/ssl/private` lists keys (by reading) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: rc.A expands the patterns as the invoking user with default Bash rules (an unmatched pattern stays literal) and passes the words to `sudo ls`. |
| Fix recipe (3.0) | `internal/sysinfo` system section: list the directory as root (`sudo find /etc/ssl/private -maxdepth 1 -name '*.pem' -o -name '*.key' -o -name '*.crt'`), sort, cap at 10. |
| Proving test | `TestLedger_SYS19` in `internal/sysinfo` (fake runner: one root-side listing call, its names printed) |
| Status | accept: done 835332a |

<a id="sys-20"></a>
#### SYS-20 · sysinfo silently keeps the last of several sections; `--help` after another word is an error

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N28 a, b) |
| Area | Contract |
| Bash location | `lib/modules/sysinfo.sh:1798-1830` |
| Observed behavior | `sysinfo identity storage --llm` prints only STORAGE, without a warning. `sysinfo --llm --help` prints `Error: Unknown argument: --help` plus the two `Info:` lines and returns 1 (help only as the first word). |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; sysinfo identity storage --llm \| grep "^==="; sysinfo --llm --help; echo rc=$?'` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced by the 2.x sysinfo grammar in `internal/cli/args`. |
| Fix recipe (3.0) | `internal/cli/args` sysinfo grammar: a second section prints `Error: Only one section allowed: X` + the existing `Info: Sections: ...` line, rc 1; `-h`/`--help` anywhere prints help. |
| Proving test | `TestLedger_SYS20` in `internal/cli/args` (vectors: two sections → error; `--llm --help` → help, rc 0) |
| Status | accept: done b0c62c3 |

<a id="sys-21"></a>
#### SYS-21 · gash_ssh_auto_unlock does not recover from a stale SSH_AUTH_SOCK

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N30, inferred from OpenSSH `ssh-add.c`) |
| Area | Correctness |
| Bash location | `lib/modules/ssh.sh:216-225` (exact text `Could not open a connection to your authentication agent`) |
| Observed behavior | Only an unset SSH_AUTH_SOCK yields that text; a socket left by a dead agent yields `Error connecting to agent: Connection refused` (exit 2), so no agent is started, every key fails inside the unlock and its lines are shown as `SSH: Error connecting to agent: ...` at info level, and RAN=1 is exported. |
| Reproduction | With an `ssh-add` stub first on PATH that prints `Error connecting to agent: Connection refused` on stderr and exits 2 for `-l`, the repo `expect` mock and one `SSH:` entry in a temporary `~/.gash_env`: `gash_ssh_auto_unlock` never calls `ssh-agent` |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Reproduced: rc.A matches the same text in `ssh-add -l` output and ignores its exit status. |
| Fix recipe (3.0) | `internal/ssh` `autoUnlock`: treat `ssh-add -l` exit status 2 (any message) as "no agent" and run the agent search; keep the 2.x error line when that fails. |
| Proving test | `TestLedger_SYS21` in `internal/ssh` (fake runner: `ssh-add -l` exit 2 with the refused text → agent start attempted) |
| Status | accept: done e541cf8 |

<a id="sys-22"></a>
#### SYS-22 · The agent search chmods an existing directory to 700; the expect hint goes to stdout

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N32) |
| Area | Correctness |
| Bash location | `lib/modules/ssh.sh:97` (`mkdir -p "$dir" && chmod 700 "$dir"`), `lib/modules/ssh.sh:234` (hint printf on stdout) |
| Observed behavior | Every agent search runs `chmod 700` on `${XDG_RUNTIME_DIR:-~/.ssh}`, also when the directory existed with other modes. When expect is missing, the install hint is printed on stdout during interactive startup, the only non-banner stdout line of startup. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH="$PWD/tests/mocks/bin:/usr/bin:/bin" LANG=C.UTF-8 MOCK_SSH_AGENT=0 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; mkdir -p ~/.ssh ~/run; chmod 755 ~/run; : >~/.ssh/key1; printf "SSH:~/.ssh/key1=pw\n" >~/.gash_env; chmod 600 ~/.gash_env; XDG_RUNTIME_DIR=~/run gash_ssh_auto_unlock; stat -c %a ~/run'` (prints `700`) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | chmod reproduced (`os.MkdirAll` then `os.Chmod(dir, 0700)`, errors ignored). The stdout hint is unreachable in rc.A, which needs no expect (SYS-37). |
| Fix recipe (3.0) | `internal/ssh` agent search: `chmod 700` only a directory it created itself; an existing directory with group or other write access is refused for the socket (fallback `~/.ssh`). If any install hint remains, print it on stderr. |
| Proving test | `TestLedger_SYS22` in `internal/ssh` (existing 0755 runtime dir keeps its mode) |
| Status | accept: done e541cf8 |

<a id="sys-23"></a>
#### SYS-23 · chrome_headless_update loses the installed version under a symlinked or relative GASH_CHROME_ROOT

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N33) |
| Area | Correctness |
| Bash location | `lib/modules/chrome.sh:141-146` (`readlink -f LINK` compared with the unresolved ROOT prefix) |
| Observed behavior | The resolved link target never starts with a symlinked or relative ROOT, so `Installed: none`, the plan is a NEW install and every kept version, the current one included, is listed under `REMOVED`; after such an install every later run re-downloads instead of saying it is up to date. |
| Reproduction | With the stubs of `tests/specs/chrome_spec.sh` (`MOCKS`), `$tmp/real/{139.0.7258.5,140.0.7339.16}/chrome-headless-shell-linux64/chrome-headless-shell`, `ln -s real $tmp/rootlink` and the link pointing through `rootlink/140.0.7339.16`: `GASH_CHROME_ROOT=$tmp/rootlink chrome_headless_update --dry-run` (shows `REMOVED: 139.0.7258.5 140.0.7339.16`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A resolves only the link (readlink -f semantics) and strips the literal `ROOT/` prefix. |
| Fix recipe (3.0) | `internal/dev` `chromeHeadlessUpdate`: resolve ROOT with `filepath.EvalSymlinks(filepath.Abs(ROOT))` (unresolved when it does not exist yet) before the prefix test and the version listing. |
| Proving test | `TestLedger_SYS23` in `internal/dev` (fixture tree with a symlinked root → `Installed: 140.0.7339.16`, `Kept for rollback`) |
| Status | accept: done 38ea0ec |

<a id="sys-24"></a>
#### SYS-24 · chrome_headless_update: version check is a substring, a pre-existing target is linked unverified

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N34) |
| Area | Security |
| Bash location | `lib/modules/chrome.sh:234` (`[[ "$got" != *"$target"* ]]`), `lib/modules/chrome.sh:243-248` |
| Observed behavior | No checksum or signature exists (Chrome for Testing publishes none); the only check is that the downloaded binary's `--version` output contains V. When `ROOT/V/chrome-headless-shell-linux64/chrome-headless-shell` is already executable (half install, manual copy), the verified download is discarded and the existing, never-checked file is linked. |
| Reproduction | With the chrome_spec stubs, a sudo stub that runs its arguments, and an executable `$GASH_CHROME_ROOT/141.0.7390.37/chrome-headless-shell-linux64/chrome-headless-shell` that prints `bogus`: `chrome_headless_update -y` links that file (by reading; temporary root only) |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Reproduced: substring test and the "target exists" short cut are kept. |
| Fix recipe (3.0) | `internal/dev`: parse `Chromium X.Y.Z.W` / `Google Chrome for Testing X.Y.Z.W` from `--version` and compare V exactly; when the target exists, run its `--version` with the same exact check and replace it with the verified tree when it fails (`Info: Replacing an unverified copy of V.`). |
| Proving test | `TestLedger_SYS24` in `internal/dev` (pre-seeded bogus executable is replaced; `--version` printing `V-extra` is refused) |
| Status | accept: done 38ea0ec |

<a id="sys-25"></a>
#### SYS-25 · services_stop, wsl_compact and chrome_headless_update call sudo without -n in unattended runs

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N35) |
| Area | Contract |
| Bash location | `lib/modules/system.sh:845-847` (services_stop), `lib/modules/system.sh:1035` (wsl_compact), `lib/modules/chrome.sh:60,244-248` |
| Observed behavior | With `-y` in headless mode or from an agent, sudo is plain `sudo`: it waits for a password on the terminal or fails with "a terminal is required", and chrome uses sudo even when ROOT and the link directory are writable by the user. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH="$PWD/tests/mocks/bin:/usr/bin:/bin" LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; sudo() { printf "sudo %s\n" "$*" >&2; }; __GASH_SYSTEMD_RUNDIR=$HOME MOCK_SYSTEMCTL_ACTIVE=mysql services_stop -y'` (prints `sudo systemctl stop mysql`, no `-n`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the same sudo argv, inheriting the caller's terminal. |
| Fix recipe (3.0) | `internal/runner` sudo helper: headless or no `/dev/tty` → `sudo -n ...`; on its "a password is required" failure print `Error: sudo needs a password: run interactively or configure NOPASSWD.` rc 1. `internal/dev`: skip sudo when ROOT and `dirname LINK` are writable. |
| Proving test | `TestLedger_SYS25` in `internal/runner` and `internal/dev` (headless → argv starts `sudo -n`; writable root → no sudo) |
| Status | accept: done 38ea0ec, 7607509 |

<a id="sys-26"></a>
#### SYS-26 · sysinfo completion does not offer `overview`

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "sysinfo lacks `overview` in bash_completion" (CONFIRMED) |
| Area | Completion |
| Bash location | `bash_completion:137-139`; accepted sections `lib/modules/sysinfo.sh:1819`, help `lib/modules/sysinfo.sh:1800` |
| Observed behavior | The word list is `identity storage services auth network security webstack mail infra system all --llm --no-color`; `sysinfo ov<Tab>` completes nothing. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; source "$G/bash_completion"; COMP_WORDS=(sysinfo ov); COMP_CWORD=1; __gash_complete; echo "[${COMPREPLY[*]}]"'` (`[]`) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: the generated `bash_completion` carries the 2.x word list. |
| Fix recipe (3.0) | `internal/registry` sysinfo entry: one section list feeding parser, help and completion; `internal/compatgen` emits `overview identity storage services auth network security webstack mail infra system all --llm --no-color`. |
| Proving test | `TestLedger_SYS26` in `internal/compatgen` (completion words equal the registry section list plus flags) and completion_spec.sh case `sysinfo ov` → `overview` |
| Status | accept: done c89d025 |

<a id="sys-27"></a>
#### SYS-27 · No completion for the flags of hgrep, port_kill, process_kill, services_stop, sudo_last and the wsl stop functions

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N37) |
| Area | Completion |
| Bash location | `bash_completion` (no case for hgrep, history_grep, port_kill, process_kill, process_find, services_stop, sudo_last, wsl_restart, wsl_shutdown, wsl_explorer, wsl_taskmanager, gash_ssh_auto_unlock) |
| Observed behavior | These fall to `COMPREPLY=()` with `-o default` (file names), so `-y`, `--dry-run` and the hgrep options are never offered. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; source "$G/bash_completion"; COMP_WORDS=(port_kill -); COMP_CWORD=1; __gash_complete; echo "[${COMPREPLY[*]}]"'` (`[]`) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: rc.A generates the 2.x completion cases only. |
| Fix recipe (3.0) | `internal/compatgen`: emit flag completion for every function from its registry grammar (`-y --yes -f --force` for the kill functions, `-y --yes --dry-run` for services_stop, the eleven hgrep options). |
| Proving test | `TestLedger_SYS27` in `internal/compatgen` (each function's completion list equals its grammar's flags) |
| Status | accept: done c89d025 |

<a id="sys-50"></a>
#### SYS-50 · deep_services WireGuard redaction misses PrivateKey without spaces and every PresharedKey

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (lead pin, sysinfo deep_services) |
| Area | Security |
| Bash location | `lib/modules/sysinfo.sh:1222-1230` (`__sysinfo_deep_services` WireGuard block; redaction `sed 's/PrivateKey = .*/PrivateKey = [REDACTED]/'` at :1228) |
| Observed behavior | The redaction matches only `PrivateKey = ` with a space on each side of `=`. A `PrivateKey=<value>` line written without spaces, an indented key line, and every `PresharedKey` line (which the sed never names) are printed in clear. Through `sysinfo services` (deep) or an AI collector these lines reach the model, so a WireGuard private or pre-shared key leaves the host. |
| Reproduction | A `wg0.conf` under `/etc/wireguard` with a no-space `PrivateKey=` line and a `PresharedKey = ` line: the deep services section prints both values unredacted while a spaced `PrivateKey = ` line is masked. (The input shape is a WireGuard config with those line forms; no key material shown here.) |
| Severity | medium |
| Public-contract impact | yes (the redaction is a stated privacy guarantee) |
| rc.A behavior | Reproduced: `internal/sysinfo/deep.go:80` compiles `PrivateKey = .*` and applies it at :350-357 only to lines matching that exact prefix, so `PrivateKey=nospace`, an indented key and every `PresharedKey` line pass through, as the `deep_test` fixtures confirm. |
| Fix recipe (3.0) | `internal/sysinfo` WireGuard redaction: match `^[[:space:]]*(PrivateKey\|PresharedKey)[[:space:]]*=` case-insensitively and replace the value with `[REDACTED]`, so both key kinds and all spacing variants are masked. Docs: the sysinfo privacy note lists PresharedKey. |
| Proving test | `TestLedger_SYS50_fix` in `internal/sysinfo` (fixture with `PrivateKey=`, `PrivateKey = `, indented and `PresharedKey = ` lines: every value `[REDACTED]`); `TestLedger_SYS50_2x` proves the rc.A reproduction |
| Status | accept: done d81ba18 |

<a id="sys-51"></a>
#### SYS-51 · deep_performance I/O-scheduler line reports the queue directory, not the block device

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (lead pin, sysinfo deep_performance) |
| Area | Correctness |
| Bash location | `lib/modules/sysinfo.sh:1668-1673` (`__sysinfo_deep_performance` I/O-scheduler loop; `echo "$(dirname "$sched" \| xargs basename): ..."` at :1672) |
| Observed behavior | For `/sys/block/sda/queue/scheduler`, `dirname` gives `/sys/block/sda/queue` and `basename` gives `queue`, so every device is labelled `queue:` instead of `sda:`. The scheduler's block device is taken from the wrong path component (one level too deep), so the drill-down cannot tell the devices apart. |
| Reproduction | With `/sys/block/sda/queue/scheduler` and `/sys/block/vda/queue/scheduler` present, the I/O-scheduler section prints two `queue: ...` lines, never `sda:`/`vda:`. |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Reproduced: `internal/sysinfo/deep.go:778-784` prints the literal `"queue: "` prefix (the source comment records that `$(dirname … \| xargs basename)` is the queue directory's name, never the device's). |
| Fix recipe (3.0) | `internal/sysinfo` I/O-scheduler: take the device name from the component two levels up (`/sys/block/<dev>/queue/scheduler` → `<dev>`) and print `<dev>: <scheduler>`. |
| Proving test | `TestLedger_SYS51_fix` in `internal/sysinfo` (`sda`/`vda` fixtures → `sda:`/`vda:`); `TestLedger_SYS51_2x` proves the `queue:` reproduction |
| Status | accept: done d81ba18 |

<a id="sys-52"></a>
#### SYS-52 · deep collectors glue the next section header onto a file with no final newline

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (lead pin, sysinfo deep collectors) |
| Area | Correctness |
| Bash location | `lib/modules/sysinfo.sh` deep `cat … \| head -N` pipelines, e.g. `:1320-1321` (custom systemd unit), recurring at `:1145`/`:1149`/`:1153` (OpenDKIM), `:1162` (BIND), `:1248` (NVIDIA), `:1258` (Collectd), `:1269` (Monit) |
| Observed behavior | A collected file whose last line has no trailing newline passes through `cat … \| head -N` unchanged (head copies the final partial line verbatim), so the next `echo "--- <header>"` is appended directly to that last line, with no line break between the file's final line and the following section header. |
| Reproduction | A deep-collected config file ending without a newline (e.g. a custom systemd unit file) followed by another section: the section header is printed on the same line as the file's last line. |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: `internal/sysinfo/text.go:247` `headN` keeps the last line as it is when the input ends without a newline, and `deep.go:72` `raw` writes those bytes verbatim (e.g. `deep.go:501`), so the following `echo` header glues on, byte for byte as 2.x. |
| Fix recipe (3.0) | `internal/sysinfo` deep output: after a `raw`/`headN` chunk that does not end in `\n`, emit a newline before the next header (equivalently, normalise each collected chunk to end with one newline). |
| Proving test | `TestLedger_SYS52_fix` in `internal/sysinfo` (a no-final-newline fixture: the next `--- ` header starts on its own line); `TestLedger_SYS52_2x` proves the glued reproduction |
| Status | accept: done d81ba18 |

### docker and compose

<a id="dock-01"></a>
#### DOCK-01 · Compose functions act on the current directory when the path argument is empty

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "Compose functions: `realpath -m` failure falls back to $PWD (wrong directory)" (CONFIRMED; the part GNU realpath reaches) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:546,861,1057` |
| Observed behavior | `path="$(realpath -m "$path" 2>/dev/null)" \|\| path="$PWD"`: GNU `realpath -m ''` fails, so an empty PATH argument (an unset variable in `dcup2 "$DIR" --yes`) checks, upgrades or scans the current directory, rc 0. `llm_docker_check ''` maps empty to `.` on purpose; these three do it by accident. |
| Reproduction | `mkdir $T/p && printf 'services:\n  cwdsvc:\n    image: nginx:latest\n' >$T/p/compose.yml && gm 'cd $T/p; docker_compose_check "" --json'` → `{"path":"<T>/p",...,"name":"cwdsvc",...}` rc 0 |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced: an empty PATH argument resolves to the canonical current directory, same output and exit status. |
| Fix recipe (3.0) | `internal/docker` `resolveComposeDir(raw)` (used by check, upgrade and scan): an empty argument or a resolution failure is refused. Human: `Error: Cannot resolve path: '<raw>'` rc 1. With `--json` (check, scan): `{"error":"path_not_found","details":"<raw>","action":"RETRY","recoverable":true,"hint":"Pass the directory that holds the compose file (omit it for the current directory)"}` rc 1. Omitting the argument still means `.`. docs/reference/docker.md usage notes. |
| Proving test | `TestLedger_DOCK01` in `internal/docker` (empty argument → rc 1, the runner fake records no docker call) and `testdata/script/dock01_empty_path.txtar` (`docker_compose_check '' --json` → stderr envelope `path_not_found`, empty stdout) |
| Status | accept: done 09ca07f |

<a id="dock-03"></a>
#### DOCK-03 · Completion: dcup2 lacks --yes; docker_prune_all, git_repair, git_sync -m and remote_* incomplete

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "Completion: dcup2 lacks --yes" (CONFIRMED) |
| Area | Completion |
| Bash location | `bash_completion:157-163` (docker_compose_upgrade), `bash_completion:106-108` (git_sync); no case for docker_prune_all, git_repair, remote_* |
| Observed behavior | `dcup2 -<TAB>` and `docker_compose_upgrade -<TAB>` offer `--dry-run --force --no-color --help` (no `--yes`, which headless use requires); docker_prune_all and git_repair complete no flags; git_sync offers `--message` but not `-m`; remote_* complete no flags. |
| Reproduction | `g 'source $GASH_SRC/bash_completion; COMP_WORDS=(dcup2 -); COMP_CWORD=1; __gash_complete; echo "${COMPREPLY[*]}"'` → `--dry-run --force --no-color --help` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | reproduced: the generated bash_completion keeps the 2.x word lists. |
| Fix recipe (3.0) | `internal/compatgen` builds each completion word list from the registry Grammar instead of the 2.x lists: docker_compose_upgrade `--dry-run --force -f --yes -y --no-color --help`; docker_prune_all and git_repair `-y --yes --dry-run --help`; git_sync `-m --message --no-push --dry-run --help`; remote_push and remote_pull `-r --recursive -p --preserve --no-progress --force --help`; remote_sync_* `--delete --dry-run --no-progress --force --exclude= --help`; remote_exec `--script`. |
| Proving test | `TestLedger_DOCK03` in `internal/compatgen` (for every registry entry the completion words equal its grammar's flags) and a completion_spec.sh case `COMP_WORDS=(dcup2 --y)` → `--yes` |
| Status | accept: done 63f58e1, 9eed0f7, 4fc4e85, e370352 |

<a id="dock-04"></a>
#### DOCK-04 · dcup2 ignores override files and COMPOSE_FILE, and --remove-orphans removes override-only services

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N1) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:68,883` |
| Observed behavior | Every compose call passes `-f <discovered file> --project-directory DIR`, so `docker-compose.override.yml` and `COMPOSE_FILE` (environment or DIR/.env) are not applied. The base images are parsed, and `up -d --remove-orphans S...` recreates the services without their override and removes containers of services that only an override defines (verified read-only with real compose: base `nginx:latest` vs auto-discovered `nginx:mainline` plus ports). |
| Reproduction | `mkdir $T/o && printf 'services:\n  web:\n    image: nginx:latest\n' >$T/o/docker-compose.yml && printf 'services:\n  web:\n    image: nginx:mainline\n' >$T/o/docker-compose.override.yml && gm 'cd $T/o; docker_compose_upgrade --dry-run'` → plan `web (nginx:latest)`, every `Would run:` line carries `-f <T>/o/docker-compose.yml` only |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | reproduced: the same argv (`-f FILE --project-directory DIR`) for `config`, `pull`, `stop`, `up -d --remove-orphans` and `ps`. |
| Fix recipe (3.0) | `internal/docker` compose argv builder: no `-f` for an auto-discovered file; every compose call gets `--project-directory DIR` and runs with its working directory set to DIR (the caller's cwd is untouched), so Compose applies its own discovery (compose.yaml precedence, override files, `COMPOSE_FILE` from the environment and DIR/.env); same for `config --format json`. The `This will:` list and docs/reference/docker.md say that override files are honored. |
| Proving test | `TestLedger_DOCK04` in `internal/docker` (runner fake: no `-f` in any compose argv, cwd = DIR) and `testdata/script/dock04_override.txtar` (a `docker` stub that records argv and serves `config --format json` with the merged `nginx:mainline`: plan shows `nginx:mainline`) |
| Status | accept: done 09ca07f |

<a id="dock-05"></a>
#### DOCK-05 · dcup2 on a compose file without image services runs compose with an empty service argument

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N2) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:902-919` |
| Observed behavior | A file whose services only have `build:` parses to one empty row (`<<<""`): plan row `OK:  ()`, `--dry-run` shows `pull `, `stop `, `up -d --remove-orphans ` with an empty argument, and `--yes` runs them with `""` (Compose treats an empty service filter like none, so every service is likely pulled, stopped and recreated, inferred). docker_compose_check refuses the same file with `no_services`. |
| Reproduction | `mkdir $T/b && printf 'services:\n  x:\n    build: .\n' >$T/b/compose.yml && gm 'cd $T/b; docker_compose_upgrade --dry-run'` → `  OK:  ()` and `  docker compose -f <T>/b/compose.yml --project-directory <T>/b pull ` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced: the empty row and the empty argv word are kept. |
| Fix recipe (3.0) | `internal/docker` upgrade: skip rows with an empty service name; no service with an image → `Error: No services with images found in <compose file>` rc 1 (docker_compose_check's text) before the plan and before any compose call. |
| Proving test | `TestLedger_DOCK05` in `internal/docker` (build-only fixture: rc 1, that stderr line, the runner fake records no compose call) |
| Status | accept: done 09ca07f |

<a id="dock-06"></a>
#### DOCK-06 · `image:tag@sha256:...` loses its digest pin

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N3) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:168-181` |
| Observed behavior | The digest is stripped first, then `:tag` is tested on the stripped value, so `nginx:1.25@sha256:<64 hex>` normalizes to tag `1.25`: docker_compose_check reports `UPDATE` with `"upgradeable":true` after querying the registry, and dcup2 treats the service as upgradeable. |
| Reproduction | `g '__gash_normalize_image nginx:1.25@sha256:$(printf a%.0s $(seq 64))'` → `docker.io\|library/nginx\|1.25` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced: the normalizer is ported rule for rule over the 2.x net/http protocol. |
| Fix recipe (3.0) | `internal/docker/registry`: parse references with go-containerregistry `name.ParseReference` (enters in 3.0); a `name.Digest` reference → status `PINNED`, `local_digest` and `remote_digest` = that digest, `"upgradeable":false`, no network; dcup2 lists it as pinned. |
| Proving test | `TestLedger_DOCK06` in `internal/docker` (`nginx:1.25@sha256:<64>` → PINNED and the httptest registry counts 0 requests) |
| Status | accept: done 09ca07f |

<a id="dock-07"></a>
#### DOCK-07 · `docker.io/<name>` and `index.docker.io/...` references are queried wrongly

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N4) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:184-197` |
| Observed behavior | `docker.io/alpine` keeps name `alpine` (no `library/`) and the Docker Hub token and manifest requests return `unauthorized` 401 (observed live); `index.docker.io/library/alpine` and `registry-1.docker.io/...` are treated as generic registries (no token) → 401. `alpine` and `docker.io/library/alpine` work. |
| Reproduction | `g '__gash_normalize_image docker.io/alpine; echo; __gash_normalize_image index.docker.io/library/alpine'` → `docker.io\|alpine\|latest` and `index.docker.io\|library/alpine\|latest` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced (same normalization, same requests). |
| Fix recipe (3.0) | `internal/docker/registry`: go-containerregistry `name.ParseReference` maps `docker.io`, `index.docker.io` and `registry-1.docker.io` to Docker Hub and adds `library/` to single-segment names; token scope and manifest URL come from `ref.Context().RepositoryStr()`. |
| Proving test | `TestLedger_DOCK07` in `internal/docker` (normalization vectors above → `docker.io`, `library/alpine`; check against an httptest Docker Hub fake → `CURRENT`) |
| Status | accept: done 09ca07f |

<a id="dock-09"></a>
#### DOCK-09 · docker_compose_scan emits invalid or misleading JSON

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N6) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:1061-1068,1077,1089-1095` |
| Observed behavior | Service names are not escaped (a YAML key `'a"b'` → `"services":["a"b"]`, invalid JSON); a file without image services gives `"services":[""]`; `--depth 05` prints `"depth":05` (invalid JSON for strict parsers; jq 1.7 accepts it); `--depth abc` (or `-1`) makes `find` fail silently → the empty shape `{"base_path":...,"compose_files":[],"total_found":0}` rc 0, which has no `depth` key. |
| Reproduction | `mkdir -p $T/s/a $T/s/b && printf "services:\n  'a\"b':\n    image: nginx:latest\n" >$T/s/a/compose.yml && printf 'services:\n  x:\n    build: .\n' >$T/s/b/compose.yml && g '__gash_compose_cmd() { return 1; }; docker_compose_scan $T/s --depth 05 --json; docker_compose_scan $T/s --depth abc --json'` → `"depth":05`, `["a"b"]`, `[""]`, then the empty shape rc 0 |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced: names written raw between quotes, `[""]` for a file without image services, the depth text emitted verbatim, and any depth `find -maxdepth` rejects (not `^[0-9]+$`) gives the empty shape rc 0. |
| Fix recipe (3.0) | `internal/docker` scan: names through the contract encoder, `[]` when a file has no image service; `--depth` must match `^[0-9]+$` and is printed as an integer (`05` → `5`); otherwise with `--json` `{"error":"invalid_depth","details":"<raw>","action":"RETRY","recoverable":true,"hint":"Use --depth N with N a non-negative integer"}`, without `Error: Invalid --depth: <raw> (use a non-negative integer)`, rc 1. Add `depth` to the empty shape. |
| Proving test | `TestLedger_DOCK09` in `internal/docker` (every output of the vectors parses with encoding/json and validates against `testdata/schema/docker_compose_scan.json`; `--depth abc` → rc 1 `invalid_depth`) |
| Status | accept: done 09ca07f |

<a id="dock-10"></a>
#### DOCK-10 · docker_compose_scan ignores unknown options, also --help after a path

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N7) |
| Area | Contract |
| Bash location | `lib/modules/docker-compose.sh:1045` |
| Observed behavior | `-*) shift` drops every unknown option: `docker_compose_scan --bogus --json` scans normally, `docker_compose_scan . --help` scans instead of printing help, rc 0. docker_compose_check and docker_compose_upgrade refuse unknown options. |
| Reproduction | `g 'docker_compose_scan $T --bogus --json; echo rc=$?'` → `{"base_path":"<T>","compose_files":[],"total_found":0}` `rc=0` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | reproduced. |
| Fix recipe (3.0) | `internal/cli/args` scan grammar: an unknown `-*` word → with `--json` or `-j` anywhere `{"error":"unknown_option","details":"<word>","action":"RETRY","recoverable":true,"hint":"Use a directory, --depth N, --json or --no-color"}`, otherwise `Error: Unknown option: <word>` (docker_compose_check's rule), rc 1; `-h`/`--help` stays help only as `$1`. |
| Proving test | `TestLedger_DOCK10` in `internal/cli/args` (vectors `--bogus`, `. --help`, `-x --json`) |
| Status | accept: done 09ca07f |

<a id="dock-13"></a>
#### DOCK-13 · Image interpolation rules differ from Compose's (default operators, precedence, `$VAR`)

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N9, rule part) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:111-156` (`.env` parsing 118-131) |
| Observed behavior | `${VAR-d}` is treated as `${VAR:-d}` (`EMPTY=` in .env → `d`; Compose: empty); DIR/.env wins over the environment (Compose: the environment wins); `$VAR` without braces is not resolved; .env lines with leading spaces, `export ` or inline comments are skipped or kept verbatim; one leading and one trailing quote are stripped independently. |
| Reproduction | `mkdir $T/f && printf 'EMPTY=\nTAG=env-file\n' >$T/f/.env && g 'export TAG=shell; __gash_resolve_env_vars "redis:\${EMPTY-dflt}/\${TAG}/\$TAG" $T/f/.env'` → `redis:dflt/env-file/$TAG` (Compose: `redis:/shell/shell`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced, where 2.x applies it: on the line-parser path (compose v1 or `config` failure) and once more on the images Compose returned. |
| Fix recipe (3.0) | `internal/docker/interp` with compose-go template semantics (`$VAR`, `${VAR}`, `:-`, `-`, `:?`, `?`, `:+`, `+`, `$$`), process environment over DIR/.env, .env read with Compose's dotenv rules; never re-interpolate images that came from `docker compose config`. |
| Proving test | `TestLedger_DOCK13` in `internal/docker` (vector table compared with `docker compose config --format json` output when compose v2 is installed, skipped otherwise) |
| Status | accept: done 09ca07f |

<a id="dock-14"></a>
#### DOCK-14 · GITHUB_TOKEN is sent raw as the ghcr.io bearer and blocks anonymous access

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N10) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:338-342` |
| Observed behavior | With GITHUB_TOKEN set, the ghcr.io manifest HEAD carries `Authorization: Bearer $GITHUB_TOKEN` with no token exchange and no anonymous fallback: a bogus token turns a public image into `forbidden` (403, observed live); a real PAT is normally exchanged at `ghcr.io/token`, so it likely fails the same way (inferred). |
| Reproduction | `g 'GITHUB_TOKEN=bogus; __gash_ghcr_token org/app; echo "$__gash_auth_token"'` → `bogus` (no request is made for the token) |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | reproduced: the raw value is the bearer of the net/http HEAD. |
| Fix recipe (3.0) | `internal/docker/registry` with go-containerregistry: exchange GITHUB_TOKEN at `https://ghcr.io/token?service=ghcr.io&scope=repository:NAME:pull` with basic auth (`authn.FromConfig(authn.AuthConfig{Username: "gash", Password: token})`), use the returned token; on 401 or 403 retry with the anonymous token; the `ghcr.io` hints stay. docs/reference/docker.md: GITHUB_TOKEN is exchanged, never sent as is. |
| Proving test | `TestLedger_DOCK14` in `internal/docker` (httptest ghcr fake that rejects raw PATs and accepts exchanged tokens → `CURRENT`; a rejected token falls back to anonymous) |
| Status | accept: done 09ca07f |

<a id="dock-15"></a>
#### DOCK-15 · Other registries: no auth challenge, https only, no credential store

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N11) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:421-427` |
| Observed behavior | Registries other than docker.io and ghcr.io get one unauthenticated HEAD and no WWW-Authenticate challenge handling: `public.ecr.aws/docker/library/alpine:latest` → `unauthorized` (observed live), quay.io and similar would fail the same way; requests are always https, also for `localhost:5000` (plain-HTTP registries → `ssl_error` or `connection_refused`, inferred); `~/.docker/config.json` credentials are never used. |
| Reproduction | `g '__gash_get_remote_digest public.ecr.aws docker/library/alpine latest; echo "$__gash_registry_curl_error"'` → `unauthorized` (one read-only HEAD to a public registry) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | reproduced: the 2.x protocol over net/http (no challenge, https only, no Docker credential store). |
| Fix recipe (3.0) | `internal/docker/registry`: go-containerregistry `remote.Head(ref, remote.WithAuth(authn.Anonymous), remote.WithTransport(t))`, which answers the bearer challenge anonymously, and its plain-HTTP fallback for localhost and loopback registries. Reading `~/.docker/config.json` (`authn.DefaultKeychain`) stays out unless the operator decides otherwise. Timeouts, HEAD-only and the error taxonomy (DOCK-24) unchanged. |
| Proving test | `TestLedger_DOCK15` in `internal/docker` (httptest registry that answers 401 with a `WWW-Authenticate: Bearer realm=...` challenge → `CURRENT`) |
| Status | accept: done 09ca07f |

<a id="dock-16"></a>
#### DOCK-16 · Local digests come from `docker inspect` without `--type image`

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N12, inferred) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:454` |
| Observed behavior | `docker inspect --format '{{range .RepoDigests}}...' REF` looks containers up before images, so a container named like the image (`container_name: redis`, `image: redis`) is inspected, has no RepoDigests, and the service is reported `NOT_PULLED` (inferred from the docker CLI's lookup order). |
| Reproduction | `fake docker && g '__gash_get_local_digest redis'; cat $T/argv` → `docker inspect --format {{range .RepoDigests}}{{.}}{{"\n"}}{{end}} redis` (no `--type image`) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | reproduced: the same docker CLI argv. |
| Fix recipe (3.0) | `internal/docker` localDigest: `docker image inspect --format '{{range .RepoDigests}}{{.}}{{"\n"}}{{end}}' -- REF`, same repo-preference rule. |
| Proving test | `TestLedger_DOCK16` in `internal/docker` (runner fake asserts the argv starts with `image inspect` and has `--` before the reference) |
| Status | accept: done 09ca07f |

<a id="dock-18"></a>
#### DOCK-18 · docker_compose_upgrade returns 0 when the upgrade is declined

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N14, exit-status part) |
| Area | Contract |
| Bash location | `lib/modules/docker-compose.sh:954-956`; `docs/security.md:236` ("Answering no returns 1") |
| Observed behavior | Interactive decline, or no terminal outside headless mode: `Info: Upgrade cancelled.` rc 0, while docker_prune_all and git_repair return 1 in the same case and docs/security.md promises 1. |
| Reproduction | `printf 'services:\n  w:\n    image: nginx:latest\n' >$T/compose.yml && gm 'cd $T; unset GASH_HEADLESS; docker_compose_upgrade </dev/null; echo rc=$?'` → `Info: Upgrade cancelled.` `rc=0` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | reproduced (rc 0 after the cancellation line). |
| Fix recipe (3.0) | `internal/docker` upgrade: return 1 after `Info: Upgrade cancelled.`; docs/security.md already says 1. |
| Proving test | `TestLedger_DOCK18` in `internal/docker` (answer `n` on a PTY → rc 1, no `pull` recorded by the runner fake) |
| Status | accept: done 09ca07f |

<a id="dock-20"></a>
#### DOCK-20 · docker_stop_all and docker_start_all hide daemon errors and ignore arguments; prune counts read 0 on a failing daemon

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N15, error part) |
| Area | Correctness |
| Bash location | `lib/modules/docker.sh:24-32,42-50,77-81` |
| Observed behavior | `docker ps ... 2>/dev/null` failing (daemon down, no permission on the socket) reads as "no containers": `Info: No running containers to stop.` rc 0 (start: `No stopped containers to start.`); `docker_prune_all --dry-run` prints zero counts, rc 0; extra arguments of docker_stop_all and docker_start_all are ignored (`docker_stop_all --bogus` rc 0). |
| Reproduction | `mkdir -p $T/bin && printf '#!/bin/sh\necho "Cannot connect to the Docker daemon" >&2; exit 1\n' >$T/bin/docker && chmod +x $T/bin/docker && g 'docker_stop_all --bogus; echo rc=$?; docker_prune_all --dry-run; echo rc=$?'` → `Info: No running containers to stop.` `rc=0`, `Info: To remove: 0 container(s), 0 image(s), 0 volume(s), 0 network(s), plus the build cache.` `rc=0` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced (exit status of the listing commands ignored, extra arguments ignored). |
| Fix recipe (3.0) | `internal/docker`: check the exit status of every listing call (`docker ps`, `docker images`, `docker volume ls`, `docker network ls`); on failure `Error: Cannot list Docker objects: <first stderr line of docker>` rc 1 before any action; unknown arguments of docker_stop_all and docker_start_all → `Error: Unknown option: X (usage: docker_stop_all)` (or `docker_start_all`) rc 1. |
| Proving test | `TestLedger_DOCK20` in `internal/docker` (failing docker stub → rc 1, stderr carries the daemon line, no `stop`/`start` call recorded) |
| Status | accept: done 09ca07f |

<a id="dock-21"></a>
#### DOCK-21 · docker_prune_all refuses headless without saying that -y is needed

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N15, headless part) |
| Area | Contract |
| Bash location | `lib/modules/docker.sh:84-89`; `lib/core/utils.sh:68-80` |
| Observed behavior | Headless without `-y`: the counts line, `Warning: This cannot be undone: volume data is lost.`, then `Info: Aborted.` rc 1. No question is shown and nothing says `-y` is required (git_repair says `Error: No terminal to ask for confirmation: pass -y to go ahead.`). |
| Reproduction | `fake docker && g 'docker_prune_all; echo rc=$?'` → `Info: To remove: 0 container(s), ...`, `Warning: This cannot be undone: volume data is lost.`, `Info: Aborted.`, `rc=1` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | reproduced. |
| Fix recipe (3.0) | `internal/docker` prune: headless or no terminal → `Error: No terminal to ask for confirmation: pass -y to go ahead.` rc 1 (the `__gash_confirm_plan` text) instead of the silent refusal; an interactive "no" keeps `Info: Aborted.` rc 1. |
| Proving test | `TestLedger_DOCK21` in `internal/docker` (headless, no `-y` → that stderr line, rc 1, no removal call) |
| Status | accept: done 09ca07f, 9eed0f7 |

<a id="dock-28"></a>
#### DOCK-28 · The YAML-subset reader cuts ` #...` inside quoted image values

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N31, comment part) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:78` |
| Observed behavior | The comment strip runs before the unquote, so `image: "nginx:latest #x"` → `"nginx:latest` (the quotes no longer pair and the leading quote stays). |
| Reproduction | `printf 'services:\n  w:\n    image: "nginx:latest #x"\n' >$T/c.yml && g '__gash_compose_cmd() { return 1; }; __gash_parse_compose_services $T/c.yml'` → `w\|"nginx:latest` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | reproduced in the line parser, which runs only with compose v1 or when `config` fails (DOCK-26). |
| Fix recipe (3.0) | `internal/docker` line parser: a value that starts with a quote ends at the matching quote; ` #...` is stripped only outside quotes. |
| Proving test | `TestLedger_DOCK28` in `internal/docker` (`"nginx:latest #x"` → `nginx:latest #x`; `nginx:latest #x` → `nginx:latest`) |
| Status | accept: done 09ca07f |

<a id="dock-29"></a>
#### DOCK-29 · Compose file precedence is the reverse of Compose's

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N31, precedence part) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:550,865` |
| Observed behavior | Gash takes the first of `docker-compose.yml`, `docker-compose.yaml`, `compose.yml`, `compose.yaml`; Compose prefers `compose.yaml` and `compose.yml` over the legacy names (and warns when several exist), so with two files Gash checks and upgrades a different file than `docker compose up` uses. |
| Reproduction | `mkdir $T/two && printf 'services:\n  old:\n    image: nginx:latest\n' >$T/two/docker-compose.yml && printf 'services:\n  new:\n    image: nginx:latest\n' >$T/two/compose.yaml && gm 'cd $T/two; docker_compose_upgrade --dry-run'` → `OK: old (nginx:latest)` and `-f <T>/two/docker-compose.yml ... pull old` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | reproduced (same order, same `compose_file` in JSON and headings). |
| Fix recipe (3.0) | with DOCK-04's fix Compose discovers the file itself; for `compose_file` and the headings use Compose's order (compose-go `DefaultFileNames`: `compose.yaml` and `compose.yml` before the docker-compose names). |
| Proving test | `TestLedger_DOCK29` in `internal/docker` (both files present → `compose_file` is `compose.yaml`; no `-f docker-compose.yml` in any argv) |
| Status | accept: done 09ca07f |

<a id="dock-30"></a>
#### DOCK-30 · docker_compose_check table: contradictory width limits misalign rows

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N32) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:699` |
| Observed behavior | Not reproduced. The entry expected `--min 2:40 --max 2:38` to cut long images to 38 while padding short ones to 40, misaligning STATUS. In fact `__gash_table` pads the cut cell back out to the `--min` width (40), so every STATUS cell stays in the same column. Verified on 2.x (`security/2.1.1`): with a 64-char `redis@sha256:…` image and a short `nginx:latest`, STATUS starts at the same byte column on both rows. |
| Reproduction | `printf 'services:\n  p:\n    image: redis@sha256:%s\n  w:\n    image: nginx:latest\n' "$(printf 1%.0s $(seq 64))" >$T/compose.yml && gm 'cd $T; docker_compose_check'` → STATUS aligned on both rows (no misalignment) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Matches 2.x: the `internal/ui` table pads the truncated cell to `--min`, so STATUS stays aligned, same as the Bash table. |
| Fix recipe (3.0) | none; no defect. (The call site may still tidy `--min 2:40 --max 2:38` to `--min 2:38 --max 2:38`, cosmetic only.) |
| Proving test | `TestLedger_DOCK30` in `internal/docker` (golden: every STATUS cell starts at the same column), which passes — confirming alignment, not a defect. |
| Status | closed: not reproduced |

<a id="dock-31"></a>
#### DOCK-31 · Option-like image references and service names reach the docker CLI without `--`

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (dossier summary of unquoted input, item 8) |
| Area | Security |
| Bash location | `lib/modules/docker-compose.sh:454,962-973` |
| Observed behavior | `docker inspect --format FMT REF` and `<compose> pull\|stop\|up -d --remove-orphans S...` put words from the compose file in option position: a service named `--help` (accepted by the line parser; Compose v2 validates service names) becomes `pull --help` (observed in `--dry-run`); an image reference starting with `-` reaches `docker inspect` as an option (inferred). |
| Reproduction | `mkdir $T/q && printf 'services:\n  --help:\n    image: nginx:latest\n' >$T/q/compose.yml && fake docker && g '__gash_compose_cmd() { echo docker-compose; }; cd $T/q; docker_compose_upgrade --dry-run'` → `docker-compose -f <T>/q/compose.yml --project-directory <T>/q pull --help` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | reproduced (same argv). |
| Fix recipe (3.0) | `internal/docker`: `docker image inspect --format FMT -- REF` (DOCK-16) and `<compose> ... pull -- S...`, `stop -- S...`, `up -d --remove-orphans -- S...`; the line parser refuses service names starting with `-` (`Error: Invalid service name '<name>' in <compose file>` rc 1). |
| Proving test | `TestLedger_DOCK31` in `internal/docker` (runner fake: `--` precedes every word taken from the compose file) |
| Status | accept: done 09ca07f |

### git module

<a id="git-01"></a>
#### GIT-01 · Tag push, tag deletion, git_sync and git_apply_patch act without confirmation or a headless --yes

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "Git module remote tag delete, tag push and git_sync run without confirmation or a headless --yes" (CONFIRMED) |
| Area | Contract |
| Bash location | `lib/modules/git.sh:107,136,190-200,226-240,451-470`; `docs/security.md:239-252` (Confirmations table omits them) |
| Observed behavior | Headless, `git_add_tag v1 m` creates and pushes the tag, `git_delete_tag v1` deletes it locally and on origin, `git_sync` stages untracked files (an `.env.local` too), commits and pushes, all rc 0; `git_sync --yes` → `Error: Unknown option: --yes ...` rc 1; `git_delete_tag v1 --yes` ignores `--yes`; git_apply_patch checks out, pulls from origin and may `reset --hard` without asking. |
| Reproduction | `g 'cd $T && git init -q --bare o.git && git init -q w && cd w && git config user.email a@b && git config user.name n && git remote add origin ../o.git && echo a >a && git add a && git commit -qm i && git_add_tag v1 m && git_delete_tag v1; echo s >.env.local; git_sync; git ls-tree -r --name-only HEAD'` → `OK: Tag 'v1' pushed to remote.`, `OK: Remote tag 'v1' deleted.`, `OK: Pushed master to origin/master (upstream set).`, tree lists `.env.local` |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | reproduced: no question, no `--yes`. |
| Fix recipe (3.0) | As docker_compose_upgrade: `-y/--yes` (any position) and `--dry-run` on git_add_tag (gates the tag creation and the push), git_delete_tag (gates both deletions), git_sync (gates commit and push; with `--no-push` gates the commit) and git_apply_patch (gates checkout, pull and apply). Without `--yes`: on a TTY print the plan (`Will push refs/tags/T to origin (<url>)`, `Will delete refs/tags/T locally and on origin (<url>)`, `Will commit N file(s) on B (M untracked) and push to U`, `Will check out MAIN, pull origin MAIN and apply FEATURE's changes since COMMIT`) and `needs_confirm_prompt "Proceed?"`, decline → `Info: Cancelled.` rc 1; headless or no TTY → `Error: Refusing to <push tag 'T'\|delete tag 'T'\|commit and push\|apply the patch> in headless mode without --yes.` rc 1. git_add_tag positionals: first non-flag word TAG, second MESSAGE, `--` ends flags. docs/security.md Confirmations rows, docs/reference/git.md usage, completion `-y --yes --dry-run`. |
| Proving test | `TestLedger_GIT01` in `internal/gitx` and `testdata/script/git01_confirm.txtar`: headless `git_delete_tag v1` → rc 1 and `git ls-remote --tags origin refs/tags/v1` still lists it; `git_delete_tag v1 --yes` → gone; headless `git_sync` on a dirty tree → rc 1, HEAD and `git status --porcelain` unchanged; `git_sync --yes` pushes; headless `git_add_tag v2` → rc 1, no local tag |
| Status | accept: done 9eed0f7 |

<a id="git-02"></a>
#### GIT-02 · git_list_tags shows peeled tags and returns 0 when ls-remote fails

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N18) |
| Area | Correctness |
| Bash location | `lib/modules/git.sh:162-165` |
| Observed behavior | An annotated tag is listed twice under "Remote tags:" (`v1.0` and `v1.0^{}`); an unreachable origin prints git's fatal error and the function still returns 0. |
| Reproduction | `g 'cd $T && git init -q --bare o.git && git init -q w && cd w && git config user.email a@b && git config user.name n && git remote add origin ../o.git && echo a >a && git add a && git commit -qm i && git tag -a v1.0 -m x && git push -q origin v1.0 && git_list_tags; git remote set-url origin $T/none.git; git_list_tags; echo rc=$?'` → `v1.0` and `v1.0^{}` under "Remote tags:", then `fatal: ... does not appear to be a git repository` and `rc=0` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | reproduced (`git ls-remote --tags origin`, field 2, first `refs/tags/` removed, rc 0). |
| Fix recipe (3.0) | `internal/gitx` list tags: `git ls-remote --tags --refs origin` (no peeled lines); a non-zero ls-remote status → git's stderr, then `Error: Cannot list the tags of origin.` rc 1. |
| Proving test | `TestLedger_GIT02` in `internal/gitx` (annotated tag listed once; unreachable origin → rc 1) |
| Status | accept: done 9eed0f7 |

<a id="git-03"></a>
#### GIT-03 · An unreachable origin counts as "tag absent"

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N19) |
| Area | Correctness |
| Bash location | `lib/modules/git.sh:254-256` |
| Observed behavior | `git ls-remote --exit-code` returns 2 for "no such ref" and 128 for a failure; both read as "absent": git_add_tag creates the local tag and then reports `Error: Tag 'v9' was created locally but could not be pushed.` rc 1; git_delete_tag deletes only the local tag, `OK: Local tag 'v9' deleted.` rc 0, no warning. |
| Reproduction | `g 'cd $T && git init -q w3 && cd w3 && git config user.email a@b && git config user.name n && git remote add origin $T/none.git && echo a >a && git add a && git commit -qm i && git tag v9 && git_delete_tag v9; echo rc=$?'` → `OK: Local tag 'v9' deleted.` `rc=0` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced. |
| Fix recipe (3.0) | `internal/gitx` RemoteTagExists: ls-remote status 0 → present, 2 → absent, anything else → `Error: Cannot reach origin (<url>): nothing was created or deleted.` rc 1, checked before any local change in git_add_tag and git_delete_tag. |
| Proving test | `TestLedger_GIT03` in `internal/gitx` (unreachable origin: rc 1 and the local tag still exists; for add, no local tag created) |
| Status | accept: done 9eed0f7 |

<a id="git-04"></a>
#### GIT-04 · Option-like tag names reach `git tag`

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N20) |
| Area | Security |
| Bash location | `lib/modules/git.sh:181,190,227,246` |
| Observed behavior | Names starting with `-` pass `git check-ref-format refs/tags/<name>` and reach `git tag -a <name> -m MSG` or `git tag -d <name>` in option position: `git_add_tag -s`, `-f` or `--list` print git's usage and `Error: Could not create tag '-s'.` (observed); no damage found, but the name is parsed as an option. |
| Reproduction | `g 'cd $T && git init -q w4 && cd w4 && git config user.email a@b && git config user.name n && echo a >a && git add a && git commit -qm i && git remote add origin $T/none.git && git_add_tag -s; echo rc=$?'` → git usage text, `Error: Could not create tag '-s'.` `rc=1` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | reproduced (same argv). |
| Fix recipe (3.0) | `internal/gitx` checkTagName: also refuse a leading `-` with the existing `Error: Invalid tag name: '<name>'.` rc 1, for add and delete. |
| Proving test | `TestLedger_GIT04` in `internal/gitx` (`-s`, `-f`, `--list` → `Invalid tag name`, runner fake records no `git tag`) |
| Status | accept: done 9eed0f7 |

<a id="git-05"></a>
#### GIT-05 · git_dump_revisions: file names parsed as revisions, absolute paths give empty files

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N21) |
| Area | Correctness |
| Bash location | `lib/modules/git.sh:36,50` |
| Observed behavior | `git log --pretty=format:%h "$file"` has no `--`: a file named like a branch (`master`) → `fatal: ambiguous argument 'master': both revision and filename`, nothing written, rc 0; an absolute path → `git show HASH:./<abs>` fails for every commit and empty `FILE.NNN.HASH` files are written, rc 0; git failures never change the exit status. |
| Reproduction | `g 'cd $T && git init -q -b master w5 && cd w5 && git config user.email a@b && git config user.name n && echo a >master && git add master && git commit -qm i && git_dump_revisions master; echo rc=$?; ls'` → the ambiguous-argument fatal, `rc=0`, only `master` in the listing |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced. |
| Fix recipe (3.0) | `internal/gitx` dumpRevisions: `git log --pretty=format:%h -- FILE`; FILE converted to its work-tree path with `git ls-files --full-name --error-unmatch -- FILE` and read as `git show HASH:<that path>`; any git failure → `Error: git failed on '<file>'.` rc 1; each output written to a temporary file and renamed, so no empty files are left. |
| Proving test | `TestLedger_GIT05` in `internal/gitx` (file `master` → files written; absolute path → same files as the relative path; git failure → rc 1) |
| Status | accept: done 9eed0f7 |

<a id="git-06"></a>
#### GIT-06 · git_apply_patch: slash branch names break the patch file, failures leave another branch checked out

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N22) |
| Area | Correctness |
| Bash location | `lib/modules/git.sh:107-131` |
| Observed behavior | The patch goes to `${FEATURE}.patch` in the current directory: `feat/x` → the redirection fails, `Error: Failed to create patch file.` rc 1, and the repository is left on `feat/x`; every failure after step 1 leaves FEATURE or MAIN checked out instead of the starting branch; step 1 pulls from origin (consent in GIT-01). The Bash redirection diagnostic is DOCK-19. |
| Reproduction | `g 'cd $T && git init -q -b master w6 && cd w6 && git config user.email a@b && git config user.name n && echo a >a && git add a && git commit -qm i && git branch feat/x && git remote add origin $T/w6 && base=$(git rev-parse HEAD) && git checkout -q -b other && git_apply_patch master feat/x $base; echo rc=$?; git branch --show-current'` → `Error: Failed to create patch file.` `rc=1` `feat/x` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced (patch path, step order and exit status; the Bash diagnostic line is not printed, DOCK-19). |
| Fix recipe (3.0) | `internal/gitx` applyPatch: write the patch with `os.CreateTemp("", "gash-patch-*.patch")` (named in `Info: Patch file kept for inspection: <path>`), record `git symbolic-ref --short HEAD` at entry and check it out again on every failure path before returning 1. |
| Proving test | `TestLedger_GIT06` in `internal/gitx` (feature `feat/x` → patch applied; a failing apply → starting branch checked out again, patch kept in the temp dir) |
| Status | accept: done 9eed0f7 |

<a id="git-07"></a>
#### GIT-07 · git_sync --dry-run promises a push that git push refuses

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N23) |
| Area | Correctness |
| Bash location | `lib/modules/git.sh:443,465` |
| Observed behavior | A branch `work` tracking `origin/master`: `--dry-run` says `Would push work to origin/master.`; the real run commits (`OK: Committed <hash> on work: x`) and then `git push --quiet` fails under `push.default=simple` (`fatal: The upstream branch of your current branch does not match the name of your current branch`), rc 1 with the commit already made. |
| Reproduction | `g 'cd $T && git init -q --bare o7.git && git init -q -b master w7 && cd w7 && git config user.email a@b && git config user.name n && git remote add origin ../o7.git && echo a >a && git add a && git commit -qm i && git push -q -u origin master && git checkout -q -b work --track origin/master && echo b >b && git_sync --dry-run && git_sync -m x; echo rc=$?'` → `Would push work to origin/master.`, `OK: Committed ... on work: x`, the upstream fatal, `rc=1` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced (plain `git push --quiet` exec'd as in 2.x). |
| Fix recipe (3.0) | `internal/gitx` sync: push explicitly to the upstream, `git push --quiet <branch.B.remote> HEAD:<branch.B.merge without refs/heads/>`, and print the same target in `--dry-run`; when no upstream, keep `push -u origin B`. |
| Proving test | `TestLedger_GIT07` in `internal/gitx` (fixture above: rc 0 and `origin/master` advanced; dry-run text names the same target) |
| Status | accept: done 9eed0f7 |

<a id="git-08"></a>
#### GIT-08 · git_repair: origin URL parsed as a clone option; empty-file deletion too broad and before the clone

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N24) |
| Area | Security |
| Bash location | `lib/modules/git.sh:296,321,324` |
| Observed behavior | `git clone --quiet --bare "$origin_url" TMP/recovery.git` has no `--`: an origin URL starting with `-` (`--upload-pack=...`) is parsed as an option and the temporary path becomes the source (`fatal: repository '/tmp/tmp.X/recovery.git' does not exist`, observed; no code ran in this shape). The empty files are deleted in step 1, before the clone has succeeded, so a failed clone leaves the repository with fewer objects; `find OBJECTS -type f -empty` also matches empty files that are not loose objects (pack `.keep`, `info/` files, inferred). |
| Reproduction | `g 'cd $T && git init -q w8 && cd w8 && git config remote.origin.url "--upload-pack=touch $T/marker;git-upload-pack" && mkdir -p .git/objects/aa && : >.git/objects/aa/$(printf b%.0s $(seq 38)) && git_repair -y; echo rc=$?; ls .git/objects/aa'` → `Cloning '--upload-pack=...'`, `fatal: repository '<tmp>/recovery.git' does not exist`, `rc=1`, the empty object already deleted, no marker |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | reproduced (same git argv and step order). |
| Fix recipe (3.0) | `internal/gitx` repair: refuse an origin URL starting with `-` (`Error: The origin URL starts with '-': refusing to pass it to git clone.` rc 1, before the plan); `git clone --quiet --bare -- "$origin_url" "$tmp/recovery.git"`; select only `OBJECTS/[0-9a-f]{2}/[0-9a-f]{38}` (or 62 for SHA-256) empty files; order clone, then delete, then copy. |
| Proving test | `TestLedger_GIT08` in `internal/gitx` (URL `--upload-pack=...` → rc 1, empty object still present; an empty `pack/x.keep` is never deleted; a failed clone deletes nothing) |
| Status | accept: done 9eed0f7 |

### remote

<a id="rem-01"></a>
#### REM-01 · Unknown flags and extra positionals are silently ignored, also with --delete

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "Remote functions: unknown flags silently ignored, including on --delete paths" (CONFIRMED) |
| Area | Contract |
| Bash location | `lib/modules/remote.sh:428-435,466-473,505-513,549-557` |
| Observed behavior | The four flag loops have no `*)` branch: `remote_sync_push box ./src /tmp/build --delete -n` and `... --delete --exclude .env` run a real `rsync ... --delete` (no dry run, no exclude, so a remote `.env` is deleted); `--delte` and `--forse` run as plain syncs; `remote_push ... --dry-run` copies for real; `remote_sync_pull ... --delete --dryrun` deletes locally; a fourth positional is ignored. Every call rc 0. |
| Reproduction | `fake ssh scp rsync && printf 'HOST:box=deploy@box.example\n' >$T/.gash_env && chmod 600 $T/.gash_env && g 'remote_sync_push box ./src /tmp/build --delete --exclude .env; remote_push box ./x /tmp/x --dry-run; echo rc=$?'; cat $T/argv` → `rsync ... --delete -- ./src deploy@box.example:/tmp/build` (no exclude), `scp -o ConnectTimeout=10 -o BatchMode=yes -- ./x deploy@box.example:/tmp/x`, `rc=0` |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | reproduced: the 2.x remote grammar ignores unknown words. |
| Fix recipe (3.0) | `internal/cli/args` remote grammars (push, pull, sync_push, sync_pull): an unknown `-*` word → `Error: Unknown option: X (usage: <the function's usage line>)` rc 1, before host resolution; a fourth positional → `Error: Unexpected argument: X (usage: ...)` rc 1; `--exclude PAT` (two words) accepted as well as `--exclude=PAT`; `-n` refused like any unknown option. docs/reference/remote.md and the registered help: "`--exclude PAT` or `--exclude=PAT`". |
| Proving test | `TestLedger_REM01` in `internal/remote` and `testdata/script/rem01_flags.txtar` (logging rsync and scp stubs: `--delete -n` → rc 1, `$T/argv` empty, stderr `Unknown option: -n`; `--exclude .env` → argv has `--exclude=.env`; `remote_push box a /tmp/x --dry-run` → rc 1, scp not called) |
| Status | accept: done 4fc4e85 |

<a id="rem-02"></a>
#### REM-02 · A remote target can be an ssh option (no `--` before the target)

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N25) |
| Area | Security |
| Bash location | `lib/modules/remote.sh:82,388,390,397`; `lib/core/config.sh:267` |
| Observed behavior | The target regex `^[^@[:space:]]+@[^@[:space:]]+$` accepts `-oProxyCommand=...@h` as a `HOST:` entry (listed by remote_hosts) and as an interactive raw target; remote_exec passes it to ssh in option position, argv `ssh -o ConnectTimeout=10 ... -o ControlPersist=60s -oProxyCommand=x@h 'unset HISTFILE HISTSIZE; true'` (observed with a logging ssh), which a real ssh parses as `-o ProxyCommand=...` and runs locally (inferred; recent OpenSSH may then reject the next word as a hostname). The 255 probe repeats it. scp and rsync already put `--` before their operands. |
| Reproduction | `fake ssh && printf 'HOST:opt=-oProxyCommand=x@h\n' >$T/.gash_env && chmod 600 $T/.gash_env && g 'remote_exec opt true'; cat $T/argv` → `ssh ... -o ControlPersist=60s -oProxyCommand=x@h unset HISTFILE HISTSIZE; true` (a working command shape is kept outside the repository) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | reproduced (same regex, same ssh argv). |
| Fix recipe (3.0) | `internal/remote` target validation, shared by the `HOST:` parser (`internal/config`) and raw targets: user `^[A-Za-z0-9._][A-Za-z0-9._-]*$`, host `^[A-Za-z0-9][A-Za-z0-9.-]*$` or a bracketed IPv6 literal, so neither part starts with `-` and the host has no `:`; a failing `HOST:` entry gets the existing warning `Line N of ~/.gash_env: HOST target must be user@host (skipping)`; a failing raw target gets `Error: Unknown host alias 'X' and not a valid user@host target.` rc 1. Every ssh argv puts `--` before the target: `ssh OPTS -- TARGET CMD`, the 255 probe too. docs/security.md remote_* section. |
| Proving test | `TestLedger_REM02` in `internal/remote` (validation table: `-oProxyCommand=x@h`, `u@-h`, `-u@h`, `u@h:22` refused; `deploy@box.example`, `u@[::1]` accepted) and `testdata/script/rem02_target.txtar` (logging ssh: `--` immediately before the target in both ssh calls) |
| Status | accept: done cdbab90 |

<a id="rem-03"></a>
#### REM-03 · Path tiers are checked on strings the remote side may expand

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N27) |
| Area | Security |
| Bash location | `lib/modules/remote.sh:141-192,227-250,447,485,529,576` |
| Observed behavior | Classification and the `--delete` safe set compare the literal string: `/e?c/shadow` and `/e[t]c/shadow` → `allow` (a DENY bypass once expanded); `$HOME`, `${HOME}`, `*`, `./*`, `$(echo ~)` and a backquoted `pwd` → `allow` and delete-safe; `remote_sync_push box ./src '$HOME' --delete` runs headless with argv `deploy@box.example:$HOME`. The remote side expands such paths with rsync clients < 3.2.4 or `--old-args`/RSYNC_OLD_ARGS, legacy scp (`-O`, OpenSSH < 9.0), and SFTP-mode scp globs remote sources (inferred from the tools' documentation). |
| Reproduction | `fake ssh scp rsync && printf 'HOST:box=deploy@box.example\n' >$T/.gash_env && chmod 600 $T/.gash_env && g 'remote_sync_push box ./src "\$HOME" --delete; echo rc=$?; __remote_classify_path "/e?c/shadow"'; cat $T/argv` → `rc=0`, `allow`, `rsync ... --delete -- ./src deploy@box.example:$HOME` |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | reproduced (literal classification, same argv). |
| Fix recipe (3.0) | `internal/remote` remote-path guard, before normalization, for push DST, pull SRC and both sync paths: refuse any remote path containing a glob character (`*`, `?`, `[`, `]`, `{`, `}`), `$`, a backquote, `;`, `&`, `\|`, `<`, `>`, `(`, `)`, `!`, a quote, a backslash or a newline, or a `~` anywhere but a leading `~`, `~/` or `~NAME[/...]`: classification `deny:metacharacters`, message `Error: Remote path 'P' contains characters the remote side may expand. Refusing OP.` rc 1, not overridable by `--force`. rsync always gets `-s` (`--protect-args`). docs/security.md tier table gains the rule. |
| Proving test | `TestLedger_REM03` in `internal/remote` (every vector above → `deny:metacharacters`) and `testdata/script/rem03_expand.txtar` (`remote_sync_push box ./src '$HOME' --delete` and the same with `--force` → rc 1, rsync not called) |
| Status | accept: done 4fc4e85 |

<a id="rem-05"></a>
#### REM-05 · The rsync -e string breaks when $HOME contains a space

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N28, inferred) |
| Area | Portability |
| Bash location | `lib/modules/remote.sh:521,568` |
| Observed behavior | `-o ControlPath=${HOME}/.ssh/cm-%C` is embedded unquoted in the single `-e` string, which rsync splits on spaces: with `HOME="/home/John Doe"` ssh receives `ControlPath=/home/John` and a stray word `Doe/.ssh/cm-%C` taken as the host, and every sync fails (the split is rsync's, inferred from its documentation). |
| Reproduction | `fake rsync && mkdir -p "$T/John Doe" && printf 'HOST:box=deploy@box.example\n' >"$T/John Doe/.gash_env" && chmod 600 "$T/John Doe/.gash_env" && env -i HOME="$T/John Doe" PATH="$T/bin:/usr/bin:/bin" bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; remote_sync_push box ./s /tmp/d'; cat $T/argv` → `-e` value `... -o ControlPath=<T>/John Doe/.ssh/cm-%C ...` unquoted |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | reproduced (same `-e` string). |
| Fix recipe (3.0) | `internal/remote` rsync: build the `-e` string with rsync's quoting, `-o 'ControlPath=<HOME>/.ssh/cm-%C'` (a `'` inside HOME written as `'\''`). |
| Proving test | `TestLedger_REM05` in `internal/remote` (the `-e` string split by rsync's rules, implemented in the test, yields exactly one ControlPath word for a HOME with spaces and quotes) |
| Status | accept: done 4fc4e85 |

<a id="rem-06"></a>
#### REM-06 · remote_sync_pull --delete checks the local destination with remote wording

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N29) |
| Area | Contract |
| Bash location | `lib/modules/remote.sh:562-565` (messages from `lib/modules/remote.sh:254-269`) |
| Observed behavior | The `--delete` check of remote_sync_pull runs on the LOCAL destination but prints the remote messages: interactive `rsync --delete targeting './' can remove files on the remote. Proceed? [y/N]`; headless `Error: rsync --delete targeting './' is outside the safe set (below /tmp, /var/tmp, /home/<user>/ or the remote home; never the home itself or .ssh). Refused in headless mode without --force.` |
| Reproduction | `fake rsync && printf 'HOST:box=deploy@box.example\n' >$T/.gash_env && chmod 600 $T/.gash_env && g 'remote_sync_pull box /srv ./ --delete; echo rc=$?'` → the headless message above naming "the remote home", `rc=1` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | reproduced (same rule, same text). |
| Fix recipe (3.0) | `internal/remote` local delete check, same safe-set rules, local wording: `Error: rsync --delete targeting the local destination 'P' is outside the safe set (below /tmp, /var/tmp, /home/<user>/ or the current directory; never your home itself or .ssh). Refused in headless mode without --force.` and the prompt `rsync --delete targeting the local destination 'P' can remove local files. Proceed? [y/N]`. |
| Proving test | `TestLedger_REM06` in `internal/remote` (headless pull with `--delete ./` → the local wording, rc 1, rsync not called) |
| Status | accept: done 4fc4e85 |

<a id="rem-07"></a>
#### REM-07 · remote_exec creates ~/.ssh with the default umask

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N30, directory part) |
| Area | Security |
| Bash location | `lib/modules/remote.sh:371` |
| Observed behavior | `mkdir -p "$HOME/.ssh"` creates the directory with the caller's umask: `drwxr-xr-x` (0755) under umask 022 (observed), where OpenSSH expects 0700. |
| Reproduction | `fake ssh && printf 'HOST:box=deploy@box.example\n' >$T/.gash_env && chmod 600 $T/.gash_env && g 'umask 022; remote_exec box true; stat -c %a $T/.ssh'` → `755` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | reproduced (`os.MkdirAll` with 0777 filtered by the umask). |
| Fix recipe (3.0) | `internal/remote` exec: create a missing `~/.ssh` with mode 0700 (`os.Mkdir(dir, 0o700)`); never change the mode of an existing directory. |
| Proving test | `TestLedger_REM07` in `internal/remote` (temp HOME without `.ssh`, umask 022 → mode 0700) |
| Status | accept: done 4fc4e85 |

<a id="rem-08"></a>
#### REM-08 · A HOST target `u@h:22` is accepted; ssh, scp and the tier check read it differently

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N30, target part) |
| Area | Correctness |
| Bash location | `lib/core/config.sh:267`; `lib/modules/remote.sh:82,447,485` |
| Observed behavior | `HOST:x=u@h:22` passes the target regex; ssh sees the host `h:22` (fails), scp sees the host `h` and the remote path `22:/dst`, while the tier check classified `/dst` (tool behavior inferred). |
| Reproduction | `fake scp && printf 'HOST:x=u@h:22\n' >$T/.gash_env && chmod 600 $T/.gash_env && g 'remote_push x ./f /tmp/f'; cat $T/argv` → `scp -o ConnectTimeout=10 -o BatchMode=yes -- ./f u@h:22:/tmp/f` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | reproduced. |
| Fix recipe (3.0) | REM-02's target validation (no `:` outside a bracketed IPv6 literal); the skip warning adds `Hint: put the port in a Host entry of ~/.ssh/config`. |
| Proving test | `TestLedger_REM08` in `internal/remote` (part of REM-02's table: `u@h:22` refused with the warning) |
| Status | accept: done cdbab90 |

<a id="rem-10"></a>
#### REM-10 · A `/.` (or `//.`) root target normalizes to `.` and classifies as allow, not deny:/

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (lead pin, remote `/.` root classification) |
| Area | Security |
| Bash location | `lib/modules/remote.sh:96-123` (`__remote_normalize_path`: the trailing-`/.` strip at `:119` runs before the `/.`->`/` special case at `:120`, so `/.` becomes `""` and `:122` prints `.`); `__remote_classify_path:141-192` then returns `allow` for the relative path, never reaching the `deny:/` branch at `:153-154` |
| Observed behavior | The remote root spelled `/.` (and `//.`, which the `//`-collapse plus the `/.` strip also reduce to `.`) is normalized to `.` — the remote home/cwd — so the safety classifier returns `allow` instead of `deny:/`. The DENY guard on the remote root is bypassed by that spelling: a transfer or exec that means the filesystem root is silently retargeted at the remote home. (rsync `--delete` has its own separate guard at `:235`, which still refuses `.`, so `--delete` is not weakened; the general allow/deny verdict is.) |
| Reproduction | `__remote_normalize_path /.` prints `.` (not `/`), and `__remote_classify_path /.` prints `allow` (not `deny:/`); likewise for `//.`. |
| Severity | medium |
| Public-contract impact | yes (the documented allow/deny classification) |
| rc.A behavior | Reproduced: `internal/remote/safety.go:56-60` trims the trailing `/.` before the `p == "/."` special case, leaving `""`, and `:65-67` returns `.`; `classifyPath` then returns `allow`. The source comment at `:30` records that `/.` (and `//.`) come out as `.`, as in 2.x. |
| Fix recipe (3.0) | `internal/remote` `normalizePath`: fold `/.` (and any absolute path that reduces to the root-dot form) to `/` before the trailing-`/.` strip, so `/.` and `//.` classify as `deny:/`; equivalently, when the trailing-`/.` strip empties an absolute path, set it to `/`. |
| Proving test | `TestLedger_REM10_fix` in `internal/remote` (`/.`, `//.`, `/./` → `deny:/`); `TestLedger_REM10_2x` proves the `allow` reproduction |
| Status | accept: done 4fc4e85 |

### php and node

<a id="dev-01"></a>
#### DEV-01 · php_detect_version reports a PHP version that is not installed

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "detected PHP version not checked as installed" (CONFIRMED) |
| Area | Correctness |
| Bash location | `lib/modules/php.sh:94-102,104-105` |
| Observed behavior | The detected `MAJOR.MINOR` is reported without any installed check: a project pinned to 8.1 on a host with only PHP 8.3 gets `"version":"8.1","php_raw_binary":"/usr/bin/php"` (the 8.3 binary) and `"php_command":"php81"`, an alias the php pack never defines; the 8.4 default is reported the same way, and any `/usr/bin/php7` serves every 7.x. stderr empty, rc 0. |
| Reproduction | `d=$(mktemp -d); printf '8.1\n' >"$d/.target-php-version"; env -i HOME="$d" PATH=/usr/local/bin:/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; php_detect_version "$HOME"'` (host without `/usr/bin/php8.1`) |
| Severity | medium |
| Public-contract impact | yes (a new stderr warning; the JSON on stdout is unchanged) |
| rc.A behavior | reproduced: same probe order (`/usr/bin/php<version>`, `/usr/bin/php<major>`, `php` on PATH, literal `php`), no warning, rc 0. |
| Fix recipe (3.0) | `internal/dev/php.go` `DetectVersion`: when neither `/usr/bin/php<version>` nor `/usr/bin/php<major>` is executable, write `Warning: PHP <version> is not installed; php_raw_binary falls back to <php_raw_binary>` on stderr in both modes (as node_detect_version warns), keep every JSON key and value and rc 0; when only the major probe matched, `Warning: PHP <version> is not installed; using <path>`. Docs: registry help and `docs/reference/dev.md` NOTES of php_detect_version describe the warning. |
| Proving test | `TestLedger_DEV01_fix` in `internal/dev` (sysroot without php8.1: stdout byte-identical to 2.x, stderr has the warning; sysroot with php8.1: stderr empty) and `testdata/script/php_detect_not_installed.txtar` |
| Status | accept: done 37bcf69 |

<a id="dev-02"></a>
#### DEV-02 · OR constraints resolve to their first X.Y (`^7.4\|^8.0` gives 7.4)

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "`^7.4\|^8.0` picks 7.4" (CONFIRMED) |
| Area | Correctness |
| Bash location | `lib/modules/php.sh:45-48` |
| Observed behavior | The first `[0-9]+\.[0-9]+` anywhere in `.require.php` wins: `^7.4\|^8.0` gives `7.4` (an end-of-life version) and `^8.0 \|\| ^7.4` gives `8.0`, so the answer depends on the order of the alternatives. |
| Reproduction | `d=$(mktemp -d); printf '{"require":{"php":"^7.4\|^8.0"}}' >"$d/composer.json"; env -i HOME="$d" PATH=/usr/local/bin:/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; php_detect_version "$HOME"'` → `{"version":"7.4",...,"php_command":"php74",...}` |
| Severity | medium |
| Public-contract impact | yes (the reported version changes for OR constraints) |
| rc.A behavior | reproduced: first X.Y of the constraint text (the text jq 1.7 `-r` prints for the value). |
| Fix recipe (3.0) | `internal/dev/php.go` `composerVersion`: split the constraint on `\|\|` and `\|`; for each alternative take its lower bound (first X.Y; a bare major as in `^7`, `>=8`, `8.*` becomes `X.0`; `*` alone gives none); pick the highest alternative whose `/usr/bin/phpX.Y` is executable, else the highest alternative; no bound at all → next source (copilot-instructions, then 8.4). Operator sign-off needed (existing projects can change version). Docs: php_detect_version NOTES state the rule. |
| Proving test | `TestLedger_DEV02_fix` in `internal/dev`, table: `^7.4\|^8.0` → 8.0 (7.4 when only php7.4 is installed), `^8.0 \|\| ^7.4` → 8.0, `>=7.2 <8.0` → 7.2, `~8.2.10` → 8.2 |
| Status | accept: done 37bcf69 |

<a id="dev-03"></a>
#### DEV-03 · Major-only constraints are ignored and give the 8.4 default

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N6) |
| Area | Correctness |
| Bash location | `lib/modules/php.sh:48,66` |
| Observed behavior | `^7`, `^8`, `>=8`, `8.*` and `*` hold no X.Y, so composer.json is skipped and the result is the 8.4 default: a `^7` project is told 8.4, a major its constraint excludes. |
| Reproduction | `d=$(mktemp -d); printf '{"require":{"php":"^7"}}' >"$d/composer.json"; env -i HOME="$d" PATH=/usr/local/bin:/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; php_detect_version "$HOME"'` → `"version":"8.4"` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced: no X.Y → next source → 8.4. |
| Fix recipe (3.0) | Same function and rule as DEV-02 (a bare major becomes `X.0`); `*` alone still falls through. Docs: as DEV-02. |
| Proving test | `TestLedger_DEV03_fix` in `internal/dev`: `^7` → 7.0, `>=8` → 8.0, `8.*` → 8.0, `*` → 8.4 |
| Status | accept: done 37bcf69 |

<a id="dev-04"></a>
#### DEV-04 · Two X.Y values on line 1 of .target-php-version give a multi-line version

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N1) |
| Area | Contract |
| Bash location | `lib/modules/php.sh:31` |
| Observed behavior | `head -1 FILE \| grep -oE '[0-9]+\.[0-9]+'` prints every match: `7.4 8.0` gives `"version":"7.4\n8.0","php_command":"php74\n80","composer_command":"composer74\n80"`; `8.2.15.3` gives `"8.2\n15.3"`. The JSON stays valid, the values are unusable. |
| Reproduction | `d=$(mktemp -d); printf '7.4 8.0\n' >"$d/.target-php-version"; env -i HOME="$d" PATH=/usr/local/bin:/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; php_detect_version "$HOME"'` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced: all matches joined with `\n`; the binary probes use major `7`. |
| Fix recipe (3.0) | `internal/dev/php.go` `targetFileVersion`: first match only (`FindString` on line 1), as the composer and copilot sources already do. Docs: none. |
| Proving test | `TestLedger_DEV04_fix` in `internal/dev`: `7.4 8.0` → `"7.4"`, `8.2.15.3` → `"8.2"` |
| Status | accept: done 37bcf69 |

<a id="dev-05"></a>
#### DEV-05 · node_detect_version counts every entry of versions/node as installed

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N8) |
| Area | Correctness |
| Bash location | `lib/modules/node.sh:275-289` |
| Observed behavior | `ls -1 $NVM_DIR/versions/node` is used unfiltered: a stray `zzz-broken` entry becomes the newest version (`Warning: Node zzz-broken not installed. Using zzz-broken. Run: nvm install zzz-broken` and `"version":"zzz-broken","node_command":"nodezzz-broken"`), and a prerelease `v20.12.0-rc.1` wins over `v20.11.1`. node_upgrade already keeps only `^v[0-9]+\.[0-9]+\.[0-9]+$` (node.sh:97-103). A name with invalid UTF-8 reaches the JSON raw (`__gash_json_escape` escapes only `\ "` and C0, utils.sh:171-195). |
| Reproduction | `d=$(mktemp -d); mkdir -p "$d/.nvm/versions/node/v20.11.1" "$d/.nvm/versions/node/zzz-broken"; env -i HOME="$d" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; cd "$HOME"; node_detect_version'` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | reproduced: every directory name, leading `v` stripped, GNU `sort -V` order (DEV-13); names written into the JSON byte for byte as 2.x does (raw invalid UTF-8 included). |
| Fix recipe (3.0) | `internal/dev/node.go` `installedVersions`: keep only names matching `^v[0-9]+\.[0-9]+\.[0-9]+$` (the node_upgrade rule); nothing left → the existing `No Node.js versions installed in NVM. Install: nvm install 20` error, rc 1. Docs: node_detect_version NOTES say "installed vX.Y.Z releases". |
| Proving test | `TestLedger_DEV05_fix` in `internal/dev`: stray entry ignored (`"version":"20"`), `v20.12.0-rc.1` ignored |
| Status | accept: done 37bcf69 |

<a id="dev-06"></a>
#### DEV-06 · node_upgrade exits 0 when installing a new patch failed

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N12) |
| Area | Contract |
| Bash location | `lib/modules/node.sh:191-199,211-224` |
| Observed behavior | A failed `nvm install` prints `Error: Installing vX failed: the vM patches are kept.` and `Warning: vY kept (installing the new vM failed).`, then `OK: Active version: ...` and returns 0: `rc` only records `nvm uninstall` failures. |
| Reproduction | `d=$(mktemp -d); mkdir -p "$d/.nvm/versions/node/v20.11.1"; env -i HOME="$d" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; nvm(){ case $1 in ls-remote) echo v20.19.5;; current) echo v20.11.1;; install) return 1;; esac; }; node_upgrade -y; echo rc=$?'` → `rc=0` |
| Severity | medium |
| Public-contract impact | yes (exit status) |
| rc.A behavior | reproduced: the apply phase in the shim (DEV-16) keeps 2.x's `rc` rule. |
| Fix recipe (3.0) | In the generated node_upgrade apply phase (`internal/compatgen` template of `lib/modules/node.sh`): set `rc=1` when any `nvm install` fails; keep the messages and the final `OK: Active version:` line, then return `rc`. Docs: node_upgrade NOTES add "exits 1 when an install or a removal failed". |
| Proving test | `testdata/script/node_upgrade_install_fail.txtar` (mock nvm whose install fails: rc 1, v20.11.1 kept, the error line present) as `TestLedger_DEV06_fix` |
| Status | accept: done dda94a4 |

<a id="dev-07"></a>
#### DEV-07 · nvm_prune_versions --force reports success when an uninstall fails

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N14) |
| Area | Contract |
| Bash location | `lib/modules/node.sh:81-93` |
| Observed behavior | The exit status of `nvm uninstall` is ignored: every run with `--force` ends with `OK: Duplicate NVM versions removed.` and rc 0, even when nothing was removed. |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; nvm(){ case $1 in current) echo v20.11.1;; ls) printf "v20.9.0\n-> v20.11.1\n";; uninstall) return 3;; esac; }; nvm_prune_versions --force; echo rc=$?'` → `OK: Duplicate NVM versions removed.` `rc=0` |
| Severity | medium |
| Public-contract impact | yes (exit status and final line) |
| rc.A behavior | reproduced in the shim. |
| Fix recipe (3.0) | Generated nvm_prune_versions shim: record each failing `nvm uninstall`, print `Error: Uninstalling vX failed.` for it, and end with `Error: N duplicate version(s) could not be removed.` rc 1 instead of the OK line when N > 0. Docs: nvm_prune_versions NOTES. |
| Proving test | `testdata/script/nvm_prune_uninstall_fail.txtar` (`TestLedger_DEV07_fix`): rc 1, no OK line |
| Status | accept: done dda94a4 |

<a id="dev-08"></a>
#### DEV-08 · nvm_prune_versions reads io.js entries as Node versions

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (dossier, nvm_prune_versions "iojs-vX parsed as node vX") |
| Area | Correctness |
| Bash location | `lib/modules/node.sh:61-68` |
| Observed behavior | `grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+'` extracts `v3.3.0` from `iojs-v3.3.0`, so io.js installs become "duplicates" of a Node major 3 and `--force` runs `nvm uninstall v3.3.0` (a Node version that is not installed), then prints the OK line. |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; nvm(){ case $1 in current) echo v20.11.1;; ls) printf "iojs-v3.3.0\niojs-v3.3.1\n-> v20.11.1\n";; esac; }; nvm_prune_versions'` → `Warning: Dry run: would uninstall v3.3.0. ...` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | reproduced in the shim. |
| Fix recipe (3.0) | Generated shim: match whole entries with `grep -oE '(^\|[[:space:]>])v[0-9]+\.[0-9]+\.[0-9]+'` and strip the prefix, so `iojs-v*` lines are skipped. Docs: none. |
| Proving test | `testdata/script/nvm_prune_iojs.txtar` (`TestLedger_DEV08_fix`): `OK: No duplicate NVM versions found.` |
| Status | accept: done dda94a4 |

### ai

<a id="ai-01"></a>
#### AI-01 · ai_sysinfo `?` reads and sends any named file after the first consent

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N24); lead decision: high-severity Security defect, reproduced in rc.A |
| Area | Security |
| Bash location | `lib/modules/ai/sysinfo.sh:53-101` (entity extraction, `head -200` / `ls -la`), `:758-760` (the only consent), `:834-867` (`?` branch); `lib/modules/ai/client.sh:239` (pattern redaction only) |
| Observed behavior | Any path under `/etc`, `/var`, `/usr`, `/opt`, `/srv`, `/home`, `/run` named in a free-text question is read (`head -200` of a file, `ls -la` of a directory) with the user's rights and sent to the provider, with no forbidden-path, secret-file or symlink check and no new consent (the one at start says "drill-down details on request"); units add `systemctl cat`/`status` and `journalctl` through the cached sudo. Pattern redaction keeps shadow-style hashes, key files without a PEM header and most config values (AI-07). Run as root on a bind-mounted fake `/etc/shadow`, the dossier saw `=== FILE: /etc/shadow ===` and the hash line in the request. |
| Reproduction | `d=$(mktemp -d -p /var/tmp); printf 'root:$6$fakesalt$fakehash:19000:0:99999:7:::\n' >"$d/id_rsa.bak"; env -i HOME="$(mktemp -d)" PATH=/usr/local/bin:/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; __sysinfo_collect_question_context "what is in $1?" \| __ai_redact' _ "$d/id_rsa.bak"` → `=== FILE: .../id_rsa.bak ===` and the line unchanged (this is the context the `?` request carries; the menu run itself is the pty reproduction kept outside the repository) |
| Severity | high |
| Public-contract impact | yes (some questions collect less; a confirmation appears) |
| rc.A behavior | reproduced: same entity regexes, same reads with the same rights (sudo only for `journalctl`), sent under the single initial consent. |
| Fix recipe (3.0) | `internal/ai/sysinfo.go` `questionContext`: for every extracted path, clean it lexically, refuse `..` components, resolve symlinks, and apply `internal/pathguard` exactly as the `llm_*` file readers do (forbidden list `/proc /sys /dev /boot /root /etc/shadow /etc/gshadow /etc/passwd /etc/sudoers /etc/sudoers.d` and root's home, secret-file patterns and directories, case-insensitive base names, on both the lexical and the resolved path); a refused path contributes only `=== FILE: <path> === (skipped: protected path)`, never content; directory listings omit entries that match the secret patterns. Then, unless `GASH_AI_CONFIRM=0`, ask per `?` request through `__ai_confirm_send` with kind `data`, WHAT = `question context (<item list: files, directories, units, ports>)`, so auto mode asks too; no terminal → the existing `Error: Not sending ... no terminal to confirm on. Set GASH_AI_CONFIRM=0 in ~/.gash_env to allow it`, back to the menu; refusal → back to the menu, nothing sent. Docs: `docs/security.md` AI section and the ai_sysinfo help (SMART QUESTIONS, PRIVACY) state the path rules and the per-question confirmation. |
| Proving test | `TestLedger_AI01_fix` in `internal/ai` (sysroot fixture: questions naming `/etc/shadow`, `/root/x`, `id_rsa.bak`, a symlink to `/etc/shadow` collect no content; answering `n` on the PTY sends no request: the fake HTTPS server sees only the initial analysis) and `testdata/script/ai_sysinfo_question_guard.txtar`; `TestLedger_AI01_2x` proves the rc.A reproduction |
| Status | accept: done 1e8a8f0 |

<a id="ai-02"></a>
#### AI-02 · `ai_query claude` sends the word "claude" as the question

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "`ai_query claude` sends the word claude" (CONFIRMED) |
| Area | Correctness |
| Bash location | `lib/modules/ai/chat.sh:290-303` |
| Observed behavior | With one argument the provider check is skipped: `ai_query claude` (and `ai_query gemini`) sends the provider name as the question to the first configured provider, rc 0 with an answer. |
| Reproduction | `d=$(mktemp -d); mkdir "$d/bin"; printf '#!/bin/sh\necho called >>"$HOME/curl.log"; exit 6\n' >"$d/bin/curl"; chmod +x "$d/bin/curl"; printf 'AI:claude=sk-ant-fake\n' >"$d/.gash_env"; chmod 600 "$d/.gash_env"; env -i HOME="$d" PATH="$d/bin:/usr/bin:/bin" bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; ai_query claude </dev/null; cat "$HOME/curl.log"'` → a request was attempted (`called`) |
| Severity | medium |
| Public-contract impact | yes (now an error, rc 1) |
| rc.A behavior | reproduced. |
| Fix recipe (3.0) | `internal/ai/query.go` argument grammar: when the only argument is `claude` or `gemini`, take it as the provider and leave the query empty → `Error: No query provided` rc 1 before any request, also with piped input (the usage requires a question). Docs: ai_query usage unchanged. |
| Proving test | `TestLedger_AI02_fix` in `internal/cli/args` and `testdata/script/ai_query_provider_only.txtar` (fake server request count 0, stderr `Error: No query provided`, rc 1; `ai_query claude hi` unchanged) |
| Status | accept: done 8049ea1 |

<a id="ai-03"></a>
#### AI-03 · ai_ask asks one question per call; a typed `exit` is sent as a question

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N17) |
| Area | Correctness |
| Bash location | `lib/modules/ai/chat.sh:202-258`; help `lib/modules/ai.sh:92-93` |
| Observed behavior | There is no chat loop: one line is read, sent, answered, and the function returns. Typing `exit` or `quit`, which the help names as the way to end "the session", sends that word to the provider. |
| Reproduction | `script -qec "env -i HOME=$d PATH=$d/bin:/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; GASH_HEADLESS=0; ai_ask claude'" /dev/null <<<exit` with `$d` set up as in AI-02 and a fake curl that saves `@body` → `.messages[0].content` ends with `exit`, one request |
| Severity | low |
| Public-contract impact | yes if the loop is added; no for the help-only fix |
| rc.A behavior | reproduced (one question per call; `exit` is a question). |
| Fix recipe (3.0) | Recommended (keeps the one-shot contract): at the prompt, an exact `exit`, `quit` or `q` (trimmed, any case) returns rc 1 with no request and no message, and the help NOTES become "One question per call; Enter on an empty line aborts, exit/quit cancel". Alternative: a real loop until exit/quit/EOF (then the help stays). |
| Proving test | `TestLedger_AI03_fix` (PTY: `exit` → rc 1, no request) in `internal/ai` |
| Status | accept: done 4fed551 |

<a id="ai-04"></a>
#### AI-04 · ai_ask without a terminal sends stdin for auto-analysis without asking

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N18) |
| Area | Security |
| Bash location | `lib/modules/ai/chat.sh:193-223,234` |
| Observed behavior | Without a controlling terminal, stdin is taken as piped content, the question read from `/dev/tty` fails with `chat.sh: line 210: /dev/tty: No such device or address`, the empty question becomes the auto-analysis prompt, and the content is sent without any confirmation (kind `query`, auto mode). |
| Reproduction | `printf 'how to list files\n' \| setsid env -i HOME="$d" PATH="$d/bin:/usr/bin:/bin" bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; ai_ask claude'` with `$d` as in AI-02 → the `/dev/tty` error line, then the fake curl is called |
| Severity | medium |
| Public-contract impact | yes (now refused) |
| rc.A behavior | reproduced: stdin taken as piped content, empty question, auto-analysis sent without confirmation, same rc. The Bash redirection error line is a forced divergence (AI-20). |
| Fix recipe (3.0) | `internal/ai/ask.go`: before reading, test that `/dev/tty` can be opened and `GASH_HEADLESS` is not `1`; otherwise `Error: ai_ask needs a terminal: use ai_query "question" (stdin is read as piped input)` rc 1, nothing read, nothing sent. Docs: ai_ask NOTES. |
| Proving test | `TestLedger_AI04_fix` in `internal/ai` (no TTY: rc 1, fake server count 0) |
| Status | accept: done 4fed551 |

<a id="ai-05"></a>
#### AI-05 · ai_query and ai_ask block on any open non-terminal stdin

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N19) |
| Area | Correctness |
| Bash location | `lib/modules/ai/chat.sh:193-194,278-281` |
| Observed behavior | `[[ ! -t 0 ]]` is the only test, so `head -c 4096` waits for EOF on any stdin that is not a terminal: an agent harness or a cron job with an open, idle pipe hangs forever. |
| Reproduction | `sleep 4 \| timeout 2 env -i HOME="$d" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; ai_query claude "what is ls"'; echo rc=$?` → `rc=124` |
| Severity | medium |
| Public-contract impact | yes (an idle stdin stops being read) |
| rc.A behavior | reproduced (stdin read until EOF or 4096 bytes when it is not a terminal). |
| Fix recipe (3.0) | `internal/ai/input.go` `readPiped`: read stdin when it is a regular file, or a FIFO/socket that becomes readable (data or EOF) within 2 s (`poll(2)` on fd 0); otherwise ignore it and print `Warning: stdin is open but sent nothing within 2s: ignored (redirect </dev/null to skip the wait)`, then go on with the question alone. Same rule in ai_ask. Docs: ai_query and ai_ask NOTES ("piped input must start within 2 seconds"). |
| Proving test | `TestLedger_AI05_fix` in `internal/ai` (socketpair/tty-less idle stdin: returns without waiting) |
| Status | accept: done 4fed551 |

<a id="ai-06"></a>
#### AI-06 · An unclosed PRIVATE KEY block swallows the rest of the text, the question included

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N22) |
| Area | Correctness |
| Bash location | `lib/modules/ai/client.sh:147-150`; `lib/modules/ai/chat.sh:226-232,311-317` |
| Observed behavior | Redaction runs on the composed text (pipe + `User question:`); a `BEGIN ... PRIVATE KEY` line without its END (common after the 4 KB cut) drops every following line, so the request carries no question and no closing `---`. Fails closed for the key, but the answer is about nothing. |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; printf "[PIPE INPUT - TROUBLESHOOT MODE]\n---\n-----BEGIN RSA PRIVATE KEY-----\nabc\n---\nUser question: q1\n" \| __ai_redact'` → ends at `[REDACTED PRIVATE KEY]` |
| Severity | medium |
| Public-contract impact | no (request content only) |
| rc.A behavior | reproduced (line-based awk rule, then the sed rules, on the whole user text). |
| Fix recipe (3.0) | `internal/ai/redact.go` callers: redact the piped part and the question separately (each with the full rule set; an unclosed block still drops the rest of its own part), then compose the wrapper; the Context prefix keeps its 2.x handling. Docs: none. |
| Proving test | `TestLedger_AI06_fix` in `internal/ai` (piped `-----BEGIN RSA PRIVATE KEY-----\nabc`, question `q1` → body contains `[REDACTED PRIVATE KEY]` and `User question: q1`, not `abc`) |
| Status | accept: done 3f3b8d0 |

<a id="ai-07"></a>
#### AI-07 · Redaction misses bearer headers, Gash's own SSH: entries and password hashes

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N23) |
| Area | Security |
| Bash location | `lib/modules/ai/client.sh:146-160` |
| Observed behavior | The pattern set keeps `Authorization: Bearer eyJ...`, `SSH:~/.ssh/id_rsa=correct horse battery` (the passphrase line format of `~/.gash_env`), crypt hashes (`$6$salt$hash`), `AKIA` keys in lower case and free text such as `the password is hunter2`; `DB_PASSWORD=...`, URL passwords and the listed token formats are redacted. The help promises "KEY=value secrets and well-known token formats", which these lines are not, except the `~/.gash_env` format. |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; printf "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.e30.abcdef\nSSH:~/.ssh/id_rsa=correct horse battery\nthe password is hunter2\nDB_PASSWORD=s3cret\n" \| __ai_redact'` → only the last line is redacted |
| Severity | medium |
| Public-contract impact | no (request content only; docs change) |
| rc.A behavior | reproduced: the awk block rule then the eight sed rules, verbatim and in that order, line based (the rail is ported unchanged). |
| Fix recipe (3.0) | `internal/ai/redact.go`: add, after the existing rules, `(?i)\b((?:proxy-)?authorization[ \t]*:[ \t]*(?:bearer\|basic\|token)[ \t]+)[^ \t]+` → `${1}[REDACTED]`; `^(SSH:[^=]*=).*` → `${1}[REDACTED]`; `\$(?:1\|2[abxy]?\|5\|6\|7\|y\|gy)\$[^:\s]+` → `[REDACTED HASH]`; `AKIA` case stays as is (AWS keys are upper case). Docs: ai.md and security.md PRIVACY lists name the new formats and say free text ("the password is ...") is not detected. |
| Proving test | `TestLedger_AI07_fix` in `internal/ai` (the four vectors above plus every `ai_client_spec.sh` vector unchanged) |
| Status | accept: done dff1b2c |

<a id="ai-08"></a>
#### AI-08 · The port pattern of ai_sysinfo questions takes clock times for ports

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N26) |
| Area | Correctness |
| Bash location | `lib/modules/ai/sysinfo.sh:103-115` |
| Observed behavior | `(port[: ]+\|:)([0-9]{1,5})([^0-9]\|$)` matches any colon before digits: "at 10:30" collects `=== PORT: 30 ===`, and the `grep -F ":30"` filter of `ss -tlnp` also matches `:3000` and `:3306` listeners, so unrelated socket data is sent. |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; __sysinfo_collect_question_context "why did the backup fail at 10:30?"'` → `=== PORT: 30 ===` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | reproduced (same pattern on the lower-cased question, same `ss -tlnp` substring filter). |
| Fix recipe (3.0) | `internal/ai/sysinfo.go` `questionContext`: accept `port[: ]+N` or `HOST:N` where HOST is an IPv4 address, `localhost`, `*` or a bracketed IPv6, with 1 <= N <= 65535; keep `ss -tlnp` rows whose local-address column ends with `:N` exactly. |
| Proving test | `TestLedger_AI08_fix` in `internal/ai` ("at 10:30" → no port; "port 3306" and "127.0.0.1:8080" → ports; filter excludes `:3000` for N=30) |
| Status | accept: done a0dec48 |

<a id="ai-09"></a>
#### AI-09 · Saving the ai_sysinfo report: a failed write is reported as saved

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N27) |
| Area | Correctness |
| Bash location | `lib/modules/ai/sysinfo.sh:792-793,866,869-875,914` |
| Observed behavior | `printf '%s' "$md" >"$filename"` is not checked: in an unwritable cwd (`/proc`) Bash prints its redirection error, then `✓ Report saved to gash-ai-sysinfo-....md`, rc 0 at quit. The buffer is built with `md+=$(...)`, so every chunk loses its trailing blank lines (`- **Info:** DHCP` runs into the next heading) and the file has no final newline; mode comes from the umask (0644 typical) although it holds system data. |
| Reproduction | `script -qec "env -i HOME=$d PATH=$d/bin:/usr/bin:/bin GASH_AI_CONFIRM=0 bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; GASH_HEADLESS=0; cd /proc; ai_sysinfo claude'" /dev/null` with a fake curl in `$d/bin` answering a valid analysis, then `s` and `q` typed → the error line, then `Report saved` |
| Severity | medium |
| Public-contract impact | yes (failure message and file bytes) |
| rc.A behavior | reproduced: unchecked write, success line, the same Markdown bytes (blank lines dropped, no final newline), umask mode. The Bash redirection error line itself is a forced divergence (AI-20). |
| Fix recipe (3.0) | `internal/ai/sysinfo.go` `saveReport`: build the buffer with the chunks' blank lines kept and a final newline; write it with `renameio` at mode 0600; on error print `Error: Could not save the report to <file>: <reason>` and no success line. Docs: ai_sysinfo help ("Option s exports ... (mode 0600)"). |
| Proving test | `TestLedger_AI09_fix` in `internal/ai` (unwritable dir: error, no success line; written file ends with `\n`, keeps the blank line between sections, mode 0600) |
| Status | accept: done 97372d5 |

<a id="ai-10"></a>
#### AI-10 · An analysis reply without `.sections` prints four jq errors and empty counts

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N29) |
| Area | Correctness |
| Bash location | `lib/modules/ai/sysinfo.sh:376-380,391` |
| Observed behavior | A schema-valid reply that lacks `sections` (e.g. `{"hostname":"x"}`) prints `jq: error (at <stdin>:1): Cannot iterate over null (null)` four times and the counts line ` critical   warnings   info   ok`, then rc 0. |
| Reproduction | fake curl (in `$d/bin`) answering `{"content":[{"type":"text","text":"{\"hostname\":\"x\"}"}],"stop_reason":"end_turn"}`; `env -i HOME="$d" PATH="$d/bin:/usr/bin:/bin" GASH_AI_CONFIRM=0 bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; ai_sysinfo claude </dev/null'` |
| Severity | low |
| Public-contract impact | yes (stderr lines and counts) |
| rc.A behavior | reproduced: the same four lines (jq 1.7 text, the documented tool-present path) on stderr and the same empty counts line, rc 0. |
| Fix recipe (3.0) | `internal/ai/sysinfo.go` `formatReport`: treat a missing or non-array `sections` as empty (`0 critical  0 warnings  0 info  0 ok`) and print `Warning: The reply holds no sections.` |
| Proving test | `TestLedger_AI10_fix` in `internal/ai` (reply without sections: no `jq:` line, zero counts, the warning) |
| Status | accept: done d3eaf94 |

<a id="ai-11"></a>
#### AI-11 · Context distro detection misses quoted IDs and Linux Mint

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N31) |
| Area | Correctness |
| Bash location | `lib/modules/ai/chat.sh:26-42` |
| Observed behavior | `grep -oP '^ID=\K\w+'` fails on quoted values, so `ID="rocky"` or `ID="almalinux"` gives `"distro":"linux","pkg":"unknown"`; `ID=opensuse-tumbleweed` stops at the hyphen (`opensuse`, still zypper); Linux Mint's real `ID=linuxmint` is not in the table (`mint` is), so pkg `unknown`. The model then suggests the wrong package manager. |
| Reproduction | `f=$(mktemp); printf 'ID="rocky"\n' >"$f"; grep -oP '^ID=\K\w+' "$f" \|\| echo linux` (the expression of chat.sh:27) |
| Severity | low |
| Public-contract impact | no (request content only) |
| rc.A behavior | reproduced: same expression semantics (`\w+` right after `ID=`, first line that matches) and the same table. |
| Fix recipe (3.0) | `internal/ai/context.go` `distro`: parse `ID=` with optional single or double quotes, value `[A-Za-z0-9._-]+`; add `linuxmint` and `almalinux` to the table; when ID is unknown, map the first known word of `ID_LIKE` (debian/ubuntu → apt, rhel/fedora/centos → dnf, arch → pacman, suse → zypper). |
| Proving test | `TestLedger_AI11_fix` in `internal/ai` (`ID="rocky"` → rocky/dnf, `ID=linuxmint` → apt, `ID=foo` + `ID_LIKE=debian` → apt) |
| Status | accept: done 8284c56 |

<a id="ai-12"></a>
#### AI-12 · GASH_AI_MODEL_GEMINI goes into the request URL unvalidated

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N36) |
| Area | Security |
| Bash location | `lib/modules/ai/client.sh:228-231` |
| Observed behavior | The URL is `https://generativelanguage.googleapis.com/v1beta/models/${model}:generateContent`; a model value with `/`, `..`, `?` or `#` changes the path or query on that host, and the `x-goog-api-key` header goes with it. The value comes from the user's own environment or `~/.gash_env`. |
| Reproduction | `env -i HOME="$d" PATH="$d/bin:/usr/bin:/bin" GASH_AI_MODEL_GEMINI='../x?y=' bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; ai_query gemini hi there </dev/null'` with an `AI:gemini=` entry and a fake curl that logs its last argument → `.../models/../x?y=:generateContent` |
| Severity | low |
| Public-contract impact | yes (invalid values become an error) |
| rc.A behavior | reproduced: the value is concatenated as is; a value that makes the URL unparsable for `net/url` gets the transport fallback message of AI-15 instead of curl's exit-3 text. |
| Fix recipe (3.0) | `internal/ai/client.go` `model`: both model settings must match `^[A-Za-z0-9][A-Za-z0-9._-]*$`, else `Error: Invalid GASH_AI_MODEL_GEMINI value (letters, digits, '.', '_' and '-' only)` rc 1, no request. Docs: configuration.md settings table. |
| Proving test | `TestLedger_AI12_fix` in `internal/ai` (`../x` refused, request count 0; `gemini-2.5-pro` accepted) |
| Status | accept: done 9bbbff9 |

### file scanners

<a id="fs-02"></a>
#### FS-02 · -L / --follow-symlinks walks into the forbidden trees

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "-L/--follow-symlinks walks into forbidden paths" (CONFIRMED) |
| Area | Security |
| Bash location | `lib/modules/files.sh:450` (files_largest `find -L`), `1368` (tree_stats main walk `find -L`); the forbidden check runs only on the PATH argument in `__gash_fs_safe_path` (`150`, `187`) |
| Observed behavior | With `-L`, find follows every symlink below ROOT: a link to /proc, /root or another protected tree is walked, its files are listed with their sizes and tree_stats counts them (files_total 49 instead of 1 on the dossier fixture). The PATH argument itself is guarded: scanning the link directly gives `forbidden_path` FATAL with the resolved path. Only files_largest and tree_stats accept `-L`. |
| Reproduction | Input shape: a temporary directory holding a symlink to one of the protected roots, scanned with `files_largest -L --json --depth 2 DIR`; the run is kept outside the repository (`work/files-scanners/repro-follow.sh`, metadata reads only, nothing written) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the walker follows links under `-L` and does not consult pathguard for entries below ROOT. A safety-rail gap, but rc.A neither loosens nor tightens rails silently; fixed in 3.0. |
| Fix recipe (3.0) | In `internal/fsscan` (walker), check every directory entered and every entry reported, after symlink resolution (`filepath.EvalSymlinks` whenever the entry is, or was reached through, a followed link), against the forbidden list of `internal/pathguard` (`pathguard.Forbidden`: /proc /sys /dev /boot /root and root's home, whole-component match, the same table the PATH check uses); on a match skip it silently: no stat of the target, no descent, not counted in `errors`. Applies to files_largest and to both tree_stats counts. AGENTS 6.1: "Scanners refuse /proc, /sys, /dev, /boot, /root, also when reached through a symlink or from /"; `-L` help: "Follow symbolic links (never into protected paths)". |
| Proving test | `TestLedger_FS02_2x` and `TestLedger_FS02_fix` in `internal/fsscan`, with the pathguard table pointed at a fixture directory (test build, sysroot indirection): rc.A lists the file behind the link; 3.0 lists only `own/f`, and tree_stats `-L` equals the counts of the tree without the link. `testdata/script/fsscan_follow_forbidden.txtar` checks the envelope. |
| Status | accept: done d6a4bf9 |

<a id="fs-03"></a>
#### FS-03 · --allow-root / walks /proc /sys /dev /boot /root

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N5) |
| Area | Security |
| Bash location | `lib/modules/files.sh:180-184` (the only `/` check), walk commands at `449-468`, `660-685`, `930-954`, `1174-1190`, `1367-1384`; header comment `23-24` ("--allow-root implies a warning") |
| Observed behavior | `--allow-root` lifts the root_scan_blocked refusal and the walk then descends into every protected tree: 46 reported paths under /proc, /root, /sys at depth 2 (7, all under /root, with `--xdev`). No warning is printed although the header comment promises one. All five scanners. |
| Reproduction | Input shape: `files_largest --allow-root --json --depth 2 -n 100000 /` piped to a jq count of paths under the protected roots; kept outside the repository (dossier N5 run, metadata reads only) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: with `--allow-root` the walk from `/` enters the protected trees; no warning. |
| Fix recipe (3.0) | Same check as FS-02 in `internal/fsscan`: every directory about to be entered is tested with `pathguard.Forbidden` (after resolution); from `/` this prunes /proc /sys /dev /boot /root at depth 1, silently and outside `errors`. No new warning line (the comment is dropped with the Bash source). AGENTS 6.1 as in FS-02. |
| Proving test | `TestLedger_FS03_2x` / `TestLedger_FS03_fix` in `internal/fsscan` on a sysroot fixture holding `proc/`, `root/` and `home/`: scanning the fixture root with `--allow-root` reports entries of `proc/` and `root/` in rc.A and only `home/` entries in 3.0, for all five scanners. |
| Status | accept: done d6a4bf9 |

<a id="fs-04"></a>
#### FS-04 · A second PATH silently wins; words after `-- PATH` are ignored

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "a second PATH silently wins instead of unexpected_argument" (CONFIRMED) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:416-419`, `629-632`, `899-902`, `1154-1157`, `1348-1351` (`*) dir="$1"`); `--` branches `407-411`, `620-624`, `890-894`, `1145-1149`, `1339-1343` |
| Observed behavior | Every bare word replaces PATH, so the last one wins: `files_largest --json A B` scans only B, rc 0. After `--` the next word becomes PATH and every later word is dropped (`-- A B` scans A). Same in all five scanners. AGENTS 3 and 9 promise `unexpected_argument`. |
| Reproduction | `d=$(mktemp -d); mkdir "$d/a" "$d/b"; printf x >"$d/b/f"; files_largest --json "$d/a" "$d/b"` lists `$d/b/f`, rc 0 |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the scanner grammar in `internal/cli/args` keeps last-PATH-wins and the `--` rule. |
| Fix recipe (3.0) | In the scanner grammar (`internal/cli/args`, scanners family): remember that a PATH was taken; a second positional (before or after `--`), or any word after the word following `--`, fails with `unexpected_argument` RETRY, details = that word, hint `Pass at most one PATH` (as `__llm_take_path`, `lib/modules/llm/core.sh:416-425`); text mode prints `Error: unexpected_argument: WORD` and `Hint: Pass at most one PATH`, rc 1. AGENTS 3 and 9 then hold. |
| Proving test | `TestLedger_FS04_2x` / `TestLedger_FS04_fix` in `internal/cli/args` (table over the five scanners): 3.0 `$fn --json A B` and `$fn --json -- A B` print `{"error":"unexpected_argument","details":"B","action":"RETRY","recoverable":true,"hint":"Pass at most one PATH"}` on stderr, nothing on stdout, rc 1; `$fn --json -- A` still scans A. |
| Status | accept: done d6a4bf9 |

<a id="fs-05"></a>
#### FS-05 · Integer flags accept digit strings of any length

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "integers of any length are accepted (contract: 1 to 9 digits)" (CONFIRMED) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:424`, `428`, `636`, `640`, `911`, `915`, `1161`, `1165`, `1355`, `1359` (`^[0-9]+$`, no length bound) |
| Observed behavior | `-n`, `--depth`, `--min-depth` and `--top` accept any number of digits. `-n` and `--top` are read by awk as decimal doubles (`-n 1234567890` is no cap, `010` is 10). find-backed depths (files_largest, dirs_list_empty, tree_stats) are decimal; above 2147483647 find refuses the value and the scanner returns `"data":[]`, `errors:1`, rc 0. AGENTS 3 says integers have at most 9 digits. |
| Reproduction | `d=$(mktemp -d); printf x >"$d/f"; files_largest --json --depth 2147483648 "$d"` gives an empty envelope with `"errors":1`, rc 0 |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced on the GNU path: any length accepted; `-n`/`--top` compare as float64; find-backed depths read as decimal, values above 2147483647 give the empty success with `errors: 1` (FS-14); du-backed depths (dirs_largest, dirs_find_large) work up to 9223372036854775807. Diverging parts: in the du-backed depths a leading zero, `08`/`09` and values above 2^63-1 get `invalid_depth` (forced divergence FS-21); non-ASCII digits get the error contract (forced divergence FS-29). |
| Fix recipe (3.0) | In the scanner grammar (`internal/cli/args`): every integer flag must match `^[0-9]{1,9}$` (ASCII) and is read in base 10 (`strconv.Atoi`, so `010` = 10); failures keep the current codes and hints (`invalid_limit`, `invalid_depth`, `invalid_top`, RETRY) with details = the raw value; per-scanner meaning of `0` unchanged and documented. Same rule as `__llm_validate_uint` (`lib/modules/llm/core.sh:280-288`). |
| Proving test | `TestLedger_FS05_2x` / `TestLedger_FS05_fix` in `internal/cli/args`: for each of the ten integer flags, `1234567890` is accepted in rc.A and gives its `invalid_*` RETRY envelope, rc 1, in 3.0; `files_largest --json --depth 999999999 D` succeeds in both; `dirs_largest --json --depth 010` on a 14-level fixture lists rows down to depth 10 in 3.0. |
| Status | accept: done d6a4bf9 |

<a id="fs-06"></a>
#### FS-06 · dirs_largest keeps paths containing a tab

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "Operator decision: paths with a tab or a newline are left out" (PARTIAL: dirs_largest part) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:681-683` (du excludes only `*\n*`), `751` (awk re-joins fields 3..NF) |
| Observed behavior | A directory `t<TAB>x` is listed: JSON `"t\tx"`, `--null` intact, plain text splits the tab into a table column (`t     x`), rich text shows a space. Contradicts AGENTS 3.2. |
| Reproduction | `d=$(mktemp -d); t="$d/$(printf 't\tx')"; mkdir "$t"; head -c 5000 /dev/zero >"$t/f"; dirs_largest --json "$d"` lists the tab path |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced in every mode (JSON, --null, plain text column split, rich space). |
| Fix recipe (3.0) | Apply the documented exclusion uniformly: the FS-01 predicate in `internal/fsscan` (tab paths skipped and left out of parent totals). |
| Proving test | `TestLedger_FS06_2x` (rc.A lists `t\tx`, plain text shows the split column) / `TestLedger_FS06_fix` (absent in every mode) in `internal/fsscan`; part of `fsscan_tabnl.txtar`. |
| Status | accept: done d6a4bf9 |

<a id="fs-07"></a>
#### FS-07 · dirs_list_empty keeps paths containing a tab

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "Operator decision: paths with a tab or a newline are left out" (PARTIAL: dirs_list_empty part) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:1186-1190` (find `! -path '*\n*'` only) |
| Observed behavior | An empty directory `et<TAB>x` is listed in every mode: JSON `"et\tx"`, `--null` intact, text with a raw tab, `--count` counts it. Newline paths are left out. |
| Reproduction | `d=$(mktemp -d); mkdir "$d/$(printf 'et\tx')"; dirs_list_empty --json "$d"` gives `{"data":["…/et\tx"],"count":1,…}` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced in every mode. |
| Fix recipe (3.0) | Apply the documented exclusion uniformly (FS-01 predicate in `internal/fsscan`). |
| Proving test | `TestLedger_FS07_2x` / `TestLedger_FS07_fix` in `internal/fsscan`: count 1 then 0 on the fixture, in --json, --null, --count and text. |
| Status | accept: done d6a4bf9 |

<a id="fs-08"></a>
#### FS-08 · tree_stats counts tab paths, and its empty-dir count counts newline directories

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "Operator decision: paths with a tab or a newline are left out" (PARTIAL: tree_stats part) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1382-1384` (main walk excludes only newline paths), `1437` (tab re-join), `1541-1548` (empty walk: no name test, `wc -l`) |
| Observed behavior | files_total, dirs_total and size_total include tab paths (`t\tx/f`, `ft\ty`); empty_dirs counts the empty `et\tx` and counts the empty `en\nx` twice (one line per newline), while dirs_total leaves `en\nx` out: on the dossier fixture files_total 3, dirs_total 4, empty_dirs 3. |
| Reproduction | `d=$(mktemp -d); mkdir "$d/$(printf 'en\nx')"; tree_stats --json "$d"` reports `"empty_dirs":2` and `"dirs_total":1` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the main count keeps tab paths; the empty-dir count equals the number of lines GNU find would print (a newline in a name adds one). |
| Fix recipe (3.0) | Apply the documented exclusion uniformly (FS-01 predicate) in both tree_stats counts, which 3.0 merges into one walk (FS-16). |
| Proving test | `TestLedger_FS08_2x` (3/4/3 on the fixture) / `TestLedger_FS08_fix` (1/2/0, size_total 1000) in `internal/fsscan`. |
| Status | accept: done d6a4bf9 |

<a id="fs-09"></a>
#### FS-09 · dirs_find_large cuts a path at its first tab and can name another directory

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N2) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1032` (`awk -F'\t' 'NF>=2 { printf "%s\t\t%s\n", $1, $2 }'`); with `--with-mtime` the path survives (`read -r bytes path`, `1023`) |
| Observed behavior | Without `--with-mtime` a directory `t<TAB>x` is reported as `…/t` in JSON, `--null` and text. If `…/t` exists, a consumer (`--null \| xargs -0 …`) acts on a different directory. With `--with-mtime` the path is intact. |
| Reproduction | `d=$(mktemp -d); t="$d/$(printf 't\tx')"; mkdir "$t"; head -c 5000 /dev/zero >"$t/f"; dirs_find_large --size 1 --json "$d"` reports `"path":"$d/t"` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the path is truncated at its first tab in every mode without `--with-mtime`, intact with it. |
| Fix recipe (3.0) | Apply the documented exclusion uniformly (FS-01 predicate); independently, `internal/fsscan` never splits a path once read. |
| Proving test | `TestLedger_FS09_2x` (rc.A reports `$d/t`) / `TestLedger_FS09_fix` (no row for the tab dir; `dirs_find_large --size 1 --null D \| xargs -0 -n1 test -d` succeeds) in `internal/fsscan`. |
| Status | accept: done d6a4bf9 |

<a id="fs-10"></a>
#### FS-10 · JSON carries raw C0 control characters (invalid JSON)

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N3) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:1704-1712` (size-list awk `esc()`: `\\ \" \n \r \t` only), `1218-1233` (dirs_list_empty: `\\ \" \t \r` only) |
| Observed behavior | Directory names holding ESC, BS, FF, SOH or another C0 byte make the files_largest, dirs_largest, dirs_find_large and dirs_list_empty envelopes invalid (`jq: … control characters from U+0000 through U+001F must be escaped`); DEL and invalid UTF-8 pass raw. Text mode writes the bytes raw, so an ESC sequence in a name reaches the terminal. tree_stats is valid (`__gash_json_escape_to`). The bytes come from file names through Gash's own escaper, not from a tool's output. |
| Reproduction | `d=$(mktemp -d); mkdir "$d/$(printf 'e\033x')"; dirs_list_empty --json "$d" \| jq .` fails with a parse error |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced byte for byte: `internal/contract` has a 2.x scanner-escaper mode used by these four envelopes (escapes only the set 2.x escaped for that envelope; other C0, DEL and invalid UTF-8 written raw); text mode raw. |
| Fix recipe (3.0) | Encode every string of the five scanner envelopes with the standard contract escaping: `\"` `\\` `\n` `\r` `\t` `\b` `\f`, every other C0 as `\u00xx` (lowercase hex), as `__gash_json_escape_to` (`lib/core/utils.sh:173-195`) and architecture.md section 4; invalid UTF-8 stays raw unless the operator decides otherwise. Text mode unchanged in 3.0 (quoting control characters there would be a separate decision). |
| Proving test | `TestLedger_FS10_2x` (rc.A output bytes equal the 2.x golden for names `esc\033x`, `bs\bx`, `ff\fx`, `soh\001x`) / `TestLedger_FS10_fix` (every scanner's `--json` passes `jq -e .` and round-trips the names) in `internal/fsscan`. |
| Status | accept: done d6a4bf9 |

<a id="fs-11"></a>
#### FS-11 · The size parser's fallback joins all digits and accepts stray units

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N4, defect part) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:49-82` (`__gash_fs_parse_size`: `LC_ALL=C numfmt --from=iec` first; on failure delete spaces, concatenate every digit, take all non-digits as the unit, strip one `B` then one `i`) |
| Observed behavior | With numfmt present (the GNU path), inputs numfmt refuses fall through to the fallback: `1K2` = 12288, `1 0K` = 10240, `K1` = 1024, `iB5` = 5, `5i` = 5 are accepted; `1kb`, `1kib`, `5KIB` are refused (only an uppercase `B` is stripped) while `1kB` and `1Ki` are accepted. Affects `--min-size` and `--size`. |
| Reproduction | `d=$(mktemp -d); files_largest --json --min-size K1 "$d"` succeeds (1024-byte bound); `files_largest --json --min-size 1kb "$d"` gives `invalid_size` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the two-stage parser of the GNU path (numfmt `--from=iec` grammar emulated: decimals, K..E, round away from zero, then the fallback rules above), except the values whose meaning came from Bash arithmetic (forced divergence FS-22). |
| Fix recipe (3.0) | One native parser in `internal/fsscan` (size grammar): `^[0-9]{1,15}(\.[0-9]{1,6})?([KMGTPE](iB\|i\|B)?\|B)?$`, unit letters case-insensitive, no spaces, value = ceil(number × 1024^k), above 2^63-1 refused; failures keep `invalid_size` RETRY with each scanner's 2.x hint. AGENTS 3.2 and the helps state the grammar (FS-34). |
| Proving test | `TestLedger_FS11_2x` / `TestLedger_FS11_fix` in `internal/fsscan`, table-driven over `work/files-scanners/sizes.txt`: rc.A rows equal the numfmt-present 2.x column; 3.0 gives the numfmt column for valid rows and `invalid_size` for `1K2`, `K1`, `iB5`, `1 0K`, `5i`. |
| Status | accept: done d6a4bf9 |

<a id="fs-12"></a>
#### FS-12 · --exclude means three different things across the scanners

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N7) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:203-225` (find `( -path GLOB -prune ) -o`), `673-676`, `943-946` (du `--exclude=GLOB`), `721-724`, `730`, `743`, `983-986`, `990`, `1002` (awk: values joined with `\|`, `split(…, "\|")`, `index(p, value) > 0` on the absolute path) |
| Observed behavior | files_largest, dirs_list_empty, tree_stats: find `-path` glob against the whole absolute path, so `--exclude src` matches nothing while `*/src`, `*.log` or an absolute path work. dirs_largest, dirs_find_large: GNU du glob on trailing components (`src`, `s*c`, `*.log`) plus a plain substring test on the absolute path: `src` also hides `mysrcdir`, `src\|logs` is split in two, a substring of ROOT (`--exclude proj` scanning `…/proj`, `--exclude tmp` under /tmp) hides every row. The helps say "substring match" (FS-39, FS-40). |
| Reproduction | `d=$(mktemp -d)/proj; mkdir -p "$d/src" "$d/mysrcdir"; printf x >"$d/mysrcdir/f"; dirs_largest --json --exclude src "$d"; dirs_largest --json --exclude proj "$d"; files_largest --json --exclude src "$d"` (first hides `mysrcdir`, second gives count 0, third still lists `mysrcdir/f`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: three engines kept per scanner: find `-path` fnmatch without FNM_PATHNAME (`*` crosses `/`) on the absolute path; GNU du exclude matching; the awk substring test with `\|` splitting. |
| Fix recipe (3.0) | One rule in `internal/fsscan` for all five: GLOB matched with fnmatch semantics where `*` does not cross `/`, against the entry's base name and against its path relative to ROOT, never against ROOT's own prefix; no `\|` splitting; excluded entries are left out of every total. Help `--exclude` line for all five: "Skip entries whose name or path below PATH matches GLOB (repeatable)"; AGENTS 3.2 likewise. |
| Proving test | `TestLedger_FS12_2x` (rc.A matrix of `src`, `*/src`, absolute, `*.log`, `s*c`, `src\|logs`, `proj` equals the 2.x results of `work/files-scanners/repro-exclude.sh`) / `TestLedger_FS12_fix` (`src` hides `src` but not `mysrcdir` in all five; `proj` hides nothing; `src\|logs` hides nothing; identical across scanners) in `internal/fsscan`. |
| Status | accept: done d6a4bf9 |

<a id="fs-13"></a>
#### FS-13 · Default prunes apply to the scan root itself; --min-depth disables them

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N9) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:209-214` (prune expression evaluated for every entry, ROOT included), `668-671`, `938-941` (du `--exclude=NAME` also matches the operand), `1176` (`-mindepth`) |
| Observed behavior | Scanning a directory named `vendor`, `node_modules`, `.git`, `__pycache__` or `.cache` gives `count: 0` in all five (tree_stats: files 0, dirs 0, empty_dirs 1) unless `--no-ignore`. A FILE named `vendor` is listed (prunes test `-type d`). `dirs_list_empty --min-depth 2` lists `node_modules/emptyNM`, since find does not evaluate `-prune` above `-mindepth`. |
| Reproduction | `d=$(mktemp -d)/vendor; mkdir -p "$d"; printf x >"$d/f"; files_largest --json "$d"` gives `"count":0` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: prunes match the root, and in dirs_list_empty they are not applied above `--min-depth`. |
| Fix recipe (3.0) | In `internal/fsscan`: default prunes and excludes apply to entries at depth >= 1 only and always during the walk; `--min-depth` filters output only. Help of `--no-ignore` unchanged. |
| Proving test | `TestLedger_FS13_2x` / `TestLedger_FS13_fix` in `internal/fsscan`: scanning `D/vendor` lists its content in 3.0 (count 0 in rc.A); `dirs_list_empty --min-depth 2 D` omits `node_modules/*` in 3.0. |
| Status | accept: done d6a4bf9 |

<a id="fs-14"></a>
#### FS-14 · Walker failures on the GNU path are reported as an empty success

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N11, GNU-path part) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:484-492`, `699-701`, `966-968`, `1199-1201`, `1396-1398` (`… \|\| true`) |
| Observed behavior | When GNU find refuses its arguments (`-maxdepth`/`-mindepth` above 2147483647) or cannot list an execute-only ROOT (`cd` works, reading fails), the scanner returns rc 0 with `"data":[]` (tree_stats: all counters 0) and `errors` = the walker's complaint lines (1; dirs_list_empty 2); text mode says `Warning: Scan completed with N unreadable path(s) (permission denied).` |
| Reproduction | `d=$(mktemp -d); dirs_list_empty --json --depth 2147483648 "$d"` gives `{"data":[],"count":0,"path":"…","errors":1}`, rc 0 |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: find-backed depths above 2147483647 give the same empty envelope with `errors: 1`; an unreadable ROOT gives the empty result with the GNU line count (FS-15). The tool-variant cases are forced divergence FS-26. |
| Fix recipe (3.0) | FS-05 validation removes the argument cases. In `internal/fsscan`, a failure to read ROOT itself returns `path_unreadable` RETRY, details `Path 'INPUT' is not readable`, hint `Check directory permissions` (the existing `__gash_fs_safe_path` wording, `files.sh:174-175`), rc 1. |
| Proving test | `TestLedger_FS14_2x` / `TestLedger_FS14_fix` in `internal/fsscan`: `--depth 2147483648` gives the empty envelope in rc.A and `invalid_depth` in 3.0; a mode-0111 ROOT (unprivileged test user) gives `path_unreadable` in 3.0. |
| Status | accept: done d6a4bf9 |

<a id="fs-15"></a>
#### FS-15 · errors counts walker stderr lines, not unreadable entries

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N14) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:500`, `766`, `1040`, `1206`, `1406` (`wc -l` of the walker's stderr) |
| Observed behavior | One unreadable directory gives `errors: 1` in four scanners and 2 in dirs_list_empty (GNU find reports it for `-empty` and again for the descent); a symlink loop under `-L` counts 1; a newline inside a reported path adds a line (inferred). AGENTS 4.1 says "errors counts unreadable entries". |
| Reproduction | As an unprivileged user: `d=$(mktemp -d); mkdir "$d/x"; chmod 000 "$d/x"; dirs_list_empty --json "$d"; files_largest --json "$d"` gives `errors` 2 and 1 |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the native walker counts the lines GNU find/du would print: 1 per unreadable directory or loop, 2 per unreadable directory in dirs_list_empty, plus 1 per newline inside the reported path. |
| Fix recipe (3.0) | In `internal/fsscan`: `errors` = number of entries whose open or stat failed, each counted once, in all five scanners. AGENTS 4.1 sentence unchanged (it becomes true). |
| Proving test | `TestLedger_FS15_2x` (1 and 2 on the fixture) / `TestLedger_FS15_fix` (1 in all five) in `internal/fsscan`, unprivileged fixture as in `work/files-scanners/repro-errors.sh`. |
| Status | accept: done d6a4bf9 |

<a id="fs-16"></a>
#### FS-16 · tree_stats empty_dirs counts pruned and excluded entries

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N15) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1541-1548` (`find ROOT … PRUNES -type d -empty \| wc -l` with no explicit `-print`, so find's implicit `-print` also prints every pruned entry; no `-L`; stderr discarded) |
| Observed behavior | Every pruned default directory and every `--exclude` match (files included) is counted as an empty directory: a tree with `node_modules/p/i.js` and `src/a` and no empty dir reports `empty_dirs: 1` (dirs_list_empty says 0); adding `logs/x.log` and `--exclude '*.log'` gives 2. |
| Reproduction | `d=$(mktemp -d); mkdir -p "$d/node_modules/p" "$d/src"; printf x >"$d/node_modules/p/i.js"; printf x >"$d/src/a"; tree_stats --json "$d"` reports `"empty_dirs":1` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the empty-dir count emulates GNU find's implicit `-print` of prune matches (no `-L`, walk errors not counted). |
| Fix recipe (3.0) | In `internal/fsscan` tree_stats: count empty directories during the main walk, with the same prunes, excludes, `-L` and FS-01 predicate; `empty_dirs` then equals `dirs_list_empty --count` for the same flags. |
| Proving test | `TestLedger_FS16_2x` (1 and 2) / `TestLedger_FS16_fix` (0 and 0, and equality with `dirs_list_empty --count` on every dossier fixture) in `internal/fsscan`. |
| Status | accept: done d6a4bf9 |

<a id="fs-17"></a>
#### FS-17 · tree_stats extension containing a tab breaks the JSON and the text

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N16) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:1557`, `1569`, `1612`, `1621` (TSV rows from awk read with `IFS=$'\t' read -r ext cnt sz`) |
| Observed behavior | A file `f.x<TAB>y` yields `{"ext":"x","count":y,"size":1<TAB>1}` in top_by_count (invalid JSON) and a text row `x  y files  0 B`. |
| Reproduction | `d=$(mktemp -d); printf x >"$d/$(printf 'f.x\ty')"; tree_stats --json "$d" \| jq .` fails with a parse error |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced byte for byte: the extension is split at its first tab into ext, count and size exactly as the `read` did, in JSON and text. |
| Fix recipe (3.0) | FS-01 predicate removes tab names; independently `internal/fsscan` carries the extension as one value and encodes it with the contract escaper. |
| Proving test | `TestLedger_FS17_2x` (rc.A bytes equal the 2.x golden) / `TestLedger_FS17_fix` (`jq -e .` passes, no `x` extension) in `internal/fsscan`. |
| Status | accept: done d6a4bf9 |

<a id="fs-18"></a>
#### FS-18 · dirs_list_empty prints one blank line when nothing is found

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N18) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:1251` (`printf '%s\n' "$output"` with an empty `$output`) |
| Observed behavior | Text mode with no empty directory prints `\n`; the size lists print nothing; AGENTS 7 promises "no lines" for empty results. |
| Reproduction | `d=$(mktemp -d); printf x >"$d/f"; dirs_list_empty "$d" \| od -c` shows `\n` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced (pinned by `output_style_render_spec.sh` fixture `dirs_list_empty:none`). |
| Fix recipe (3.0) | In `internal/fsscan` dirs_list_empty text mode: print nothing when the list is empty; update the render fixture. |
| Proving test | `TestLedger_FS18_2x` (stdout is exactly `\n`) / `TestLedger_FS18_fix` (stdout empty) in `internal/fsscan`. |
| Status | accept: done d6a4bf9 |

<a id="fs-19"></a>
#### FS-19 · The --json pre-scan also matches a flag value

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N19) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:328-334`, and the same loop at `547-553`, `813-819`, `1083-1089`, `1281-1287` |
| Observed behavior | Before parsing, any word equal to `--json` ahead of `--` turns on JSON mode, even when it is the value of a value flag: `files_largest --exclude --json D` uses `--json` as the exclude glob and also emits JSON. |
| Reproduction | `d=$(mktemp -d); printf x >"$d/f"; files_largest --exclude --json "$d"` prints a JSON envelope |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced (the scanner grammar pre-scans every word). |
| Fix recipe (3.0) | In `internal/cli/args` (scanners family): the pre-scan skips the word after each value flag (`-n`, `--limit`, `--min-size`, `--size`, `--exclude`, `--depth`, `--max-depth`, `--min-depth`, `--top`), so errors met before a real `--json` still get JSON. |
| Proving test | `TestLedger_FS19_2x` (JSON output) / `TestLedger_FS19_fix` (text output, `--json` used only as the glob) in `internal/cli/args`. |
| Status | accept: done d6a4bf9 |

### archive_extract and file_backup

<a id="file-01"></a>
#### FILE-01 · `.Z` archives are emptied by archive_extract

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (S1 audit R1, `.Z` archive truncated by archive_extract) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1919,1946` |
| Observed behavior | `shopt -s nocasematch` (1919) routes `*.Z`, the real compress suffix, and `.tar.Z` to the `*.z` branch, but `basename F .z` strips only a lowercase `.z`, so the output name equals the archive's. With OUTPUT_DIR = the archive's directory (the default `.` for an archive in cwd) the redirection truncates the archive before `uncompress` reads it: `gzip: foo.Z: unexpected end of file`, rc 1, archive 0 bytes. Same with a real `compress` file and when `uncompress` is missing (the redirection runs first). With another OUTPUT_DIR the data is decompressed but keeps the `.Z` name. |
| Reproduction | `R 'printf "hi\n" \| gzip > foo.Z; archive_extract foo.Z; echo rc=$?; wc -c < foo.Z'` → `gzip: foo.Z: unexpected end of file`, `rc=1`, `0` |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: suffixes matched case-insensitively in the 2.x order, the output name computed with `basename`'s case-sensitive suffix rule, `OUTPUT_DIR/foo.Z` opened with `O_TRUNC` before `uncompress foo.Z -c` starts, so the archive is emptied as in 2.x |
| Fix recipe (3.0) | In `internal/files`, `singleFileTarget()`: strip the matched suffix case-insensitively (`strings.EqualFold` on the last `len(suffix)` bytes of the base name) and only when something remains; before opening, refuse when the remainder is empty or the target is the archive itself (`os.SameFile` on both `os.Stat` results, so through symlinks too): `Error: Output 'P' would overwrite the archive 'F'.`, rc 1, nothing opened. Route `*.tar.z` and `*.taz` before `*.z` (FILE-13). Use `uncompress -c -- F` (FILE-14). Help NOTES (FILE-26). |
| Proving test | `TestLedger_FILE01_fix` in `internal/files`: in a temp dir, `foo.Z` (gzip output) and `real.Z` (compress output, skipped without ncompress) extracted with OUTPUT_DIR `.`: archive bytes unchanged, `foo` and `real` hold the payload, rc 0. `TestLedger_FILE01_2x`: 0-byte archive, rc 1. `testdata/script/archive_extract_compress.txtar` runs both through the shim |
| Status | accept: done a2a30ac |

<a id="file-02"></a>
#### FILE-02 · Upper- and mixed-case suffixes (`.GZ`, `.XZ`, `.Bz2`, `.ZST`) empty the archive; a file named `.gz` empties itself

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N1) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1919,1931,1934,1939,1941` |
| Observed behavior | Dispatch is case-insensitive, the `basename F .gz`/`.xz`/`.bz2`/`.zst` strip is not, so `UP.GZ`, `BIG.XZ`, `Cap.Bz2`, `S.ZST` get an output name equal to the archive: in the archive's directory it is truncated before the decompressor reads it (observed 31→0, 64→0, 44→0 bytes), rc 1. A file named exactly `.gz` (or `.xz`, `.bz2`, `.zst`) is truncated the same way, since `basename` never strips the whole name. With another OUTPUT_DIR the output keeps the compressed suffix (`out/UP.GZ` holds plain text). |
| Reproduction | `R 'printf "hi\n" \| gzip > UP.GZ; archive_extract UP.GZ; echo rc=$?; wc -c < UP.GZ'` → `gzip: UP.GZ: unexpected end of file`, `rc=1`, `0` |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced (the FILE-01 name rule and open-and-truncate before the tool) |
| Fix recipe (3.0) | The FILE-01 change in `singleFileTarget()`: case-insensitive strip (`UP.GZ` → `UP`, `Cap.Bz2` → `Cap`, `S.ZST` → `S`) and the refusal `Error: Output 'P' would overwrite the archive 'F'.` rc 1 for a name that would stay equal to the archive (`.gz`, `.xz`, `.bz2`, `.zst`, `.z` alone). The help's "(case-insensitive)" becomes true (FILE-26) |
| Proving test | `TestLedger_FILE02_fix` in `internal/files`: table `UP.GZ BIG.XZ Cap.Bz2 S.ZST .gz` run in the archive's directory: archive bytes unchanged; `UP`, `BIG`, `Cap`, `S` hold the payload, rc 0; `.gz` gives the refusal line, rc 1, `.gz` unchanged. `TestLedger_FILE02_2x`: 0 bytes, rc 1 |
| Status | accept: done a2a30ac |

<a id="file-03"></a>
#### FILE-03 · Single-file formats truncate the output before the tool runs: a corrupt archive or a missing tool destroys an existing file

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N2) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1931,1939,1941,1946` (the `>` redirection; `1934` for zst runs only after its zstd guard) |
| Observed behavior | The shell opens `OUTPUT_DIR/<name>` with `O_TRUNC` before the tool starts. A corrupt input, or a missing `gunzip`, `bunzip2`, `xz` or `uncompress`, leaves an empty file and destroys a pre-existing file of that name, rc 1. An existing symlink at that path is followed and its target rewritten (`out/s.txt -> ../victim/s.txt`: `victim/s.txt` replaced, rc 0). zstd is the exception for the missing-tool case. |
| Reproduction | `R 'printf "not gzip" > s.txt.gz; printf "PRECIOUS\n" > s.txt; archive_extract s.txt.gz; echo rc=$?; wc -c < s.txt'` → `gzip: s.txt.gz: not in gzip format`, `rc=1`, `0` |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: open `O_WRONLY\|O_CREAT\|O_TRUNC`, 0666 (umask applied by the kernel), following symlinks, then start the tool with that file as stdout; a missing tool is reported after the open (FILE-19), so the empty file stays |
| Fix recipe (3.0) | In `internal/files`, single-file branch: (1) resolve the tool first (`exec.LookPath`); missing → `Error: Command 'gunzip' is not installed or not in PATH.` (the `__gash_require_command` default, `lib/core/validation.sh:29`), rc 1, nothing created; (2) decompress into a new file in OUTPUT_DIR opened with `O_WRONLY\|O_CREAT\|O_EXCL`, mode 0666, random name `.axe-XXXXXX` (kernel umask keeps the 2.x mode); (3) on tool success `os.Rename` it over the target (a symlink at the target is replaced, never written through); on failure remove it. An existing target is still replaced on success (2.x contract) |
| Proving test | `TestLedger_FILE03_fix` in `internal/files`: corrupt `s.txt.gz` keeps `s.txt` = `PRECIOUS`, rc 1, no `.axe-*` left; PATH without gunzip → the error line, no `s.txt`; symlink `out/s.txt -> ../victim/s.txt` leaves `victim/s.txt` unchanged and `out/s.txt` a regular file; a good archive still replaces `s.txt`, rc 0. `TestLedger_FILE03_2x`: the empty file |
| Status | accept: done a2a30ac |

<a id="file-04"></a>
#### FILE-04 · file_backup: two backups in the same second (or in a DST fall-back hour) overwrite each other

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N9) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1973-1974` |
| Observed behavior | The name has second resolution, local time, no zone and no counter. An existing regular file of that name is truncated and rewritten in place by `cp` (it keeps its own mode), rc 0, silently. Two calls within one second leave one backup holding the second content. In a DST fall-back hour two instants one hour apart get the same name (`TZ=America/New_York date -d @1730611800` and `@1730615400` both print `20241103013000`). |
| Reproduction | `R 'printf 1 > a; file_backup a >/dev/null; printf 2 > a; file_backup a >/dev/null; ls \| grep -c a_backup_; cat a_backup_*'` → `1`, `2` (the first backup is gone; both calls fall in one second) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: same name, same `cp -v -- FILE BACKUP` |
| Fix recipe (3.0) | In `internal/files`, `fileBackup()`: reserve the name atomically: `os.OpenFile(name, O_WRONLY\|O_CREAT\|O_EXCL\|O_NOFOLLOW, srcMode&0777)` (kernel umask gives the mode cp would give a new file); on `EEXIST` retry with `_1`, `_2`, ... appended to the stamp; then run `cp -v -- FILE NAME`, which rewrites the reserved file in place. Help NOTES: "a backup never replaces an existing file; a second backup in the same second gets `_1`" (FILE-27) |
| Proving test | `TestLedger_FILE04_fix` in `internal/files`, frozen stamp (`clock.Fake`, or a PATH `date` stub if FILE-25 keeps the CLI): three calls leave `a_backup_S`, `a_backup_S_1`, `a_backup_S_2` with the three contents, rc 0 each; the two New York instants give two files. `TestLedger_FILE04_2x`: one file |
| Status | accept: done a6a47ca |

<a id="file-05"></a>
#### FILE-05 · file_backup writes through an existing symlink or hard link, or copies into a directory, at the backup name

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N9) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1973-1979` |
| Observed behavior | Nothing checks that BACKUP is free. A symlink there is followed and its target's content replaced, rc 0; a hard link to another file: that file's content replaced, rc 0; a directory: cp copies into it (`'a' -> 'a_backup_…/a'`), then `Error: Failed to create backup file.`, rc 1, the copy left inside; a dangling symlink: `cp: not writing through dangling symlink 'a_backup_…'` plus the Error line, rc 1. |
| Reproduction | `R 'printf NEW > a; printf OLD > v; now=$(date +%s); for s in 0 1 2; do ln -s v "a_backup_$(date -d @$((now+s)) +%Y%m%d%H%M%S)"; done; file_backup a >/dev/null; echo rc=$?; cat v'` → `rc=0`, `NEW` (names pre-created for three consecutive seconds from one base, so the run cannot miss them; `v` is a throwaway file) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: cp runs with the 2.x argv and applies its own write-through, into-directory and dangling-link rules |
| Fix recipe (3.0) | The FILE-04 reservation: `O_CREAT\|O_EXCL\|O_NOFOLLOW` fails with `EEXIST` on any existing entry (symlink, dangling symlink, hard link, directory), so the next suffix is tried and the existing entry is never opened |
| Proving test | `TestLedger_FILE05_fix` in `internal/files`: with a symlink to `victim`, a hard link to `other`, a directory and a dangling symlink pre-created at the frozen-stamp name (one per case), `victim`, `other` and the directory are unchanged and the backup is `a_backup_S_1`, rc 0. `TestLedger_FILE05_2x`: `victim` rewritten |
| Status | accept: done a6a47ca |

<a id="file-06"></a>
#### FILE-06 · file_backup ignores cp's exit status

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N10) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1974-1979` |
| Observed behavior | Success is inferred from `[ -f "$backup" ]` after `cp -v`; cp's status is never read. When a backup of the same second already exists, a failing copy (I/O error, a full disk mid-copy, permission) still returns 0, so the user believes the current content was saved; a copy that fails partway leaves a partial regular file that passes the check too (inferred). |
| Reproduction | `R 'printf x > a; file_backup a >/dev/null; cp(){ return 1; }; file_backup a; echo rc=$?'` → `rc=0` (the shell function stands in for a failing cp) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: Go runs cp, ignores its status and returns 1 only when BACKUP is not a regular file afterwards (`os.Stat`, following symlinks) |
| Fix recipe (3.0) | In `internal/files`, `fileBackup()`: a non-zero cp status → `Error: Failed to create backup file.` (2.x text), rc 1, and remove the file reserved by FILE-04 so no partial backup remains; keep the regular-file check after a zero status |
| Proving test | `TestLedger_FILE06_fix` in `internal/files` with `runner.Fake` making cp write one byte and exit 1: rc 1, the Error line, no backup file. `testdata/script/file_backup_cp_fail.txtar` with a PATH `cp` stub exiting 1 after an earlier backup of the same (frozen) second |
| Status | accept: done a6a47ca |

<a id="file-07"></a>
#### FILE-07 · OUTPUT_DIR is created before the format and its tool are checked

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N3) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1904-1909` (before the dispatch at `1920-1951`) |
| Observed behavior | `mkdir -p OUTPUT_DIR` runs first, so an unsupported type, a missing zstd, a missing tool or a corrupt archive leave a new empty directory behind, rc 1 |
| Reproduction | `R 'printf x > f.abc; archive_extract f.abc newdir; echo rc=$?; ls -d newdir'` → `Error: Cannot extract 'f.abc', unsupported file type.`, `rc=1`, `newdir` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: `mkdir -p` with the 2.x argv before the dispatch |
| Fix recipe (3.0) | In `internal/files`, `archiveExtract()`: pick the branch from the suffix table and resolve its tools first (FILE-03, FILE-19); an unsupported type gives the 2.x error with no directory created; only then run `mkdir -p -- DIR` (FILE-08) |
| Proving test | `TestLedger_FILE07_fix` in `internal/files`: unsupported type, PATH without zstd for `.tar.zst`, PATH without unzip for `.zip`: rc 1 and no `newdir` |
| Status | accept: done a2a30ac |

<a id="file-08"></a>
#### FILE-08 · An option-like OUTPUT_DIR reaches `mkdir` without `--`

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N4) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1905` |
| Observed behavior | A missing `-x` → `mkdir: invalid option -- 'x'`, `Try 'mkdir --help' for more information.`, `Error: Failed to create output directory '-x'.`, rc 1; `-v` → `mkdir: missing operand`; `--help` → mkdir's full help on stdout, mkdir exits 0, then `tar: --help: Cannot open: No such file or directory`, rc 1 (`--version` likewise). Help is recognized only as `$1`, so `archive_extract a.tgz --help` takes this path. An existing directory `-x` works for tar, unzip, 7z and the single-file formats (unrar: FILE-09). |
| Reproduction | `R 'printf x > s; tar czf a.tgz s; archive_extract a.tgz --help \| head -1'` → `Usage: mkdir [OPTION]... DIRECTORY...` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: Go runs `mkdir -p OUTPUT_DIR` with the 2.x argv, so the lines and statuses come from mkdir itself |
| Fix recipe (3.0) | In `archiveExtract()`: run `mkdir -p -- DIR` (mkdir's own error lines are kept), and prefix `./` to an OUTPUT_DIR that starts with `-` before every later use, as `files.sh:1913` does for the archive name |
| Proving test | `TestLedger_FILE08_fix` in `internal/files`: `archive_extract a.tgz --help` creates `--help/s`, prints no mkdir help, rc 0; `-x` creates `-x/s`, rc 0 |
| Status | accept: done a2a30ac |

<a id="file-09"></a>
#### FILE-09 · unrar reads a dash-named OUTPUT_DIR as a switch

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N4) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1940` |
| Observed behavior | `unrar x F OUT` has no `--`: with an existing directory `-x`, `archive_extract plain.rar -x` extracts into the cwd (`./src/a.txt`), rc 0. A directory named like a real switch changes unrar's behavior (`-o+` means overwrite all; inferred from unrar's switch list, not run). |
| Reproduction | `R "mkdir -- -x; cp $W/evil/arch/plain.rar .; archive_extract plain.rar -x; echo rc=\$?; ls src"` → `rc=0`, `a.txt` (in cwd, not in `-x/`) |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Reproduced (same argv) |
| Fix recipe (3.0) | `unrar x -- F DIR/`: `-` stops switch scanning (listed in UnRAR 7.00's usage text), the trailing `/` marks the destination, and DIR gets the FILE-08 `./` prefix |
| Proving test | `TestLedger_FILE09_fix` in `internal/files` (skips without unrar): existing `-x`: `-x/src/a.txt` written, no `src/` in cwd, rc 0 |
| Status | accept: done a2a30ac |

<a id="file-10"></a>
#### FILE-10 · unzip, 7z and unrar expand wildcards in the archive name

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N5) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1940,1945,1947` |
| Observed behavior | With `x?.zip` and `xb.zip` present, `archive_extract 'x?.zip' out` extracts both (`2 archives were successfully processed.`), rc 0; same for `.7z` and `.rar`. With `p[1].zip` and `p1.zip` present, `archive_extract 'p[1].zip' o` extracts only `p1.zip` (`Archive:  p1.zip`): the named archive is not extracted at all (checked during bookkeeping). `@name` is not taken as a list file (7z, unrar). |
| Reproduction | `R "cp '$d/p[1].zip' $d/p1.zip .; archive_extract 'p[1].zip' o \| head -1; ls o"` (`$d`: a scratch dir with two throwaway zips holding `A/a` and `Bd/b`) → `Archive:  p1.zip`, `Bd` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced (same argv) |
| Fix recipe (3.0) | 7z: add the literal-name switch `-spd` (`7z x -spd -oDIR -- F`; verified with 7-Zip 23.01: `y?.7z` alone extracted with `yb.7z` present). unzip has no literal mode and ignores backslash escapes; unrar has no such switch and ignores backslash escapes (both verified): when the base name contains `*`, `?` or `[`, refuse before creating anything: `Error: Archive name 'X' contains wildcard characters (* ? [) that unzip would expand; rename it.` (tool name per branch), rc 1. Alternative for the operator: run the tool on a private symlink with a plain name inside an `os.MkdirTemp` dir (names keep working; the banner shows the temporary path) |
| Proving test | `TestLedger_FILE10_fix` in `internal/files`: `p[1].zip` + `p1.zip` → the refusal, nothing extracted, rc 1; `y?.7z` + `yb.7z` → only `y?.7z`'s content, rc 0 |
| Status | accept: done a2a30ac |

<a id="file-11"></a>
#### FILE-11 · unzip, unrar and 7z prompt and read stdin in headless mode

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N6) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:1940,1945,1947` |
| Observed behavior | Overwrite prompts come from the tools and ignore `GASH_HEADLESS`, the TTY state and Gash's confirm primitive. With an existing `src/a.txt` and stdin `/dev/null`: unzip `replace ./src/a.txt? [y]es, [n]o, [A]ll, [N]one, [r]ename:` then `(EOF or read error, treating as "[N]one" ...)`, rc 1; unrar `Program aborted`, rc 1; 7z `Break signaled`, rc 1. A piped stdin is consumed as answers (`printf 'A\n' \| archive_extract a.zip` overwrites). Password prompts too (bookkeeping check): an encrypted `.7z` reads the password from stdin (`printf 'X\n' \| archive_extract enc.7z p` extracted, rc 0); unzip asks on `/dev/tty` (`unable to get password` without a controlling terminal, a blocking prompt with one). Headless on a terminal, the call blocks. |
| Reproduction | `R "mkdir src; printf P > src/a.txt; cp $W/fx1/a.zip .; archive_extract a.zip </dev/null; echo rc=\$?"` → the unzip prompt and EOF lines on stderr, `rc=1`, `src/a.txt` unchanged |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: the binary inherits the caller's stdin and terminal (`compat.md` 3), so the tools prompt and read exactly as in 2.x |
| Fix recipe (3.0) | Never prompt in headless (PRD 6.2 spirit, MUST NOT 15). In `archiveExtract()`, when `GASH_HEADLESS=1` or stdin is not a TTY: start unzip, unrar and 7z with stdin `/dev/null` and `SysProcAttr{Setsid: true}` (no controlling terminal, so no `/dev/tty` prompt), plus the never-overwrite switches `unzip -n`, `unrar x -o- -p-` (`-p-`: do not query a password, from RAR's documentation, not run), `7z x -aos`; an encrypted archive then fails at once with the tool's error (7z prints its password prompt and stops on EOF; documented in the help NOTES, FILE-26). On a TTY outside headless the prompts stay |
| Proving test | `TestLedger_FILE11_fix` in `internal/files` (`runner.Fake`: headless argv carries the switches, stdin is `/dev/null`, `Setsid` set). `testdata/script/archive_extract_headless.txtar` under a PTY with `GASH_HEADLESS=1`: existing file unchanged, no prompt text from unzip, return within 2 s |
| Status | accept: done a2a30ac |

<a id="file-12"></a>
#### FILE-12 · "File 'X' does not exist." for existing non-regular paths (both functions)

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N11) |
| Area | Correctness |
| Bash location | `lib/core/validation.sh:78-84` via `lib/modules/files.sh:1902,1970` |
| Observed behavior | `[[ -f ]]` rejects directories, FIFOs, sockets, devices and dangling symlinks with the "does not exist" text, rc 1; `file_backup f/` (trailing slash) too. The paths exist, so the message misleads |
| Reproduction | `R 'mkdir d.zip; archive_extract d.zip; mkdir d; file_backup d'` → `Error: File 'd.zip' does not exist.`, `Error: File 'd' does not exist.`, rc 1 each |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced (same text from the Go port of `__gash_require_file`) |
| Fix recipe (3.0) | In `internal/files`, before the shared check: `os.Stat` succeeds and the mode is not regular, or `os.Lstat` finds a dangling symlink → `Error: 'X' is not a regular file.`, rc 1. The shared helper keeps its text for other callers |
| Proving test | `TestLedger_FILE12_fix` in `internal/files`: directory, FIFO and dangling symlink give the new text for both functions; a missing path keeps "does not exist" |
| Status | accept: done a2a30ac |

<a id="file-13"></a>
#### FILE-13 · `.tar.Z` is decompressed, not unpacked; `.taz`, `.tar.lz`, `.tlz`, `.tar.lzma`, `.lz4`, `.tbz` are unsupported

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (dossier D-A8) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1920-1951` |
| Observed behavior | `.tar.Z` falls to the `*.z` branch: in another directory the output is `o/a.tar.Z`, a plain tar file keeping the `.Z` name (rc 0); in the archive's directory the archive is emptied (FILE-01). `.taz`, `.tar.lz`, `.tlz`, `.tar.lzma`, `.lz4`, `.tbz`, `.tb2` give `Cannot extract 'X', unsupported file type.`, rc 1 |
| Reproduction | `R "mkdir o; cp $W/fx1/a.tar.Z .; archive_extract a.tar.Z o; echo rc=\$?; file o/*"` → `rc=0`, `o/a.tar.Z: POSIX tar archive (GNU)` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced |
| Fix recipe (3.0) | Before `*.z` in the suffix table: `*.tar.z` and `*.taz` → `uncompress -c -- F \| tar xvf - -C DIR --no-same-owner --no-same-permissions`, rc 1 when either side fails (FILE-17). Not `tar xvZf`: it needs ncompress's `compress` (verified: `tar (child): compress: Cannot exec`), while gzip's `uncompress` is always there on Linux. New formats (`.tar.lz`/`.tlz` via `tar --lzip`, `.tar.lzma` via `tar --lzma`, `.tbz` as `.tbz2`) only on operator request, with the help updated |
| Proving test | `TestLedger_FILE13_fix` in `internal/files`: `a.tar.Z` and `a.taz` → `o/src/a.txt`, rc 0, archive unchanged |
| Status | accept: done a2a30ac |

<a id="file-14"></a>
#### FILE-14 · `uncompress F -c` puts `-c` after the operand

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (dossier format table note; BSD case inferred) |
| Area | Portability |
| Bash location | `lib/modules/files.sh:1946` |
| Observed behavior | GNU getopt (gzip's `uncompress` wrapper on Linux) permutes `-c` and the branch works. A BSD-getopt `uncompress` (macOS) stops at the first operand, so it would decompress F in place (removing F, writing F minus `.Z`) and take `-c` as a second file, with nothing on stdout (inferred, no macOS host) |
| Reproduction | Linux shows only the working case: `R 'printf "hi\n" \| gzip > s.z; archive_extract s.z o; echo rc=$?; cat o/s'` → `rc=0`, `hi` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: argv `uncompress F -c` on every OS |
| Fix recipe (3.0) | `uncompress -c -- F` in the `*.z` branch and in the FILE-13 pipeline |
| Proving test | `TestLedger_FILE14_fix` in `internal/files` (`runner.Fake` asserts argv `uncompress -c -- F`); macOS CI run of `testdata/script/archive_extract_compress.txtar`: `s.z` kept, `o/s` written |
| Status | accept: done a2a30ac |

<a id="file-15"></a>
#### FILE-15 · A trailing newline in the archive name is dropped from the output name

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (dossier A-11) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1931,1934,1939,1941,1946` (`$(basename ...)`) |
| Observed behavior | Command substitution strips trailing newlines, so `$'foo\n.gz'` is written to `OUTPUT_DIR/foo`; an existing `foo` is overwritten |
| Reproduction | `R 'n=$(printf "foo\n.gz"); printf "hi\n" \| gzip > "$n"; archive_extract "$n"; echo rc=$?; cat foo'` → `rc=0`, `hi` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: Go strips trailing `\n` bytes from the computed name, as `$(...)` does. The dossier listed it as a forced-divergence candidate; it is a pure string rule Go can reproduce, so it stays a defect |
| Fix recipe (3.0) | In `singleFileTarget()`: keep the exact base name minus the suffix, no newline stripping |
| Proving test | `TestLedger_FILE15_fix` in `internal/files`: `$'foo\n.gz'` → `$'foo\n'` holds the payload, an existing `foo` untouched |
| Status | accept: done a2a30ac |

### shell layer

<a id="shell-02"></a>
#### SHELL-02 · The terminal title writes the raw working directory, USER and HOSTNAME into an OSC sequence

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (this fragment; companion of N1) |
| Area | Security |
| Bash location | `lib/shell/prompt.sh:234-245` (`__gash_set_terminal_title`, `printf '\e]2;%s\a'` at `:241` and `:243`) |
| Observed behavior | The title is `printf '\e]2;%s\a' "${cwd} [${USER}]"` (or `USER@HOSTNAME:cwd` over SSH) with no filtering, so a directory name holding BEL or ESC ends the title sequence early and the rest of the name reaches the terminal as escape sequences (title spoofing, and whatever the terminal emulator accepts after `ESC ]` or `ESC [`). Nothing is executed by the shell; the bytes go to the terminal. |
| Reproduction | `$E bash --noprofile --norc -i -c 'source $C/gash.sh; d=$HOME/d$(printf "\a\e]0;X\a"); mkdir "$d"; cd "$d"; __gash_set_terminal_title \| od -c' </dev/null` → `033 ] 2 ; ~ / d \a 033 ] 0 ; X \a   [ ] \a` (a second, attacker-chosen title sequence) |
| Severity | medium |
| Public-contract impact | no (ordinary names give the same bytes) |
| rc.A behavior | Reproduced: the hook emitted by `gash init bash` prints the title from the same values with the same `printf`, bytes unchanged (the prompt stays a Bash hook in 3.x). |
| Fix recipe (3.0) | In the `internal/shellinit` hook template, before the `printf`, replace control characters in each part: `local t="${cwd} [${USER-}]"; t=${t//[[:cntrl:]]/?}` (same for the SSH form), so C0 bytes, DEL and, in a UTF-8 locale, U+0080-U+009F become `?`. No new error; the title is cosmetic. Note in `docs/configuration.md` (Prompt) that control characters in the title are shown as `?`. |
| Proving test | `TestLedger_SHELL02` in `internal/shellinit` (template sanitizes every title part); `testdata/script/prompt_title_controls.txtar` (cwd holding `\a` and `\e`: the title output is exactly one `\e]2;...\a` frame with `?` in place of each control; an ordinary cwd gives the 2.x bytes) |
| Status | accept: done 3e2b9ab |

<a id="shell-03"></a>
#### SHELL-03 · Completion offers `gash help`, which fails

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "completion offers a help topic that errors" (CONFIRMED) |
| Area | Completion |
| Bash location | `bash_completion:96` (`topics="all help"`) vs `lib/modules/gash.sh:840-852` (topic lookup, error at `:850`) |
| Observed behavior | TAB after `gash ` offers `all help files system nav git docker remote dev llm ai gash`; `gash help` prints `Error: Unknown topic: help. Topics: files system nav git docker remote dev llm ai gash, all` and returns 1. `completion_spec.sh` passes anyway. |
| Reproduction | `$E bash --noprofile --norc -i -c 'source $C/gash.sh; source $C/bash_completion; COMP_WORDS=(gash ""); COMP_CWORD=1; __gash_complete; echo "${COMPREPLY[*]}"; gash help; echo rc=$?' </dev/null` |
| Severity | low |
| Public-contract impact | no (completion candidates are not a documented output; `gash help` keeps its error) |
| rc.A behavior | Reproduced: the generated `bash_completion` keeps `help` among the candidates, and `gash help` keeps the error text and rc 1. |
| Fix recipe (3.0) | `internal/compatgen` builds the candidate list from the card's own topic table in `internal/registry` plus `all` and `migrate` (the subcommands `gash` accepts), never a literal list; drop `help`. `gash help` keeps its error (users have `gash_help`). |
| Proving test | `TestLedger_SHELL03` in `internal/compatgen` (each completion candidate for `gash` runs with rc 0 under a fake terminal; `help` is not a candidate) |
| Status | accept: done 8128400 |

<a id="shell-04"></a>
#### SHELL-04 · GASH_HISTORY overwrites the user's HISTCONTROL, while the migration says it is respected

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "history settings overwrite the user's HISTCONTROL, contrary to the migration's claim" (CONFIRMED) |
| Area | Correctness |
| Bash location | `lib/shell/history.sh:14-20` (`HISTCONTROL=ignoreboth` at `:15`); claim at `lib/modules/migrate.sh:671` (`Your EDITOR, HISTCONTROL and PROMPT_COMMAND are respected.`) |
| Observed behavior | With `HISTCONTROL=erasedups HISTSIZE=5000` set before the Gash block, after load the values are `ignoreboth -1 -1`. EDITOR (set only when empty) and PROMPT_COMMAND (prepended) are respected, HISTCONTROL is not. `docs/configuration.md:180` and `.gash_env.template:104` document `ignoreboth`. |
| Reproduction | `$E HISTCONTROL=erasedups HISTSIZE=5000 bash --noprofile --norc -i -c 'source $C/gash.sh; echo "$HISTCONTROL $HISTSIZE $HISTFILESIZE"' </dev/null` → `ignoreboth -1 -1` |
| Severity | medium |
| Public-contract impact | yes (documented history setting) |
| rc.A behavior | Reproduced: the history block emitted by `gash init bash` assigns the same four values unconditionally when `GASH_HISTORY` is not `0`, and the migration text is unchanged. |
| Fix recipe (3.0) | In the `internal/shellinit` history block: keep a non-empty HISTCONTROL and add `ignoreboth` only when none of `ignorespace`, `ignoredups`, `ignoreboth` is present (`case ":$HISTCONTROL:" in *:ignore*) ;; *) HISTCONTROL=${HISTCONTROL:+$HISTCONTROL:}ignoreboth ;; esac`); keep `HISTSIZE=-1`, `HISTFILESIZE=-1` (documented). Update `docs/configuration.md:180` and `.gash_env.template:104`: "adds ignoreboth unless you set your own HISTCONTROL". |
| Proving test | `TestLedger_SHELL04` in `internal/shellinit`; `testdata/script/history_respects_histcontrol.txtar` (`HISTCONTROL=erasedups` before load → `erasedups:ignoreboth`; `ignoredups` → unchanged; unset → `ignoreboth`) |
| Status | accept: done dc77d5b |

<a id="shell-06"></a>
#### SHELL-06 · The `grep` color alias is never defined

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N2) |
| Area | Correctness |
| Bash location | `lib/aliases/colors.sh:43` (probe `command grep --color=auto -q "" /dev/null`) |
| Observed behavior | The probe reads an empty file, so grep exits 1 on every system and `alias grep='grep --color=auto'` is never defined (`alias grep` → `bash: alias: grep: not found` with GNU grep 3.11). `docs/reference/system.md:617` and `docs/configuration.md:50` list the alias. |
| Reproduction | `$E bash --noprofile --norc -i -c 'source $C/gash.sh; alias grep' </dev/null` → `bash: alias: grep: not found` |
| Severity | low |
| Public-contract impact | yes (an alias appears after the fix) |
| rc.A behavior | Reproduced: the alias pack evaluated by the binary keeps the 2.x outcome (no `grep` alias), and the generated reference keeps listing it. |
| Fix recipe (3.0) | In the `internal/registry` alias pack, define `grep` when `grep --color=auto -q x` succeeds on the input `x` (Go: run once at `gash init bash` time, no cache); docs already match the fix. |
| Proving test | `TestLedger_SHELL06` in `internal/shellinit` (fake GNU grep that honors `--color`: init output contains `alias grep='grep --color=auto'`; a grep that rejects the flag: no alias) |
| Status | accept: done 582e82b |

<a id="shell-08"></a>
#### SHELL-08 · The first word of GASH_PATH_ADD is taken as a path_add option

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (this fragment) |
| Area | Correctness |
| Bash location | `lib/shell/toolchains.sh:16-19` (`path_add ${GASH_PATH_ADD}`), `lib/core/utils.sh:329-358` (`path_add` parses `--append`, `-h`, `--help`) |
| Observed behavior | `GASH_PATH_ADD='--append /opt/x'` appends instead of prepending; `GASH_PATH_ADD='-h /opt/x'` prints the 23-line `path_add` help at every shell start (on stdout, in an interactive shell) and adds nothing. |
| Reproduction | `$E GASH_PATH_ADD='-h /opt/x' bash --noprofile --norc -i -c 'source $C/gash.sh' </dev/null 2>/dev/null \| head -2` → `  path_add  (module: utils)` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: the init layer passes the split words (SHELL-07) to the same option parsing, so `--append` appends and `-h` prints the help. |
| Fix recipe (3.0) | `internal/shellinit` adds the directories with an option-less helper (`__gash_path_prepend DIR...`, same once-only rule as `path_add`); a word that starts with `-` is a directory name like any other (it does not exist, so it is skipped). `docs/configuration.md` (GASH_PATH_ADD): "directories only, no options". |
| Proving test | `TestLedger_SHELL08` in `internal/shellinit` (`GASH_PATH_ADD='-h /opt/x'` emits no help text and adds `/opt/x` if it exists) |
| Status | accept: done f40af4e |

<a id="shell-09"></a>
#### SHELL-09 · Gash overwrites aliases defined before it, and unload does not restore them

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N6) |
| Area | Correctness |
| Bash location | `lib/aliases/navigation.sh:21` and the other unconditional `alias` lines; `lib/aliases/_loader.sh:131,155` (pre-existing names only skip the shadow rule); `lib/modules/gash.sh:426-429` (unload keeps PRE names without their values); doc `docs/configuration.md:51-52` |
| Observed behavior | Ubuntu's skel `.bashrc` defines `ll='ls -alF'`; after load it is `ls -l --color=auto`, and after `gash_unload` it is still Gash's value. The docs say an alias "you defined before Gash loaded" stays. |
| Reproduction | `$E bash --noprofile --norc -i -c 'alias ll="ls -alF"; source $C/gash.sh; alias ll; gash_unload; alias ll' </dev/null` → `alias ll='ls -l --color=auto'` twice |
| Severity | medium |
| Public-contract impact | yes (documented alias rule) |
| rc.A behavior | Reproduced: the init layer defines the pack aliases unconditionally as 2.x does, and the emitted unload keeps the 2.x snapshot (names only). |
| Fix recipe (3.0) | In `internal/shellinit`, emit every pack alias guarded by `[[ -v 'BASH_ALIASES[NAME]' ]] \|\| alias NAME=...` (a name the user defined first is left alone) and record the names Gash defined in `__GASH_ALIAS_NAMES`; unload removes only those. Makes `docs/configuration.md:51-52` true; note it in CHANGELOG (aliases the user had stop being replaced). |
| Proving test | `TestLedger_SHELL09` in `internal/shellinit`; `testdata/script/alias_user_first.txtar` (`alias ll='ls -alF'` before load: same after load and after unload; no prior alias: Gash's after load, none after unload) |
| Status | accept: done 5da215a |

<a id="shell-11"></a>
#### SHELL-11 · Sourcing gash.sh through a symlinked file breaks GASH_DIR

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N10) |
| Area | Correctness |
| Bash location | `gash.sh:52-57` (`GASH_DIR` from `${BASH_SOURCE[0]%/*}` with no link resolution; the comment at `:50-51` says symlinked checkouts work) |
| Observed behavior | `source ~/bin/gash.sh` where `~/bin/gash.sh` is a link to the real file sets `GASH_DIR` to `~/bin`: five `lib/core/*.sh: missing` load errors, `GASH_VERSION=unknown`, no prompt. A symlinked directory works. A relative path such as `../gash/./gash.sh` stays un-normalized (cosmetic). |
| Reproduction | `mkdir $H/bin; ln -s $C/gash.sh $H/bin/gash.sh; $E bash --noprofile --norc -i -c 'source $HOME/bin/gash.sh; echo "V=$GASH_VERSION"' </dev/null` → `V=unknown` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: the generated `gash.sh` computes `GASH_DIR` the same way, so a linked file fails as in 2.x (the binary is then not found next to it). |
| Fix recipe (3.0) | `internal/compatgen` template for `gash.sh`: resolve the file with a loop (`f=${BASH_SOURCE[0]}; while [[ -L $f ]]; do l=$(readlink -- "$f"); [[ $l == /* ]] \|\| l=${f%/*}/$l; f=$l; done`) before taking the directory; `gash init bash` prints the resolved absolute path. |
| Proving test | `TestLedger_SHELL11` in `internal/compatgen`; `testdata/script/gash_sh_symlink.txtar` (source through a file link and a chain of two links: `GASH_VERSION` is the tree's version, no load errors) |
| Status | accept: done aeb52af (GASH_DIR is resolved in gash.sh, not printed by gash init) |

<a id="shell-12"></a>
#### SHELL-12 · Module aliases are outside the shadow rule

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N15) |
| Area | Correctness |
| Bash location | `lib/aliases/_loader.sh:155` (`__gash_alias_before` taken after the modules loaded, `gash.sh:210`); `hg` has its own check at `lib/modules/system.sh:1067`; `sl` is exempt by list |
| Observed behavior | The "before" list includes every module alias, so a module alias is never dropped when a program of that name is on PATH (the rule that drops `gp`, `gc`, `dc`, `gs`); `sl` is exempt by list although it is also a real program. |
| Reproduction | `mkdir $H/b; printf '#!/bin/sh\necho real\n' >$H/b/dcc; chmod +x $H/b/dcc; $E PATH=$H/b:/usr/bin:/bin bash --noprofile --norc -i -c 'source $C/gash.sh; type -t dcc' </dev/null` → `alias` (the program is shadowed; `gp` in the same setup is dropped) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: the init layer applies the shadow rule (`[[ -v 'BASH_ALIASES[NAME]' ]]` guard and PATH check) to the alias-pack names the 2.x loader checked and not to module aliases. |
| Fix recipe (3.0) | In `internal/registry`, mark each alias "shadowable"; apply the PATH check to module aliases too (except the ones whose purpose is to replace a program, listed explicitly with a reason), and remove `sl` from the exempt list or document why. `docs/configuration.md` (aliases) states the rule for module aliases. |
| Proving test | `TestLedger_SHELL12` in `internal/shellinit` (fake `dcc` on PATH: no `dcc` alias emitted; without it: emitted) |
| Status | accept: done f75f7f8, 07d81fe |

### self-management

<a id="self-01"></a>
#### SELF-01 · gash_upgrade compares for equality only and can downgrade

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "gash_upgrade compares for equality only (can downgrade)" (CONFIRMED) |
| Area | Self-management |
| Bash location | `lib/modules/gash.sh:205-206` (`current_tag` exact match, `==` test), `:148-152` (`__gash_latest_tag` filters out pre-releases) |
| Observed behavior | From a checkout at `v3.0.0-rc.1` it prints `OK: Upgraded Gash from v3.0.0-rc.1 to v2.1.0.` and checks out v2.1.0, rc 0; from an untagged commit after v2.1.0: `OK: Upgraded Gash to v2.1.0.` (moves back). On an rc build, the 2.x-style `gash_upgrade` therefore returns to v2.1.0; that is also the rollback path from rc builds. |
| Reproduction | `X=$(mktemp -d); git clone -q $MIRROR $X; git -C $X tag v3.0.0-rc.1 go-refactor; git -C $X checkout -q v3.0.0-rc.1; $E bash -c 'GASH_HEADLESS=1; source $C/gash.sh; GASH_DIR=$X gash_upgrade' </dev/null` (`$MIRROR` = a local bare mirror, never the real checkout) → `OK: Upgraded Gash from v3.0.0-rc.1 to v2.1.0.`, HEAD at v2.1.0 |
| Severity | medium |
| Public-contract impact | yes (documented messages and the resulting version) |
| rc.A behavior | Reproduced: `gash_upgrade` (decision `redesign`: name, output lines and exit codes kept, `upgrade.md` 3.6 flow) still checks out the highest final release when it differs from HEAD, so an rc.A install returns to v2.1.0. |
| Fix recipe (3.0) | In `internal/selfmgmt`: parse current and latest as semver (`v` optional, a pre-release is lower than its release); without an exact tag use `git describe --tags --abbrev=0` plus `git merge-base --is-ancestor LATEST HEAD`; when current >= latest or LATEST is an ancestor of HEAD print `Info: This checkout (CUR) is newer than the latest release (LATEST); not downgrading.` and return 0 with no checkout. Rollback becomes an explicit command (`upgrade.md`). Update `docs/reference/gash.md` (gash_upgrade). |
| Proving test | `TestLedger_SELF01` in `internal/selfmgmt` (mirror with v2.0.0, v2.1.0, v3.0.0-rc.1 and an untagged commit after v2.1.0: from rc.1 and from the untagged commit HEAD is unchanged and the newer-than line is printed; from v2.0.0 HEAD moves to v2.1.0) |
| Status | accept: done 9f7520e |

<a id="self-02"></a>
#### SELF-02 · gash_uninstall deletes ~/.gashrc without a backup and then deletes its own profile backups

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "gash_uninstall deletes ~/.gashrc without backup" (CONFIRMED) |
| Area | Self-management |
| Bash location | `lib/modules/gash.sh:280-288` (`rm -f "$HOME/.gashrc"` at `:283`), `:318-320` (removes every `PROFILE.gash-uninstall.bak`) |
| Observed behavior | Answering `y` removes `~/.gashrc` with no copy, and the `.gash-uninstall.bak` files `sed -i` made of the profiles are removed at the end, so neither the user's lines in `~/.gashrc` (the 1.x migration appends them there, `migrate.sh:111-121,243-257`) nor the pre-edit profile can be recovered. |
| Reproduction | in a pty (`script -qec`), HOME with a fake install `$H/.gash` (a copy of `$C`), `~/.bashrc` holding the Gash block and `~/.gashrc` = the template plus `alias mine='echo mine'`; `printf 'y\ny\n' \| script -qec "$E bash --noprofile --norc -i -c 'source \$HOME/.gash/gash.sh; gash_uninstall'" /dev/null` → `~/.gashrc` and `~/.bashrc.gash-uninstall.bak` both gone |
| Severity | high |
| Public-contract impact | yes (file effects of a documented command) |
| rc.A behavior | Reproduced: rc.A removes `~/.gashrc` and the profile backups exactly as 2.x does. |
| Fix recipe (3.0) | In `internal/selfmgmt` uninstall: when `~/.gashrc` differs from every known template (2.x `.gashrc`, its custom-dir variant, the four 1.x variants), rename it to `~/.gashrc.gash-uninstall.bak` and print `Info: Saved your ~/.gashrc as ~/.gashrc.gash-uninstall.bak`; keep the profile `.gash-uninstall.bak` files and list them in the final lines (`Info: Backups kept: ...`). `docs/troubleshooting.md` (uninstall) lists them. |
| Proving test | `TestLedger_SELF02` in `internal/selfmgmt` (modified `~/.gashrc` → `.bak` with the same bytes; pristine template → no `.bak`; profile backup present after uninstall) |
| Status | accept: done 37931cb |

<a id="self-03"></a>
#### SELF-03 · gash_unload leaves completion specs, exports and an empty PROMPT_COMMAND

| Field | Value |
|---|---|
| Section | Defects |
| Origin | Seed: "gash_unload leaves completion specs and some exports" (CONFIRMED) |
| Area | Correctness |
| Bash location | `lib/modules/gash.sh:389-464` (no `complete -r`, no export bookkeeping); snapshot list `gash.sh:179-193` (`:182`); `lib/shell/prompt.sh:302-303` |
| Observed behavior | After `source gash.sh; source bash_completion; gash_unload`: 139 `complete -o default -F __gash_complete` specs remain (TAB then prints `bash: completion: function '__gash_complete' not found`), `LESSOPEN`, `LESSCLOSE`, `LS_COLORS` (and `LESS_TERMCAP_*` when `LESS` was set) stay exported, `PROMPT_COMMAND` becomes `""` instead of unset. |
| Reproduction | `$E bash --noprofile --norc -i -c 'source $C/gash.sh; source $C/bash_completion; gash_unload; complete -p \| wc -l; declare -px \| grep -cE "LESSOPEN\|LESSCLOSE\|LS_COLORS"; declare -p PROMPT_COMMAND' </dev/null` → `139`, `3`, `declare -- PROMPT_COMMAND=""` |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Reproduced: the unload function emitted by `gash init bash` stays Bash and keeps the 2.x leftovers (`shell.md`), reading the 2.x snapshot format so a 2.x-loaded shell can still unload. |
| Fix recipe (3.0) | In `internal/shellinit`: record `__GASH_COMPLETE_NAMES` when registering and run `complete -r "${__GASH_COMPLETE_NAMES[@]}"` on unload; add a second array `__GASH_SNAPSHOT2` holding set/unset and export state of `LESSOPEN LESSCLOSE LS_COLORS LSCOLORS LESS_TERMCAP_* PAGER NVM_DIR debian_chroot PROMPT_COMMAND` and pre-existing `GASH_*` (SELF-05), restored exactly; the 2.x `__GASH_SNAPSHOT` format stays valid. |
| Proving test | `TestLedger_SELF03` in `internal/shellinit`; `testdata/script/unload_exact.txtar` (`declare -p` minus dynamic variables, `alias -p`, `complete -p`, `shopt -p`, `set -o`, `trap -p`, `umask` identical before load and after unload) |
| Status | accept: done 542e801 |

<a id="self-04"></a>
#### SELF-04 · gash_uninstall replaces a symlinked profile with a regular file

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N4) |
| Area | Self-management |
| Bash location | `lib/modules/gash.sh:268,272` (`sed -i.gash-uninstall.bak` without `--follow-symlinks`) |
| Observed behavior | With `~/.bashrc -> dotfiles/bashrc` holding the Gash block, uninstall leaves `~/.bashrc` as a regular file without the block and the dotfiles target still with it (GNU `sed -i` writes a new file in place of the link). |
| Reproduction | as SELF-02 with `mkdir $H/dot; mv $H/.bashrc $H/dot/bashrc; ln -s dot/bashrc $H/.bashrc` before the run; afterwards `test -L $H/.bashrc \|\| echo replaced; grep -c 'GASH START' $H/dot/bashrc` → `replaced`, `1` |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Reproduced: rc.A writes the edited profile in place of the link (temp file renamed over the link path), with the same `.gash-uninstall.bak` side copy. With BSD `sed` (macOS) `sed -i` refuses the link instead and the profile keeps the block, as 2.x does there; the fix recipe drops the `sed` dependency on both. |
| Fix recipe (3.0) | `internal/selfmgmt` profile editing resolves the link (`filepath.EvalSymlinks`) and writes a temp file in the target's directory, then renames it over the target, keeping its mode; the link stays. Same for install. |
| Proving test | `TestLedger_SELF04` in `internal/selfmgmt` (linked `~/.bashrc`: link intact, target without the block, mode kept) |
| Status | accept: done 25f241e |

<a id="self-05"></a>
#### SELF-05 · gash_unload removes the user's own GASH_* settings

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N5) |
| Area | Correctness |
| Bash location | `lib/modules/gash.sh:450-456` (unsets every `GASH_*` and `__GASH_*` name); doc `docs/configuration.md:236-238` ("every ... GASH_* variable") vs the help line "Restores the shell state saved before Gash was loaded" |
| Observed behavior | `export GASH_BANNER=0` set in `.bashrc` before the block is gone after `gash_unload`, so `source ~/.gashrc` afterwards prints the banner. |
| Reproduction | `$E bash --noprofile --norc -i -c 'export GASH_BANNER=0; source $C/gash.sh; gash_unload; declare -p GASH_BANNER' </dev/null` → `bash: declare: GASH_BANNER: not found` |
| Severity | low |
| Public-contract impact | no (the docs page describes the 2.x sweep; it changes with the fix) |
| rc.A behavior | Reproduced: the emitted unload sweeps `GASH_*` and `__GASH_*` as 2.x does. |
| Fix recipe (3.0) | `internal/shellinit`: record `compgen -v GASH_` (names, values, export flag) in `__GASH_SNAPSHOT2` before load (SELF-03); unload restores those and unsets only the names Gash created; the secret caches (`__GASH_ENV_*`) are always unset. Update `docs/configuration.md:236-238`. |
| Proving test | `TestLedger_SELF05` in `internal/shellinit` (`GASH_BANNER=0` exported before load: same value and export flag after unload; `GASH_VERSION` gone) |
| Status | accept: done 542e801 |

<a id="self-06"></a>
#### SELF-06 · gash_doctor always returns 0, ignores unknown options and checks incomplete lists

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N7) |
| Area | Self-management |
| Bash location | `lib/modules/gash.sh:546-652` (options loop `:551-556` ignores the rest; core list `:592-594`; module list `:605`; summary `:646-651` returns the status of the last `__gash_warning`, 0) |
| Observed behavior | `gash_doctor --bogus` runs the checks; with `lib/modules/php.sh` missing it prints `Warning:   php — not found` and `Warning: 1 issue(s) found.` and returns 0. The module list omits `chrome` and `migrate`; the core list omits `bash_completion`, `VERSION`, `quotes/list.txt`, `.gash_env.template`. |
| Reproduction | `cp -r $C $T/g; mv $T/g/lib/modules/php.sh $T/; $E bash --noprofile --norc -i -c 'source $T/g/gash.sh; gash_doctor --bogus; echo rc=$?' </dev/null 2>&1 \| grep -E 'issue\|rc='` → `Warning: 1 issue(s) found.`, `rc=0` |
| Severity | medium |
| Public-contract impact | yes (exit status of a documented command) |
| rc.A behavior | Reproduced: `gash_doctor` (decision `redesign`, SELF-17) keeps the 2.x output lines, rc 0 with issues, and ignores unknown options. |
| Fix recipe (3.0) | In `internal/selfmgmt` doctor: return 1 when issues > 0; reject an unknown option with `Error: Unknown option: X` rc 2 (the project's usage status); take the file list from the shipped manifest (SELF-17). Update `docs/reference/gash.md` (gash_doctor: "returns 1 when a check fails"). |
| Proving test | `TestLedger_SELF06` in `internal/selfmgmt` (missing shipped file → rc 1 and the issue line; `--bogus` → usage error, no checks run) |
| Status | accept: done 94d8efc |

<a id="self-07"></a>
#### SELF-07 · The installer leaves an unreleased checkout behind when GASH_REQUIRE_SIGNED refuses

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N11) |
| Area | Security |
| Bash location | `install.sh:149-166` (`gashinst_verify_tag`, `exit 1` at `:162-164`) runs after the clone at `:194-199` |
| Observed behavior | `GASH_REQUIRE_SIGNED=1` with an unsigned latest tag prints `GASH_REQUIRE_SIGNED=1 is set; refusing to install v2.1.0.` and exits 1, but `~/.gash` stays cloned on the default branch (unreleased code); the next plain run says `Gash is already installed in ~/.gash, updating it.`, and `source ~/.gash/gash.sh` loads the unreleased tree. |
| Reproduction | `$E GASH_REQUIRE_SIGNED=1 GASH_INSTALL_GIT_REPO=$MIRROR bash $C/install.sh -y; echo rc=$?; git -C $HOME/.gash rev-parse --abbrev-ref HEAD` (temporary HOME, local bare mirror with unsigned tags) → the refusal, `rc=1`, a branch name |
| Severity | high |
| Public-contract impact | no (the refusal text and rc stay; only the leftover directory goes) |
| rc.A behavior | Reproduced: the rewritten `install.sh` (`layout.md`: clone and tag as before, then fetch and verify the binary; `upgrade.md` 3.7) keeps the clone-then-verify order, so the clone stays after the refusal and no binary is fetched. |
| Fix recipe (3.0) | `install.sh`: clone into `mktemp -d "${INSTALL_DIR%/*}/.gash-install.XXXXXX"`, check out and verify the tag there, then `mv` it to `INSTALL_DIR`; on refusal remove the temporary clone and print `Info: Nothing was installed.`; for an existing install verify before the checkout (already so); the binary fetch runs only after the rename. |
| Proving test | `testdata/script/install_require_signed.txtar` (unsigned mirror, `GASH_REQUIRE_SIGNED=1`: rc 1, refusal line, `~/.gash` absent; signed and verifiable: installed) |
| Status | accept: done fa0cb46 |

<a id="self-08"></a>
#### SELF-08 · Migration backups are world-readable, keep the old mode of ~/.gash_env and are never pruned

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N12) |
| Area | Security |
| Bash location | `lib/modules/migrate.sh:619-640` (`mkdir -p "$backup"` with the umask, `cp -p` of each touched file, no pruning) |
| Observed behavior | With umask 022 the backup directory is 755 and the copy of a 0644 `~/.gash_env` (the mode the migration fixes) stays 0644, readable by other local users; every run adds a directory under `~/.local/state/gash/backups/`. |
| Reproduction | temporary HOME with a 1.x `~/.gashrc` (`export GASH_BASH=$HOME/.gash/gash.sh`) and a 0644 `~/.gash_env`; `$E bash --noprofile --norc -i -c 'umask 022; source $C/gash.sh; gash_migrate --yes --plain' </dev/null; find $HOME/.local/state/gash/backups -printf '%m %p\n'` → `755 .../backups/STAMP`, `644 .../STAMP/2-.gash_env` |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Moot (D-6): 3.x does not port the 1.x migration; `gash_migrate` prints a pointer to the 2.x route and writes nothing. |
| Fix recipe (3.0) | In `internal/selfmgmt` migrate (and the Bash fallback): create `backups/` and `STAMP/` with mode 0700, write each copy with 0600, keep the 5 newest stamps and remove older ones after a successful write (never the one just made); `--status` lists them. Document in `docs/troubleshooting.md` (migration backups). |
| Proving test | `TestLedger_SELF08` in `internal/selfmgmt` (umask 022: directory 0700, files 0600; seven runs leave five stamps; `--rollback` still restores the newest) |
| Status | accept: done (moot under D-6); TestSelfmgmt_MigratePointer |

<a id="self-09"></a>
#### SELF-09 · The migration does not mention the 1.x history behavior it drops

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N13) |
| Area | Self-management |
| Bash location | `lib/modules/migrate.sh:383-560` (migration items; none for history); 1.x behavior from `v1.5.3:gash.sh:151` (`HISTCONTROL=ignoreboth:erasedups`) and `:379` (`history -a; history -c; history -r` in PROMPT_COMMAND) |
| Observed behavior | 1.x shared history between terminals live and used `erasedups`; 2.x defaults to `GASH_HISTORY_SYNC=0` and `ignoreboth`, and no migration screen or closing note says so. |
| Reproduction | `git -C $MIRROR grep -n 'erasedups\|history -a; history -c' v1.5.3 -- gash.sh` (1.x: `gash.sh:151`, `:379`) and `grep -c HISTORY_SYNC $C/lib/modules/migrate.sh` → `0` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Moot (D-6): 3.x does not port the 1.x migration screens. |
| Fix recipe (3.0) | Add a migration item in `internal/selfmgmt` (and `migrate.sh` while it ships): "Gash 1.x shared history between open terminals. Keep that (GASH_HISTORY_SYNC=1)?", recommended choice no, writes `GASH_HISTORY_SYNC=1` to `~/.gash_env` on yes. Add the line to `docs/migration` notes. |
| Proving test | `TestLedger_SELF09` in `internal/selfmgmt` (1.x `~/.gashrc`: the plan contains the history item; `--yes` leaves the setting unset) |
| Status | accept: done (moot under D-6) |

<a id="self-10"></a>
#### SELF-10 · Tag signatures are detected by a text match

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N14; confirmed here for an annotated tag whose message quotes the marker) |
| Area | Self-management |
| Bash location | `lib/modules/gash.sh:157-172` (`__gash_verify_tag`, grep at `:159`); `install.sh:151` |
| Observed behavior | `git cat-file -p refs/tags/TAG \| grep 'BEGIN (PGP\|SSH) SIGNATURE'` also matches an unsigned annotated tag whose message contains the marker line (and, by reading, a lightweight tag pointing at a signed commit), so the user is told `TAG is signed, but the signature could not be verified (is the key in your keyring?).` and, with `GASH_REQUIRE_SIGNED=1`, the upgrade is refused. |
| Reproduction | `git -C $R tag -a v9.9.9 -m 'quoting -----BEGIN PGP SIGNATURE----- here'; $E bash -c 'GASH_HEADLESS=1; source $C/gash.sh; GASH_REQUIRE_SIGNED=1 __gash_verify_tag $R v9.9.9; echo rc=$?'` (scratch repository `$R`) → the "signed, but could not be verified" warning, `rc=1` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: `internal/gitx` runs the same `cat-file -p` and applies the same unanchored pattern before `verify-tag`. |
| Fix recipe (3.0) | `internal/gitx.TagSignature`: require `git cat-file -t refs/tags/TAG` = `tag`, then a line equal to `-----BEGIN PGP SIGNATURE-----` or `-----BEGIN SSH SIGNATURE-----` after the header block, then `git verify-tag`; an unsigned annotated or lightweight tag is "not signed". Same in `install.sh`. |
| Proving test | `TestLedger_SELF10` in `internal/gitx` (annotated tag quoting the marker → "not signed"; lightweight tag on a signed commit → "not signed"; signed tag with the key → verified) |
| Status | accept: done 156b8bb |

<a id="self-12"></a>
#### SELF-12 · The `gash` card counts and lists aliases that are not defined

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N16, counts part) |
| Area | Correctness |
| Bash location | `lib/modules/gash.sh:679-721` (`__gash_card_aliases` parses `lib/aliases/*.sh` text), `:882-884` (overview counts) |
| Observed behavior | Counts and topic lists come from the alias files, not the shell: aliases dropped by the shadow rule (`gp`, `gc` when programs of those names exist) and the docker and php packs when their tools are absent are still counted and listed (static 109 versus 133 live in the audit shell). |
| Reproduction | fake `gp` and `gc` programs in `$H/b`; `$E PATH=$H/b:/usr/bin:/bin bash --noprofile --norc -i -c 'source $C/gash.sh; gash --no-color \| grep "^  git "; gash git --no-color \| grep -cE "^ +(gp\|gc) "; type -t gp gc' </dev/null` → `git 7 44 ...`, `2`, `file`, `file` |
| Severity | low |
| Public-contract impact | no (the card is human output) |
| rc.A behavior | Reproduced: the binary renders the card from the static registry lists with the same counts, whatever the caller's shell defined. |
| Fix recipe (3.0) | The generated `gash` shell function passes the live alias names (`GASH__ALIASES="${!BASH_ALIASES[*]}"`) to the binary; `internal/registry` card code counts and lists only registered aliases present in that list (all registered ones when the variable is absent, e.g. from a script). |
| Proving test | `TestLedger_SELF12` in `internal/registry` (`GASH__ALIASES` without `gp gc` → git count 42 and neither listed) |
| Status | accept: done 1a72c4e |

<a id="self-13"></a>
#### SELF-13 · Small message defects: positional reported as an option, wrong line count in a comment

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N17, code parts; the help-text part is SELF-21) |
| Area | Correctness |
| Bash location | `lib/modules/gash.sh:66-72` (`gash_inspiring_quote`, message at `:70`); `:270-272` (comment "4 lines total" over a sed that deletes 5) |
| Observed behavior | `gash_inspiring_quote foo` prints `Error: Unknown option: foo (usage: gash_inspiring_quote [--source])`, rc 1, although `foo` is not an option. The uninstall fallback comment says "4 lines total" while the 1.x block it removes (`# Load Gash Bash`, `# This loads the Gash customized Bash.`, `if`, `source`, `fi`) has 5 lines, which the sed deletes correctly. |
| Reproduction | `$E bash --noprofile --norc -i -c 'source $C/gash.sh; gash_inspiring_quote foo; echo rc=$?' </dev/null` → the `Unknown option: foo` line, `rc=1` |
| Severity | low |
| Public-contract impact | no (rc stays 1; only the wording changes) |
| rc.A behavior | Reproduced: same message and rc. |
| Fix recipe (3.0) | `internal/selfmgmt` quote command: a word not starting with `-` gives `Error: Unexpected argument: foo (usage: gash_inspiring_quote [--source])` rc 1; options keep `Unknown option`. Comment fixed in the Bash it replaces (no code effect). |
| Proving test | `TestLedger_SELF13` in `internal/selfmgmt` (`foo` → unexpected-argument line, rc 1; `--bogus` → unknown-option line, rc 1) |
| Status | accept: done 0b987ab |

<a id="self-14"></a>
#### SELF-14 · A 1.x shell reading the 2.x/3.x quote file can print a comment line

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N18, inferred) |
| Area | Self-management |
| Bash location | 1.x quote reader (`v1.5.3`, picks any random line of `quotes/list.txt`); 2.x format adds `# source` comment lines (`quotes/list.txt`) |
| Observed behavior | The 1.x reader loads every line with `mapfile` and picks one with `RANDOM`; the 2.x file has a `# source` comment above each quote (375 comment lines), so after the tree moves to 2.x or 3.x under a running 1.x shell, about half of its banners print a comment line as the quote. Only while a 1.x shell stays open after an upgrade. |
| Reproduction | `git -C $MIRROR show v1.5.3:lib/modules/gash.sh \| sed -n 65,79p` (`mapfile -t quotes`, `RANDOM % ${#quotes[@]}` over every line) and `grep -c '^#' $C/quotes/list.txt` → `375` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: rc.A keeps `quotes/list.txt` in the 2.x format at the same path (`layout.md`), so an open 1.x shell behaves the same. |
| Fix recipe (3.0) | None in code (1.x is frozen); `upgrade.md` 3.5 notes "open a new terminal after upgrading from 1.x". Keep the file format stable (no new line kinds) through 3.x. |
| Proving test | `TestLedger_SELF14` in `internal/selfmgmt` (the embedded and shipped `quotes/list.txt` hold only quote lines and `# source` lines, the 2.x grammar) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="self-15"></a>
#### SELF-15 · Arguments are ignored, and gash_doctor --timing runs the user's ~/.bashrc three times

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (N19) |
| Area | Self-management |
| Bash location | `lib/core/config.sh:597-598` (`gash_env_init` checks only `$1 == --force`); `lib/modules/gash.sh:176` (`gash_upgrade`), `gash_unload`, `gash_username`, `gash_uninstall` (no argument checks); `:515-525` (`--timing`: `bash -i -c exit` three times, call at `:519`); rule `CONTRIBUTING.md:50` ("fails on unknown options") |
| Observed behavior | `gash_env_init --bogus` creates the file, rc 0; `gash_upgrade --bogus` upgrades; `gash_username --bogus` prints the user name, rc 0. `gash_doctor --timing` starts three interactive shells, so the user's `~/.bashrc` (and whatever it starts) runs three times. |
| Reproduction | `printf 'echo RC >>"$HOME/count"\n' >$H/.bashrc; $E bash --noprofile --norc -i -c 'source $C/gash.sh; gash_env_init --bogus; echo rc=$?; gash_doctor --timing >/dev/null 2>&1; wc -l <"$HOME/count"' </dev/null` → `rc=0`, `3` |
| Severity | low |
| Public-contract impact | yes (unknown options start failing) |
| rc.A behavior | Reproduced: the same commands accept and ignore extra arguments, and `--timing` keeps the three whole-shell runs (the measurement is its documented purpose). |
| Fix recipe (3.0) | Every self-management command in `internal/selfmgmt` rejects an unknown option with `Error: Unknown option: X` and an unexpected positional with `Error: Unexpected argument: X`, rc 2 (the registry's usage status), before any side effect. `gash_doctor --timing` prints `Info: --timing starts your shell 3 times (your ~/.bashrc runs each time).` before measuring. Update `docs/reference/gash.md`. |
| Proving test | `TestLedger_SELF15` in `internal/selfmgmt` (each command with `--bogus`: rc 2, no file written, no git call) |
| Status | accept: done f6ba544 |

### lib/core

<a id="core-03"></a>
#### CORE-03 · gash_db_list shows the password when the URL has an empty user name

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (this fragment) |
| Area | Security |
| Bash location | `lib/core/config.sh:511-512` (mask regex `^([^:]+://[^:]+:).+(@[^@]*)$` needs a non-empty user); doc `AGENTS.md:91` ("`gash_db_list` prints the names with masked passwords") |
| Observed behavior | `DB:default=mysql://:SECRETPW@h/d` (password-only authentication, valid for the parser at `config.sh:368-436`) is printed unmasked, in headless mode too, so an agent following AGENTS.md sees the secret. `mysql://u:p@h/d` is masked as documented. |
| Reproduction | `printf 'DB:default=mysql://:SECRETPW@h/d\n' >$H/.gash_env; chmod 600 $H/.gash_env; $E bash -c 'GASH_HEADLESS=1; source $C/gash.sh; gash_db_list' </dev/null` → `  default         mysql://:SECRETPW@h/d` (placeholder password) |
| Severity | high |
| Public-contract impact | no (the documented output is the masked one) |
| rc.A behavior | Reproduced: the binary applies the same mask rule and prints the URL as 2.x does (rule 2; the fix is the first 3.0 change). |
| Fix recipe (3.0) | `internal/config.MaskDBURL`: when the userinfo (text before the last `@`) contains `:`, print `driver://USER:****@HOSTPART` with USER possibly empty; equivalently the regex `^([^:]+://[^:@]*:).+(@[^@]*)$`. Shared by every place that shows a DB URL. |
| Proving test | `TestLedger_CORE03` in `internal/config` (`mysql://:p@h/d` → `mysql://:****@h/d`; `u:p@@h` → `u:****@h`; no password → unchanged); `testdata/script/db_list_mask.txtar` |
| Status | accept: done fc62bda |

<a id="core-04"></a>
#### CORE-04 · gash_db_list lists every duplicate name, while only the first is used

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (this fragment) |
| Area | Correctness |
| Bash location | `lib/core/config.sh:243-251` (every `DB:` line appended), `:485-517` (`gash_db_list` prints all), `__gash_get_db_url` (first match) |
| Observed behavior | Two `DB:default=` lines both appear under `default`; `gash_db_test` and the `llm_db_*` tools use the first one, with no warning about the second. |
| Reproduction | `printf 'DB:default=mysql://a:p1@h1/d\nDB:default=pgsql://b:p2@h2/e\n' >$H/.gash_env; chmod 600 $H/.gash_env; $E bash -c 'GASH_HEADLESS=1; source $C/gash.sh; gash_db_list; __gash_get_db_url default' </dev/null` → two `default` rows, then `mysql://a:p1@h1/d` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: rc.A lists both rows and resolves the first. |
| Fix recipe (3.0) | `internal/config` loader: on a repeated `DB:` (and `SSH:`, `HOST:`) name, keep the first and warn `Line N of ~/.gash_env: duplicate DB:NAME, ignored (first is at line M)` in interactive mode; `gash_db_list` shows only the entries in effect. |
| Proving test | `TestLedger_CORE04` in `internal/config` (duplicate name → one entry, the first, one warning with both line numbers) |
| Status | accept: done 43e391e |

<a id="core-06"></a>
#### CORE-06 · Messages name ~/.gash_env even when GASH_ENV_FILE points elsewhere

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (this fragment) |
| Area | Correctness |
| Bash location | `lib/core/config.sh:493` (`No database connections configured in ~/.gash_env`), `:533` (`Connection 'NAME' not found in ~/.gash_env`), `:680` (`No AI providers configured in ~/.gash_env`) |
| Observed behavior | With `GASH_ENV_FILE=$HOME/other` the loader reads that file, but the empty-list and not-found messages still say `~/.gash_env` (the loader's own warnings, `gash_env_init` and `gash_doctor` use the real path). |
| Reproduction | `$E bash -c 'GASH_HEADLESS=1; source $C/gash.sh; GASH_ENV_FILE=$HOME/other; : >"$GASH_ENV_FILE"; chmod 600 "$GASH_ENV_FILE"; __gash_reload_env; gash_db_test nope' </dev/null` → `Error: Connection 'nope' not found in ~/.gash_env` |
| Severity | low |
| Public-contract impact | yes (message text) |
| rc.A behavior | Reproduced: the same literal text. |
| Fix recipe (3.0) | `internal/config` exposes `ShownPath()` (`${GASH_ENV_FILE:-~/.gash_env}` with `$HOME` shown as `~`); the three messages use it. `--help` bodies stay as they are (they describe the default file). |
| Proving test | `TestLedger_CORE06` in `internal/config` (`GASH_ENV_FILE=/x/y`: the three messages name `/x/y`) |
| Status | accept: done 75b93b7, d16347c |

<a id="core-11"></a>
#### CORE-11 · A bracketed IPv6 host in a DB URL is split at its first colon

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (this fragment) |
| Area | Correctness |
| Bash location | `lib/core/config.sh:420-433` (`host=${hostinfo%%:*}`, `port=${hostinfo#*:}`) |
| Observed behavior | `pgsql://u:p@[::1]:5432/d` parses as host `[` and port `:1]:5432`; the connection then fails with a client error instead of reaching `::1`. |
| Reproduction | `$E bash -c 'GASH_HEADLESS=1; source $C/gash.sh; __gash_parse_db_url "pgsql://u:p@[::1]:5432/d" dr us pw ho po db; echo "host=[$ho] port=[$po]"' </dev/null` → `host=[[] port=[:1]:5432]` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: `internal/config.ParseDBURL` splits the same way, so the native driver is handed host `[` and fails as 2.x did (error text per DB-07). |
| Fix recipe (3.0) | `ParseDBURL`: when hostinfo starts with `[`, the host is up to the matching `]` (brackets removed for the driver), the port follows `]:`; a missing `]` is `invalid_db_url` with hint `Write IPv6 hosts as [addr]:port`. Document in `.gash_env.template` (DB section). |
| Proving test | `TestLedger_CORE11` in `internal/config` (`[::1]:5432` → host `::1`, port `5432`; `[::1]` → default port; `[::1` → `invalid_db_url`) |
| Status | accept: done 2e890cb, d16347c |

<a id="core-13"></a>
#### CORE-13 · DB passwords lose trailing newlines and `%00` when decoded

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (rc.A port of `internal/config` and `internal/sqlguard`) |
| Area | Correctness |
| Bash location | `lib/modules/llm/db.sh:246`, `lib/core/config.sh:545` (`$(__gash_url_decode ...)`) |
| Observed behavior | The decoded password is captured with command substitution, which strips trailing newlines, and Bash variables cannot hold NUL. A password ending in `%0A` loses the newlines; `%00` disappears. The client then authenticates with a different password. |
| Reproduction | a DB URL whose password ends in `%0A%0A` decodes to the password without them (temp `~/.gash_env`) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: `config.DecodePassword` strips trailing newlines and drops NUL, as 2.x does. |
| Fix recipe (3.0) | Decode into the value directly (no command substitution) and refuse a password that contains NUL with a clear error. |
| Proving test | `TestLedger_CORE13_2x` pins the 2.x result in `internal/config`. |
| Status | accept: done ce8eff2, d16347c |

<a id="core-14"></a>
#### CORE-14 · HOME is matched as a glob when a path is shown with `~`

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (rc.A port of `internal/config` and `internal/sqlguard`) |
| Area | Correctness |
| Bash location | `lib/core/config.sh:137`, `:140`, `:170` (`${file/#$HOME/\~}`) |
| Observed behavior | `$HOME` is an unquoted pattern in the substitution, so glob characters in HOME match other names, and no `/` is required after it: with HOME=/home/a, `/home/ab/.gash_env` is shown as `~b/.gash_env`. |
| Reproduction | HOME=/home/a and GASH_ENV_FILE=/home/ab/.gash_env, then a permission warning names `~b/.gash_env` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: `config.ShownPath` matches HOME with a port of Bash's C-locale pattern matcher. Only patterns with an equivalence class or a multi-character collating symbol (`[=]=]`) can differ, because they depend on the locale (forced, same reasoning as CORE-08). |
| Fix recipe (3.0) | Replace only a literal HOME followed by `/` or the end of the path. |
| Proving test | `TestLedger_CORE14_2x` in `internal/config`. |
| Status | accept: done ce8eff2, 95b09cc |

<a id="core-15"></a>
#### CORE-15 · SSH key paths: `~user/` is not a home directory, and `&` in HOME is rewritten on Bash 5.2

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (rc.A port of `internal/config` and `internal/sqlguard`) |
| Area | Correctness |
| Bash location | `lib/core/config.sh:235` (`${key/#\~/$HOME}`) |
| Observed behavior | Only the first `~` is replaced by HOME, so `~user/k` becomes `$HOMEuser/k`. Under Bash 5.2's patsub_replacement an unquoted `&` in HOME stands for the matched `~`, so a HOME containing `&` gives a different path on Bash 5.2 than on 4.4 to 5.1. |
| Reproduction | `SSH:~other/k=` gives the path `$HOME` + `other/k`; with HOME containing `&`, the path has `~` in its place on Bash 5.2 (temp HOME) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced as Bash 5.2 does it (the version CI and the parity tests use): `~` prefix replaced by HOME, `&` in HOME read as the match. |
| Fix recipe (3.0) | Expand only `~` and `~/`, quote the replacement, and refuse `~user`. |
| Proving test | `TestLedger_CORE15_2x` in `internal/config`. |
| Status | accept: done ce8eff2 |

<a id="core-19"></a>
#### CORE-19 · A located download URL that 404s with a Content-Length is treated as present

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (core slice, RemoteSize / chrome size parity) |
| Area | Correctness |
| Bash location | `lib/core/utils.sh:437-439` (`__gash_remote_size`: `curl -fsIL URL \| … awk 'END { print n + 0 }'`); caller `lib/modules/chrome.sh:89-98` (`__gash_chs_locate`, `size="$(__gash_remote_size "$url")"; ((size > 0))`) |
| Observed behavior | `curl -fsIL` on a 4xx still prints the response's `Content-Length` header, and the pipeline reads it while ignoring curl's exit 22 (curl is the first stage of the pipe; only awk's status is seen). So a located chrome-headless-shell URL that 404s but carries a `Content-Length` yields a non-zero size; `__gash_chs_locate`'s `((size > 0))` succeeds and the download proceeds, only to fail once the body is the error page. |
| Reproduction | `__gash_remote_size` against a URL that returns 404 with a non-zero `Content-Length` prints that length (not 0), and `__gash_chs_locate` then returns success. |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Reproduced: `internal/ui/download.go:123-136` `Downloader.RemoteSize` returns `resp.ContentLength` of the HEAD response even on a 404 (only a response with no/`<0` length is 0), matching 2.x `curl -fsIL`; `download_test.go` records that a 404 reports the size of its own body. |
| Fix recipe (3.0) | `internal/ui` `RemoteSize`: treat a non-2xx HEAD response as size 0 (or an explicit error), so a 404 with a `Content-Length` is not read as a present download; `internal/chrome` `locate` then fails cleanly with a clear message instead of starting a doomed download. |
| Proving test | `TestLedger_CORE19_fix` in `internal/ui`/`internal/chrome` (HEAD 404 with a non-zero Content-Length → size 0, locate fails with a message); `TestLedger_CORE19_2x` proves the reproduction |
| Status | accept: done d34ccf4 |

### test harness

<a id="test-01"></a>
#### TEST-01 · The test runner leaks the caller's NO_COLOR into the hermetic environment

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (tests dossier H1) |
| Area | Test harness |
| Bash location | `tests/run.sh:165` |
| Observed behavior | `[[ -n "${NO_COLOR-}" ]] && envv+=("NO_COLOR=1")` forwards the caller's `NO_COLOR` into every spec, so results depend on the caller: with `NO_COLOR=1` exported, 5 tests fail (`aliases_refactor:178`, `color_gating:304`, `color_gating:316`, `core_output:7`, `prompt:261`). |
| Reproduction | `NO_COLOR=1 bash tests/run.sh -f 'defines color variables' specs/core_output_spec.sh` → `FAIL ... 0 passed, 1 failed` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Fixed in the harness on the branch before Go mode starts (test infrastructure, not Gash; no expectation changes). The Bash framework is removed at cutover. |
| Fix recipe (3.0) | Never forward `NO_COLOR` into the spec environment; pass a runner-only `GASH_TEST_NO_COLOR=1` and read it in `tests/gash-test.sh:36` for the runner's own colors. testscript sets an explicit environment per script. |
| Proving test | `NO_COLOR=1 bash tests/run.sh` gives 807/0; testscript scripts never inherit `NO_COLOR` (`testdata/script/env_hermetic.txtar`) |
| Status | accept: done c72b558 |

<a id="test-02"></a>
#### TEST-02 · `tests/run.sh -j1` is refused

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (tests dossier H2) |
| Area | Test harness |
| Bash location | `tests/run.sh:40-47`, `:93-95` |
| Observed behavior | Only `-j 1` is accepted; `-j1` falls through to the unknown-option branch. |
| Reproduction | `bash tests/run.sh -j1 specs/unload_spec.sh` → `run.sh: unknown option -j1`, exit 2 |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Fixed in the harness (it gains `--impl` at the same time). |
| Fix recipe (3.0) | Accept `-j*` and `--jobs=*`; validate an integer ≥ 1. |
| Proving test | `bash tests/run.sh -j1 specs/unload_spec.sh` exits 0 |
| Status | accept: done (obsolete at cutover: go test -p replaces tests/run.sh) |

<a id="test-03"></a>
#### TEST-03 · 88 conditional skips print no reason

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (tests dossier H3) |
| Area | Test harness |
| Bash location | `tests/gash-test.sh:134`; 88 `command -v X ... \|\| exit 77` sites in `tests/specs/` |
| Observed behavior | The runner extracts a reason only from a `SKIP:` line, so 5 of the 7 baseline skips (sqlite3 and gawk missing) show no cause. |
| Reproduction | `env PATH=/usr/bin:/bin bash tests/run.sh specs/llm_db_security_spec.sh` on a host without sqlite3: `SKIP` lines without a reason |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Fixed in the harness (`gash_require X` everywhere, `(no reason)` fallback). |
| Fix recipe (3.0) | Replace the idiom with `gash_require X`; testscript uses `[exec:X]` conditions, which print the condition. |
| Proving test | Without sqlite3 every DB skip prints `(sqlite3 not available)` |
| Status | accept: done (rc.A); every skip carries a reason |

<a id="test-04"></a>
#### TEST-04 · Specs fake `kill` with a fixed PID, a hazard once the code under test is a binary

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (tests dossier H4) |
| Area | Test harness |
| Bash location | `tests/specs/functions_edge_spec.sh:87-105`, `tests/specs/functions_more_spec.sh:226`, render fixtures `process_kill:yes` and `process_kill:survivor` |
| Observed behavior | `process_kill my-proc --force` runs with a Bash `kill()` override and `pgrep` faked to return PID 2222. A Go port calling `kill(2)` bypasses the override and would send SIGKILL to whatever real process has PID 2222. |
| Reproduction | By inspection: `grep -n 'kill()' tests/specs/functions_edge_spec.sh tests/specs/functions_more_spec.sh` and the pgrep stub returning 2222 |
| Severity | high |
| Public-contract impact | no |
| rc.A behavior | These tests are rewritten around a real victim (`sleep 300 & pid=$!`, pgrep stub returns `$pid`, as `functions_smoke_spec.sh:218` already does) before any Go-mode run; Go-mode runs also happen in a PID and network namespace. Expectations unchanged. |
| Fix recipe (3.0) | Same rewrite in the testscript port; no test may name a PID it did not create. |
| Proving test | Go mode: the victim is dead, no other PID was signalled (the namespace contains only the test's processes) |
| Status | accept: done 8f03ff2 |

<a id="test-05"></a>
#### TEST-05 · 52 render fixtures fake external commands as Bash functions

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (tests dossier H5) |
| Area | Test harness |
| Bash location | `tests/specs/output_style_render_spec.sh` fixtures (`curl(){...}` in the 7 `chrome_headless_update` fixtures, `wget(){...}` in `ip_public`, `ssh/scp/rsync(){...}` in `remote_*`, ...) |
| Observed behavior | The fakes work only because the Bash implementation calls these names in the same shell; a binary bypasses them and would reach the real network or tools (`ip_public` would contact the internet). |
| Reproduction | `grep -c '(){' tests/specs/output_style_render_spec.sh` and the fixture list in the tests dossier |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | A fixture helper writes such fakes as PATH stubs; Go-mode runs have no network, so a missed fake fails instead of escaping. Shell-side names (`history`, `fc`, `nvm`) stay function overrides. |
| Fix recipe (3.0) | Same in the testscript port (stubs on PATH, network namespace). |
| Proving test | Go mode under `unshare -n` passes R2-R6 |
| Status | accept: done 10ed38f |

<a id="test-06"></a>
#### TEST-06 · `llm_spec.sh` uses the repository's own `lib/` as a fixture tree

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (tests dossier H6) |
| Area | Test harness |
| Bash location | `tests/specs/llm_spec.sh:198,211,238,264` |
| Observed behavior | Results depend on the source layout; in Go mode `$GASH_TEST_ROOT/lib` is the generated shim tree, and at cutover `lib/` shrinks. |
| Reproduction | By inspection of the four tests |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | A private fixture tree in `$tmp` replaces it; assertions unchanged. |
| Fix recipe (3.0) | Same fixture in testscript. |
| Proving test | Identical output in both modes |
| Status | accept: done (rc.A); llm txtars use archive fixtures |

<a id="test-07"></a>
#### TEST-07 · 18 specs source Gash at top level in the runner process

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (tests dossier H7) |
| Area | Test harness |
| Bash location | e.g. `tests/specs/chrome_spec.sh:7`, `files_spec.sh:11`, `wsl_spec.sh:7` |
| Observed behavior | Every test re-sources inside its own `bash -c`, so the top-level load is dead weight, and a module with a load error breaks the whole file before any test runs. |
| Reproduction | `grep -ln '^gash_source_all' tests/specs/*.sh` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Lines removed in the harness. |
| Fix recipe (3.0) | n/a after cutover (testscript). |
| Proving test | Same results, faster start |
| Status | accept: done (obsolete at cutover) |

<a id="test-09"></a>
#### TEST-09 · `tests/run.sh` can end in FAIL with 0 failed: a reaped job is waited for again

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (rc.A gate run) |
| Area | Test harness |
| Bash location | `tests/run.sh` parallel loop (`wait -n` to throttle, then `wait "${pids[$i]}"` per spec) |
| Observed behavior | The throttle's `wait -n` can reap a spec process whose status Bash then no longer holds, so the later `wait PID` prints `wait: pid N is not a child of this shell`, returns 127 and the run ends in FAIL although every test passed. Intermittent; seen once with 807 passed, 0 failed. |
| Reproduction | repeated `bash tests/run.sh` on a loaded machine; the summary line reads FAIL with 0 failed |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Fixed in the harness: each spec process records its own exit status in the run directory, and the summary reads those files instead of the `wait` status. |
| Fix recipe (3.0) | n/a after cutover (testscript). |
| Proving test | The summary status follows the recorded statuses; a run with every spec passing never ends in FAIL. |
| Status | accept: done (obsolete at cutover) |

<a id="test-10"></a>
#### TEST-10 · Mock-client DB specs depend on the runner's speed: the slow-query rule runs the mock twice

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (rc.A CI, macOS runner) |
| Area | Test harness |
| Bash location | `tests/specs/llm_db_security_spec.sh` (MySQL and PostgreSQL client tests), `tests/specs/llm_db_names_spec.sh` (`__dbn_mocks`); the rule is `lib/modules/llm/core.sh:577` |
| Observed behavior | The specs record what a mock `mysql`/`psql` received (argv, stdin, options) and assert on it. When the call takes 100 ms or more, which on a slow runner the client spawn alone can (DB-06), `llm_db_query` runs the auto-EXPLAIN through the same mock, which overwrites the recorded files, and the assertions fail. Seen once on the macOS job. |
| Reproduction | the PostgreSQL client test on a loaded runner, or with a mock that sleeps 150 ms |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Fixed in the specs: after sourcing they pin `EPOCHREALTIME` to a constant, as `llm_portability_spec.sh` already does, so the elapsed time is 0 and the mock runs once. Assertions unchanged. |
| Fix recipe (3.0) | n/a after cutover (testscript, with the clock injected in `internal/db`). |
| Proving test | The three tests pass with a mock client that sleeps 150 ms. |
| Status | accept: done (rc.A); injected clock |

<a id="test-11"></a>
#### TEST-11 · Specs fake curl per test, but the binary speaks HTTP itself and never runs those fakes

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (rc.A, Go-mode HTTPS fake) |
| Area | Test harness |
| Bash location | `tests/specs/chrome_spec.sh` (the `MOCKS` curl stub and the "npm registry is unreachable" stub), `tests/specs/toolchains_spec.sh` (the `__gash_download` stub), `tests/specs/output_style_render_spec.sh` (six `chrome_headless_update` fixtures with a `curl()` function, also TEST-05); `tests/mocks/bin/curl` (its 404 and 500 answers) |
| Observed behavior | These fakes answer the argv the Bash implementation gives curl: `-fsIL` for a size probe, `-o FILE` for a download, `exit 22` or `exit 6` for a failure. The binary sends its requests with net/http, so in Go mode none of them runs and the requests reach the HTTPS fake, which knows only the shared mock's answers. Separately, the shared mock exits 0 with its 404 and 500 bodies even when the caller passed `-f`, where curl exits 22; the fake answers with real statuses. No spec relies on that today: the callers that pass `-f` (`chrome.sh`, `__gash_download`) are faked per test. |
| Reproduction | `bash tests/run.sh --impl go specs/chrome_spec.sh` once `chrome_headless_update` is ported |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Fixed in the harness: a test writes the HTTP answers it wants (method, URL pattern, status, headers, body, or "unreachable") to a route file named by `MOCK_CURL_ROUTES`. `tests/mocks/bin/curl` serves routed answers with curl's `-f`, `-I` and `-o` semantics (Bash mode), and the HTTPS fake serves the same file (Go mode). The per-test curl fakes listed above become route files; assertions stay byte for byte the same and the Bash reference stays 807/0. Without `MOCK_CURL_ROUTES` the mock and the fake answer as before. |
| Fix recipe (3.0) | n/a after cutover (testscript, with a local HTTPS server per script). |
| Proving test | The converted tests pass in both modes; a route answering 404 gives exit 22 from `curl -f` in Bash mode and the binary's exit 22 path in Go mode. |
| Status | accept: done (rc.A); per-script HTTPS servers |

<a id="test-12"></a>
#### TEST-12 · The settings leak check also scans the random temp path, which can hold a checked word

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (rc.A CI, Bash 5.0 job) |
| Area | Test harness |
| Bash location | `tests/specs/settings_spec.sh` "unknown settings and malformed lines warn with a line number, never any content" |
| Observed behavior | The test checks that no warning repeats content of the settings file (words such as `box`, `ftp`, `prod`, `pass`), but every warning names the file, whose path comes from `mktemp -d`. A random name such as `tmp.zw8oyhboxu` holds `box`, and the test fails although nothing leaked. Seen once in CI. |
| Reproduction | the test with `TMPDIR` set to a directory named `boxprod` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Fixed in the spec: the leak words are checked in the output with the temp path replaced by `TMP`; the line-number checks and the `~/` check still read the output as printed. |
| Fix recipe (3.0) | n/a after cutover (testscript, fixed work directory). |
| Proving test | The test passes with `TMPDIR` named `boxprod` and still fails when a warning prints a value from the file. |
| Status | accept: done 2566e9d |

<a id="test-13"></a>
#### TEST-13 · Specs call 2.x private helpers that rc.A does not have

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (rc.A port of llm_* and internal/db) |
| Area | Test harness |
| Bash location | `llm_spec.sh` (`__llm_validate_command`, `__llm_validate_path`, `__llm_is_secret_file`, `__llm_clean_error_msg`, `__llm_resolve_db`), `llm_exec_security_spec.sh` (two tests), `llm_db_security_spec.sh` (`__llm_sql_guard`, two tests), `llm_db_names_spec.sh` (`__llm_tsv_to_json`), `llm_portability_spec.sh` (`__llm_elapsed_us`, `__llm_realpath`), `llm_flags_spec.sh` (overrides `__llm_has_rg` and sources `lib/modules/llm/core.sh`), `help_spec.sh` (the seven `__gash_register_help` registry tests; core pins the registry in `internal/registry`), `files_spec.sh` (four tests calling `__gash_fs_parse_size` and `__gash_fs_human_size`; `TestParseSize` in `internal/fsscan`), `fs_render_spec.sh` (three tests overriding `__gash_fs_rich_ok` and `__gash_use_color` to force the terminal layout) |
| Observed behavior | These tests call or override `__llm_*` helpers of the Bash implementation. MATRIX lists them as moot internals (PRD 5.5): the generated shims do not define them, so the tests cannot run in Go mode. |
| Reproduction | `bash tests/run.sh --impl go specs/llm_spec.sh` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The tests stay as they are and run in Bash mode; Go mode reports them under TEST-13. Their rules are pinned by the Go unit tests of `internal/llmexec`, `internal/pathguard`, `internal/sqlguard` and `internal/llm`, which reuse their vectors. |
| Fix recipe (3.0) | n/a after cutover (testscript drives the public functions). |
| Proving test | PARITY lists each of these tests as TEST-13 in the Go column. |
| Status | accept: done (obsolete at cutover; rules pinned in llmexec, pathguard, sqlguard, llm) |

<a id="test-14"></a>
#### TEST-14 · Mock-client DB specs: the native drivers never run a mock mysql or psql

| Field | Value |
|---|---|
| Section | Defects |
| Origin | New finding (rc.A port of llm_* and internal/db) |
| Area | Test harness |
| Bash location | `llm_db_security_spec.sh` (MySQL and PostgreSQL client tests), `llm_db_names_spec.sh` (two tests), `llm_portability_spec.sh` (the decimal-comma clock test) |
| Observed behavior | The tests put a fake `mysql`/`mariadb`/`psql` on PATH and assert on the argv, stdin and environment it received. The binary connects with native drivers (DB-07, DB-09), so no client runs and nothing is recorded. |
| Reproduction | `bash tests/run.sh --impl go specs/llm_db_security_spec.sh` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The tests run in Bash mode; Go mode reports them under TEST-14. What they check is pinned in `internal/db`: `TestMySQLMatches2x` (read-only session, row cap, the database from `-d` and `.target-database`, the password never in argv or environment) and `TestPgsqlMatches2x` (read-only, standard strings and timeout settings, `current_database()`), plus the byte-for-byte comparison against real psql on a private PostgreSQL 16. |
| Fix recipe (3.0) | n/a after cutover (testscript with the in-process fakes). |
| Proving test | PARITY lists each of these tests as TEST-14 in the Go column. |
| Status | accept: done (rc.A); TestMySQLMatches2x, TestPgsqlMatches2x |

## Forced divergences

| Id | Title | Area | Severity | Contract |
|---|---|---|---|---|
| [LLM-01](#llm-01) | llm_grep --context is evaluated as Bash arithmetic | Security | critical | yes |
| [LLM-03](#llm-03) | llm_grep / llm_find --contains results depend on rg vs the grep fallback | Portability | medium | yes |
| [LLM-04](#llm-04) | llm_git_status emits invalid JSON with exit 0 when jq is absent | Contract | medium | yes |
| [LLM-16](#llm-16) | RIPGREP_CONFIG_PATH and rg global ignore files change llm_grep / llm_find output | Portability | low | yes |
| [LLM-21](#llm-21) | llm_docker_check clobbers the caller's shell variables | Self-management | low | no |
| [LLM-22](#llm-22) | llm_grep --ext leaks globals ext and exts into the caller | Self-management | low | no |
| [LLM-24](#llm-24) | trailing newlines in a PATH argument are stripped | Correctness | low | no |
| [LLM-26](#llm-26) | the llm_grep grep fallback has no -I, so binary files leak or misbehave | Portability | low | yes |
| [LLM-35](#llm-35) | llm_config output depends on which yq is installed | Portability | yes | The binary converts YAML itself, always to JSON: mapping order kept, numbers and booleans as YAML 1.2 reads them, the layout `jq .` gives python-yq's output. No yq runs; the raw-text fallback remains only for a file that does not parse. The flavour test is reported under LLM-35 in Go mode. |
| [LLM-36](#llm-36) | llm_tree and llm_find order entries by the locale's collation | Portability | yes | Bytewise order (the `C` order) in every locale. |
| [LLM-37](#llm-37) | llm_grep's regex dialect is ripgrep's Rust regex, or ERE in the grep fallback | Portability | yes | One engine, Go's RE2 (`regexp`): the Rust regex syntax rg accepts, minus the few constructs RE2 lacks, which are refused with `invalid_regex`. LLM-03 keeps its result rules. |
| [LLM-38](#llm-38) | A NUL late in a large file: rg searches the text before it | Portability | yes | A file with a NUL anywhere is binary and skipped whole, wherever the NUL is. |
| [LLM-39](#llm-39) | llm_env lists `_`, whose value is Bash's last argument | Correctness | yes | `_` is left out (in Go mode its value would be the binary's path). |
| [DB-03](#db-03) | the awk/TSV transport artifacts (dropped empty rows, raw control chars) vanish with native drivers | Correctness | medium | no |
| [DB-05](#db-05) | without jq the DB envelope silently becomes a raw result array | Contract | medium | yes |
| [DB-07](#db-07) | DB error `details` text comes from the native driver, not the client | Contract | low | yes |
| [DB-08](#db-08) | the SQLite EXPLAIN QUERY PLAN text tree must be rebuilt from plan rows | Contract | medium | yes |
| [DB-09](#db-09) | psql client meta-commands and PGPASSWORD-in-child are unreachable with the native driver | Security | medium | no |
| [DB-13](#db-13) | The SQL guard's case folding and spaces follow the locale | Security | medium | yes |
| [DB-15](#db-15) | sqlite3's JSON mode mangles bytes that are not UTF-8 and cuts text at a NUL | Correctness | yes | The driver returns the stored bytes and the contract encoder writes valid JSON (one U+FFFD per invalid sequence, `\u0000` for NUL), as DB-03 does for the TSV transport. |
| [DB-16](#db-16) | MySQL on localhost: the client uses the Unix socket and lets ~/.my.cnf override the URL | Correctness | yes | Host `localhost` connects through the Unix socket: the `socket=` of the `[client]` group of the option files the client reads (`/etc/my.cnf`, `/etc/mysql/my.cnf` with its `!include`/`!includedir`, `~/.my.cnf`), else the first existing platform default (`/run/mysqld/mysqld.sock`, `/var/run/mysqld/mysqld.sock`, `/tmp/mysql.sock`, `/var/lib/mysql/mysql.sock`). No other option-file setting applies: the `DB:` URL is the connection (lead decision, 2026-10-18). Any other host uses TCP. |
| [DB-17](#db-17) | Table and database names are checked with locale-dependent character ranges | Portability | yes | ASCII ranges in every locale, the stricter reading (as DB-13 does for the guard). |
| [DB-18](#db-18) | MySQL ZEROFILL columns lose their padding, MariaDB YEAR(2) prints four digits | Contract | low | yes |
| [EXEC-06](#exec-06) | glob expansion order differs under non-C collation | Portability | low | yes |
| [EXEC-07](#exec-07) | A trailing backslash makes llm_exec check one word and run another | Security | high | yes |
| [SYS-28](#sys-28) | hgrep passes PATTERN through awk `-v`: backslashes are rewritten and the regex dialect depends on the awk | Portability | medium | yes |
| [SYS-29](#sys-29) | hgrep prints a gawk warning for fixed patterns containing regex metacharacters | Portability | low | no |
| [SYS-30](#sys-30) | hgrep -n is compared with Bash arithmetic | Contract | low | yes |
| [SYS-31](#sys-31) | hgrep orders commands with the same timestamp by awk hash order | Portability | low | no |
| [SYS-32](#sys-32) | hgrep case folding, highlighting and timestamps depend on the awk and the locale | Portability | medium | no |
| [SYS-33](#sys-33) | port_kill accepts 2^64+N as port N | Contract | low | yes |
| [SYS-34](#sys-34) | sysinfo evaluates COLUMNS as Bash arithmetic (command substitution runs) | Security | medium | no |
| [SYS-35](#sys-35) | gash_ssh_auto_unlock glob-expands each key/passphrase row: keys dropped, passphrase printed | Security | high | no |
| [SYS-36](#sys-36) | ip_public and chrome_headless_update use net/http: the curl, wget and jq dependency errors become unreachable | Portability | low | yes |
| [SYS-37](#sys-37) | gash_ssh_auto_unlock drives ssh-add through a PTY: expect is no longer required | Portability | low | yes |
| [SYS-38](#sys-38) | history_grep and process_find match with grep's BRE: results depend on grep flavor and locale | Portability | low | no |
| [SYS-39](#sys-39) | The agent start evals ssh-agent's output in the caller's shell | Security | low | no |
| [SYS-40](#sys-40) | /etc/os-release is sourced as shell code | Security | low | no |
| [SYS-41](#sys-41) | sysinfo listing order follows LC_COLLATE and raw directory order | Portability | low | no |
| [SYS-42](#sys-42) | Shell state left behind by 2.x calls (sysinfo globals, BASH_REMATCH) | Contract | low | no |
| [DOCK-02](#dock-02) | A realpath without -m (BSD, macOS, BusyBox) makes the compose functions act on the current directory's stack | Portability | high | no |
| [DOCK-08](#dock-08) | A `\|` in an image shifts fields and injects keys into docker_compose_check --json | Security | medium | no |
| [DOCK-11](#dock-11) | Compose functions leave variables in the caller's shell, including the registry token | Security | medium | no |
| [DOCK-12](#dock-12) | Image interpolation reads shell-only variables and applies Bash pattern rules | Correctness | medium | no |
| [DOCK-19](#dock-19) | Bash's own diagnostics naming 2.x source lines (no terminal, failed redirections) | Contract | low | no |
| [DOCK-23](#dock-23) | curl behaviors in registry calls: URL globbing, ~/.curlrc, curl's proxy and CA variables, token in a curl config | Portability | low | no |
| [DOCK-24](#dock-24) | curl exit codes in the registry error vocabulary (`curl_error_N`, `no_response`) | Contract | low | yes |
| [DOCK-25](#dock-25) | Manifest digest headers of redirect hops are concatenated | Correctness | low | no |
| [DOCK-26](#dock-26) | Compose v2 without jq: Go always parses `docker compose config --format json` | Portability | low | no |
| [DOCK-27](#dock-27) | docker_compose_scan orders files by the locale's collation | Portability | low | no |
| [GIT-09](#git-09) | git_dump_revisions leaves `commit` in the caller's shell | Contract | low | no |
| [REM-04](#rem-04) | remote_exec quotes script arguments with Bash `printf %q` | Portability | medium | no |
| [DEV-09](#dev-09) | The copilot-instructions source depends on the PCRE2 version | Portability | medium | yes |
| [DEV-10](#dev-10) | jq's presence and version decide the composer.json and package.json sources | Portability | low | yes |
| [DEV-11](#dev-11) | A relative project_dir is resolved through CDPATH | Portability | low | no |
| [DEV-12](#dev-12) | php_raw_binary and composer_raw_binary come from Bash's command hash table | Portability | low | no |
| [DEV-13](#dev-13) | Version ordering depends on GNU `sort -V` | Portability | low | no |
| [DEV-14](#dev-14) | The caller's IFS breaks node_upgrade's plan | Correctness | low | no |
| [DEV-15](#dev-15) | `shopt -s failglob` leaks a Bash error from node_upgrade | Portability | low | no |
| [DEV-16](#dev-16) | nvm is a shell function: node_upgrade and nvm_prune_versions keep their nvm calls in the Bash shim | Contract | low | no |
| [AI-13](#ai-13) | The curl and jq dependency errors become unreachable (ai_sysinfo --raw included) | Portability | low | yes |
| [AI-14](#ai-14) | The AI transport no longer reads curl's configuration | Security | low | no |
| [AI-15](#ai-15) | Transport errors keyed on curl exit codes are mapped from Go errors | Contract | low | yes |
| [AI-16](#ai-16) | The caller's IFS joins the ai_query question | Portability | low | no |
| [AI-17](#ai-17) | `shopt -s nocasematch` changes provider and GASH_AI_CONFIRM matching | Portability | low | no |
| [AI-18](#ai-18) | Whether extglob is on decides which question paths ai_sysinfo reads | Portability | low | no |
| [AI-19](#ai-19) | The locale decides characters or bytes for the 4 KB cut marker, the "bytes" info and the consent size | Portability | low | no |
| [AI-20](#ai-20) | Bash's own diagnostics from the AI functions are not reproduced | Contract | low | no |
| [AI-21](#ai-21) | `grep -P` availability decides the context distro | Portability | low | no |
| [AI-22](#ai-22) | No request temp files, and no orphaned sudo keep-alive, after an interrupt | Security | low | no |
| [FS-20](#fs-20) | A newline inside an --exclude value turns it into extra find predicates | Security | high | yes |
| [FS-21](#fs-21) | dirs_largest / dirs_find_large --depth goes through Bash arithmetic | Contract | medium | yes |
| [FS-22](#fs-22) | The size grammar depends on numfmt, and thresholds overflow in Bash arithmetic | Portability | medium | yes |
| [FS-23](#fs-23) | awk -v unescapes the scan root and the --exclude values | Correctness | low | yes |
| [FS-24](#fs-24) | Without GNU du, prune names act as regexes and pruned trees still count | Portability | medium | yes |
| [FS-25](#fs-25) | A relative PATH is resolved through CDPATH | Portability | medium | yes |
| [FS-26](#fs-26) | A find without -printf (BusyBox, BSD, macOS) empties files_largest and tree_stats | Portability | medium | yes |
| [FS-27](#fs-27) | Without GNU du a newline-named directory is reported under a truncated, wrong path | Portability | medium | yes |
| [FS-28](#fs-28) | The order of equal entries depends on the locale and the awk implementation | Portability | low | yes |
| [FS-29](#fs-29) | Non-ASCII digits pass integer and size validation in UTF-8 locales | Portability | medium | yes |
| [FS-30](#fs-30) | An unusable TMPDIR forces errors to 0 and leaks a mktemp message | Portability | low | no |
| [FS-50](#fs-50) | tree_stats evaluates text from a scanned file name as Bash arithmetic | Security | critical | yes |
| [FILE-16](#file-16) | `nocasematch` stays on in the caller's shell after Ctrl-C | Contract | low | no |
| [FILE-17](#file-17) | `.tar.zst` exit status depends on the caller's `pipefail` | Correctness | medium | yes |
| [FILE-18](#file-18) | The caller's `noclobber` makes single-file extraction fail | Correctness | low | no |
| [FILE-19](#file-19) | Missing tools are reported by Bash's "command not found" text | Contract | low | yes |
| [FILE-20](#file-20) | User shell functions named like the tools are called by 2.x | Contract | low | no |
| [FILE-21](#file-21) | The `cp -v` line of file_backup depends on the locale | Portability | low | yes |
| [SHELL-01](#shell-01) | The prompt re-expands the git branch name, VIRTUAL_ENV, debian_chroot and STY as code | Security | critical | no |
| [SHELL-07](#shell-07) | GASH_PATH_ADD is word-split on the caller's IFS and glob-expanded | Correctness | low | no |
| [SHELL-10](#shell-10) | Loading leaves BASH_REMATCH and readonly or array internals in the caller's shell | Contract | low | no |
| [SHELL-13](#shell-13) | Headless load prints a plain-text 1.x notice and writes a state file | Contract | medium | yes |
| [SELF-11](#self-11) | Card and help lists sort by the user's locale | Portability | low | no |
| [SELF-16](#self-16) | gash_uninstall leaks the global variable profile_file into the caller's shell | Contract | low | no |
| [SELF-22](#self-22) | 3.0 drops the 1.x→2.x migration (D-6) | Self-management | low | no |
| [CORE-01](#core-01) | needs_confirm_prompt prints Bash's own /dev/tty error when there is no terminal | Contract | low | no |
| [CORE-02](#core-02) | Compat helpers evaluate numeric arguments as Bash arithmetic | Security | low | no |
| [CORE-05](#core-05) | gash_db_test behaviors that only the mysql/mariadb/psql clients produced | Contract | low | yes |
| [CORE-07](#core-07) | gash_help --list orders the modules outside its fixed list by Bash's hash order | Portability | low | no |
| [CORE-08](#core-08) | ~/.gash_env lines are trimmed with the locale's notion of whitespace | Portability | low | no |
| [CORE-09](#core-09) | __gash_json_str and __gash_json_arr fail without jq | Portability | low | no |
| [CORE-10](#core-10) | __gash_download's INT trap re-parses the destination path as shell code | Security | low | no |
| [CORE-16](#core-16) | Character counts in messages follow the locale (bytes in C, characters in UTF-8) | Portability | low | yes |
| [CORE-17](#core-17) | The mode in the ~/.gash_env permission warning depends on the installed stat | Portability | low | no |
| [CORE-18](#core-18) | Without the startup load, settings warnings come at the first call, before its argument checks | Contract | low | no |

### llm_* agent functions

<a id="llm-01"></a>
#### LLM-01 · llm_grep --context is evaluated as Bash arithmetic

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | Seed: "llm_grep --context unvalidated, Bash arithmetic (code execution), kills set -u" (CONFIRMED) |
| Area | Security |
| Bash location | `lib/modules/llm/fs.sh:382,439,459,463` |
| Observed behavior | `--context` is stored unvalidated and used in `[[ $context -gt 0 ]]`, so Bash arithmetic evaluates it: a command-substitution value executes code, a non-numeric value kills a `set -u` caller, `08` prints a base error, `1+1` reaches `rg -C 1+1` and silently empties output. |
| Reproduction | `env -i HOME=$t … llm_grep foo . --context abc` under `set -u` aborts the caller (effects stay in a temp dir); the code-execution variant is kept outside the repository |
| Severity | critical |
| Public-contract impact | yes |
| rc.A behavior | Go must never evaluate a user string as arithmetic. rc.A parses `--context` with `^[0-9]{1,9}$` and emits a new `invalid_context`/RETRY on failure; it does not reproduce the arithmetic or the `set -u` abort. |
| Fix recipe (3.0) | In the grep argument grammar (`internal/cli/args` llm family), after parsing `--context`, call the uint validator (name `context`); on failure emit `invalid_context` RETRY, details `Value 'V' is not a positive integer below 1000000000`, hint `Provide a plain integer for context`; add `invalid_context` to AGENTS.md section 5. Bash shim: `__llm_validate_uint context` before use. |
| Proving test | `TestLedger_LLM01` in `internal/cli/args` asserts `--context abc` -> `invalid_context` and no code runs; `testdata/script/llm_grep_context.txtar` asserts a `set -u` caller survives. |
| Status | accept: done 90a9fcd |

<a id="llm-03"></a>
#### LLM-03 · llm_grep / llm_find --contains results depend on rg vs the grep fallback

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | Seed: "rg vs grep fallback: regex syntax, hidden and ignored files differ" (CONFIRMED) |
| Area | Portability |
| Bash location | `lib/modules/llm/fs.sh:324-334,425-444,445-469,479-505` |
| Observed behavior | With rg present, matches use Rust-regex dialect and skip hidden/gitignored/binary files; without rg the GNU-grep fallback uses BRE, searches hidden and gitignored files, has no `-I`, and GNU `--exclude` even refuses an explicit command-line file. So the same query returns different lines depending on whether rg is installed. |
| Reproduction | `llm_grep 'TODO | FIXME' sub --ext js` over the fixture tree returns 2 lines with rg, 1 with the grep fallback (effects in a temp dir) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | One native engine with ripgrep semantics (Go `regexp`, `.gitignore`/`.ignore`/`.rgignore`, hidden and binary files skipped, deterministic path order). rc.A always behaves as the rg path did; the grep-fallback behavior (BRE, hidden/ignored searched, no `-I`, N18) is not reproduced. `RIPGREP_CONFIG_PATH` (LLM-16) and rg's nondeterministic order are also dropped. |
| Fix recipe (3.0) | `internal/fsscan` single engine + `internal/fsscan/ignore`; document RE2 syntax, hidden+ignored skipped, binary skipped, sorted path order; one code path for both `llm_grep` and `llm_find --contains`. |
| Proving test | `TestLedger_LLM03` runs the fixture-corpus vectors (TODO | FIXME, fo+, a{3}) and asserts identical output to the pinned golden, independent of any installed rg/grep. |
| Status | accept: done 63ee2b9 (docs) |

<a id="llm-04"></a>
#### LLM-04 · llm_git_status emits invalid JSON with exit 0 when jq is absent

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | Seed: "without jq, llm_git_status emits invalid JSON with exit 0" (CONFIRMED) |
| Area | Contract |
| Bash location | `lib/modules/llm/git.sh:86-94`; `lib/core/utils.sh:245-247,254-266` |
| Observed behavior | `__llm_git_status_impl` has no `__llm_has_jq` guard and calls `__gash_json_str`/`__gash_json_arr`, which shell out to jq. Without jq the branch/array values are empty (`"branch": ,`), stderr carries `jq: command not found`, and the function returns 0 with invalid JSON. |
| Reproduction | `PATH` without jq, then `llm_git_status` in a repo -> invalid JSON, `rc=0` (temp HOME) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | The Go binary builds JSON natively and never depends on jq, so rc.A always emits the jq-present valid JSON. The no-jq degraded path is a forced divergence: it cannot arise because there is no jq dependency. |
| Fix recipe (3.0) | None in Go beyond the native encoder (`internal/contract`). For a transitional Bash build, add the `llm_git_log` guard (`jq_not_found` FATAL) or render with `__gash_json_escape`. Keep the pretty 2-space layout byte-for-byte. |
| Proving test | `TestLedger_LLM04` validates `llm_git_status` output against `testdata/schema/git_status.json` with no jq on PATH (shim run). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="llm-16"></a>
#### LLM-16 · RIPGREP_CONFIG_PATH and rg global ignore files change llm_grep / llm_find output

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N9) |
| Area | Portability |
| Bash location | `lib/modules/llm/fs.sh:326,443` |
| Observed behavior | The rg calls carry no `--no-config`, so a hostile or unexpected `RIPGREP_CONFIG_PATH` (and rg's global ignore files) silently change which lines llm_grep / llm_find --contains return. |
| Reproduction | `printf -- '--ignore-case\n' > rgrc; RIPGREP_CONFIG_PATH=$PWD/rgrc llm_grep todo a.php` changes the match set (temp dir) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | The native engine reads no rg config and ignores `RIPGREP_CONFIG_PATH`; output is fixed by the engine's own ignore rules. The rg-config dependency cannot be reproduced. |
| Fix recipe (3.0) | None beyond the native engine; document that external rg config has no effect. |
| Proving test | `TestLedger_LLM16` runs `llm_grep` with a hostile `RIPGREP_CONFIG_PATH` set and asserts unchanged output. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="llm-21"></a>
#### LLM-21 · llm_docker_check clobbers the caller's shell variables

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N13) |
| Area | Self-management |
| Bash location | `lib/modules/docker-compose.sh:586-651` (undeclared loop vars); `:111-156` |
| Observed behavior | `docker_compose_check` runs many loop variables (`name tag registry f r svc img local_d remote_d stat upgr err service_name image_ref`) and registry globals without `local`, so after `llm_docker_check` the caller's `name`, `tag`, `f`, etc. are overwritten. |
| Reproduction | `name=MINE; llm_docker_check >/dev/null; declare -p name` shows `name=library/nginx` (temp dir) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | A Go binary runs in its own process and cannot set caller shell variables, so the leak is impossible; the one-line stub never exports anything. Forced divergence (a binary cannot reproduce the clobber). |
| Fix recipe (3.0) | Moot in Go. Bash fix if kept: declare all loop/registry vars `local` in `docker_compose_check`. |
| Proving test | `TestLedger_LLM21` (txtar) runs `llm_docker_check` through the shim and asserts the caller's `name`/`tag`/`f` are unchanged. |
| Status | accept: done (rc.A); TestLedger_LLM21.txtar |

<a id="llm-22"></a>
#### LLM-22 · llm_grep --ext leaks globals ext and exts into the caller

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N14) |
| Area | Self-management |
| Bash location | `lib/modules/llm/fs.sh:433-435,453-455` |
| Observed behavior | `__llm_grep_impl` assigns `ext` and `exts` without `local`, so `llm_grep … --ext php,js` leaves `ext=js` and `exts=(php js)` in the caller's shell. |
| Reproduction | `ext=MINE; llm_grep TODO . --ext php,js >/dev/null; declare -p ext exts` (temp dir) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | A Go binary cannot set caller variables; the stub leaks nothing. Forced divergence. |
| Fix recipe (3.0) | Moot in Go. Bash fix if kept: `local -a exts=(); local ext`. |
| Proving test | `TestLedger_LLM22` (txtar) asserts the caller's `ext`/`exts` survive a `--ext` call. |
| Status | accept: done (rc.A); TestLedger_LLM22.txtar |

<a id="llm-24"></a>
#### LLM-24 · trailing newlines in a PATH argument are stripped

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N16) |
| Area | Correctness |
| Bash location | `lib/modules/llm/core.sh:151`; callers `fs.sh:76,306,419,559`, `git.sh:41,130,177`, `project.sh:26,140` |
| Observed behavior | `__llm_validate_path` prints the resolved path with `echo` and every caller captures it with `$(...)`, which strips trailing newlines, so a directory whose name ends in a newline resolves to the wrong path. |
| Reproduction | dirs `x` and `x<NL>`: `llm_tree $'x\n'` lists `x` (temp dir) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Go passes paths as values, never through a `$(...)` capture, so a trailing-newline path is kept intact: rc.A diverges from 2.x (it does not strip). The `$(...)` accident cannot and should not be reproduced. |
| Fix recipe (3.0) | Moot in Go (paths are `[]byte`/`string` values). Bash fix if kept: return the path through a nameref. |
| Proving test | `TestLedger_LLM24` asserts a trailing-newline directory name is preserved end to end. |
| Status | accept: done (rc.A); TestLedger_LLM24 |

<a id="llm-26"></a>
#### LLM-26 · the llm_grep grep fallback has no -I, so binary files leak or misbehave

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N18) |
| Area | Portability |
| Bash location | `lib/modules/llm/fs.sh:446,462` |
| Observed behavior | The GNU-grep fallback is called without `-I`, so binary-file matches depend on the grep version (GNU >= 3.5 prints "binary file matches" on stderr and drops the line; older grep prints `Binary file X matches` on stdout), while rg skips binary files entirely. |
| Reproduction | a binary file containing the pattern gives different output with/without rg (temp dir) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | The single native engine skips binary files as rg does, so the grep-fallback behavior (and its version dependence) is not reproduced; folded into the one-engine decision (LLM-03). |
| Fix recipe (3.0) | None beyond the native engine's binary-file skip (`internal/fsscan`). Bash fix if the fallback is kept: add `-I`. |
| Proving test | Covered by `TestLedger_LLM03` (binary files skipped regardless of installed tools). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="llm-35"></a>
#### LLM-35 · llm_config output depends on which yq is installed

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of llm_* and internal/db) |
| Area | Portability |
| Bash location | `lib/modules/llm/fs.sh` llm_config (`yq . FILE` for python-yq, `yq eval -o=json . FILE` for mikefarah, raw text otherwise) |
| Observed behavior | A YAML file comes back as JSON from python-yq, as JSON in mikefarah's layout, or as the raw text when no known yq is installed or it fails: the result depends on an installed tool (PRD MUST NOT 8). |
| Reproduction | `llm_portability_spec.sh` "llm_config converts YAML with either yq flavour …" |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | The binary converts YAML itself, always to JSON: mapping order kept, numbers and booleans as YAML 1.2 reads them, the layout `jq .` gives python-yq's output. No yq runs; the raw-text fallback remains only for a file that does not parse. The flavour test is reported under LLM-35 in Go mode. |
| Fix recipe (3.0) | Same as rc.A. |
| Proving test | `TestLedger_LLM35` in `internal/llm` (the spec's YAML and the python-yq bytes; a parse error gives the raw text). |
| Status | accept: done 63ee2b9 (docs) |

<a id="llm-36"></a>
#### LLM-36 · llm_tree and llm_find order entries by the locale's collation

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of llm_* and internal/db) |
| Area | Portability |
| Bash location | `lib/modules/llm/fs.sh` (`sort` without `LC_ALL=C`) |
| Observed behavior | Entry order follows `LC_COLLATE`, so the same tree lists in different orders under `C` and `en_US.UTF-8` (case, punctuation, accents). |
| Reproduction | the same fixture tree under `LC_ALL=C` and `LC_ALL=en_US.UTF-8` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Bytewise order (the `C` order) in every locale. |
| Fix recipe (3.0) | Same as rc.A. |
| Proving test | `TestLedger_LLM36` in `internal/llm` (mixed-case and punctuation names in `C` order). |
| Status | accept: done (rc.A); test runnable 6bbfa68 |

<a id="llm-37"></a>
#### LLM-37 · llm_grep's regex dialect is ripgrep's Rust regex, or ERE in the grep fallback

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of llm_* and internal/db) |
| Area | Portability |
| Bash location | `lib/modules/llm/fs.sh` llm_grep (rg, else `grep -E`; LLM-03) |
| Observed behavior | Patterns are read by rg's Rust regex engine, or by the installed grep's ERE in the fallback; the two accept different syntax (`\d`, `\b`, Unicode classes, `{,n}`). 2.x ran rg with `2>/dev/null`, so a pattern rg's own parser rejected printed nothing and exited 0 (a silent empty success). |
| Reproduction | a pattern with `\d` with and without rg on PATH; a pattern rg itself rejects (`a**`, `[a-\d]`) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | One engine, Go's RE2 (`regexp`). Decision (operator go-ahead, "soundest option"): every pattern that cannot be read exactly as ripgrep 14 reads it fails with `invalid_regex` (RETRY), rg's own syntax errors included. This is the forced divergence from 2.x's silent-empty on rg errors; no spec depended on that behaviour (PRD MUST NOT: never silently return empty, never tool-dependent results). Where the two engines agree the dialect is translated, not refused: `\d`/`\w`/`\s` and negations become rg's Unicode ranges, numeric escapes and `\u`/`\U` become `\x{...}`, spaces in counted repetitions are dropped. `\b`/`\B` and invalid-UTF-8 bytes, where RE2's ASCII `\b` or `.` would differ from rg, are matched exactly by a small NFA over RE2's compiled program (Unicode word boundaries; an invalid byte matches nothing). Constructs RE2 misreads or lacks (`\<` `\>` `\b{start}`, class set ops, `\Q..\E`, octal/backrefs, Rust-only property names, `(?x)` etc.) are refused. LLM-03 keeps its result rules. Measured against rg 14.1.0. |
| Fix recipe (3.0) | Same as rc.A. |
| Proving test | `TestLedger_LLM37` in `internal/llm` (rg's documented syntax table: translated, matched exactly, or refused with `invalid_regex`, never misread), for both llm_grep and llm_find --contains. |
| Status | accept: done (rc.A); AGENTS table 517f190 |
| Note | `--ext`/`--glob` filter values are out of scope: a bad filter still matches nothing silently, as in 2.x (it reaches no regex engine and executes no code), so no divergence there. |

<a id="llm-38"></a>
#### LLM-38 · A NUL late in a large file: rg searches the text before it

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of llm_* and internal/db) |
| Area | Portability |
| Bash location | `lib/modules/llm/fs.sh` llm_grep (rg's binary detection) |
| Observed behavior | rg treats a file as binary at its first NUL and stops there, so matches before a NUL past its first buffer are still reported; the grep fallback behaves differently again (LLM-26). |
| Reproduction | a 1 MiB text file with a match, then a NUL at the end |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | A file with a NUL anywhere is binary and skipped whole, wherever the NUL is. |
| Fix recipe (3.0) | Same as rc.A. |
| Proving test | `TestLedger_LLM38` in `internal/llm` (a NUL at byte 0, in the middle and at the end: the file is skipped). |
| Status | accept: done (rc.A); test runnable 6bbfa68 |

<a id="llm-39"></a>
#### LLM-39 · llm_env lists `_`, whose value is Bash's last argument

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of llm_* and internal/db) |
| Area | Correctness |
| Bash location | `lib/modules/llm/system.sh` llm_env (`compgen -e`) |
| Observed behavior | `compgen -e` includes `_`, the last argument of the previous command, so its value says nothing about the environment and changes from call to call. |
| Reproduction | `llm_env --filter '^_$'` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | `_` is left out (in Go mode its value would be the binary's path). |
| Fix recipe (3.0) | Same as rc.A. |
| Proving test | `TestLedger_LLM39` in `internal/llm`. |
| Status | accept: done (rc.A); test runnable 6bbfa68 |

### databases

<a id="db-03"></a>
#### DB-03 · the awk/TSV transport artifacts (dropped empty rows, raw control chars) vanish with native drivers

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | Seed: "TSV->JSON drops single-column empty rows; raw leaves control chars" (CONFIRMED); also NF1 |
| Area | Correctness |
| Bash location | `lib/modules/llm/core.sh:624-694` (`__llm_tsv_to_json`: `NF==0` drop at :674, `esc_raw` at :627-632) |
| Observed behavior | The MySQL and PostgreSQL non-wrappable paths pipe client output through an awk TSV parser that (a) drops any row whose whole line is empty (a single empty-string column), undercounting `rows`, and (b) escapes only `\` and `"`, so a value with a C0 control char yields invalid JSON that breaks the envelope. SQLite is immune (native `-json`). |
| Reproduction | `printf 'name\nalice\n\nbob\n' | __llm_tsv_to_json raw` -> two rows, empty row dropped; a control char yields invalid JSON (temp dir) |
| Severity | medium |
| Public-contract impact | no (no spec pins the drop or the raw control char) |
| rc.A behavior | Not reproduced: there is no client and no TSV pipe. The native drivers return typed rows, so empty single-column rows are preserved (correct `rows`) and the contract encoder escapes control chars. The awk-TSV transport cannot be reproduced because the CLIs are gone. |
| Fix recipe (3.0) | None beyond using the native drivers and encoder in `internal/db`/`internal/contract`. |
| Proving test | `TestLedger_DB03` asserts an empty single-column row is counted and a control-char value round-trips as valid JSON through the native path. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="db-05"></a>
#### DB-05 · without jq the DB envelope silently becomes a raw result array

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | Seed: "without jq the DB envelope silently becomes raw" (CONFIRMED) |
| Area | Contract |
| Bash location | `lib/modules/llm/core.sh:574-593` (no-jq branch at :591-592) |
| Observed behavior | `__llm_db_envelope` builds `{data,rows,query_time}` with jq; when jq is absent it just prints the impl's raw stdout with no envelope and no timing, and emits no `jq_not_found` error although AGENTS.md section 5 lists it as FATAL. |
| Reproduction | jq removed from PATH, then `llm_db_query "SELECT 1 AS id" -c <conn>` -> raw array, no envelope (temp HOME; mock client) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Not reproduced: the Go binary builds JSON natively and always emits `{data,rows,query_time}`; the no-jq degraded path cannot arise because there is no jq dependency. |
| Fix recipe (3.0) | None beyond the native encoder in `internal/contract`. |
| Proving test | `TestLedger_DB05` asserts `llm_db_query` with no jq on PATH still yields the `{data,rows,query_time}` envelope. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="db-07"></a>
#### DB-07 · DB error `details` text comes from the native driver, not the client

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (B.8 item 4) |
| Area | Contract |
| Bash location | `lib/modules/llm/core.sh:476-486` (`__llm_clean_error_msg`); `db.sh:338-345,360-389,395-436` |
| Observed behavior | `sqlite_error`/`mysql_error`/`pgsql_error` details are the client's stderr, cleaned (newlines to spaces, runs collapsed, trimmed; mysql drops two warning lines) — e.g. the `psql:<stdin>:N: ERROR: …` prefix. |
| Reproduction | n/a (depends on the client binary) |
| Severity | low |
| Public-contract impact | yes (details text only) |
| rc.A behavior | The native drivers (pgx, go-sql-driver/mysql, modernc sqlite) produce their own error strings (e.g. `ERROR: … (SQLSTATE 42P01)`), so the `details` text differs; the `*_error` code and RETRY action are preserved. No spec pins the exact text; the only load-bearing fragment is the sqlite plan `test("u")` substring (DB-08). The client binary cannot be reproduced. The client-not-found errors of llm_db_* (`sqlite_not_found`, `mysql_not_found`, `psql_not_found`, and gash_db_test's, CORE-05) cannot occur either, since no client runs; the codes stay registered. |
| Fix recipe (3.0) | In `internal/db` keep the `*_error` codes and RETRY action; map driver errors to a cleaned `details` string; document that the message text is driver-sourced. |
| Proving test | `TestLedger_DB07` asserts each engine's error yields the correct `*_error` code and RETRY action with a non-empty cleaned `details`. |
| Status | accept: done 6682f4b (documented) |

<a id="db-08"></a>
#### DB-08 · the SQLite EXPLAIN QUERY PLAN text tree must be rebuilt from plan rows

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (B.8 item 3) |
| Area | Contract |
| Bash location | `lib/modules/llm/db.sh:696-744` (sqlite branch), rendering at `:739-744` |
| Observed behavior | The sqlite3 CLI special-cases EXPLAIN QUERY PLAN and renders a text tree in every output mode (`QUERY PLAN` then ``-- `` / `` | -- `` connectors), which llm_db_explain wraps as `[{plan:TEXT}]`. This is pinned by `envelope_schema_spec.sh:88`, `llm_db_security_spec.sh:69-70` (`.data[0].plan | test("u")`) and AGENTS.md section 4.4. |
| Reproduction | `llm_db_explain "SELECT * FROM u WHERE id=1" -f db.sqlite` -> `[{"plan":"QUERY PLAN\n`--…"}]` (temp sqlite) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | modernc sqlite's `EXPLAIN QUERY PLAN` returns rows (id, parent, notused, detail), not the tree, so rc.A must synthesize the `QUERY PLAN` text tree from those rows (indentation and ``-- ``/`` | -- `` connectors by id/parent) to match the pinned string. The CLI's own rendering cannot be reproduced. The plan content itself follows the SQLite engine: in 2.x the installed sqlite3 (3.45 here), in rc.A the embedded modernc engine (3.53), so the plan of the same query can differ (recursive CTEs). |
| Fix recipe (3.0) | In `internal/db` build the text tree from the plan rows in the sqlite3 CLI layout; pin it with the envelope schema. |
| Proving test | `TestLedger_DB08` asserts the synthesized plan string equals the sqlite3 CLI tree and matches `test("u")`. |
| Status | accept: done (rc.A); TestLedger_DB08 |

<a id="db-09"></a>
#### DB-09 · psql client meta-commands and PGPASSWORD-in-child are unreachable with the native driver

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (B.8 item 6; architecture 9) |
| Area | Security |
| Bash location | `lib/modules/llm/db.sh:395-436` (psql transport, PGPASSWORD at :416-420) |
| Observed behavior | 2.x drives PostgreSQL through the psql client over `-f -`, which processes backslash meta-commands (`\!`, `\copy`, `\i`) and receives the password through `PGPASSWORD` in a child environment; those client features are the enabler for the DB-01 shell-execution impact. |
| Reproduction | n/a (consequence of the transport; the DB-01 mechanism covers the exploit, kept outside the repository) |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | With pgx the query string goes straight to the server, which never processes psql meta-commands, so `\!`/`\copy`/`\i` cannot run and no password reaches a child process; the psql transport cannot be reproduced. This removes the DB-01 impact in rc.A even though the guard lexer quirk is still reproduced. |
| Fix recipe (3.0) | None beyond using pgx in `internal/db`; document that psql meta-commands are not available. |
| Proving test | `TestLedger_DB09` asserts a query that would be a psql meta-command reaches the server as SQL (and errors there), never running a shell. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="db-13"></a>
#### DB-13 · The SQL guard's case folding and spaces follow the locale

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of `internal/config` and `internal/sqlguard`) |
| Area | Security |
| Bash location | `lib/modules/llm/db.sh` guard (`${q^^}`, `[[:space:]]`, `[A-Z0-9_]` word scan, dot-command trim) |
| Observed behavior | Upper-casing and `[[:space:]]` follow `LC_CTYPE`. In a UTF-8 locale `ſ` folds to S and `ı` to I, and U+3000 is a space; in C they are not. So `SELECT ſleep(1)` is refused in UTF-8 and accepted in C, while `ſelect 1` is accepted in UTF-8 and refused in C. |
| Reproduction | the same harmless query under `LC_ALL=C` and `LC_ALL=C.UTF-8` gets different verdicts (temp SQLite) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | The libc tables cannot be reproduced, and PRD 10.5 forbids loosening the rail, so the guard checks every query under three readings in a fixed order (C; UTF-8 with the case mappings glibc makes to ASCII, `ſ` and `ı`; UTF-8 that also folds runes whose case orbit holds an ASCII letter, such as the Kelvin sign) and refuses with the first refusal. Blanks in the UTF-8 readings are Unicode spaces plus ASCII, except in the MySQL `--` comment rule, where a wider set would hide text: there they are glibc's exact `iswspace` set. A query passes only when every reading accepts, so nothing 2.x refuses in C or UTF-8 is accepted; a few queries no 2.x locale refuses (an NBSP or Kelvin sign inside a keyword) are refused. |
| Fix recipe (3.0) | Same as rc.A. |
| Proving test | `TestLedger_DB13` (19 vectors checked against 2.x in both locales) and `TestLedger_DB13_Monotonic` (a refusal under any reading is a refusal) in `internal/sqlguard`. |
| Status | accept: done (rc.A); TestLedger_DB13, TestLedger_DB13_Monotonic |

<a id="db-15"></a>
#### DB-15 · sqlite3's JSON mode mangles bytes that are not UTF-8 and cuts text at a NUL

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of llm_* and internal/db) |
| Area | Correctness |
| Bash location | `lib/modules/llm/db.sh` SQLite path (`sqlite3 -json`, 3.45) |
| Observed behavior | Values with invalid UTF-8 come out as sign-extended `\uffffffff` escapes (not valid JSON), and text with a NUL is cut at the NUL. |
| Reproduction | a table with `X'ff'` and `'a'||char(0)||'b'` (temp SQLite) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | The driver returns the stored bytes and the contract encoder writes valid JSON (one U+FFFD per invalid sequence, `\u0000` for NUL), as DB-03 does for the TSV transport. |
| Fix recipe (3.0) | Same as rc.A. |
| Proving test | `TestLedger_DB15` in `internal/db`. |
| Status | accept: done (rc.A) |

<a id="db-16"></a>
#### DB-16 · MySQL on localhost: the client uses the Unix socket and lets ~/.my.cnf override the URL

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of llm_* and internal/db) |
| Area | Correctness |
| Bash location | `lib/modules/llm/db.sh:374` (`--defaults-extra-file` with `host=localhost`) |
| Observed behavior | libmysqlclient connects to `localhost` through the Unix socket (the `[client] socket` of its option files, else the built-in path), not TCP. It also reads `~/.my.cnf` after the extra file, so a user, password or host there overrides the `DB:` URL. |
| Reproduction | `DB:x=mysql://u:p@localhost/app` with MySQL listening on the socket only; a `~/.my.cnf` with another `user=` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Host `localhost` connects through the Unix socket: the `socket=` of the `[client]` group of the option files the client reads (`/etc/my.cnf`, `/etc/mysql/my.cnf` with its `!include`/`!includedir`, `~/.my.cnf`), else the first existing platform default (`/run/mysqld/mysqld.sock`, `/var/run/mysqld/mysqld.sock`, `/tmp/mysql.sock`, `/var/lib/mysql/mysql.sock`). No other option-file setting applies: the `DB:` URL is the connection (lead decision, 2026-10-18). Any other host uses TCP. |
| Fix recipe (3.0) | Same as rc.A. |
| Proving test | `TestLedger_DB16` in `internal/db` (socket path from an option-file fixture, from the defaults, and a `~/.my.cnf` user that does not apply). |
| Status | accept: done (rc.A); TestLedger_DB16 |

<a id="db-17"></a>
#### DB-17 · Table and database names are checked with locale-dependent character ranges

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of llm_* and internal/db) |
| Area | Portability |
| Bash location | `lib/modules/llm/db.sh` name checks (`[[ =~ ^[A-Za-z0-9_]+$ ]]` and similar) |
| Observed behavior | Bracket ranges follow the locale's collation, so in some locales they also match letters outside ASCII. |
| Reproduction | a name with an accented letter under `C` and a UTF-8 locale |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | ASCII ranges in every locale, the stricter reading (as DB-13 does for the guard). |
| Fix recipe (3.0) | Same as rc.A. |
| Proving test | `TestLedger_DB17` in `internal/db`. |
| Status | accept: done (rc.A) |

<a id="db-18"></a>
#### DB-18 · MySQL ZEROFILL columns lose their padding, MariaDB YEAR(2) prints four digits

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of internal/db, go-sql-driver/mysql v1.10.1) |
| Area | Contract |
| Bash location | `lib/modules/llm/db.sh` MySQL path (`mysql --batch` prints the server's text) |
| Observed behavior | A `ZEROFILL` integer column comes back padded (`00042`), as the server sends it. |
| Reproduction | `CREATE TABLE t (n INT(5) ZEROFILL); INSERT INTO t VALUES (42);` then `llm_db_query "SELECT n FROM t"` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | `"42"`: the driver parses the column to an integer before Gash sees it, and the zerofill flag is not exposed, so the padding cannot be rebuilt. The other numeric types keep the client's text (DB-02). `ZEROFILL` is deprecated since MySQL 8.0.17. For the same reason (the column length is not exposed), a MariaDB `YEAR(2)` column prints four digits. The alternatives were pinning the driver to v1.7.1 (no ed25519 authentication, three years of fixes missing) or carrying a patched fork (lead decision, 2026-10-18). |
| Fix recipe (3.0) | Same as rc.A, unless DB-02's operator decision makes MySQL values typed, which removes the question. |
| Proving test | `TestLedger_DB18` in `internal/db` (in-process fake sending `00042` for a zerofill column → `"42"`; YEAR `0000` → `"0000"`; DOUBLE `1e20` → `"1e20"`; FLOAT `1234570` → `"1234570"`). |
| Status | accept: done (rc.A) |

### llm_exec

<a id="exec-06"></a>
#### EXEC-06 · glob expansion order differs under non-C collation

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (architecture 8.3) |
| Area | Portability |
| Bash location | `lib/modules/llm/exec.sh:633-646` (globs expanded via `compgen -G`) |
| Observed behavior | llm_exec expands glob words with `compgen -G`, whose match order follows the shell's locale collation, so the order of expanded words (and thus argv order passed to the allowed command) depends on `LC_COLLATE`. |
| Reproduction | n/a (locale-dependent ordering; effects confined to a temp dir) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Go expands globs itself against the same cwd tracking it validated, in byte (C) order; this differs from 2.x only under non-C collation. Go must never hand an unexpanded or re-globbable word to the shell, so the locale-dependent order cannot be reproduced. |
| Fix recipe (3.0) | Document byte-order glob expansion in `internal/llmexec`; the canonical single-quoted script (operator decision D-1 option A) carries the already-expanded words so bash does no globbing. |
| Proving test | `TestLedger_EXEC06` asserts glob expansion is byte-ordered and stable regardless of `LC_COLLATE`. |
| Status | accept: done (rc.A); docs 30410b6 |

<a id="exec-07"></a>
#### EXEC-07 · A trailing backslash makes llm_exec check one word and run another

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of the llm_exec renderer) |
| Area | Security |
| Bash location | `lib/modules/llm/exec.sh:834-837` |
| Observed behavior | When the command line ends with a backslash, the 2.x parser drops it and checks the word without it, but bash keeps the backslash as a literal character. The path and secret-file checks therefore run on one name and the command opens another, which can reach a file the rules refuse (for example through a link whose name ends with a backslash). |
| Reproduction | in a temp dir, a link whose name ends with a backslash points at a secret-pattern marker; a read command naming the link with the backslash last prints the marker (input shapes only, no working line) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Go never runs a word it did not check (PRD 10.7), so 2.x cannot be reproduced. A command line that ends with an unquoted backslash is refused with `construct_not_allowed` (RETRY), the error contract 2.x already uses for constructs it does not handle. The refusal comes after the other checks of the line, so a line 2.x refused for another reason keeps that refusal. A backslash before a newline stays a line continuation, and a backslash inside quotes or a comment behaves as in 2.x. Unrestricted mode validates nothing and is unchanged. |
| Fix recipe (3.0) | Keep the refusal. For 2.x, the same refusal at the end-of-input branch of the parser. |
| Proving test | `TestLedger_EXEC07` asserts a line ending with a backslash gives `construct_not_allowed` and runs nothing. |
| Status | accept: done (rc.A); TestLedger_EXEC07 runs without 2.x (30410b6) |

### system, sysinfo, ssh, chrome

<a id="sys-28"></a>
#### SYS-28 · hgrep passes PATTERN through awk `-v`: backslashes are rewritten and the regex dialect depends on the awk

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N1) |
| Area | Portability |
| Bash location | `lib/modules/system.sh:304-307` (`-v pattern=`, `tolower(pattern)`), `lib/modules/system.sh:314-317`, `lib/modules/system.sh:438,527` (`-v hlpat=`) |
| Observed behavior | awk applies its escape processing to `-v` values: `hgrep 'C:\new'` finds nothing on gawk and mawk (`\n` became a newline); gawk turns `\.` into `.` with a warning, so `hgrep -E 'a\.b'` also matches `axb`, while mawk keeps `\.`; gawk-only operators (`\y`, `\<`) are destroyed. The pattern is lowercased before it is compiled, so `\S`, `\W`, `\D` become `\s`, `\w`, `\d`. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; printf "#1700000000\ncmd a.b\n#1700000100\ncmd axb\n" >"$HOME/h"; HISTFILE="$HOME/h" hgrep -E "a\.b"'` (with gawk: two warnings and both commands) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | The pattern reaches the matcher verbatim, since no awk's rewriting is a contract and the two disagree: fixed mode is a case-insensitive (Unicode simple folding, SYS-32) substring test, `-E` compiles `(?i)` + PATTERN with Go regexp and matches the original command; highlighting uses the same matcher. Syntax Go regexp lacks (`\y`, `\<`, `\>`, back-references) takes the invalid-regex path of SYS-05. |
| Fix recipe (3.0) | Keep the rc.A matcher in `internal/system` `hgrep`. Help OPTIONS (3.0, with SYS-47): `-E, --regex  PATTERN is a Go regular expression (RE2 syntax), case-insensitive`; docs/troubleshooting.md drops the awk advice for hgrep. |
| Proving test | `TestLedger_SYS28` in `internal/system` (`C:\new` matches literally; `-E 'a\.b'` does not match `axb`; `-E '\S+'` matches non-space; stderr empty) |
| Status | accept: done 105f468 |

<a id="sys-29"></a>
#### SYS-29 · hgrep prints a gawk warning for fixed patterns containing regex metacharacters

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N2) |
| Area | Portability |
| Bash location | `lib/modules/system.sh:399-402` (sed-escaped `hlpat`), `lib/modules/system.sh:524-527` (passed with `-v` even when unused) |
| Observed behavior | Under gawk, `hgrep 'a.b'` with a matching entry prints `gawk: warning: escape sequence '\.' treated as plain '.'` on stderr, in plain mode too; mawk prints nothing. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; printf "#1700000000\ncmd a.b\n" >"$HOME/h"; HISTFILE="$HOME/h" hgrep a.b >/dev/null'` (needs gawk on PATH) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | No warning: rc.A runs no awk; stdout as on the gawk path. |
| Fix recipe (3.0) | None beyond rc.A (`internal/system` builds the highlight from the matcher, only when colors are on). |
| Proving test | `TestLedger_SYS29` in `internal/system` (`hgrep a.b` with a match → stderr empty) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="sys-30"></a>
#### SYS-30 · hgrep -n is compared with Bash arithmetic

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N4) |
| Area | Contract |
| Bash location | `lib/modules/system.sh:221` (digits check only), `lib/modules/system.sh:329` (`[[ "$limit" -gt 0 ]]`) |
| Observed behavior | `-n 08` is octal to `[[ -gt ]]`: stderr `[[: 08: value too great for base (error token is "08")` and every result is printed, rc 0. A value over the 64-bit range reaches `tail -n` and fails with `tail: invalid number of lines` rc 1. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; printf "#1700000000\ngit a\n#1700000100\ngit b\n" >"$HOME/h"; HISTFILE="$HOME/h" hgrep git -n 08'` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | The value is parsed as decimal (`08` is 8, leading zeros allowed, 0 = all) and never evaluated; a value above 2^63-1 gets `Error: Option -n requires a numeric argument` rc 1. |
| Fix recipe (3.0) | `internal/cli/args` hgrep grammar: at most 9 digits, like the llm family's limit rule; longer → `Error: Option -n requires a numeric argument` rc 1. |
| Proving test | `TestLedger_SYS30` in `internal/cli/args` (`08` → 8; `1234567890` → error) |
| Status | accept: done 105f468 |

<a id="sys-31"></a>
#### SYS-31 · hgrep orders commands with the same timestamp by awk hash order

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N6) |
| Area | Portability |
| Bash location | `lib/modules/system.sh:327-328` (`for (c in last)` then a stable numeric sort) |
| Observed behavior | Equal epochs keep the awk's hash iteration order: three commands at `#1705330400` print `zeta tie, mid tie, alpha tie` with gawk 5.2 and `alpha tie, zeta tie, mid tie` with mawk 1.3.4. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 GASH_AWK=mawk bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; printf "#1705330400\nzeta tie\n#1705330400\nmid tie\n#1705330400\nalpha tie\n" >"$HOME/t"; HISTFILE="$HOME/t" hgrep tie'` (compare with `GASH_AWK=gawk`) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Deterministic: ties keep the order of each command's last occurrence in the file (reversed with `-r`). Neither awk order is reproducible or documented. |
| Fix recipe (3.0) | None beyond rc.A; note the tie rule in the help NOTES (3.0). |
| Proving test | `TestLedger_SYS31` in `internal/system` (three equal epochs → file order, `-r` → reverse) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="sys-32"></a>
#### SYS-32 · hgrep case folding, highlighting and timestamps depend on the awk and the locale

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N7; strftime availability from N9 e) |
| Area | Portability |
| Bash location | `lib/modules/system.sh:307,314` (`tolower`), `lib/modules/system.sh:439,528` (`IGNORECASE = 1`), every `strftime` in the renderers; `__GASH_AWK` in `lib/core/utils.sh:10-16` |
| Observed behavior | gawk in a UTF-8 locale folds non-ASCII (`hgrep école` finds `ÉCOLE`), mawk folds ASCII only; highlighting is case-insensitive only under gawk (`IGNORECASE`); BSD awk and old mawk lack `strftime`, so every text and JSON mode dies there. With gawk in the C locale folding is ASCII too. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 GASH_AWK=mawk bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; printf "#1700000000\nÉCOLE upper\n" >"$HOME/h"; HISTFILE="$HOME/h" hgrep école'` (nothing; with `GASH_AWK=gawk` one row) |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | The gawk path in a UTF-8 locale, whatever is installed: Unicode simple case folding for matching and highlighting (`strings.EqualFold` rules, `(?i)` for `-E`), timestamps from Go `time` in local time. `GASH_AWK` no longer has an effect (MATRIX `drop`). |
| Fix recipe (3.0) | None beyond rc.A; docs/troubleshooting.md loses the "hgrep needs gawk/strftime" entry; `GASH_AWK` is removed from docs/configuration.md once no 3.x function uses awk. |
| Proving test | `TestLedger_SYS32` in `internal/system` (`école` finds `ÉCOLE`; on a fake TTY `git` highlights `GIT`; same result with `LC_ALL=C`) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="sys-33"></a>
#### SYS-33 · port_kill accepts 2^64+N as port N

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N15) |
| Area | Contract |
| Bash location | `lib/modules/system.sh:727-731` (`10#$port` wraps in 64-bit arithmetic) |
| Observed behavior | Any digit string passes the regex and is reduced modulo 2^64: `port_kill 18446744073709551696` acts on port 80. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; port_kill 18446744073709551696'` (`Error: Nothing is listening on port 80.` instead of `Invalid port`) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Digits are compared as a decimal number without wraparound (leading zeros still accepted: `018080` is 18080): anything outside 1-65535 gets `Error: Invalid port: X (expected 1-65535).` rc 1. |
| Fix recipe (3.0) | None beyond rc.A (`internal/cli/args` port_kill grammar). |
| Proving test | `TestLedger_SYS33` in `internal/cli/args` (`18446744073709551696` → invalid; `018080` → 18080) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="sys-34"></a>
#### SYS-34 · sysinfo evaluates COLUMNS as Bash arithmetic (command substitution runs)

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N27; owner of the helper: core output) |
| Area | Security |
| Bash location | `lib/core/output.sh:542-544` (`((width > 60))`, `i < width` on `${COLUMNS:-80}`), reached from `lib/modules/sysinfo.sh:126` |
| Observed behavior | A COLUMNS value holding an array subscript with a command substitution is evaluated in the arithmetic context and `cmd` runs (twice for one section rule). A plain word gives width 0 (empty rule). Only the verbose mode prints the rule; `--llm` does not. |
| Reproduction | the reproduction kept outside the repository: `sysinfo mail` in headless mode with such a COLUMNS value whose command only prints a marker on stderr; the marker is printed twice. Harmless check: `COLUMNS=abc` gives an empty section rule. |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | COLUMNS is used only when it is a run of digits (read as 2.x arithmetic reads digits: a leading 0 is octal); anything else is treated as unset (80), so the rule is 60 glyphs. It is never evaluated. The generated `__gash_section` shim follows the same rule. |
| Fix recipe (3.0) | None beyond rc.A in `internal/ui` (one COLUMNS parser for every primitive); the generated `lib/core/output.sh` shim checks `[[ $COLUMNS =~ ^[0-9]+$ ]]` before any arithmetic. |
| Proving test | `TestLedger_SYS34` in `internal/ui` and `testdata/script/ledger_sys34.txtar` (that COLUMNS → no `INJECTED`, a 60-glyph rule) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="sys-35"></a>
#### SYS-35 · gash_ssh_auto_unlock glob-expands each key/passphrase row: keys dropped, passphrase printed

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N29); lead decision: forced divergence |
| Area | Security |
| Bash location | `lib/modules/ssh.sh:242,249` (`local IFS=$'\n'; for row in $ssh_keys`) |
| Observed behavior | Each `KEYPATH<TAB>PASSPHRASE` row is an unquoted word, so the caller's glob options apply: with `shopt -s nullglob` a row whose passphrase has `*`, `?` or `[` vanishes (the key is silently not added, RAN=1); with `shopt -s failglob` Bash prints `no match: KEYPATH<TAB>PASSPHRASE` on stderr, disclosing the passphrase, and a non-interactive caller exits. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH="$PWD/tests/mocks/bin:/usr/bin:/bin" LANG=C.UTF-8 MOCK_SSH_AGENT=1 bash -c 'mkdir -p ~/.ssh; : >~/.ssh/key1; printf "SSH:~/.ssh/key1=pa*ss\n" >~/.gash_env; chmod 600 ~/.gash_env; GASH_HEADLESS=1; source "$G/gash.sh"; shopt -s failglob; gash_ssh_auto_unlock'` (fake passphrase `pa*ss` appears on stderr) |
| Severity | high |
| Public-contract impact | no |
| rc.A behavior | rc.A iterates the parsed entries of `internal/config` (no word splitting, no globbing, the caller's shopts are invisible to the binary): every key is offered, nothing is printed; this is the default-shopt 2.x result. |
| Fix recipe (3.0) | None beyond rc.A in `internal/ssh`. For any 2.x maintenance release: `while IFS=$'\t' read -r keyfile password; do ...; done <<<"$ssh_keys"`. |
| Proving test | `TestLedger_SYS35` in `internal/ssh` (passphrases `pa*ss`, `a?b`, `[x]` → both keys reach the PTY driver, stderr empty) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="sys-36"></a>
#### SYS-36 · ip_public and chrome_headless_update use net/http: the curl, wget and jq dependency errors become unreachable

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (lead decision: curl/wget calls become net/http) |
| Area | Portability |
| Bash location | `lib/modules/system.sh:567-575` (wget, else curl, else error), `lib/modules/chrome.sh:67-99,129-130` (curl and jq required), `lib/core/utils.sh:437-466` (`__gash_remote_size`, `__gash_download`) |
| Observed behavior | Without wget and curl ip_public fails with `Error: This function requires either 'wget' or 'curl' to be installed.`; chrome fails with `Error: chrome_headless_update needs 'curl'.` or `... needs 'jq'.`. With both tools, ip_public uses wget, which returns nothing on an HTTP error status, while the curl path (no `-f`) prints the error body as the address. curl and wget also read `~/.curlrc`, `~/.wgetrc` and their own proxy variables. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/nonexistent /bin/bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; ip_public; echo rc=$?'` (the wget/curl error, rc 1) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | The three messages are never printed: the requests go through net/http (proxy from `HTTPS_PROXY`/`NO_PROXY`, CA from the system or `SSL_CERT_FILE`, no rc files). ip_public takes the wget path: a non-2xx status is `Error: Failed to retrieve public IP address.` rc 1. chrome keeps requiring unzip and ldd (still CLIs). The specs `functions_smoke_spec.sh` "ip_public fails gracefully without curl/wget" and the render fixture `ip_public no-tool` become `n/a` in Go mode; chrome specs reach a local HTTPS fake through `HTTPS_PROXY` (`testing.md` 2.4). |
| Fix recipe (3.0) | None beyond rc.A: remove the three messages from the registry `Errors` lists and the curl/jq requirement from `docs/reference/dev.md` and the chrome help NOTES (help bodies unchanged in rc.A). |
| Proving test | `TestLedger_SYS36` in `internal/system` and `internal/dev` (empty PATH: ip_public reaches the fake and prints `Public IP: 203.0.113.7`; a 503 → failure message, rc 1) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="sys-37"></a>
#### SYS-37 · gash_ssh_auto_unlock drives ssh-add through a PTY: expect is no longer required

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (design: `architecture.md` 7, ssh-add through `creack/pty`) |
| Area | Portability |
| Bash location | `lib/modules/ssh.sh:227-235` (missing expect: warning and hint), `lib/modules/ssh.sh:130-172` (generated expect script), `lib/modules/ssh.sh:282-287` (temp script, payload on stdin); docs `docs/reference/remote.md:43-48`, `docs/configuration.md:106-108`, registered help REQUIREMENTS `lib/modules/ssh.sh:321-323` |
| Observed behavior | Without expect, 2.x prints `Warning: 'expect' is not installed.` plus a distro install hint on stdout, returns 0 and leaves RAN unset, so every new shell repeats it. With expect, a script in `${TMPDIR:-/tmp}/gash-ssh-expect.XXXXXX` receives base64 key/passphrase pairs on stdin and spawns `ssh-add -- KEY` per key. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH="$PWD/tests/mocks/bin:/usr/bin:/bin" LANG=C.UTF-8 MOCK_SSH_AGENT=1 bash -c 'mkdir -p ~/.ssh ~/bin; : >~/.ssh/key1; printf "SSH:~/.ssh/key1=pw\n" >~/.gash_env; chmod 600 ~/.gash_env; for t in ssh-add ssh-agent ssh-keygen bash env base64 tr sed; do ln -s "$(command -v $t)" ~/bin/; done; GASH_HEADLESS=1; source "$G/gash.sh"; PATH=~/bin gash_ssh_auto_unlock'` (warning and hint) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | The binary spawns `ssh-add -- KEY` on a PTY, answers `Enter passphrase.*:` with the passphrase (from memory, never argv, env or a file), stops on the same four patterns, eof or 20 s, and formats the transcript with the 2.x line rules (`Identity added` success, `Bad passphrase` error, ...). The missing-expect warning, the hint, the temp script and the `Invalid input` exit are unreachable; keys are unlocked where 2.x refused. `ssh_auto_unlock_spec.sh` "prints install hint when expect is missing" becomes `n/a` in Go mode. |
| Fix recipe (3.0) | None beyond rc.A. 3.0 help and docs: REQUIREMENTS lists `ssh-add` and `ssh-agent` only (help body unchanged in rc.A, corrected in 3.0); `docs/configuration.md` drops "through `expect` (required)". |
| Proving test | `TestLedger_SYS37` in `internal/ssh` (PTY fake of ssh-add: passphrase sent once, `SSH: Identity added: KEY` on stderr, no file under TMPDIR) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="sys-38"></a>
#### SYS-38 · history_grep and process_find match with grep's BRE: results depend on grep flavor and locale

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (dossier Go port notes of process_find and history_grep) |
| Area | Portability |
| Bash location | `lib/modules/system.sh:125` (`grep -i -- "$@"`), `lib/modules/system.sh:602` (`grep -i -- "$process_name"`) |
| Observed behavior | The pattern is a basic regular expression: `+`, `?`, `\|` are literal unless backslashed (GNU extensions `\+`, `\?`, `\|`, absent from BSD grep); `-i` folds non-ASCII only in a UTF-8 locale (`history_grep école` finds `echo ÉCOLE` with `LC_ALL=C.UTF-8`, nothing with `LC_ALL=C`). |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LC_ALL=C bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; set -o history; history -s "echo ÉCOLE"; history_grep école'` (nothing; with `LC_ALL=C.UTF-8` one row) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | GNU grep's BRE (with its `\+ \? \| \{m,n\} \( \)` extensions and POSIX classes) translated to Go regexp, case-insensitive with Unicode simple folding in every locale (the UTF-8 path). Back-references have no Go equivalent: such a pattern gets `Error: Invalid pattern: P` rc 1 instead of a grep match. |
| Fix recipe (3.0) | None beyond rc.A in `internal/system` (one `breToRE2` translator shared by both functions, with GNU grep vectors). |
| Proving test | `TestLedger_SYS38` in `internal/system` (`a+b` literal, `a\+b` repetition, `ssh\|cron` alternation, `école` vs `ÉCOLE` under `LC_ALL=C`, `\(a\)\1` → error) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="sys-39"></a>
#### SYS-39 · The agent start evals ssh-agent's output in the caller's shell

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (new) |
| Area | Security |
| Bash location | `lib/modules/ssh.sh:113-119` (`agent_out=$(ssh-agent ... \| sed ...)`, `eval "$agent_out"`) |
| Observed behavior | Whatever the first `ssh-agent` on PATH prints (minus the `echo Agent pid` line) is executed by the interactive shell at startup; the real agent prints only `SSH_AUTH_SOCK=...; export SSH_AUTH_SOCK;` and `SSH_AGENT_PID=...; export SSH_AGENT_PID;`. |
| Reproduction | With an `ssh-agent` stub first on PATH that prints `: >"$HOME/marker"; SSH_AUTH_SOCK=/tmp/x.sock; export SSH_AUTH_SOCK;`, the repo `ssh-add` mock with `MOCK_SSH_AGENT=0` and one `SSH:` entry in a temporary `~/.gash_env`: `gash_ssh_auto_unlock` creates the empty marker file |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The binary parses only `SSH_AUTH_SOCK=VALUE` and `SSH_AGENT_PID=DIGITS` assignments from the agent's output and prints them as quoted `export` lines (`mvdan.cc/sh/v3/syntax` quoting) for the shim to eval; anything else is ignored. Same variables as 2.x for a real agent. |
| Fix recipe (3.0) | None beyond rc.A in `internal/ssh`. |
| Proving test | `TestLedger_SYS39` in `internal/ssh` (agent output with an extra command → only the two exports, quoted) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="sys-40"></a>
#### SYS-40 · /etc/os-release is sourced as shell code

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (dossier accident note, sysinfo overview; ssh install hint) |
| Area | Security |
| Bash location | `lib/modules/sysinfo.sh:240` (`. /etc/os-release` in a subshell), `lib/modules/ssh.sh:24-26` (sourced inside the function, setting `ID`, `NAME`, `VERSION`... in the caller's shell) |
| Observed behavior | Any command in /etc/os-release runs as the user; the ssh hint path also leaves every os-release variable set in the interactive shell (`NAME`, `ID`, `VERSION_ID`, `PRETTY_NAME`, ...). |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; __gash_os_expect_install_hint >/dev/null; echo "ID=$ID"'` (prints the distro ID: the file was sourced into the shell) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The file is parsed, never executed: KEY=VALUE lines with os-release(5) quoting through `internal/platform`; `PRETTY_NAME`, else `NAME`, as 2.x showed. No variable reaches the caller (and the ssh hint is unreachable, SYS-37). |
| Fix recipe (3.0) | None beyond rc.A. |
| Proving test | `TestLedger_SYS40` in `internal/platform` (os-release fixture containing a command line → value parsed, nothing run) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="sys-41"></a>
#### SYS-41 · sysinfo listing order follows LC_COLLATE and raw directory order

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (dossier Go port notes, sysinfo) |
| Area | Portability |
| Bash location | `lib/modules/sysinfo.sh:508-517` (`ls`), `535-536`, `570`, `578`, `609`, `633`, `685`, `699`, `760`, `801`, `819`, `848`, `917`, `930`, `943` (unsorted `find`), plus the deep collectors' unsorted `find` at `1172` (unbound), `1184` (proftpd), `1215` (openvpn), `1246` (nvidia), `1256` (collectd), `1349` (overrides), `1493` (netplan), `1655` (sysctl.d), `707-722` (`sort` in the user's locale) |
| Observed behavior | `ls` and `sort` order by the locale's collation (`B` before `a` in C, after it in en_US.UTF-8); `find` lists in directory (inode/hash) order, which differs between filesystems and runs, and with `head -20` (openvpn) it changes which items are shown. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LC_ALL=C bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; sysinfo services --llm' > c.txt` and the same with `LC_ALL=en_US.UTF-8` into `u.txt`, then `diff c.txt u.txt` (on a host with mixed-case names under the listed /etc directories) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Byte order everywhere: directory listings come from `os.ReadDir` (sorted by name) and every `sort` is a byte sort; caps such as `head -20` apply after sorting. |
| Fix recipe (3.0) | None beyond rc.A in `internal/sysinfo`. |
| Proving test | `TestLedger_SYS41` in `internal/sysinfo` (sysroot fixture with `B.conf`, `a.conf`, created in reverse order → `B.conf` then `a.conf` in every locale) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="sys-42"></a>
#### SYS-42 · Shell state left behind by 2.x calls (sysinfo globals, BASH_REMATCH)

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (dossier sysinfo lifecycle note) |
| Area | Contract |
| Bash location | `lib/modules/sysinfo.sh:26-28,1806` (`__SYSINFO_MODE`, `__SYSINFO_HAS_SUDO`, `__SYSINFO_SUDO_KEEPALIVE_PID`), `lib/modules/system.sh:221,727`, `lib/modules/chrome.sh:81,145,148` (`=~` sets the global BASH_REMATCH) |
| Observed behavior | After `sysinfo --llm` the caller's shell has `__SYSINFO_MODE=llm`; after `port_kill 80` it has `BASH_REMATCH=([0]="80")`. The RETURN trap that sysinfo sets does not leak: Bash restores the caller's (tested in a function and at top level). |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; sysinfo overview --llm >/dev/null; echo "mode=$__SYSINFO_MODE"; port_kill 80 2>/dev/null; declare -p BASH_REMATCH'` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Nothing is left in the caller (a child process cannot set them); the stub defines no such variable. `ai_sysinfo`, the only reader of `__SYSINFO_MODE`, is ported inside the binary. |
| Fix recipe (3.0) | None beyond rc.A. |
| Proving test | `TestLedger_SYS42` as `testdata/script/TestLedger_SYS42.txtar` (after the two calls through the shim, `declare -p __SYSINFO_MODE BASH_REMATCH` fails) |
| Status | accept: no 3.0 change (Fix recipe: none) |

### docker and compose

<a id="dock-02"></a>
#### DOCK-02 · A realpath without -m (BSD, macOS, BusyBox) makes the compose functions act on the current directory's stack

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | Seed: "Compose functions: `realpath -m` failure falls back to $PWD (wrong directory)" (CONFIRMED; tool-variant part) |
| Area | Portability |
| Bash location | `lib/modules/docker-compose.sh:546,861,1057`; `docs/troubleshooting.md:100` |
| Observed behavior | With a `realpath` that rejects `-m`, every PATH argument falls back to `$PWD`: `dcc TARGET --json`, `dcup2 TARGET --dry-run` and `dcscan TARGET --json` report the cwd project, and `dcup2 TARGET --yes` pulls, stops and recreates the cwd stack (mock log). docs/troubleshooting.md lists `realpath -m` as affecting `llm_*` only. |
| Reproduction | `mkdir -p $T/bin $T/p $T/t && printf '#!/bin/sh\nexit 64\n' >$T/bin/realpath && chmod +x $T/bin/realpath && printf 'services:\n  cwdsvc:\n    image: nginx:latest\n' >$T/p/compose.yml && printf 'services:\n  targetsvc:\n    image: nginx:latest\n' >$T/t/compose.yml && gm 'cd $T/p; docker_compose_check $T/t --json'` → `"path":"<T>/p"`, service `cwdsvc` |
| Severity | high |
| Public-contract impact | no |
| rc.A behavior | Go resolves paths natively with GNU `realpath -m` semantics (absolute, lexically cleaned, symlinks of the longest existing prefix resolved), so the target directory is always the one named; only the empty string still falls back (DOCK-01). The Bash result depends on which realpath is installed, which the external tool policy removes. |
| Fix recipe (3.0) | none beyond rc.A and DOCK-01. docs/troubleshooting.md: delete the `realpath -m` row (3.x needs no external realpath). |
| Proving test | `TestLedger_DOCK02` in `internal/docker` (resolver equals GNU `realpath -m` on relative, nonexistent and symlinked vectors) and `testdata/script/dock02_no_realpath.txtar` (a failing `realpath` stub first on PATH; `docker_compose_check $WORK/t --json` reports `$WORK/t`) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dock-08"></a>
#### DOCK-08 · A `|` in an image shifts fields and injects keys into docker_compose_check --json

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N5) |
| Area | Security |
| Bash location | `lib/modules/docker-compose.sh:641,651,666` |
| Observed behavior | Per-service results travel as `\|`-joined rows re-split with `read`, and `status` is printed unescaped: an image value holding `\|` (from DIR/.env or the compose file) shifts the fields, so a crafted value adds keys and fake digests to the JSON an agent reads: `{"name":"svc","image":"redis:a","local_digest":"L","remote_digest":"R","status":"S","evil":"1","upgradeable":false,"error":"\|sha256:...\|UPDATE\|1\|"}` (observed). |
| Reproduction | `mkdir $T/j && printf 'T=a\|L\|R\|S","evil":"1\|0\|\n' >$T/j/.env && printf 'services:\n  svc:\n    image: "redis:${T}"\n' >$T/j/compose.yml && gm 'docker_compose_check $T/j --json'` → the object above |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Go keeps each service in a struct and the contract encoder escapes every string, so the image is reported intact (`"image":"redis:a\|L\|R\|S\",\"evil\":\"1\|0\|"`) and no key outside the envelope schema is emitted. Reproducing the shift would mean re-implementing the Bash row transport and emitting attacker-shaped keys. |
| Fix recipe (3.0) | none beyond rc.A; with DOCK-06's parser such a reference becomes a row with status `ERROR` and `"error":"invalid_reference"` (new value of the per-service error vocabulary, human message `Invalid image reference`, hint `Check the image name in the compose file`). |
| Proving test | `TestLedger_DOCK08` in `internal/docker` (fixture above: the service object's keys equal the schema keys, `.image` intact) plus `testdata/schema/docker_compose_check.json` validation |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dock-11"></a>
#### DOCK-11 · Compose functions leave variables in the caller's shell, including the registry token

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N8) |
| Area | Security |
| Bash location | `lib/modules/docker-compose.sh:301-321,387,550,586,596,650-651,838,865,905,911,1080,1109` |
| Observed behavior | After dcc, dcup2 or dcscan the caller's shell holds `registry name tag service_name image_ref arg f r svc img local_d remote_d stat upgr err file` (user variables with those names are overwritten) and `__gash_auth_token` (a Docker Hub bearer or a copy of GITHUB_TOKEN), `__gash_remote_digest`, `__gash_registry_curl_body`, `__gash_registry_curl_http_code`, `__gash_registry_curl_error`. |
| Reproduction | `gm 'printf "services:\n  w:\n    image: nginx:latest\n" >$T/compose.yml; name=mine; docker_compose_check $T --json >/dev/null; echo "name=$name"; declare -p __gash_auth_token'` → `name=library/nginx` and `declare -- __gash_auth_token="mock-dockerhub-token-abc123"` |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | none of these variables is set: a binary cannot change the caller's shell, and the shim must not re-create them (the token stays in the binary's memory). |
| Fix recipe (3.0) | none (moot in 3.x). For a 2.x patch release only: declare them `local` and `unset __gash_auth_token` after use. |
| Proving test | `TestLedger_DOCK11` as `testdata/script/dock11_no_leak.txtar` (`declare -p` before and after `docker_compose_check`, `docker_compose_upgrade --dry-run` and `docker_compose_scan` through the shim are identical) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dock-12"></a>
#### DOCK-12 · Image interpolation reads shell-only variables and applies Bash pattern rules

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N9, Bash-semantics part) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:139-153` (151 `${!var_name}`, 152 unquoted pattern and replacement) |
| Observed behavior | `${VAR}` falls back to any shell variable, exported or not, the function's own locals included (`${compose_file}` → the caller's compose path); a default with glob characters (`${TAG:-[ab]}`) never matches itself → 50 iterations and `Warning: Unresolvable variable reference (circular?) in: redis:${TAG:-[ab]}`, string kept; with bash 5.2 patsub_replacement a `&` in the value is the matched text (`AMP=a&b` → `redis:aaaa...${AMP}bbbb...` plus the warning; bash < 5.2 gives `a&b`). |
| Reproduction | `mkdir $T/e && printf 'AMP=a&b\n' >$T/e/.env && g '__gash_resolve_env_vars "redis:\${AMP}" $T/e/.env; echo; __gash_resolve_env_vars "redis:\${TAG:-[ab]}" /dev/null; echo; compose_file=x; __gash_resolve_env_vars "nginx:\${compose_file}" /dev/null'` → two warnings, `redis:aaaa...`, `redis:${TAG:-[ab]}`, `nginx:x` |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Go resolves `${VAR}` from DIR/.env, then the binary's own environment (exported variables only; the shim passes none of these), then the default, with literal replacement: `redis:a&b`, `redis:[ab]`, `nginx:` with no warning. A binary cannot see the caller's unexported or local variables (a non-exported GITHUB_TOKEN is invisible too), and Bash pattern and patsub rules depend on the Bash version. This code runs only on the line-parser path and on images Compose already interpolated (DOCK-26). |
| Fix recipe (3.0) | DOCK-13 (Compose interpolation semantics). docs/reference/docker.md: say that only exported variables and DIR/.env are used. |
| Proving test | `TestLedger_DOCK12` in `internal/docker` (vectors above: `redis:[ab]`, `redis:a&b`, no warning; a variable set only in the test shell is not used) |
| Status | accept: done 63f58e1 (code already right; docs/help) |

<a id="dock-19"></a>
#### DOCK-19 · Bash's own diagnostics naming 2.x source lines (no terminal, failed redirections)

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N14, diagnostic part; also seen in N22) |
| Area | Contract |
| Bash location | `lib/core/utils.sh:77` (needs_confirm_prompt, reached from `lib/modules/docker.sh:86`, `lib/modules/docker-compose.sh:954`, the remote ASK and --delete prompts in `lib/modules/remote.sh:198-269`); `lib/modules/git.sh:120` |
| Observed behavior | Outside headless mode without a controlling terminal the prompt text is followed by `/home/<user>/.gash/lib/core/utils.sh: line 77: /dev/tty: No such device or address` (the redirection fails before `2>/dev/null` applies), then the refusal. git_apply_patch with a `/` in the feature branch prints `.../lib/modules/git.sh: line 120: feat/x.patch: No such file or directory` before `Error: Failed to create patch file.` |
| Reproduction | the DOCK-18 command → stderr `Proceed with upgrade? [y/N] $GASH_SRC/lib/core/utils.sh: line 77: /dev/tty: No such device or address` then `Info: Upgrade cancelled.` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The question line, the refusal and the exit statuses are kept; the Bash diagnostic is not printed and nothing replaces it: it names a 2.x script path and line number that the binary does not have. |
| Fix recipe (3.0) | none beyond rc.A. |
| Proving test | `TestLedger_DOCK19` as `testdata/script/dock19_no_tty.txtar` (under `setsid`, stderr holds the question and `Info: Upgrade cancelled.`, no `line 77`) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dock-23"></a>
#### DOCK-23 · curl behaviors in registry calls: URL globbing, ~/.curlrc, curl's proxy and CA variables, token in a curl config

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N16; dossier summary item 6) |
| Area | Portability |
| Bash location | `lib/modules/docker-compose.sh:261-262,304,345,403,405,415,417,425` |
| Observed behavior | curl runs without `--globoff` and without `-q`: `{}` or `[]` in an image name or tag turn the URL into a glob (several transfers, captured code `000000` → `http_000000`); every `~/.curlrc` option applies to every registry call; proxies follow curl's rules (ALL_PROXY honored); CURL_CA_BUNDLE is honored; the bearer token is written into `--config <(printf 'header = "Authorization: Bearer %s"')`, so a `"` or a newline in GITHUB_TOKEN adds curl config lines. |
| Reproduction | `cd $T && touch a b && curl -sS -w '%{http_code}' -o g.out "file://$T/{a,b}"` → `000000` (two transfers in one call: the globbing the registry URLs are exposed to) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Go builds each URL with net/url (no globbing; an invalid name fails as `transport_error`, DOCK-24), never reads `~/.curlrc`, takes proxies from `http.ProxyFromEnvironment` (HTTPS_PROXY, HTTP_PROXY, NO_PROXY and lowercase; not ALL_PROXY) and CAs from SSL_CERT_FILE, SSL_CERT_DIR and the system pool, and keeps the token in-process as one header value (a CR or LF in it fails before sending, `transport_error`). The 2.x results depend on the curl build and the user's curl configuration. |
| Fix recipe (3.0) | none beyond rc.A. docs/reference/docker.md: registry calls honor HTTPS_PROXY, NO_PROXY and SSL_CERT_FILE, not `~/.curlrc` or ALL_PROXY. |
| Proving test | `TestLedger_DOCK23` in `internal/docker` (image `app{a,b}:latest` → one request with the escaped path to the httptest fake; a HOME with a `.curlrc` changes nothing; HTTPS_PROXY pointing at the fake is used) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dock-24"></a>
#### DOCK-24 · curl exit codes in the registry error vocabulary (`curl_error_N`, `no_response`)

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (dossier "Registry port notes" 2) |
| Area | Contract |
| Bash location | `lib/modules/docker-compose.sh:264-278` |
| Observed behavior | curl exit 6 → `dns_error`, 7 → `connection_refused`, 28 → `timeout`, 35 and 60 → `ssl_error`, any other → `curl_error_N` (3 malformed URL, 5 proxy not resolved, 47 too many redirects, 52 empty reply, 56 receive error, ...); exit 0 without a status code → `no_response`. The human report shows all of these as "Could not reach registry or get manifest" with the registry's hint. |
| Reproduction | `mkdir -p $T/bin && printf '#!/bin/sh\nexit 52\n' >$T/bin/curl && chmod +x $T/bin/curl && g '__gash_registry_curl https://registry-1.docker.io/v2/; echo "$__gash_registry_curl_error"'` → `curl_error_52` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Go maps net/http errors onto the same strings: `*net.DNSError` → `dns_error`, ECONNREFUSED → `connection_refused` (whether the refused connection is to the registry host or to the proxy, matching curl exit 7), any deadline (10 s connect, 20 s total) → `timeout`, TLS handshake and x509 errors → `ssl_error`; every other transport failure — a proxy CONNECT refusal returned as an error, a connection reset, EOF or empty reply, too many redirects, invalid URL or header value — → the fixed string `transport_error` (which is also where curl exit 56 landed, so docker needs no special tunnel handling). `no_response` cannot occur (net/http yields a status or an error) and stays documented only. Human output is unchanged (`transport_error` takes the default branch, as `curl_error_N` did). curl exit numbers have no Go equivalent. |
| Fix recipe (3.0) | keep `transport_error`; document the vocabulary in docs/reference/docker.md and the AGENTS.md llm_docker_check notes: `dns_error connection_refused timeout ssl_error transport_error unauthorized forbidden not_found rate_limited server_error http_NNN malformed_token no_digest_in_response registry_unreachable`. |
| Proving test | `TestLedger_DOCK24` in `internal/docker` (httptest server closing the connection → `transport_error`; unresolvable host → `dns_error`; closed port → `connection_refused`; stalled server with a short deadline → `timeout`; self-signed certificate → `ssl_error`) |
| Status | accept: done 63f58e1 (code already right; docs/help) |

<a id="dock-25"></a>
#### DOCK-25 · Manifest digest headers of redirect hops are concatenated

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N17) |
| Area | Correctness |
| Bash location | `lib/modules/docker-compose.sh:405-417,432` |
| Observed behavior | `curl -I -L -o FILE` stores the headers of every hop; `grep -i docker-content-digest: \| awk '{print $2}' \| tr -d '\r\n'` joins every Docker-Content-Digest it finds without a separator, so a redirect hop that carries one yields a digest that never matches (permanent `UPDATE`). |
| Reproduction | `mkdir -p $T/bin && printf '#!/bin/sh\nwhile [ "$1" != -o ]; do shift; done; printf "HTTP/1.1 307\\r\\nDocker-Content-Digest: sha256:aa\\r\\n\\r\\nHTTP/1.1 200\\r\\nDocker-Content-Digest: sha256:bb\\r\\n" >"$2"; printf 200\n' >$T/bin/curl && chmod +x $T/bin/curl && g '__gash_get_remote_digest example.org app v1 >/dev/null; echo "$__gash_remote_digest"'` → `sha256:aasha256:bb` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Go takes Docker-Content-Digest from the final response only (net/http follows the redirects); concatenating intermediate hops is an artifact of curl's header dump. |
| Fix recipe (3.0) | none beyond rc.A. |
| Proving test | `TestLedger_DOCK25` in `internal/docker` (httptest redirect chain with a digest on both hops → the final digest) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dock-26"></a>
#### DOCK-26 · Compose v2 without jq: Go always parses `docker compose config --format json`

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (dossier "Service parsing"; lead decision) |
| Area | Portability |
| Bash location | `lib/modules/docker-compose.sh:62-106` |
| Observed behavior | With compose v2 the services come from `docker compose -f FILE --project-directory DIR config --format json \| jq` only when jq is on PATH; without jq (and with compose v1, or when `config` fails) the awk YAML subset runs: block style only, `image:` at the first child indentation only, quotes and ` #comments` stripped, no anchors or flow style, interpolation by Gash alone, file order instead of Compose's alphabetical order, awk variant from `${__GASH_AWK:-awk}`. |
| Reproduction | `mkdir $T/y && printf 'x-img: &img nginx:latest\nservices:\n  b:\n    image: *img\n  a:\n    image: redis:7\n' >$T/y/compose.yml && g '__gash_parse_compose_services $T/y/compose.yml; echo ---; __gash_compose_cmd() { return 1; }; __gash_parse_compose_services $T/y/compose.yml'` (host with compose v2 and jq) → `a\|redis:7`, `b\|nginx:latest`, then the subset reader's `b\|*img`, `a\|redis:7` (what a host without jq got) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | With compose v2 on PATH Go always parses `config --format json` with encoding/json (jq is irrelevant), keeping Compose's semantics and order; the line parser (the awk subset ported rule for rule) runs only with compose v1 or when `config` fails, as in 2.x. The jq-absent result depended on an optional tool, which the external tool policy removes. |
| Fix recipe (3.0) | none beyond rc.A; docs: jq is no longer needed by the compose functions. |
| Proving test | `TestLedger_DOCK26` in `internal/docker` (runner fake serving config JSON with no jq on PATH → services from the JSON in its order; a compose v1 fake → line-parser vectors: block style, quotes, ` #`, CRLF, tabs) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dock-27"></a>
#### DOCK-27 · docker_compose_scan orders files by the locale's collation

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N31, sort part) |
| Area | Portability |
| Bash location | `lib/modules/docker-compose.sh:1061-1064` |
| Observed behavior | `find ... \| sort` sorts the paths with the caller's LC_COLLATE: under en_US.UTF-8 `B/` sorts after `a/` and punctuation is ignored at first level, under C or C.UTF-8 by bytes (`B/` first). |
| Reproduction | `mkdir -p $T/s2/B $T/s2/a && printf 'services: {}\n' >$T/s2/B/compose.yml && cp $T/s2/B/compose.yml $T/s2/a/ && for l in C.UTF-8 en_US.UTF-8; do env -i HOME=$T PATH=/usr/bin:/bin LC_ALL=$l bash -c "GASH_HEADLESS=1; source $GASH_SRC/gash.sh; docker_compose_scan $T/s2 --json"; done` → `B` first under C.UTF-8 (observed), `a` first under en_US.UTF-8 (inferred from glibc collation; that locale is not installed in the audit sandbox) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | byte order of the full path (as `LC_ALL=C sort`) whatever the locale; locale-dependent sorting is excluded by the external tool policy. |
| Fix recipe (3.0) | none beyond rc.A. |
| Proving test | `TestLedger_DOCK27` in `internal/docker` (paths `B/`, `a/`, `a-b/`, `a_b/` come out in byte order with LANG=en_US.UTF-8 in the environment) |
| Status | accept: no 3.0 change (Fix recipe: none) |

### git module

<a id="git-09"></a>
#### GIT-09 · git_dump_revisions leaves `commit` in the caller's shell

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N8, git part) |
| Area | Contract |
| Bash location | `lib/modules/git.sh:36` |
| Observed behavior | The loop variable `commit` is not local: after `gdr FILE` the caller's `$commit` holds the oldest commit's short hash, overwriting a user variable. |
| Reproduction | `g 'cd $T && git init -q w9 && cd w9 && git config user.email a@b && git config user.name n && echo a >f && git add f && git commit -qm i && commit=mine && git_dump_revisions f 2>/dev/null; echo "commit=$commit"'` → `commit=<short hash>` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | `$commit` is untouched: a binary cannot set variables in the caller's shell. |
| Fix recipe (3.0) | none (moot). |
| Proving test | `TestLedger_GIT09` as `testdata/script/git09_no_leak.txtar` (`commit=mine; git_dump_revisions f; echo $commit` → `mine`) |
| Status | accept: no 3.0 change (Fix recipe: none) |

### remote

<a id="rem-04"></a>
#### REM-04 · remote_exec quotes script arguments with Bash `printf %q`

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N26) |
| Area | Portability |
| Bash location | `lib/modules/remote.sh:386` |
| Observed behavior | Script arguments are quoted with `printf '%q '`, which is Bash syntax and locale-dependent: `a\ b \~ '' \$\(id\)` and `$'x\ny'`; `é` stays raw under C.UTF-8 but becomes `$'\303\251'` under the C locale; a dash login shell on the remote reads `$'...'` literally (`[$x\ny]`, `[$\303\251]` where bash gives `[x⏎y]`, `[é]`, observed); the command string ends with a space. |
| Reproduction | `fake ssh && printf 'HOST:box=deploy@box.example\n' >$T/.gash_env && chmod 600 $T/.gash_env && printf 'echo ok\n' >$T/s.sh && g 'remote_exec box --script $T/s.sh -- "a b" "~" "" "\$(id)"'; cat $T/argv` → `... deploy@box.example unset HISTFILE HISTSIZE; bash -s -- a\ b \~ '' \$\(id\) ` |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Go quotes each argument POSIX-style, locale-independent, valid for sh, dash, bash and zsh: `unset HISTFILE HISTSIZE; bash -s -- 'a b' '~' '' '$(id)'` (an embedded `'` as `'\''`, no trailing space). With a bash remote the script receives the same arguments as in 2.x; the remote command string differs byte for byte. `printf %q` is Bash-only quoting. |
| Fix recipe (3.0) | none beyond rc.A. |
| Proving test | `TestLedger_REM04` in `internal/remote` (for the vectors `a b`, a newline, `é`, `~`, the empty string, `$(id)` and `it's`, `sh -c` and `bash -c` on the built string print identical argv) |
| Status | accept: no 3.0 change (Fix recipe: none) |

### php and node

<a id="dev-09"></a>
#### DEV-09 · The copilot-instructions source depends on the PCRE2 version

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | Seed: "variable-length lookbehind kills one php_detect_version source" (PARTIAL: dead here because of PCRE2 10.42, not grep 3.11 itself) |
| Area | Portability |
| Bash location | `lib/modules/php.sh:56-64` |
| Observed behavior | `grep -oP '(?i)(?<=php[[:space:]:]{0,3})[0-9]+\.[0-9]+'` is rejected by PCRE2 < 10.43 (`grep: lookbehind assertion is not fixed length`, rc 2, hidden by `2>/dev/null`) and by greps without `-P` (BSD, BusyBox), so the documented third source is dead there (`PHP 8.2` in `.github/copilot-instructions.md` gives 8.4); PCRE2 >= 10.43 accepts the bounded lookbehind and detects 8.2 (inferred from the PCRE2 10.43 changelog). |
| Reproduction | `d=$(mktemp -d); mkdir "$d/.github"; printf 'This project targets PHP 8.2 on Laravel 11.\n' >"$d/.github/copilot-instructions.md"; env -i HOME="$d" PATH=/usr/local/bin:/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; php_detect_version "$HOME"'` → `"version":"8.4"` on PCRE2 10.42 |
| Severity | medium |
| Public-contract impact | yes (rc.A returns 8.2 where 2.x on PCRE2 < 10.43 returned 8.4) |
| rc.A behavior | Go `regexp` has no lookbehind; rc.A implements the source as the code intended (the PCRE2 >= 10.43 result): `(?i)php[[:space:]:]{0,3}([0-9]+\.[0-9]+)` applied line by line, first submatch in file order, in its slot after composer.json and before 8.4, DIR only. Results checked with Go 1.27: `PHP 8.2` → 8.2, `php:8.3-fpm` → 8.3, `PHPUnit 10.5` → none, four spaces before the number → none, `php` and the number on different lines → none. |
| Fix recipe (3.0) | None beyond rc.A. Docs: php_detect_version NOTES keep the three sources and add "copilot-instructions: first `php` followed by up to three spaces or colons and X.Y"; release notes: "2.x on PCRE2 < 10.43 or without `grep -P` ignored this source". |
| Proving test | `TestLedger_DEV09_2x` in `internal/dev` (the vectors above; with `.target-php-version 7.4` → 7.4; with composer `^8.1` → 8.1) and `testdata/script/php_detect_copilot.txtar` |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dev-10"></a>
#### DEV-10 · jq's presence and version decide the composer.json and package.json sources

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (A4; dossier php "missing jq disables step 2 silently", node "jq (optional)") |
| Area | Portability |
| Bash location | `lib/modules/php.sh:45`, `lib/modules/node.sh:325` |
| Observed behavior | Without jq, `.require.php` and `.engines.node` are skipped silently (next source or default). With jq the number text depends on its version: `"php": 8.10` prints `8.10` with jq 1.7 and `8.1` with jq 1.6 (inferred); a non-string value is printed as jq's JSON text and its first X.Y is used (`{"v":"7.3"}` → 7.3). |
| Reproduction | `d=$(mktemp -d); printf '{"require":{"php":"^8.2"}}' >"$d/composer.json"; mkdir "$d/bin"; printf '#!/bin/sh\nexit 127\n' >"$d/bin/jq"; chmod +x "$d/bin/jq"; env -i HOME="$d" PATH="$d/bin:/usr/local/bin:/usr/bin:/bin" bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; php_detect_version "$HOME"'` → `"version":"8.4"` (8.2 with a working jq) |
| Severity | low |
| Public-contract impact | yes (hosts without jq get the file's version in rc.A) |
| rc.A behavior | Native JSON, as 2.x with jq 1.7 present: number values keep their source text (`8.10`), strings as is, objects/arrays rendered as jq 1.7 `-r` renders them before the X.Y search, invalid JSON or a non-object `require` skipped silently. |
| Fix recipe (3.0) | None beyond rc.A. Docs: dev.md drops "needs jq". |
| Proving test | `TestLedger_DEV10_2x` in `internal/dev`: `8.10` number → `"8.10"`, `{"v":"7.3"}` → 7.3, `"require":"x"` → falls through |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dev-11"></a>
#### DEV-11 · A relative project_dir is resolved through CDPATH

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (A1, A11) |
| Area | Portability |
| Bash location | `lib/modules/php.sh:25,43,56` |
| Observed behavior | `search_dir=$(cd "$project_dir" && pwd)` honours the caller's `CDPATH` (and prints the directory, which lands in the capture), so the walk-up can start in a CDPATH directory while composer.json and copilot-instructions are still read relative to the cwd. Separately, the walk-up uses the logical `$PWD` chain (symlinks kept, `..` lexical). |
| Reproduction | `d=$(mktemp -d); mkdir "$d/proj"; printf '7.4\n' >"$d/.target-php-version"; env -i HOME="$d" PATH=/usr/local/bin:/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; cd /; CDPATH="$HOME"; php_detect_version proj'` → `"version":"7.4"`; without CDPATH → `"version":"8.4"` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | `CDPATH` is ignored (a non-exported shell variable the binary does not receive, and not a documented input): a relative DIR is joined to the logical cwd (`$PWD` when it names the cwd, as `os.Getwd` does) and cleaned lexically, as `cd -L` without CDPATH; a missing DIR keeps the 2.x rule (absolute: walk up from the literal path; relative: no walk-up). |
| Fix recipe (3.0) | None beyond rc.A. Docs: none. |
| Proving test | `TestLedger_DEV11_2x` in `internal/dev` (relative, absolute, missing, symlinked DIR vectors of the dossier; CDPATH set in the environment has no effect) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dev-12"></a>
#### DEV-12 · php_raw_binary and composer_raw_binary come from Bash's command hash table

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (A2) |
| Area | Portability |
| Bash location | `lib/modules/php.sh:101,109` |
| Observed behavior | `type -P` answers from the caller's hash table first, so a stale or hand-set entry is reported even when it no longer exists. |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/local/bin:/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; hash -p /opt/stale/php php; php_detect_version /nonexistent'` → `"php_raw_binary":"/opt/stale/php"` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | `exec.LookPath` over the inherited PATH (the hash table is shell state a binary cannot read); the `/usr/bin/php<version>` and `/usr/bin/php<major>` probes and the composer fallbacks keep their order. |
| Fix recipe (3.0) | None beyond rc.A. Docs: none. |
| Proving test | `TestLedger_DEV12_2x` in `internal/dev` (PATH fixture decides; no hash state exists) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dev-13"></a>
#### DEV-13 · Version ordering depends on GNU `sort -V`

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (A10) |
| Area | Portability |
| Bash location | `lib/modules/node.sh:68,102,278`; `lib/shell/toolchains.sh:46` |
| Observed behavior | "Newest installed" and the node_upgrade targets come from `sort -V`; GNU sorts a prerelease after its release and after older patches (`v20.12.0-rc.1` > `v20.11.1`, `20.12.0` < `20.12.0-rc.1`); BSD and BusyBox sort differ or lack `-V` (inferred). |
| Reproduction | `printf 'v20.12.0-rc.1\nv20.11.1\nv20.12.0\n' \| sort -V` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | A Go port of GNU coreutils `filevercmp` (the documented GNU result) on every platform. |
| Fix recipe (3.0) | None beyond rc.A; DEV-05's filter removes prereleases from node_detect_version. |
| Proving test | `TestLedger_DEV13_2x` in `internal/dev` (comparison table checked against GNU `sort -V` output captured at S1) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dev-14"></a>
#### DEV-14 · The caller's IFS breaks node_upgrade's plan

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (A5, node part) |
| Area | Correctness |
| Bash location | `lib/modules/node.sh:149,161,176,180` |
| Observed behavior | The majors, the per-major versions and the REMOVED/kept rows are split and joined with the caller's IFS. With `IFS=,` and v18.20.3, v18.20.4, v20.1.0 installed (index v18.20.8, v20.1.0) the plan becomes `1 newly installed, 1 to remove, 0 up to date.`, lists v20.1.0 as REMOVED, and `-y` calls `nvm uninstall` with one two-line argument (verified with a mock nvm). |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; mkdir -p "$HOME"/.nvm/versions/node/v18.20.{3,4} "$HOME/.nvm/versions/node/v20.1.0"; nvm(){ [ "$1" = ls-remote ] && printf "v18.20.8\nv20.1.0\n"; }; IFS=,; node_upgrade --dry-run'` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The plan is computed with explicit splitting (one version per line) and rows joined with one space, the default-IFS result whatever the caller's IFS; the shim passes each version to `nvm` as its own argument. |
| Fix recipe (3.0) | None beyond rc.A. |
| Proving test | `TestLedger_DEV14_2x` in `internal/dev` (plan for the fixture above: `1 newly installed, 2 to remove, 1 up to date.`) and `testdata/script/node_upgrade_ifs.txtar` (caller `IFS=,`) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dev-15"></a>
#### DEV-15 · `shopt -s failglob` leaks a Bash error from node_upgrade

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (A12) |
| Area | Portability |
| Bash location | `lib/modules/node.sh:99` |
| Observed behavior | With failglob and no installed version, `for d in "$dir"/v*` prints `node.sh: line 99: no match: .../versions/node/v*` before `Info: No Node.js version is installed with nvm.` |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; nvm(){ :; }; shopt -s failglob; node_upgrade --dry-run'` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The directory is listed natively; only the `Info:` line is printed, rc 0 (shell options do not reach the binary). |
| Fix recipe (3.0) | None beyond rc.A. |
| Proving test | `testdata/script/node_upgrade_failglob.txtar` (`TestLedger_DEV15_2x`: stderr has no `no match`) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="dev-16"></a>
#### DEV-16 · nvm is a shell function: node_upgrade and nvm_prune_versions keep their nvm calls in the Bash shim

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (dossier Go port notes; lead decision) |
| Area | Contract |
| Bash location | `lib/modules/node.sh:18-24,54,68,139-142,189-223,229-233`; `lib/shell/toolchains.sh:77-88` |
| Observed behavior | Both functions require `nvm` to be a function of the caller's shell (`declare -f nvm`), call it directly (`nvm current`, `ls`, `ls-remote`, `install --reinstall-packages-from`, `use`, `uninstall`), and `nvm use` changes the caller's PATH. Under `GASH_NVM=lazy` the first direct call sources `nvm.sh --no-use` into the caller's shell (calls inside `$(...)` source it in the subshell only). The specs override `nvm(){...}`. |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; node_upgrade --dry-run; echo rc=$?'` → `Error: nvm is not loaded in this shell. Source $NVM_DIR/nvm.sh first.` `rc=1` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | A binary cannot call a shell function or change the caller's PATH, so the generated shim runs every nvm call in the caller's shell in 2.x order (same `declare -f nvm` check and message); work that needs no nvm (installed tags, plan, rows and counts) may run in the binary, fed by the shim. stdout, stderr, exit codes, the PATH change and the lazy-load sourcing are those of 2.x; spec overrides of `nvm` keep working. |
| Fix recipe (3.0) | None (permanent design). Docs: `compat.md` section 4 lists both functions under "shell-mutating". |
| Proving test | `tests/specs/node_spec.sh` and `node_upgrade_spec.sh` green with `--impl go`; `testdata/script/node_upgrade_path.txtar` (`TestLedger_DEV16_2x`: after `-y` with a mock nvm, `nvm use` changed PATH in the calling shell) |
| Status | accept: no 3.0 change (Fix recipe: none) |

### ai

<a id="ai-13"></a>
#### AI-13 · The curl and jq dependency errors become unreachable (ai_sysinfo --raw included)

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | Lead decision (AI requests over `net/http`, no curl or jq); New finding (N28) |
| Area | Portability |
| Bash location | `lib/modules/ai/client.sh:20-36`; `lib/modules/ai/chat.sh:163-164,320-321`; `lib/modules/ai/sysinfo.sh:693-695` |
| Observed behavior | Without curl or jq on PATH every AI function stops with `Error: curl is required for AI module. Install with: sudo apt install curl` or `Error: jq is required for AI module. Install with: sudo apt install jq`, rc 1; ai_sysinfo checks them before `--raw`, which sends nothing (N28). |
| Reproduction | `d=$(mktemp -d); mkdir "$d/bin"; for f in /usr/bin/*; do ln -s "$f" "$d/bin/"; done; rm "$d/bin/curl"; env -i HOME="$d" PATH="$d/bin" bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; ai_sysinfo --raw </dev/null'` → the curl error, no data |
| Severity | low |
| Public-contract impact | yes (two error messages disappear; `--raw` works without curl) |
| rc.A behavior | Requests, response parsing and sanitizing are native (`net/http`, `encoding/json`), so both messages are unreachable and `ai_sysinfo --raw` prints the data whatever is installed. The 2.x behavior with both tools present is kept: same request bodies (key order `model,max_tokens,system,messages,output_config` and `systemInstruction,contents,generationConfig`, as jq built them), same retry policy (HTTP 429, 500, 502-504, 529, at most three retries, delay from `Retry-After`, else Gemini `RetryInfo.retryDelay` in `Ns` or `Nms`, else 5 s, clamped 1-60 s, the `<label> API busy (HTTP N), retrying in Ns (i/3)...` warning and the `still busy ... after 3 retries` error), same error texts, same control-character stripping. |
| Fix recipe (3.0) | None beyond rc.A. Docs: ai.md and `ai.sh` header drop "Runtime: curl, jq"; the PRIVACY text "The API key travels in a curl config read from a pipe" becomes "The API key stays inside the gash process" in prose docs (help bodies per AI-25). |
| Proving test | `TestLedger_AI13_2x` in `internal/ai` (no curl/jq on PATH: `--raw` prints data rc 0; a request reaches the fake HTTPS server) and `testdata/script/ai_no_curl.txtar` |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="ai-14"></a>
#### AI-14 · The AI transport no longer reads curl's configuration

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (A13; `~/.curlrc` checked in this pass) |
| Area | Security |
| Bash location | `lib/modules/ai/client.sh:275-280,283,287` |
| Observed behavior | curl runs without `-q`, so `~/.curlrc` (or `$CURL_HOME/.curlrc`) and curl-only variables (`ALL_PROXY`, `CURL_CA_BUNDLE`) shape every request; a `verbose` line there makes curl print `> x-api-key: <token>` on stderr (verified with a local server), which Gash keeps in the curl-stderr temp file and prints in the `GASH_AI_DEBUG=1` "Curl Stderr" block and in `curl failed (exit N): <stderr>` (inferred from lines 283, 287). The key goes through `--config` quoting: `ab"cd` arrives as `ab`, `\t` as a TAB, `\\` as `\`. |
| Reproduction | `d=$(mktemp -d); printf 'verbose\n' >"$d/.curlrc"; python3 -m http.server 18765 --bind 127.0.0.1 & sleep 1; env -i HOME="$d" PATH=/usr/bin:/bin curl -sS -o /dev/null --config <(printf 'header = "x-api-key: FAKE123"\n') http://127.0.0.1:18765/ 2>&1 \| grep FAKE123; kill $!` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | `net/http` with the key header set in-process and sent byte for byte (no config quoting); proxies from `HTTPS_PROXY`/`https_proxy` and `NO_PROXY`/`no_proxy` only (`ALL_PROXY` and `~/.curlrc` ignored), CA from the system pool or `SSL_CERT_FILE`/`SSL_CERT_DIR` (`CURL_CA_BUNDLE` ignored), connect timeout 5 s, total timeout 60 s (120 s for ai_sysinfo) as `--max-time`; no request header is ever printed, so the key cannot reach debug output. A token holding bytes `net/http` refuses in a header (control characters) fails as a transport error (AI-15 fallback) where curl sent it. |
| Fix recipe (3.0) | None beyond rc.A. Docs: configuration.md says the AI requests honour `HTTPS_PROXY`/`NO_PROXY` and `SSL_CERT_FILE`, not curl's configuration. |
| Proving test | `TestLedger_AI14_2x` in `internal/ai` (a `~/.curlrc` with `verbose` and `ALL_PROXY` set change nothing; the fake server receives the token `ab"cd` intact; debug output never contains the token) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="ai-15"></a>
#### AI-15 · Transport errors keyed on curl exit codes are mapped from Go errors

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (dossier client section, Go port notes) |
| Area | Contract |
| Bash location | `lib/modules/ai/client.sh:44-81,275-281` |
| Observed behavior | Messages are chosen by curl's exit code: 5 proxy, 6 DNS, 7 connect, 18 partial, 22 HTTP error, 28 timeout (`Connection timed out after Ns (server unreachable)` when elapsed <= 7 s, else `Response timed out after Ns`), 35 TLS, 47 redirects, 52 empty reply, 55 send, 56 receive, 60 certificate, any other `curl failed (exit N)[: stderr]`; an empty or `000` HTTP code gives `No response received (network error)`. curl runs without `-L`, so a 3xx is reported as `API error (3xx)`. |
| Reproduction | `env -i HOME="$d" PATH="$d/bin:/usr/bin:/bin" bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; ai_query claude what is ls </dev/null'` with the AI-02 fake curl (exit 6) → `Error: Claude API: Could not resolve host (check DNS/internet)` |
| Severity | low |
| Public-contract impact | yes (texts of unmapped errors) |
| rc.A behavior | Go errors are mapped onto the same texts: `*net.DNSError` → 6 (5 when it names the proxy host), `ECONNREFUSED`/`EHOSTUNREACH`/`ENETUNREACH` → 7, dial timeout → 28 "Connection timed out" text, client timeout → 28 by the same elapsed rule (whole seconds), TLS handshake errors → 35, `x509` verification errors → 60, unexpected EOF in the body → 18, connection closed before a status line → 52, write errors → 55, read errors and resets → 56; redirects are not followed (`CheckRedirect` returns `http.ErrUseLastResponse`), so 47 and 22 are unreachable as with curl; anything else prints `<label> API: request failed: <error, control characters stripped>` in place of `curl failed (exit N)`, and the debug "Curl Stderr" block never appears. HTTP-level handling (200, retry codes, `.error.message // .error // "Unknown error"`, refusal and max-token stops) is unchanged. |
| Fix recipe (3.0) | None beyond rc.A. Docs: none (ai.md has no error list). |
| Proving test | `TestLedger_AI15_2x` in `internal/ai` (table of injected transport errors → exact message; a 301 reply → `Claude API error (301): Unknown error`) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="ai-16"></a>
#### AI-16 · The caller's IFS joins the ai_query question

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (A5, ai part) |
| Area | Portability |
| Bash location | `lib/modules/ai/chat.sh:298,301` |
| Observed behavior | `query="$*"` joins the words with the first IFS character: `IFS=,; ai_query what is up` sends `what,is,up`; `IFS=` sends `whatisup`. |
| Reproduction | `env -i HOME="$d" PATH="$d/bin:/usr/bin:/bin" bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; IFS=,; ai_query what is up </dev/null'` with the AI-03 body-saving fake curl → content ends `what,is,up` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Words joined with one space (the default-IFS result); the caller's IFS does not reach the binary. |
| Fix recipe (3.0) | None beyond rc.A. |
| Proving test | `testdata/script/ai_query_ifs.txtar` (`TestLedger_AI16_2x`: caller `IFS=,`, body ends `what is up`) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="ai-17"></a>
#### AI-17 · `shopt -s nocasematch` changes provider and GASH_AI_CONFIRM matching

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (A6) |
| Area | Portability |
| Bash location | `lib/modules/ai/chat.sh:295`; `lib/modules/ai/client.sh:171-172`; `lib/modules/ai/sysinfo.sh:675`; `lib/core/config.sh` token lookup |
| Observed behavior | Under the caller's `nocasematch`, `ai_query Claude hi` takes `Claude` as the provider and sends `hi` (normally it sends `Claude hi` to the default provider), `ai_sysinfo CLAUDE` is accepted, and `GASH_AI_CONFIRM=AUTO` behaves as `auto`. |
| Reproduction | `env -i HOME="$d" PATH="$d/bin:/usr/bin:/bin" bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; shopt -s nocasematch; ai_query Claude hi </dev/null'` with the AI-03 body-saving fake curl → content ends `hi` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Case-sensitive matching everywhere (the default-shell result): `Claude hi` is the question, `CLAUDE` is an unknown argument for ai_sysinfo, `AUTO` means "ask" (AI-23). |
| Fix recipe (3.0) | None beyond rc.A. |
| Proving test | `TestLedger_AI17_2x` in `internal/cli/args` (grammar table) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="ai-18"></a>
#### AI-18 · Whether extglob is on decides which question paths ai_sysinfo reads

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (A7) |
| Area | Portability |
| Bash location | `lib/modules/ai/sysinfo.sh:86-88` |
| Observed behavior | `${fpath%%[?.!,:;)\"\']+([?.!,:;)\"\'])}` is an extglob pattern; Gash never enables extglob, but bash-completion does in most interactive shells. With extglob off only the last punctuation character is stripped, so `/etc/hostname?!` reads nothing; with it on, all trailing punctuation goes and the file is read. |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; __sysinfo_collect_question_context "see /etc/hostname?!" \| head -1; shopt -s extglob; __sysinfo_collect_question_context "see /etc/hostname?!" \| head -1'` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | All trailing `? . ! , : ; ) " '` characters are stripped (the extglob-on result, what the code comment intends); the AI-01 reproduction still applies to the path that remains. |
| Fix recipe (3.0) | None beyond rc.A (AI-01's path rules apply after stripping). |
| Proving test | `TestLedger_AI18_2x` in `internal/ai` (`/etc/hostname?!` → `/etc/hostname`) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="ai-19"></a>
#### AI-19 · The locale decides characters or bytes for the 4 KB cut marker, the "bytes" info and the consent size

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (A8) |
| Area | Portability |
| Bash location | `lib/modules/ai/chat.sh:194-199,207-209,234,281-286,341`; `lib/modules/ai/sysinfo.sh:760` |
| Observed behavior | `head -c 4096` cuts bytes but `${#var}` counts characters in a UTF-8 locale: 3000 × `é` piped (6000 bytes) is cut to 4096 bytes and gets no `[... truncated to 4KB]` marker under C.UTF-8 (it does under `LANG=C`); `Piped input received (N lines, B bytes)` and the consent size (`2 KB` for 2000 × `é`, 4000 bytes) are characters. A cut inside a multibyte character reaches jq, which turns it into U+FFFD in the body. |
| Reproduction | `head -c 6000 /dev/zero \| tr '\0' 'x' \| sed 's/x/é/g' \| head -c 6000 \| env -i HOME="$d" PATH="$d/bin:/usr/bin:/bin" LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; ai_query claude explain'` with the AI-03 body-saving fake curl → body without the truncation marker |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Bytes everywhere (the C-locale result): the cut is exactly 4096 bytes as `head -c`, the marker is added when 4096 bytes were read, the info and the consent count bytes; the partial UTF-8 sequence is written into the JSON body as U+FFFD, as jq did. |
| Fix recipe (3.0) | None beyond rc.A (a 3.0 option: cut on a UTF-8 boundary). |
| Proving test | `TestLedger_AI19_2x` in `internal/ai` (6000-byte `é` input: marker present, body ends with U+FFFD before the marker, consent `4 KB`) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="ai-20"></a>
#### AI-20 · Bash's own diagnostics from the AI functions are not reproduced

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New findings (A9, N18 and N27 side effects) |
| Area | Contract |
| Bash location | `lib/modules/ai/chat.sh:194,210,281`; `lib/modules/ai/sysinfo.sh:872` |
| Observed behavior | Bash itself prints, with its script path and line: `chat.sh: line 281: warning: command substitution: ignored null byte in input` (NUL bytes in piped input, dropped; line 194 for ai_ask), `chat.sh: line 210: /dev/tty: No such device or address` (ai_ask without a terminal, AI-04), `sysinfo.sh: line 872: <file>: No such file or directory` or `Permission denied` (failed report save, AI-09). |
| Reproduction | `printf 'a\0b\n' \| env -i HOME="$d" PATH="$d/bin:/usr/bin:/bin" bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; ai_query claude what is this'` with the AI-02 fake curl → the null-byte warning |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | These lines name Bash source files and lines that do not exist in 3.x and come from the interpreter, so rc.A prints nothing in their place; NUL bytes are dropped silently; the rest of each flow (what is sent, the success line of AI-09, exit codes) is as 2.x. |
| Fix recipe (3.0) | None beyond rc.A; AI-04 and AI-09 fixes add proper messages. |
| Proving test | `TestLedger_AI20_2x` in `internal/ai` (NUL input: stderr has no `warning:` line, body has `ab`) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="ai-21"></a>
#### AI-21 · `grep -P` availability decides the context distro

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (A3, ai part) |
| Area | Portability |
| Bash location | `lib/modules/ai/chat.sh:27` |
| Observed behavior | `grep -oP '^ID=\K\w+' /etc/os-release \|\| echo linux`: BusyBox and BSD grep have no `-P`, so every such host (Alpine, a Linux with BSD grep) reports `"distro":"linux","pkg":"unknown"`, even `alpine`, which the table maps to apk. |
| Reproduction | `grep -oP '^ID=\K\w+' /etc/os-release` on GNU grep vs `busybox grep -oP '^ID=\K\w+' /etc/os-release` (no `-P`) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The expression is evaluated natively (first line `^ID=` followed by `[A-Za-z0-9_]+`), the GNU grep result on every host; the quoting gaps stay (AI-11). |
| Fix recipe (3.0) | None beyond rc.A (AI-11 fixes the parsing). |
| Proving test | `TestLedger_AI21_2x` in `internal/ai` (`ID=alpine` → alpine/apk with no grep present) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="ai-22"></a>
#### AI-22 · No request temp files, and no orphaned sudo keep-alive, after an interrupt

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N30) |
| Area | Security |
| Bash location | `lib/modules/ai/client.sh:205-215`; `lib/modules/sysinfo.sh:66-71`; `lib/modules/ai/sysinfo.sh:658` |
| Observed behavior | The request body (redacted question and context) and the reply sit in `mktemp` files (0600) that are removed after the call but not on Ctrl+C (a 2174-byte `tmp.*` stayed in `$TMPDIR` in the dossier run); ai_sysinfo's disowned `sudo -n true; sleep 50` keep-alive is killed by a RETURN trap that an interrupt skips, so it can keep the sudo timestamp fresh after the function is gone (inferred: the run was as root, where no keep-alive starts). |
| Reproduction | `mkdir -p "$d/t"; env -i HOME="$d" PATH="$d/bin:/usr/bin:/bin" TMPDIR="$d/t" bash -c 'timeout -s INT 1 bash -c "GASH_HEADLESS=1; source $GASH_SRC/gash.sh; ai_query claude hi" </dev/null; sleep 5; ls -l "$TMPDIR"'` with `$d` as in AI-02 and a fake curl that sleeps 5 s → four `tmp.*` files left, one of them the 2 KB request body |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The request is built and sent in memory (no temp files, `TMPDIR` unused); a keep-alive, if ported, is a goroutine or a child in the binary's process group, ended with the process on SIGINT/SIGTERM. Neither leftover can occur. |
| Fix recipe (3.0) | None beyond rc.A. |
| Proving test | `TestLedger_AI22_2x` in `internal/ai` (SIGINT during a slow fake request: no file created under `TMPDIR`, no child left) |
| Status | accept: no 3.0 change (Fix recipe: none) |

### file scanners

<a id="fs-20"></a>
#### FS-20 · A newline inside an --exclude value turns it into extra find predicates

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N1) |
| Area | Security |
| Bash location | `lib/modules/files.sh:201-225` (`__gash_fs_find_prune_args` prints one find word per line), callers rebuild the array with `while IFS= read -r prune_line` at `457-459`, `1180-1182`, `1374-1376`, `1544-1546` |
| Observed behavior | In files_largest, dirs_list_empty and tree_stats (both walks) an exclude value with embedded newlines is split into several find words, so a value shaped `GLOB` NL `-ACTION` NL `ARG…` NL `;` reaches find as `( -path GLOB -ACTION ARG… ; -prune ) -o …` and any find action (`-exec`, `-delete`, `-fprint`) runs on the matching entries. No shell is involved. dirs_largest and dirs_find_large pass the value to du as one word and are not injectable. |
| Reproduction | Input shape: an `--exclude` value whose lines are a match-all glob, `-exec`, `touch`, a marker path inside a fresh temp dir, and `;`, scanning that temp dir; the only effect is an empty `MARKER` file there. Kept outside the repository (`work/files-scanners/repro-inject.sh`). |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Go never re-parses a user string as find predicates: the native walk cannot be injected (MUST NOT 7). An `--exclude` value containing a newline gets the error contract in all five scanners (one grammar): `invalid_filter` RETRY (the existing argument-value code of AGENTS 5; no exclude-specific code exists), details = the value (JSON-escaped), hint `--exclude takes a glob without newlines, e.g. '*/node_modules'`, rc 1, nothing on stdout. 2.x's injected predicates cannot be kept; dirs_largest/dirs_find_large, which accepted such a value as one du glob matching nothing, diverge the same way. |
| Fix recipe (3.0) | Keep the rc.A rule in `internal/cli/args` (scanners family, `--exclude` value check); list `invalid_filter` for the scanners in AGENTS 5 and add "no newlines" to each `--exclude` help line in 3.0. |
| Proving test | `TestLedger_FS20` in `internal/cli/args` (for each scanner: the reproduction's value gives the `invalid_filter` envelope, stdout empty, rc 1) and `testdata/script/fsscan_exclude_newline.txtar` (no `MARKER` exists after the five calls). |
| Status | accept: done d6a4bf9, ca0b3dd |

<a id="fs-21"></a>
#### FS-21 · dirs_largest / dirs_find_large --depth goes through Bash arithmetic

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | Seed: "integers of any length are accepted" (CONFIRMED: Bash-arithmetic part) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:644` (`[[ "$max_depth" -lt 1 ]] && max_depth=1`), `932` (`[[ -n "$max_depth" && "$max_depth" -gt 0 ]] && du_cmd+=(-d "$max_depth")`) |
| Observed behavior | The depth passes `^[0-9]+$` and is then compared in Bash arithmetic: `010` is octal 8 (and du's base-0 parse agrees); `08`/`09` print `files.sh: line 644: [[: 08: value too great for base (error token is "08")` on stderr even under `--json`, then dirs_largest returns du's refusal as `"data":[]`, `errors:2`, rc 0 and dirs_find_large silently scans unbounded; `9223372036854775808` wraps negative (dirs_largest silently depth 1, dirs_find_large unbounded); `99999999999999999999` wraps positive and du refuses it (`errors:2`). |
| Reproduction | `d=$(mktemp -d); dirs_largest --json --depth 08 "$d"` prints the raw Bash error line on stderr and an empty envelope with `"errors":2`, rc 0 |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Decimal values without a leading zero up to 9223372036854775807 behave as on the GNU path (`0` becomes 1 in dirs_largest and means unbounded in dirs_find_large). A value with a leading zero (other than `0` itself) or above 9223372036854775807 gets `invalid_depth` RETRY with the scanner's 2.x hint (`Provide a positive integer` in dirs_largest, `Provide a non-negative integer` in dirs_find_large), details = the raw value, rc 1. Those meanings came only from Bash arithmetic (octal, base errors, 64-bit wrap); Go never evaluates user strings as arithmetic. |
| Fix recipe (3.0) | Superseded by the FS-05 rule (1 to 9 ASCII digits, base 10: `010` = 10, `08` = 8). |
| Proving test | `TestLedger_FS21` in `internal/cli/args`: `010`, `08`, `09`, `9223372036854775808`, `99999999999999999999` give `invalid_depth` for both scanners with empty stdout; `0`, `1`, `2147483648` scan. |
| Status | accept: done d6a4bf9 |

<a id="fs-22"></a>
#### FS-22 · The size grammar depends on numfmt, and thresholds overflow in Bash arithmetic

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N4, tool and arithmetic part) |
| Area | Portability |
| Bash location | `lib/modules/files.sh:49-82` (`numfmt` first, fallback with `$((num * 1024 …))` at `75-79`), `927` (`threshold_kb=$(((threshold_bytes + 1023) / 1024))`) |
| Observed behavior | With numfmt, `1.5K` = 1536, `0.5K` = 512, `1.9999K` = 2048, `1E` = 2^60 and `8E` = 2^63 are valid; without numfmt (BusyBox, macOS) the same inputs are `invalid_size` and `08` is returned verbatim, which aborts dirs_find_large at line 927 (`value too great for base`, rc 1, no JSON envelope). `--size 8E`, `9223372036854775807` or `8192P` make `threshold_kb` wrap negative, so dirs_find_large lists every directory; the fallback product wraps too (`9999999P` is accepted as 6484057563506671616). |
| Reproduction | `d=$(mktemp -d); mkdir "$d/a"; dirs_find_large --json --size 8E "$d"` lists `$d/a` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | The numfmt-present grammar on every platform (numfmt `--from=iec` emulated, then the fallback rules of FS-11). A value whose 2.x meaning came from wrapped 64-bit Bash arithmetic gets `invalid_size` RETRY with the scanner's 2.x hint, rc 1: a fallback product above 2^63-1 (all three size flags), and a dirs_find_large size above 9223372036854774784 where `(bytes+1023)/1024` overflows. `--min-size` values numfmt accepts up to 2^63 still compare as float64, as awk did. |
| Fix recipe (3.0) | The FS-11 native parser (64-bit integers, refuse above 2^63-1). |
| Proving test | `TestLedger_FS22` in `internal/fsscan`: `--size 8E`, `--size 9223372036854775807`, `--size 8192P`, `--min-size 9999999P` give `invalid_size`; `1.5K`, `1E`, `08` parse as numfmt does with no numfmt on PATH. |
| Status | accept: done d6a4bf9 |

<a id="fs-23"></a>
#### FS-23 · awk -v unescapes the scan root and the --exclude values

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N6) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:714`, `977` (`-v self="$safe_path"`), `721-724`, `983-986` (`-v user_excludes=`), `1800` (`-v root=` in the rich renderer) |
| Observed behavior | POSIX awk processes backslash escapes in `-v` values: a ROOT named `a\tb` (backslash, t) is not recognised as itself and is listed as its own row by dirs_largest and dirs_find_large; `--exclude 'x\ty'` hides a directory named `x<TAB>y`. |
| Reproduction | `d=$(mktemp -d); mkdir -p "$d/a\\tb/sub"; dirs_largest --json "$d/a\\tb"` lists the root itself |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Literal string comparison: the root is never listed and `x\ty` matches only that literal text. awk's escape processing is a property of the awk language, not of the scanners' contract; Go has no counterpart to reproduce. |
| Fix recipe (3.0) | None beyond rc.A. |
| Proving test | `TestLedger_FS23` in `internal/fsscan`: both reproductions (root not listed; `--exclude 'x\ty'` hides nothing). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="fs-24"></a>
#### FS-24 · Without GNU du, prune names act as regexes and pruned trees still count

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N8) |
| Area | Portability |
| Bash location | `lib/modules/files.sh:95-104` (probe `du --help \| grep -q -- --exclude`, cached in `__GASH_FS_DU_HAS_EXCLUDE`), `732-739`, `992-999` (`p ~ "/" name "/"`) |
| Observed behavior | Where du has no `--exclude` (BSD/macOS, BusyBox, whose help goes to stderr so the probe fails, or an inherited `__GASH_FS_DU_HAS_EXCLUDE=0`), the awk filter treats the default prune names as regexes: `.git` also drops `xgit` and `agit/q`, `.cache` drops `Xcache`; pruned directories still count in their parents' sizes (`proj` 110592 vs 90112 bytes with GNU du). |
| Reproduction | With a PATH whose du lacks `--exclude` (`work/files-scanners/bbbin`): `d=$(mktemp -d); mkdir "$d/xgit"; head -c 5000 /dev/zero >"$d/xgit/f"; dirs_largest --json "$d"` omits `xgit` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | The GNU du semantics on every platform (native walk, no probe, no cache variable). |
| Fix recipe (3.0) | None beyond rc.A. |
| Proving test | `TestLedger_FS24` in `internal/fsscan`: the fixture gives the GNU-du result whatever du is on PATH. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="fs-25"></a>
#### FS-25 · A relative PATH is resolved through CDPATH

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N10) |
| Area | Portability |
| Bash location | `lib/modules/files.sh:171` (`resolved="$(cd -- "$input" 2>/dev/null && pwd -P)"`: CDPATH applies and `cd` prints the directory, so `resolved` holds two lines) |
| Observed behavior | When a relative PATH exists both in the working directory and under a CDPATH entry, `cd` goes to the CDPATH one and the two-line result is walked: `{"data":[],"count":0,…,"errors":1}`, rc 0. `./src` works. All five scanners. |
| Reproduction | `d=$(mktemp -d); mkdir -p "$d/a/src" "$d/b/src"; printf x >"$d/a/src/f"; cd "$d/a"; CDPATH="$d/b" files_largest --json src` gives an empty envelope with `"errors":1` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | A relative PATH is resolved against the working directory only (`filepath.Abs`, then `EvalSymlinks`); CDPATH is not read, so the reproduction lists `$d/a/src/f`. CDPATH is a feature of Bash's `cd` builtin, and its 2.x effect (a broken two-line path) is an accident a binary cannot and should not imitate. |
| Fix recipe (3.0) | None beyond rc.A. |
| Proving test | `TestLedger_FS25` in `internal/fsscan` (and `testdata/script/fsscan_cdpath.txtar` through the shim with CDPATH set in the caller): the reproduction lists `a/src/f`. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="fs-26"></a>
#### FS-26 · A find without -printf (BusyBox, BSD, macOS) empties files_largest and tree_stats

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N11, tool-variant part) |
| Area | Portability |
| Bash location | `lib/modules/files.sh:468`, `1384` (`-printf`), `484-492`, `1396-1398` (`\|\| true`) |
| Observed behavior | With BusyBox 1.36 find or a BSD find, files_largest returns `"data":[]` and tree_stats all counters 0, rc 0, with `errors` = lines of find's complaint (BusyBox: 58, message plus usage); text mode says `Warning: Scan completed with 58 unreadable path(s) (permission denied).` macOS inferred. |
| Reproduction | `d=$(mktemp -d); printf x >"$d/f"; PATH=<dir of BusyBox applets>:$PATH files_largest --json "$d"` gives an empty envelope with `"errors":58` (`work/files-scanners/bbbin`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Native walk: the GNU-path result on every platform, whatever find is installed (MUST NOT 8). |
| Fix recipe (3.0) | None beyond rc.A. |
| Proving test | `TestLedger_FS26` in `internal/fsscan`: with a BusyBox-like find first on PATH (testscript), files_largest and tree_stats give the GNU-path results. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="fs-27"></a>
#### FS-27 · Without GNU du a newline-named directory is reported under a truncated, wrong path

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N12) |
| Area | Portability |
| Bash location | `lib/modules/files.sh:681-683`, `950-952` (newline exclusion only when the probe found GNU `--exclude`) |
| Observed behavior | With BSD or BusyBox du, a directory `n<NL>x` (9000 B) next to `n` is reported by dirs_largest and dirs_find_large as `[{"size":16384,"path":"…/n"},{"size":8192,"path":"…/ok"},{"size":4096,"path":"…/n"}]` (the first row is the newline dir under the wrong name); `--null` gives `n`, `ok`, `n`. GNU du: `ok`, `n`. |
| Reproduction | `d=$(mktemp -d); mkdir "$d/$(printf 'n\nx')" "$d/n" "$d/ok"; head -c 9000 /dev/zero >"$d/$(printf 'n\nx')/f"; dirs_largest --json "$d"` with a du lacking `--exclude` first on PATH |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | The GNU path: newline directories are left out, also from parent totals. |
| Fix recipe (3.0) | FS-01 predicate (unchanged for newlines). |
| Proving test | `TestLedger_FS27` in `internal/fsscan`: the fixture gives `ok`, `n` with the GNU-path sizes under any PATH. |
| Status | accept: done d6a4bf9 |

<a id="fs-28"></a>
#### FS-28 · The order of equal entries depends on the locale and the awk implementation

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N13) |
| Area | Portability |
| Bash location | `lib/modules/files.sh:486-487`, `773`, `1016` (`sort -nr -k1,1 -t$'\t'`: ties by reversed whole-line comparison in LC_COLLATE), `1417-1513` (tree_stats tops by repeated max selection over `for (e in arr)`, `tolower`) |
| Observed behavior | Ten 1000-byte files: C/C.UTF-8 order `g.Ünï f.PNÉ e.py d.rs c.go b.md a _c Z.txt B`, en_US/de_DE `Z.txt g.Ünï f.PNÉ e.py d.rs c.go _c b.md B a`; the sorted line includes the mtime, so a newer file wins a size tie. tree_stats ties follow awk's hash order (mawk, gawk-UTF-8 and gawk-C all differ); gawk in a UTF-8 locale lowercases non-ASCII extensions (`PNÉ` -> `pné`), mawk and gawk-C do not. |
| Reproduction | `d=$(mktemp -d); for n in a B _c Z.txt; do head -c 1000 /dev/zero >"$d/$n"; done; LC_ALL=en_US.UTF-8 files_largest --json "$d"; LC_ALL=C files_largest --json "$d"` (different orders; locale must be installed) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Size lists: size descending, then the 2.x row text (`BYTES\tMTIME\tPATH`) in C byte order descending, which equals 2.x under C/C.UTF-8. tree_stats tops: count desc, size desc, extension asc in bytes (top_by_size: size desc, count desc, extension asc); `tolower` ASCII only (gawk in the C locale). gawk's hash iteration order is an implementation detail and is not reproduced. Table widths stay counted in characters as 2.x counts them in a UTF-8 locale (architecture.md section 5; inferred effect on non-ASCII paths only). |
| Fix recipe (3.0) | Keep the rc.A tie rules and document them in AGENTS 4.1 and 4.3. |
| Proving test | `TestLedger_FS28` in `internal/fsscan`: the variant fixture gives the C order under every LANG/LC_ALL; tree_stats tops are identical over 20 runs and across locales. Because this top order is not reproduced, the differential helper (`normScan`/`same2x`) canonicalizes the `top_by_count`/`top_by_size` element order before comparing go to live 2.x, so a 2.x awk-hash order (e.g. FS-26 on mawk) does not fail the exact diff; the entries and every other field are still compared exactly. |
| Status | accept: done ca0b3dd (rc.A tie rules documented) |

<a id="fs-29"></a>
#### FS-29 · Non-ASCII digits pass integer and size validation in UTF-8 locales

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N17) |
| Area | Portability |
| Bash location | `[[ … =~ ^[0-9]+$ ]]` at `lib/modules/files.sh:424`, `428`, `636`, `640`, `911`, `915`, `1161`, `1165`, `1355`, `1359`; `${s//[^0-9]/}` in `__gash_fs_parse_size` (`49-82`) |
| Observed behavior | In en_US, it_IT and de_DE UTF-8 locales `[0-9]` follows collation and matches `١ ۵ १ ১ ๑ １ ² ½ ⑤ 𝟙`: `files_largest --json -n ١ D` lists everything (awk string compare), `dirs_largest --json --depth ١ D` prints `files.sh: line 644: [[: ١: syntax error…` and `"data":[]`, `dirs_list_empty --json --depth ١ D` gives `errors: 1` and no rows, `dirs_find_large --size ١` aborts at line 927. In C/C.UTF-8 each gives its `invalid_*` code. |
| Reproduction | `LC_ALL=en_US.UTF-8 files_largest --json -n ١ "$(mktemp -d)"` succeeds (the locale must be installed) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | ASCII digits only (the C-locale path): such a value gets its scanner's `invalid_limit`, `invalid_depth`, `invalid_top` or `invalid_size` RETRY envelope with the 2.x hint, rc 1, in every locale. |
| Fix recipe (3.0) | None beyond rc.A (Go `regexp` and `strconv` are ASCII). |
| Proving test | `TestLedger_FS29` in `internal/cli/args`: under `LC_ALL=en_US.UTF-8`, `١` is refused by every integer and size flag of the five scanners. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="fs-30"></a>
#### FS-30 · An unusable TMPDIR forces errors to 0 and leaks a mktemp message

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (bookkeeping check; not in the dossier, verified on the dossier's test bed) |
| Area | Portability |
| Bash location | `lib/modules/files.sh:475`, `691`, `960`, `1193`, `1390` (`err_tmp="$(mktemp)" \|\| err_tmp=""`, then the walker's stderr goes to /dev/null) |
| Observed behavior | With TMPDIR naming a missing or read-only directory, mktemp prints `mktemp: failed to create file via template '/nonexistent/tmp.XXXXXXXXXX': No such file or directory` on stderr (also under `--json`), the walker's complaints are discarded and `errors` is always 0: `files_largest --json --depth 2147483648 D` gives `errors: 0` instead of 1. |
| Reproduction | `d=$(mktemp -d); TMPDIR=/nonexistent files_largest --json --depth 2147483648 "$d"` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | No temp file: errors are counted in-process (FS-15 rule) and no mktemp line is printed; TMPDIR is not read. The temp file is Bash stderr transport, so its failure mode depends on the environment rather than on the contract. |
| Fix recipe (3.0) | None beyond rc.A. |
| Proving test | `TestLedger_FS30` in `testdata/script/fsscan_tmpdir.txtar`: with `TMPDIR=/nonexistent` the envelope still says `"errors":1` and stderr is empty. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="fs-50"></a>
#### FS-50 · tree_stats evaluates text from a scanned file name as Bash arithmetic

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of lib/modules/files.sh, files worker; confirmed by the lead on a 2.1.0 copy) |
| Area | Security |
| Bash location | `lib/modules/files.sh:1664-1666` and `:1675-1677` (terminal renderer of tree_stats) |
| Observed behavior | The "By size" and "By count" rows are split with `IFS=$'\t' read -r ext cnt sz`, and the count and size fields go straight into `$((${sz:-0} * 1000 / ...))` and `$((${cnt:-0} * 1000 / ...))`. An extension that holds a tab shifts the split (FS-17), so text from a file name in the scanned tree reaches Bash arithmetic evaluation, which runs command substitutions inside array subscripts. Anyone who can create a file in a tree the user later runs `tree_stats` on (a cloned repository, an extracted archive, a shared directory) can run commands as that user. The `--json` output does not take this path. |
| Reproduction | the reproduction kept outside the repository: a file whose extension holds a tab followed by an array subscript with a command substitution that creates an empty marker file; `tree_stats DIR` on a terminal and the marker appears in the current directory |
| Severity | critical |
| Public-contract impact | yes |
| rc.A behavior | Never evaluated: the fields are read as digits only (`ui.Int`, CORE-02), anything else counts as 0, and the rows keep the 2.x layout for every ordinary name. |
| Fix recipe (3.0) | Same as rc.A. 2.x patch (for a 2.1.x release, D-7): check `cnt` and `sz` against `^[0-9]+$` before the arithmetic and treat anything else as 0, as `__gash_fs_parse_size` already does for its input. |
| Proving test | `TestLedger_FS50` in `internal/fsscan` (a name with a tab and an arithmetic payload: no command runs, the row reads 0); in a 2.1.1 series, a regression spec that fails on 2.1.0. |
| Status | accept: done (rc.A); TestLedger_FS50 c3522e5 |

### archive_extract and file_backup

<a id="file-16"></a>
#### FILE-16 · `nocasematch` stays on in the caller's shell after Ctrl-C

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N7) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:1916-1919,1953` |
| Observed behavior | `shopt -s nocasematch` is undone by `eval` of the saved `shopt -p` line only on a normal return; there is no trap, so an interrupt (Ctrl-C while tar runs) leaves `nocasematch` on in the interactive shell and changes every later `[[ == ]]` and `case` of the session |
| Reproduction | Interactive PTY with a `tar` on PATH that sleeps 5 s (`$W/bin_slowtar`): `shopt nocasematch` → `off`; `archive_extract a.tgz`, Ctrl-C; `shopt nocasematch` → `on` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Not reproduced: a binary cannot change an option of the parent shell and the stub sets none; suffixes are matched in Go (`strings.EqualFold`). An interrupted call leaves the caller's options as they were |
| Fix recipe (3.0) | None in 3.x (fixed by construction). For a 2.x patch release: drop `shopt`, use `local lc=${archive_file,,}; case "$lc" in` |
| Proving test | `testdata/script/archive_extract_interrupt.txtar`: under a PTY, SIGINT during a slow `tar` stub, then `shopt nocasematch` prints `off`; `TestArchiveSuffixMatch` in `internal/files` (case table) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="file-17"></a>
#### FILE-17 · `.tar.zst` exit status depends on the caller's `pipefail`

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N8) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1926` |
| Observed behavior | `zstd -dc -- F \| tar xvf - ...` returns tar's status unless the caller has `set -o pipefail`. A `.tar.zst` with trailing garbage (`zstd: t.tar.zst: unsupported format` after a complete tar stream) gives rc 0 without pipefail and rc 1 with it, same stderr both times |
| Reproduction | `R "{ cat $W/fx1/a.tar.zst; printf garbage; } > t.tar.zst; archive_extract t.tar.zst o >/dev/null; echo \$?; set -o pipefail; archive_extract t.tar.zst o2 >/dev/null; echo \$?"` → `0`, `1` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Pipefail-on semantics whatever the caller set: rc 1 when zstd or tar exits non-zero. The binary cannot see the caller's shell options; this is the path the spec exercises (`files_spec.sh:394` runs under `set -euo pipefail`). Where 2.x returned 0 (no pipefail, zstd failed, tar succeeded), rc.A returns 1 |
| Fix recipe (3.0) | None beyond rc.A: wait for both processes, rc 1 when either fails; same rule for the FILE-13 `.tar.Z` pipeline |
| Proving test | `TestLedger_FILE17` in `internal/files` (`runner.Fake`: zstd 1 / tar 0 → 1; 0/1 → 1; 0/0 → 0); `testdata/script/archive_extract_tzst.txtar`: the garbage-tail archive → rc 1 from a caller without pipefail |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="file-18"></a>
#### FILE-18 · The caller's `noclobber` makes single-file extraction fail

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (dossier A-2, noclobber) |
| Area | Correctness |
| Bash location | `lib/modules/files.sh:1931,1934,1939,1941,1946` |
| Observed behavior | With `set -o noclobber` (`set -C`) in the caller, the `>` redirection refuses an existing output: `$GASH_SRC/lib/modules/files.sh: line 1941: ./s.txt: cannot overwrite existing file`, rc 1, the file kept; without it the file is overwritten |
| Reproduction | `R 'printf "hi\n" \| gzip > s.txt.gz; printf old > s.txt; set -C; archive_extract s.txt.gz; echo rc=$?; cat s.txt'` → the Bash line, `rc=1`, `old` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | noclobber ignored: the binary cannot see the caller's shell options, so the output is always opened with `O_TRUNC` (the default path) and an existing file is overwritten, as 2.x does without `set -C` |
| Fix recipe (3.0) | None; the FILE-03 temp-and-rename keeps "replaced on success" as the one rule, stated in the help NOTES (FILE-26) |
| Proving test | `testdata/script/archive_extract_noclobber.txtar`: a `set -C` caller, existing `s.txt` replaced, rc 0 |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="file-19"></a>
#### FILE-19 · Missing tools are reported by Bash's "command not found" text

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (dossier A-5, A-14) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:1921-1947` (every branch but zstd) |
| Observed behavior | Only zstd is checked (`Error: Extracting .zst needs zstd.`). Elsewhere Bash prints its own line, whose prefix depends on how Gash was loaded (`$GASH_SRC/lib/modules/files.sh: line 1945: unzip: command not found` from the sourced file, `bash: tar: command not found` at a prompt), rc 1, OUTPUT_DIR left behind and, for single-file formats, the empty output (FILE-03). A missing decompressor under tar is reported by tar (`tar (child): gzip: Cannot exec: No such file or directory` plus three lines), unchanged in rc.A |
| Reproduction | `RPATH=$W/bin_nounzip R "cp $W/fx1/a.zip .; archive_extract a.zip; echo rc=\$?"` → `$GASH_SRC/lib/modules/files.sh: line 1945: unzip: command not found`, `rc=1` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Bash's text names the sourcing file and line (or `bash`) and cannot be reproduced byte for byte. rc.A emits the 2.x dependency error contract: `Error: Command 'unzip' is not installed or not in PATH.` (the `__gash_require_command` default, `lib/core/validation.sh:29`, through the `Error:` emitter and its color gate), rc 1, after the 2.x file effects (OUTPUT_DIR created; single-file output opened and truncated first). zstd keeps `Error: Extracting .zst needs zstd.`; for `.tar.zst` with tar missing, zstd is not started |
| Fix recipe (3.0) | Keep the line; check the branch's tools before creating anything (FILE-03, FILE-07) |
| Proving test | `TestLedger_FILE19` in `internal/files`: PATH without each tool gives the exact line and rc 1; the single-file case leaves the empty output in rc.A; `testdata/script/archive_extract_missing_tool.txtar` |
| Status | accept: done a2a30ac |

<a id="file-20"></a>
#### FILE-20 · User shell functions named like the tools are called by 2.x

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (dossier accidents of both functions) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:1905,1921-1947,1973-1974`; `lib/core/validation.sh:25` (`command -v`) |
| Observed behavior | Inside a Gash function, a user's shell function `tar`, `unzip`, `7z`, `unrar`, `zstd`, `gunzip`, `mkdir`, `basename`, `date` or `cp` runs instead of the binary (aliases do not: `gash.sh:100-101` turns `expand_aliases` off while Gash files are sourced); a function named `zstd` also passes the `command -v` guard |
| Reproduction | `R 'printf x > a; date(){ echo FIXED; }; file_backup a'` → `'a' -> 'a_backup_FIXED'` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Not reproduced: a binary cannot call the caller's shell functions and Go never executes user-supplied code. It runs the executables found on the PATH it receives (`exec.LookPath`; shell functions, aliases and `hash -p` entries ignored) and takes the stamp per FILE-25 |
| Fix recipe (3.0) | None (by construction); `docs/port` notes that tool overrides must be executables on PATH |
| Proving test | `testdata/script/files_shadow.txtar`: with `date(){ echo FIXED; }` and `tar(){ return 9; }` defined in the caller, the backup name ends in 14 digits and the `.tgz` extracts, rc 0 |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="file-21"></a>
#### FILE-21 · The `cp -v` line of file_backup depends on the locale

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (dossier file_backup output) |
| Area | Portability |
| Bash location | `lib/modules/files.sh:1974` |
| Observed behavior | GNU cp `-v` quotes names in shell-escape-always style. Valid UTF-8 multibyte characters print raw in a UTF-8 locale (`'più' -> 'più_backup_…'`) and as `$'\ooo'` octal per byte in C (`'pi'$'\303\271' -> 'pi'$'\303\271''_backup_…'`). Control bytes (`$'\t'`, `$'\033'`, `$'\177'`), invalid UTF-8 bytes (`$'\377'`), quotes, spaces and `*?$~=` print identically in both (checked with coreutils 9.4). Under a translated locale cp's diagnostics are translated. BSD/macOS cp prints `a -> a_backup_…` unquoted (inferred) |
| Reproduction | `R 'printf x > più; file_backup più'` → `'più' -> 'più_backup_<stamp>'`; `RLANG=C R 'printf x > più; file_backup più'` → `'pi'$'\303\271' -> 'pi'$'\303\271''_backup_<stamp>'` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | rc.A runs cp with `LC_ALL=C` added to its environment, so the line always has C-locale quoting: every byte 0x80-0xFF is written as `\ooo` (three octal digits) inside `$'…'`, consecutive such bytes in one group, exactly as 2.x printed under `LANG=C`; ASCII names, control bytes and invalid bytes print as under every 2.x locale; cp's diagnostics are untranslated English with `'…'` quotes. Differs from 2.x only for non-ASCII names, or translated messages, under a non-C locale; the caller's locale would otherwise decide the stdout line. On macOS the system cp's own format stays, as in 2.x there |
| Fix recipe (3.0) | None required. Optional: print the line in Go (`internal/ui` shell-escape quoting) to drop the GNU/BSD difference |
| Proving test | `TestLedger_FILE21` in `internal/files`: caller `LANG=C.UTF-8`, name `più` → stdout `'pi'$'\303\271' -> 'pi'$'\303\271''_backup_<14 digits>'`; `testdata/script/file_backup_quoting.txtar` (tab, ESC, `it's`, `a b`) |
| Status | accept: no 3.0 change (Fix recipe: none) |

### shell layer

<a id="shell-01"></a>
#### SHELL-01 · The prompt re-expands the git branch name, VIRTUAL_ENV, debian_chroot and STY as code

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N1) |
| Area | Security |
| Bash location | `lib/shell/prompt.sh:104,130` (branch), `:168` (`debian_chroot`), `:171` (`${VIRTUAL_ENV##*/}`), `:219-221` (`${STY%%.*}`); PS1 assigned at `:229` |
| Observed behavior | `__gash_ps1_build` splices these strings raw into PS1; Bash (`promptvars` on) re-expands PS1 before every prompt, so `$(...)`, backticks and `${...}` inside them run. Git refnames allow the characters a command substitution needs, and a clone checks out the remote's default branch, so cloning a hostile repository and `cd` into it runs its code with no other user action. Confirmed in a real pty (marker file created when the prompt is drawn). Not reachable through the agent API (headless mode builds no prompt). |
| Reproduction | the reproduction kept outside the repository: a repository whose current branch name holds a command substitution that creates an empty marker file in a temporary directory; `cd` into it in an interactive Gash shell (or run `__gash_prompt_command; : "${PS1@P}"`) and the marker appears. The branch name used is kept outside the repository. |
| Severity | critical |
| Public-contract impact | no (ordinary names render byte-identically) |
| rc.A behavior | The emitted hook stores each untrusted string in its own variable (`__GASH_PS1_BRANCH`, `__GASH_PS1_VENV`, `__GASH_PS1_CHROOT`, `__GASH_PS1_STY`) and PS1 holds only the reference (`${__GASH_PS1_BRANCH}`), so Bash expands it once and never re-scans the value: ordinary names render byte for byte as in 2.x and no text from a branch, venv path, chroot file or screen session is ever executed (PRD MUST NOT 7: no shell re-parsing of user strings). Reproducing the 2.x execution is not allowed. Prompt cost unchanged (still no exec outside repositories, one `git status` inside). |
| Fix recipe (3.0) | Same as rc.A (the divergence is the fix). In `internal/shellinit` the hook template assigns `__GASH_PS1_BRANCH=$branch` etc. and appends the literal text `${__GASH_PS1_BRANCH}` to `ps1`; a shellinit lint test fails if any non-palette variable is concatenated into `ps1` by value. Document in `docs/configuration.md` (Prompt) that names are shown literally. |
| Proving test | `TestLedger_SHELL01` in `internal/shellinit` (the template references, never interpolates, the four strings); `testdata/script/prompt_hostile_names.txtar` (branch names with `$(...)`, backticks, `${...}`; `VIRTUAL_ENV`, `debian_chroot`, `STY` with the same shapes: after `__gash_prompt_command; : "${PS1@P}"` no marker file exists and the expanded prompt contains the literal text; ordinary names give the 2.x golden PS1) |
| Status | accept: done ba208f0 |

<a id="shell-07"></a>
#### SHELL-07 · GASH_PATH_ADD is word-split on the caller's IFS and glob-expanded

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N3) |
| Area | Correctness |
| Bash location | `lib/shell/toolchains.sh:16-19` (`path_add ${GASH_PATH_ADD}` unquoted) |
| Observed behavior | The value is split on whatever `IFS` the caller has when `gash.sh` is sourced and each word is pathname-expanded: `GASH_PATH_ADD="$HOME/bin*"` adds every matching directory; with `IFS=:` set before sourcing, `"a:b"` becomes two directories (with the default IFS it is one non-existent directory, skipped). `~` and `~/...` at the start of a word are expanded afterwards by `path_add` (`__gash_expand_tilde_path`, `lib/core/utils.sh:311-323`); `~user` is not. |
| Reproduction | `$E GASH_PATH_ADD="$HOME/bin*" bash --noprofile --norc -i -c 'source $C/gash.sh >/dev/null 2>&1; echo "$PATH"' </dev/null` with `~/bin1` and `~/bin2` present: both are prepended |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The binary splits the value on space, tab and newline regardless of `IFS`, never globs, and emits `path_add 'w1' 'w2' ...` (single-quoted words), so every other rule is the 2.x `path_add`: `~` and a leading `~/` expanded (not `~user`), one trailing `/` dropped, missing directories skipped, existing PATH entries kept in place, prepended in the order given (a first word `--append`, `-h` or `--help` keeps its 2.x meaning: SHELL-08). A word with glob characters is taken literally (normally a missing directory, skipped). Glob expansion and IFS-dependent splitting of a user string cannot be kept (they depend on the caller's shell state and the filesystem at load). |
| Fix recipe (3.0) | Same as rc.A. `internal/shellinit`: `config.SplitPathAdd(v)` = `strings.FieldsFunc(v, func(r rune) bool { return r == ' ' \|\| r == '\t' \|\| r == '\n' })`. Docs: `.gash_env.template:116-118` and `docs/configuration.md` say "separated by spaces; no wildcards". |
| Proving test | `TestLedger_SHELL07` in `internal/shellinit` (`"~/a  ~/b\t/c*"` → `path_add '~/a' '~/b' '/c*'`); `testdata/script/toolchains_path_add_split.txtar` (`IFS=:` before sourcing does not change the result; a `*` word adds nothing) |
| Status | accept: done f40af4e |

<a id="shell-10"></a>
#### SHELL-10 · Loading leaves BASH_REMATCH and readonly or array internals in the caller's shell

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N9) |
| Area | Contract |
| Bash location | `lib/shell/terminal.sh:38` (`=~` on `GASH_UMASK`), `lib/shell/toolchains.sh:39` (`=~` in `__gash_nvm_default_dir`); `lib/modules/ai/prompts.sh:18-92` (`readonly __AI_*`); `__LLM_*`, `__REMOTE_*`, `__SYSINFO_*` globals of the modules; CONTRIBUTING.md:45-46 forbids leaving `BASH_REMATCH` |
| Observed behavior | With `GASH_UMASK=027` (or `GASH_NVM=lazy` with a resolvable default) `declare -p BASH_REMATCH` shows `([0]="027")` after load. Every load also defines about 200 `__GASH_*` variables, readonly `__AI_*` constants (cannot be unset, survive `gash_unload`) and the module arrays and strings. |
| Reproduction | `$E GASH_UMASK=027 bash --noprofile --norc -i -c 'source $C/gash.sh >/dev/null 2>&1; declare -p BASH_REMATCH' </dev/null` → `declare -a BASH_REMATCH=([0]="027")` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The init script and the generated shims test values without regexes (the binary validates `GASH_UMASK` and resolves the nvm directory), declare no readonly variables and none of the module internals (they live in the binary), so `BASH_REMATCH` is untouched and nothing readonly is left. A binary cannot leave Bash internals in the caller's shell, and the shim must not (`compat.md` 4: sourcing leaves no `BASH_REMATCH` or other globals). Only the documented `__GASH_*` bookkeeping that `gash_unload` and completion need remains. |
| Fix recipe (3.0) | Same as rc.A. `internal/compatgen` lint: no `=~`, `readonly` or `declare -r` at top level of any generated file or emitted init script; an allowlist of the `__GASH_*` names the shims may define. |
| Proving test | `TestLedger_SHELL10` in `internal/compatgen` (lint over the generated files and an emitted init script); `testdata/script/load_hygiene.txtar` (`GASH_UMASK=027`, `GASH_NVM=lazy` fixture: `BASH_REMATCH` unchanged, `declare -pr` shows no Gash name) |
| Status | accept: done 39b9fc9 |

<a id="shell-13"></a>
#### SHELL-13 · Headless load prints a plain-text 1.x notice and writes a state file

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N20) |
| Area | Contract |
| Bash location | `gash.sh:150` (autostart outside `__gash_source`, so stderr is not silenced); `lib/modules/migrate.sh:56-71` (`__gash_migrate_autostart`, notice at :69) |
| Observed behavior | With a 1.x `~/.gashrc` (it contains `GASH_BASH`) and no schema file, `GASH_HEADLESS=1; source gash.sh` prints on stderr `gash: your configuration is from Gash 1.x; open a terminal and run "gash migrate" to review the changes in 2.0` (not JSON) and writes `${XDG_STATE_HOME:-~/.local/state}/gash/notice`; it repeats after 24 h. README.md:116-117 says nothing is printed while loading; hygiene_spec.sh:120 does not cover the case; AGENTS.md:31 documents the exception. |
| Reproduction | `H=$(mktemp -d); printf 'export GASH_BASH=$HOME/.gash/gash.sh\n' >$H/.gashrc; env -i HOME=$H PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $C/gash.sh' </dev/null` → the line on stderr, `$H/.local/state/gash/notice` created |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Headless sourcing prints nothing and writes nothing (PRD MUST NOT 4: nothing printed while sourcing in headless mode); the generated `gash.sh` does not run the autostart at all when `GASH_HEADLESS=1`. Interactive shells keep the 2.x behavior: with a terminal `gash_migrate --auto`, without one the once-a-day notice and the `notice` state file. |
| Fix recipe (3.0) | Same as rc.A. `internal/compatgen` (gash.sh template): the headless branch returns before any autostart; `internal/shellinit` keeps the interactive autostart. AGENTS.md:31: drop the exception ("sourcing in headless mode never prints"). |
| Proving test | `TestLedger_SHELL13` in `internal/compatgen`; `testdata/script/headless_1x_gashrc.txtar` (1.x `~/.gashrc`, headless source: empty stderr, status 0, no `~/.local/state/gash` created; interactive without a terminal: the 2.x line once, then silence within 24 h) |
| Status | accept: done adb2213 |

### self-management

<a id="self-11"></a>
#### SELF-11 · Card and help lists sort by the user's locale

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N16, sorting part) |
| Area | Portability |
| Bash location | `lib/modules/gash.sh:739-746` (`__gash_card_functions ... \| sort`); `lib/core/help.sh:269,337,358` (`sort` in `--search` and `--list`); `help.sh:266-276` (`${keyword,,}` case folding) |
| Observed behavior | `gash`, `gash TOPIC`, `gash_help`, `gash_help --list` and `--search` sort function names with `sort` under the caller's `LC_ALL`/`LC_COLLATE`; in collations that ignore `_` at the first level (en_US.UTF-8 and most glibc locales) the order of names differing in underscores differs from byte order (inferred: only C and C.UTF-8 exist on the test VM, where both are byte order). `tools/gen-docs.sh:18` forces `LC_ALL=C`, so the published reference and an interactive card may disagree. `--search` lower-cases with `${,,}`, which folds non-ASCII letters only in UTF-8 locales. |
| Reproduction | `env -i HOME=$(mktemp -d) PATH=/usr/bin:/bin LC_ALL=en_US.UTF-8 bash -c 'GASH_HEADLESS=1; source $C/gash.sh; gash_help --list'` against the same with `LC_ALL=C` (needs the en_US.UTF-8 locale installed) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Names sort by bytes, as `LC_ALL=C` (the order `tools/gen-docs.sh` already publishes); `--search` folds case with Unicode simple lower-casing (`strings.ToLower`), equal to Bash's result in UTF-8 locales. Locale-dependent collation is a property of the host's libc and cannot be reproduced deterministically. |
| Fix recipe (3.0) | Same as rc.A: `internal/registry` sorts with `slices.Sort` on the names; `internal/ui` help search uses `strings.ToLower` on both sides. Docs: none. |
| Proving test | `TestLedger_SELF11` in `internal/registry` (names `a_b`, `ab`, `a_c` sort as `a_b a_c ab`); `testdata/script/help_list_order.txtar` (`LC_ALL=en_US.UTF-8` and `LC_ALL=C` give identical `gash_help --list`) |
| Status | accept: done (rc.A); TestLedger_SELF11 |

<a id="self-16"></a>
#### SELF-16 · gash_uninstall leaks the global variable profile_file into the caller's shell

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (this fragment; not in the dossier) |
| Area | Contract |
| Bash location | `lib/modules/gash.sh:262,318` (`for profile_file in ...` without `local`) |
| Observed behavior | After a confirmed uninstall the caller's shell holds `profile_file="$HOME/.profile"` (the last element of the loop). |
| Reproduction | fake install dir inside a temporary HOME, headless load, `needs_confirm_prompt() { return 0; }; GASH_DIR=$HOME/fake gash_uninstall; declare -p profile_file` → `declare -- profile_file=".../.profile"` (only the fake dir is removed) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | `gash_uninstall` runs in the binary; the stub cannot and does not set `profile_file` in the caller. A child process cannot leak variables into the parent shell. |
| Fix recipe (3.0) | Same as rc.A; nothing to change in Go. |
| Proving test | `TestLedger_SELF16`: `testdata/script/uninstall_no_globals.txtar` (`declare -p` diff around a confirmed uninstall of a fake install shows no new variable) |
| Status | accept: done (rc.A); TestLedger_SELF16.txtar |

<a id="self-22"></a>
#### SELF-22 · 3.0 drops the 1.x→2.x migration (D-6)

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | Operator decision D-6 ("lascia stare", 2026-10-18): drop the 1.x→3.x compatibility work |
| Area | Self-management |
| Bash location | `lib/modules/migrate.sh` (`__gash_migrate_*`: detection, the migration screen, the backups, `--yes`, `--rollback`, `--status`) |
| Observed behavior | 2.x `gash_migrate`/`gash migrate` detects a 1.x `~/.gashrc`, runs an interactive migration screen whose choices land in `~/.gash_env`, backs up the old files under `~/.local/state/gash/backups/`, and offers `--yes`, `--rollback` and `--status` (migrate_spec.sh:45/64/89/106/131). |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | D-6: 3.x does not port the 1.x→2.x migration. `gash_migrate`/`gash migrate` keeps only its name: it prints one line pointing at the 2.x route (check out v2.1.0, run `gash_migrate` there, then `gash_upgrade`), writes nothing (no `~/.gash_env`, no backups, `~/.gashrc` untouched) and returns 1; `--yes`, `--status` and the screen choices all take that same route. This supersedes the migration-logic rows of `migrate_spec.sh` (detection :45, `--yes`/backup/`--rollback` :64, screen→`~/.gash_env` :89, nothing-written :106, `--status`/option errors :131). SELF-08/SELF-09 are moot under the same decision; the once-a-day headless reminder stays (SHELL-13) and the registered `--help` still carries the 2.x usages (SHELL-18). |
| Fix recipe (3.0) | Same as rc.A: `internal/selfmgmt/migrate.go` is the thin pointer; no migration engine is ported. Reconcile the registered `--help`/reminder wording down to the pointer reality (SHELL-18). |
| Proving test | `testdata/script/migrate_d6_route.txtar` (headless: `gash_migrate`, `gash migrate --yes`, the screen choices, `--auto`, `--status` and an option error all point at the 2.x route, write nothing and return non-zero; `--help` still lists `--rollback`). |
| Status | accept: done 38f0d14 |

### lib/core

<a id="core-01"></a>
#### CORE-01 · needs_confirm_prompt prints Bash's own /dev/tty error when there is no terminal

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (N8) |
| Area | Contract |
| Bash location | `lib/core/utils.sh:77` (`read -r REPLY </dev/tty 2>/dev/null`: the failing `</dev/tty` is processed before `2>/dev/null`) |
| Observed behavior | In an interactive shell without a controlling terminal, `gash_uninstall` prints `bash: /dev/tty: No such device or address` (the shell's name and the errno text) before `Info: Uninstall cancelled.`; every caller of `needs_confirm_prompt` does the same. The refusal itself (status 1) is right. |
| Reproduction | `setsid -w $E bash --noprofile --norc -i -c 'source $C/gash.sh >/dev/null 2>&1; needs_confirm_prompt "Continue?"; echo rc=$?' </dev/null` → `Continue? [y/N] bash: /dev/tty: No such device or address`, `rc=1` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The binary (every interactive 2.x function it runs) prints the question and, when `/dev/tty` cannot be opened, only the 2.x refusal path (for `gash_uninstall`: `Info: Uninstall cancelled.`), never Bash's error text, which a Go program cannot produce. The generated `needs_confirm_prompt` in `lib/core/utils.sh` uses `{ read -r REPLY </dev/tty; } 2>/dev/null` so the Bash helper matches. |
| Fix recipe (3.0) | Same as rc.A: `internal/ui.Confirm` returns false on an open error of `/dev/tty`; compatgen template of `needs_confirm_prompt` as above. |
| Proving test | `TestLedger_CORE01` in `internal/ui` (no tty: false, stderr holds only the question); `testdata/script/confirm_no_tty.txtar` (`setsid`, `needs_confirm_prompt q` and `gash_uninstall`: stderr has no `/dev/tty:` text) |
| Status | accept: done bb63583 |

<a id="core-02"></a>
#### CORE-02 · Compat helpers evaluate numeric arguments as Bash arithmetic

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (this fragment; `compat.md` section 4 asks for it) |
| Area | Security |
| Bash location | `lib/core/validation.sh:223` (`__gash_require_args_min`: `[[ "$count" -lt "$minimum" ]]`); `lib/core/output.sh:207` (`__gash_want_colors`: `-eq`), `:514-515` (`__gash_progress`), `:543` (`__gash_section`: `COLUMNS`), `:783-785` (`__gash_bar`), `:814` (`__gash_wrap` width), `:839` (`__gash_choose` default) |
| Observed behavior | A non-integer operand is evaluated as an arithmetic expression: a plain word reads the variable of that name (unset = 0), and an array subscript with a command substitution runs it. Verified with an empty marker file in a temporary directory for `__gash_require_args_min`, `__gash_bar`, `__gash_want_colors`, `__gash_wrap`, `__gash_section` (through `COLUMNS`) and `__gash_choose`; `__gash_human_size` and `__gash_group_digits` validate their input and run nothing. No Gash code path passes untrusted text to these helpers (callers pass `$#`, validated digits or literals); the exposure is an external harness that sources the core files and passes its own input, or a hostile `COLUMNS`. |
| Reproduction | the reproduction kept outside the repository: `__gash_require_args_min VALUE 1` in headless mode, where VALUE holds an array subscript with a command substitution that creates an empty marker file in a temporary directory; the marker appears. Harmless check: `__gash_require_args_min abc 1` takes `abc` as 0. |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The generated helpers (and their Go counterparts in `internal/ui`) accept an operand only when it matches `^-?[0-9]+$`; any other value is taken as 0, the value Bash gives a word naming an unset variable, and is never evaluated. Integer inputs give byte-identical output; `__gash_require_args_min abc 1` still prints `Error: Expected at least 1 argument(s), got abc.` and returns 1. Executing text from an argument cannot be reproduced (MUST NOT 7). |
| Fix recipe (3.0) | Same as rc.A. compatgen: a shared `__gash_int VAR VALUE` helper (glob test `[[ $v == ?(-)+([0-9]) ]]` with extglob saved and restored, no regex) used by every helper above; `COLUMNS` read through `__gash_tty_width`'s existing digit check in `__gash_section`. |
| Proving test | `TestLedger_CORE02` in `internal/compatgen` (each helper called with a subscript-with-substitution value leaves no marker file; integer vectors give the 2.x output); `testdata/script/compat_helpers_arith.txtar` |
| Status | accept: done 470b4b2 (test; the fix was already in __gash_int) |

<a id="core-05"></a>
#### CORE-05 · gash_db_test behaviors that only the mysql/mariadb/psql clients produced

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (this fragment; architecture.md 9) |
| Area | Contract |
| Bash location | `lib/core/config.sh:540-585` (client lookup `type -P mysql`/`mariadb`/`psql`, `--defaults-extra-file` ini at :561, `PGPASSWORD` child env at :574, output discarded `>/dev/null 2>&1`) |
| Observed behavior | Without a client: `Error: MySQL/MariaDB client not found` / `Error: PostgreSQL client (psql) not found`, rc 1. With a client the result follows the client's rules: option files (`/etc/mysql/*.cnf`, `~/.my.cnf`), `localhost` meaning the Unix socket for mysql, libpq environment and `~/.pgpass` when the password is empty, no explicit connect timeout. The decoded password is written into an ini line, so a `%0A` in it starts a new option line (inferred; the user's own file). Any failure prints only `Error: Connection failed` (client output discarded). |
| Reproduction | `printf 'DB:default=mysql://u:p@db/app\n' >$H/.gash_env; chmod 600 $H/.gash_env` then `gash_db_test` in a headless load with no mysql client on PATH → `Info: Testing connection 'default' (mysql://u@db:3306/app)...`, `Error: MySQL/MariaDB client not found`, rc 1 |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | The test runs `SELECT 1` through the native drivers (`internal/db`, go-sql-driver/mysql and pgx), so the "client not found" lines cannot occur and client option files and the socket meaning of `localhost` do not apply; the password goes to the driver, never into an ini file or a child environment. Kept exactly: the `Info: Testing connection 'NAME' (DRIVER://USER@HOST:PORT/DB)...` line, `OK: Connection successful!` / `Error: Connection failed` / `Error: Unknown driver: X` / `Error: Connection 'NAME' not found in ~/.gash_env`, rc 0/1, default name `default`, default ports 3306/5432. Same consequence as DB-07 and DB-09 for `llm_db_*` (lead dedupes). |
| Fix recipe (3.0) | In `internal/db.TestConnection`: connect timeout `GASH_DB_TIMEOUT` (default 30 s); mysql host `localhost` connects through the first existing of `/run/mysqld/mysqld.sock`, `/var/run/mysqld/mysqld.sock`, `/tmp/mysql.sock`, else TCP; on failure print `Error: Connection failed` then `Hint: <driver error, cleaned>`. Docs: `docs/configuration.md` (Databases) says no client is needed and option files are not read. |
| Proving test | `TestLedger_CORE05` in `internal/db` (fake server: success line, failure line, rc; no process started); `testdata/script/gash_db_test_noclient.txtar` (PATH without mysql/psql, unreachable host: `Connection failed`, never `client not found`) |
| Status | accept: done d16347c (localhost socket via DB-16) |

<a id="core-07"></a>
#### CORE-07 · gash_help --list orders the modules outside its fixed list by Bash's hash order

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (this fragment) |
| Area | Portability |
| Bash location | `lib/core/help.sh:313-324` (fixed `module_order`, then `for module in "${!seen_modules[@]}"` appends the others in associative-array iteration order) |
| Observed behavior | The fixed order is `files system git docker docker-compose ai sysinfo gash ssh llm utils`; the four other modules follow in hash-table order, observed `NODE REMOTE CHROME PHP` with Bash 5.2.21. The order of an associative array's keys is an implementation detail of Bash's hash function and table size (inferred: it can differ on 4.4, 5.0, 5.1, 5.3 or after a module is added). Same list in `gash_help` with no argument. |
| Reproduction | `env -i HOME=$(mktemp -d) PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source $C/gash.sh; gash_help --list' \| grep -E '^  [A-Z]'` → `... LLM UTILS NODE REMOTE CHROME PHP` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The registry lists modules in one fixed order: the 2.x fixed list, then `node remote chrome php` (what Bash 5.2.21, the S1 reference, prints). Bash's hash order cannot be reproduced in general. |
| Fix recipe (3.0) | Same as rc.A: `internal/registry.ModuleOrder` is a constant slice; a test fails when a module is missing from it. |
| Proving test | `TestLedger_CORE07` in `internal/registry` (every module of the registry is in `ModuleOrder`; `gash_help --list` headings equal the golden list) |
| Status | accept: done 91efd3b |

<a id="core-08"></a>
#### CORE-08 · ~/.gash_env lines are trimmed with the locale's notion of whitespace

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (this fragment) |
| Area | Portability |
| Bash location | `lib/core/config.sh:179-180` (`[![:space:]]` trim of every line), `:209` (after a closing quote), `:215-216` (unquoted value: `%%[[:space:]]#*` and trailing trim) |
| Observed behavior | `[[:space:]]` follows `LC_CTYPE`: in a UTF-8 locale it also matches non-ASCII spaces such as U+3000 (glibc `iswspace`), in C/POSIX only ASCII. A line `U+3000 GASH_DB_TIMEOUT=7 U+3000` is accepted under `LANG=C.UTF-8` and rejected (`Line 1 ...: unrecognized entry`, skipped) under `LANG=C`. Settings, `DB:`/`AI:`/`SSH:`/`HOST:` entries alike. |
| Reproduction | `printf '\xe3\x80\x80GASH_DB_TIMEOUT=7\xe3\x80\x80\n' >$H/.gash_env; chmod 600 $H/.gash_env`, then `source $C/lib/core/output.sh; source $C/lib/core/utils.sh; source $C/lib/core/config.sh; __gash_load_env; echo "$__GASH_ENV_SETTINGS"` with `LC_ALL=C.UTF-8` (setting read) and `LC_ALL=C` (warning, nothing read) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | `internal/config` trims and splits on ASCII whitespace only (space, `\t`, `\n`, `\v`, `\f`, `\r`), as Bash does in the C locale; a line padded with non-ASCII spaces is "unrecognized" with the 2.x warning text. The libc's locale tables cannot be reproduced deterministically. |
| Fix recipe (3.0) | Same as rc.A; `docs/configuration.md` (file format): "spaces and tabs around a line are ignored". |
| Proving test | `TestLedger_CORE08` in `internal/config` (U+3000 and U+00A0 padded lines are warned and skipped; ASCII padding accepted) |
| Status | accept: done 91efd3b |

<a id="core-09"></a>
#### CORE-09 · __gash_json_str and __gash_json_arr fail without jq

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (this fragment; same root as LLM-04 and DB-05) |
| Area | Portability |
| Bash location | `lib/core/utils.sh:245-266` (`jq -cn --arg s`, `jq -c .`) |
| Observed behavior | Without jq on PATH both helpers print `.../lib/core/utils.sh: line 246: jq: command not found` (265 for the array) on stderr, output nothing and return 127. With jq, invalid UTF-8 in a value becomes U+FFFD (jq's normalization) and the layout is jq's compact form. |
| Reproduction | `env -i HOME=$T PATH=$NOJQ bash -c 'source $C/lib/core/output.sh; source $C/lib/core/utils.sh; v=$(__gash_json_str x); echo "rc=$? [$v]"'` (`$NOJQ` = a directory with bash, env, cat only) → the jq error, `rc=127 []` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The generated helpers ask the binary's encoder (`gash __compat json str`, `json arr`, values on stdin): the jq layout of the documented path (compact, `\u00xx` lowercase, U+FFFD for invalid UTF-8, as jq produced it), status 0, whether or not jq is installed. Without the binary they fall back to `__gash_json_escape_to` (pure Bash, raw bytes). The missing-jq failure is a result that depends on an optional tool (architecture 7, MUST NOT 8). |
| Fix recipe (3.0) | Same as rc.A; `internal/contract` encoder shared with every JSON emitter. |
| Proving test | `TestLedger_CORE09` in `internal/contract` (golden vectors of jq 1.7 for quotes, controls, invalid UTF-8); `testdata/script/compat_json_nojq.txtar` (PATH without jq: `__gash_json_arr a 'b"c'` prints `["a","b\"c"]`, rc 0) |
| Status | accept: done 91efd3b |

<a id="core-10"></a>
#### CORE-10 · __gash_download's INT trap re-parses the destination path as shell code

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (this fragment) |
| Area | Security |
| Bash location | `lib/core/utils.sh:469` (`trap "...; command rm -f '$errf' '$dest'; ..." INT`, the paths expanded into the trap text) |
| Observed behavior | The trap string embeds `$errf` and `$dest` inside single quotes; a path holding `'` ends the quoting, so a Ctrl-C during the download runs whatever follows (`$errf` derives from `TMPDIR`). Gash's own callers (`node_upgrade`, `chrome_headless_update`) pass `mktemp` paths; the exposure is a harness passing its own path, or a `TMPDIR` with a quote. |
| Reproduction | stub `curl` on PATH that sends `SIGINT` to its parent (`kill -INT $PPID; sleep 1`), then `__gash_download http://x "$T/o'\$(touch \$T/MARK)'"` in a temporary dir → `Aborted.`, rc 130, `$T/MARK` created |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | `__gash_download` is delegated to the binary (`compat.md` 4); Go handles SIGINT in-process, removes the partial file and the error file by path, prints `Aborted.` and returns 130, and no path is ever parsed as code. Re-parsing a path inside a trap cannot be reproduced. |
| Fix recipe (3.0) | Same as rc.A; `internal/ui.Download` with `signal.NotifyContext`, cleanup with `os.Remove`. |
| Proving test | `TestLedger_CORE10` in `internal/ui` (cancelled download with a quote in DEST: file removed, status 130, no marker) |
| Status | accept: done (rc.A); TestLedger_CORE10 |

<a id="core-16"></a>
#### CORE-16 · Character counts in messages follow the locale (bytes in C, characters in UTF-8)

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of `internal/config` and `internal/sqlguard`) |
| Area | Portability |
| Bash location | `lib/modules/llm/db.sh:180` (`${line:0:24}` in `dot_command_blocked` details), `lib/core/config.sh:693` (`${token:0:8}` in `gash_ai_list`) |
| Observed behavior | Substring offsets count characters in a UTF-8 locale and bytes in C, so a multibyte character near the cut gives different text, and in C it can be cut in half (invalid UTF-8 in a JSON envelope). |
| Reproduction | a dot-command line with a multibyte character before byte 24 under `LC_ALL=C` and `LC_ALL=C.UTF-8` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Characters, as 2.x in a UTF-8 locale: a character is never cut. |
| Fix recipe (3.0) | Same as rc.A. |
| Proving test | `TestLedger_CORE16` in `internal/config` and `internal/sqlguard`. |
| Status | accept: done (rc.A); existing tests |

<a id="core-17"></a>
#### CORE-17 · The mode in the ~/.gash_env permission warning depends on the installed stat

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of `internal/config` and `internal/sqlguard`) |
| Area | Portability |
| Bash location | `lib/core/config.sh:127-133` (GNU `stat -c %a`, then BSD `stat -f %Lp`) |
| Observed behavior | With GNU stat the mode includes the setuid, setgid and sticky digit (`4600`); BSD `%Lp` prints only the permission bits (`600`), so the warning text depends on which stat is on PATH. |
| Reproduction | a `~/.gash_env` with mode 4644, warning text with GNU stat and with BSD stat |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The mode is read with `os.Lstat`/`Stat` and printed in the GNU `%a` form on every platform. |
| Fix recipe (3.0) | Same as rc.A. |
| Proving test | `TestLedger_CORE17` in `internal/config`. |
| Status | accept: done (rc.A); existing tests |

<a id="core-18"></a>
#### CORE-18 · Without the startup load, settings warnings come at the first call, before its argument checks

| Field | Value |
|---|---|
| Section | Forced divergences |
| Origin | New finding (rc.A port of llm_exec, `llm_exec_security_spec.sh` "reads ~/.gash_env once per call") |
| Area | Contract |
| Bash location | `lib/core/config.sh:147-277` (`__gash_load_env`, cached for the life of the shell) and its callers in the function bodies (`llm/exec.sh:1128`, `llm/db.sh:239`, `remote.sh:71,298`, `ssh.sh:198`, `ai/chat.sh`, `ai/sysinfo.sh`, the `gash_db_*`/`gash_ai_*` helpers of `config.sh`) |
| Observed behavior | gash.sh loads the settings file at startup and prints its warnings then. A shell that sources only the module files (the compat paths) has no startup load: the first function that needs settings loads the file when its body reaches `__gash_load_env`, after its help, argument and validation checks, and prints the warnings once for the life of the shell. |
| Reproduction | a `~/.gash_env` with a malformed line, the modules sourced without gash.sh, then two llm_exec calls: one "Line 1" warning on stderr |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The cache lives in the caller's shell, which the binary cannot fill, and printing from the binary would repeat the warnings on every call. So the generated stub of every function whose 2.x body loads the settings runs the shim's `__gash_load_env` (the 2.x helper, cached, warnings printed once per shell) before it calls the binary, and the binary reads the file silently. Same warnings, same count, same shell; in a modules-only, non-headless shell they now also appear when that first call only prints help or is refused, where 2.x printed nothing yet. Shells that load gash.sh interactively are unchanged. A headless gash.sh load fills no cache at source time (no binary start while sourcing), so it follows the 2.x headless startup load instead: without `GASH_DEBUG=1` it marks the shell (`__GASH_ENV_QUIET`) and the shim's `__gash_load_env` later parses quietly (stderr of `env load` discarded; the binary's loader skips the permission check), and with `GASH_DEBUG=1` it runs `__gash_load_env` at source time with its warnings, as 2.x did. A modules-only shell with `GASH_HEADLESS=1` stays loud, as in 2.x, where only gash.sh's startup load was quiet. |
| Fix recipe (3.0) | Same as rc.A. |
| Proving test | `llm_exec_security_spec.sh` "reads ~/.gash_env once per call" in Go mode (both halves), plus `TestLedger_CORE18` in `internal/compatgen` (the stub of each such function calls `__gash_load_env` first, and no other stub does; headless gash.sh: no warning without `GASH_DEBUG=1`, the 2.x warnings at source time with it). |
| Status | accept: done (rc.A); existing tests |

## Decisions for the operator

| Id | Title | Area | Severity | Contract |
|---|---|---|---|---|
| [LLM-17](#llm-17) | nonexistent PATH and invalid regex are silent successes in llm_grep / llm_find | Contract | low | yes |
| [LLM-18](#llm-18) | an empty first positional is skipped and the next word becomes PATTERN | Contract | low | yes |
| [LLM-25](#llm-25) | ~user is textual, not user expansion | Contract | low | yes |
| [LLM-29](#llm-29) | scanners exclude paths containing a tab or newline | Contract | low | yes |
| [DB-02](#db-02) | MySQL returns every value as a string and NULL as the string "NULL" | Contract | low | yes |
| [DB-10](#db-10) | llm_db_schema key sets differ per engine | Contract | low | yes |
| [DB-11](#db-11) | -h always means help on llm_db_*, never a host option | Contract | low | no |
| [DB-12](#db-12) | the pre-connection SQL precheck returns the SQLite verdict when all engines reject | Contract | low | yes |
| [EXEC-03](#exec-03) | the llm_exec blocklist matches text inside quotes | Security | low | no |
| [EXEC-04](#exec-04) | pattern operands are checked as file names | Security | low | no |
| [EXEC-05](#exec-05) | GASH_LLM_EXEC_ALLOW commands bypass the argument and walk rules | Security | low | no |
| [SYS-43](#sys-43) | hgrep -r -n N returns the N oldest matches, not the N most recent | Contract | low | yes |
| [SYS-44](#sys-44) | SSH passphrases lose trailing whitespace | Contract | low | yes |
| [SYS-45](#sys-45) | wsl_compact drops caches and compacts memory as root with no question and no headless gate | Contract | medium | yes |
| [SYS-46](#sys-46) | wsl_compact's constant `sudo sh -c` script versus the runner's no-shell rule | Contract | low | no |
| [DOCK-22](#dock-22) | docker_stop_all and docker_start_all act on every container without a question, also headless | Contract | medium | yes |
| [GIT-10](#git-10) | git module functions run with the user's git config, hooks and signing | Security | low | yes |
| [DEV-17](#dev-17) | The legacy hte aliases are dropped (sudo ran a user-writable phar as root) | Security | medium | yes |
| [DEV-18](#dev-18) | A missing or unusable project_dir never errors | Contract | low | yes |
| [DEV-19](#dev-19) | Exact Node pins are reduced to their major | Contract | low | yes |
| [DEV-20](#dev-20) | Flag meanings and check order differ between the node functions | Contract | low | yes |
| [AI-23](#ai-23) | GASH_AI_CONFIRM is case-sensitive and every value but 0 and auto means "always ask" | Contract | low | yes |
| [AI-24](#ai-24) | `-h`/`--help` is help only as the first argument of the AI functions | Contract | low | yes |
| [FS-01](#fs-01) | Scanners leave out paths containing a tab or a newline (PRD 4.3) | Contract | medium | yes |
| [FILE-22](#file-22) | Hostile archive members: protection depends on the installed tool version | Security | high | yes |
| [FILE-23](#file-23) | Tool exit statuses collapse to 0/1, and tool warnings are failures for some formats only | Contract | low | yes |
| [FILE-24](#file-24) | Tool configuration in the environment and in `~/.rarrc` changes archive_extract results | Portability | low | no |
| [FILE-25](#file-25) | Backup stamp: Go's clock does not read POSIX TZ strings the way `date` does | Portability | low | no |
| [SHELL-05](#shell-05) | ~/.bash_aliases is sourced twice on distributions whose ~/.bashrc already sources it | Contract | low | yes |
| [SELF-17](#self-17) | gash_doctor checks the 2.x file list, which the 3.x tree no longer ships | Self-management | medium | yes |
| [TEST-08](#test-08) | `tests/mocks/bin/sudo` executes its arguments | Test harness | low | no |
| [SHELL-15](#shell-15) | Reconcile SHELL-06: the grep-alias probe lives in shellinit, not the registry pack | Docs | low | no |

### llm_* agent functions

<a id="llm-17"></a>
#### LLM-17 · nonexistent PATH and invalid regex are silent successes in llm_grep / llm_find

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (N10) |
| Area | Contract |
| Bash location | `lib/modules/llm/fs.sh:328,332,341-347,443,464,466` |
| Observed behavior | A nonexistent PATH, zero matches and an invalid regex all return exit 0 with no output (rg/grep errors are swallowed with `2>/dev/null … |  | true`), so the agent cannot tell "no matches" from "bad path" or "bad pattern". |
| Reproduction | `llm_grep x /nonexistent; echo $?` -> 0; `llm_grep '(' .` -> 0 (temp dir) |
| Severity | low |
| Public-contract impact | yes (if changed) |
| rc.A behavior | Missing path: kept, exit 0 with no output, matching 2.x. Invalid regex: no longer silent — see [LLM-37](#llm-37). Under the one-engine decision a pattern that cannot be read as ripgrep reads it fails with `invalid_regex` (RETRY), so only the missing-path half of this silent-success is kept. |
| Fix recipe (3.0) | Missing root: if the operator wants a distinction, emit `path_not_found`/RETRY; otherwise document "empty results and a missing path are both exit 0, no output". The regex half is settled by LLM-37 (`invalid_regex`). |
| Proving test | `TestLedger_LLM17` pins the kept behavior for `/nonexistent` (missing path only); the invalid-pattern case is covered by `TestLedger_LLM37`. |
| Status | accept: emit path_not_found (RETRY) for a missing root (operator 2026-10-24); done e250421 |

<a id="llm-18"></a>
#### LLM-18 · an empty first positional is skipped and the next word becomes PATTERN

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (N11) |
| Area | Contract |
| Bash location | `lib/modules/llm/fs.sh:280-284,400-404` |
| Observed behavior | llm_find and llm_grep store the first positional into `pattern` only while `pattern` is empty, so an empty first argument is skipped and the next positional becomes the pattern: `llm_find '' '*.js'` searches `*.js`, `llm_grep '' dir` searches for `dir`. |
| Reproduction | `llm_find '' '*.js'` lists js files instead of erroring (temp dir) |
| Severity | low |
| Public-contract impact | yes (if changed) |
| rc.A behavior | Kept: the argument grammar skips an empty first positional exactly as 2.x does (this is the documented grammar quirk). |
| Fix recipe (3.0) | If the operator wants strictness: track "pattern given" rather than "pattern non-empty" and emit `missing_pattern` for an empty pattern; otherwise document the skip. |
| Proving test | `TestLedger_LLM18` pins the chosen behavior for `llm_find '' x`. |
| Status | accept: strict: missing_pattern for an empty pattern (operator 2026-10-24); done e250421 |

<a id="llm-25"></a>
#### LLM-25 · ~user is textual, not user expansion

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (N17) |
| Area | Contract |
| Bash location | `lib/modules/llm/core.sh:140` |
| Observed behavior | `__llm_validate_path` replaces a leading `~` with `$HOME` textually (`${path/#\~/$HOME}`), so `~root/x` becomes `$HOME` + `root/x`, not root's home; `~user` is never real user expansion. |
| Reproduction | `llm_config '~root/x'` -> file_not_found with details `$HOME/root/x` (temp HOME) |
| Severity | low |
| Public-contract impact | yes (if changed) |
| rc.A behavior | Kept: rc.A replaces a leading `~` with `$HOME` textually and does not expand `~user`, matching 2.x. |
| Fix recipe (3.0) | If the operator wants correctness: expand only `~` and `~/…` to `$HOME`, and either resolve `~user` via the passwd database or refuse it; otherwise document the textual rule. |
| Proving test | `TestLedger_LLM25` pins the `~root/x` -> `$HOME/root/x` resolution. |
| Status | accept: expand ~ and ~/ only; refuse ~user (operator 2026-10-24); done 39a5990, 517f190 |

<a id="llm-29"></a>
#### LLM-29 · scanners exclude paths containing a tab or newline

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (PRD 4.3 / N1, N12) |
| Area | Contract |
| Bash location | `lib/modules/llm/fs.sh:123-141` (llm_tree JSON), `213` (llm_find name search) |
| Observed behavior | The scanners exclude any entry whose path contains a tab or newline (`! -path '*\t*'`/`'*\n*'`), so files with those bytes in a component never appear in llm_tree JSON or llm_find name output; llm_tree --text instead splits a newline name into bogus lines. |
| Reproduction | a file named with an embedded newline is absent from `llm_tree .` JSON (temp dir) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Kept: the native scanner excludes tab/newline paths in JSON and name-search modes, matching 2.x (PRD 4.3). |
| Fix recipe (3.0) | Operator decision: keep the exclusion (documented), or carry such names as JSON strings (the native encoder can escape them) and drop the exclusion. If kept, apply it by entry name so the root is not caught (see LLM-08). |
| Proving test | `TestLedger_LLM29` pins whichever rule the operator chooses for a tab/newline file name. |
| Status | accept: keep the exclusion, applied by entry name (as FS-01) (operator 2026-10-24); done 80338c9 |

### databases

<a id="db-02"></a>
#### DB-02 · MySQL returns every value as a string and NULL as the string "NULL"

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | Seed: "MySQL NULL -> \"NULL\"" (CONFIRMED, part of seed 4) |
| Area | Contract |
| Bash location | `lib/modules/llm/db.sh:360-389` (mysql tsv); `core.sh:624-694`; `AGENTS.md:3.3` |
| Observed behavior | `mysql --batch` prints every value as text and a SQL NULL as the literal token `NULL`, so `__llm_tsv_to_json mysql` renders all MySQL values as JSON strings and a NULL as the string `"NULL"`, indistinguishable from a real `"NULL"`. AGENTS.md section 3.3 documents the all-strings behavior. |
| Reproduction | `printf 'a\tb\nNULL\t1\n' | __llm_tsv_to_json mysql` -> `[{"a":"NULL","b":"1"}]` (temp dir) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Kept: the native go-sql-driver/mysql path stringifies all values and renders NULL as the string `"NULL"` to preserve the documented contract (the driver returns typed values and real null, which rc.A deliberately coerces). Each value is the text the mysql client printed: integers in decimal, `YEAR` zero-padded to four digits, `FLOAT` and `DOUBLE` rebuilt from the parsed value with the server's own formatting (shortest digits, `1e20` exponent form), `DECIMAL`, strings and dates as sent. go-sql-driver/mysql parses numeric columns even on the text protocol (since v1.8.0), so the server's bytes are not available; `ZEROFILL` padding is the one loss (DB-18). |
| Fix recipe (3.0) | Operator decision: keep all-strings + NULL->"NULL" (matches AGENTS.md section 3.3), or switch MySQL to typed values and real JSON `null` (a documented contract change). |
| Proving test | `TestLedger_DB02` pins MySQL values as strings and NULL as the chosen representation. |
| Status | accept: keep all-strings and NULL as "NULL" (operator 2026-10-24); done 6682f4b (kept, documented) |

<a id="db-10"></a>
#### DB-10 · llm_db_schema key sets differ per engine

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | Seed: "per-engine llm_db_schema keys differ" (CONFIRMED) |
| Area | Contract |
| Bash location | `lib/modules/llm/db.sh:640-662` |
| Observed behavior | llm_db_schema returns different key sets per engine: sqlite `cid,name,type,notnull,dflt_value,pk`; mysql/mariadb `name,type,nullable,key,default`; pgsql `column_name,data_type,is_nullable,column_default`. They are not unified. `envelope_schema_spec.sh:85` pins the sqlite `name` keys. |
| Reproduction | `llm_db_schema <table> -f db.sqlite` vs a pgsql connection show different keys (temp HOME) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Kept: rc.A emits each engine's key set exactly as 2.x does. |
| Fix recipe (3.0) | Operator decision: keep per-engine keys (matches the pinned spec) or define one unified schema shape across engines (a contract change). |
| Proving test | `TestLedger_DB10` pins each engine's key set. |
| Status | accept: keep per-engine key sets (operator 2026-10-24); done 6682f4b (kept, documented) |

<a id="db-11"></a>
#### DB-11 · -h always means help on llm_db_*, never a host option

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | Seed: "-h means help on llm_db_*, never host" (CONFIRMED) |
| Area | Contract |
| Bash location | `lib/modules/llm/core.sh:403-413` (`__llm_needs_help` intercepts before parsing) |
| Observed behavior | `-h`/`--help` is intercepted before argument parsing, and there is no host option (the host comes only from the connection URL), so `llm_db_query "SELECT 1" -h` prints help; an operator expecting `-h HOST` gets help. |
| Reproduction | `llm_db_query "SELECT 1" -h` prints help (temp HOME) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Kept: the db grammar intercepts `-h`/`--help` as help before parsing. |
| Fix recipe (3.0) | Operator decision: keep (document "-h is help; host comes from the connection URL") — recommended for consistency with the rest of the family. |
| Proving test | `TestLedger_DB11` pins `-h` as help for every llm_db_* function. |
| Status | accept: keep; document that the host comes from the URL (operator 2026-10-24); done 6682f4b (documented) |

<a id="db-12"></a>
#### DB-12 · the pre-connection SQL precheck returns the SQLite verdict when all engines reject

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (NF5) |
| Area | Contract |
| Bash location | `lib/modules/llm/db.sh:220-225` |
| Observed behavior | `__llm_sql_precheck` tries mysql, then pgsql, then sqlite before the connection is resolved; if all reject, it emits the SQLite verdict's error, which can be misleading for a MySQL/pgsql target (e.g. a backtick query gets sqlite's message). The authoritative re-check with the real driver fixes the final behavior for accepted queries. |
| Reproduction | a query no engine accepts shows the sqlite guard's error before any connection (temp HOME) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Kept: rc.A reproduces the precheck order and emits the sqlite verdict pre-connection to match 2.x error text/codes. |
| Fix recipe (3.0) | Operator decision: keep the sqlite-verdict precheck, or defer guard errors until the driver is known so the message matches the target engine. |
| Proving test | `TestLedger_DB12` pins the pre-connection verdict for a query all engines reject. |
| Status | accept: defer guard errors until the engine is known (operator 2026-10-24); done 4d21014 |

### llm_exec

<a id="exec-03"></a>
#### EXEC-03 · the llm_exec blocklist matches text inside quotes

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | Seed: "llm_exec blocklist matches quoted text" (CONFIRMED) |
| Area | Security |
| Bash location | `lib/modules/llm/core.sh:306-326,313-315` |
| Observed behavior | The blocklist is a regex over the whitespace-collapsed whole line regardless of quoting, so harmless lines that merely mention a blocklisted token are refused (e.g. echoing a sentence that contains a shutdown-type word, or a grep whose pattern contains a secret-env token name). |
| Reproduction | `llm_exec "echo 'please shutdown now'"` -> `dangerous_command_blocked` (temp HOME) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Kept: rc.A runs the same regexes (ported as Go regexps over the space-normalized line, including the cosmetically dead patterns) against the whole line, quotes included. |
| Fix recipe (3.0) | Operator decision: keep the quote-oblivious tripwire (documented in AGENTS.md section 6.3 as "a regex tripwire, easy to evade") or make it structural. If kept, document that quoted text is matched. |
| Proving test | `TestLedger_EXEC03` pins that a quoted blocklist token is refused. |
| Status | accept: keep the quote-oblivious tripwire; document it (operator 2026-10-24); done 5c38905 (kept, documented) |

<a id="exec-04"></a>
#### EXEC-04 · pattern operands are checked as file names

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | Seed: "pattern operands are checked as file names" (CONFIRMED) |
| Area | Security |
| Bash location | `lib/modules/llm/exec.sh:649,654` |
| Observed behavior | The whole-word secret check at exec.sh:649 runs before the pattern-operand skip at exec.sh:654, so a grep/rg/sed/awk/jq/yq pattern that looks like a secret file name is refused even though it is a pattern, not a file (while a pattern that looks like a protected path is allowed, because the path check is skipped for patterns). |
| Reproduction | `llm_exec "grep .env notes.txt"` -> `secret_file_blocked` (temp dir) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Kept: rc.A runs the secret-file check on the first positional before skipping it as a pattern, matching 2.x. |
| Fix recipe (3.0) | Operator decision: keep (a pattern resembling a secret name is refused) or move the secret check after the pattern-skip so only real file operands are checked. Document the chosen rule. |
| Proving test | `TestLedger_EXEC04` pins the behavior for a secret-looking pattern vs a path-looking pattern. |
| Status | accept: keep (a pattern resembling a secret name is refused) (operator 2026-10-24); done (rc.A); documented 5c38905 |

<a id="exec-05"></a>
#### EXEC-05 · GASH_LLM_EXEC_ALLOW commands bypass the argument and walk rules

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (NF4) |
| Area | Security |
| Bash location | `lib/modules/llm/exec.sh:605-612`; `docs/security.md:85-87` |
| Observed behavior | A command named in `GASH_LLM_EXEC_ALLOW` skips both `__llm_exec_check_args` and `__llm_exec_check_walk`; only the per-word secret/path checks remain, so an allow-listed walker loses the symlink-follow and walk-root protections. This is documented ("any arguments, without per-command rules") but is a sharp edge. |
| Reproduction | n/a (configuration behavior; file-only setting) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Kept: rc.A reproduces the full bypass for allow-listed commands (per-word secret/path checks still apply). |
| Fix recipe (3.0) | Operator decision: keep the documented bypass, or keep the walk-root/symlink checks even for allow-listed commands. Document whichever is chosen. |
| Proving test | `TestLedger_EXEC05` pins that an allow-listed command skips arg/walk rules but not the secret/path checks. |
| Status | accept: keep the walk-root/symlink checks for allow-listed commands too (operator 2026-10-24); done 5c38905 |

### system, sysinfo, ssh, chrome

<a id="sys-43"></a>
#### SYS-43 · hgrep -r -n N returns the N oldest matches, not the N most recent

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (N9 b) |
| Area | Contract |
| Bash location | `lib/modules/system.sh:302,328-329` (descending sort, then `tail -n N`) |
| Observed behavior | `-n N` always keeps the last N lines of the sorted list: newest last by default, so `-n 1` is the most recent; with `-r` (newest first) `tail` keeps the N oldest, and `hgrep x -r -n 1` shows the oldest match. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; printf "#1700000000\necho one\n#1700000100\necho two\n" >"$HOME/h"; HISTFILE="$HOME/h" hgrep echo -r -n 1'` (`echo one`) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Keeps 2.x: limit applied after the sort, as `tail -n N`. |
| Fix recipe (3.0) | Decision: (a) keep, documenting "with -r, -n keeps the oldest N"; or (b, recommended) `-n N` always means the N most recent: `internal/system` `hgrep` takes the newest N in ascending order, then reverses for `-r`. Help NOTES state the rule. |
| Proving test | `TestLedger_SYS43` in `internal/system` (two entries, `-r -n 1` → `echo two` under option b) |
| Status | accept: option (b): -n N is always the N most recent (operator 2026-10-24); done 105f468 |

<a id="sys-44"></a>
#### SYS-44 · SSH passphrases lose trailing whitespace

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (N31; owner of the parser: config loader) |
| Area | Contract |
| Bash location | `lib/core/config.sh:179-180` (whole line trimmed before `SSH:` is split) |
| Observed behavior | `SSH:~/.ssh/key2=trail   ` stores the passphrase `trail`: a passphrase that really ends in spaces can never be used, while an accidental trailing space in the file is forgiven. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'mkdir -p ~/.ssh; : >~/.ssh/key2; printf "SSH:~/.ssh/key2=trail   \n" >~/.gash_env; chmod 600 ~/.gash_env; GASH_HEADLESS=1; source "$G/gash.sh"; __gash_load_env; __gash_get_ssh_keys \| od -c \| tail -2'` (value ends at `l`) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Keeps 2.x: `internal/config` trims the whole line (the config-spec vectors stay as they are). |
| Fix recipe (3.0) | Decision: (a) keep trimming and document it in `docs/configuration.md` ("trailing spaces of a passphrase are dropped"); or (b) trim only the left side and the key part, keep the value verbatim minus a final `\r`, with a `Warning: Line N of FILE: passphrase ends with whitespace` so accidental spaces are visible. |
| Proving test | `TestLedger_SYS44` in `internal/config` (value `trail   ` round-trips under option b; warning printed once) |
| Status | accept: option (b): keep the value verbatim minus a final \r, warn on trailing whitespace (operator 2026-10-24); done 652aa51 |

<a id="sys-45"></a>
#### SYS-45 · wsl_compact drops caches and compacts memory as root with no question and no headless gate

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (N36) |
| Area | Contract |
| Bash location | `lib/modules/system.sh:1034-1038` (unprompted `sudo sh -c ...`), `lib/modules/system.sh:1022-1023` (`dmesg` failures counted as 0) |
| Observed behavior | Without `--status` the function runs sudo right away, also with `GASH_HEADLESS=1` (unlike every other root or destructive function of the module). When dmesg is unreadable (`kernel.dmesg_restrict=1`, non-root) the line says `0` failures. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; __gash_require_wsl() { return 0; }; dmesg() { :; }; sudo() { printf "sudo %s\n" "$*" >&2; }; wsl_compact'` (the sudo line appears with no question) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Keeps 2.x: no question, no headless gate, `0` when dmesg fails. |
| Fix recipe (3.0) | Decision: (a) keep (it is the repair and is harmless to data); or (b, recommended) keep it unprompted on a terminal, but headless without `-y` prints `Error: wsl_compact needs root: pass -y to run it unattended.` rc 1 (new `-y`/`--yes` flag), and print `unknown (dmesg not readable)` instead of `0` when dmesg exits non-zero. |
| Proving test | `TestLedger_SYS45` in `internal/system` (WSL sysroot, headless, no `-y` → rc 1 and no sudo call; failing dmesg → `unknown`) |
| Status | accept: option (b): headless needs -y; dmesg failure prints unknown (operator 2026-10-24); done 7607509 |

<a id="sys-46"></a>
#### SYS-46 · wsl_compact's constant `sudo sh -c` script versus the runner's no-shell rule

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (new; `architecture.md` 7: no `sh -c` except `llm_exec` and the nvm bridge) |
| Area | Contract |
| Bash location | `lib/modules/system.sh:1035` (`sudo sh -c 'sync && echo 3 > /proc/sys/vm/drop_caches && echo 1 > /proc/sys/vm/compact_memory'`) |
| Observed behavior | One sudo call runs a constant three-step shell script (no user input in it); a sudo stub or the sudo log sees exactly `sudo sh -c <script>`. |
| Reproduction | The SYS-45 reproduction prints `sudo sh -c sync && echo 3 > /proc/sys/vm/drop_caches && echo 1 > /proc/sys/vm/compact_memory` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Keeps 2.x: the identical `sudo sh -c` argv, which needs a runner exception. |
| Fix recipe (3.0) | Decision: (a, recommended for rc.A parity) `internal/runner` gains `ConstScript`, accepted only for compile-time constant scripts declared in the registry (this one), listed in `architecture.md` 7; or (b) three calls `sudo sync`, `sudo tee /proc/sys/vm/drop_caches` (stdin `3`), `sudo tee /proc/sys/vm/compact_memory` (stdin `1`), stdout discarded, failure message unchanged; render fixture `wsl_compact compact` then sees three sudo lines. |
| Proving test | `TestLedger_SYS46` in `internal/runner` (option a: a non-constant script is refused at build time by a vet check; option b: fake runner sees the three argv) |
| Status | accept: option (a): runner ConstScript for registered constant scripts (operator 2026-10-24); done (rc.A); runner.ConstScript ScriptWSLCompact, TestLedger_SYS46 |

### docker and compose

<a id="dock-22"></a>
#### DOCK-22 · docker_stop_all and docker_start_all act on every container without a question, also headless

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (N15, consent part) |
| Area | Contract |
| Bash location | `lib/modules/docker.sh:19-51` |
| Observed behavior | Both act on every container of the daemon at once: no plan, no `-y`, no headless gate (docker_prune_all and services_stop ask first). Stopping is reversible but interrupts every running service on the host. |
| Reproduction | `gm 'export MOCK_DOCKER_CONTAINERS=1; docker_stop_all; echo rc=$?'` → `Info: Stopping Docker containers...` `rc=0`, no question asked |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | keeps 2.x: no question, no flag. |
| Fix recipe (3.0) | If accepted: print `Will stop N container(s): NAME...` (or `Will start ...`), then `__gash_confirm_plan` (Y/n) with `-y/--yes`; headless or no terminal → `Error: No terminal to ask for confirmation: pass -y to go ahead.` rc 1; rows in docs/security.md "Confirmations"; completion `-y --yes`. |
| Proving test | `TestLedger_DOCK22` in `internal/docker` (headless without `-y` → rc 1 and no `docker stop` call; with `-y` → stop called once with every ID) |
| Status | accept: list the containers and confirm; -y/--yes for headless (operator 2026-10-24); done 09ca07f, 63f58e1 |

### git module

<a id="git-10"></a>
#### GIT-10 · git module functions run with the user's git config, hooks and signing

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (dossier git module preamble) |
| Area | Security |
| Bash location | `lib/modules/git.sh` (every git call); `go-impl/design/architecture.md` section 7 ("The git CLI with the 2.x hardening flags") |
| Observed behavior | git_sync runs pre-commit and commit-msg hooks and signs when `commit.gpgSign` is set; git_add_tag signs with `tag.gpgSign=true` (without a key: `error: unable to sign the tag` → `Error: Could not create tag 'v5'.`, observed); credential helpers, `core.sshCommand` and `url.<base>.insteadOf` apply to pushes, pulls and clones. The design's "2.x hardening flags" belong to `llm_git_*` and `llm_exec`, not to these functions. |
| Reproduction | `g 'cd $T && git init -q w10 && cd w10 && git config user.email a@b && git config user.name n && printf "#!/bin/sh\n: >\"$T/hook-ran\"\n" >.git/hooks/pre-commit && chmod +x .git/hooks/pre-commit && echo a >a && git_sync --no-push; ls $T'` → `OK: Committed ...` and the empty marker `hook-ran` in `$T` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | keeps 2.x: `internal/gitx` runs the git module functions with the caller's environment and configuration, without `-c core.hooksPath=...` or other hardening; only the `llm_*` git calls get the hardening flags. |
| Fix recipe (3.0) | recommended: keep (they are the user's own repository operations); architecture.md section 7 and docs/security.md say that git module functions run the user's hooks and config. If the operator wants hardening, a separate decision with its own flags. |
| Proving test | `TestLedger_GIT10` as `testdata/script/git10_hooks.txtar` (a pre-commit hook creates a marker; `git_sync --no-push` → marker exists) |
| Status | accept: keep; document that git module functions run the user hooks and config (operator 2026-10-24); done 9eed0f7 |

### php and node

<a id="dev-17"></a>
#### DEV-17 · The legacy hte aliases are dropped (sudo ran a user-writable phar as root)

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (php alias pack review, `lib/aliases/php.sh`); lead decision: drop, replaced by `gash hte` (PRD 5.5) |
| Area | Security |
| Bash location | `lib/aliases/php.sh:58-65` |
| Observed behavior | When `~/.config/composer/vendor/bin/hte-cli` exists and a phpX.Y is on PATH, interactive shells get `hte` and `hte-create` (both `... hte-cli create`), `hte-remove` and `hte-details`, each `sudo /usr/bin/phpX.Y -d allow_url_fopen=1 -d memory_limit=2048M $HOME/.config/composer/vendor/bin/hte-cli <cmd>`: a file in the user's writable Composer tree runs as root, so anything able to write the user's home (a compromised `composer global update`, any user-level process) gains root at the next `hte`. The `gash dev` card and dev.md list them even when hte-cli is absent (DEV-21). |
| Reproduction | `d=$(mktemp -d); mkdir -p "$d/.config/composer/vendor/bin"; : >"$d/.config/composer/vendor/bin/hte-cli"; env -i HOME="$d" PATH=/usr/local/bin:/usr/bin:/bin TERM=xterm bash -ic 'source $GASH_SRC/gash.sh >/dev/null 2>&1; alias hte'` → `alias hte='sudo /usr/bin/php8.3 -d allow_url_fopen=1 -d memory_limit=2048M <HOME>/.config/composer/vendor/bin/hte-cli create'` (defines only, runs nothing) |
| Severity | medium |
| Public-contract impact | yes (four aliases disappear) |
| rc.A behavior | Not carried: `gash init bash` defines none of the four and never probes `~/.config/composer/vendor/bin/hte-cli`; `gash hte ...` (design `hte.md`: user-run stack, root-owned hosts helper, `gash hte migrate` for existing hte-cli sites) replaces hte-cli. Typing `hte` in 3.x gives Bash's `command not found`. |
| Fix recipe (3.0) | None beyond rc.A. Docs: `docs/reference/dev.md` drops the four alias rows; `docs/migration.md` (2.x → 3.x) gains "hte, hte-create, hte-remove, hte-details are gone: use `gash hte` (`gash hte migrate` imports hte-cli sites)"; CHANGELOG lists the removal. |
| Proving test | `testdata/script/alias_parity.txtar` (`TestLedger_DEV17_2x`): with a fake hte-cli file, none of the four aliases is defined after `eval "$(gash init bash)"` |
| Status | accept: confirm the removal (gash hte replaces them) (operator 2026-10-24); done (removed in rc.A; docs/reference/dev.md says so) |

<a id="dev-18"></a>
#### DEV-18 · A missing or unusable project_dir never errors

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (N4) |
| Area | Contract |
| Bash location | `lib/modules/php.sh:22-28,43,56`; `lib/modules/node.sh:295-343` |
| Observed behavior | `php_detect_version /does/not/exist` walks up from the literal path and answers 8.4, rc 0; a missing relative DIR skips the walk-up entirely, so the cwd's `.target-php-version` is ignored; a file given as DIR walks up from it. node_detect_version just finds no source files (rc 0). |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/local/bin:/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; php_detect_version /does/not/exist; echo rc=$?'` → `{"version":"8.4",...}` `rc=0` |
| Severity | low |
| Public-contract impact | yes if changed (exit status or stderr) |
| rc.A behavior | Keeps the 2.x behavior (frozen contract: defaults and exit codes). |
| Fix recipe (3.0) | Recommended: keep stdout and rc 0, add `Warning: Directory not found: <DIR>` on stderr (both functions, both modes) when DIR is not a directory. Alternative (contract break): `Error: Directory not found: <DIR>` rc 1. Docs: NOTES of both functions. |
| Proving test | `TestLedger_DEV18_fix` in `internal/dev` (missing DIR: same stdout, the warning on stderr) |
| Status | accept: keep stdout and rc 0, add the stderr warning (operator 2026-10-24); done 37bcf69 |

<a id="dev-19"></a>
#### DEV-19 · Exact Node pins are reduced to their major

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (N9) |
| Area | Contract |
| Bash location | `lib/modules/node.sh:270-273,376-377` |
| Observed behavior | Every source is reduced to a major and `full_version` is the newest installed patch of it: `.node-version v20.9.0` with v20.9.0 and v20.11.1 installed gives `"full_version":"20.11.1"`. The help says only "Priority: ...". |
| Reproduction | `d=$(mktemp -d); mkdir -p "$d/.nvm/versions/node/v20.9.0" "$d/.nvm/versions/node/v20.11.1"; printf 'v20.9.0\n' >"$d/.node-version"; env -i HOME="$d" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; cd "$HOME"; node_detect_version'` |
| Severity | low |
| Public-contract impact | yes if changed |
| rc.A behavior | Keeps the 2.x behavior (the `version` key is the major by contract). |
| Fix recipe (3.0) | Recommended: keep, and document it in the NOTES ("exact pins resolve to the newest installed patch of their major"). Alternative: an installed exact pin becomes `full_version` (value change, no new key). |
| Proving test | `TestLedger_DEV19_2x` in `internal/dev` (pin v20.9.0 → `20.11.1`) |
| Status | accept: keep and document (operator 2026-10-24); done 37bcf69 |

<a id="dev-20"></a>
#### DEV-20 · Flag meanings and check order differ between the node functions

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (dossier node_upgrade / nvm_prune_versions / node_detect_version signatures) |
| Area | Contract |
| Bash location | `lib/modules/node.sh:40-51,117-129,354-371` |
| Observed behavior | `--force` means "skip the question" in node_upgrade (`-y\|--yes\|--force`) and "apply" in nvm_prune_versions (`--force\|-f`), which refuses `-y`. node_upgrade parses its arguments before the nvm check, nvm_prune_versions after it (so `nvm_prune_versions --bogus` without nvm gives the nvm error), and node_detect_version checks the NVM directory first (`node_detect_version --bogus` without `~/.nvm` gives the NVM-directory error, rc 1). |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; node_detect_version --bogus; nvm_prune_versions --bogus'` |
| Severity | low |
| Public-contract impact | yes if changed |
| rc.A behavior | Keeps every 2.x rule and message (the 2.x grammars are ported rule for rule). |
| Fix recipe (3.0) | Recommended: accept `-y`/`--yes` in nvm_prune_versions as synonyms of `--force`; parse arguments before the environment checks in all three (unknown option first, `Error: Unknown option: X ...` rc 1). Docs: usage lines. |
| Proving test | `TestLedger_DEV20_fix` in `internal/cli/args` (grammar table: order of errors, `-y` accepted) |
| Status | accept: as recommended: -y/--yes synonyms, arguments parsed before environment checks (operator 2026-10-24); done dda94a4 |

### ai

<a id="ai-23"></a>
#### AI-23 · GASH_AI_CONFIRM is case-sensitive and every value but 0 and auto means "always ask"

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (N34) |
| Area | Contract |
| Bash location | `lib/modules/ai/client.sh:168-172` |
| Observed behavior | `0` never asks; `auto` (and unset or empty) asks only for kind `data`; anything else (`1`, `off`, `no`, `false`, `AUTO`, `yes`) asks before every request, so headless runs with `GASH_AI_CONFIRM=off` are refused with the "no terminal to confirm on" error. The docs list only `auto`, `1`, `0`. |
| Reproduction | `env -i HOME="$d" PATH="$d/bin:/usr/bin:/bin" GASH_AI_CONFIRM=off bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; ai_query claude what is ls </dev/null'` → `Error: Not sending your question (10 B) to claude: no terminal to confirm on. Set GASH_AI_CONFIRM=0 in ~/.gash_env to allow it` |
| Severity | low |
| Public-contract impact | yes if changed |
| rc.A behavior | Keeps the 2.x rule (fail closed on unknown values). |
| Fix recipe (3.0) | Recommended: keep fail-closed and add `Warning: GASH_AI_CONFIRM=<value> is not 0, 1 or auto: asking before every request` (once per call) for other values; do not alias `off` to `0` (an unattended send must stay explicit). Docs: configuration.md settings table states the rule. |
| Proving test | `TestLedger_AI23_fix` in `internal/ai` (`off` → asks/refuses and warns; `0`, `1`, `auto` unchanged) |
| Status | accept: keep fail-closed, add the warning (operator 2026-10-24); done 2980327 |

<a id="ai-24"></a>
#### AI-24 · `-h`/`--help` is help only as the first argument of the AI functions

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (N35) |
| Area | Contract |
| Bash location | `lib/modules/ai/chat.sh:152-154,265-267`; `lib/modules/ai/sysinfo.sh:649-651`; `lib/core/utils.sh:29-50` |
| Observed behavior | `ai_query claude --help` sends `--help` as the question; `ai_ask claude --help` starts the prompt; `ai_sysinfo claude --help` is `Error: Unknown argument: --help` with the usage line. |
| Reproduction | `env -i HOME="$d" PATH="$d/bin:/usr/bin:/bin" bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; ai_query claude --help </dev/null'` with the AI-02 fake curl → a request is attempted |
| Severity | low |
| Public-contract impact | yes if changed |
| rc.A behavior | Keeps the 2.x rule (needs_help on `$1` only), as the architecture's grammar port requires. |
| Fix recipe (3.0) | Recommended: also treat `-h`/`--help` as help when it is the only word after a provider (`ai_query claude --help`, `ai_ask gemini -h`) and accept it anywhere in ai_sysinfo; questions that merely contain `--help` among other words stay questions. Docs: usage NOTES. |
| Proving test | `TestLedger_AI24_fix` in `internal/cli/args` (vectors above; `ai_query what does ls --help print` stays a question) |
| Status | accept: as recommended: help also as the only word after a provider (operator 2026-10-24); done 8049ea1 |

### file scanners

<a id="fs-01"></a>
#### FS-01 · Scanners leave out paths containing a tab or a newline (PRD 4.3)

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | Seed: "Operator decision: paths with a tab or a newline are left out of the results" (PARTIAL) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:465-468` (files_largest `! -path '*\n*' ! -path '*\t*'`), `681-683`, `950-952` (du `--exclude='*\n*'` only), `1186-1190`, `1382-1384` (find `! -path '*\n*'` only), `1541-1548` (tree_stats empty walk: no test), help NOTES `2040-2043`; AGENTS.md 3.2 |
| Observed behavior | The rule is on record (AGENTS 3.2, files_largest NOTES) and only files_largest applies it in every mode. On the GNU path the other four leave newline paths out but not tab paths: dirs_largest and dirs_list_empty keep them, dirs_find_large cuts them at the first tab, tree_stats counts them, and its empty-dir count also counts newline directories. |
| Reproduction | `d=$(mktemp -d); mkdir "$d/$(printf 't\tx')" "$d/$(printf 'et\tx')"; head -c 5000 /dev/zero >"$d/$(printf 't\tx')/f"; for f in files_largest dirs_largest dirs_list_empty; do $f --json "$d"; done` (temp dir only; full fixture in `work/files-scanners/repro-tabnl.sh`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Decision kept as on record. rc.A reproduces each scanner's GNU-path behavior: files_largest excludes both; the deviations of the others are Defects reproduced: FS-06 (dirs_largest), FS-07 (dirs_list_empty), FS-08 (tree_stats counts), FS-09 (dirs_find_large cut), FS-17 (tree_stats extensions). |
| Fix recipe (3.0) | Once the operator confirms the rule, apply the documented exclusion uniformly: one predicate in `internal/fsscan` (walker): an entry whose path relative to ROOT contains `\t` or `\n` is skipped, never descended into, and its size is added to no total (the GNU du `--exclude` semantics 2.x already has for newlines). Used by all five scanners and by both tree_stats counts. AGENTS 3.2 unchanged; the files_largest NOTES paragraph is added to the help of the other four in 3.0. |
| Proving test | `TestLedger_FS01_fix` in `internal/fsscan`: on the tab/newline fixture (dirs `t\tx`, `n\nx`, empty `et\tx`, `en\nx`, files `ft\ty`, `fn\ny`, `norm/f`) no scanner in any mode (--json, --null, text, dirs_find_large with and without --with-mtime) reports a path containing `\t` or `\n` or the truncated names `t`, `et`, `n`; tree_stats gives files_total 1, dirs_total 2, empty_dirs 0, size_total 1000; `testdata/script/fsscan_tabnl.txtar` runs `jq -e .` on every JSON line. |
| Status | accept: apply the exclusion uniformly (one predicate in internal/fsscan) (operator 2026-10-24); done d6a4bf9 |

### archive_extract and file_backup

<a id="file-22"></a>
#### FILE-22 · Hostile archive members: protection depends on the installed tool version

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (dossier A-12) |
| Area | Security |
| Bash location | `lib/modules/files.sh:1921-1947` (no check of its own) |
| Observed behavior | Gash hands archives to the tools unchecked. On the audit host (GNU tar 1.35, UnZip 6.00 Debian, UNRAR 7.00, 7-Zip 23.01) no marker escaped OUTPUT_DIR: `../M` skipped by tar and rewritten as `out/M` by unzip, unrar and 7z; absolute names stripped; a symlink member pointing outside followed by a member written through it refused by every tool (rc 1), but tar and unzip leave the outside-pointing symlink in `out/`; hard links to `../` refused by tar. These guards are version-specific (GNU tar delayed symlinks, Debian's unzip patches, unrar 7 link checks, 7-Zip's "Dangerous link path"); older tools (p7zip 16.02, unpatched Info-ZIP) behave differently (inferred) |
| Reproduction | `$W/scripts/mkevil.py SANDBOX ARCHDIR` (kept outside the repository) builds tar, zip, rar 4 and 7z archives whose every target is an empty marker inside a `mktemp -d` SANDBOX; `cd RUN && archive_extract X out`; then no `*_MARKER` may exist outside `out/` |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | Keeps 2.x: same tools, same argv, whatever they enforce; outside-pointing symlink members stay where the tool put them |
| Fix recipe (3.0) | Options: (a) keep 2.x, document the tool versions tested; (b) recommended: an in-process pre-check of the member list before the tool runs: tar formats through Go's `archive/tar` on the same decompressed stream (`compress/gzip`, `compress/bzip2`, or the `xz -dc`, `zstd -dc`, `uncompress -c` output), zip through `archive/zip`, 7z through `7z l -slt -ba -- F`, rar through `unrar lt -- F`; refuse when a member name is absolute, has a `..` component, is a symlink or hard link whose target resolves outside OUTPUT_DIR, or lies below a symlink member: `Error: Archive 'F' has an unsafe member 'M' (absolute path, .. component or link outside the destination); nothing extracted.`, rc 1, before OUTPUT_DIR is created (in `internal/files`, `memberGuard()`); the tool then extracts as today; (c) as (b) with an opt-out flag (new grammar for a function that has none; not recommended) |
| Proving test | `TestLedger_FILE22_fix` in `internal/files`: the mkevil corpus (tar, tgz, zip, rar 4 stored, 7z with `-snl` and `-spf`) is refused per archive, the sandbox gains no file; a clean archive still extracts, rc 0 |
| Status | accept: option (b): in-process member pre-check (operator 2026-10-24); done a2a30ac (option b; guard extended to rar symlinks and 7z link targets) |

<a id="file-23"></a>
#### FILE-23 · Tool exit statuses collapse to 0/1, and tool warnings are failures for some formats only

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (dossier D-A7) |
| Area | Contract |
| Bash location | `lib/modules/files.sh:1921-1947` (`\|\| rc=1`) |
| Observed behavior | Every tool status becomes 1. gzip "trailing garbage ignored" (exit 2, data written) → rc 1; unzip exit 1 after stripping `../` or an absolute name (files written) → rc 1; 7z "Cannot open the file as [7z] archive ... open as [zip]" (a zip renamed `.7z`, extracted) → rc 0. The caller cannot tell a warning from a failure |
| Reproduction | `R '{ printf "hi\n" \| gzip; printf junk; } > w.gz; archive_extract w.gz o; echo rc=$?; cat o/w'` → `gzip: w.gz: decompression OK, trailing garbage ignored`, `rc=1`, `hi` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Keeps 2.x (0 or 1) |
| Fix recipe (3.0) | Options: (a) keep 0/1 (recommended: 2.x contract, simplest for scripts); (b) pass each tool's status through (gzip 2, unzip 1, 7z 1/2, tar 2); (c) rc 0 on the known warning statuses (gzip 2, unzip 1) with `Warning: <tool> reported warnings; files were extracted.` on stderr |
| Proving test | `TestLedger_FILE23` in `internal/files` pins the chosen mapping per tool with `runner.Fake` |
| Status | accept: option (a): keep 0/1 (operator 2026-10-24); done (option a, rc.A); TestLedger_FILE23 |

<a id="file-24"></a>
#### FILE-24 · Tool configuration in the environment and in `~/.rarrc` changes archive_extract results

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (bookkeeping check, not in the dossier) |
| Area | Portability |
| Bash location | `lib/modules/files.sh:1921-1947` (the tools inherit the whole environment and HOME) |
| Observed behavior | Checked on the restored tree: `TAR_OPTIONS=--keep-old-files` makes `archive_extract a.tgz o` fail (`tar: src/a.txt: Cannot open: File exists`, rc 1) and keep the old file; `UNZIP=-o` overwrites without the prompt (rc 0); `RAR=-inul` and `switches=-inul` in `~/.rarrc` silence unrar; `GZIP=-v`, `BZIP2=-v`, `XZ_OPT=-v` add stderr lines (gzip also warns that `GZIP` is deprecated). Documented by the tools, not run: `UNZIPOPT`, `BZIP`, `XZ_DEFAULTS` |
| Reproduction | `REXTRA="TAR_OPTIONS=--keep-old-files" R "mkdir -p o/src; printf OLD > o/src/a.txt; cp $W/fx1/a.tgz .; archive_extract a.tgz o >/dev/null; echo rc=\$?; cat o/src/a.txt"` → `tar: src/a.txt: Cannot open: File exists`, `rc=1`, `OLD` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Keeps 2.x: the extraction tools get the caller's environment and HOME unchanged (2.x passed everything through, so the runner's allowlist rule, `architecture.md` 7, keeps the full environment for these tools) |
| Fix recipe (3.0) | Options: (a) keep pass-through and document it in the help NOTES (recommended: the user's tool configuration is respected); (b) drop these variables and run unrar with an empty temporary HOME, for results independent of user configuration (behavior change for users who rely on them) |
| Proving test | `TestLedger_FILE24_2x` in `internal/files` (`runner.Fake` asserts the variables and HOME reach the tool); `testdata/script/archive_extract_tool_env.txtar` (`TAR_OPTIONS=--keep-old-files` → rc 1, old file kept) |
| Status | accept: option (a): keep pass-through and document it (operator 2026-10-24); done ca0b3dd (option a, documented) |

<a id="file-25"></a>
#### FILE-25 · Backup stamp: Go's clock does not read POSIX TZ strings the way `date` does

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (bookkeeping check, not in the dossier) |
| Area | Portability |
| Bash location | `lib/modules/files.sh:1973` (`date +%Y%m%d%H%M%S`) |
| Observed behavior | 2.x takes local time from glibc's `date`, which honors POSIX TZ strings. Go's `time` package loads zone names and files only and falls back to UTC for other values. At one instant (go1.24.7 against coreutils 9.4): `TZ=JST-9` date `20251018182200`, Go `20251018092200`; `TZ=EST5EDT,M3.2.0,M11.1.0` date `20251018052200`, Go `20251018092200`; `TZ=UTC+3` date `20251018062200`, Go `20251018092200`; equal for `Asia/Tokyo`, `:Asia/Tokyo`, `Etc/UTC`, empty, unset and an unknown name (UTC in both) |
| Reproduction | `TZ=JST-9 date -d @1760779320 +%Y%m%d%H%M%S` → `20251018182200`; the Go program in `work/files-archive-ledger/gotz` (prints `time.Unix(1760779320, 0).Format("20060102150405")`) prints `20251018092200` under the same TZ |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Operator choice; the pure-port default is (a) |
| Fix recipe (3.0) | Options: (a) rc.A runs `date +%Y%m%d%H%M%S` from PATH, as 2.x did (exact parity including POSIX TZ strings; a user `date` function stays ignored, FILE-20); (b) Go's clock plus a POSIX TZ rule parser for values that are not zone names; (c) Go's clock as is (a forced divergence for POSIX TZ strings). Recheck with go1.27.2 before choosing (b) or (c) |
| Proving test | `TestLedger_FILE25_2x` in `internal/files`: under `TZ=JST-9` and `TZ=EST5EDT,M3.2.0,M11.1.0` at a fixed instant, the stamp equals `date -d @N +%Y%m%d%H%M%S` |
| Status | accept: option (a): date from PATH (operator 2026-10-24); done (option a, rc.A); TestLedger_FILE25_2x |

### shell layer

<a id="shell-05"></a>
#### SHELL-05 · ~/.bash_aliases is sourced twice on distributions whose ~/.bashrc already sources it

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | Seed: "~/.bash_aliases sourced twice on Ubuntu" (CONFIRMED, documented) |
| Area | Contract |
| Bash location | `gash.sh:238-239` (sources `~/.bash_aliases` then `~/.bash_local`); Ubuntu 24.04 `/etc/skel/.bashrc:104-106`; documented at `docs/configuration.md:220-224` ("in that case it runs twice") |
| Observed behavior | With the skel `~/.bashrc` plus the Gash block, side effects in `~/.bash_aliases` run twice (a `PATH=$PATH:/opt/x` line adds `/opt/x` twice). Gash re-sources it so that the user's aliases win over Gash's, which load later. |
| Reproduction | `cp /etc/skel/.bashrc $H/.bashrc`, append the Gash block sourcing `$C/gash.sh`, `echo 'echo SOURCED >&2' >$H/.bash_aliases`; `$E bash -i -c exit </dev/null 2>&1 \| grep -c SOURCED` → `2` |
| Severity | low |
| Public-contract impact | yes (documented load order) |
| rc.A behavior | Keeps 2.x: the generated `gash.sh` sources both user files after `gash init bash`, so `~/.bash_aliases` runs twice on such systems. |
| Fix recipe (3.0) | Options: (a) keep the documented double sourcing; (b) recommended: once pack aliases no longer replace the user's (SHELL-09 fix), stop re-sourcing `~/.bash_aliases` when the profile already sourced it (detect with a guard variable set by a one-line wrapper the installer adds, or simply source it only when no alias existed before load), keep `~/.bash_local`; update `docs/configuration.md:220-224` and CHANGELOG. |
| Proving test | `testdata/script/user_files_once.txtar` (skel `.bashrc`: side effect runs once and `ll` keeps the user's definition; without the distro line: still sourced once) |
| Status | accept: option (b): source ~/.bash_aliases once (operator 2026-10-24); done b3596d7, d0a6dce |

<a id="shell-15"></a>
#### SHELL-15 · Reconcile SHELL-06: the grep-alias probe lives in shellinit, not the registry pack

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (reconcile of SHELL-06 against rc.A) |
| Area | Docs |
| Bash location | 2.x `lib/aliases/colors.sh:40-46`; rc.A `internal/shellinit/aliases.go:111-112` (the live probe) and the catalog entry `internal/registry/catalog_gen.go:3581` (condition `$__GASH_PROBE_GREP_COLOR`) |
| Observed behavior | SHELL-06's Observed and rc.A lines are correct as verified against source: rc.A runs the identical 2.x probe `grep --color=auto -q "" /dev/null` at `aliases.go:111`, which fails on every system, so no `grep` alias is emitted (faithful reproduction). The one mismatch is SHELL-06's Fix recipe, which places the 3.0 fix "in the `internal/registry` alias pack", while the probe that decides emission and the proving test (`TestLedger_SHELL06`) are in `internal/shellinit`; the registry only carries the catalog condition. |
| Reproduction | `grep -n 'grep --color=auto' internal/shellinit/aliases.go internal/registry/catalog_gen.go` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | As SHELL-06 states: no `grep` alias; the generated reference still lists it. |
| Fix recipe (3.0) | Decide which package owns the SHELL-06 fix: (a) recommended, fix the probe in `internal/shellinit` (run `grep --color=auto -q x` on real input at `gash init bash` time) and reword SHELL-06's Fix recipe to name `internal/shellinit`; (b) keep the decision in the registry catalog condition. Left for the lead rather than edited into SHELL-06, to avoid guessing the intended fix location. |
| Proving test | the SHELL-06 proving test (`TestLedger_SHELL06` in `internal/shellinit`) already lives where option (a) puts the fix |
| Status | accept: option (a): fix the probe in internal/shellinit (operator 2026-10-24); done 582e82b (option a) |

### self-management

<a id="self-17"></a>
#### SELF-17 · gash_doctor checks the 2.x file list, which the 3.x tree no longer ships

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (this fragment; follows N7 and `upgrade.md` 7.1) |
| Area | Self-management |
| Bash location | `lib/modules/gash.sh:592-594` (12 core paths: `lib/core/help.sh`, five `lib/shell/*.sh`, `lib/aliases/_loader.sh`), `:605` (13 module names) |
| Observed behavior | The 2.x doctor checks the existence of 2.x implementation files. In the 3.x layout (`layout.md`: `lib/shell/`, `lib/aliases/`, `help.sh` and the 2.x modules removed; the binary under `versions/`), the same list would report about 20 missing files on every healthy install and say nothing about the binary or the generated shims. |
| Reproduction | `grep -n 'core_files=\|expected_modules=' $C/lib/modules/gash.sh` and compare with the "Final layout" list of `go-impl/design/layout.md` |
| Severity | medium |
| Public-contract impact | yes (doctor output lines) |
| rc.A behavior | Proposed (MATRIX `redesign`): rc.A keeps the 2.x headings, line format (`  PATH` / `  PATH — MISSING`, `  NAME — not found`), settings and tools sections and the rc 0, but takes the file list from the shipped 3.x manifest (the generated files, `VERSION`, `quotes/list.txt`, `.gash_env.template`, the binary for this `VERSION`); keeping the literal 2.x list would flag every healthy rc.A install. An open 2.x shell keeps its own list (soft degradation, `upgrade.md` 7.1). |
| Fix recipe (3.0) | Options: (a) the proposal above (recommended); (b) keep the 2.x list byte for byte and add a 3.x section (every install shows warnings); (c) replace the doctor by `gash version --check`. With (a): `internal/selfmgmt` doctor reads the manifest that `internal/compatgen` writes, adds `Binary:` and `Shims:` checks (sha256 against the release manifest), rc per SELF-06. |
| Proving test | `TestLedger_SELF17` in `internal/selfmgmt` (staged 3.x tree: `All checks passed.`; one generated shim removed: its `— MISSING` line) |
| Status | accept: option (a): file list from the 3.x manifest plus binary and shim checks (operator 2026-10-24); done 94d8efc |

### test harness

<a id="test-08"></a>
#### TEST-08 · `tests/mocks/bin/sudo` executes its arguments

| Field | Value |
|---|---|
| Section | Decisions for the operator |
| Origin | New finding (tests dossier H8) |
| Area | Test harness |
| Bash location | `tests/mocks/bin/sudo` (`exec "$@"`) |
| Observed behavior | Any test that reaches `sudo` with mocks on PATH really runs the command, unelevated. Safe today because the targets are mocks too (systemctl). |
| Reproduction | `cat tests/mocks/bin/sudo` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Kept; Go-mode runs happen in a namespace. |
| Fix recipe (3.0) | Options: keep; or make the stub log and refuse unless the target is itself a stub on PATH. |
| Proving test | A Go-mode test reaching sudo with an unmocked target fails visibly |
| Status | accept: make the stub log and refuse unless the target is a stub (operator 2026-10-24); done 2f66411 |

## Doc drift

| Id | Title | Area | Severity | Contract |
|---|---|---|---|---|
| [LLM-07](#llm-07) | llm_grep single-file output is `line:content`, not `file:line:content` | Docs | low | no |
| [LLM-28](#llm-28) | invalid_depth has two detail formats and the uint message says "positive" while 0 is accepted | Docs | low | yes |
| [LLM-30](#llm-30) | AGENTS.md section 3 misstates the llm_* grammar and search behavior | Docs | low | no |
| [LLM-31](#llm-31) | docs/reference/llm.md omits flags and gives rg-only examples | Docs | low | no |
| [LLM-32](#llm-32) | AGENTS.md section 6 security doc drift | Docs | low | no |
| [LLM-33](#llm-33) | docker_compose_check help says UP-TO-DATE but the code emits CURRENT; NOT_PULLED undocumented | Docs | low | no |
| [SYS-47](#sys-47) | hgrep help and docs: reversed `-r` description, missing options, wrong example, "POSIX awk" claim | Docs | low | no |
| [SYS-48](#sys-48) | sysinfo section descriptions do not match what the sections show | Docs | low | no |
| [SYS-49](#sys-49) | ip_public's registered help and reference page omit `--no-color` | Docs | low | no |
| [DOCK-17](#dock-17) | Doc drift: docker_compose_* statuses, registries, exit status and usage lines | Docs | low | no |
| [GIT-11](#git-11) | Doc drift: git reference, inline help and the confirmations table | Docs | low | no |
| [REM-09](#rem-09) | Doc drift: remote reference and the security deny list | Docs | low | no |
| [DEV-21](#dev-21) | The php alias table and the `gash dev` card list aliases that are not defined and miss those that are | Docs | low | no |
| [DEV-22](#dev-22) | php_detect_version and node_detect_version help omit how far detection reaches and what it does not check | Docs | low | no |
| [AI-25](#ai-25) | ai_sysinfo help omits --no-color; ai_ask help describes a chat session | Docs | low | no |
| [FS-31](#fs-31) | AGENTS 3 promises unexpected_argument for a second PATH | Docs | low | no |
| [FS-32](#fs-32) | AGENTS 3 says integers have at most 9 digits | Docs | low | no |
| [FS-33](#fs-33) | AGENTS 3.2 and the files_largest NOTES say tab/newline paths are left out | Docs | low | no |
| [FS-34](#fs-34) | AGENTS 3.2 lists sizes as bytes or K/M/G/T/P | Docs | low | yes |
| [FS-35](#fs-35) | AGENTS 4.1 says errors counts unreadable entries | Docs | low | no |
| [FS-36](#fs-36) | AGENTS 5 says scanners in --json mode report errors as one JSON line | Docs | low | no |
| [FS-37](#fs-37) | AGENTS 6.1 says scanners refuse /proc, /sys, /dev, /boot, /root | Docs | medium | no |
| [FS-38](#fs-38) | AGENTS 7 says empty results print no lines | Docs | low | no |
| [FS-39](#fs-39) | files_largest help: --exclude is a "substring match" | Docs | low | yes |
| [FS-40](#fs-40) | dirs_largest help: --exclude is a "substring match" | Docs | low | yes |
| [FS-41](#fs-41) | --human help: "auto-on on TTY" | Docs | low | yes |
| [FS-42](#fs-42) | --no-color help: "Disable ANSI colors" (the rich layout stays) | Docs | low | yes |
| [FS-43](#fs-43) | files_largest help lists the JSON envelope without total_size_human and kind | Docs | low | yes |
| [FS-44](#fs-44) | dirs_largest help does not say --depth 0 becomes 1 | Docs | low | yes |
| [FS-45](#fs-45) | dirs_find_large help does not say --depth 0 means unbounded | Docs | low | yes |
| [FS-46](#fs-46) | tree_stats help example scans / without --allow-root | Docs | low | yes |
| [FS-47](#fs-47) | Header comment: "--allow-root implies a warning" | Docs | low | no |
| [FS-48](#fs-48) | Prune-args comment: "null-safe if the caller uses mapfile -d ''" | Docs | low | no |
| [FS-49](#fs-49) | Hints say "positive integer" where 0 is accepted | Docs | low | yes |
| [FILE-26](#file-26) | archive_extract help overpromises case-insensitivity, lists `.z` for `.Z`, and omits the file effects | Docs | low | no |
| [FILE-27](#file-27) | file_backup help is silent on local time, collisions, symlinks and the stdout line | Docs | low | no |
| [SHELL-14](#shell-14) | CONTRIBUTING.md promises a BSD userland, the code uses GNU-only find -printf | Docs | low | no |
| [SELF-18](#self-18) | CONTRIBUTING.md says breaking changes need a major version; 2.1.0 carried some | Docs | low | no |
| [SELF-19](#self-19) | troubleshooting.md describes uninstall incompletely | Docs | low | no |
| [SELF-20](#self-20) | README counts of functions and aliases are stale | Docs | low | no |
| [SELF-21](#self-21) | gash_upgrade --help says it "cleans up" | Docs | low | no |
| [CORE-12](#core-12) | .gash_env.template overstates which password characters need URL-encoding | Docs | low | no |
| [CORE-20](#core-20) | gash_unload's pre-symbol snapshot is taken before the module surface, reproducing 2.x | Self-management | low | no |
| [CORE-21](#core-21) | architecture.md names only two sanctioned bash -c uses; there are now five | Docs | low | no |
| [SHELL-16](#shell-16) | The prompt and startup bench harness drops GASH_BIN, so a 3.x run uses the in-tree binary only | Docs | low | no |
| [SHELL-17](#shell-17) | design/shell.md §7 describes SSH auto-unlock via a __compat sub-command; rc.A uses the ssh module on StartPTY | Docs | low | no |
| [SHELL-18](#shell-18) | gash_migrate's registered help and the init reminder still describe the 1.x->2.0 migration | Docs | low | no |
| [SHELL-19](#shell-19) | SHELL-10 and SHELL-13 still need their dedicated proving tests written | Docs | low | no |

### llm_* agent functions

<a id="llm-07"></a>
#### LLM-07 · llm_grep single-file output is `line:content`, not `file:line:content`

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | Seed: "llm_grep on a single file prints line:content" (CONFIRMED with AGENTS.md drift) |
| Area | Docs |
| Bash location | `lib/modules/llm/fs.sh:443,466`; `AGENTS.md:51-61`; `lib/modules/llm.sh:136` |
| Observed behavior | For a single-file PATH llm_grep prints `line:content` (context lines `line-content`), while a directory PATH prints `abspath:line:content`; AGENTS.md section 3.1 and the help registry both say "file:line:content" lines. |
| Reproduction | `llm_grep TODO a.php` -> `1:TODO one` (temp dir) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: rc.A prints `line:content` for a file PATH and `abspath:line:content` for a directory, matching 2.x. |
| Fix recipe (3.0) | Code is right; fix the docs: "file:line:content (a FILE PATH: line:content); context lines use `-`, groups are separated by `--`, paths are absolute". Update `AGENTS.md:51-61` and the registry Help for llm_grep. |
| Proving test | `TestLedger_LLM07` pins `line:content` for a file and `abspath:line:content` for a directory. |
| Status | accept: done e250421 |

<a id="llm-28"></a>
#### LLM-28 · invalid_depth has two detail formats and the uint message says "positive" while 0 is accepted

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N19) |
| Area | Docs |
| Bash location | `lib/modules/llm/core.sh:284` vs `fs.sh:70` |
| Observed behavior | `invalid_depth` is emitted with two different detail strings (the uint message "is not a positive integer below 1000000000" at core.sh:284, and the bare-value "0" message with hint "Provide a positive integer" at fs.sh:70); the uint message says "positive" though 0 is accepted for limit/size/port. |
| Reproduction | `llm_tree --depth 0` vs `llm_tree --depth abc` give different `invalid_depth` details (temp dir) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: rc.A emits both detail variants exactly where 2.x does. |
| Fix recipe (3.0) | Decide one `invalid_depth` message (depth must be >= 1) and reword the uint hint to "non-negative"; keep the distinct `<1` check for depth. Document in AGENTS.md section 5. |
| Proving test | `TestLedger_LLM28` pins the chosen `invalid_depth` details for 0 and for a non-integer. |
| Status | accept: done 177c377 |

<a id="llm-30"></a>
#### LLM-30 · AGENTS.md section 3 misstates the llm_* grammar and search behavior

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (dossier section 2) |
| Area | Docs |
| Bash location | `AGENTS.md:42-61` |
| Observed behavior | Section 3 claims "a second PATH gives unexpected_argument" (false for llm_docker_check, llm_env, llm_ports, llm_procs), "integers have at most 9 digits" (false for llm_grep --context, LLM-01), and "all three search functions use ripgrep when installed" (llm_tree never uses rg, llm_find only with --contains, and rg vs grep changes llm_grep results, LLM-03); it does not document that `--flag=value` and `--` are unsupported or that `-h`/`-h` as a flag value also prints help. |
| Reproduction | n/a (documentation review) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | rc.A reproduces the real grammar; AGENTS.md, rewritten for 3.x (PRD 11.1), describes it. Help bodies stay byte-identical until 3.0. |
| Fix recipe (3.0) | Regenerate AGENTS.md section 3 from the registry Grammar/Help: per-function second-positional behavior, `--context` validation, the actual rg usage, and the no-`--flag=value`/no-`--`/help-anywhere rules. |
| Proving test | A registry-vs-docs unit test (`internal/registry`) asserts the generated section 3 matches each function's grammar. |
| Status | accept: done 0b3997d |

<a id="llm-31"></a>
#### LLM-31 · docs/reference/llm.md omits flags and gives rg-only examples

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (dossier section 2) |
| Area | Docs |
| Bash location | `docs/reference/llm.md` (generated from `lib/modules/llm.sh` Help) |
| Observed behavior | The reference omits `llm_tree --stats`/`--max-depth`, `llm_grep -n/--limit`/`-C`, `llm_git_log -n`, `llm_git_diff --cached`, `llm_ports -l`, `llm_env -f`/the bare form (and does not say the filter is an ERE); the llm_grep `"TODO | FIXME"` example only works with rg; it claims llm_find behaves "the same with or without ripgrep" and that llm_config reads only JSON/YAML (it reads any non-secret file); the llm_git_status example is compact but output is pretty. |
| Reproduction | n/a (documentation review) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: the help bodies are byte-identical in rc.A, and `docs/reference/` is generated from them, so the omissions stay until 3.0; AGENTS.md (rewritten for 3.x) states the real flags. |
| Fix recipe (3.0) | Ensure every flag and synonym is in each function's registry Help; replace the rg-only example; state the ERE/RE2 dialect; describe llm_config reading any non-secret file; show pretty git_status output. |
| Proving test | The registry-vs-docs unit test covers the generated `docs/reference/llm.md`. |
| Status | accept: done ed20656 |

<a id="llm-32"></a>
#### LLM-32 · AGENTS.md section 6 security doc drift

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (dossier section 2) |
| Area | Docs |
| Bash location | `AGENTS.md:252-289` |
| Observed behavior | Section 6.1's protected list omits `/etc/sudoers.d` (present in code) and claims the trees are protected for all llm_* path arguments although only llm_exec walks are restricted (LLM-09), and llm_docker_check has no guard (LLM-20); `/etc/shadow-`/`/etc/gshadow-` are not protected (LLM-10). Section 6.2 writes `credentials*`/`secrets*` but the code matches `credentials`/`credentials.*`/`secrets`/`secrets.*` (so `credentials_old` is allowed), and says templates like `.env.example`/`*.pub` are "allowed" though walks never search them (LLM-15). |
| Reproduction | n/a (documentation review) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | rc.A matches the code; the shared pathguard/secret tables drive both the guard and docs so they cannot drift once the gaps (LLM-09/10/15) are decided. |
| Fix recipe (3.0) | Generate section 6 from the `internal/pathguard` tables; reconcile the glob wording with the actual patterns; state that only llm_exec restricts walks until LLM-09 is accepted. |
| Proving test | The pathguard-table-vs-docs unit test asserts section 6 matches the tables. |
| Status | accept: done 70020ee |

<a id="llm-33"></a>
#### LLM-33 · docker_compose_check help says UP-TO-DATE but the code emits CURRENT; NOT_PULLED undocumented

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (dossier section 1, llm_docker_check) |
| Area | Docs |
| Bash location | `lib/modules/docker-compose.sh:1175` (help) vs `:632` (code); status set at `:646-671` |
| Observed behavior | The help registry for docker_compose_check lists a status `UP-TO-DATE`, but the code emits `CURRENT`; the `NOT_PULLED` status the code can emit is not documented. |
| Reproduction | n/a (documentation review) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: rc.A emits `CURRENT`/`NOT_PULLED` as the code does and keeps the 2.x help body byte for byte; the help text is corrected in 3.0 with this item. |
| Fix recipe (3.0) | Fix the registry Help to list `UPDATE`, `CURRENT`, `PINNED`, `NOT_PULLED`, `ERROR`. |
| Proving test | Covered by the registry-vs-docs unit test for the docker module. |
| Status | accept: done 63f58e1 (devops slice, docker_compose_check help) |

### system, sysinfo, ssh, chrome

<a id="sys-47"></a>
#### SYS-47 · hgrep help and docs: reversed `-r` description, missing options, wrong example, "POSIX awk" claim

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N9 a, c, d, e) |
| Area | Docs |
| Bash location | `lib/modules/system.sh:153,173` (inline help: `-r, --reverse  Show oldest first (default: newest last)`), `lib/modules/system.sh:1139-1146,1164` (registered help: `Reverse order (oldest first)`, no `-g/--group`, `--pick`, `--no-color`, `--limit`), `docs/reference/system.md:97`, example `lib/modules/system.sh:193-197`, comment `lib/modules/system.sh:296`, `docs/troubleshooting.md:84-91` |
| Observed behavior | The default order is oldest first and `-r` is newest first, the opposite of both help texts. The two help bodies list different options. The example shows `[2025-01-15 14:32:01]` for epoch 1705330321, which is `2024-01-15 14:52:01` UTC. The code comment says the awk program works with BSD awk, and troubleshooting says only `--json` needs strftime, but every text mode needs it and case-insensitive highlighting needs gawk's IGNORECASE. The code is right. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; printf "#1700000000\necho one\n#1700000100\necho two\n" >"$HOME/h"; HISTFILE="$HOME/h" hgrep echo; HISTFILE="$HOME/h" hgrep echo -r'` (default: one, two; `-r`: two, one) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: help body unchanged in rc.A (both the `hgrep -h` heredoc and the registered `gash_help hgrep` text), corrected in 3.0. |
| Fix recipe (3.0) | `internal/registry` hgrep entry: one help body for `-h` and `gash_help` listing all eleven options; `-r, --reverse  Newest first (default: oldest first)`; example timestamps computed from the example epochs in UTC; troubleshooting entry removed with SYS-32; regenerate `docs/reference/system.md`. |
| Proving test | `TestLedger_SYS47` in `internal/registry` (hgrep help lists every flag of its grammar; `-h` and `gash_help` bodies equal) |
| Status | accept: done 105f468 |

<a id="sys-48"></a>
#### SYS-48 · sysinfo section descriptions do not match what the sections show

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N28 c) |
| Area | Docs |
| Bash location | `lib/modules/sysinfo.sh:1903-1914` (registered help), `docs/reference/system.md:408-419` |
| Observed behavior | storage says "NFS exports/config" but shows fstab, crypttab, LVM, mdadm, ZFS, sanoid, smartd, hdparm and NFS; network says "Interfaces ... hostname" but shows neither live interfaces nor the hostname; security says "Firewall rules, SSH hardening" but shows config presence plus fail2ban/crowdsec (SSH is under auth); system says "systemd timers" (they are under services); infra says "backup systems" (none). The code is right. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; sysinfo network --llm; sysinfo --help \| grep -E "^ +(network\|storage) "'` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. |
| Fix recipe (3.0) | `internal/registry` sysinfo entry: rewrite the eleven descriptions from the collectors' sub-headers (e.g. storage `fstab, crypttab, LVM, RAID, ZFS, SMART, NFS`; network `resolv.conf, hosts, interfaces files, netplan, dhcpcd`); regenerate `docs/reference/system.md`. |
| Proving test | `TestLedger_SYS48` in `internal/sysinfo` (each section's help description names only sub-headers that section can print) |
| Status | accept: done 4c2026b |

<a id="sys-49"></a>
#### SYS-49 · ip_public's registered help and reference page omit `--no-color`

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (new) |
| Area | Docs |
| Bash location | `lib/modules/system.sh:1173-1174` (registered USAGE `ip_public`) versus `lib/modules/system.sh:549,557-560` (function comment and inline help `ip_public [--no-color]`); `docs/reference/system.md:171` |
| Observed behavior | `ip_public --no-color` is accepted and documented by the function's own fallback help, but `gash_help ip_public` and the generated reference page show `ip_public` with no option (disk_usage, the twin function, documents its `--no-color`). The code is right. |
| Reproduction | `env -i G="$PWD" HOME="$(mktemp -d)" PATH=/usr/bin:/bin LANG=C.UTF-8 bash -c 'GASH_HEADLESS=1; source "$G/gash.sh"; ip_public --help \| grep -A1 USAGE'` (`ip_public` without `--no-color`) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. |
| Fix recipe (3.0) | `internal/registry` ip_public entry: USAGE `ip_public [--no-color]` (plus `--raw` if SYS-13 is accepted); regenerate `docs/reference/system.md`. |
| Proving test | `TestLedger_SYS49` in `internal/registry` (every flag of each grammar appears in its USAGE or OPTIONS) |
| Status | accept: done 168324c |

### docker and compose

<a id="dock-17"></a>
#### DOCK-17 · Doc drift: docker_compose_* statuses, registries, exit status and usage lines

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N13) |
| Area | Docs |
| Bash location | `lib/modules/docker-compose.sh:1173-1181` (registered help), `lib/modules/docker-compose.sh:481-503,798-824,1001-1018` (`-h` heredocs), `docs/reference/docker.md:26,42-56,69,99` |
| Observed behavior | The registered help and docker.md list `UP-TO-DATE` (the code emits `CURRENT`), omit `NOT_PULLED` and `ERROR`, say only Docker Hub and GHCR are queried (every registry is, others anonymously) and do not say that the exit status is 0 when ERROR rows are printed; docker.md usage lines omit `--no-color` (check, scan), `--yes`, `-y`, `-f` and `--no-color` (upgrade, where `--yes` is required headless), `-d` and `-j` (scan); the `-h` heredocs and the registered help differ from each other. The code is right. |
| Reproduction | `g 'gash_help docker_compose_check' \| grep -n 'UP-TO-DATE'` → the status line that the code never emits |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: help body unchanged in rc.A (both the `-h` heredocs and the registered help, byte for byte), corrected in 3.0. |
| Fix recipe (3.0) | Registry Help for docker_compose_check, docker_compose_upgrade and docker_compose_scan and the generated docs/reference/docker.md: statuses `CURRENT`, `UPDATE`, `NOT_PULLED`, `PINNED`, `ERROR`; every registry queried (Docker Hub and GHCR with tokens, the others anonymously); exit 0 whenever the report prints; full usage lines from the Grammar; one help text per function (the `-h` heredoc becomes the registered help). |
| Proving test | `TestLedger_DOCK17` in `internal/compatgen` (docs generation: usage lines equal the Grammar; the help names every status the code can emit) |
| Status | accept: done 63f58e1 |

### git module

<a id="git-11"></a>
#### GIT-11 · Doc drift: git reference, inline help and the confirmations table

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (dossier git doc drift) |
| Area | Docs |
| Bash location | `docs/reference/git.md:46,78`; `lib/modules/git.sh:13,62,65`; `docs/security.md:239-252` |
| Observed behavior | git.md says git_add_tag "Requires a clean git repo" (there is no clean check); git_apply_patch "creates a patch from the specified commit" and "The commit hash must exist on the feature branch" (the code runs `git diff-index COMMIT`, the diff from COMMIT to the feature work tree, and only checks that COMMIT exists); git_dump_revisions does not say that 001 is the newest revision; the `needs_help` fallback text and header comments of git_apply_patch say "Alias: gap" (gap is `git add --patch`; the registered help says gpatch); security.md's Confirmations table does not say that tag push and delete, git_sync and git_apply_patch never ask. The code is right. |
| Reproduction | `grep -n 'clean git repo\|must exist on the feature' $GASH_SRC/docs/reference/git.md; grep -n 'Alias: gap' $GASH_SRC/lib/modules/git.sh` → git.md:46, git.md:78, git.sh:62,65 |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: help body unchanged in rc.A (the registered help, byte for byte; the "Alias: gap" fallback is never shown because the Go registry always has the help), corrected in 3.0. |
| Fix recipe (3.0) | docs/reference/git.md (generated from the registry): drop "clean", describe the diff-index semantics, say 001 is the newest; drop the gap mentions; docs/security.md: rows for git_add_tag, git_delete_tag, git_sync and git_apply_patch (with GIT-01's confirmations). |
| Proving test | `TestLedger_GIT11` in `internal/compatgen` (docs generation check; no "gap" in git_apply_patch help) |
| Status | accept: done 9eed0f7 |

### remote

<a id="rem-09"></a>
#### REM-09 · Doc drift: remote reference and the security deny list

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (dossier remote doc drift) |
| Area | Docs |
| Bash location | `docs/reference/remote.md`; `lib/modules/remote.sh:595-693` (registered help); `docs/security.md:209-229` |
| Observed behavior | Usage lines do not say that unknown flags are ignored (REM-01) or that `--exclude` needs `=`; the remote_hosts note omits GASH_ENV_FILE; remote_exec omits the exit 255 probe (rc 255 when the connection is healthy, 1 otherwise) and the creation of `~/.ssh`; security.md lists `/lib*` as denied while the code denies `/lib`, `/lib64`, `/lib32` and `/libx32` only (`/libexec`, `/library` are allowed). The code is right; the docs describe a glob the code does not implement. |
| Reproduction | `grep -n '/lib\*' $GASH_SRC/docs/security.md; grep -n 'exclude=PAT' $GASH_SRC/docs/reference/remote.md` → the deny-list row, the usage lines without the `=`-only note |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. |
| Fix recipe (3.0) | Registered help and generated docs/reference/remote.md: GASH_ENV_FILE, the 255 probe, `~/.ssh` creation, and the REM-01 flag rules; docs/security.md deny row rendered from the `internal/remote` table (`/lib /lib64 /lib32 /libx32`). |
| Proving test | `TestLedger_REM09` in `internal/compatgen` (docs generation: the security.md deny row equals the Go table) |
| Status | accept: done 4fc4e85 |

### php and node

<a id="dev-21"></a>
#### DEV-21 · The php alias table and the `gash dev` card list aliases that are not defined and miss those that are

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (verified: `gash dev` card and `docs/reference/dev.md` against `alias` output) |
| Area | Docs |
| Bash location | `lib/modules/gash.sh:676-716` (awk parse of `lib/aliases/php.sh`); `docs/reference/dev.md:187-197` |
| Observed behavior | The card and dev.md show `php → phpX.Y` without its `GASH_REPLACE_TOOLS=1` condition and the four hte aliases even when hte-cli is absent; they never show `phpXY`, `composerXY`, `1composerXY` (names built in a loop), `composer` and `composer1-self-update` (defined behind `[[ ... ]] &&`, which the awk pattern skips). On this host the shell defines php83, composer83, composer-packages-update, composer-self-update; the card shows php, composer-packages-update, composer-self-update, hte, hte-create, hte-remove, hte-details. The code is right. |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/local/bin:/usr/bin:/bin TERM=xterm bash -ic 'source $GASH_SRC/gash.sh >/dev/null 2>&1; alias \| grep -E "php\|composer\|hte"; gash dev \| grep -A9 Aliases'` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: the card's alias names and static expansions and dev.md unchanged in rc.A (except the hte rows, dropped with DEV-17), corrected in 3.0. The values 2.x took from the live shell (`BASH_ALIASES`) cannot be read by a binary, so rc.A shows the static expansion for every row (forced part, for the owner of the `gash` card). |
| Fix recipe (3.0) | The registry carries each php-pack alias with its condition (MATRIX rows) and the card and dev.md are generated from it (PRD 5.5): families `phpXY`, `composerXY`, `1composerXY` "for each installed phpX.Y", `php` and `composer` "with GASH_REPLACE_TOOLS=1", `composer1-self-update` "when composer1 is installed". |
| Proving test | `TestRegistry_AliasDocsMatchConditions` in `internal/registry` and a `gash dev` golden in `testdata/script/card_dev.txtar` |
| Status | accept: done 7b19f1d |

<a id="dev-22"></a>
#### DEV-22 · php_detect_version and node_detect_version help omit how far detection reaches and what it does not check

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (dossier php_detect_version "Defects / drift", node N9) |
| Area | Docs |
| Bash location | `lib/modules/php.sh:144-162`, `lib/modules/node.sh:444-462`; `docs/reference/dev.md` |
| Observed behavior | The help says "`.target-php-version` (walk-up)" but not that the walk goes above the project to every ancestor except `/`, that composer.json and copilot-instructions are read in DIR only, that the version is not checked against installed binaries, or that `--llm` is accepted; node's help omits that sources are reduced to a major, that a missing major is replaced by the newest installed version with a warning, and that DIR is not walked up. The code is right. |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; php_detect_version --help; node_detect_version --help'` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. |
| Fix recipe (3.0) | Registry help NOTES of both functions (and the generated dev.md) state the rules above, plus the DEV-01/DEV-05/DEV-09/DEV-18 outcomes that are accepted. |
| Proving test | `TestRegistry_HelpGolden` (help bodies golden-tested; updated with the new NOTES) |
| Status | accept: done 37bcf69 |

### ai

<a id="ai-25"></a>
#### AI-25 · ai_sysinfo help omits --no-color; ai_ask help describes a chat session

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N32, with N17) |
| Area | Docs |
| Bash location | `lib/modules/ai.sh:46,93,99,171,184`; `docs/reference/ai.md:15,77,86,173,190`; parser `lib/modules/ai/sysinfo.sh:649,671,681` |
| Observed behavior | The registered usage `ai_sysinfo [provider] [--raw]` and ai.md omit `--no-color`, which the parser, the short help (`needs_help`) and the error usage line accept and print. ai_ask is called "Interactive AI chat" and its NOTES say "Type 'exit' or 'quit' or press Ctrl+C to end the session", but it answers one question per call (AI-03). The code is right in both cases. (After AI-13 the PRIVACY line "The API key travels in a curl config read from a pipe" no longer describes 3.x.) |
| Reproduction | `env -i HOME="$(mktemp -d)" PATH=/usr/bin:/bin bash -c 'GASH_HEADLESS=1; source $GASH_SRC/gash.sh; ai_sysinfo --help \| grep -n "USAGE" -A1; ai_ask --help \| grep -n session'` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. Prose docs rewritten for 3.x (AGENTS.md, docs/*.md other than the generated reference) may describe the real behavior, including the in-process API key. |
| Fix recipe (3.0) | Registry help: ai_sysinfo usage `ai_sysinfo [claude\|gemini] [--raw] [--no-color]`; ai_ask short text "Ask the AI one question from the terminal", NOTES "One question per call" (plus AI-03's exit/quit rule if accepted); PRIVACY "The API key stays inside the gash process, never on a command line"; regenerate docs/reference/ai.md. |
| Proving test | `TestRegistry_HelpGolden` (updated goldens for ai_sysinfo and ai_ask) |
| Status | accept: done 989eb6e |

### file scanners

<a id="fs-31"></a>
#### FS-31 · AGENTS 3 promises unexpected_argument for a second PATH

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: AGENTS 3, second PATH) |
| Area | Docs |
| Bash location | `AGENTS.md:43-44` ("reject … a second PATH with `unexpected_argument`"), `AGENTS.md:459` (section 9); code `lib/modules/files.sh:416-419` and copies |
| Observed behavior | The scanners take the last PATH silently (FS-04). The doc is right; the code is wrong. |
| Reproduction | `d=$(mktemp -d); mkdir "$d/a" "$d/b"; files_largest --json "$d/a" "$d/b"` succeeds, rc 0 |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Code reproduces 2.x (FS-04); the rc.A AGENTS.md (prose, rewritten for 3.x) may state the real scanner behavior with a pointer to FS-04. |
| Fix recipe (3.0) | Fixed by FS-04; the AGENTS sentences then hold as written. |
| Proving test | `TestLedger_FS04_fix`; `TestDocs_AgentsCatalog` (generated catalog matches the registry grammar). |
| Status | accept: done via FS-04 (TestLedger_FS04_fix) |

<a id="fs-32"></a>
#### FS-32 · AGENTS 3 says integers have at most 9 digits

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: AGENTS 3, integers) |
| Area | Docs |
| Bash location | `AGENTS.md:45-46`; code `lib/modules/files.sh:424` and the other `^[0-9]+$` checks |
| Observed behavior | Scanner integers of any length are accepted (FS-05). The doc is right; the code is wrong. |
| Reproduction | `files_largest --json -n 1234567890 "$(mktemp -d)"` succeeds |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Code reproduces 2.x (FS-05, with FS-21 and FS-29 diverging); rc.A AGENTS.md may describe the real range. |
| Fix recipe (3.0) | Fixed by FS-05. |
| Proving test | `TestLedger_FS05_fix`. |
| Status | accept: done via FS-05 (TestLedger_FS05_fix) |

<a id="fs-33"></a>
#### FS-33 · AGENTS 3.2 and the files_largest NOTES say tab/newline paths are left out

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: AGENTS 3.2, tab/newline) |
| Area | Docs |
| Bash location | `AGENTS.md:76-77`, help NOTES `lib/modules/files.sh:2040-2043` |
| Observed behavior | Only files_largest leaves both out; the other four keep or mangle tab paths (FS-06 to FS-09, FS-17). The doc states the operator decision (FS-01); the code is wrong. |
| Reproduction | `d=$(mktemp -d); mkdir "$d/$(printf 'et\tx')"; dirs_list_empty --json "$d"` lists the tab path |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Code reproduces 2.x; files_largest help body unchanged; rc.A AGENTS.md may say which scanners apply the rule. |
| Fix recipe (3.0) | Fixed by the FS-01 predicate; copy the NOTES paragraph into the four other helps. |
| Proving test | `TestLedger_FS01_fix`. |
| Status | accept: done ca0b3dd |

<a id="fs-34"></a>
#### FS-34 · AGENTS 3.2 lists sizes as bytes or K/M/G/T/P

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: AGENTS 3.2, sizes) |
| Area | Docs |
| Bash location | `AGENTS.md:76`; `dirs_find_large` help NOTES ("numfmt is optional: a pure-bash IEC-suffix parser provides fallback", `lib/modules/files.sh:2135`); code `49-82` |
| Observed behavior | On the GNU path the parser also takes `E`, decimals (`1.5K`), the suffixes `B`, `iB`, `i` and the fallback oddities (FS-11, FS-22); the help NOTES imply the fallback is equivalent, which it is not. The code is the 2.x contract; the doc is incomplete. |
| Reproduction | `files_largest --json --min-size 1.5K "$(mktemp -d)"` succeeds |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0; rc.A AGENTS.md may state the GNU-path grammar. |
| Fix recipe (3.0) | After FS-11, AGENTS 3.2 and every size flag's help state the 3.0 grammar ("bytes, or a number with up to 6 decimals and K, M, G, T, P or E, optionally followed by B, i or iB"); the numfmt NOTES line is removed. |
| Proving test | `TestDocs_SizeGrammar` in `internal/registry`: the help text of every size flag matches the parser's documented grammar string. |
| Status | accept: done ca0b3dd |

<a id="fs-35"></a>
#### FS-35 · AGENTS 4.1 says errors counts unreadable entries

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: AGENTS 4.1, errors) |
| Area | Docs |
| Bash location | `AGENTS.md:148-149`; code `lib/modules/files.sh:500` and copies |
| Observed behavior | `errors` is the number of stderr lines of the walker (FS-15, FS-30). The doc is right; the code is wrong. |
| Reproduction | As an unprivileged user, one `chmod 000` directory gives `errors: 2` in dirs_list_empty and 1 elsewhere |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Code reproduces 2.x (FS-15); rc.A AGENTS.md may describe the GNU line count. |
| Fix recipe (3.0) | Fixed by FS-15. |
| Proving test | `TestLedger_FS15_fix`. |
| Status | accept: done via FS-15 (TestLedger_FS15_fix) |

<a id="fs-36"></a>
#### FS-36 · AGENTS 5 says scanners in --json mode report errors as one JSON line

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: AGENTS 5) |
| Area | Docs |
| Bash location | `AGENTS.md:207-208`; raw Bash lines from `lib/modules/files.sh:644`, `927`, `932` |
| Observed behavior | Bash arithmetic errors (`[[: 08: value too great for base`, `syntax error` for non-ASCII digits) reach stderr as raw lines under `--json`, beside an rc 0 envelope or an rc 1 with no envelope (FS-21, FS-22, FS-29). The doc is right; the code is wrong. |
| Reproduction | `dirs_largest --json --depth 08 "$(mktemp -d)"` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Already true in rc.A: those inputs get the JSON error contract (forced divergences FS-21, FS-22, FS-29). |
| Fix recipe (3.0) | None beyond rc.A. |
| Proving test | `TestLedger_FS21`, `TestLedger_FS22`, `TestLedger_FS29`. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="fs-37"></a>
#### FS-37 · AGENTS 6.1 says scanners refuse /proc, /sys, /dev, /boot, /root

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: AGENTS 6.1) |
| Area | Docs |
| Bash location | `AGENTS.md:257-258`; code `lib/modules/files.sh:150`, `187` (PATH argument only) |
| Observed behavior | Only the PATH argument is checked; `-L` and `--allow-root /` walk into those trees (FS-02, FS-03). The doc states the intent; the code is wrong. |
| Reproduction | See FS-02 (input shape, kept outside the repository) |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Code reproduces 2.x; rc.A AGENTS.md should say "as scan roots" until 3.0, so agents are not told the rail is wider than it is. |
| Fix recipe (3.0) | Fixed by FS-02 and FS-03; AGENTS 6.1 gains "also when reached through a symlink or from /". |
| Proving test | `TestLedger_FS02_fix`, `TestLedger_FS03_fix`. |
| Status | accept: done ca0b3dd |

<a id="fs-38"></a>
#### FS-38 · AGENTS 7 says empty results print no lines

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: AGENTS 7) |
| Area | Docs |
| Bash location | `AGENTS.md:388-389`; code `lib/modules/files.sh:1251` |
| Observed behavior | dirs_list_empty prints one blank line (FS-18). The doc is right; the code is wrong. |
| Reproduction | `d=$(mktemp -d); printf x >"$d/f"; dirs_list_empty "$d" \| od -c` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Code reproduces 2.x (FS-18); rc.A AGENTS.md may note the blank line. |
| Fix recipe (3.0) | Fixed by FS-18. |
| Proving test | `TestLedger_FS18_fix`. |
| Status | accept: done via FS-18 (TestLedger_FS18_fix) |

<a id="fs-39"></a>
#### FS-39 · files_largest help: --exclude is a "substring match"

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: files_largest `--exclude` help) |
| Area | Docs |
| Bash location | `lib/modules/files.sh:2029`; code `203-225` |
| Observed behavior | files_largest matches the value as a find `-path` glob against the whole absolute path (`--exclude src` matches nothing) (FS-12). The help is wrong. |
| Reproduction | `d=$(mktemp -d); mkdir "$d/src"; printf x >"$d/src/f"; files_largest --json --exclude src "$d"` still lists `src/f` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. |
| Fix recipe (3.0) | With FS-12, the line reads "Skip entries whose name or path below PATH matches GLOB (repeatable)" in all five helps. |
| Proving test | `TestLedger_FS12_fix` plus the help golden of each scanner in `internal/registry`. |
| Status | accept: done ca0b3dd |

<a id="fs-40"></a>
#### FS-40 · dirs_largest help: --exclude is a "substring match"

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: dirs_largest `--exclude` help) |
| Area | Docs |
| Bash location | `lib/modules/files.sh:2075`; code `673-676`, `721-744` |
| Observed behavior | dirs_largest applies a du glob plus a substring test, and `\|` splits one value (FS-12); "substring match" describes only the second layer. The help is incomplete. |
| Reproduction | `d=$(mktemp -d); mkdir "$d/src" "$d/logs"; dirs_largest --json --exclude 'src\|logs' "$d"` hides both |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. |
| Fix recipe (3.0) | As FS-39 (one wording for all five). |
| Proving test | `TestLedger_FS12_fix` plus the help golden. |
| Status | accept: done ca0b3dd |

<a id="fs-41"></a>
#### FS-41 · --human help: "auto-on on TTY"

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: `--human` help) |
| Area | Docs |
| Bash location | `lib/modules/files.sh:2037` (files_largest help), header comment `20`; code `444-446` and copies |
| Observed behavior | Human sizes are on whenever neither `--json` nor `--null` is given, pipes included; the TTY is not consulted. The code is the contract; the help is wrong. |
| Reproduction | `d=$(mktemp -d); head -c 2048 /dev/zero >"$d/f"; files_largest "$d" \| cat` prints `2.0 KiB` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. |
| Fix recipe (3.0) | files_largest help line: "Human-readable sizes (default except with --json or --null)". |
| Proving test | Help golden in `internal/registry`; `TestFSScan_HumanToggle` (piped output is human). |
| Status | accept: done ca0b3dd |

<a id="fs-42"></a>
#### FS-42 · --no-color help: "Disable ANSI colors" (the rich layout stays)

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: `--no-color` help) |
| Area | Docs |
| Bash location | `lib/modules/files.sh:2038`, `2082`, `2127`, `2212`; code `1762-1764` (`__gash_fs_rich_ok` ignores `--no-color`) |
| Observed behavior | On a TTY `--no-color` drops the colors but keeps the rich layout (ranks, bars, relative paths, footer); only NO_COLOR, GASH_NO_COLOR, GASH_HEADLESS or a pipe give the plain rows. The help is incomplete. |
| Reproduction | On a terminal: `files_largest --no-color "$(mktemp -d)"` prints the rich heading and `  No files matched.` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. |
| Fix recipe (3.0) | Help line: "Disable ANSI colors (layout unchanged; NO_COLOR also honored)". |
| Proving test | Help golden; `fs_render_spec.sh` (no ANSI with `--no-color` under the rich layout). |
| Status | accept: done ca0b3dd |

<a id="fs-43"></a>
#### FS-43 · files_largest help lists the JSON envelope without total_size_human and kind

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: files_largest `--json` help) |
| Area | Docs |
| Bash location | `lib/modules/files.sh:2035`, example comment `2017`; code `1694-1758` |
| Observed behavior | The envelope is `{data, count, total_size, total_size_human, kind, scan_time, errors}`; the help says `{data, count, total_size, scan_time, errors}` and the example comment `data/count/total_size/scan_time`. AGENTS 4.1 is right. The help is incomplete. |
| Reproduction | `files_largest --json "$(mktemp -d)" \| jq -c keys` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. |
| Fix recipe (3.0) | Help line: "Emit JSON envelope: {data, count, total_size, total_size_human, kind, scan_time, errors}"; same keys in the example comment. |
| Proving test | Help golden; `envelope_schema_spec.sh` (key set). |
| Status | accept: done ca0b3dd |

<a id="fs-44"></a>
#### FS-44 · dirs_largest help does not say --depth 0 becomes 1

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: dirs_largest `--depth` help) |
| Area | Docs |
| Bash location | `lib/modules/files.sh:2072-2073`; code `644` |
| Observed behavior | `--depth 0` is silently treated as 1. The code is the contract; the help omits it. |
| Reproduction | `d=$(mktemp -d); mkdir -p "$d/a/b"; dirs_largest --json --depth 0 "$d"` lists `a` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. |
| Fix recipe (3.0) | Help: "Depth limit (default 1, top-level only; 0 means 1)". |
| Proving test | Help golden; `TestFSScan_DirsLargestDepthZero`. |
| Status | accept: done ca0b3dd |

<a id="fs-45"></a>
#### FS-45 · dirs_find_large help does not say --depth 0 means unbounded

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: dirs_find_large `--depth` help) |
| Area | Docs |
| Bash location | `lib/modules/files.sh:2117`; code `932` |
| Observed behavior | `--depth 0` (and an empty value) scan without a limit. The code is the contract; the help omits it. |
| Reproduction | `d=$(mktemp -d); mkdir -p "$d/a/b"; head -c 5000 /dev/zero >"$d/a/b/f"; dirs_find_large --json --size 1 --depth 0 "$d"` lists `a/b` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. |
| Fix recipe (3.0) | Help: "Depth limit (default unbounded; 0 also means unbounded)". |
| Proving test | Help golden; `TestFSScan_FindLargeDepthZero`. |
| Status | accept: done ca0b3dd |

<a id="fs-46"></a>
#### FS-46 · tree_stats help example scans / without --allow-root

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: tree_stats example) |
| Area | Docs |
| Bash location | `lib/modules/files.sh:2200-2201` (`tree_stats --xdev --depth 8 /`); code `180-184` |
| Observed behavior | The example fails with `Error: root_scan_blocked: Scanning '/' is blocked`, rc 1. The code is right (the rail); the example is wrong. |
| Reproduction | `tree_stats --xdev --depth 8 /` (refused before any walk) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. |
| Fix recipe (3.0) | Example: `tree_stats --xdev --depth 8 --allow-root /`. |
| Proving test | `TestRegistry_HelpExamplesParse` in `internal/registry` (every help example passes its grammar and path rails on a fixture root). |
| Status | accept: done ca0b3dd |

<a id="fs-47"></a>
#### FS-47 · Header comment: "--allow-root implies a warning"

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: files.sh header comment) |
| Area | Docs |
| Bash location | `lib/modules/files.sh:23-24` |
| Observed behavior | No warning is printed with `--allow-root` (FS-03). Internal comment; the code is the contract. |
| Reproduction | `files_largest --allow-root --json --depth 0 /` prints no warning (depth 0: no walk) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Moot: the Bash source is not ported; no Go action. |
| Fix recipe (3.0) | None (comment removed with the Bash implementation at cutover); FS-03 covers the walk. |
| Proving test | None needed (comment only). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="fs-48"></a>
#### FS-48 · Prune-args comment: "null-safe if the caller uses mapfile -d ''"

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: `__gash_fs_find_prune_args` comment) |
| Area | Docs |
| Bash location | `lib/modules/files.sh:202` |
| Observed behavior | The helper prints newline-separated words, not NUL-separated ones; that is the root of FS-20. Internal comment, wrong. |
| Reproduction | See FS-20 (input shape only) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Moot: the helper is not ported (native matcher). |
| Fix recipe (3.0) | None (removed at cutover). |
| Proving test | None needed (comment only). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="fs-49"></a>
#### FS-49 · Hints say "positive integer" where 0 is accepted

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N20: error hints) |
| Area | Docs |
| Bash location | `lib/modules/files.sh:425`, `637`, `912` (invalid_limit "Provide a positive integer"), `641` (dirs_largest invalid_depth "Provide a positive integer"), `1360` (invalid_top "Provide a positive integer") |
| Observed behavior | `-n 0`, `--top 0` and `dirs_largest --depth 0` are accepted, so the hint understates the range; the other scanners' invalid_depth says "non-negative". The code is the contract; the hints are imprecise. |
| Reproduction | `files_largest --json -n x "$(mktemp -d)"` shows the hint; `-n 0` succeeds |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Reproduced: hint strings byte-identical in rc.A (they are envelope content), corrected in 3.0. |
| Fix recipe (3.0) | With FS-05: hints `Provide a non-negative integer (at most 9 digits)` for invalid_limit, invalid_depth and invalid_top in all five scanners. |
| Proving test | `TestLedger_FS05_fix` asserts the hint strings. |
| Status | accept: done d6a4bf9 |

### archive_extract and file_backup

<a id="file-26"></a>
#### FILE-26 · archive_extract help overpromises case-insensitivity, lists `.z` for `.Z`, and omits the file effects

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N12) |
| Area | Docs |
| Bash location | `lib/modules/files.sh:1895,2220-2244`; `docs/reference/files.md:9,17-51` (generated from the registry) |
| Observed behavior | "(case-insensitive)" holds for the dispatch only; the output name is stripped case-sensitively (FILE-01, FILE-02). `.z` is listed in lowercase although the common format is compress `.Z` (lowercase `.z` is pack), and the short line "(tar, zip, gz, bz2, xz, zst, rar, 7z)" omits it. Nothing says that tar, unzip, 7z and unrar print member lists and banners on stdout, that existing files are replaced (tar, single-file) or prompted for (unzip, unrar, 7z; FILE-11), that OUTPUT_DIR is created even on failure (FILE-07), how single-file outputs are named, or which tool each format needs. The `needs_help` fallback text (`files.sh:1895`) is accurate. `CHANGELOG.md:98` matches the code. The code is right about what it dispatches; the help is wrong on `.z`/case and incomplete |
| Reproduction | `R 'archive_extract --help'` → `SUPPORTED FORMATS` ending in `.7z, .z` and `(case-insensitive)` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: help body unchanged in rc.A (registry `Help` byte-identical), corrected in 3.0 |
| Fix recipe (3.0) | In the `archive_extract` registry entry (`internal/registry`; in 2.x the `__gash_register_help` heredoc): list `.Z` (compress), `.z` (pack) and, after FILE-13, `.tar.Z`/`.taz`; add a NOTES block: the tool each format needs; single-file output = archive name minus the suffix, in OUTPUT_DIR, replaced on success (FILE-03); tools' listings go to stdout; headless never prompts and never overwrites in unzip/unrar/7z (FILE-11); wildcard names refused (FILE-10). Keep "(case-insensitive)" once FILE-01/02 are fixed. Regenerate `docs/reference/files.md`; the 3.x prose docs may describe the real behavior now |
| Proving test | `tests/specs/docs_spec.sh:15` (reference generated from the registry) stays green; `TestRegistryHelpGolden` in `internal/registry` pins the new archive_extract body |
| Status | accept: done ca0b3dd |

<a id="file-27"></a>
#### FILE-27 · file_backup help is silent on local time, collisions, symlinks and the stdout line

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N12) |
| Area | Docs |
| Bash location | `lib/modules/files.sh:1966,2246-2264`; `docs/reference/files.md:13,228-254` |
| Observed behavior | The help shows the name format (`FILE_backup_YYYYMMDDHHMMSS`, example `/etc/nginx/nginx.conf_backup_20240115143022`, both right) but not that the stamp is local time without zone, that a second backup in the same second replaces the first (FILE-04) or writes through what is there (FILE-05), that a symlinked FILE is dereferenced, that `cp -v` prints `'SRC' -> 'DST'` on stdout, or the mode rules (setuid, setgid and sticky dropped, umask applied, mtime = now). Code is right; the help is incomplete |
| Reproduction | `R 'file_backup --help'` → USAGE and EXAMPLES only |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0 |
| Fix recipe (3.0) | NOTES block in the `file_backup` registry entry: local time, the collision suffix after FILE-04, source symlinks followed, the one stdout line, mode and mtime rules; regenerate `docs/reference/files.md` |
| Proving test | `tests/specs/docs_spec.sh:15` stays green; `TestRegistryHelpGolden` in `internal/registry` pins the new file_backup body |
| Status | accept: done ca0b3dd |

### shell layer

<a id="shell-14"></a>
#### SHELL-14 · CONTRIBUTING.md promises a BSD userland, the code uses GNU-only find -printf

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | Seed 7, row "CONTRIBUTING.md:40 Bash 4.4+, GNU or BSD userland" (CONFIRMED) |
| Area | Docs |
| Bash location | `CONTRIBUTING.md:41`; `find -printf` at `lib/modules/files.sh:468,1026,1384` and `lib/modules/llm/fs.sh:141` |
| Observed behavior | BSD and macOS `find` have no `-printf`, so those functions fail there; only the doctor has a `stat -c`/`stat -f` fallback. The code is the reference (GNU userland); the doc is wrong. |
| Reproduction | `grep -n 'GNU or BSD' $C/CONTRIBUTING.md; grep -n -- '-printf' $C/lib/modules/files.sh $C/lib/modules/llm/fs.sh` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Moot for the ported functions (native directory walk in Go, identical on Linux and macOS); the remaining Bash (generated shims, init output) needs only Bash 4.4+ builtins and coreutils. |
| Fix recipe (3.0) | Rewrite `CONTRIBUTING.md` "Portability": "Bash 4.4+ for the generated shell layer; the binary needs no external userland except the tools a command documents"; the line about BSD userland goes. |
| Proving test | none (doc change; the macOS CI job of `testing.md` covers the walk) |
| Status | accept: done 9d937ae |

<a id="shell-16"></a>
#### SHELL-16 · The prompt and startup bench harness drops GASH_BIN, so a 3.x run uses the in-tree binary only

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (S3 bench-harness note) |
| Area | Docs |
| Bash location | `go-impl/bench/shell/prompt_bench.sh:42-45` and `startup_bench.sh:22` (the `env -i HOME=… PATH=… TERM=… LANG=… USER=… LOGNAME=…` launch, no `GASH_BIN` in the pass-through); shim resolution `go-impl/design/compat.md:35` (`GASH_BIN` first when set) |
| Observed behavior | The bench scripts run each measurement under `env -i` with a fixed, small environment that does not include `GASH_BIN`. The 3.x generated `gash.sh` therefore cannot see a caller-provided `GASH_BIN` and always resolves the in-tree `bin/gash` of the checkout it is pointed at; a bench run cannot be aimed at an externally built or installed binary. |
| Reproduction | `grep -n 'env -i' go-impl/bench/shell/prompt_bench.sh go-impl/bench/shell/startup_bench.sh` (no `GASH_BIN` in the list) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Not a runtime behaviour: the scripts benchmark whatever `gash.sh` the given checkout resolves. For 3.x that is the tree's own `bin/gash`; `GASH_BIN` is scrubbed. |
| Fix recipe (3.0) | At the S3 3.x bench pass, add `GASH_BIN` to the `env -i` pass-through of `prompt_bench.sh` and `startup_bench.sh` (and the `bench/shell/README.md` usage), so a bench can measure a chosen binary; otherwise document that the 3.x checkout must hold the built `bin/gash`. |
| Proving test | none (bench harness; covered when the 3.x bench numbers are filled in `bench/shell/README.md`) |
| Status | accept: done 939ad3f, 91cecfe |

<a id="shell-17"></a>
#### SHELL-17 · design/shell.md §7 describes SSH auto-unlock via a __compat sub-command; rc.A uses the ssh module on StartPTY

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (shell design drift) |
| Area | Docs |
| Bash location | `go-impl/design/shell.md:157-162` (init script runs `eval "$("$__gash_bin" __compat ssh-unlock)"`); 2.x `lib/modules/ssh.sh` (`gash_ssh_auto_unlock`); rc.A `internal/shellinit/banner.go:42-47` (emits the `gash_ssh_auto_unlock` stub call) and `internal/ssh/pty.go:53` (ssh-add driven through `runner.StartPTY`) |
| Observed behavior | §7 says the emitted init script performs the SSH auto-unlock by `eval`-ing the output of a hidden `gash __compat ssh-unlock` command. rc.A has no `__compat ssh-unlock` sub-command (none in the tree); instead the init script calls the registered `gash_ssh_auto_unlock` stub, which `internal/ssh` handles, driving `ssh-add` through a PTY on `runner.StartPTY` and handing the agent exports back through the stub hand-off. The design prose names a mechanism that was superseded. |
| Reproduction | `grep -rn 'ssh-unlock' go-impl/internal go-impl/cmd` (none) versus `go-impl/design/shell.md:157` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | SSH auto-unlock is the `gash_ssh_auto_unlock` function handled by `internal/ssh`; `ssh-add` runs on `runner.StartPTY` (secrets on the PTY only), and the init script emits the stub call plus the `GASH_SSH_AUTOUNLOCK_RAN=1` export, as 2.x. |
| Fix recipe (3.0) | Update `design/shell.md` §7 to describe the auto-unlock as the `gash_ssh_auto_unlock` stub handled by `internal/ssh` on `runner.StartPTY`, not an `eval "$(gash __compat ssh-unlock)"`. |
| Proving test | none (design-doc change; the auto-unlock behaviour is covered by `internal/ssh` tests) |
| Status | accept: done 07d81fe |

<a id="shell-18"></a>
#### SHELL-18 · gash_migrate's registered help and the init reminder still describe the 1.x->2.0 migration

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (D-6 context; same thread as the core-slice migrate note and SHELL-13) |
| Area | Docs |
| Bash location | 2.x `lib/modules/migrate.sh:3,384-385` (help "Review the Gash 2.0 changes … with a diff preview and backups"), `:69` (the daily reminder line); rc.A `internal/registry/catalog_gen.go:1313-1362` (the same registered help and the `--yes`/`--rollback`/`--status` usages) and `internal/selfmgmt/migrate.go:14-26` (the ported function) |
| Observed behavior | Per D-6 the ported `gash_migrate` does not migrate a 1.x configuration: any call (options included) prints one pointer line to the 2.x route and returns 1. But its registered `--help` still carries the 2.x text and the `[--yes] [--plain]`, `--rollback` and `--status` usages, as if the review, backups and rollback were present; and the generated daily reminder still points at "gash migrate" to review the 1.x changes. The help and the reminder describe a flow the function no longer runs. |
| Reproduction | `grep -n 'Review the Gash 2.0' go-impl/internal/registry/catalog_gen.go` versus `go-impl/internal/selfmgmt/migrate.go` (`MigratePointer`) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | `gash_migrate` prints the 2.x-route pointer and returns 1 (D-6); the registered help and the daily reminder are inherited from 2.x unchanged. The reminder staying is itself correct (D-6: "the daily reminder stays as in 2.x"); the drift is the help's flags/description and the reminder's "review the changes" wording. |
| Fix recipe (3.0) | Reduce the `gash_migrate` registered help to the D-6 reality (the name, no `--rollback`/`--status`/`--yes` review, a one-line description that it points at the 2.x route) and reword the daily reminder to the pointer, keeping the once-a-day cadence. |
| Proving test | `TestLedger_SHELL18` (help text and reminder match the D-6 pointer) once `gash init bash` and the migrate help are reconciled |
| Status | accept: done 38f0d14 |

<a id="shell-19"></a>
#### SHELL-19 · SHELL-10 and SHELL-13 still need their dedicated proving tests written

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (test-coverage note for the S2 LEDGER pass / S3) |
| Area | Docs |
| Bash location | the proving tests named by LEDGER SHELL-10 (`TestLedger_SHELL10` in `internal/compatgen`, `testdata/script/load_hygiene.txtar`) and SHELL-13 (`TestLedger_SHELL13` in `internal/compatgen`, `testdata/script/headless_1x_gashrc.txtar`) |
| Observed behavior | SHELL-10 (load leaves no `BASH_REMATCH`/readonly/array internals in the caller's shell) and SHELL-13 (headless load prints and writes nothing; the interactive once-a-day 1.x notice) are recorded with named proving tests that are not yet present. Both are forced divergences that need a test pinning the rc.A behaviour and the lint/compatgen gate they describe. |
| Reproduction | the two tests are named in the SHELL-10 and SHELL-13 entries; they do not yet exist in `internal/compatgen`/`testdata/script` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The behaviours are implemented (compatgen lint for SHELL-10; the headless early return for SHELL-13); only the dedicated LEDGER proving tests are outstanding. |
| Fix recipe (3.0) | At the S2 LEDGER pass / S3, add `TestLedger_SHELL10` (lint over the generated files and an emitted init script; `load_hygiene.txtar`) and `TestLedger_SHELL13` (`headless_1x_gashrc.txtar`: empty stderr and no state file headless; the once-a-day line interactively) as the SHELL-10/SHELL-13 entries specify. |
| Proving test | the tests this item asks for (`TestLedger_SHELL10`, `TestLedger_SHELL13`) |
| Status | accept: done 39b9fc9; TestLedger_SHELL13 |

### self-management

<a id="self-18"></a>
#### SELF-18 · CONTRIBUTING.md says breaking changes need a major version; 2.1.0 carried some

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | Seed 7, row "CONTRIBUTING.md:101-102 Breaking ... needs a major version" (CONFIRMED) |
| Area | Docs |
| Bash location | `CONTRIBUTING.md:100-101`; `CHANGELOG.md:113-115` (aliases `gp gc dc gs` dropped when a program exists), `:120` (`process_kill` always asks), `:121-122` (`quit` no longer passes `--force`), `:175-182` (agent contract: second PATH argument fails with `unexpected_argument`, unknown options rejected) |
| Observed behavior | A minor release changed documented aliases, a documented alias's behavior and agent-visible errors. Both sides are defensible; the rule text is what users read. |
| Reproduction | `sed -n 100,101p $C/CONTRIBUTING.md; sed -n 113,123p $C/CHANGELOG.md` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Not applicable (rc.A keeps the 2.1.0 contract). |
| Fix recipe (3.0) | 3.0 is the major that records these: add a "Breaking changes since 2.0" list to the 3.0 CHANGELOG entry naming the 2.1.0 items above, and keep the CONTRIBUTING rule. |
| Proving test | none (doc change) |
| Status | accept: done (CHANGELOG 3.0.0) |

<a id="self-19"></a>
#### SELF-19 · troubleshooting.md describes uninstall incompletely

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | Seed 7, row "docs/troubleshooting.md:162-167" (PARTIAL) |
| Area | Docs |
| Bash location | `docs/troubleshooting.md:161-167`; `lib/modules/gash.sh:252-256` (cancel), `:297-311` (env file) |
| Observed behavior | The page says uninstall "refuses in headless mode": it prints `Info: Uninstall cancelled.` and returns 0 (no error). "Left behind" omits `~/.cache/gash/probes`, the ssh-agent socket and running agent; it says a `GASH_ENV_FILE` file is left behind, but uninstall asks about it when `GASH_ENV_FILE` is set in that shell. The code is right; the page is incomplete. |
| Reproduction | `mkdir -p $H/fk/lib/core; : >$H/fk/gash.sh; $E bash -c 'GASH_HEADLESS=1; source $C/gash.sh; GASH_DIR=$HOME/fk gash_uninstall; echo rc=$?' </dev/null` → the `This will remove Gash` warning, `Info: Uninstall cancelled.`, `rc=0`, `$H/fk` still present |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: rc.A behaves as the code (cancel, rc 0) and the page is unchanged in rc.A. |
| Fix recipe (3.0) | Rewrite the uninstall paragraph from the 3.0 behavior: no terminal → cancelled, nothing removed; left behind: `~/.local/state/gash`, `~/.cache/gash/probes` (2.x only), the ssh-agent socket, a `PROFILE=` block, and `GASH_ENV_FILE` only when not set in the uninstalling shell; the binary `versions/` directory goes with the install directory. |
| Proving test | none (doc change; uninstall behavior covered by SELF-02) |
| Status | accept: done 52ac68f |

<a id="self-20"></a>
#### SELF-20 · README counts of functions and aliases are stale

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | Seed 7, row "README.md:12 and README.md:96" (PARTIAL) |
| Area | Docs |
| Bash location | `README.md:12` ("about eighty functions and a hundred aliases"), `README.md:96` ("44 short aliases" for git) |
| Observed behavior | 85 registered functions on the card, 94 public function names, 133 aliases in the audit shell (more with docker and php); the card's static count is 109. Git: 38 aliases plus 6 functions = 44, so ":96" is right only if the gl* functions count as aliases. The code is the reference. |
| Reproduction | `$E bash --noprofile --norc -i -c 'source $C/gash.sh; alias -p \| wc -l; gash --no-color \| grep functions' </dev/null` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: README unchanged in rc.A. |
| Fix recipe (3.0) | Generate both numbers from `internal/registry` into README by `internal/compatgen` (docs generation), or write "about ninety functions and over a hundred aliases" and "38 short aliases and 6 log helpers". |
| Proving test | `TestRegistry_ReadmeCounts` in `internal/registry` (README numbers equal the registry's) |
| Status | accept: done 5b94155 |

<a id="self-21"></a>
#### SELF-21 · gash_upgrade --help says it "cleans up"

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (N17, help-text part) |
| Area | Docs |
| Bash location | `lib/modules/gash.sh:1170-1172` (help NOTES: "Fetches the latest git tag, checks out that version, and cleans up.") |
| Observed behavior | 2.x does no cleanup (that was 1.x: `git reset --hard`, `reflog expire`, `gc --aggressive`); it fetches, checks out the tag and stops. The code is right. |
| Reproduction | `$E bash --noprofile --norc -i -c 'source $C/gash.sh; gash_upgrade --help \| tail -3' </dev/null` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: help body unchanged in rc.A, corrected in 3.0. |
| Fix recipe (3.0) | Registry help NOTES for `gash_upgrade`: "Fetches the release tags, checks out the newest release (never an older one), and fetches its binary. Open a new terminal afterwards." |
| Proving test | `TestRegistry_HelpGolden` golden updated in 3.0 (rc.A golden is the 2.x body) |
| Status | accept: done d37b1eb |

### lib/core

<a id="core-12"></a>
#### CORE-12 · .gash_env.template overstates which password characters need URL-encoding

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | Seed 7, row ".gash_env.template:32-37" (PARTIAL) |
| Area | Docs |
| Bash location | `.gash_env.template:32-37`; parser `lib/core/config.sh:368-436` (user and password split at the first `:`, host at the last `@`; only the password is URL-decoded) |
| Observed behavior | Raw `@`, `:`, `/` and `#` in a password parse correctly; only a literal `%XX` sequence must be written `%25XX`. The username is not decoded at all, which the template does not say. The code is the reference. |
| Reproduction | `$E bash -c 'GASH_HEADLESS=1; source $C/gash.sh; __gash_parse_db_url "mysql://u:p@:/#x@h/d" a b pw c d e; echo "$pw"' </dev/null` → `p@:/#x` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Reproduced: template unchanged in rc.A (the binary embeds the same file). |
| Fix recipe (3.0) | Template text: "Write `%` as `%25`; other characters may be written raw or URL-encoded. The user name is used as written (not decoded)." Same in `docs/configuration.md` (DB entries). |
| Proving test | `TestLedger_CORE12` in `internal/config` (password `p@:/#x` raw and encoded parse the same; `%2540` decodes to `%40`) |
| Status | accept: done 2e890cb |

<a id="core-20"></a>
#### CORE-20 · gash_unload's pre-symbol snapshot is taken before the module surface, reproducing 2.x

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (core slice, gash_unload parity note) |
| Area | Self-management |
| Bash location | 2.x `gash.sh:191` (`__gash_symbols_now __GASH_PRE_SYMBOLS`, before the core and module sources); 3.x generated `go-impl/compat/gash.sh.tmpl:199-212` (snapshot before the `__gash_source` of bootstrap, the four core shims and the module range) |
| Observed behavior | The `__GASH_PRE_SYMBOLS` snapshot (the function and alias names that existed before Gash) is taken before the stub/alias surface is defined, not after, so every Gash function and alias counts as "added" and is swept on `gash_unload`. This is intentional and interactive-only; a re-source keeps the first load's pre-Gash state. It is a structural choice of the generated `gash.sh`, not a defect. |
| Reproduction | `grep -n '__gash_symbols_now __GASH_PRE_SYMBOLS' go-impl/compat/gash.sh.tmpl` shows the call precedes the module `__gash_source`/`range .Modules` block; 2.x `gash.sh:191` is likewise before its core and module sources |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Parity with 2.x: the generated `gash.sh.tmpl` snapshots `__GASH_PRE_SYMBOLS` and the symbol helpers before the stub/alias surface (interactive only), so non-`gash_`-prefixed module functions and aliases are swept on unload exactly as 2.x does. |
| Fix recipe (3.0) | None: the placement is deliberate and matches 2.x. Recorded so the snapshot is not "moved after the stub surface" in a later refactor; the `gash.sh.tmpl` comment already states the intent. |
| Proving test | covered by the `gash_unload` parity tests of `internal/selfmgmt` (the module surface is removed and the pre-Gash symbols restored) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="core-21"></a>
#### CORE-21 · architecture.md names only two sanctioned bash -c uses; there are now five

| Field | Value |
|---|---|
| Section | Doc drift |
| Origin | New finding (core slice, gash_doctor --timing) |
| Area | Docs |
| Bash location | `go-impl/design/architecture.md:143` ("`sh -c` anywhere except `llm_exec` … and the nvm bridge"); the sanctioned set is `internal/runner/bash.go:13-36` (`BashLLMExec`, `BashNVMBridge`, `BashDoctorLoad`, `BashDoctorShell`, `BashDoctorBare`) |
| Observed behavior | The architecture text names only two sanctioned `bash -c` uses (llm_exec and the nvm bridge). rc.A has five: the three `BashDoctor*` shells were added for `gash_doctor --timing` (the load probe, the whole-shell timing `bash -i -c exit`, and the bare-shell timing). The `internal/runner` doc comment already lists all five; the design prose does not. |
| Reproduction | `grep -n 'nvm bridge' go-impl/design/architecture.md` and the `BashUse` enum in `internal/runner/bash.go` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Five sanctioned uses, each gated by `BashUse` and checked by `runner`; the doctor-timing shells carry `-i` and run only fixed scripts. |
| Fix recipe (3.0) | Update `architecture.md` to name the five sanctioned `bash -c` uses (llm_exec, the nvm bridge, and the three `gash_doctor --timing` shells), or point at the `BashUse` enum as the source of truth. |
| Proving test | none (doc change; the `BashUse` check is covered by `internal/runner` tests) |
| Status | accept: done d4ec579 |

## HTE design delta

| Id | Title | Area | Severity | Contract |
|---|---|---|---|---|
| [HTE-01](#hte-01) | `--ssl=no` and `--forcessl=no` never produce an HTTP-only or non-forced site | Correctness | medium | yes |
| [HTE-02](#hte-02) | Invalid `--docroot`, `--phpver`, `--profile` silently fall back under `-n` | Correctness | medium | yes |
| [HTE-03](#hte-03) | An invalid default loops forever under `-n` (create, `hosts add`) | Correctness | medium | yes |
| [HTE-04](#hte-04) | `remove` and `modify` act on site #1 when `--domain` is a typo | Security | high | yes |
| [HTE-05](#hte-05) | Site identity comes from the FPM socket name; the pool is found by substring | Correctness | high | no |
| [HTE-06](#hte-06) | Certificate generation runs `rm -rf <certdir>/<domain>.*` | Security | high | no |
| [HTE-07](#hte-07) | PHP slow log and error log are written inside the document root | Security | medium | yes |
| [HTE-08](#hte-08) | The document root is interpolated unquoted into the vhost and the pool | Security | high | no |
| [HTE-09](#hte-09) | Apache restart without a config test, and no rollback | Correctness | high | yes |
| [HTE-10](#hte-10) | Forced HTTPS redirects with 301 | Correctness | low | yes |
| [HTE-11](#hte-11) | `modify` regenerates the whole vhost and drops manual edits | Correctness | medium | yes |
| [HTE-12](#hte-12) | Partial failures exit 0 with "successfully!" | Contract | medium | yes |
| [HTE-13](#hte-13) | Numbered vhost file names, index counting foreign files, renumbering after remove | Correctness | low | yes |
| [HTE-14](#hte-14) | Unused 128-character passphrase next to `-nodes` | Security | low | no |
| [HTE-15](#hte-15) | Weak `SSLCipherSuite`, no `SSLProtocol` | Security | medium | no |
| [HTE-16](#hte-16) | Apache 2.2 `order allow,deny` / `allow from all` beside `Require all granted` | Correctness | low | no |
| [HTE-17](#hte-17) | A generated certificate script runs as root from `$TMPDIR` | Security | low | no |
| [HTE-18](#hte-18) | The pool's `user`/`group` follow the invoker; `modify` as root writes `user = root` | Correctness | high | yes |
| [HTE-19](#hte-19) | Who the user is comes from `USER`/`SUDO_USER`, not from the process | Security | medium | yes |
| [HTE-20](#hte-20) | `--domain` is used verbatim; duplicates are found case-sensitively | Correctness | low | yes |
| [HTE-21](#hte-21) | IP addresses are accepted as domains | Correctness | low | yes |
| [HTE-22](#hte-22) | A bare `/etc/php/X.Y` directory counts as an installed PHP-FPM version | Correctness | medium | yes |
| [HTE-23](#hte-23) | `hte-cli fix` has no counterpart; its "restarted successfully" was always printed | Contract | low | yes |
| [HTE-24](#hte-24) | `/etc/hosts` lines tagged `# hte-cli` are edited line by line | Correctness | low | yes |
| [HTE-25](#hte-25) | `remove` leaves the `.info` file; disabling SSL leaves all three certificate files | Correctness | low | yes |
| [HTE-26](#hte-26) | The `.htaccess` HTTPS-redirect warning appears only in `fix` and misses the X-Forwarded-Proto form | Correctness | low | yes |
| [HTE-27](#hte-27) | `create` restarts Apache before PHP-FPM | Correctness | low | no |
| [HTE-28](#hte-28) | Discovery recurses and stops silently at the first unreadable directory | Correctness | low | no |
| [HTE-29](#hte-29) | Trailing slash and relative document roots are written as typed | Correctness | low | yes |
| [HTE-30](#hte-30) | "Forced" is a substring test on a comment | Correctness | low | no |
| [HTE-31](#hte-31) | hte-cli 1.1.0 crashes on a marked vhost without a parseable socket | Correctness | low | no |
| [HTE-32](#hte-32) | User-owned Apache and PHP-FPM masters instead of the system services | Contract | medium | yes |
| [HTE-33](#hte-33) | Every file moves from `/etc` and `/var/run` to the user's XDG directories | Contract | medium | yes |
| [HTE-34](#hte-34) | Declarative TOML state; rendered files are generations, never the source of truth | Contract | low | yes |
| [HTE-35](#hte-35) | FPM socket path and name change; the domain is no longer in the socket name | Contract | low | yes |
| [HTE-36](#hte-36) | Apache listens on loopback only instead of on every interface | Security | medium | yes |
| [HTE-37](#hte-37) | Vhosts also answer on `[::1]` | Portability | low | yes |
| [HTE-38](#hte-38) | HTTP/2 on cleartext (h2c), and a contradiction in the design sample | Correctness | low | yes |
| [HTE-39](#hte-39) | Certificates: ECDSA P-256, 10 years, proper extensions, made by the openssl CLI | Security | low | yes |
| [HTE-40](#hte-40) | One certificate directory per site, user-owned, new file names | Contract | low | yes |
| [HTE-41](#hte-41) | Per-version session directory where the platform's default is not writable | Portability | low | yes |
| [HTE-42](#hte-42) | The staging/hardened `open_basedir` list is Debian's on every platform | Portability | low | yes |
| [HTE-43](#hte-43) | `slowlog` is set without `request_slowlog_timeout`, so it stays empty | Correctness | low | yes |
| [HTE-44](#hte-44) | Pools add `security.limit_extensions = .php` | Security | low | yes |
| [HTE-45](#hte-45) | migrate leaves legacy `# hte-cli` hosts lines untouched | Correctness | low | yes |
| [HTE-46](#hte-46) | No DNS cache flush on Linux after a hosts change | Portability | low | no |
| [HTE-47](#hte-47) | `www.` alias rules, extra aliases, and their certificate coverage | Contract | low | yes |
| [HTE-48](#hte-48) | Apache logs: no `${APACHE_LOG_DIR}`, per-site directory, a per-site error log, no rotation yet | Contract | low | yes |
| [HTE-49](#hte-49) | Errors are the Gash envelope on stderr, not a box on stdout | Contract | medium | yes |
| [HTE-50](#hte-50) | `--json` on every command | Contract | low | yes |
| [HTE-51](#hte-51) | No prompts or menus without a TTY; destructive steps need `--yes` | Contract | medium | yes |
| [HTE-52](#hte-52) | Force-HTTPS bypass for 127.0.0.1 and ::1 makes the redirect a no-op under loopback-only binding | Correctness | medium | yes |
| [HTE-53](#hte-53) | Command and flag surface: `hte-cli <cmd>` becomes `gash hte site\|hosts\|...` | Contract | medium | yes |
| [HTE-54](#hte-54) | Hosts entries are always managed; `--hosts=no` has no counterpart | Contract | low | yes |
| [HTE-55](#hte-55) | Defaults when a flag is absent | Contract | low | yes |
| [HTE-56](#hte-56) | Ports 80/443 without a root master: sysctl, pf, or high ports | Portability | medium | yes |
| [HTE-57](#hte-57) | Autostart through a user unit, a LaunchAgent, or the first shell | Portability | low | yes |
| [HTE-58](#hte-58) | Supported platforms: Debian/Ubuntu only becomes Debian/Ubuntu, Fedora, Arch, macOS | Portability | low | yes |
| [HTE-59](#hte-59) | The server-level configuration is rendered by gash, not Debian's `apache2.conf` | Security | medium | yes |
| [HTE-60](#hte-60) | A missing document root is never created | Correctness | low | yes |
| [HTE-61](#hte-61) | An `install` step, a receipt, and a restoring `uninstall` | Self-management | low | yes |
| [HTE-62](#hte-62) | Mutations take a lock and append to an audit log | Correctness | low | no |
| [HTE-63](#hte-63) | New commands with no hte-cli counterpart | Contract | low | yes |
| [HTE-64](#hte-64) | `migrate --apply` stops and disables the system Apache; legacy files are archived | Contract | medium | yes |
| [HTE-65](#hte-65) | Imported forced sites change behavior: V1 lose their redirect, all move to 307 | Contract | low | yes |
| [HTE-66](#hte-66) | migrate finds the pool by exact path; pool values, profile and owner are reconciled | Correctness | medium | yes |
| [HTE-67](#hte-67) | migrate copies the legacy certificate pair through the root step, or reissues it | Contract | low | yes |
| [HTE-68](#hte-68) | migrate reports duplicates, shared sockets, custom ports and orphans that hte-cli never noticed | Correctness | low | yes |
| [HTE-69](#hte-69) | The hosts helper cannot rename over a bind-mounted /etc/hosts (containers) | Platform | low | no |
| [HTE-70](#hte-70) | migrate --apply with --ports high keeps the system Apache running | Correctness | medium | yes |
| [HTE-71](#hte-71) | A reload after an overlay edit could not roll back | Correctness | medium | no |
| [HTE-72](#hte-72) | Error codes for a missing openssl, a bad flag value and PHP settings on a site without PHP | Contract | low | yes |
| [HTE-73](#hte-73) | With systemd autostart the stack has one supervisor | Correctness | low | no |
| [HTE-74](#hte-74) | A root operation restarts the system Apache when migrate fails after the switch | Security | medium | no |
| [HTE-75](#hte-75) | install next to a system Apache it may not stop exits 0 with the stack not started | Contract | low | yes |

### HTE (design delta vs hte-cli)

<a id="hte-01"></a>
#### HTE-01 · `--ssl=no` and `--forcessl=no` never produce an HTTP-only or non-forced site

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (answer_to_bool and HTTP-only sites); hte-cli dossier F1 |
| Area | Correctness |
| hte-cli location | `app/helpers.php:43-61`; `app/Commands/CreateCommand.php:112-114,136-145,171,193`; `app/Commands/ModifyCommand.php:509-510,534-540` |
| Observed behavior | `answer_to_bool` returns null for `no n 0 false`, so create re-prompts and `-n` takes the default `y`; an interactive "no" is returned as the truthy string `'no'`. Since 1.0.5 (phar 6cf4d53) create always writes the 443 block and the force block; modify `--ssl=no`/`--forcessl=false`/`--forcessl=0` say "No changes specified" and exit 0. 1.0.0 to 1.0.3 worked |
| Reproduction | `work/hte/run.sh $SB create --domain=a.test --docroot=$SB/d --phpver=8.3 --ssl=no --forcessl=no --hosts=no --profile=dev -n` (writes the 443 block, the force block and a hosts line) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Boolean flag pairs `--https`/`--no-https` and `--force-https`/`--no-force-https` (cobra bool flags) set `https`/`force_https` in the site TOML. No string-to-bool helper exists; a value cobra cannot parse fails with the flag error of the Gash contract, never with a default. `--force-https` with `--no-https` fails (RETRY) instead of being ignored |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE01` in `internal/hte/state` (every flag combination → TOML values, the contradictory pair refused); `testdata/script/hte_site_create_http_only.txtar` (`--no-https` renders no `:443` vhost and no certificate) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-02"></a>
#### HTE-02 · Invalid `--docroot`, `--phpver`, `--profile` silently fall back under `-n`

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (silent fallbacks); hte-cli dossier F2 |
| Area | Correctness |
| hte-cli location | `app/Commands/CreateCommand.php:125-135,155-162,165-168`; `app/CommandWrapper.php:165-182` |
| Observed behavior | Under `-n` a missing directory becomes `getcwd()`, an unknown PHP version the highest installed one, an unknown profile `dev`; the duplicate-domain check runs only after every prompt |
| Reproduction | `work/hte/run.sh $SB create --domain=a.test --docroot=/nonexistent --phpver=9.9 --profile=x --ssl=y --forcessl=y --hosts=n -n` (site created on `$SB/cwd`, PHP 8.3, profile dev) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Each invalid value fails before anything is written, in argument order: `docroot_not_found` / `invalid_docroot`, `php_version_not_installed`, `invalid_profile`, `site_exists` (all RETRY), hint listing the valid values (installed versions, `dev, staging, hardened`). An absent optional flag takes the documented default (HTE-55), never a substitute for an invalid value |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE02` in `internal/hte/state` (table: each invalid value → its code, no file under the sysroot changed) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-03"></a>
#### HTE-03 · An invalid default loops forever under `-n` (create, `hosts add`)

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (the infinite loop); hte-cli dossier F2, A.6 |
| Area | Correctness |
| hte-cli location | `app/CommandWrapper.php:165-182`; `app/Commands/CreateCommand.php:117-124`; `app/Commands/HostsCommand.php:213` |
| Observed behavior | `keepAsking` re-asks until the validator passes; under `-n` `ask()` returns the default `""`, which never passes: 164,083 "Invalid input. Please try again." lines in 2 s |
| Reproduction | `timeout 2 work/hte/run.sh $SB create --domain=bad_domain --docroot=$SB/d --phpver=8.3 --ssl=y --forcessl=y --hosts=n --profile=dev -n \| wc -l` |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | No retry loop exists. Without a TTY (or with `GASH_HEADLESS=1`) an invalid value fails once with its code (`invalid_domain`, RETRY); a missing required flag fails with `missing_value`. On a TTY a missing required flag is asked once; an invalid answer fails with the same code |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE03` in `internal/hte/cli` (prompt fake answering an invalid value: one error, one exit); `testdata/script/hte_no_tty.txtar` (stdin `/dev/null`, missing `--domain` → `missing_value`, exits within 1 s) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-04"></a>
#### HTE-04 · `remove` and `modify` act on site #1 when `--domain` is a typo

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (remove of site #1 on a typo); hte-cli dossier F2 |
| Area | Security |
| hte-cli location | `app/Commands/RemoveCommand.php:106-118`; `app/Commands/ModifyCommand.php:152-163`; `app/Traits/InteractiveMenu.php:120-146` |
| Observed behavior | An unknown `--domain` opens a menu whose default is the first vhost; under `-n` it is chosen silently. `remove --force -n --domain=typo.test` removed `one.test` (vhost, pool, crt, key, hosts line) and renumbered the rest |
| Reproduction | `work/hte/run.sh $SB remove --domain=typo.test --force -n` (on a sandbox holding one site created as in HTE-01) |
| Severity | high |
| Public-contract impact | yes |
| rc.A behavior | `--domain` is required on `site modify/remove/show/enable/disable` and `cert show/renew`; an unknown one fails with `site_not_found` (RETRY, hint: `gash hte site list`). No command ever picks a site the user did not name |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE04` in `internal/hte/state` (two sites, `remove --domain typo.test --yes` → `site_not_found`, both sites byte-identical) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-05"></a>
#### HTE-05 · Site identity comes from the FPM socket name; the pool is found by substring

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (socket-name identity and substring pool lookup); hte-cli dossier F3 |
| Area | Correctness |
| hte-cli location | `app/Services/ApacheManager.php:73-76,148-159`; `app/Services/PhpFpmManager.php:70-112` (`strpos($contents, "[{$domain}]")` at :101) |
| Observed behavior | Domain and PHP version are parsed back from `php{v}-fpm-{domain}.sock`; the pool is the first file under `/etc/php/**/fpm/pool.d/` that contains `[{domain}]` anywhere, comments included and no marker needed. `valid.test` resolved to the unrelated `7.4/fpm/pool.d/aaa-notes.conf`, which `remove` would delete |
| Reproduction | Fixture `Malformed/` (`025-valid.test.conf` and `etc/php/7.4/fpm/pool.d/aaa-notes.conf`), read back as in `Malformed/expected-discovery-1.1.1.json` |
| Severity | high |
| Public-contract impact | no |
| rc.A behavior | The identity is the `domain` key of `sites/<domain>.toml` (file name and key must agree, else the site is reported invalid by `doctor`). Rendered files and sockets are derived from it; nothing is parsed back from rendered config. migrate uses the exact pool path plus the `[<domain>]` first section (HTE-66) |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE05` in `internal/hte/migrate` (on `Malformed/`, `valid.test` gets its exact-path pool `/etc/php/8.3/fpm/pool.d/valid.test.conf`, `aaa-notes.conf` is reported as an orphan pool and left untouched, and a pool at the exact path whose first section names another site is not taken) and in `internal/hte/state` (a TOML whose `domain` differs from its file name is refused) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-06"></a>
#### HTE-06 · Certificate generation runs `rm -rf <certdir>/<domain>.*`

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (rm -rf certificate cleanup); hte-cli dossier F4 |
| Area | Security |
| hte-cli location | `app/Services/SslCertManager.php:92` |
| Observed behavior | The prefix glob deletes other sites' files: creating `shop.test` deleted `shop.test.it.crt/.key/.info`, so Apache fails for `shop.test.it` on its next start |
| Reproduction | `work/hte/run.sh $SB create --domain=shop.test.it --docroot=$SB/d --phpver=8.3 --ssl=y --forcessl=y --hosts=n --profile=dev -n && work/hte/run.sh $SB create --domain=shop.test --docroot=$SB/d --phpver=8.3 --ssl=y --forcessl=y --hosts=n --profile=dev -n && ls $SB/etc/apache2/certs-selfsigned` |
| Severity | high |
| Public-contract impact | no |
| rc.A behavior | No glob and no shell: each site owns `certs/<domain>/` (HTE-40); create, renew and remove touch only that directory, by exact path, through Go file calls |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE06` in `internal/hte/certs` (sites `shop.test.it` then `shop.test`: the first site's `cert.pem`/`key.pem` unchanged byte for byte after create, renew and remove of the second) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-07"></a>
#### HTE-07 · PHP slow log and error log are written inside the document root

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (PHP logs in the docroot); hte-cli dossier F5 |
| Area | Security |
| hte-cli location | `app/Logic/Preprocessors/PhpProfiles.php:216,218`; `app/Services/PhpFpmManager.php:165,167` (fallback template) |
| Observed behavior | `slowlog = {docroot}/php{v}-fpm-slow.log` and `php_admin_value[error_log] = {docroot}/php{v}-fpm-errors.log`: PHP errors (paths, SQL fragments) can be fetched over HTTP as static files; no rotation (LAMP-HARDENING.md:28) |
| Reproduction | `grep -n 'docroot\|fpm-' go-impl/testdata/hte/legacy/templates/v2-pool-dev.conf` |
| Severity | medium |
| Public-contract impact | yes (log locations) |
| rc.A behavior | `slowlog = $XDG_STATE_HOME/gash/hte/logs/<domain>/php-slow.log`, `php_admin_value[error_log] = .../logs/<domain>/php-error.log`, FPM master log `logs/php-fpm-<X.Y>.log`; nothing is ever written under a document root. `service logs --site D` shows them. migrate never deletes files, so old `php*-fpm-*.log` files stay in the docroot |
| Fix recipe (3.0) | none: the delta is the design. Recommended addition to migrate: list the legacy log files found in each docroot in the plan, with a note that they may be served |
| Proving test | `TestLedger_HTE07` in `internal/hte/render` (golden pools per profile: no rendered path starts with the docroot except `open_basedir`); `hte-e2e`: a PHP warning lands in `logs/<domain>/php-error.log` and the docroot listing is unchanged. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), step "logs stay out of the document root" (the warning in `php-error.log`, the docroot listing unchanged). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-08"></a>
#### HTE-08 · The document root is interpolated unquoted into the vhost and the pool

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (unquoted docroot injection); hte-cli dossier F6 |
| Area | Security |
| hte-cli location | `app/Services/ApacheManager.php:406,426,445,460`; `app/Logic/Preprocessors/PhpProfiles.php:104,216,218` |
| Observed behavior | A docroot with a space gives `DocumentRoot /x/my docs` ("takes one argument", config invalid) and is read back as `/x/my`. A directory name containing a newline injected a directive into both vhost blocks and into the pool; the injected directive is parsed by the root-started master (shape only; the harmless reproduction injects `Header set X-Injected yes`) |
| Reproduction | `mkdir -p "$SB/x$(printf '\n')Header set X-Injected yes" && work/hte/run.sh $SB create --domain=inj.test --docroot="$SB/x$(printf '\n')Header set X-Injected yes" --phpver=8.3 --ssl=n --forcessl=n --hosts=n --profile=staging -n` |
| Severity | high |
| Public-contract impact | no |
| rc.A behavior | Validation refuses a docroot with any control character, newline included (`invalid_docroot`, RETRY). Apache paths go through `quoteApacheArg` (double quotes, `"` and `\` escaped), so spaces work. The characters Apache or php-fpm would read as syntax are refused too, with `invalid_docroot` naming the character: `$` (Apache and the INI parser expand `${NAME}`), `*`, `?`, `[`, `]` (`<Directory>` wildcards), `;` (INI comment), `"` and `\` (INI quoting), besides `:` (the `open_basedir` separator); migrate reports a legacy vhost with such a root and leaves it in the system. Apache 2.4.58 `mod_proxy_fcgi` sends `SCRIPT_FILENAME` percent-encoded, so a root with a space gave "File not found.": the vhost sets `ProxyFCGISetEnvIf "true" SCRIPT_FILENAME "%{unescape:%{reqenv:SCRIPT_FILENAME}}"`, which gives FPM the path Apache resolved (lead decision, 2026-10-18) |
| Fix recipe (3.0) | Design addition: `invalid_docroot` also for a docroot containing `$`, `*`, `?`, `[`, `;`, `:`, `"` or `\` (hint: "rename the directory or use a symlink without these characters"); `:` because it separates the `open_basedir` list (lead decision, 2026-10-18); pool values double-quoted. Prove with `httpd -t` and `php-fpm -t` on the rendered files |
| Proving test | `TestLedger_HTE08` in `internal/hte/render` (table of docroots: spaces accepted and rendered quoted; each refused character → `invalid_docroot`; fuzz `quoteApacheArg` so that the output never has a newline); `hte-e2e`: a docroot with a space serves `index.php`. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), step "a document root with a space" (`/index.php` and `/index.php/extra?q=a%20b`). |
| Status | accept: done (rc.A); proven by TestLedger_HTE08, FuzzQuoteApacheArg, E2E docroot with a space |

<a id="hte-09"></a>
#### HTE-09 · Apache restart without a config test, and no rollback

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (restart without config test, no rollback); hte-cli dossier F7 |
| Area | Correctness |
| hte-cli location | `app/Commands/CreateCommand.php:222-241`; `app/Commands/ModifyCommand.php:597-611,664-669`; `app/Services/ServiceManager.php:27-30` |
| Observed behavior | No `apachectl configtest` anywhere; `systemctl restart apache2.service` (not reload) runs on an untested config, so one bad vhost stops every site. a2ensite and restart failures exit 1 and leave the vhost, pool, certificate and symlink in place; modify ignores the a2* results |
| Reproduction | `HTE_FAIL='restart apache2' work/hte/run.sh $SB create --domain=a.test --docroot=$SB/d --phpver=8.3 --ssl=n --forcessl=n --hosts=n --profile=dev -n; ls $SB/etc/apache2/sites-available` |
| Severity | high |
| Public-contract impact | yes (reload instead of restart) |
| rc.A behavior | Every mutation renders a new generation `render/gen-<n>/`, runs `httpd -t` and `php-fpm -t` on it, swaps `render/current`, sends Apache `-k graceful` and FPM `SIGUSR2`, then health-checks each site. Any failure restores the previous generation and state: `config_test_failed` (STOP) before the swap, `reload_failed` (STOP, details say whether the rollback succeeded) after it |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE09` in `internal/hte/service` (runner fake: config test fails → `current` unchanged, state unchanged; reload fails → previous generation back, `reload_failed` with `rollback: ok`); `hte-e2e`: an overlay with a syntax error leaves the other sites answering. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), steps "an overlay error leaves the sites answering" and "a reload that fails rolls back" (a busy extra port: Apache exits on the graceful restart, the previous generation runs again). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-10"></a>
#### HTE-10 · Forced HTTPS redirects with 301

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (301 redirects); hte-cli dossier F8 |
| Area | Correctness |
| hte-cli location | `app/Services/ApacheManager.php:398` (and the `fix` output at :312,:330) |
| Observed behavior | `RewriteRule ^/(.*) https://%{HTTP_HOST}%{REQUEST_URI} [R=301,L]`: browsers cache the redirect permanently, so turning force-HTTPS off later does not reach a browser that saw it |
| Reproduction | `grep -n 'R=301' go-impl/testdata/hte/legacy/templates/v2-vhost-forced.conf` |
| Severity | low |
| Public-contract impact | yes (redirect status) |
| rc.A behavior | `[R=307,L]`, method and body preserved, never cached as permanent. migrate imports forced sites as `force_https = true`, so they render 307; browsers that already cached a 301 keep it until their cache is cleared |
| Fix recipe (3.0) | none: the delta is the design. Recommended: the migrate plan notes the cached-301 effect for each forced site |
| Proving test | `TestLedger_HTE10` in `internal/hte/render` (golden `vhost-https-forced.conf` holds `R=307`); `hte-e2e`: a request from a non-bypassed address (HTE-52) gets 307. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), step "forced HTTPS without the bypass" (307 to `https://<site>/`, from loopback too). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-11"></a>
#### HTE-11 · `modify` regenerates the whole vhost and drops manual edits

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (modify regenerating the vhost); hte-cli dossier F9 |
| Area | Correctness |
| hte-cli location | `app/Commands/ModifyCommand.php:602-604,643-650` |
| Observed behavior | `getConf()` rewrites the file from the template: a hand-added `Header set X-Manual-Edit` was lost, a V1 marker became V2, and the pool owner became the current sudo user |
| Reproduction | `printf '    Header set X-Manual-Edit 1\n' >> $SB/etc/apache2/sites-available/001-a.test.conf && work/hte/run.sh $SB modify --domain=a.test --profile=staging -n && grep -c X-Manual-Edit $SB/etc/apache2/sites-available/001-a.test.conf` (site from HTE-01; prints 0) |
| Severity | medium |
| Public-contract impact | yes (where manual edits go) |
| rc.A behavior | Rendered files are artifacts, rebuilt on every change and never edited. User directives go into `sites/<domain>.apache.conf` (included in every vhost of the site), `apache.conf` (server level) and the directory `php/<X.Y>/` (its `*.ini` files are read by that version's FPM master after the system ones, through `PHP_INI_SCAN_DIR=<system scan dir>:<overlay dir>` written out in full; lead decision, 2026-10-18); the renderer includes or passes them and never writes them. The site's `[php_ini]` is rendered into its pool: a key the profile sets replaces the profile line with the same admin kind, other keys are `php_value`/`php_flag`, boolean words are bare flags, other values double-quoted; control characters, `"`, `\` and `${` are refused (`invalid_state`). `site modify` changes only the TOML. The design does not say what migrate does with hand edits in a legacy vhost |
| Fix recipe (3.0) | none for the overlay model. Operator choice for hand edits found by migrate (lines that are not in the variant's template): (a) report them in the plan only; (b) copy them into `sites/<domain>.apache.conf`, validated by the config test, and report them (recommended) |
| Proving test | `TestLedger_HTE11` in `internal/hte/render` (overlay bytes unchanged after modify of every field); `TestPoolPHPIni` and `TestManifest` in `internal/hte/render` (php_ini rules, no overlay in pools, the full `PHP_INI_SCAN_DIR`); `internal/hte/migrate` (V2 fixture plus one added line → the chosen outcome); `hte-e2e`: a value in `php/<X.Y>/global.ini` shows in `phpinfo()`. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), step "php ini and the global overlay" (`ini_get` shows the `global.ini` and `[php_ini]` values). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-12"></a>
#### HTE-12 · Partial failures exit 0 with "successfully!"

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (exit 0 on partial failure); hte-cli dossier F10 |
| Area | Contract |
| hte-cli location | `app/Commands/ModifyCommand.php:604-681`; `app/Commands/RemoveCommand.php:166-263`; `app/Commands/FixCommand.php:182-212`; `app/Commands/HostsCommand.php:196,236,287`; `app/Commands/DetailsCommand.php:130-135`; `app/CommandWrapper.php:69-80` (`handle(): void`) |
| Observed behavior | Every failure after the pre-checks is a warning and the command exits 0 ("VirtualHost x removed successfully!" with a failed restart). `details --domain=unknown` and `hosts` failures also exit 0; `modify` with nothing to change exits 0 |
| Reproduction | `HTE_FAIL='restart' work/hte/run.sh $SB remove --domain=a.test --force -n` (prints `[exit=0]`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | A command exits 0 only when every step it ran succeeded; otherwise the first failure's envelope, exit 1, and the rollback of HTE-09. Unknown sites fail with `site_not_found` |
| Fix recipe (3.0) | none for failures. Operator choice for `site modify` with no effective change: (a) exit 0, `{"changed":false}` in `--json`, one stderr note (recommended, idempotent for scripts); (b) fail with `unexpected_argument`-style RETRY (hte-cli's message, non-zero) |
| Proving test | `TestLedger_HTE12` in `internal/hte` (each step failing in turn → non-zero exit, no "success" text on stdout); `testdata/script/hte_modify_noop.txtar` (the chosen no-op contract). The engine test is `TestLedger_HTE12` in `internal/hte` (a failing config test fails `site create` and `site modify` and puts the state back; a no-op modify is `changed=false` and runs nothing); the txtar waits for the CLI. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-13"></a>
#### HTE-13 · Numbered vhost file names, index counting foreign files, renumbering after remove

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (numbered file names and renumbering); hte-cli dossier F11 |
| Area | Correctness |
| hte-cli location | `app/Services/ApacheManager.php:107-111,164-180,202-245`; `app/Commands/RemoveCommand.php:243-259` |
| Observed behavior | Files are `{NNN}-{domain}.conf`; the next index is the max over every `*.conf` in sites-available, `000-default.conf` included. `remove` renames the rest to `001…` (`002-two` → `001-two`), replaces the relative `a2ensite` symlink with an absolute one, and does it after the Apache restart, so the running config and the files disagree |
| Reproduction | `work/hte/run.sh $SB remove --domain=one.test --force -n && ls -l $SB/etc/apache2/sites-available $SB/etc/apache2/sites-enabled` (sandbox with `one.test`, `two.test`, `three.test`) |
| Severity | low |
| Public-contract impact | yes (file names) |
| rc.A behavior | No numbers anywhere: `sites/<domain>.toml`, `render/current/sites/<domain>.conf`, `fpm/<X.Y>/pools/<domain>.conf`. Include order is the sorted domain list, so rendering is deterministic. Nothing is renamed on remove; `--no-normalize` has no counterpart. migrate ignores the legacy index and reports gaps only as information |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE13` in `internal/hte/render` (three sites, remove the first: the other two rendered files byte-identical, names unchanged) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-14"></a>
#### HTE-14 · Unused 128-character passphrase next to `-nodes`

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (fake passphrase with -nodes); hte-cli dossier F12a |
| Area | Security |
| hte-cli location | `app/Services/SslCertManager.php:59,100` |
| Observed behavior | The script exports a random `PASSPHRASE`, passes `-passin pass:$PASSPHRASE` and `-nodes`: the key is unencrypted and the passphrase is noise that suggests protection |
| Reproduction | `grep -n 'PASSPHRASE\|-nodes' go-impl/testdata/hte/legacy/cert-script/sscert_site.test.sh.fixture` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The key is written unencrypted on purpose (a userland master cannot prompt for it), protected by file mode 0600 in a 0700 directory owned by the user; no passphrase code exists |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE14` in `internal/hte/certs` (key file is a plain `PRIVATE KEY` PEM, mode 0600, parent 0700) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-15"></a>
#### HTE-15 · Weak `SSLCipherSuite`, no `SSLProtocol`

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (weak cipher string); hte-cli dossier F12b |
| Area | Security |
| hte-cli location | `app/Services/ApacheManager.php:478` |
| Observed behavior | Every HTTPS vhost carries `SSLCipherSuite ALL:!ADH:!EXPORT56:RC4+RSA:+HIGH:+MEDIUM:+LOW:+SSLv2:+EXP` and no `SSLProtocol`, overriding the distro's `ssl.conf` defaults for the cipher list |
| Reproduction | `grep -n SSLCipherSuite go-impl/testdata/hte/legacy/templates/v2-vhost-https.conf` |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Managed vhosts carry no cipher or protocol directive. The server level sets the Debian `ssl.conf` equivalents: `SSLProtocol all -SSLv3 -TLSv1 -TLSv1.1`, `SSLCipherSuite HIGH:!aNULL`, `SSLHonorCipherOrder off`, `SSLSessionCache shmcb:<run>/ssl_scache(512000)`, `SSLSessionCacheTimeout 300`. A raw imported vhost keeps its own SSL directives (only paths are rewritten) |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE15` in `internal/hte/render` (golden `httpd.conf` and vhosts: no `SSLCipherSuite` in any managed vhost); `hte-e2e`: a TLS 1.1 handshake to `127.0.0.1:443` is refused. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), step "TLS, HTTP/2 and the certificate" (a TLS 1.1 handshake is refused). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-16"></a>
#### HTE-16 · Apache 2.2 `order allow,deny` / `allow from all` beside `Require all granted`

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (Apache 2.2 directives); hte-cli dossier F12c |
| Area | Correctness |
| hte-cli location | `app/Services/ApacheManager.php:429-430,463-464` |
| Observed behavior | Both `<Directory>` blocks mix the 2.2 access directives (they need `mod_access_compat`) with the 2.4 `Require all granted` |
| Reproduction | `grep -n 'order allow\|allow from\|Require' go-impl/testdata/hte/legacy/templates/v2-vhost-https.conf` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Managed vhosts use `Require all granted` only. `access_compat` stays in the userland module list solely for raw imported vhosts that still use `order`/`allow` |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE16` in `internal/hte/render` (no `order`/`allow from` in any managed golden vhost) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-17"></a>
#### HTE-17 · A generated certificate script runs as root from `$TMPDIR`

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.7 defect (generated script run as root from /tmp); hte-cli dossier F12d |
| Area | Security |
| hte-cli location | `app/Commands/CreateCommand.php:197-213`; `app/Commands/ModifyCommand.php:617-625`; `app/Services/SslCertManager.php:62` |
| Observed behavior | `tempnam(sys_get_temp_dir(), "sscert_{d}")`, chmod 0755 (world-readable), executed by root, then deleted; the script itself uses a hardcoded `/tmp/openssl.cnf.XXXXXX`. tempnam's O_EXCL and the sticky `/tmp` prevent substitution, so the risk is low, but the pattern is the one PRD 6.7 forbids |
| Reproduction | `work/hte/run.sh $SB create --domain=a.test --docroot=$SB/d --phpver=8.3 --ssl=y --forcessl=n --hosts=n --profile=dev -n && grep sscert_ $SB/exec.log` (the harness keeps a copy in `$SB/last-cert-script.sh`) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Certificates are made by the openssl CLI as the user (HTE-39), with a fixed argument vector; no script is generated. Root runs only one `sudo <gash binary> hte __root` session per `install`, `uninstall` and `migrate --apply`: the plan the user saw arrives on stdin first, then root runs its operations by id, each once, from a closed typed set, each checked against root's own reading of the system (a vhost or pool to archive must be one `legacy.Scan` finds, a key to copy must be the `SSLCertificateKeyFile` of such a vhost); and the fixed hosts helper |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE17` in `internal/hte/rootops` (an op not in the plan, run twice, or naming a file root does not find in the system configuration, is refused; a bad plan is refused before anything runs) and `TestOpenSSLArgv` in `internal/hte/certs` (the only children of create/renew are openssl genpkey and req, fixed argv, no secret, new session) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-18"></a>
#### HTE-18 · The pool's `user`/`group` follow the invoker; `modify` as root writes `user = root`

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | hte-cli dossier I1; PRD 6.5 behavior, changed (pool socket owned by the user, mode 0660) |
| Area | Correctness |
| hte-cli location | `app/Logic/Preprocessors/PhpProfiles.php:183-188`; `app/Commands/ModifyCommand.php:118-121,643-650` (no `requireNonRootUser()`) |
| Observed behavior | Pools carry `user`, `group`, `listen.owner`, `listen.group` = the invoking sudo user; `modify` rewrites them with the current invoker, and as plain root writes `user = root`, which php-fpm refuses, taking down every site of that PHP version |
| Reproduction | `HSUDO= work/hte/run.sh $SB modify --domain=a.test --profile=staging -n && grep -n '^user\|^listen.owner' $SB/etc/php/8.3/fpm/pool.d/a.test.conf` |
| Severity | high |
| Public-contract impact | yes (pool settings) |
| rc.A behavior | Pools have no `user`, `group`, `listen.owner` or `listen.group`: the FPM master runs as the user, so workers and socket belong to the user; `listen.mode = 0660` kept. The userland Apache runs as the same user and reaches the socket without group tricks |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE18` in `internal/hte/render` (golden pools per profile: none of the four directives); `hte-e2e`: `stat` of the socket shows the user and 0660, `ps` shows workers as the user. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), step "logs stay out of the document root" (the masters and workers are the user's processes). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-19"></a>
#### HTE-19 · Who the user is comes from `USER`/`SUDO_USER`, not from the process

| Field | Value |
|---|---|
| Section | HTE design delta · Decision for the operator |
| Origin | hte-cli dossier I2 |
| Area | Security |
| hte-cli location | `app/Services/UserContext.php:132-164`; `app/CommandWrapper.php:190-213`; `app/Commands/CreateCommand.php:100-103` |
| Observed behavior | `USER=root` plus `SUDO_USER=x` means "sudo by x"; the euid and `SUDO_UID` are never read. `sudo -E` is refused; `USER=root SUDO_USER=alice` from a root shell writes pools for alice. create refuses direct root, modify and remove do not |
| Reproduction | code reading (`app/Services/UserContext.php:134-150`); the harness itself sets the identity through these two variables (`work/hte/run.sh`) |
| Severity | medium |
| Public-contract impact | yes (no sudo for site commands) |
| rc.A behavior | Site, service, php, apache, cert and hosts commands run as the user with no sudo; the user is the process's own uid (`os.Getuid`) and XDG directories. Root is reached only through `sudo <gash binary> hte __root <op>` (HTE-17) and the hosts helper, whose sudoers line names the user. The design does not say what `gash hte site ...` does when euid is 0 |
| Fix recipe (3.0) | Operator choice for euid 0: (a) refuse every `gash hte` command except `install`, `uninstall`, `migrate` and `doctor` with a new code `root_not_allowed` (FATAL, hint: "Run gash hte as your own user; it asks for sudo itself when a step needs root") (recommended); (b) let root keep its own userland stack under `/root` |
| Proving test | `TestLedger_HTE19` in `internal/hte` (identity from the uid fake, environment variables ignored; euid 0 → the chosen outcome). Implemented with option (a) in `internal/hte` (`Engine.CheckIdentity`); one engine choice to confirm: `migrate --apply` as euid 0 is `root_not_allowed` too (the dry run is allowed), since the sites go into the invoking user's own stack |
| Status | accept: done (rc.A); TestLedger_HTE19 extended to every engine entry point (c0583ec) |

<a id="hte-20"></a>
#### HTE-20 · `--domain` is used verbatim; duplicates are found case-sensitively

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | hte-cli dossier I3 (case-insensitive domains, domain lowercasing) |
| Area | Correctness |
| hte-cli location | `app/Commands/CreateCommand.php:117-124,165-168`; `app/Services/ApacheManager.php:153` |
| Observed behavior | Only a prompted domain is lowercased and stripped of `http://`, `https://`, `www.`; a `--domain` value is kept as typed: `--domain=WWW.Upper.Test` wrote `002-WWW.Upper.Test.conf` and `ServerAlias www.WWW.Upper.Test`. The duplicate check is case-sensitive, so `App.test` and `app.test` coexist |
| Reproduction | `work/hte/run.sh $SB create --domain=WWW.Upper.Test --docroot=$SB/d --phpver=8.3 --ssl=n --forcessl=n --hosts=n --profile=dev -n && ls $SB/etc/apache2/sites-available` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | `--domain` is lowercased, then checked against a hostname grammar (the design gives it for the hosts helper; site validation must use the same one so that every site fits in the block: labels `[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?`, at least two labels, at most 253 bytes, no wildcard, no trailing dot); a scheme or path fails with `invalid_domain` (no silent stripping). Identity and duplicate checks compare lowercased names (`site_exists`). migrate reports case-only duplicates and imports one only with `--site` |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE20` in `internal/hte/state` (`Shop.Test` stored as `shop.test`; creating `shop.test` afterwards → `site_exists`); `internal/hte/migrate` (two vhosts `App.test`/`app.test` → both reported as duplicates) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-21"></a>
#### HTE-21 · IP addresses are accepted as domains

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | hte-cli dossier I4 (IP addresses refused as domains) |
| Area | Correctness |
| hte-cli location | `app/helpers.php:22-28` (`FILTER_VALIDATE_IP` since v1.0.8, `3fb5050`); `app/Services/SslCertManager.php:32-40` |
| Observed behavior | IPv4 gives `ServerName 127.0.0.1`, `ServerAlias www.127.0.0.1`, a hosts line `127.0.0.1    127.0.0.1 www.127.0.0.1` and a DNS SAN holding an IP. IPv6 passes validation, then crashes in the certificate step (uncaught `InvalidArgumentException`) after the vhost and pool are written (`001-::1.conf`, `::1.conf` left behind) |
| Reproduction | `work/hte/run.sh $SB create --domain=::1 --docroot=$SB/d --phpver=8.3 --ssl=y --forcessl=y --hosts=n --profile=dev -n; ls $SB/etc/apache2/sites-available` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | An IP address fails with `invalid_domain` (RETRY, hint: "a site name is a hostname such as app.test"). migrate reports IP-named vhosts and does not import them |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE21` in `internal/hte/state` (`127.0.0.1`, `::1`, `[::1]` → `invalid_domain`); `internal/hte/migrate` (a vhost named `127.0.0.1` → reported, not imported) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-22"></a>
#### HTE-22 · A bare `/etc/php/X.Y` directory counts as an installed PHP-FPM version

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | hte-cli dossier I5 (PHP version discovery requiring a working FPM binary) |
| Area | Correctness |
| hte-cli location | `app/Services/PhpFpmManager.php:24-28,43-57`; `app/Services/SystemFilesystem.php:184-211` |
| Observed behavior | Any directory named like a version under `/etc/php` is "installed", `fpm/` or the unit are not checked; `/etc/php/8.4/cli` alone made 8.4 the create default, then `systemctl restart php8.4-fpm` failed after the pool was written |
| Reproduction | `mkdir -p $SB/etc/php/8.4/cli && work/hte/run.sh $SB create --domain=a.test --docroot=$SB/d --ssl=n --forcessl=n --hosts=n --profile=dev -n && ls $SB/etc/php/8.4` |
| Severity | medium |
| Public-contract impact | yes (which versions are offered) |
| rc.A behavior | A version 5.6 to 8.5 counts only when its FPM binary runs and `<bin> -v` reports that version, and its `php.ini` exists. Probed layouts: Debian/Ubuntu (`/usr/sbin/php-fpmX.Y`), Fedora distro and Remi, Arch `php`/`php-legacy` and AUR, Homebrew `php@X.Y`. Cached by binary path, size and mtime. `php versions` lists them; `doctor` prints the platform's install command for a missing one |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE22` in `internal/hte/discover` (sysroot with `etc/php/8.4/cli` only → 8.4 not listed; fake binary answering `PHP 8.3.x (fpm-fcgi)` → 8.3 listed; cache invalidated by an mtime change) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-23"></a>
#### HTE-23 · `hte-cli fix` has no counterpart; its "restarted successfully" was always printed

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | hte-cli dossier I6; Design choice |
| Area | Contract |
| hte-cli location | `app/Commands/FixCommand.php:85-249` (:192 always reports success); `app/Services/ApacheManager.php:250-349` |
| Observed behavior | `fix` rewrites the force block of V1 and broken-bypass vhosts to the bypass form; after a failing `systemctl restart` it still prints `Apache2 restarted successfully` and exits 0 |
| Reproduction | `cp -r go-impl/testdata/hte/legacy/V1/etc $SB/ && HTE_FAIL='restart apache2' work/hte/run.sh $SB fix --force -n` |
| Severity | low |
| Public-contract impact | yes (command removed) |
| rc.A behavior | No `fix` command: managed sites are rendered from state, so there is no stale force block to repair. migrate recognizes V1, V1-fixed and Broken-bypass force blocks (with `fix`'s two patterns, `ApacheManager.php:301,319`) and imports them as `force_https = true` |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE23` in `internal/hte/migrate` (V1, V1-fixed and both Broken-bypass files → `force_https = true`; the rendered vhost equals the golden forced vhost) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-24"></a>
#### HTE-24 · `/etc/hosts` lines tagged `# hte-cli` are edited line by line

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | hte-cli dossier I7; Design choice (hosts block with markers) |
| Area | Correctness |
| hte-cli location | `app/Services/HostsManager.php:61-225`; `app/Contracts/HostsManagerInterface.php:13` |
| Observed behavior | `add` appends `"\n127.0.0.1    d www.d # hte-cli\n"` with `FILE_APPEND` (no atomic write) and flushes the DNS cache; `remove` drops every line holding the token and the marker but leaves the blank lines; `getEntry` takes the first line holding the token, managed or not, so an unrelated `10.0.0.5 two.test` makes `add` report "not managed"; `hosts remove --domain=www.two.test` deletes the managed `two.test` line; `sync` drops every marker line and appends without blank lines |
| Reproduction | `cp -r go-impl/testdata/hte/legacy/V2/etc $SB/ && work/hte/run.sh $SB hosts remove --domain=www.http.test -n && cat -A $SB/etc/hosts` (dossier B.6, observed with `two.test`) |
| Severity | low |
| Public-contract impact | yes (hosts file format; `hosts list\|sync\|add\|remove` become `hosts list\|apply\|check\|windows`) |
| rc.A behavior | One region between `# >>> gash hte >>>` and `# <<< gash hte <<<`, appended when absent, regenerated from state by the root helper `gash-hosts-helper apply` (stdin grammar: at most 4096 lines `ADDRESS NAME...`, ADDRESS exactly `127.0.0.1` or `::1`; anything else exit 2, nothing written), written to a temporary file in `/etc`, fsynced, renamed with the original owner and mode. Lines outside the block are never read for decisions nor changed. Applied on every site change, on `service start` and on `doctor`. No per-domain `add`/`remove` |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE24` in `internal/hostshelper` (every refusal path: bad address, wildcard, uppercase, injected newline, 4097 lines; bytes outside the block unchanged; owner and mode kept); `hte-e2e`: helper through the sudoers line. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), step "hosts drift is repaired" (the helper through the sudoers line, its refusals, an extra argument refused by sudo). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-25"></a>
#### HTE-25 · `remove` leaves the `.info` file; disabling SSL leaves all three certificate files

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | hte-cli dossier I8 |
| Area | Correctness |
| hte-cli location | `app/Commands/RemoveCommand.php:192-207`; `app/Commands/ModifyCommand.php:566-685` |
| Observed behavior | remove deletes the `.crt` and `.key` paths parsed from the vhost, never `{domain}.info`; `modify` from SSL on to off rewrites the vhost and keeps `.crt`, `.key` and `.info` |
| Reproduction | `work/hte/run.sh $SB remove --domain=a.test --force -n && ls $SB/etc/apache2/certs-selfsigned` (site created with `--ssl=y`) |
| Severity | low |
| Public-contract impact | yes (no `.info` file; `cert show` replaces it) |
| rc.A behavior | No `.info` file is written (`cert show --domain D` prints subject, SAN, validity and fingerprint). `site remove` deletes the whole `certs/<domain>/` directory. The design does not say what `site modify --no-https` does with the certificate |
| Fix recipe (3.0) | Operator choice for `--no-https`: (a) keep `certs/<domain>/` so that `--https` later reuses it, and let `doctor` list certificates of HTTP-only sites (recommended); (b) delete it at once |
| Proving test | `TestLedger_HTE25` in `internal/hte/certs` (remove → directory gone, other sites untouched; `--no-https` → the chosen outcome) |
| Status | accept: done e3e2f15 |

<a id="hte-26"></a>
#### HTE-26 · The `.htaccess` HTTPS-redirect warning appears only in `fix` and misses the X-Forwarded-Proto form

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | hte-cli dossier I9; PRD 6.5 behavior, changed (warning when the site's `.htaccess` has its own HTTPS redirect) |
| Area | Correctness |
| hte-cli location | `app/Services/ApacheManager.php:354-381`; `app/Commands/FixCommand.php:104-120,214-249` |
| Observed behavior | The regex `/RewriteCond\s+.*(%{HTTPS}\|%{SERVER_PORT}\|HTTP_X_FORWARDED_PROTO).*(off\|!=on\|80\|http).*?\n\s*RewriteRule.*https:\/\//is` misses `RewriteCond %{HTTP:X-Forwarded-Proto} !https` (the form Laravel and most guides use), and its `/s` flag lets `.*` span unrelated lines. Only `fix` calls it; create, modify and details never warn |
| Reproduction | dossier B.2: the regex applied to an `.htaccess` holding `RewriteCond %{HTTP:X-Forwarded-Proto} !https` then `RewriteRule ^ https://%{HTTP_HOST}%{REQUEST_URI} [L,R=301]` does not match |
| Severity | low |
| Public-contract impact | yes (new warnings on stderr and in `--json`) |
| rc.A behavior | `site create`, `site modify`, `site show`, `doctor` and `migrate` warn on stderr, and add a `warnings` array in `--json`, when the docroot's `.htaccess` has a `RewriteRule` or `Redirect`/`RedirectMatch` to `https://`, including the `%{HTTP:X-Forwarded-Proto}` form. The warning names the file and line. Never an error |
| Fix recipe (3.0) | none: the delta is the design. Implementation note: match per rule (the `RewriteCond` lines directly above one `RewriteRule`), not with a multi-line `.*` |
| Proving test | `TestLedger_HTE26` in `internal/hte/htaccess` (table: the hte-cli forms, the X-Forwarded-Proto form, `Redirect permanent / https://...`, an unrelated `RewriteCond` followed later by an unrelated rule → no warning) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-27"></a>
#### HTE-27 · `create` restarts Apache before PHP-FPM

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | hte-cli dossier I10 |
| Area | Correctness |
| hte-cli location | `app/Commands/CreateCommand.php:230-241`; `app/Commands/RemoveCommand.php:209-259` |
| Observed behavior | Apache is restarted first and serves the new vhost while its FPM socket does not exist yet (503 for a moment); remove renumbers files after the restart |
| Reproduction | `work/hte/run.sh $SB create --domain=a.test --docroot=$SB/d --phpver=8.3 --ssl=n --forcessl=n --hosts=n --profile=dev -n && grep systemctl $SB/exec.log` (apache2 line before php8.3-fpm) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | `service start` starts each FPM master, waits for its sockets, then starts Apache and waits for the ports. The design's `service reload` row lists "Apache `-k graceful`, FPM `SIGUSR2`" without an order |
| Fix recipe (3.0) | Design text to amend (no operator choice): reload FPM first (`SIGUSR2`, wait until every socket of the new generation accepts), then Apache `-k graceful`; on remove, Apache first, then FPM |
| Proving test | `TestLedger_HTE27` in `internal/hte/service` (runner fake records FPM reload and socket wait before the Apache graceful on create, the reverse on remove) |
| Status | accept: done (rc.A); TestLedger_HTE27 |

<a id="hte-28"></a>
#### HTE-28 · Discovery recurses and stops silently at the first unreadable directory

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | hte-cli dossier I11 |
| Area | Correctness |
| hte-cli location | `app/Services/SystemFilesystem.php:155-179` |
| Observed behavior | `findByExtension` walks sites-available and the whole `/etc/php` tree recursively; an exception from an unreadable directory ends the walk and returns what was found so far, without a message |
| Reproduction | code reading (`catch (\Exception $e) { // Silently ignore unreadable directories }`) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Managed sites are read from `sites/*.toml`; nothing is discovered. migrate walks the platform's vhost directories recursively, as hte-cli did; the design does not say what it does with an unreadable entry |
| Fix recipe (3.0) | Operator choice for migrate: (a) list every unreadable path in the plan as a warning and go on (recommended; the root step can read what the user cannot, except the expected 0400 keys, which are handled by HTE-67); (b) refuse the plan (STOP) until every path is readable |
| Proving test | `TestLedger_HTE28` in `internal/hte/migrate` (sysroot with a mode-000 subdirectory → the chosen outcome; the other vhosts still classified) |
| Status | accept: done (rc.A); TestLedger_HTE28, design text f3dd534 |

<a id="hte-29"></a>
#### HTE-29 · Trailing slash and relative document roots are written as typed

| Field | Value |
|---|---|
| Section | HTE design delta · Decision for the operator |
| Origin | hte-cli dossier I12 |
| Area | Correctness |
| hte-cli location | `app/Commands/CreateCommand.php:125-129`; `app/Services/ApacheManager.php:406,426` |
| Observed behavior | `--docroot=rel/pub/` writes `DocumentRoot rel/pub/` (resolved by Apache against its ServerRoot, not the user's directory) and `<Directory rel/pub//>` |
| Reproduction | `mkdir -p $SB/cwd/rel/pub && work/hte/run.sh $SB create --domain=rel.test --docroot=rel/pub/ --phpver=8.3 --ssl=n --forcessl=n --hosts=n --profile=dev -n && grep -n 'DocumentRoot\|Directory ' $SB/etc/apache2/sites-available/001-rel.test.conf` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | The stored docroot is absolute and cleaned (`filepath.Clean`, no trailing slash); `<Directory "<docroot>/">` gets exactly one slash. It must exist and be a directory (`docroot_not_found`, `invalid_docroot`). The design says "absolute" but not what happens to a relative argument |
| Fix recipe (3.0) | Operator choice: (a) resolve a relative argument against the current directory and store the absolute path (recommended: `--docroot public` from a project root is the common call); (b) refuse it with `invalid_docroot` (hint: "give an absolute path") |
| Proving test | `TestLedger_HTE29` in `internal/hte/state` (`/x/pub/` → `/x/pub`; `rel/pub` → the chosen outcome; golden `<Directory "/x/pub/">`) |
| Status | accept: done (rc.A); TestLedger_HTE29 |

<a id="hte-30"></a>
#### HTE-30 · "Forced" is a substring test on a comment

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | hte-cli dossier I13 |
| Area | Correctness |
| hte-cli location | `app/Services/ApacheManager.php:102` |
| Observed behavior | `forcedssl` is true when `# Auto-generated Force HTTPS` appears anywhere; a vhost whose rewrite lines were commented out by hand, marker left, stays "forced" |
| Reproduction | `cp -r go-impl/testdata/hte/legacy/V2/etc $SB/ && sed -i 's/^RewriteRule/#RewriteRule/' $SB/etc/apache2/sites-available/003-forced.test.conf && work/hte/run.sh $SB details -n` (Force SSL still "Yes") |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Managed sites store `force_https` in the TOML. migrate's step 4 reads "forced: either force comment", the hte-cli rule |
| Fix recipe (3.0) | Operator choice for migrate: (a) the comment alone, as hte-cli (design today); (b) the comment and an active `RewriteRule ... https://` in the port-80 vhost, a mismatch reported and imported as not forced (recommended) |
| Proving test | `TestLedger_HTE30` in `internal/hte/migrate` (V2 `003-forced.test.conf` with the rule commented → the chosen outcome) |
| Status | accept: done (rc.A); TestLedger_HTE30 |

<a id="hte-31"></a>
#### HTE-31 · hte-cli 1.1.0 crashes on a marked vhost without a parseable socket

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | hte-cli dossier I14 |
| Area | Correctness |
| hte-cli location | `a195f84:app/Services/ApacheManager.php:73`; `a195f84:app/Services/PhpFpmManager.php:70` (`getConfByDomain(string $domain)` called with null); 1.1.1 skip at `app/Services/ApacheManager.php:81-83` |
| Observed behavior | 1.1.0 dies with a TypeError on any marker-bearing file whose `SetHandler` does not match; 1.1.1 skips the file silently. Users still on 1.1.0 may have such files |
| Reproduction | fixture `Malformed/` and `Malformed/expected-discovery-1.1.0.crash.txt` (1.1.0 phar on that tree) |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | migrate classifies the file as Malformed: reported with the reason (no `SetHandler`, empty domain, default socket), not imported, never modified. If it is enabled in the system Apache, `install` and `migrate --apply` refuse to stop the system Apache until it is excluded. Option (a) was chosen (lead decision, 2026-10-18): `--exclude` takes a domain (every vhost serving it) or an absolute vhost file path as the plan lists it (that file, whatever its class: Malformed files, Debian's `000-default.conf`, one of two duplicates). Nothing is excluded implicitly, not even `000-default.conf`; each enabled vhost that blocks the stop is listed with the exact `--exclude PATH` to add (shell-quoted when needed), in the text plan and as `blocking_stop[].exclude` in `--json`. A relative path is `invalid_domain`, an unknown path `site_not_found`, and `--site D` whose every vhost is excluded `conflicting_options` |
| Fix recipe (3.0) | Design gap, operator choice: (a) `--exclude` also accepts a vhost file path (recommended); (b) Malformed files never block the stop, and the plan says they will no longer be served |
| Proving test | `TestLedger_HTE31` and `TestExcludePath` in `internal/hte/migrate` (every file of `Malformed/` → its class and reason; `025-valid.test` imported; bytes of the tree unchanged; `--exclude PATH` unblocks the stop and resolves duplicates) |
| Status | accept: done (rc.A); TestLedger_HTE31, TestExcludePath |

<a id="hte-32"></a>
#### HTE-32 · User-owned Apache and PHP-FPM masters instead of the system services

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Contract |
| hte-cli location | `app/Services/ServiceManager.php:27-57` (`systemctl restart apache2.service`, `systemctl restart 'php{v}-fpm.service'`, `a2ensite`, `a2dissite`) |
| Observed behavior | Sites live in the distro's root-started `apache2.service` and `php{v}-fpm.service`; hte-cli needs sudo for every change and restarts those units |
| Reproduction | by design |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | One Apache master (distro or Homebrew `apache2`/`httpd` binary and modules) and one `php-fpm` master per PHP version in use, all started by `gash hte service start` as the user, daemonized, tracked by PID files in the runtime directory; `service stop` signals a PID only when its command line names gash's config path. No root process at runtime. `systemctl status apache2` no longer describes the sites; `install`/`migrate --apply` stop and disable the system units (HTE-64) |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE32` in `internal/hte/service` (a PID file pointing at a foreign process is never signalled); `hte-e2e`: masters and workers owned by the user, `service stop` leaves no process. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), step "stop, start, restart, status, logs" (no process of the stack after `service stop`). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-33"></a>
#### HTE-33 · Every file moves from `/etc` and `/var/run` to the user's XDG directories

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice (XDG locations) |
| Area | Contract |
| hte-cli location | `app/Providers/AppServiceProvider.php:270-312`; dossier B.1 |
| Observed behavior | vhosts `/etc/apache2/sites-available/{NNN}-{d}.conf` (+ `sites-enabled` symlink), pools `/etc/php/{X.Y}/fpm/pool.d/{d}.conf`, certificates `/etc/apache2/certs-selfsigned/{d}.{crt,key,info}`, sockets `/var/run/php/php{X.Y}-fpm-{d}.sock`, logs `${APACHE_LOG_DIR}/…` and the docroot |
| Reproduction | by design |
| Severity | medium |
| Public-contract impact | yes (file locations) |
| rc.A behavior | Settings `$XDG_CONFIG_HOME/gash/hte/hte.toml`; sites and overlays `$XDG_CONFIG_HOME/gash/hte/sites/`, `apache.conf`, `php/<X.Y>.ini`; rendered config `$XDG_STATE_HOME/gash/hte/render/gen-<n>/` + `render/current`; logs, receipt, audit log and lock under `$XDG_STATE_HOME/gash/hte/`; certificates `$XDG_DATA_HOME/gash/hte/certs/<domain>/`; runtime (PIDs, sockets, mutexes, TLS cache) `$XDG_RUNTIME_DIR/gash-hte/`, else `/tmp/gash-hte-<uid>/` (0700, owner and non-symlink checked). macOS uses the same XDG layout, not `~/Library`. Root-owned files only: helper, sudoers drop-in, sysctl/pf files, `/var/lib/gash-hte/` (receipt copy, backups, archive) |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE33` in `internal/platform` (XDG variables set and unset → the paths above; runtime fallback refused when it is a symlink or foreign-owned) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-34"></a>
#### HTE-34 · Declarative TOML state; rendered files are generations, never the source of truth

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice (TOML state) |
| Area | Contract |
| hte-cli location | `app/Services/ApacheManager.php:50-143` (state parsed back from the vhost files); `app/Services/ApacheManager.php:386-497` (marker `# Auto-generated by HTE-Cli on {date}`); `app/Logic/Preprocessors/PhpProfiles.php:182` (`; Profile:`) |
| Observed behavior | The vhost and pool files are both the configuration and the database: hte-cli reads PHP version, SSL, force, docroot and profile back from them with regexes; the marker carries the UTC creation date |
| Reproduction | by design |
| Severity | low |
| Public-contract impact | yes (where the configuration lives) |
| rc.A behavior | `sites/<domain>.toml` (keys `domain`, `aliases`, `docroot`, `php`, `profile`, `https`, `force_https`, `http2`, `enabled`, `kind`, `raw_conf`, `[php_ini]`) is the only source; rendering is a pure function of state, overlays and platform facts, golden-tested. Rendered vhosts carry ``# Managed by gash hte. Edit the site with `gash hte site modify`, or add directives to the overlay below.`` and pools `; Managed by gash hte. Profile: <p>`, no date (the audit log keeps history). The previous generation is kept for rollback |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE34` in `internal/hte/render` (same state twice → byte-identical generations; golden files per variant and platform) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-35"></a>
#### HTE-35 · FPM socket path and name change; the domain is no longer in the socket name

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice (domain as identity) |
| Area | Contract |
| hte-cli location | `app/Logic/Preprocessors/PhpProfiles.php:185`; `app/Services/ApacheManager.php:419,474` |
| Observed behavior | `listen = /var/run/php/php{X.Y}-fpm-{domain}.sock`, and the vhost's `SetHandler` names the same path |
| Reproduction | by design |
| Severity | low |
| Public-contract impact | yes (anything else pointing at the socket breaks) |
| rc.A behavior | `<runtime>/gash-hte/<X.Y>-<first 10 hex of sha256(domain)>.sock`, to stay under the 104-byte `sun_path` limit of macOS. The name is never parsed (identity is the TOML). migrate rewrites socket paths in raw vhosts it imports; a raw vhost that used another site's socket gets its own pool (reported) |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE35` in `internal/hte/render` (socket path length ≤ 103 bytes for a 253-byte domain under a 60-byte runtime directory; vhost and pool name the same path) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-36"></a>
#### HTE-36 · Apache listens on loopback only instead of on every interface

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.5 behavior, changed (binding on loopback only); hte-cli dossier G |
| Area | Security |
| hte-cli location | `app/Services/ApacheManager.php:402,441` (`<VirtualHost 127.0.0.1:80>`/`:443`); no `Listen` anywhere in `app/` |
| Observed behavior | The vhosts are IP-based on 127.0.0.1, but hte-cli never manages `Listen`, so Debian's `ports.conf` (`Listen 80`, `Listen 443`) binds every interface: LAN clients reach the system's default vhost, not the HTE sites |
| Reproduction | `grep -rn 'Listen' $HTE_CLI_SRC/app` (no match) |
| Severity | medium |
| Public-contract impact | yes (ports) |
| rc.A behavior | `Listen 127.0.0.1:<http>` and `Listen 127.0.0.1:<https>`, plus `[::1]` when IPv6 loopback is available; never a wildcard address. Nothing answers on a LAN or Docker-bridge address: a container that reached the host's Apache through its bridge IP (or `host.docker.internal`) no longer gets any response |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE36` in `internal/hte/render` (golden `httpd.conf`: only loopback `Listen` lines); `hte-e2e`: connect to the container's non-loopback address on 80/443 refused |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-37"></a>
#### HTE-37 · Vhosts also answer on `[::1]`

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice ([::1] vhosts); hte-cli dossier G (dead `::1` condition) |
| Area | Portability |
| hte-cli location | `app/Services/ApacheManager.php:396,402,441`; `app/Services/HostsManager.php:71` |
| Observed behavior | No IPv6 vhost exists, so the bypass condition `RewriteCond %{REMOTE_ADDR} !^::1$` is dead code; hosts lines are 127.0.0.1 only |
| Reproduction | `grep -n 'VirtualHost\|::1' go-impl/testdata/hte/legacy/templates/v2-vhost-forced.conf` |
| Severity | low |
| Public-contract impact | yes (addresses) |
| rc.A behavior | When IPv6 loopback exists, both vhosts of a site also bind `[::1]:<http>`/`[::1]:<https>` and the `::1` bypass condition is live. The helper accepts `::1` lines; the block holds `127.0.0.1` lines only, option (a) (lead decision, 2026-10-18) |
| Fix recipe (3.0) | Operator choice: (a) the block lists `127.0.0.1` lines only (hte-cli parity); (b) it also lists `::1` lines, only while the `[::1]` listener is configured (recommended: clients that prefer IPv6 then hit a live vhost instead of falling back) |
| Proving test | `TestLedger_HTE37` in `internal/hte/render` (golden vhosts with and without IPv6 loopback on the platform fake) and in `internal/hte/hosts` (only `127.0.0.1` lines) |
| Status | accept: done (rc.A); TestLedger_HTE37 |

<a id="hte-38"></a>
#### HTE-38 · HTTP/2 on cleartext (h2c), and a contradiction in the design sample

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | PRD 6.5 behavior, changed (HTTP/2); hte-cli dossier G |
| Area | Correctness |
| hte-cli location | `app/Services/ApacheManager.php:415,454` (`Protocols h2 http/1.1` in both vhosts) |
| Observed behavior | `h2` on the port-80 vhost has no effect (cleartext HTTP/2 needs `h2c`), so HTTP/2 is HTTPS-only in practice (Apache semantics, inferred in the dossier) |
| Reproduction | `grep -n Protocols go-impl/testdata/hte/legacy/templates/v2-vhost-https.conf` |
| Severity | low |
| Public-contract impact | yes (protocols offered on port 80) |
| rc.A behavior | The server level sets `Protocols h2 h2c http/1.1`; each site has `http2 = true` by default (`--no-http2`). The design's sample port-80 vhost still carries `Protocols h2 http/1.1`, which overrides the server value in that vhost, so h2c would stay off |
| Fix recipe (3.0) | Design text to amend (no operator choice): port-80 vhost `Protocols h2c http/1.1`, HTTPS vhost `Protocols h2 http/1.1`; `http2 = false` → `Protocols http/1.1` in both |
| Proving test | `TestLedger_HTE38` in `internal/hte/render` (golden vhosts for `http2` true and false); `hte-e2e`: `curl --http2-prior-knowledge http://<site>/` answers with HTTP/2, `curl --http2 https://<site>/` too. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), step "TLS, HTTP/2 and the certificate" (HTTP/2 over TLS). |
| Status | accept: done (rc.A); TestLedger_HTE38, E2E h2c check ff3eed4 |

<a id="hte-39"></a>
#### HTE-39 · Certificates: ECDSA P-256, 10 years, proper extensions, made by the openssl CLI

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice (ECDSA P-256 and 10-year validity instead of RSA 2048 30-year) |
| Area | Security |
| hte-cli location | `app/Services/SslCertManager.php:32,59-108` (`default_bits = 2048`, `-newkey rsa:2048 -sha256 -nodes`, `-days 10950`, SAN at :85-86) |
| Observed behavior | Self-signed RSA 2048/SHA-256 certificate valid 30 years, subject `C=IT, ST=Italy, L=Italy, O=Acme Corp, OU=The Dev Team, emailAddress=webmaster@{d}, CN={d}`, SAN `*.{d}` + `{d}`, no basicConstraints, plus a `.info` text dump |
| Reproduction | `openssl x509 -noout -subject -dates -ext subjectAltName,basicConstraints -in go-impl/testdata/hte/legacy/V2/etc/apache2/certs-selfsigned/https.test.crt` |
| Severity | low |
| Public-contract impact | yes (certificate content; anyone who trusted the old certificate must trust the new one) |
| rc.A behavior | `openssl genpkey` EC P-256 key (PKCS#8 `PRIVATE KEY`), `openssl req -x509` self-signed certificate from a generated request configuration, random serial, subject `CN=<domain>` only, SAN `<domain>`, `*.<domain>` (covers `www.`) and every alias (HTE-47), validity 10 years, `basicConstraints CA:FALSE`, `keyUsage digitalSignature`, `extKeyUsage serverAuth`. Private keys only through the openssl CLI, never read or parsed by gash (lead decision, 2026-10-18; PRD MUST NOT 6); certificates are parsed with `crypto/x509`. The pair is checked by comparing the public keys openssl prints for the certificate and derives from the key. `cert renew` replaces both and reloads; `doctor` warns 30 days before expiry. No local CA, no mkcert |
| Fix recipe (3.0) | none: the delta is the design. To verify in the WSL checklist (not run): a Windows browser accepts the CA:FALSE self-signed certificate once it is added to the Windows trust store |
| Proving test | `TestLedger_HTE39` in `internal/hte/certs` (parse the generated certificate: algorithm, curve, SAN set, validity, extensions); `hte-e2e`: `curl --cacert cert.pem https://<site>/` succeeds. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), step "TLS, HTTP/2 and the certificate" (verified with `cert.pem` as the only root). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-40"></a>
#### HTE-40 · One certificate directory per site, user-owned, new file names

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice (per-site cert directory); hte-cli dossier F4 |
| Area | Contract |
| hte-cli location | `app/Services/SslCertManager.php:39-42,92-108`; `app/Services/ApacheManager.php:479-480` |
| Observed behavior | All sites share `/etc/apache2/certs-selfsigned/` (0755, root): `{d}.crt` 0644, `{d}.key` 0400 root, `{d}.info` 0644 |
| Reproduction | `ls -l go-impl/testdata/hte/legacy/V2/etc/apache2/certs-selfsigned` |
| Severity | low |
| Public-contract impact | yes (file locations) |
| rc.A behavior | `$XDG_DATA_HOME/gash/hte/certs/<domain>/cert.pem` and `key.pem`; directory 0700, key 0600, owned by the user, written by atomic rename. The vhost names these paths through `quoteApacheArg`. migrate copies the legacy pair there (HTE-67); unmarked vhosts that used HTE certificates are imported as raw with a copy |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE40` in `internal/hte/certs` (modes, ownership on the sysroot fake, no temporary file left after an interrupted write) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-41"></a>
#### HTE-41 · Per-version session directory where the platform's default is not writable

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice (session.save_path on Fedora) |
| Area | Portability |
| hte-cli location | `app/Logic/Preprocessors/PhpProfiles.php:181-228` (no `session.save_path`) |
| Observed behavior | Pools inherit the distro's `session.save_path`; Debian's `/var/lib/php/sessions` (1733) is writable by any user, so hte-cli never needed one |
| Reproduction | by design |
| Severity | low |
| Public-contract impact | yes (pool settings) |
| rc.A behavior | When the platform default is not writable by the user (Fedora's `/var/lib/php/session` is group `apache`), the pool sets `session.save_path` to `$XDG_STATE_HOME/gash/hte/php/<X.Y>/sessions` and appends that directory to `open_basedir` in staging and hardened. Debian and Ubuntu pools are unchanged |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE41` in `internal/hte/render` (sysroot fakes: Debian → no `session.save_path`; Fedora → the per-version directory, also in `open_basedir`); `hte-e2e` Fedora: a session survives two requests |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-42"></a>
#### HTE-42 · The staging/hardened `open_basedir` list is Debian's on every platform

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Portability |
| hte-cli location | `app/Logic/Preprocessors/PhpProfiles.php:104` |
| Observed behavior | `open_basedir = {docroot}:/var/lib/php:/usr/lib/php:/usr/share/php:/tmp:/dev/urandom`, written for Debian/Ubuntu (session directory and PEAR/include paths) |
| Reproduction | `grep -n open_basedir go-impl/testdata/hte/legacy/templates/v2-pool-staging.conf` |
| Severity | low |
| Public-contract impact | yes (pool settings) |
| rc.A behavior | The design copies the list verbatim to every platform (plus HTE-41's session directory). On Fedora, Arch and Homebrew the PEAR and include directories live elsewhere (`/usr/share/pear`, `<prefix>/share/pear`, inferred), so code that includes from them fails under staging/hardened there. Logs outside the docroot need no entry: PHP checks `error_log` against `open_basedir` only when it is set at runtime, and writes it without the check (inferred from PHP's source; verified by the proving test) |
| Fix recipe (3.0) | Options: (a) keep the list verbatim on every platform (design today; same text as hte-cli); (b) keep the Debian list on Debian/Ubuntu and substitute each platform's include directories (read from the platform `php.ini` `include_path`) elsewhere (recommended: the profiles then mean the same thing everywhere) |
| Proving test | `TestLedger_HTE42` in `internal/hte/render` (golden staging pool per platform fake); `hte-e2e`: staging profile, a PHP warning reaches `logs/<domain>/php-error.log` |
| Status | accept: done (rc.A); TestLedger_HTE42 |

<a id="hte-43"></a>
#### HTE-43 · `slowlog` is set without `request_slowlog_timeout`, so it stays empty

| Field | Value |
|---|---|
| Section | HTE design delta · Decision for the operator |
| Origin | Design choice (slowlog inert as in hte-cli); hte-cli dossier C |
| Area | Correctness |
| hte-cli location | `app/Logic/Preprocessors/PhpProfiles.php:216` |
| Observed behavior | Every profile writes `slowlog = …`, none sets `request_slowlog_timeout` (default 0, disabled), so php-fpm never writes the slow log |
| Reproduction | `grep -n slowlog go-impl/testdata/hte/legacy/templates/v2-pool-*.conf` |
| Severity | low |
| Public-contract impact | yes (pool settings, log contents) |
| rc.A behavior | The design keeps the inert setting (path moved to `logs/<domain>/php-slow.log`, HTE-07), so the profile values stay identical to hte-cli's |
| Fix recipe (3.0) | Options: (a) keep it inert (design today; exact hte-cli values); (b) add `request_slowlog_timeout = 5s` to every profile so the log works (php-fpm then traces slow workers with ptrace, which a master running as the same user may do on its own children, inferred; check under Docker's default seccomp); (c) drop the `slowlog` line |
| Proving test | `TestLedger_HTE43` in `internal/hte/render` (golden pools hold the chosen lines); for (b) `hte-e2e`: a `sleep(6)` script leaves a backtrace in `php-slow.log` |
| Status | accept: done (rc.A); TestLedger_HTE43 |

<a id="hte-44"></a>
#### HTE-44 · Pools add `security.limit_extensions = .php`

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Security |
| hte-cli location | `app/Logic/Preprocessors/PhpProfiles.php:181-228` (not set) |
| Observed behavior | Pools use php-fpm's default, which also allows other extensions (`.phar` among them, per the php-fpm documentation, inferred) |
| Reproduction | `grep -c limit_extensions go-impl/testdata/hte/legacy/templates/v2-pool-dev.conf` (0) |
| Severity | low |
| Public-contract impact | yes (pool settings) |
| rc.A behavior | Every pool sets `security.limit_extensions = .php`. Managed vhosts only pass `\.php$` to FPM, so this matters for raw imported vhosts and overlays that route other extensions to the socket |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE44` in `internal/hte/render` (golden pools) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-45"></a>
#### HTE-45 · migrate leaves legacy `# hte-cli` hosts lines untouched

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice (legacy lines left untouched) |
| Area | Correctness |
| hte-cli location | `app/Services/HostsManager.php:71` (line format); fixture `V2/etc/hosts` |
| Observed behavior | Each hte-cli 1.1.x site has `127.0.0.1    d www.d # hte-cli` (1.0.x sites have none) |
| Reproduction | `cat -A go-impl/testdata/hte/legacy/V2/etc/hosts` |
| Severity | low |
| Public-contract impact | yes (hosts file) |
| rc.A behavior | migrate takes the names from the marked lines into the managed block (the names no imported site serves are kept in `hosts.toml`, so later applies keep them) and never deletes or rewrites a line outside it. The names then resolve twice to the same address, and keep resolving after `site remove` or `uninstall` |
| Fix recipe (3.0) | Options: (a) leave them untouched (design today); (b) also report, in `doctor`, legacy `# hte-cli` lines whose name is no longer a site, with the line to delete (recommended addition to (a)); (c) a root operation of `migrate --apply` removes them, recorded in the receipt with the prior bytes so that `uninstall` restores them |
| Proving test | `TestLedger_HTE45` in `internal/hte/migrate` (V2 hosts: block holds the three names, the legacy lines byte-identical); `internal/hte/doctor` for (b) |
| Status | accept: done (rc.A); TestLedger_HTE45 |

<a id="hte-46"></a>
#### HTE-46 · No DNS cache flush on Linux after a hosts change

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Portability |
| hte-cli location | `app/Services/HostsManager.php:212-225` |
| Observed behavior | After every hosts change: `systemd-resolve --flush-caches` and `nscd -i hosts` on Linux, `dscacheutil -flushcache` and `sudo killall -HUP mDNSResponder` on Darwin, errors discarded (`systemd-resolve` no longer exists on recent systemd, which ships `resolvectl`) |
| Reproduction | `grep -n 'flush\|nscd\|mDNS' $HTE_CLI_SRC/app/Services/HostsManager.php` |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | The helper flushes the resolver cache on macOS with two fixed commands; on Linux it runs nothing (systemd-resolved re-reads `/etc/hosts` when it changes; a host running nscd with hosts caching may answer with the old data until its cache expires, inferred) |
| Fix recipe (3.0) | Options: (a) nothing on Linux (design today); (b) the helper also runs the fixed command `nscd -i hosts` when an `nscd` binary exists at a fixed path, result ignored |
| Proving test | `TestLedger_HTE46` in `internal/hostshelper` (runner fake: the exact flush commands per platform, none on Linux for (a)) |
| Status | accept: done (rc.A); TestLedger_HTE46 |

<a id="hte-47"></a>
#### HTE-47 · `www.` alias rules, extra aliases, and their certificate coverage

| Field | Value |
|---|---|
| Section | HTE design delta · Decision for the operator |
| Origin | PRD 6.5 behavior, changed (www. alias); hte-cli dossier G, I3 |
| Area | Contract |
| hte-cli location | `app/Services/ApacheManager.php:405,444`; `app/Services/HostsManager.php:71`; `app/Commands/CreateCommand.php:117-124` |
| Observed behavior | Every site gets `ServerAlias www.{d}` and the hosts line lists `www.{d}`; there is no way to drop it or add another name. A prompted domain loses a leading `www.`, a `--domain` value keeps it, giving `ServerAlias www.www.x.test`. The SAN `*.{d}` covers `www.{d}` |
| Reproduction | `work/hte/run.sh $SB create --domain=www.x.test --docroot=$SB/d --phpver=8.3 --ssl=n --forcessl=n --hosts=n --profile=dev -n && grep -n ServerAlias $SB/etc/apache2/sites-available/001-www.x.test.conf` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | `aliases = ["www.<domain>"]` by default, `--no-www` drops it, `--alias N` (repeatable) adds names; aliases enter `ServerAlias` and the hosts block. The design does not say (1) what a domain starting with `www.` gets, nor (2) how an alias outside `*.<domain>` is covered by the certificate (SAN is `<domain>` and `*.<domain>` only) |
| Fix recipe (3.0) | Operator choice. (1): (a) keep the domain as given and add no `www.www.` alias (recommended); (b) refuse it with `invalid_domain`; (c) strip `www.` and keep it as an alias, as the hte-cli prompt did. (2): (a) put every alias in the SAN and reissue on alias changes (recommended); (b) accept only aliases under `<domain>` (`invalid_domain` otherwise) |
| Proving test | `TestLedger_HTE47` in `internal/hte/state` and `internal/hte/certs` (default alias, `--no-www`, `--alias other.test`, domain `www.x.test` → the chosen outcomes; SAN set equals the names served) |
| Status | accept: done 0424b69 |

<a id="hte-48"></a>
#### HTE-48 · Apache logs: no `${APACHE_LOG_DIR}`, per-site directory, a per-site error log, no rotation yet

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice (no `${APACHE_LOG_DIR}`); hte-cli dossier G |
| Area | Contract |
| hte-cli location | `app/Services/ApacheManager.php:411-412,450-451` |
| Observed behavior | `CustomLog ${APACHE_LOG_DIR}/{d} combined`, `${APACHE_LOG_DIR}/{d}-bytes_log`, `ssl-{d}`, `ssl-{d}-bytes_log` (variable from Debian's `envvars`); no `ErrorLog`, so errors go to the global log. The names do not end in `.log`, so Debian's logrotate rule for `/var/log/apache2/*.log` never rotates them (inferred) |
| Reproduction | `grep -n 'Log ' go-impl/testdata/hte/legacy/templates/v2-vhost-https.conf` |
| Severity | low |
| Public-contract impact | yes (log locations) |
| rc.A behavior | `$XDG_STATE_HOME/gash/hte/logs/<domain>/access.log` (combined), `bytes.log` (same format), `error.log` (new per-site `ErrorLog`), the HTTPS vhost with hte-cli's `ssl-` names and HTTPS bytes format, global `logs/error.log` with `LogLevel warn`. `${APACHE_LOG_DIR}` never appears (the userland master has no `envvars`); migrate rewrites it in raw vhosts. `service logs` tails them without a follow mode. The design gives no rotation and does not say whether `site remove` deletes `logs/<domain>/` |
| Fix recipe (3.0) | Operator choice. Rotation: (a) none (design today); (b) size-based rotation by gash at `service start`/`reload` (keep N old files); (c) piped `rotatelogs` from the platform's Apache utilities. On `site remove`: (a) keep the log directory (recommended, matches "never deletes the docroot" spirit); (b) delete it |
| Proving test | `TestLedger_HTE48` in `internal/hte/render` (no `${` in any managed golden file; exact log paths); `hte-e2e`: a request appears in `logs/<domain>/access.log`. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), step "logs stay out of the document root" (the request in `logs/<domain>/access.log`). |
| Status | accept: done (rc.A); TestLedger_HTE48 render and engine, design text 2e415ee |

<a id="hte-49"></a>
#### HTE-49 · Errors are the Gash envelope on stderr, not a box on stdout

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice (errors as the Gash envelope on stderr) |
| Area | Contract |
| hte-cli location | `app/CommandWrapper.php:69-80,147-150,220-234,287-305` |
| Observed behavior | Banner, progress lines, warnings, errors and the `*` error box all go to stdout (the box survives `2>/dev/null`); the banner prints raw `\e[1;97m` escapes even under `--no-ansi`; exit 1 only for "critical" errors |
| Reproduction | `work/hte/run.sh $SB details --domain=none.test -n 2>/dev/null` (banner and message on stdout, `[exit=0]`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Data on stdout; progress and warnings on stderr; an error is one JSON envelope line on stderr (`error`, `details`, `action`, `recoverable`, `hint`, codes from the HTE catalog of design section 9), exit 1. No banner. Color only per the Gash gate (TTY, `NO_COLOR`, `GASH_NO_COLOR`, `GASH_HEADLESS`, `--no-color`) |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE49` in `internal/hte/cli` (every HTE code emitted with its catalog action); `testdata/script/hte_errors.txtar` (stdout empty on error, envelope validated against `testdata/schema/error.json`) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-50"></a>
#### HTE-50 · `--json` on every command

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice (--json everywhere) |
| Area | Contract |
| hte-cli location | `app/Commands/DetailsCommand.php:102-189`; `app/Commands/HostsCommand.php` (`list`) |
| Observed behavior | Output is Symfony tables and prose only; scripts had to scrape `details` or read the files |
| Reproduction | `work/hte/run.sh $SB details -n` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Every `gash hte` command takes `--json`: site state, service status per master and site, doctor checks with remedies, migrate plans, receipts; `warnings` array where the design warns (HTE-26). One JSON Schema per shape under `testdata/schema/` |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE50` in `internal/hte/cli` (every command registered with `--json`); `testdata/script/hte_json.txtar` (each output validated against its schema) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-51"></a>
#### HTE-51 · No prompts or menus without a TTY; destructive steps need `--yes`

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice (no prompts without a TTY); hte-cli dossier A.0, F2 |
| Area | Contract |
| hte-cli location | `app/CommandWrapper.php:165-182`; `app/Traits/InteractiveMenu.php:73-146`; `app/Commands/RemoveCommand.php:157`; `app/Commands/ModifyCommand.php:186-258` |
| Observed behavior | Missing values are prompted, sites chosen from arrow-key menus; `-n` takes every default silently; `remove` without `--force` asks, and under `-n` cancels with exit 0; `modify --interactive` opens a change menu |
| Reproduction | `work/hte/run.sh $SB remove --domain=a.test -n` (`Operation cancelled.`, `[exit=0]`) |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Without a TTY or in headless mode nothing is asked: a missing required flag fails with `missing_value`, a destructive step without `--yes` fails with `confirmation_required` (RETRY, non-zero). On a TTY only a missing required flag is asked. No menus, no `--interactive`, no `-n` |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE51` in `internal/hte/cli` (prompt fake never called without a TTY); `testdata/script/hte_no_tty.txtar` (`site remove` without `--yes` → `confirmation_required`, site intact) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-52"></a>
#### HTE-52 · Force-HTTPS bypass for 127.0.0.1 and ::1 makes the redirect a no-op under loopback-only binding

| Field | Value |
|---|---|
| Section | HTE design delta · Decision for the operator |
| Origin | PRD 6.5 behavior, changed (optional force-HTTPS that does not redirect 127.0.0.1 / ::1); hte-cli dossier G; design D-2 |
| Area | Correctness |
| hte-cli location | `app/Services/ApacheManager.php:392-399` (bypass block); V1 had no bypass (`6edce81:app/Logic/Daemons/Apache2.php:214-321`) |
| Observed behavior | 1.1.x adds `RewriteCond %{REMOTE_ADDR} !^127\.0\.0\.1$` and `!^::1$` to the force block. Its vhosts match only connections to 127.0.0.1, whose client address is normally 127.0.0.1 too, so the redirect almost never fires (the `::1` condition was dead code). V1 sites redirected every request |
| Reproduction | `grep -n -A5 'Force HTTPS' go-impl/testdata/hte/legacy/templates/v2-vhost-forced.conf` (and the dossier G note, inferred from Apache's address matching) |
| Severity | medium |
| Public-contract impact | yes (redirects) |
| rc.A behavior | Decided at S1 (D-2, option C): the bypass is the per-site setting `force_https_bypass_loopback` (`--force-https-bypass`/`--no-force-https-bypass`), off by default, so force-HTTPS redirects every plain-HTTP request with 307; with the bypass on, requests from `127.0.0.1` and `::1` are not redirected (PRD 6.5 behavior). With `Listen` on loopback only every client address is loopback, so with the bypass on force-HTTPS redirects nothing. `migrate` sets the bypass per legacy variant (HTE-65) |
| Fix recipe (3.0) | Decision. Options: (a) keep the bypass as PRD 6.5 says (design today; force-HTTPS is inert, documented as such in `site show` and the docs); (b) drop the bypass for HTTPS-forced sites (force means a 307 for every plain-HTTP request; local scripts must call `https://` or follow redirects); (c) make the bypass a per-site flag (`force_https_bypass_loopback`, `--force-https-bypass`/`--no-force-https-bypass`) with a default to choose: "off" gives force-HTTPS a meaning and keeps an escape hatch for local tools |
| Proving test | `TestLedger_HTE52` in `internal/hte/render` (golden forced vhost per option); `hte-e2e`: `curl -sI http://<forced site>/` from loopback → 200 for (a), 307 for (b) and for (c) with the bypass off. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), steps "migrate --apply" (a V2 forced site answers 200 from loopback: option a) and "forced HTTPS without the bypass". |
| Status | accept: done (rc.A, option c); TestLedger_HTE52, E2E forced HTTPS without the bypass |

<a id="hte-53"></a>
#### HTE-53 · Command and flag surface: `hte-cli <cmd>` becomes `gash hte site|hosts|...`

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Contract |
| hte-cli location | `app/Commands/CreateCommand.php:30-37`; `ModifyCommand.php:31-38`; `RemoveCommand.php:29-32`; `DetailsCommand.php`; `FixCommand.php:19-22`; `HostsCommand.php:16-18` |
| Observed behavior | `sudo hte-cli create --domain= --docroot= --phpver= --ssl= --forcessl= --hosts= --profile=`; `modify` (same plus `--interactive`); `remove --domain= --force --no-normalize`; `details --domain=`; `fix --domain= --force --dry-run`; `hosts list\|sync\|add\|remove --domain=`; Symfony globals `-h -q -V --ansi/--no-ansi -n -v`; an unknown option exits 1 |
| Reproduction | `work/hte/run.sh $SB list` |
| Severity | medium |
| Public-contract impact | yes (every command and flag) |
| rc.A behavior | `gash hte site create --domain D --docroot P [--php X.Y] [--profile P] [--https\|--no-https] [--force-https\|--no-force-https] [--alias N]... [--no-www] [--no-http2]`; `site modify --domain D [same flags]`; `site remove --domain D --yes`; `site list`, `site show --domain D`, `site enable\|disable`; `hosts list\|apply\|check\|windows`; no `fix` (HTE-23), no `--hosts` (HTE-54), no `--interactive`/`--no-normalize`/`--dry-run` on site commands. cobra flags (`--flag value` and `--flag=value`), `--json`, `--help`, the Gash envelope for unknown flags. Run as the user, no sudo |
| Fix recipe (3.0) | Operator choice: (a) no compatibility layer (design today; the docs carry an old-to-new table); (b) a deprecated `hte-cli` wrapper installed by `migrate --apply` that maps the old commands and prints the new form on stderr |
| Proving test | `TestLedger_HTE53` in `internal/hte/cli` (the command tree and flags equal the design table); `testdata/script/hte_help.txtar` |
| Status | accept: done 57d1b5c, 2ad46b3 (TestLedger_HTE53; docs/hte.md command table) |

<a id="hte-54"></a>
#### HTE-54 · Hosts entries are always managed; `--hosts=no` has no counterpart

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Contract |
| hte-cli location | `app/Commands/CreateCommand.php:114,148-152,244-255` |
| Observed behavior | create asks "Add domain to /etc/hosts?" (an interactive "no" works; `--hosts=no` hits HTE-01 and becomes yes under `-n`); 1.0.x never touched hosts |
| Reproduction | `work/hte/run.sh $SB create --domain=b.test --docroot=$SB/d --phpver=8.3 --ssl=n --forcessl=n --hosts=no --profile=dev -n && grep b.test $SB/etc/hosts` |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Every site's names are written to the managed block on each site change, `service start` and `doctor`. Users who resolve `*.test` with their own resolver (dnsmasq, systemd-resolved stub zones) still get hosts lines |
| Fix recipe (3.0) | Operator choice: (a) always (design today); (b) a per-site `hosts = false` (`--no-hosts`); (c) a global `hosts = "managed" \| "off"` in `hte.toml` (recommended: the resolver choice is per machine, not per site) |
| Proving test | `TestLedger_HTE54` in `internal/hte/hosts` (block content for the chosen settings) |
| Status | accept: done (rc.A); TestLedger_HTE54 |

<a id="hte-55"></a>
#### HTE-55 · Defaults when a flag is absent

| Field | Value |
|---|---|
| Section | HTE design delta · Decision for the operator |
| Origin | Design choice |
| Area | Contract |
| hte-cli location | `app/Commands/CreateCommand.php:126-162` |
| Observed behavior | Prompt defaults: docroot = current directory, PHP = highest "installed" version, HTTPS yes, force yes, hosts yes, profile dev (README.md:616-618 documents "yes" defaults, true only because of HTE-01) |
| Reproduction | `work/hte/run.sh $SB create --domain=c.test --docroot=$SB/d -n` (8.3, SSL, forced, hosts, dev) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | `--domain` and `--docroot` are required. The design's sample state shows `https = true`, `force_https = false`, `http2 = true`, profile `dev`, and `hte.toml` holds a default PHP; it does not state the defaults of absent flags or what happens when no default PHP is set |
| Fix recipe (3.0) | Operator choice: (a) `https` on, `force_https` off, `http2` on, profile `dev`, PHP = the `hte.toml` default, else the highest discovered version (recommended); (b) hte-cli's defaults, force-HTTPS on (pointless while HTE-52 keeps the bypass) |
| Proving test | `TestLedger_HTE55` in `internal/hte/state` (create with only the required flags → the chosen TOML) |
| Status | accept: done (rc.A); TestLedger_HTE55 |

<a id="hte-56"></a>
#### HTE-56 · Ports 80/443 without a root master: sysctl, pf, or high ports

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Portability |
| hte-cli location | none (the root-started system Apache binds 80/443 through Debian's `ports.conf`) |
| Observed behavior | Sites answer on 80/443 because the system master starts as root |
| Reproduction | by design |
| Severity | medium |
| Public-contract impact | yes (ports, URLs, a system-wide setting) |
| rc.A behavior | Linux and WSL2: `install` sets `net.ipv4.ip_unprivileged_port_start=80` (`/etc/sysctl.d/50-gash-hte.conf` + `sysctl -w`, recorded in the receipt, reverted by `uninstall`; already ≤ 80 in Docker, left as is) after stating that every local user may then bind 80 to 1023. macOS: a direct bind when the kernel allows it, else a `pf` anchor redirecting `127.0.0.1:80/443` to 8080/8443, loaded at boot by a root LaunchDaemon. Declined or impossible: 8080/8443 (configurable), site URLs carry the port, `doctor` reports it |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE56` in `internal/hte/rootops` (install then uninstall on the sysroot fake restores the sysctl file and value byte for byte; a value already at 80 or lower changes nothing and the receipt says so); `hte-e2e`: unprivileged master bound to `127.0.0.1:80`. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), steps "install next to the system Apache" and "migrate --apply" (the sysctl set to 80, the unprivileged master on `127.0.0.1:80`). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-57"></a>
#### HTE-57 · Autostart through a user unit, a LaunchAgent, or the first shell

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Portability |
| hte-cli location | none (relies on the distro enabling `apache2.service` and `php{v}-fpm.service` at boot) |
| Observed behavior | Sites come up at boot with the system services |
| Reproduction | by design |
| Severity | low |
| Public-contract impact | yes (when sites are up) |
| rc.A behavior | systemd: user unit `gash-hte.service` (`Type=oneshot`, `RemainAfterExit=yes`) enabled with `systemctl --user enable` plus `loginctl enable-linger` (root step) so it starts at boot. No systemd (containers, WSL without systemd): no boot hook, `/etc/wsl.conf` untouched, optional `autostart = "shell"` (the first interactive shell running `gash init bash` starts the stack). macOS: LaunchAgent with `RunAtLoad`, so sites start at login, not at boot |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE57` in `internal/hte/service` (rendered unit and plist golden files; receipt entries for linger) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-58"></a>
#### HTE-58 · Supported platforms: Debian/Ubuntu only becomes Debian/Ubuntu, Fedora, Arch, macOS

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Portability |
| hte-cli location | `app/CommandWrapper.php:90-127`; `app/Services/EnvironmentChecker.php:37-78` |
| Observed behavior | Needs `apache2` and `php` on PATH, the Debian `/etc/apache2` layout, `a2ensite`, `systemctl`, a `SHELL` variable ("HTE-Cli must be run from a terminal."); refuses Windows |
| Reproduction | `env -i HOME=$SB PATH=/usr/bin:/bin USER=root SUDO_USER=ubuntu HTE_SANDBOX=$SB php work/hte/harness/hte details` (no `SHELL`: "HTE-Cli must be run from a terminal.") |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Debian/Ubuntu (`apache2`, `/usr/lib/apache2/modules`), Fedora (`httpd`, `/usr/lib64/httpd/modules`, distro PHP and Remi), Arch (`httpd`, `php`, `php-legacy`), Homebrew `httpd` on macOS, WSL2 with a manual checklist. `install` and `doctor` check prerequisites per platform: `apache_not_found`, `platform_unsupported` (FATAL). No `SHELL` requirement |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE58` in `internal/hte/discover` (sysroot fakes per platform → binary, module directory, PHP layouts); `hte-e2e` matrix (Debian, Ubuntu, Fedora, Arch, macOS) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-59"></a>
#### HTE-59 · The server-level configuration is rendered by gash, not Debian's `apache2.conf`

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Security |
| hte-cli location | none (the system `apache2.conf`, `mods-enabled/`, `conf-enabled/` apply); README.md:128-148 (`a2enmod actions fcgid alias proxy_fcgi setenvif rewrite headers ssl http2 mpm_event brotli`, `a2dismod php* mpm_prefork`) |
| Observed behavior | Server settings, modules and snippets are whatever the system Apache has: Debian's `apache2.conf` denies `/` and every `^\.ht` file, `dir.conf` sets `DirectoryIndex index.html index.cgi index.pl index.php index.xhtml index.htm`, `security.conf` sets `ServerTokens`/`ServerSignature`/`TraceEnable` (Debian file contents, inferred from the packages, not run) |
| Reproduction | by design |
| Severity | medium |
| Public-contract impact | yes (modules and snippets enabled in the system Apache no longer apply) |
| rc.A behavior | `render/current/httpd.conf`: `ServerRoot`, `PidFile`, `DefaultRuntimeDir`, `Mutex` in the runtime directory, global `ErrorLog`, `LogLevel warn`, `LoadModule` for the fixed userland list minus built-ins (read from `httpd -l`), TLS defaults (HTE-15), `ServerTokens`/`ServerSignature`/`TraceEnable Off`, `Protocols`, `TypesConfig`, `AddDefaultCharset`, then the global overlay and the sites. `apache modules enable NAME` adds a module from the system module directory only. A module of the list (or an enabled extra one) that the system module directory lacks fails with `apache_module_missing` (STOP), whose hint names the platform's install command (`sudo apt install --reinstall apache2`, `sudo dnf install mod_ssl mod_http2`, `sudo pacman -S apache`, `brew reinstall httpd`); `apache_not_found` (FATAL) stays for a missing server binary (lead decision, 2026-10-18). The design's list does not mention three Debian defaults the sites rely on: `<Directory /> Require all denied`, `<FilesMatch "^\.ht"> Require all denied` (without it `.htaccess` and `.htpasswd` in a docroot are served as files), and a `DirectoryIndex` that includes `index.php` |
| Fix recipe (3.0) | Design text to amend (no operator choice): render `<Directory "/"> AllowOverride None` + `Require all denied`, `<FilesMatch "^\.ht"> Require all denied`, and Debian's `DirectoryIndex` list at server level on every platform |
| Proving test | `TestLedger_HTE59` in `internal/hte/render` (golden `httpd.conf` per platform holds the three blocks; `LoadModule` omitted for built-ins); `TestResolveModules` in `internal/hte/discover` (missing modules → `apache_module_missing` with the Fedora packages); `hte-e2e`: `GET /.htaccess` → 403, `GET /` serves `index.php` |
| Status | accept: done (rc.A); TestLedger_HTE59, E2E dotfile 403 check ff3eed4 |

<a id="hte-60"></a>
#### HTE-60 · A missing document root is never created

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Correctness |
| hte-cli location | `app/Commands/ModifyCommand.php:400-432` |
| Observed behavior | Interactive `modify` offers "Directory 'x' does not exist. Create it?" and creates it with mode 0755 as root (root-owned directory in the user's tree) |
| Reproduction | code reading (interactive menu only; `-n` cannot reach it) |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | The docroot must exist and is never created or modified: `docroot_not_found` (RETRY, hint: "create the directory first") |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE60` in `internal/hte/state` (missing directory → `docroot_not_found`, nothing created) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-61"></a>
#### HTE-61 · An `install` step, a receipt, and a restoring `uninstall`

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Self-management |
| hte-cli location | none (a phar run with `sudo php`, README.md:101-108) |
| Observed behavior | No setup and no undo: hte-cli edits system files directly on every command |
| Reproduction | by design |
| Severity | low |
| Public-contract impact | yes (new required step) |
| rc.A behavior | `install` (one interactive sudo: hosts helper `/usr/local/libexec/gash/gash-hosts-helper`, sudoers drop-in `/etc/sudoers.d/gash-hte` validated with `visudo -c -f`, port setting, linger, freeing 80/443) writes the receipt before the first change and after each one (user copy and root copy `/var/lib/gash-hte/receipt-<user>.json`, prior contents of modified files kept in `/var/lib/gash-hte/backup/`). Every command except `install`, `doctor`, `migrate` (dry-run) and `php versions` fails with `hte_not_installed` (STOP) before it. `uninstall` restores from the root copy (refused with `receipt_mismatch`, FATAL, when the user copy differs) and moves the userland tree into `$XDG_STATE_HOME/gash/hte-stage-<ts>/`, which a later `install` offers to restore |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE61` and `TestInterrupted` in `internal/hte/rootops` (install then uninstall on the fixture sysroot: byte-identical tree; interrupted install undone from a partial receipt, its last change still pending); `TestCompare` in `internal/hte/receipt` (tampered user copy → `receipt_mismatch`). Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`) and `TestE2EReinstall`: after both uninstalls `/etc`, `/usr/local` and `/var/lib/gash-hte` equal their state before install, the system Apache runs again; a second install records nothing. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-62"></a>
#### HTE-62 · Mutations take a lock and append to an audit log

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Correctness |
| hte-cli location | none (no locking; concurrent runs race on the same files and on `getConfMaxIndex`) |
| Observed behavior | Two concurrent `create` calls can pick the same index (inferred from `app/Services/ApacheManager.php:164-180`) |
| Reproduction | by design |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | Exclusive `flock` on `$XDG_STATE_HOME/gash/hte/lock` for every mutation (30 s, then `lock_timeout`, RETRY); reads take none. One JSON line per mutation in `audit.jsonl`: `{"ts","command","site","outcome","details"}` |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE62` in `internal/hte/state` (second mutation while the lock is held → `lock_timeout` after the fake clock passes 30 s; audit line per mutation, none per read) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-63"></a>
#### HTE-63 · New commands with no hte-cli counterpart

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice |
| Area | Contract |
| hte-cli location | none |
| Observed behavior | hte-cli offers only create, modify, remove, details, fix and hosts; services, logs, certificates, PHP settings and modules are managed by hand |
| Reproduction | by design |
| Severity | low |
| Public-contract impact | yes (additive) |
| rc.A behavior | `install`, `uninstall`, `doctor` (binaries, modules, ports, supervision, sockets, config test, per-site health, name resolution, certificate expiry and SAN, receipt integrity), `migrate`, `service start\|stop\|restart\|reload\|status\|logs`, `cert show\|renew`, `php versions`, `php ini get\|set\|unset --domain D`, `php global get\|set\|unset --version X.Y`, `apache config path`, `apache modules list\|enable\|disable`, `site enable\|disable`, `hosts check\|windows` (WSL: the lines to add to the Windows hosts file for names Windows cannot resolve) |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE63` in `internal/hte/cli` (command tree); one `testdata/script/hte_<command>.txtar` per command |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-64"></a>
#### HTE-64 · `migrate --apply` stops and disables the system Apache; legacy files are archived

| Field | Value |
|---|---|
| Section | HTE design delta · Decision for the operator |
| Origin | Design choice |
| Area | Contract |
| hte-cli location | none (hte-cli shares the system Apache with every other vhost) |
| Observed behavior | HTE sites, hand-written vhosts, reverse proxies and Debian `conf-enabled/` snippets (for example an `Alias /phpmyadmin`) all run in `apache2.service` |
| Reproduction | by design |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | Dry-run plan by default. Per site: create the userland site and verify it on a staging listener (`127.0.0.1:<high port>`, Host header and SNI) while the system Apache still holds 80/443; when every enabled system vhost is migrated or excluded, stop and disable the system Apache and the FPM units left without pools, start the userland stack on 80/443, verify again; only then `a2dissite` (or the platform equivalent) and move the legacy files to `/var/lib/gash-hte/archive/<ts>/`, each step in the receipt. Unmarked vhosts are imported as `kind = raw` (system paths and `${APACHE_LOG_DIR}` rewritten, certificates copied, config-tested). Two consequences the design does not spell out: an excluded enabled vhost stops being served, and Debian's `conf-enabled/` snippets are neither read nor classified (Fedora's `conf.d` is read). `--exclude` takes a domain or an absolute vhost file path; nothing is excluded implicitly (HTE-31) |
| Fix recipe (3.0) | Operator choice for `conf-enabled/` snippets: (a) list each enabled one in the plan as "not migrated, no longer served" (recommended); (b) import them as raw server-level includes. In both cases the plan names every excluded enabled vhost as no longer served |
| Proving test | `TestLedger_HTE64` in `internal/hte/migrate` (plan on a sysroot with V2 sites, one raw reverse proxy, one excluded vhost and one `conf-enabled` snippet → each listed with its outcome; `--apply` on the fake runner: archive and receipt entries in order, nothing deleted) |
| Status | accept: done (rc.A); TestLedger_HTE64, TestLedger_HTE64Apply 0e7d513 |

<a id="hte-65"></a>
#### HTE-65 · Imported forced sites change behavior: V1 lose their redirect, all move to 307

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice; hte-cli dossier D.1 to D.4 |
| Area | Contract |
| hte-cli location | `6edce81:app/Logic/Daemons/Apache2.php:214-321` (V1 force block without bypass); `app/Services/ApacheManager.php:250-349` (fix patterns) |
| Observed behavior | V1 forced sites redirect every plain-HTTP request with 301 (no bypass); V1-fixed, V2 and repaired Broken-bypass sites carry the bypass and redirect almost nothing (HTE-52) |
| Reproduction | `diff go-impl/testdata/hte/legacy/V1/etc/apache2/sites-available/003-forced.test.conf go-impl/testdata/hte/legacy/V1-fixed/etc/apache2/sites-available/003-forced.test.conf` |
| Severity | low |
| Public-contract impact | yes (redirects) |
| rc.A behavior | Every variant imports as `force_https = true` and renders gash's block with `R=307`; the bypass follows the variant (D-2): off for V1 (they keep redirecting every request), on for V1-fixed, V2 and Broken-bypass (they keep redirecting almost nothing). The only change is 301 to 307 |
| Fix recipe (3.0) | none: the delta is the design (follows the HTE-52 decision). Recommended: the plan notes the change for each V1 forced site |
| Proving test | `TestLedger_HTE65` in `internal/hte/migrate` (V1, V1-fixed, V2, Broken-bypass forced sites → identical rendered force blocks) |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-66"></a>
#### HTE-66 · migrate finds the pool by exact path; pool values, profile and owner are reconciled

| Field | Value |
|---|---|
| Section | HTE design delta · Decision for the operator |
| Origin | Design choice; hte-cli dossier D.5, D.6, F3, I1 |
| Area | Correctness |
| hte-cli location | `app/Services/PhpFpmManager.php:70-112,183-205` |
| Observed behavior | hte-cli takes the first pool containing `[{d}]` (HTE-05) and reads the profile from `; Profile:` (lowercased in 1.1.1, `dev` when absent or unknown, silently); V1 pools have no marker and write `php_value[short_open_tag] =  On` (two spaces) |
| Reproduction | fixture `Malformed/` (`024-quoted.test` pool `; Profile: custom` → dev; `nocert.test` pool `; Profile: HARDENED` → hardened) |
| Severity | medium |
| Public-contract impact | yes (pool settings) |
| rc.A behavior | Pool = `/etc/php/<X.Y>/fpm/pool.d/<domain>.conf` (or the platform equivalent) whose first section is `[<domain>]`. Profile: `; Profile:` lowercased; absent (V1) → `dev` with a note; unknown → `dev` with a warning. The pool's values are compared with the profile and the differences reported; with `--keep-pool-values` they go to the site's `[php_ini]` (the value text without its INI quotes or trailing `;` comment; a value `[php_ini]` cannot hold, with `"`, `\` or `${`, is reported as not kept). A kept key that the profile sets replaces the profile line with the profile's admin kind; any other key becomes `php_value`/`php_flag`, so an extra `php_admin_value` of the legacy pool loses its admin kind (lead decision on `[php_ini]`, 2026-10-18). Inconsistencies in the design: `--keep-pool-values` is missing from the `migrate` signature of section 9, and the V1 row of the class table says "profile inferred from the pool values" while step 4 says `dev` (both give `dev` on an unmodified V1 pool). The design does not say what happens when the pool's `user` is not the invoking user (another sudo user, or `root` after HTE-18) |
| Fix recipe (3.0) | Design text to amend: add `--keep-pool-values` to the signature; one profile rule (recommended: `; Profile:` if present, else the profile whose values match exactly, else `dev` with the differences reported); compare values with whitespace normalized. Operator choice for a foreign pool `user`: (a) import and report (the site then runs as the current user; recommended); (b) skip the site, reported |
| Proving test | `TestLedger_HTE66` in `internal/hte/migrate` (V1 → dev with a note and no differences; V2 staging/hardened → their profile; a pool with an extra `php_admin_value` → reported, kept only with `--keep-pool-values`; foreign `user` → the chosen outcome) |
| Status | accept: done (rc.A); TestLedger_HTE66, TestDetectProfile |

<a id="hte-67"></a>
#### HTE-67 · migrate copies the legacy certificate pair through the root step, or reissues it

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice; hte-cli dossier D.6, F4 |
| Area | Contract |
| hte-cli location | `app/Services/SslCertManager.php:92-108` (key `chmod 400`, root); `app/Services/ApacheManager.php:88-99` (1.1.1 SSL rule) |
| Observed behavior | Keys are root 0400 in `/etc/apache2/certs-selfsigned/`; certificates are RSA 2048, 30 years; `.info` files sit beside them |
| Reproduction | `ls -l go-impl/testdata/hte/legacy/V2/etc/apache2/certs-selfsigned` |
| Severity | low |
| Public-contract impact | yes (certificate kept or replaced) |
| rc.A behavior | HTTPS = a `:443` vhost plus both `SSLCertificate*File` lines. Exactly `<certdir>/<domain>.crt` and `.key` (never a glob; `.info` ignored, reported as orphan when alone); the root step copies the key into `certs/<domain>/key.pem` (0600, user), or `--reissue-certs` makes a new ECDSA pair (HTE-39). A copied pair keeps its RSA 30-year certificate. `--reissue-certs` is missing from the `migrate` signature of design section 9 |
| Fix recipe (3.0) | Design text to amend: add `--reissue-certs` to the signature. Recommended: `doctor` notes copied legacy certificates (RSA, no `basicConstraints`) and suggests `cert renew` |
| Proving test | `TestLedger_HTE67` in `internal/hte/migrate` (V2 `https.test` → root op "copy key" planned with the exact source path; `Malformed/023-nocert.test` → HTTP only; with `--reissue-certs` no root op for keys) |
| Status | accept: done (rc.A); TestLedger_HTE67 0342218 |

<a id="hte-68"></a>
#### HTE-68 · migrate reports duplicates, shared sockets, custom ports and orphans that hte-cli never noticed

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice; hte-cli dossier B.2, D.5, I3 |
| Area | Correctness |
| hte-cli location | `app/Services/ApacheManager.php:148-159` (first match wins); `app/Services/ApacheManager.php:164-180` (index over every file) |
| Observed behavior | Duplicate domains resolve to the first file; a vhost using another site's socket is not detected; `Listen` ports, orphan pools, certificates, `.info` files and index gaps are invisible |
| Reproduction | fixture `Malformed/` (the non-HTE `000-default.conf` takes part in the next-index computation, `026`; orphan pool `etc/php/custom/fpm/pool.d/orphan.test.conf`, skipped silently) |
| Severity | low |
| Public-contract impact | yes (custom ports move to the high range) |
| rc.A behavior | Duplicates (case-insensitive): both reported, one picked with `--site` or the other excluded. Shared socket: both sites get their own pool, reported. Custom ports: the ports of a vhost's `<VirtualHost>` sections other than 80, 443 and the stack's own become the site's `extra_http_ports` / `extra_https_ports` (by the section's `SSLEngine on`), for managed and raw imports alike, rendered as `Listen` on the loopback addresses plus addresses of the site's vhosts; each is reported, with a note when it is below 1024 (binding needs `ip_unprivileged_port_start` at most that port). A section without a port, a port used for both protocols, or a TLS port without a certificate keeps the vhost in the system (lead decision, 2026-10-18: `site create|modify --extra-http-port N` and `--extra-https-port N`, repeatable, 1 to 65535, never 80, 443 or the stack's ports; one port has one protocol across sites). Orphan pools, certificates, `.info` files and index gaps: reported, never deleted. `APACHE_RUN_USER` set to the user in `envvars`: reported (the master is still root). IP-named vhosts: HTE-21 |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE68` and `TestRawCustomPorts` in `internal/hte/migrate` (one sysroot fixture per case → the exact plan entries; tree bytes unchanged by a dry run); `TestExtraPorts` in `internal/hte/state`, `TestExtraListen` in `internal/hte/render`; `hte-e2e`: a site answers on its extra port. Container run: `internal/hte/e2e` (`TestE2E`, container run of `testdata/hte/e2e/run.sh`), step "an extra port". |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-69"></a>
#### HTE-69 · The hosts helper cannot rename over a bind-mounted /etc/hosts (containers)

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice; rc.A HTE E2E |
| Area | Platform |
| hte-cli location | `internal/hostshelper` (`Rewrite`); design 7.1 ("renamed over the original") |
| Observed behavior | Docker bind-mounts `/etc/hosts`, so `rename(2)` of the helper's temporary file over it fails with EBUSY: `rename /etc/.hosts.gash-hte-* /etc/hosts: device or resource busy`. Containers are a listed platform. |
| Reproduction | `printf '127.0.0.1 repro.test\n' \| gash-hosts-helper apply` in a stock container |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | When the rename fails with EBUSY or EXDEV, the helper removes its temporary file and writes the same validated content in place (open without truncation, write, truncate to the new length, fsync). Same file, same content checks, same single sudoers line: no new surface. Elsewhere the rename stays (lead decision, 2026-10-18). |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestRewriteInPlace` in `internal/hostshelper` (a rename that fails with EBUSY gives the same bytes in place and leaves no temporary file); container run without the mount swap in `setup.sh`. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-70"></a>
#### HTE-70 · migrate --apply with --ports high keeps the system Apache running

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice; rc.A HTE phase |
| Area | Correctness |
| hte-cli location | design 9, `--apply` step 2 (written for standard ports) |
| Observed behavior | Step 2 stops and disables the system Apache to free 80/443. With `--ports high` the stack does not use 80/443, so the stop only takes away what the system Apache still serves: excluded vhosts and the default site. |
| Reproduction | `install --ports high`, then `migrate --apply --yes` with an excluded vhost: 80/443 go unanswered |
| Severity | medium |
| Public-contract impact | yes |
| rc.A behavior | With high ports the system Apache is not stopped. Each migrated vhost is disabled and archived, the system configuration is tested (`apachectl configtest` as root), and the system Apache gets a graceful reload; a failed test re-enables the vhosts, nothing is reloaded, and the command reports `config_test_failed`. System FPM units left without pools stop as with standard ports. The plan says which ports stay with the system Apache (lead decision, 2026-10-18). |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE70` in `internal/hte` (plan with `--ports high`: no unit stop, disable, configtest and reload steps; a failed configtest undoes the disable); container run: the excluded vhost still answers on 80. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-71"></a>
#### HTE-71 · A reload after an overlay edit could not roll back

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice; rc.A HTE phase |
| Area | Correctness |
| hte-cli location | design 5 (vhost `IncludeOptional` of the user's overlay), 10 (generations) |
| Observed behavior | The vhost included the user's overlay file directly. When a graceful reload passed the config test but failed at run time (a busy port), the previous generation included the same, new overlay, so it could not come back as it was. |
| Reproduction | edit an overlay to `Listen` a busy port, `gash hte service reload` |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | Each generation holds a copy of every site overlay it includes, and the vhost includes the copy. `service reload` copies the overlays into the new generation, tests, reloads, and on failure brings back the previous generation with its own copies. The user's overlay file is never modified; `apache config path` still names it (lead decision, 2026-10-18). |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE71` in `internal/hte` (an overlay whose reload fails leaves the previous generation running and the overlay file unchanged); container run: step "a busy extra port" repeated through an overlay. |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-72"></a>
#### HTE-72 · Error codes for a missing openssl, a bad flag value and PHP settings on a site without PHP

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice; rc.A HTE phase |
| Area | Contract |
| hte-cli location | design 11 (error codes) |
| Observed behavior | The engine answered a missing openssl with `platform_unsupported`, a bad value of an enum flag with `conflicting_options`, and `php ini set` on a site without PHP with `conflicting_options`. Those codes mean an unsupported system and a flag pair that contradicts itself. |
| Reproduction | `gash hte site create x.test --ports bogus`; `gash hte php ini set` on a `--no-php` site; PATH without openssl |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | `openssl_not_found` (FATAL, hint with the platform's install command, like `apache_not_found`); `invalid_<flag>` (RETRY, details the value and the accepted ones) for every enum flag, like the existing `invalid_profile`; `php_not_enabled` (RETRY, hint `site modify --php VERSION`). Registered in the contract catalog (lead decision, 2026-10-18). |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE72` in `internal/hte` (each case → its code and action). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-73"></a>
#### HTE-73 · With systemd autostart the stack has one supervisor

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice; rc.A HTE phase |
| Area | Correctness |
| hte-cli location | `internal/hte/service`, `internal/hte/migrate.go` |
| Observed behavior | With `--autostart systemd`, `migrate --apply` started the masters directly and then ran `systemctl --user start`, whose ExecStart found the stack already running: the unit and the processes it should own diverge. |
| Reproduction | not reproduced: no systemd in the E2E containers |
| Severity | low |
| Public-contract impact | no |
| rc.A behavior | When the autostart is a systemd user unit, every start, stop and restart of the stack (install, migrate, service commands) goes through `systemctl --user`; the direct start is used only for the other autostart kinds. Proven with the runner fake; a container with systemd as PID 1 proves it when the environment allows one (lead decision, 2026-10-18). |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE73` in `internal/hte` (with systemd autostart the only lifecycle children are `systemctl --user start\|stop\|restart gash-hte.service`). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-74"></a>
#### HTE-74 · A root operation restarts the system Apache when migrate fails after the switch

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice; rc.A HTE phase |
| Area | Security |
| hte-cli location | `internal/hte/rootops` (`undo_unit_stop`); design 7, 9 |
| Observed behavior | The S1 set of root operations had no way to give the system Apache back when a site does not answer after the switch, short of a full uninstall. |
| Reproduction | a site that answers on staging but not after the switch |
| Severity | medium |
| Public-contract impact | no |
| rc.A behavior | `undo_unit_stop`: a conditional step of the plan, run only when a site does not answer after the switch, and only when the unit stop is the last change of the root receipt and was made in the same session; it starts and re-enables that unit, recorded in the receipt. The imported sites stay and the next run completes the move (lead decision, 2026-10-18). |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE74` in `internal/hte/rootops` (refused when the stop is not the receipt's last change, or comes from another session; runs once). |
| Status | accept: no 3.0 change (Fix recipe: none) |

<a id="hte-75"></a>
#### HTE-75 · install next to a system Apache it may not stop exits 0 with the stack not started

| Field | Value |
|---|---|
| Section | HTE design delta |
| Origin | Design choice; rc.A HTE phase |
| Area | Contract |
| hte-cli location | `internal/hte/install.go` |
| Observed behavior | With standard ports and enabled vhosts that block the stop (MUST NOT 13), install completes every step but cannot start the stack on 80/443. |
| Reproduction | install on a machine whose system Apache serves an enabled vhost |
| Severity | low |
| Public-contract impact | yes |
| rc.A behavior | Exit 0: install did what it can do before `migrate`. The JSON has `started: false` and `next: "gash hte migrate"`, the human output a warning naming the blocking vhosts and the command. A scripted `install && migrate` works (lead decision, 2026-10-18). |
| Fix recipe (3.0) | none: the delta is the design |
| Proving test | `TestLedger_HTE75` in `internal/hte` (blocking vhosts → exit 0, started false, next set). |
| Status | accept: no 3.0 change (Fix recipe: none) |

## Seed index

Every row of the PRD section 8 seed table, verified against 2.1.0, and the items that record it.
CONFIRMED means the audit reproduced the finding as stated; PARTIAL means part of it holds and the
item says which part.

| Area | Seed (PRD section 8, short) | Verdict | Items |
|---|---|---|---|
| Security | `llm_exec`: attached short-option values and option clusters skip the secret-file and forbidden-path checks | CONFIRMED | [EXEC-01](#exec-01) |
| Security | `llm_db_*` on PostgreSQL: E-string mislex bypasses the guard; `SET` not forbidden; psql meta-commands reachable | CONFIRMED | [DB-01](#db-01) |
| Security | `llm_grep --context` unvalidated, evaluated as Bash arithmetic; a non-number kills a `set -u` caller | CONFIRMED | [LLM-01](#llm-01) |
| Security | `llm_exec` allows `git stash list -p` | CONFIRMED | [EXEC-02](#exec-02) |
| Security | `llm_env` password masking fails with `@` or `/` | CONFIRMED | [LLM-02](#llm-02) |
| Security | Scanners with `-L` follow symlinks into forbidden paths | CONFIRMED | [FS-02](#fs-02) |
| Contract | Git module: remote tag delete, tag push and `git_sync` without confirmation or headless `--yes` | CONFIRMED | [GIT-01](#git-01) |
| Correctness | `llm_grep` fallback BRE vs ripgrep syntax; hidden and ignored files differ | CONFIRMED | [LLM-03](#llm-03) |
| Correctness | Without `jq`: `llm_git_status` invalid JSON with exit 0; DB envelope silently raw | CONFIRMED | [LLM-04](#llm-04), [DB-05](#db-05) |
| Correctness | Invalid manifest leaks a raw `jq` error with exit 5 | PARTIAL (`llm_deps` yes, `llm_project` no) | [LLM-05](#llm-05) |
| Correctness | DB TSV to JSON drops single-column empty rows; raw mode leaves control characters; MySQL NULL is `"NULL"` | CONFIRMED | [DB-03](#db-03), [DB-02](#db-02) |
| Correctness | awk JSON escapers miss U+0001 to U+001F besides tab, newline, CR | CONFIRMED | [LLM-06](#llm-06) |
| Correctness | Scanners: the last PATH silently wins; integers of any length accepted | CONFIRMED | [FS-04](#fs-04), [FS-05](#fs-05), [FS-21](#fs-21) |
| Correctness | Slow-query threshold includes client spawn and connect time | CONFIRMED | [DB-06](#db-06) |
| Correctness | `php_detect_version`: lookbehind kills one source; detected PHP not checked as installed; `^7.4\|^8.0` picks 7.4 | PARTIAL (the source is dead on PCRE2 before 10.43 and on greps without `-P`, not because of grep 3.11 itself); rest CONFIRMED | [DEV-09](#dev-09), [DEV-01](#dev-01), [DEV-02](#dev-02) |
| Correctness | Compose functions: `realpath -m` failure falls back to `$PWD` | CONFIRMED | [DOCK-01](#dock-01), [DOCK-02](#dock-02) |
| Correctness | Remote functions: unknown flags silently ignored, including on `--delete` paths | CONFIRMED | [REM-01](#rem-01) |
| Correctness | `ai_query claude` sends the word "claude" as the query | CONFIRMED | [AI-02](#ai-02) |
| Correctness | History settings overwrite the user's `HISTCONTROL` | CONFIRMED | [SHELL-04](#shell-04) |
| Correctness | `~/.bash_aliases` sourced twice on Ubuntu | CONFIRMED (filed as a decision: the fix changes which file the user's aliases come from) | [SHELL-05](#shell-05) |
| Self-management | `gash_upgrade` equality only (can downgrade); uninstall deletes `~/.gashrc` without backup; unload leaves completion specs and exports | CONFIRMED | [SELF-01](#self-01), [SELF-02](#self-02), [SELF-03](#self-03) |
| Completion | Offers a `help` topic that errors; `sysinfo` lacks `overview`; `dcup2` lacks `--yes` | CONFIRMED | [SHELL-03](#shell-03), [SYS-26](#sys-26), [DOCK-03](#dock-03) |
| Decision | Scanner exclusion of tab/newline paths | PARTIAL (the exclusion differs per scanner) | [FS-01](#fs-01), [FS-06](#fs-06), [FS-07](#fs-07), [FS-08](#fs-08), [LLM-29](#llm-29) |
| Decision | MySQL NULL representation | CONFIRMED | [DB-02](#db-02) |
| Decision | Per-engine `llm_db_schema` keys | CONFIRMED | [DB-10](#db-10) |
| Decision | `-h` meaning help on `llm_db_*` | CONFIRMED | [DB-11](#db-11) |
| Decision | `llm_exec` blocklist matching quoted text | CONFIRMED | [EXEC-03](#exec-03) |
| Decision | Pattern operands checked as file names | CONFIRMED | [EXEC-04](#exec-04) |
| Doc drift | `AGENTS.md` section 3: second PATH, 9-digit rule, ripgrep use, `file:line:content` | CONFIRMED | [FS-31](#fs-31), [FS-32](#fs-32), [LLM-30](#llm-30), [LLM-07](#llm-07) |
| Doc drift | `AGENTS.md` section 6.2 (secret patterns) | CONFIRMED | [LLM-32](#llm-32) |
| Doc drift | `docs/security.md` | CONFIRMED | [GIT-11](#git-11), [REM-09](#rem-09) |
| Doc drift | `CONTRIBUTING.md`: GNU/BSD userland; major vs minor for breaking changes | CONFIRMED | [SHELL-14](#shell-14), [SELF-18](#self-18) |
| Doc drift | `docs/troubleshooting.md` | PARTIAL | [SELF-19](#self-19) |
| Doc drift | `.gash_env.template` URL-encoding claim | PARTIAL | [CORE-12](#core-12) |
| Doc drift | README function count | PARTIAL | [SELF-20](#self-20) |

## Seeds not confirmed

None: every seed is CONFIRMED or PARTIAL (see the seed index).
