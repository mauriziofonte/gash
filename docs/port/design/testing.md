<!-- Gash 3.0 design: test strategy. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Test strategy

## 1. Starting point (measured at S1)

* `bash tests/run.sh`: 800 passed, 0 failed, 7 skipped (807) in 33-37 s; the 7
  skips need `sqlite3` (6) and `gawk` (1), which CI installs (807/0/0 once
  both are present). The slow `output_style_render_spec.sh` passes 288/288
  in 472 s on 4 CPUs.
* All 1095 tests were classified one by one (`PARITY.md`):

  | Scope | BLACKBOX | MOCK-DEPENDENT | MIXED | INTERNAL | Total |
  |---|---|---|---|---|---|
  | normal run (56 specs) | 343 | 55 | 62 | 347 | 807 |
  | render spec | 135 | 96 | 57 | 0 | 288 |

  BLACKBOX: public surface only. MOCK-DEPENDENT: public surface, but relies on
  a fake (a `tests/mocks/bin` stub, a PATH stub, or a Bash function that
  overrides a command). MIXED: public assertions plus an internal probe or a
  Bash seam. INTERNAL: calls `__gash_*`/`__llm_*` helpers or checks source
  text.

## 2. During the port: one spec suite, two implementations

### 2.1 Selector

`tests/run.sh --impl bash|go|both` (env `GASH_TEST_IMPL`, default `bash`):

* `bash`: as today; it stays the reference and must stay 807/0.
* `go`: `go build` the binary, then `gash __compat stage --out DIR` writes a
  shim tree with the 2.x layout (generated `gash.sh`, `bash_completion`, the 7
  shim paths, one generated `lib/modules/<module>.sh` per 2.x module file that
  defines that module's stubs, `lib/aliases/` and the prompt and toolchain
  Bash that stay Bash, links to `VERSION`, `quotes/`, `docs/`,
  `.gash_env.template`, `install.sh`, `tests/`). `GASH_TEST_ROOT` points
  there and `GASH_BIN` at the build. All three loader styles of the specs
  (`source gash.sh`, `gash_source_all`, `source lib/...` by path: 298 tests
  source files by path) then work unchanged, and the static specs lint the
  generated Bash, which must pass rules S1-S12 too.
* `both`: runs the two and prints the parity matrix (tests green in Bash and
  red in Go).

`gash-test.sh` reads `tests/parity/bash-only.tsv` (test name → the Go unit test
that replaces it, generated from the INTERNAL rows) and reports those as
`n/a` in Go mode, in a counter of their own, so real skips stay visible.
`gash_impl_is go` lets a MIXED test branch where splitting it would only
duplicate it.

### 2.2 Gate

```bash
bash tests/run.sh                          # Bash reference, 807/0 with sqlite3 and gawk
bash tests/run.sh --impl go                # same specs, Go behind the shim
(cd go-impl && GOTOOLCHAIN=go1.27.3 go test ./...)
```

### 2.3 Seams a binary cannot honor

| Seam in 2.x specs | Go-mode replacement |
|---|---|
| `needs_confirm_prompt(){...}` overrides (23 tests) | `--yes` where the function has it; else answers fed on a PTY (`script`) |
| `__gash_use_color`, `__gash_fs_rich_ok`, `__gash_stderr_fancy` overrides (21) | Run under a PTY with `COLUMNS` set. No new "force color" variable (it would be new public surface) |
| `__gash_require_wsl` (15) | A WSL fixture root (`platform.Sysroot` with `/proc/sys/fs/binfmt_misc/WSLInterop` and `/proc/version`), set through the test-only `GASH__SYSROOT` variable that only test builds (`-tags gashtest`) read |
| `__sysinfo_ensure_sudo` (5) | PATH `sudo` stub failing `sudo -n true` |
| `__llm_has_rg` (2) | Moot: one search engine (`architecture.md` 7); the specs run once |
| `kill(){...}` with `pgrep` faked to return PID 2222 (`functions_edge_spec.sh:87`, `functions_more_spec.sh:226`, 3 render fixtures) | **Hazard**: a binary calling `kill(2)` would signal a real PID 2222. These tests are rewritten around a real victim (`sleep 300 & pgrep stub returns $!`), as `functions_smoke_spec.sh:218` already does, before Go mode runs them; every Go-mode run happens in a PID and network namespace (`unshare -pnf --mount-proc`, or a container in CI) as a second barrier |
| Bash-function fakes of external commands in render fixtures (52 of 286: `curl(){...}`, `wget`, `ssh`, `scp`, `rsync`) | A fixture helper writes them as PATH stubs; the Go-mode run has no network, so a missed fake fails instead of reaching the internet |
| `history`, `fc`, `nvm` overrides | Unchanged: those stay in the caller's shell (the shim reads `fc` and calls `nvm` there) |
| `__GASH_ENV_AI_PROVIDERS` injection | An `AI:<provider>=` line in the temporary `~/.gash_env` (mode 600) |
| `__GASH_ENV_LOADED=` cache resets | No-op (Go reads the file on every call) |

