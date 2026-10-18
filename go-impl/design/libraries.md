<!-- Gash 3.0 design: library stack. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Library stack (PRD 9)

Checked on 2026-10-18 with go1.27.2, `CGO_ENABLED=0`, against
proxy.golang.org (`@v/list`, `.info`, `.mod`). "Latest stable" is the highest
`vX.Y.Z` without a prerelease suffix; no pinned module carries a
`// Deprecated:` comment. A test program that imports and calls every PRD
library resolved with `go mod tidy` (223 modules in the graph, 107 linked, no
conflicts), built a 35 MB static ELF and cross-compiled for darwin/arm64 and
linux/arm64.

Not verifiable from the cloud session (recorded in `STATE.md`): GitHub's
"archived" flag (github.com returns 403 to this session; pkg.go.dev does not
show it), deps.dev scorecards, and the vulnerability database
(`vuln.go.dev` blocked, so govulncheck builds but cannot scan here). As a
maintenance signal the default branch's latest commit time from the proxy
was used. CI runs govulncheck on every push; the first CI run on the branch
closes the gap, and the archived flag is checked there through the GitHub
API before rc.A.

## 1. Pinned (used)

| Area | Module | Version | Status | Use |
|---|---|---|---|---|
| Toolchain | Go | 1.27.2 | `encoding/json/v2` on by default (`GOEXPERIMENT=nojsonv2` breaks the build) | `go 1.27` in go.mod, `toolchain go1.27.2` |
| CLI | `github.com/spf13/cobra` | v1.10.2 | active | command tree from the registry (`architecture.md` 3) |
| JSON | `encoding/json/v2`, `encoding/json/jsontext` | stdlib | | the contract encoder sits on `jsontext` (token-level control, number text kept) |
| YAML | `go.yaml.in/yaml/v3` | v3.0.5 | active (YAML org) | `llm_config` YAML conversion, compose fallback parsing where 2.x had the awk subset is ported as a line parser instead (`architecture.md` 7) |
| TOML | `github.com/BurntSushi/toml` | v1.6.0 | active | HTE site state and settings |
| Templates | `text/template` | stdlib | | Apache and FPM rendering, golden files |
| Databases | `github.com/go-sql-driver/mysql` | v1.10.1 | active | `llm_db_*`, `gash_db_*` |
| | `github.com/jackc/pgx/v5` (`stdlib`) | v5.11.0 | active | same |
| | `modernc.org/sqlite` | v1.60.1 | active (SQLite 3.53.4, pure Go) | same; also replaces the `sqlite3` client |
| System info | `github.com/shirou/gopsutil/v4` | v4.26.9 | active | `sysinfo`, `llm_procs`, `llm_ports` fields where 2.x parsed tool text |
| XDG | `github.com/adrg/xdg` | v0.5.3 | maintained, slow tags (last tag 2024-10, commits 2026-09) | HTE and binary store paths |
| Atomic writes | `github.com/google/renameio/v2` | v2.0.2 | active, POSIX only (fine: linux and darwin targets) | rendered configs, receipts, state |
| Locks | `github.com/gofrs/flock` | v0.13.1 | active | HTE lock, binary fetch lock in Go |
| Shell quoting | `mvdan.cc/sh/v3/syntax` | v3.14.1 | active | `gash init bash`, compat generation, `__compat env load` |
| PTY | `github.com/creack/pty` | v1.1.24 | maintained, slow tags | `gash_ssh_auto_unlock` (ssh-add) |
| Concurrency | `golang.org/x/sync` (`errgroup`) | v0.24.0 | active | HTE parallel health checks |
| Terminal | `golang.org/x/term`, `golang.org/x/sys` | latest at implementation start | official | TTY checks, window size, `statfs`, ioctls (addition: PRD lists none for this) |
| Tests | `github.com/stretchr/testify` (`require`) | v1.12.1 | active | |
| | `github.com/google/go-cmp` | v0.7.0 | maintained, slow tags | |
| | `github.com/rogpeppe/go-internal/testscript` | v1.16.0 | active | black-box suite |
| | `github.com/santhosh-tekuri/jsonschema/v6` | v6.0.3 | active | envelope schemas |
| | `github.com/spf13/afero`, `testing/fstest` | v1.15.0, stdlib | active | filesystem fakes |
| Quality | `golangci-lint` v2 | v2.15.0 (Go 1.27 support since v2.13.0) | active | release binary in CI, not `go install` (upstream advice) |
| | `golang.org/x/vuln` (govulncheck) | v1.9.0 | active | CI |
| Release | `goreleaser` | v2.18.3 | active | `.goreleaser.yaml`, tags only |

After the first `go mod tidy` the build runs `go get -u ./...` once so that
minimal version selection does not link old indirect modules (with the full
PRD stack MVS picked x/crypto v0.40.0 and x/net v0.41.0 while v0.58.0 and
v0.61.0 exist); govulncheck gates CI.

## 2. Deviations (not used), with the reason

