<!-- Gash 3.0 design: index and decisions. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Gash 3.0 design (stop S1)

This directory is the S1 design of the Go port. No production Go exists yet:
it starts after the operator's go-ahead.

## Documents

| Document | Content |
|---|---|
| [`architecture.md`](architecture.md) | Process model, the declarative registry, command line and the 2.x argument grammars, the error contract, color gating, configuration, which external tools stay CLIs, the safety core (paths and secrets, SQL guard, `llm_exec`), databases, errors |
| [`layout.md`](layout.md) | Repository layout during the port and after cutover; where the bookkeeping goes |
| [`compat.md`](compat.md) | The generated Bash: `gash.sh`, the 7 shim paths, `bash_completion`, binary resolution, the call wrapper, what stays Bash |
| [`shell.md`](shell.md) | `gash init bash`: load order, prompt, history, terminal, toolchains, alias packs, banner, completion, unload, startup, tests |
| [`upgrade.md`](upgrade.md) | 2.x to 3.x upgrade, binary distribution, missing binary, rollback to `v2.1.0`, 1.x landings, `gash update`, and the audit of runtime reads and 1.x loaders |
| [`libraries.md`](libraries.md) | Library stack with pinned versions, deviations and their evidence |
| [`testing.md`](testing.md) | Test strategy: the specs against both implementations, seams, unit, testscript, upgrade, HTE, CI |
| [`hte.md`](hte.md) | HTE in userland: files, rendering, certificates, PHP-FPM, supervision, ports, privilege boundary, receipt, commands, migration, platforms |
| [`../PORT/MATRIX.md`](../PORT/MATRIX.md) | Every 2.x public function, alias, flag, env var, setting and file with its decision; new 3.x surface for approval |
| [`../PORT/LEDGER.md`](../PORT/LEDGER.md) | Defects, forced divergences, decisions, doc drift and HTE design delta found so far, with fix recipes; completed at S2 |
| [`../PORT/PARITY.md`](../PORT/PARITY.md) | All 1095 spec tests classified, with the plan for each in Go mode |
| [`../PORT/STATE.md`](../PORT/STATE.md) | Phase, gate, environment blockers, open questions |
| [`../bench/shell/`](../bench/shell/) | Prompt and startup benchmarks with the 2.x numbers |

## The design in ten lines

1. One static binary (`CGO_ENABLED=0`) plus Bash generated from one
   registry and committed: `gash.sh`, the 7 shim paths, `bash_completion`,
   `bin/gash`. A checkout works with no build step; the binary for the
   checkout's `VERSION` is fetched and verified on first use, outside the
   install directory.
2. Every 2.x function stays a real shell function (`declare -F`), a one-line
   stub that calls the binary with the caller's state; same names, arguments,
   output, exit codes, envelopes.
3. What must act on the calling shell stays Bash, emitted by `gash init
   bash`; the prompt stays a Bash hook with the 2.x process count, so it is
   not slower than 2.x.
4. 2.x argument grammars are ported per function (no cobra parsing on 2.x
   commands); new commands use cobra flags.
5. Output primitives are reimplemented byte for byte; one color gate.
6. External tools: native Go where 2.x results depended on which tool or
   variant was installed (walks, search, JSON, YAML, databases, HTTP); the
   CLIs where the user's configuration matters (git, docker, ssh, systemd,
   process tools).
7. Safety rails are ported with their spec vectors; nothing user-supplied is
   executed as code.
8. Upgrades: 2.x `gash_upgrade` lands on a working 3.x shell; no binary
   means one clear line or one envelope, never a broken shell; `git checkout
   v2.1.0` rolls back.
9. HTE: a user-owned Apache master and one PHP-FPM master per version, state
   in TOML, rendered configs with test, reload and rollback, a receipt for
   uninstall, one root-owned hosts helper as the only NOPASSWD entry.
10. Tests: the 2.x specs run against Bash and Go during the port; testscript
    and Go unit tests replace them at cutover.

## Decisions requested

Each has a recommendation; the design proceeds on it unless the operator says
otherwise.