Each rewrite changes how a test reaches the code, never what it expects
(MUST NOT 2); `PARITY.md` lists them.

### 2.4 Fakes Go no longer calls

Where 3.0 replaces a CLI by an in-process implementation (`architecture.md`
7), its `tests/mocks/bin` stub is bypassed:

| Fake | Replaced by | Tests affected | Go-mode treatment |
|---|---|---|---|
| `curl` (AI, registries, chrome, `ip_public`) | `net/http` | ~51 normal, 59 render | A local HTTPS fake reached through `HTTPS_PROXY`, with its CA in `SSL_CERT_FILE` (both honored by Go's standard library on Linux), serving the same canned responses as `tests/mocks/bin/curl`. Assertions on curl's argv become `n/a` with a Go test on the request (headers, body, no secret outside the request) |
| `mysql`, `mariadb`, `psql`, `sqlite3` | native drivers | 7 argv tests + the 6 sqlite3 tests | sqlite3 tests run unchanged (in-process engine, same files); argv tests (password never in argv, SQL on stdin) become `n/a`, replaced by Go tests proving no child process exists; MySQL and PostgreSQL behavior proven by Go integration tests against real servers in containers (`mysql:8.4`, `mariadb:11`, `postgres:16`) |
| `yq` | `go.yaml.in/yaml/v3` | `llm_portability_spec.sh:92` (flavor matrix) | `n/a`; the output layout is pinned by a golden test |
| `numfmt`, `env -0`, `realpath -m` | native | 4 | `n/a` (moot) |

CLIs that Go keeps executing (git, docker and compose, ssh/scp/rsync,
ssh-add, systemctl, journalctl, `ps`/`pgrep`/`lsof`, `ss`/`netstat`, `df`
where 2.x showed or parsed its output) keep their stubs, so those
MOCK-DEPENDENT tests run unchanged against Go, and the dependency errors
(`ports_tool_not_found`, `port_kill` without lsof, ...) stay reachable.

### 2.5 Harness defects found at S1 (LEDGER, fixed in the harness, not in Gash)

* `run.sh:165` forwards the caller's `NO_COLOR` into the hermetic test
  environment: with `NO_COLOR=1` exported, 5 tests fail.
* `-j1` is refused (only `-j 1`).
* 88 `|| exit 77` skips print no reason.
* `llm_spec.sh:198-264` uses the repository's own `lib/` as a fixture tree;
  it moves to a private fixture.
* 18 specs source Gash at top level in the runner process (dead weight, and
  a load error kills the whole file).

These are fixed on the branch before Go mode starts (test-harness changes,
expectations untouched), each with its LEDGER id.

## 3. Unit tests (Go)

* Every INTERNAL spec (347) maps to a named Go test; its vectors are kept
  verbatim as tables: the SQL guard table, the `llm_exec` refusal and walk
  cases, the config grammar cases, the secret and forbidden path matrices,
  the output primitives (byte-exact strings with and without color), the
  argument grammars.
* The audit's reproductions become regression tests in two halves: rc.A
  proves the 2.x behavior is reproduced (named `TestLedger_<id>_2x`), 3.0
  proves the accepted fix (`TestLedger_<id>_fix`, the "proving test" of the
  LEDGER entry), replacing the first.
* Fakes injected through `internal/app`: process runner, clock, environment
  snapshot, filesystem (`afero`, `testing/fstest`), sysroot. No test touches
  the real `~/.gash`, `~/.gash_env`, system Apache, PHP or hosts file.

## 4. Black-box suite (testscript)

* `testdata/script/*.txtar`, run by `github.com/rogpeppe/go-internal/testscript`
  with the built binary and the generated shim on PATH, a temporary `HOME`,
  `bash` available (the shim is exercised through `bash -c 'source ...'`
  exactly as agents and users call it). Golden files with `-update` support.
* Ported from the BLACKBOX specs during the port, so that at cutover the
  Bash framework can go: every BLACKBOX and MIXED public assertion exists as
  a testscript case before `tests/` is removed, checked by a script that maps
  each spec test name to a txtar case.
* Contract checks: every JSON output and error envelope validated against
  `testdata/schema/*.json`; color gating matrix (TTY, pipe, `NO_COLOR`,
  `GASH_NO_COLOR`, `GASH_HEADLESS`, `--no-color`) on every human function;
  `set -euo pipefail` callers; `declare -F` and alias parity with the 2.x
  inventory captured at S1.
* The render spec's R1-R6 rules (SGR set, widths, stream policy) become a
  testscript family driven by the same 286 fixtures.

## 5. Upgrade and compatibility tests

The scripts of `upgrade.md` section 6 (2.x install → `gash_upgrade` → 3.x,
headless and interactive; no binary; rollback to `v2.1.0`;
`gash update` with a broken target), built from fixture clones with local
tags and a `file://` release directory. They run in CI on Linux and macOS,
and on Bash 4.4, 5.0, 5.1 and 5.3 (the compat Bash must load on 4.4).

## 6. HTE

* Unit: rendering golden files (vhost variants, pools per profile, httpd and
  FPM globals per platform), the hosts helper parser (every refusal path),
  the sudoers line, the receipt round trip, migrate classification on the
  legacy fixtures (V1, V1-fixed, V2, Broken-bypass, Malformed, raw vhosts,
  duplicates, shared pools, custom ports), install then uninstall on a
  fixture sysroot compared byte for byte.
* E2E in containers without systemd, real Apache and real PHP-FPM, as an
  unprivileged user: Debian 12 and Ubuntu 24.04 (multi-PHP), Fedora (distro
  plus Remi), Arch (`php` and `php-legacy`). In the cloud session only the
  Debian/Ubuntu images are reachable (`STATE.md`), with multi-PHP assembled
  from official `php:X.Y-fpm` images; Fedora and Arch run in CI.
* macOS: GitHub Actions runner with Homebrew `httpd` and PHP, including the
  port mechanism check.
* WSL2: the operator's manual checklist (`docs/hte/wsl-checklist.md`).

## 7. CI (GitHub Actions)

| Job | Content |
|---|---|
| lint | `golangci-lint` v2 (release binary), `go vet`, shellcheck and shfmt on the generated Bash, `gash __compat generate --check`, docs generation check |
| vuln | `govulncheck ./...` |
| unit | `go test -race ./...` on ubuntu-latest and macos-latest |
| specs (until cutover) | `tests/run.sh --impl bash` and `--impl go`, plus the render spec, on Linux; Bash version matrix 4.4/5.0/5.1/5.3 for the compat layer |
| testscript | Linux and macOS |
| upgrade | section 5 |
| hte-e2e | container matrix of section 6: Debian 12, Ubuntu 24.04, Fedora (Remi), Arch; multi-PHP on Ubuntu, systemd on Debian and Ubuntu |
| hte-macos | section 6 natively on a macOS runner (Homebrew `httpd`, `php`, `php@8.3`) |
| snapshot | `goreleaser release --snapshot --clean` (no publish) |
| release | `.github/workflows/release.yml`, on tags only |

## 8. What this replaces at cutover

`tests/run.sh`, `tests/gash-test.sh`, `tests/mocks/bin` and `tests/specs/`
are removed at cutover (PRD 2), after `PARITY.md` shows every reusable spec
passing against Go or mapped to a forced divergence, and every public
assertion present in testscript.