| PRD module | Decision | Reason |
|---|---|---|
| `charm.land/fang/v2` v2.0.1 | Not used | 2.x help text and error lines are contract (`--help` bodies, `Error:`/`Hint:`, envelopes). fang reformats both (uppercase titles, lines padded to 120 columns, errors capitalized with a period) and has no option to set a profile or writer. On a PTY that does not answer the OSC 11 background query (agent harnesses, `script`, some CI) every help or error call stalls 4.03 s and leaks the query bytes into the output (measured), with `--no-color` too. Plain cobra with Gash's own help printer |
| `charm.land/lipgloss/v2` v2.0.6 (table, tree, list) | Not used | `tests/specs/output_style_render_spec.sh` rule R3 allows only the 2.x palette SGRs, attributes and `\e[0m`; lipgloss renders truecolor and resets with `\e[m` (measured: `\e[1;38;2;255;0;0mRAW\e[m`). Tables, boxes and lists are 2.x primitives reimplemented byte for byte in `internal/ui`, used by new commands too, so 3.x keeps one design system |
| `github.com/charmbracelet/colorprofile` v0.4.3 | Not used | 2.x color is on or off, never downsampled; `NO_COLOR` in colorprofile only drops to ASCII (bold and underline stay), which contradicts the 2.x gate. One gate in `internal/term` (`architecture.md` 5) |
| `charm.land/huh/v2` v2.0.3 | Not used | Prompts are 2.x primitives (`__gash_choose`, confirm lines) with pinned output; new commands prompt only for a missing flag on a TTY, with the same primitives |
| `charm.land/log/v2` v2.0.1 | Not used | Diagnostics have the 2.x `__gash_debug` format; `log/slog` with a small handler writes it |
| `github.com/moby/moby/client` v0.6.2, `.../api` v1.56.1 | Not used | Every docker operation of 2.x goes through the `docker` CLI, whose output Gash passes through (`docker system prune`, `compose pull/up/ps`); Compose has no Go API at all. Keeping the CLI keeps the user's contexts, config and credential helpers and byte-identical output. The client is also pre-1.0 and adds about 3.6 ms of start-up to every `gash` process (with go-containerregistry) |
| `github.com/google/go-containerregistry` v0.22.1 | Not used in rc.A | The 2.x registry protocol (token endpoints, the four Accept headers, HEAD only, error taxonomy `dns_error` ... `no_digest_in_response`, https everywhere, no Docker credential store) is ported over `net/http`. ggcr changes each of these by default (challenge auth, `~/.docker/config.json`, plain HTTP for localhost, stricter reference parsing). Its fixes (digest-pinned refs, `docker.io/<name>`, `GITHUB_TOKEN` exchange, challenge auth for other registries) are LEDGER items; the fix recipes use ggcr, so it enters in 3.0 if those are accepted |
| `github.com/anthropics/anthropic-sdk-go` v1.82.0, `google.golang.org/genai` v1.75.0 | Not used | Start-up cost lands on every `gash` call, not only AI: the full stack starts in 11.4-13.0 ms against 2.0 ms for a bare binary, and genai alone is 8.2 ms (grpc, otel, cloud auth); anthropic-sdk-go links a prerelease `go.yaml.in/yaml/v4` (rc). Parity: 2.x builds each request itself (model from settings, prompt layout, redaction first, one attempt); the SDKs retry by default, which would send the user's data twice. Both APIs are one HTTPS POST with a JSON body: `internal/ai` ports the 2.x requests and error mapping over `net/http` |
| `github.com/dustin/go-humanize` v1.1.0 | Not used | 2.x size and duration formats (`512.0 KiB`, `580.3 KiB`, `32.5ms`, `N.NNs`) differ from humanize's; one small formatter reproduces them |
| `log/slog` with `charm.land/log/v2` as TTY handler | `log/slog` only | See log above |

Expected start-up after these deviations: about 5-7 ms per call (cobra,
database drivers, gopsutil); the benchmark test in `architecture.md` 1 keeps
`gash version` under 10 ms.

## 3. Additions not in the PRD list

| Module | Why |
|---|---|
| `golang.org/x/term`, `golang.org/x/sys` | TTY tests on the exact file descriptors, window size, `statfs` for `disk_usage` and the scanners, `ioctl` for the PTY driver |
| (none for gitignore) | The content search follows ripgrep's ignore rules (`.gitignore` inside a git repository, `.ignore`, `.rgignore`, `.git/info/exclude`, `core.excludesFile`); a small matcher in `internal/fsscan/ignore`, implemented from gitignore(5) and checked against `git check-ignore` and `rg --files` on a fixture corpus, avoids pulling go-git for one package |

## 4. Explicitly out (PRD 9)

Full-screen TUIs, `gopkg.in/yaml.v3` (its README says "THIS PROJECT IS
UNMAINTAINED"; it appears in the module graph only as a test dependency of
other modules and is never linked), `github.com/docker/docker` (deprecated
from v29; the proxy has nothing after v28.5.2+incompatible), CGO.

## 5. Module path notes

* Charm v2 modules declare `charm.land/...`; `github.com/charmbracelet/.../v2`
  fails with a module path mismatch (verified for fang, lipgloss, huh, log).
  `colorprofile` is the reverse: only `github.com/charmbracelet/colorprofile`
  is correct. Recorded for the record, since none of them is used.
* `encoding/json/v2` refuses invalid UTF-8; `jsontext.AllowInvalidUTF8(true)`
  replaces bad bytes with U+FFFD. Where 2.x let raw bytes through, the
  contract encoder writes them itself (`architecture.md` 4), and file names
  are carried as bytes, never as `string` round-trips through v2.
