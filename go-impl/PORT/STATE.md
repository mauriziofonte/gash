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

Still to start: php/node/ai (M5), self-management (gash.sh upgrade/uninstall/unload/doctor, gash_env_init, the gash_migrate pointer), shell layer (M6), HTE CLI wiring (M7), cutover (M8). Spec changes are allowed only for TEST-03..07 as the LEDGER says, expectations unchanged.

Wiring notes for M1 and later:
- llm_exec's registry entry lists `BASH` among the shell variables it reads, so the stub exports the caller's `$BASH` (named in `GASH__ADDED`, kept out of children); `llmexec` uses it as `Environ.BashPath` and falls back to a PATH lookup, because 2.x ran `${BASH:-bash}` and bash prefixes its errors with that path.
- `runner` refuses any `sh -c`-style start except `BashLLMExec` and `BashNVMBridge`. `wsl_compact` follows SYS-46; `gash_doctor --timing` (fixed script) and `sudo_last` (stays in the shim) are decided by their module workers.
- `runner` divergences to ledger where a module makes them visible: a shebang-less script gets 126, a background grandchild holding stdout is cut after KillAfter, a timeout signals only the child.
- SSH unlock needs a PTY (creack/pty, M4).
- M8 adds a check that only `internal/runner` starts processes. Still outside it: `internal/hte/proc` (moves onto runner: credentials, new session, pipes), `internal/llmexec/probe.go`, `internal/selfmgmt/proc.go`.

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
