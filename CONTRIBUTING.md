# Contributing to Gash

Thanks for helping. Gash is a Go binary with a thin generated Bash layer
that runs inside people's interactive shells and inside their agents' tool
calls, so a change is judged by two questions: does it keep the user's
shell intact, and does it keep the public contract (functions, `--json` envelopes, error contract,
`GASH_*` settings) stable?

## Getting started

```bash
git clone https://github.com/mauriziofonte/gash.git
cd gash
go test ./...                                  # unit tests and the testscript suite
go build -o "$HOME/gash-dev" ./cmd/gash        # a binary of this working copy
GASH_BIN="$HOME/gash-dev" GASH_HEADLESS=1 bash -c 'source ./gash.sh; files_largest --json .'
```

`gash.sh` calls the binary named by `GASH_BIN`; without it, the release
binary of the checkout's `VERSION` (`~/.local/share/gash/versions/`), fetched
on first use. `bin/gash` is the launcher script, not the binary.

To try a working copy as your shell, point `~/.gashrc` at it with
`GASH_DIR=/path/to/gash` or run
`GASH_BIN="$HOME/gash-dev" bash --rcfile <(echo "source $PWD/gash.sh")`.

## Before you open a pull request

```bash
gofmt -l . && go vet ./...
go test -p 2 ./...                                  # unit tests and the testscript suite
CGO_ENABLED=1 go test -race ./internal/<touched>/... # the packages you changed
go test ./cmd/gash -run TestScript                  # the black-box suite alone
go run ./cmd/gash __compat generate --out .         # after a compat/*.tmpl change
go test ./internal/compatgen -update                # then refresh the goldens
go run ./cmd/gash __compat generate --out . --check # the generated Bash is current
go run ./internal/tools/covermap                    # every 2.x assertion has a home
go run ./internal/tools/bashonlymap -check          # tests/parity/bash-only.tsv is current
shellcheck -s bash -S warning $(git ls-files 'lib/**/*.sh' gash.sh bash_completion .gashrc install.sh)
shfmt -d $(git ls-files 'lib/**/*.sh' gash.sh bash_completion .gashrc bin/gash)
go test ./internal/registry -run ReferenceDocs      # when help text changed: update docs/reference
```

CI runs the same commands on Linux and macOS, as a non-root user.

## Conventions

- **Layout.** Each module is a Go package under `internal/` (`fsscan`,
  `remote`, `docker`, `llm`, ...). Its `Register(*invoke.Table)` binds every
  public function with `t.Handle("name", fn)`, and `internal/handlers.Table`
  lists the modules: a new module adds one line there, nothing registers
  itself in `init()`.
- **The registry** (`internal/registry/catalog_gen.go`, maintained by hand
  despite its name) holds each function's help body, short line, aliases,
  completion and settings. A new function gets an entry there; its handler
  starts with `if help.NeedsOf(c, c.Arg(0)) { return 0 }`, answers `--help`
  without side effects, and fails on unknown options.
- **Handlers** read `c.Env`, write `c.Stdout` and `c.Stderr`, honor `c.Ctx`
  and return the exit status; they never call `os.Getenv` or `os.Exit`.
- **Agent-facing functions** print JSON on stdout and errors as the error
  envelope on stderr with `c.Fail(contract.New(code, details, hint))`. A new
  code goes in `internal/contract/codes.go` and in the AGENTS.md section 5
  table, with the `action` an agent branches on. New keys may be added to an
  envelope; existing keys never change type.
- **Output** goes through `internal/ui`: `ui.StyleOf(c.Term, noColor)` gives
  the semantic palette and the blocks (`Section`, `Heading`, `KV`, `Table`,
  `List`, `Box`, `Bar`, `Confirm`, `ConfirmPlan`), and its `Printer` the
  diagnostics (`Error`, `Hint`, ...). Colors and glyphs follow the terminal
  state in `c.Term`, with an ASCII fallback.
- **Settings** are `GASH_*` lines in `~/.gash_env`, listed in
  `.gash_env.template` and in the registry's settings; the environment wins
  over the file, except for the `llm_exec` permissions.
