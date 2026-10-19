<!-- Gash 3.0 port: testscript coverage-declaration convention. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Testscript coverage declarations

The black-box suite lives in `go-impl/testdata/script/*.txtar`, run by
`github.com/rogpeppe/go-internal/testscript` against the built binary and the
generated 2.x shim (`design/testing.md` §4). Before the Bash `tests/`
framework can be removed at cutover (§8), **every public 2.x assertion must
have a black-box home here.** The universe of public assertions is every
`PORT/PARITY.md` row whose Plan is `run`, `run*` or `split`.

`internal/tools/covermap` is the gate that enforces this. Each `.txtar`
declares, in its **leading comment block**, which PARITY rows it covers. This
file is the exact syntax that gate parses. Keep declarations accurate: an
over-claim (saying you cover a row you do not) hides an untested assertion;
leaving a case undeclared is an error.

## Where declarations go

Declarations are `#` comment lines at the very top of the file, in the run of
comment and blank lines **before the first testscript command or `-- file --`
marker**. `#` comments that appear later in the script are ordinary comments
and are ignored by the gate. Every file begins with the author header; the
declarations follow it:

```
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# covers: llm_flags_spec.sh:42
# covers: llm_flags_spec.sh:58

env GASH_HEADLESS=1
exec gash ...
```

## Directives

Each directive is one line, `#` then the keyword then a value. Keywords are
exact and lowercase. All are repeatable.

| Directive | Meaning |
|---|---|
| `# covers: <spec_file>:<line>` | This case covers the PARITY row in section `### <spec_file>` whose Line cell is `<line>`. The spec file keeps its 2.x `*_spec.sh` name; the line is the number in the Line column (the 2.x source line), not a line in this file. |
| `# covers-test: <exact test name>` | This case covers the PARITY row whose Test cell equals this string **exactly** (copy it verbatim from PARITY.md). Use it when the line number is volatile. The name must be unique across the whole table; the gate errors if it matches zero or more than one row. |
| `# covers: none (<reason>)` | This case covers no 2.x public assertion; the gate ignores it. The reason is free text, e.g. `3.0 surface`, `3.0 HTE surface`, `3.0 shim surface`, `3.0 self-management proof`. Required on any case that has no `covers:`/`covers-test:`/`forced-divergence:` row, so that "covers nothing" is always a deliberate, visible choice. |
| `# forced-divergence: <LEDGER-id>` | This case proves an accepted forced divergence. `<LEDGER-id>` is a canonical id from the LEDGER "Forced divergences" table (hyphenated: `LLM-37`, `SHELL-01`, `SELF-16`). The gate validates the id. See below. |
| `# ledger: <LEDGER-id>` | Annotation only: ties the case to a LEDGER **Defect** or decision it reproduces (not a divergence). The gate validates the id but it does not change coverage. Use it for pure-port proofs of a recorded defect. |

A case must carry at least one of `covers:`, `covers-test:`,
`forced-divergence:` or `covers: none`. A file with none is a gate error.

## Forced divergences

A forced divergence is a place where rc.A deliberately differs from 2.x
(`PORT/LEDGER.md` "Forced divergences"). There are two shapes:

* **The divergence supersedes a 2.x public assertion.** The old assertion is a
  `run`/`run*`/`split` PARITY row that would now fail against Go. Declare both
  the id and the row it supersedes:

  ```
  # forced-divergence: LLM-37
  # covers: llm_spec.sh:331
  ```

  When PARITY's Go (rc.A) cell for that row reads `FD <id>`, the gate also
  counts it covered through the forced-divergence channel; the explicit
  `# covers:` keeps it covered while the Go column is still `not run`.

* **The divergence has no 2.x counterpart** (an audit finding 2.x never
  tested, e.g. an internal shell-state leak). There is no row to cover:

  ```
  # forced-divergence: SYS-42
  # covers: none (3.0 self-management proof)
  ```

## Render-spec fixtures

The render spec (`output_style_render_spec.sh`, the R1-R6 family of
`design/testing.md` §4, driven by the 286 fixtures) is the largest block. Its
cases are grouped in a `render/` subdirectory:

```
testdata/script/render/<function>_<case>.txtar
```

