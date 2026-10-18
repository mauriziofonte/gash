<!-- Gash 3.0 port state. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Port state

Read this first in every session, then `git log --oneline -20`, then run the gate.

## Phase

S1 (design). No production Go is written before the operator's go-ahead in the session.

## Last completed step

- Branch `go-refactor` created from `main` (a4fbc8b) and pushed.
- Bash baseline: `bash tests/run.sh` → 800 passed, 0 failed, 7 skipped (807) in 33 s on the cloud VM (4 CPU, Bash 5.2.21, mawk, no gawk/sqlite3).
- Go 1.27.2 obtained through the toolchain mechanism (`GOTOOLCHAIN=go1.27.2`); `encoding/json/v2` importable without GOEXPERIMENT and rejects invalid UTF-8 by default.
- Userland Apache + PHP-FPM spike green on an Ubuntu 24.04 container (non-root master on 127.0.0.1:80/443, HTTP/2, proxy_fcgi, graceful reload).
- S1 design written: `go-impl/design/` (README with the decisions requested, architecture, layout, compat, shell, upgrade, libraries, testing, hte), `PARITY.md` (all 1095 spec tests), prompt and startup benchmarks with the 2.x numbers (`go-impl/bench/shell/`), legacy hte-cli fixtures (`go-impl/testdata/hte/legacy/`).
- Audit of every 2.x module done; `MATRIX.md` and `LEDGER.md` are generated from per-module fragments.

## Next step

Open the S1 draft PR, then wait for the operator's go-ahead in the session. After it: apply the answers to the decisions (D-1 to D-8 in `go-impl/design/README.md`), then start the Go module (`go-impl/go.mod`, registry, contract, compat generator, `tests/run.sh --impl`).

## Gate

During S1 (no Go yet):

```bash
bash tests/run.sh                         # Bash reference: must stay 800 passed / 0 failed
```

From the first Go commit on (after S1 approval):

```bash
bash tests/run.sh                         # reference implementation (Bash)
bash tests/run.sh --impl go               # same specs against the Go port behind the generated shims
(cd go-impl && GOTOOLCHAIN=go1.27.2 go test ./...)
```

## Environment blockers (cloud session)

| Blocker | Workaround |
|---|---|
| Docker Hub anonymous pulls return 429 | Pull through `mirror.gcr.io/library/<image>` (works for ubuntu, debian, fedora, archlinux) |
| `quay.io`, `public.ecr.aws` (data limit) refused | Use `mirror.gcr.io` |
| `ppa.launchpadcontent.net` (ondrej/php), `packages.sury.org`, `rpms.remirepo.net` blocked | Multi-PHP E2E here from official `php:X.Y-fpm` images copied side by side; distro multi-PHP repositories only in GitHub Actions |
| Fedora and Arch package mirrors blocked (dnf/pacman fail inside containers) | Fedora/Arch E2E only in GitHub Actions |
| `api.deps.dev`, `api.securityscorecards.dev`, `libraries.io` blocked | Module status from proxy.golang.org (`@v/list`, `.mod` deprecation comments) and pkg.go.dev |
| No Docker daemon at session start | `nohup dockerd >/tmp/claude-0/dockerd.log 2>&1 &` |
| Installed Go is 1.24.7 | `GOTOOLCHAIN=go1.27.2` (checksum verification stays on) |
| Tag pushes are rejected by the proxy | Tags are the operator's |
| github.com web and API return 403 to plain HTTPS from the session | GitHub through the MCP tools; archived-flag checks of the libraries run in CI |
| `vuln.go.dev` blocked | govulncheck runs in CI |
| The per-module audit notes of S1 live only in the session and are lost when the VM is reclaimed | Everything later steps need is in `MATRIX.md`, `LEDGER.md` and `go-impl/design/`; the Bash source stays the reference |
| Running Gash self-management functions with Gash sourced from the working checkout acts on the checkout itself (`gash.sh` sets `GASH_DIR` to its own directory): one audit run checked out `v2.1.0` here | Experiments source a copy (`git archive go-refactor \| tar -x -C DIR`) or a scratch clone, never this checkout |

## Open questions for the operator

The S1 decisions D-1 to D-8 in `go-impl/design/README.md` (each with a recommendation the work proceeds on unless the operator says otherwise). D-7 (2.1.x security release outside `go-refactor`) needs the operator: a push outside `go-refactor` and a tag are stop conditions.