| Id | Question | Options | Recommended |
|---|---|---|---|
| D-1 | How `llm_exec` runs a validated line (`architecture.md` 8.3) | A: Go parses and validates, then renders a script of single-quoted words that `bash --noprofile --norc -c` runs. B: Go runs the pipeline itself and emulates the allowed builtins | **A**: one parser decides, Bash only sees quoted words; builtins and exit statuses stay Bash's, so 2.x output is kept. B re-creates `printf`, `test`, `type` and friends and will diverge |
| D-2 | Force-HTTPS bypass for `127.0.0.1` and `::1` under loopback-only binding (`hte.md` 3, LEDGER HTE-52) | A: keep it as PRD 6.5 says; force-HTTPS then redirects nothing, as in hte-cli in practice. B: drop the bypass, so force-HTTPS means a 307 for every plain-HTTP request. C: a per-site bypass flag with a default to choose | **A** for rc.A (PRD as written); B or C is a one-template change if the operator prefers a redirect that works |
| D-3 | Binary integrity (`upgrade.md` 2) | A: SHA-256 from the release's `checksums.txt` over HTTPS. B: SHA-256 committed in the tagged tree (`release/SHA256SUMS`), so a signed tag covers the binary too | **B**: keeps `GASH_REQUIRE_SIGNED` meaningful for the binary. Cost: one step before each tag (reproducible build, commit the sums), re-checked by the release workflow |
| D-4 | Ports 80/443 on Linux and WSL2 (`hte.md` 6) | A: `net.ipv4.ip_unprivileged_port_start=80` (any local user may bind 80-1023), stated and asked by `install`. B: high ports by default, sysctl as an opt-in | **A**: the PRD wants 80/443; on a developer machine the trade-off is small and it is reverted by `uninstall` |
| D-5 | Library deviations (`libraries.md` 2) | Approve or reject each: fang, lipgloss, colorprofile, huh, charm log, moby client, go-containerregistry (rc.A), the two AI SDKs, go-humanize | **Approve all**: each is measured (render rules, a 4 s stall on PTYs, start-up cost on every call, byte-exact 2.x formats) |
| D-6 | The 1.x to 2.x migration (`gash_migrate`, PRD 5.5) | A: port it (10 specs, the TUI, backups, rollback). B: keep the name and `--status`/`--rollback`, replace the review with a pointer to the docs | **A**: 1.x installs land on 3.x through their own upgrade (`upgrade.md` 3.5), MUST NOT 3 keeps the name, and the specs make it a bounded port. B saves about 700 lines of Bash to port if 1.x installs no longer matter |
| D-7 | Security fixes for 2.1.0 (outside `go-refactor`) | The audit found issues in the released 2.1.0 that rc.A fixes only where a divergence is forced: a PostgreSQL `llm_db_query` path to psql meta-commands, and the prompt executing text from a repository's branch name (both critical, LEDGER), plus the seed items. A 2.1.x release needs a branch from `main` and a tag, which only the operator can authorize | Ship a 2.1.1 with the critical items; the LEDGER recipes apply to the Bash code as written. The LEDGER leaves out working payloads until then |
| D-8 | New 3.x surface (`MATRIX.md`, "New in 3.x") | Approve the list: `gash init bash`, `gash update`, `gash version`, `gash hte ...`, the hidden `__compat`/`__complete` commands, `bin/gash`, `lib/compat/bootstrap.sh`, `GASH_BIN`, `GASH_RELEASE_URL`, `install.sh --repair`, new error codes | Approve: each is required by PRD sections 5.3, 5.4 or 6 |

## Notes for the operator

* **Commit identity.** File headers, `LICENSE`, the module and package
  metadata carry only Maurizio Fonte. Commits on the branch are authored by
  the cloud environment's git identity (Claude, noreply), signed by that
  environment, because the session cannot sign as the operator; no
  co-author trailer is added. A squash merge with the operator's authorship
  removes it from `main`'s history.
* **Tags.** 1.x installs move to the newest tag by creation date, whatever
  its name; any tag created after `v2.1.0`, an rc included, reaches them
  (`upgrade.md` 3.5). 2.x installs never move onto an rc by themselves.
* **Operator decisions on record (PRD 4.3)** are kept: the interactive
  design of `.gashrc`; scanners and `llm_tree` leave out paths with a tab or
  newline (listed in the LEDGER as a decision).
* **LEDGER at S1** holds the verified seeds and the findings of the S1 audit;
  it is completed at S2. Its permanent home after cutover is
  `docs/port/LEDGER.md` (`layout.md`).
* **Environment limits** of the cloud session (blocked mirrors, no macOS,
  no vulnerability database) are in `STATE.md`; the affected checks run in
  GitHub Actions.