Each render `.txtar` declares one `# covers:` line per PARITY row of
`output_style_render_spec.sh` it drives (a single file may drive several, so
several `# covers:` lines are expected). Use `# covers-test:` with the exact
fixture name where line numbers churn. Shared fixture inputs (the stub tree, a
golden file) live beside the `.txtar` as its `-- file --` sections or, when
reused across cases, under `render/testdata/`.

## Running the gate

```bash
cd go-impl
go run ./internal/tools/covermap           # totals + uncovered grouped by spec
go run ./internal/tools/covermap -v        # also the full uncovered list
go run ./internal/tools/covermap -summary  # one line for CI
```

It exits non-zero while any universe row is uncovered or any declaration is
invalid. During M8a that is expected: coverage is built up case by case until
it reaches zero uncovered. The gate is a standalone tool, never part of
`go test ./...`, so the default suite stays green meanwhile.

## Authoring tips (from the pilot slice)

- **Pattern quoting.** `testscript` groups `stdout`/`stderr`/`cmp` patterns by
  SINGLE quotes only; a double quote does not group. Gash messages quote
  identifiers (`Tag 'v1' created.`), and that literal `'` cannot sit inside a
  single-quoted pattern. Match the quote (and any regex metachar such as `(`
  `)` in `Empty (corrupted) object files:`) with a regex `.`:
  `stderr 'Tag .v1. created'`. Patterns are regexp by default; use `cmp` only
  against a captured golden file.
- **Shell-out streams.** Functions that run git/docker/ssh with inherited
  streams leak the child's own stdout/stderr to the case (e.g. `git push`
  prints `* [new tag] ...`). Do not `cmp stderr` a golden for those; match the
  Gash-authored lines with regex and let the child noise fall where it may.
- **Determinism for git-driving cases.** The git module runs real git with the
  caller's env/HOME (GIT-10, no hardening), so a git case must create its own
  `home/.gitconfig` with `user.name`/`user.email`, `init.defaultBranch=main`
  and `advice.detachedHead=false`, and set fixed `GIT_AUTHOR_DATE` /
  `GIT_COMMITTER_DATE`. Match a commit short hash in output with a regex, not a
  golden. Each case builds its own repo/origin in a subdir; drive the function
  through the shim (`bash -c 'source "$GASH_SH"; <fn> ...'`), assert the
  observable contract, then check state with `exec git ...`.
- **Optional tools are not on the CI runner.** `rg`, `fd`, `tree`, `bat` and
  friends are installed in the dev container but NOT on the GitHub Actions
  runner. A case that *executes* such a tool (checks its output) must guard it
  exactly as the 2.x spec does (`if command -v rg >/dev/null 2>&1; then …; fi`),
  or it fails in CI with `command not found`. A case that only proves `llm_exec`
  *refuses* the tool (asserts the `.error` code) needs no guard: the guard
  rejects it before it runs.
- **Never depend on real network (un)reachability.** A net/http function
  (`ip_public`, `chrome_headless_update`, the `ai_*` calls, the docker/compose
  registry checks) must be driven through `httpsfake` (start it, set
  `HTTPS_PROXY`/`SSL_CERT_FILE`, answer via `MOCK_CURL_*`, exported before the
  fake starts: it reads its own environment). Do NOT write a case
  that unsets the proxy and expects the call to FAIL because the endpoint is
  unreachable: the CI runner has open egress, so the Go binary's net/http path
  reaches the real endpoint and the case passes/fails opposite to a restricted
  dev container. To prove a failure path, give the fake no route (it answers
  404); to prove "no tool needed" (SYS-36), keep the fake and run with
  `PATH=/nonexistent` so net/http still succeeds without curl/wget.
- **PARITY Line numbers.** `# covers:` uses the Line cell from the PARITY row
  verbatim (the `it`-body line, one past the `it` keyword); covermap matches on
  that value, so copy it from PARITY, do not recount from the spec.
- **Pure-port defects stay plain `# covers:`.** A row that reproduces a 2.x
  defect the Go port matches byte-for-byte is covered normally; reserve
  `# forced-divergence:` for a LEDGER "Forced divergences" id where rc.A
  deliberately differs, and `# ledger:` as a non-coverage annotation.
