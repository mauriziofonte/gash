# Developing Gash

This folder is for people who change Gash. Users start at the
[README](../README.md) and [docs/](../docs/README.md).

| Page | What it covers |
|---|---|
| [CONTRIBUTING.md](../CONTRIBUTING.md) | Getting started, the checks to run before a pull request, the code conventions, commits, and the release steps |
| [documentation.md](documentation.md) | How the documentation is organized, what is generated, verified examples, the rules every page follows |
| [port/design/architecture.md](port/design/architecture.md) | The shape of the binary: registry, handlers, the generated Bash, the runner, the contract |
| [port/design/](port/design/README.md) | The other design notes of the Go port: the Bash layer, the shell integration, upgrades, the local web stack, testing |
| [tapes/](tapes/README.md) | The demo GIFs of the README: how they are recorded |
| [port/](port/STATE.md) | The record of the port from Gash 2.x: plan, ledger of every 2.x behaviour, decisions. Kept as written then; not maintained |

The code is the reference for anything these pages and the code disagree
on; fix the page in the same pull request.

## Where things are

| Path | What |
|---|---|
| `cmd/gash` | The binary's `main`, and the black-box test suite (`testdata/script/*.txtar`) |
| `internal/registry` | Every public function: help, aliases, completion, settings, what it needs |
| `internal/handlers` | The table that binds each function to its module |
| `internal/<module>` | One package per area (`fsscan`, `gitx`, `llm`, `agent`, `db`, `remote`, `hte`, ...) |
| `internal/prereq` | The catalog of external tools, distribution detection, the missing-tool report |
| `internal/contract` | Error codes and the JSON error envelope |
| `internal/docgen` | The generated documentation and its checks |
| `compat/` | Templates of the generated Bash (`gash.sh`, `lib/`, `bash_completion`) |
| `install.sh` | The installer (its prerequisite functions are generated from `internal/prereq`) |
| `testdata/distro`, `testdata/hte/e2e` | The distribution smoke test and the local web stack end-to-end tests |
| `release/SHA256SUMS` | The pinned hashes of the release binaries |
