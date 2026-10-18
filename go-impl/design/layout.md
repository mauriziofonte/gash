<!-- Gash 3.0 design: repository layout. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Repository layout

## During the port (branch `go-refactor`, before cutover)

```
gash.sh, lib/, tests/, bash_completion, ...   Bash 2.x: the reference implementation, untouched
go-impl/
  go.mod                         module github.com/mauriziofonte/gash
  cmd/gash/                      the binary
  cmd/gash-hosts-helper/         the root hosts helper (HTE)
  internal/...                   see "Packages" below
  compat/                        templates of the generated Bash files (gash.sh, the 7 shim paths, bash_completion, bin/gash)
  testdata/                      fixtures, golden files, txtar scripts
  design/                        this design (S1)
  PORT/                          STATE.md, MATRIX.md, LEDGER.md, PARITY.md
```

`tests/run.sh --impl go` builds the binary, stages a tree (a copy of the
repository whose Bash implementation files are replaced by the generated
shims, `go run ./cmd/gash __compat stage --out DIR`) and runs the unchanged
specs against it. `--impl bash` (default) runs them against the checkout as
today.

## Final layout (rc.A and later; `go-impl/` content moved to the root)

```
go.mod, go.sum                   module github.com/mauriziofonte/gash, go 1.27
cmd/gash/main.go
cmd/gash-hosts-helper/main.go
internal/
  app/          process entry: env snapshot, stream setup, exit codes, panic → FATAL envelope
  registry/     the one declarative catalog: every command, alias, flag, setting, env var, help body
  cli/          cobra tree built from the registry; per-command 2.x argument grammars (args/)
  contract/     error envelope, codes → actions table, JSON encoders with jq-compatible escaping
  term/         color gating (one place), TTY detection, locale → glyph set
  ui/           2.x output primitives: emitters, kv, table, box, heading, list, mark, bar, wrap, choose, confirm, spinner, progress
  config/       ~/.gash_env grammar, settings precedence, file-only settings
  pathguard/    traversal, forbidden paths, secret-file patterns, symlink resolution
  runner/       the only place that starts processes (exec with explicit argv/env/stdin; fakes for tests)
  fsscan/       files_largest, dirs_largest, dirs_find_large, dirs_list_empty, tree_stats
  llm/          llm_tree, llm_find, llm_grep, llm_config, llm_git_*, llm_env, llm_ports, llm_procs, llm_project, llm_deps
  llmexec/      llm_exec: lexer, parser, allowlist, blocklist, child environment, bwrap
  sqlguard/     per-engine SQL lexers and the read-only rules
  db/           llm_db_* and gash_db_*: sessions, envelopes, timing, EXPLAIN
  gitx/         git CLI runner (hardening flags for the llm_* callers only); git module functions
  docker/       docker and compose functions (docker CLI), registry digests
  remote/       remote_* (ssh/scp/rsync CLI), host whitelist, deny list
  files/        archive_extract, file_backup
  system/       disk_usage, ports, processes, services, history, WSL helpers
  sysinfo/      sysinfo sections (gopsutil + /proc + tools)
  dev/          php_detect_version, node_*, chrome_headless_update
  ai/           ai_ask, ai_query, ai_sysinfo, redaction
  ssh/          gash_ssh_auto_unlock (PTY driver for ssh-add)
  selfmgmt/     version store, binary fetch, update/upgrade, uninstall, doctor, unload data, 1.x pointer (D-6)
  shellinit/    `gash init bash`: the interactive Bash layer, quoted with mvdan.cc/sh/v3/syntax
  compatgen/    generator of gash.sh, the 7 shim files, bash_completion, bin/gash, docs/reference
  hte/          state, render, apache, fpm, certs, hosts, service, platform, receipt, migrate, doctor, root ops
  hostshelper/  the helper's parser and writer (shared by the helper binary and the tests)
  platform/     OS/distro detection, XDG paths, sysroot indirection for tests
gash.sh                          generated, committed: compat entry (sourced by every ~/.gashrc)
bash_completion                  generated, committed
.gashrc                          template, unchanged (existing ~/.gashrc files are never rewritten)
.gash_env.template               kept
bin/gash                         generated, committed: POSIX sh launcher that execs the right binary
lib/core/output.sh               generated, committed compat shims (the 7 paths of PRD 5.2)
lib/core/validation.sh
lib/core/utils.sh
lib/core/config.sh
lib/modules/php.sh
lib/modules/node.sh
lib/modules/remote.sh
lib/compat/bootstrap.sh          generated, committed: binary resolution and first-run fetch, shared by the files above
quotes/list.txt                  kept (embedded in the binary too; open 2.x shells read it)
VERSION
install.sh                       rewritten: clone/tag as before, then fetch and verify the binary
testdata/script/*.txtar          testscript black-box suite (with the compat shims)
testdata/schema/*.json           JSON Schemas of every envelope
testdata/hte/                    legacy vhost/pool fixtures, sysroot fixtures
docs/                            user docs; docs/reference generated from the registry
docs/port/                       permanent home of LEDGER.md and decisions (see below)
.github/workflows/ci.yml         lint, vuln, unit + testscript on Linux and macOS, HTE E2E matrix, snapshot build
.github/workflows/release.yml    tags only: goreleaser
.goreleaser.yaml
```

Removed at cutover: `go-impl/`, `tests/` (the Bash framework, `tests/mocks/bin`,
`tests/run.sh`), the Bash implementation under `lib/` other than the 8 generated
files above, `tools/gen-docs.sh`.

Kept on disk for shells still running 2.x code after `git checkout`:
`quotes/list.txt`, `.gash_env.template`, `.gashrc`, `gash.sh`, `lib/core/`,
`VERSION` (all also used by 3.x). `lib/aliases/` and the 2.x modules go; the
effect on an open 2.x shell is two soft degradations until the next shell
(`upgrade.md` 7.1).

## Bookkeeping after cutover

| File | Moves to |
|---|---|
| `go-impl/PORT/LEDGER.md` | `docs/port/LEDGER.md` (kept for the 3.x line: triage record and fix recipes) |
| `go-impl/design/*.md` | `docs/port/design/` (decisions as approved at S1, then amended) |
| `go-impl/PORT/MATRIX.md`, `PARITY.md` | `docs/port/archive/` (final state at rc.A) |
| `go-impl/PORT/STATE.md` | removed at S3 |