- **Generated Bash.** `gash.sh`, `bash_completion` and the shims under
  `lib/` come from `compat/*.tmpl` through `gash __compat generate`; edit the
  template, never the output. The shell layer is Bash 4.4+ with only features
  every Bash in the matrix has (no `\b` in regexes: macOS lacks it), safe
  under `set -euo pipefail` (no `[[ … ]] && cmd` as the last command of a
  function, no globals left behind at load time), and its traps clean up
  with `command rm` and remove themselves.
- **Portability.** The binary needs no external userland except the tools a
  command documents (`git`, `docker`, `ssh`, `rsync`, ...); directory walks,
  text handling and formatting are Go, identical on Linux and macOS. Tests
  pass on macOS (BSD userland, case-insensitive APFS) and as a non-root user.
- **Tests** are Go unit tests next to the code, plus the black-box suite in
  `testdata/script/*.txtar` (testscript against the built binary and the
  generated shims). Each `.txtar` declares in its leading comments which 2.x
  assertions it covers (`# covers: ...`, or `# covers: none (...)` for a 3.0
  surface); `testdata/script/README.md` has the syntax and
  `go run ./internal/tools/covermap` enforces it. The 2.x assertions with no
  black-box home are listed, with their Go test, in
  `tests/parity/bash-only.tsv`, which `go run ./internal/tools/bashonlymap
  -check` verifies. Tests are hermetic (temporary `HOME`, stub binaries on
  `PATH`) and skip when an optional tool is missing.
- **Docs**: `docs/reference/` is checked against the registry; a test fails
  when it is stale or when a page cites a function or setting that does not
  exist.

## Quotes

`quotes/list.txt` feeds `gash_inspiring_quote` and the banner. Every quote
needs a source a reader can check: a book, essay, talk, interview or mailing
list post where the person said or wrote it, not a quote site. Put the source
on the `# ` comment line right above the quote, with a URL when there is one:

```text
# Linus Torvalds, linux-kernel mailing list, 2000-08-25, https://lkml.org/lkml/2000/8/25/132
"Talk is cheap. Show me the code." — Linus Torvalds
```

Quote exactly, in English, and name the person who said it (a law named after
someone is attributed to whoever wrote it down). When Wikiquote lists a line
as disputed or misattributed, leave it out.
`go test ./internal/handlers -run TestShippedQuotes` checks the source lines,
the format and duplicates.

## Commits and pull requests

Small, focused pull requests are easiest to review. Describe what a user
sees before and after, add a line to `CHANGELOG.md` under the unreleased
version, and say how you tested it. Breaking a documented function,
envelope or setting needs a major version; adding one needs a minor.

## Releasing

1. Set `VERSION` (e.g. `3.0.0`) and add the dated heading to `CHANGELOG.md`.
2. `go run ./internal/tools/releasesums` writes `release/SHA256SUMS`, a
   reproducible build of gash for darwin/linux × amd64/arm64. Commit
   `VERSION`, `CHANGELOG.md` and `release/SHA256SUMS` together;
   `go run ./internal/tools/releasesums -check` verifies that the file is
   current.
3. Merge it to `main` and wait for CI to pass on the merge commit.
4. Run the **Tag release** workflow (Actions tab, or
   `gh workflow run tag.yml --ref main`). It checks that CI passed on the
   commit, that `release/SHA256SUMS` is current and that `CHANGELOG.md` has
   the entry, creates the annotated tag `v<VERSION>` and dispatches the
   **Release** workflow on it. That runs goreleaser (which refuses a tag
   that differs from `VERSION` and a binary that differs from its pin),
   takes the release notes from the `CHANGELOG.md` entry, signs
   `checksums.txt` keyless with Sigstore, and attaches the SBOMs and the
   build provenance. No signing key is involved: the signature names the
   workflow, the tag and the commit (docs/security.md, Verifying a release).
5. Any commit that changes Go code after step 2 needs step 2 again before
   tagging.

## Reporting security issues

Please do not open a public issue; see [SECURITY.md](SECURITY.md).

By contributing you agree that your contributions are licensed under the
[MIT License](LICENSE).
