<!-- Gash 3.0 port state. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Port state

Read this first in every session, then `git log --oneline -20`, then run the gate.

## Phase

rc.A implementation. S1 was approved by the operator in the session on 2026-10-18 (draft PR #5, `go-refactor` → `main`); the answers to D-1 to D-9 are in `go-impl/design/README.md`, "Decisions taken".

## Last completed step

- Branch `go-refactor` created from `main` (a4fbc8b) and pushed.
- Bash baseline: `bash tests/run.sh` → 800 passed, 0 failed, 7 skipped (807) in 33 s on the cloud VM (4 CPU, Bash 5.2.21, mawk, no gawk/sqlite3).
- Go 1.27.2 obtained through the toolchain mechanism (`GOTOOLCHAIN=go1.27.2`); `encoding/json/v2` importable without GOEXPERIMENT and rejects invalid UTF-8 by default.
- Userland Apache + PHP-FPM spike green on an Ubuntu 24.04 container (non-root master on 127.0.0.1:80/443, HTTP/2, proxy_fcgi, graceful reload).
- S1 design written: `go-impl/design/` (README with the decisions requested, architecture, layout, compat, shell, upgrade, libraries, testing, hte), `PARITY.md` (all 1095 spec tests), prompt and startup benchmarks with the 2.x numbers (`go-impl/bench/shell/`), legacy hte-cli fixtures (`go-impl/testdata/hte/legacy/`).
- S1 draft PR #5 opened; CI green (12 checks).
- Operator decisions recorded (D-2 option C, D-6 no 1.x work, the rest as recommended) in the design, MATRIX and LEDGER.
- Audit of every 2.x module done: `MATRIX.md` (881 rows for 2.x plus the new 3.x surface) and `LEDGER.md` (every PRD seed indexed) merged from per-module fragments. The merge scripts are gone: both files are now edited by hand, by the lead session only.
- rc.A work merged so far: HTE core (`internal/platform`, `internal/hte/*`, `internal/hostshelper`, `cmd/gash-hosts-helper`), path guard and llm_exec core (`internal/pathguard`, `internal/llmexec`, canonical script per D-1 A, 2.x child prelude byte for byte), config, SQL guard and runner (`internal/config`, `internal/sqlguard`, `internal/runner`).
- The two install specs that snapshot the working tree into a fresh repository disable git's background maintenance: with the Go tree the detached repack raced the clone and the cleanup on git 2.55 (CI only).
- Core output primitives, layout emitters and confirmations, runner starts (`Start`, `StartPTY`, `RunScript`) and the leaf config loader merged; stubs of the 21 functions whose 2.x body loads the settings run the cached `__gash_load_env` first (CORE-18).
- Harness: `tests/run.sh` reads each spec's recorded status (TEST-09); Go mode starts the HTTPS fake (`internal/tools/httpsfake`, the shared curl mock's answers over `HTTPS_PROXY`); the mock-client DB specs pin the clock (TEST-10). Per-test curl fakes move to `MOCK_CURL_ROUTES` route files (TEST-11, core first, then each slice). Go mode: 233 passed of 807 at 883e12d.
- HTE engine merged (b7a905b): sites, service, certificates (openssl CLI, HTE-39), hosts, supervision, receipt, root session, install/uninstall, `migrate --apply`, doctor, php ini/global; Docker E2E (`go-impl/testdata/hte/e2e/run.sh`, two stories, /etc and /usr/local identical after uninstall) on Ubuntu 24.04. Decisions on its report: HTE-08 (docroot characters, SCRIPT_FILENAME unescape), HTE-69..75.
- llm slice merged (49dc78c): every llm_* function, llm_exec, and `internal/db` on native drivers (modernc sqlite, go-sql-driver/mysql, pgx) for llm_db_* and gash_db_test; core added gash_help, gash_db_list and gash_ai_list. Go mode: 329 passed of 807. Findings ledgered: TEST-13/14, DB-14..17, LLM-35..39. M5 (php, node, ai) goes to `wt/llm` after its DB follow-ups (DB-02 raw text, DB-16 socket).
- files slice merged (ba82e24): `internal/fsscan` and `internal/files` (the scanners and archive/backup). Findings ledgered FS-02..30. During this port the lead confirmed FS-50 on a 2.1.0 copy (tree_stats evaluates text from a scanned file name as Bash arithmetic; a tab in an extension shifts file-name text into the numeric fields). FS-50 is ledgered critical (Security, Forced divergences, mechanism-and-fix only) and reported to the operator; rc.A reads those fields as integers (`ui.Int`), never evaluated.
- FS-50 2.1.x patch prepared (local, unpushed) in `/mnt/project-files/security/gash-2.1.1/` in both series (on-v2.1.0 773/0, on-main 815/0, gates clean); the 3-fix series kept as `*-3fix-*.bak`. Pending operator decision D-7 on whether/when a 2.1.1 ships.
- LLM-37 decided (operator "soundest option" go-ahead): one engine (Go RE2), option (b) — any pattern not read exactly as rg 14 fails `invalid_regex` (RETRY), rg's own syntax errors included; the silent-empty on rg errors is the accepted forced divergence. Code: `invalid_regex` added to `internal/contract/codes.go` (38032ae); LLM-37 rc.A and LLM-17 amended (a53d312). `--ext`/`--glob` stay silent (no engine, no code). AGENTS.md section 5 is a doc-drift sweep for S3 (invalid_context isn't listed there either).
- sys slice merged (after ec2f7f4): `internal/system` (disk_usage, hgrep/history_grep, ip_public, mkdir_cd, port_kill, process_find/kill, services_stop, sudo_last, wsl_*), `internal/ssh` (gash_ssh_auto_unlock on runner.StartPTY), `internal/chrome` (chrome_headless_update on net/http), `internal/sysinfo` (overview, ten sections, six deep collectors, and the Begin/Collect/Deep/Sudo/Release API for ai_sysinfo). handlers table + Names unioned with devops (66 names). go vet/build/test green; the four packages pass. Still to transcribe into the LEDGER (worker report): three new defects — (1) deep_services WireGuard redaction only catches `PrivateKey = ` with spaces, so `PrivateKey=` and `PresharedKey` lines reach the AI provider in clear (medium, security; rc.A reproduces it); (2) deep_performance I/O scheduler line reads the queue dir name not the device (`dirname|xargs basename`); (3) deep collectors drop a file's missing final newline, gluing the next header on (`cat|head`). SYS-41's find-order Bash location should also list the deep collectors (openvpn, sysctl.d, netplan, collectd, nvidia, overrides, unbound, proftpd). SYS-42 txtar name fixed. Known systemic doc-drift: many SYS/DOCK entries name a proving test in `internal/cli/args`, which does not exist (arg parsing landed per-module, e.g. internal/db/args.go); S3 doc sweep. Core need relayed: RemoteSize returns 0 on a 4xx, but 2.x/curl read the 404's Content-Length (chrome nodownload scenario).
- devops slice merged (ec2f7f4): `internal/docker` (docker_stop_all/start_all/prune_all, docker_compose_check/upgrade/scan), `internal/gitx` (git_list_tags/add_tag/delete_tag/dump_revisions/apply_patch/repair/sync), `internal/remote` (remote_hosts/exec/push/pull/sync_push/sync_pull). handlers table + TestTableBuilds unioned (both sides, sorted, 48 names). go vet/build/test green; the three new packages pass. DOCK/GIT/REMOTE findings from the worker's report still to be transcribed into the LEDGER. Three defects relayed to core: runner.checkNoShell false positive (bash.go:117), pathguard.Realpath symlink-loop semantics vs GNU `realpath -m`, and exporting config.validHostTarget; plus the remote `/.` root classification (medium) to ledger.
- hte CLI slice merged (8e0829c) and wired by the lead (28a252c): `internal/hte/htecmd` is the `gash hte` cobra tree over the engine (M7 item 4) — install/uninstall/doctor/migrate, site/php/apache/service/cert/hosts, hidden `__root`/`__unit`/`__autostart`; `--json` one object, human text otherwise; one envelope + exit 1 on error, partial work printed first. Engine gained `BeforeSudo` and partial Install/Uninstall results. Lead wiring: `invoke.Table.HandleCommand/LookupCommand` + `cmds` map; `htecmd.Register`; `handlers.Table` registers it; `cli.go` hteCmd dispatches through `LookupCommand("hte")` (dropped the hte case from TestNotPortedCommands); the 8 pending HTE codes added to `contract/codes.go` (apache_module_missing, root_not_allowed, conflicting_options, invalid_state, openssl_not_found, php_not_enabled, invalid_ports, invalid_autostart) and `hteerr` pendingContract emptied; the 4 hte txtar scripts moved to `testdata/script` and the package-local harness removed, so `cmd/gash` runs them through the real binary (added a `root` euid condition to its harness). go vet/build/`go test ./...` green. Proving tests: TestLedger_HTE08/70..75, TestCLI*, htecmd TestEnvelope/EngineBuild/MissingArg, TestScript/hte_*, TestPartialResults. Deferred (worker note g, not this slice): `gash init bash` should background `gash hte __autostart` when autostart=shell; registry help text for the 3.x commands. M7 items 1-3 (hte/hostshelper/helper, E2E) were already on the branch.
- core slice merged (381b368): M1 self-management completed — `gash_uninstall` (staged protocol via stub) and `gash_unload` (pure-Bash stub reproducing 2.x; the 3.x `gash.sh.tmpl` now snapshots `__GASH_PRE_SYMBOLS` and the symbol helpers *before* the stub/alias surface, interactive-only, so non-`gash_`-prefixed module functions and aliases are swept like 2.x). Four cross-cutting fixes other workers relayed to core landed here: runner.checkNoShell rewritten into `noshell.go` (checks only argv[0] and what recognized wrappers run); pathguard.Realpath now matches GNU `realpath -m` symlink-loop semantics; `config.ValidHostTarget` exported; `ui.Downloader.RemoteSize` now returns an error response's Content-Length, matching 2.x `curl -fsIL` (verified empirically: `curl -fsIL` on a 4xx still prints the Content-Length header; the pipeline ignores curl's exit 22). Lead resolved the one cross-slice break this caused: `internal/chrome TestDownloadLookup` "No size anywhere" relied on the old RemoteSize returning 0 on the fallback 404; the mock's default 404 carries a 9-byte body (Content-Length 9), so with the faithful RemoteSize chrome proceeded. Fixed the test to drive the size genuinely to 0 (`HEAD zipURL` → 404 with an empty body), keeping the no-download branch covered and 2.x-faithful. go vet/build/`go test ./...` green.
- Pending LEDGER transcription from the core slice (lead, with the devops/sys backlog): (1) the RemoteSize / chrome size parity above — a 2.x `curl -fsIL` reads an error response's Content-Length, so a located URL that 404s with a size is treated as present and chrome proceeds to download (then fails); rc.A reproduces it. (2) the 3.x `gash_unload` snapshot is taken before the stub surface, not after (parity note). (3) STATE/architecture say "only two sanctioned `bash -c` uses"; there are now five (added BashDoctorLoad/Shell/Bare for `gash_doctor --timing`) — doc update. Open questions the worker flagged (not blocking rc.A, for the S2/S3 design pass): `gash_migrate`'s registered help still describes the 1.x->2.x migration while the ported function only points at the 2.x route (D-6); `gash_upgrade` is the 2.x flow, not the 3.6 binary-prefetch/rollback design nor a `gash update` command; the generated `gash.sh` lacks the 2.x daily migrate reminder (D-6 says it stays). SELF-03/SELF-05 leftovers can't be exercised until `gash init bash` is ported (M6). Many startup/settings spec rows still fail for the same unported-init reason.
- shell slice merged (9efb510): M6 complete — `internal/shellinit` (the `gash init bash` interactive layer: terminal detection, history settings, prompt, toolchains/PATH, the alias pack, banner), `internal/selfmgmt/banner.go`, `internal/gitx/gitlog.go` (gl/gla/glo/glg/gls/glf log family), `config.SplitPathAdd`. SHELL-01 forced divergence implemented and proven: the prompt stores git/venv/etc. values in holder vars (`__GASH_PS1_BRANCH` ...) and PS1 holds only `${__GASH_PS1_*}` references, so `${PS1@P}` never re-executes injected `$(...)` from a branch name (txtar `prompt_hostile_names.txtar`). `cli.initCmd` wired to `shellinit.New().Emit`. handlers Names unioned to add gl/gla/glf/glg/glo/gls (worker pre-updated handlers_test.go in-commit; no divergence, union a no-op for the lead). No new compat ops/stubs/registry. Benchmarks: 3.x startup 40.42ms vs 2.x 94.75ms, prompt faster; go-mode 572 passed / 235 failed, 0 PARITY regressions, +192 improvements. gofmt/vet/build/`go test ./...` (53 pkgs) green on go-refactor; Bash ref untouched (807/0). Pending LEDGER transcription / S2-S3 reconciliation from this slice: (1) the bench harness honours no `GASH_BIN`, so a benchmark run always builds its own binary (harness note, S3); (2) LEDGER SHELL-06 wording inconsistent with the ported behaviour (reconcile in the S2 LEDGER pass); (3) SHELL-13 text vs D-6 (the generated `gash.sh` daily migrate reminder — D-6 says the reminder stays, confirm SHELL-13 agrees); (4) `shell.md` §7 describes SSH auto-unlock at init, now handled by `internal/ssh` on runner.StartPTY — doc drift; (5) `gash_migrate`'s registered help / the init reminder text still mentions the 1.x->2.x path (same D-6 thread as the core-slice note); (6) SHELL-10/SHELL-13 want dedicated proving tests (add in the S2 LEDGER pass). None blocks rc.A.
- llm/dev/ai slice merged (182a83a): M5 complete — `internal/dev` (php_detect_version, node_detect_version; node_upgrade and nvm_prune_versions as Bash stub overrides since they drive nvm, a sourced shell function — DEV-16) and `internal/ai` (ai_ask, ai_query, the non-interactive ai_sysinfo and its tty-only interactive drill-down menu), plus the db follow-ups (DB-02 raw text, DB-16 socket) and LLM/DB ledger tests. Conflicts resolved by the lead: `handlers.go` auto-merged (ai+dev+htecmd all register); `handlers_test.go` Names unioned to 78 (self-mgmt + ai/dev); `compat/stubs.tmpl` unioned (core's gash_doctor/uninstall/unload + llm's node_upgrade/nvm_prune_versions); the generated `testdata/compat/gash.sh` and node.sh goldens regenerated via `go test ./internal/compatgen -update`. go vet/build/`go test ./...` green; go.mod/go.sum unchanged (ai/dev are stdlib-only); no lib/ changes (the "node.sh" the worker named is the testdata/compat golden). Pending LEDGER transcription from this slice: AI-08 (bare `:` port regex also matches clock times), AI-09 (ai_sysinfo `s` save writes unchecked, always prints success, bare filename), AI-10 (missing `.sections` -> empty summary counts), AI-18 (whole trailing-punctuation run stripped from paths). Fidelity notes: node_upgrade/nvm_prune_versions are Bash stubs (nvm is a shell function, not a PATH binary); the 2.x spinners (__gash_spinner_*) have no Go primitive so ai_sysinfo omits them (consistent with the merged non-interactive path); the ai_sysinfo interactive menu is tty-only so go-mode shell specs never reach it (unit tests are the proving mechanism). ai_spec.sh go-mode is 17 passed / 18 failed, all bash-reference-only (the spec sources internal `__ai_*` shell functions that don't exist in Go mode) — same accepted pattern as sysinfo_spec, baseline unchanged by this work.

## Next step

M0 is merged (76310d1): Bash reference 807/0, Go mode 202 passed of 807. The module wave runs in parallel worktrees, each owning its packages (shared packages belong to `wt/core`):

| Worktree | Slice |
|---|---|
| `wt/core` (M1) | output primitives and compat operations first, then lib/core, gash_help and the card, TEST-03/07, Bash 4.4 load check of the generated Bash in CI |
| `wt/llm` (M2) | llm_*, llm_exec wiring, internal/db native drivers, gash_db_test |
| `wt/files` (M3) | lib/modules/files.sh |
| `wt/devops` (M4b) | docker, compose, git, remote |
| `wt/sys` (M4a) | system, sysinfo, ssh, chrome |
| `wt/cfg` | SQL guard DB-13 (strictest of both locales) |
| `wt/hte` | HTE remainder (certs, hosts, supervision, receipt, migrate --apply, doctor, E2E) |

Merged: M1-M7 (all module slices, self-management, HTE CLI wiring) and M6 (shell layer) are all in go-refactor now. Still to start: cutover (M8: go-impl to the root, the "only internal/runner starts processes" check, Bash framework removed, docs/AGENTS.md, CI/release workflows, VERSION 3.0.0-rc.A, PARITY.md final), and the LEDGER transcription backlog (devops DOCK/GIT/REMOTE + 3 defects; sys 3 defects + SYS-41; core RemoteSize/unload-snapshot/5-bash-c-uses; llm AI-08/09/10/18; shell's 6 notes above). Spec changes are allowed only for TEST-03..07 as the LEDGER says, expectations unchanged.

Wiring notes for M1 and later:
- llm_exec's registry entry lists `BASH` among the shell variables it reads, so the stub exports the caller's `$BASH` (named in `GASH__ADDED`, kept out of children); `llmexec` uses it as `Environ.BashPath` and falls back to a PATH lookup, because 2.x ran `${BASH:-bash}` and bash prefixes its errors with that path.
- `runner` refuses any `sh -c`-style start except `BashLLMExec` and `BashNVMBridge`. `wsl_compact` follows SYS-46; `gash_doctor --timing` (fixed script) and `sudo_last` (stays in the shim) are decided by their module workers.
- `runner` divergences to ledger where a module makes them visible: a shebang-less script gets 126, a background grandchild holding stdout is cut after KillAfter, a timeout signals only the child.
- SSH unlock needs a PTY (creack/pty, M4).
- M8 adds a check that only `internal/runner` starts processes. HTE now starts everything through `internal/runner` (the runner gained `Options.Setsid/Credential/FileCapture`, the `Credential` type, `Session/SessionStarter/Exec.StartSession`, and `Call.Session/Setsid/Credential/Background/PTY`); `internal/hte/proc` is deleted. Still outside the runner: `internal/llmexec/probe.go`, `internal/selfmgmt/proc.go`.

## Plan to rc.A (stop S2)

| Milestone | Content | Done when |
|---|---|---|
| M0 Foundation | `go-impl/go.mod` with the pinned stack (`libraries.md`), `cmd/gash`, `internal/{app,term,contract,registry,cli}` skeleton, `internal/compatgen` (gash.sh, the 7 shims, `lib/compat/bootstrap.sh`, `bin/gash`, `bash_completion`), `gash __compat stage`, `tests/run.sh --impl bash\|go\|both`, a Go job in CI | `go test ./...` green; `tests/run.sh --impl go` runs every spec against a staged tree (failures expected) |
| M1 Core | `config`, `pathguard`, `runner`, `ui` primitives byte for byte, help and card from the registry, `platform` | core, output, config and help specs pass in Go mode |
| M2 Agent API | `llm`, `llmexec`, `sqlguard`, `db` | llm, exec and db specs pass or map to a forced divergence |
| M3 Files | `fsscan`, `files` | files specs |
| M4 System and devops | `system`, `sysinfo`, `ssh`, `docker`, `gitx`, `remote` | their specs |
| M5 Dev, AI, self-management | `dev`, `ai`, `selfmgmt` | their specs; upgrade scripts of `upgrade.md` 6 |
| M6 Shell layer | `shellinit` complete; prompt and startup benchmarks against 2.x | shell specs; prompt not slower than 2.x |
| M7 HTE | `hte`, `hostshelper`, `cmd/gash-hosts-helper`; E2E on Debian/Ubuntu containers here, Fedora/Arch/macOS in CI | PRD 11.1 HTE items |
| M8 Cutover | `go-impl/` to the root, testscript suite and schemas, Bash framework removed, docs and `AGENTS.md`, CI and release workflows, `VERSION` `3.0.0-rc.A`, `PARITY.md` final | PRD 11.1 all true, then stop S2 |

M2 to M5 and M7 run in parallel after M1, each in its own git worktree
(`git worktree add ../wt-<name> -b wt/<name> go-refactor`), merged into
`go-refactor` by the lead session; only `go-refactor` is pushed.

## Gate

Bash reference: 807 passed, 0 failed (sqlite3 and gawk installed). Until M0 lands, the Go part is `go vet` and `go test`:

```bash
bash tests/run.sh                         # reference implementation (Bash)
bash tests/run.sh --impl go               # same specs against the Go port behind the generated shims
(cd go-impl && GOTOOLCHAIN=go1.27.2 CGO_ENABLED=0 go vet ./... && GOTOOLCHAIN=go1.27.2 CGO_ENABLED=0 go test ./...)
```

After any edit of `MATRIX.md`, run `go generate ./internal/registry` and commit `catalog_gen.go` and the compat goldens it changes in the same push: `TestCatalogIsCurrent` renders the catalog from the 2.x sources and MATRIX and fails on a stale file.

## Environment blockers (cloud session)

| Blocker | Workaround |
|---|---|
| Docker Hub anonymous pulls return 429 | Pull through `mirror.gcr.io/library/<image>` (works for ubuntu, debian, fedora, archlinux) |
| `quay.io`, `public.ecr.aws` (data limit) refused | Use `mirror.gcr.io` |
| `ppa.launchpadcontent.net` (ondrej/php), `packages.sury.org`, `rpms.remirepo.net` blocked | Multi-PHP E2E here from official `php:X.Y-fpm` images copied side by side; distro multi-PHP repositories only in GitHub Actions |
| Fedora and Arch package mirrors blocked (dnf/pacman fail inside containers) | Fedora/Arch E2E only in GitHub Actions |
| `deb.debian.org` returns 403 inside containers | Debian HTE E2E only in GitHub Actions |
| `api.deps.dev`, `api.securityscorecards.dev`, `libraries.io` blocked | Module status from proxy.golang.org (`@v/list`, `.mod` deprecation comments) and pkg.go.dev |
| No Docker daemon at session start | `nohup dockerd >/tmp/claude-0/dockerd.log 2>&1 &` |
| Installed Go is 1.24.7 | `GOTOOLCHAIN=go1.27.2` (checksum verification stays on) |
| Tag pushes are rejected by the proxy | Tags are the operator's |
| github.com web and API return 403 to plain HTTPS from the session | GitHub through the MCP tools; archived-flag checks of the libraries run in CI |
| `vuln.go.dev` blocked | govulncheck runs in CI |
| The per-module audit notes of S1 live only in the session and are lost when the VM is reclaimed | Everything later steps need is in `MATRIX.md`, `LEDGER.md` and `go-impl/design/`; the Bash source stays the reference |
| Running Gash self-management functions with Gash sourced from the working checkout acts on the checkout itself (`gash.sh` sets `GASH_DIR` to its own directory): one audit run checked out `v2.1.0` here | Experiments source a copy (`git archive go-refactor \| tar -x -C DIR`) or a scratch clone, never this checkout |

## Open questions for the operator

The S1 decisions D-1 to D-9 in `go-impl/design/README.md` (each with a recommendation the work proceeds on unless the operator says otherwise). D-7 (2.1.x security release outside `go-refactor`) needs the operator: a push outside `go-refactor` and a tag are stop conditions.
