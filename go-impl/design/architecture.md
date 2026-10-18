<!-- Gash 3.0 design: architecture. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Architecture

## 1. Shape

* One static binary, `gash` (`CGO_ENABLED=0`, linux and darwin, amd64 and
  arm64), plus `gash-hosts-helper`, the small root-owned helper of HTE
  (`hte.md` section 7).
* Bash only where a child process cannot act: the parent shell (cd, aliases,
  exports, PATH, prompt, history, completion). That Bash is generated from
  the registry and committed (`compat.md`).
* Every 2.x public function becomes one stub, `f() { __gash_call f "$@"; }`:
  one `fork`+`exec` of the binary per call. Measured on the S1 VM: a static Go
  binary costs about 3 ms per call from Bash (`/bin/true`: 2.3 ms). Budget for
  the real binary: `gash version` under 10 ms, enforced by a benchmark test
  (no work in `init()`, the registry is static data, cobra commands are built
  lazily for the one being run).
* The prompt never executes the binary (`shell.md`).

```
cmd/gash ──► internal/app ──► internal/cli ──► internal/registry (catalog)
                  │                 │
                  │                 └─► handler (llm, fsscan, db, gitx, docker, remote, system, ...)
                  │                            │
                  ▼                            ▼
          contract (envelopes, JSON)    runner (the only process starter) · pathguard · config · ui/term
```

## 2. The registry (`internal/registry`)

One declarative catalog is the single source of truth. Each entry:

| Field | Use |
|---|---|
| `Name`, `Module`, `Topic`, `Aliases` (module short alias, alias-pack aliases with their conditions) | stubs, completion, reference card, docs, `declare -F` parity |
| `Class` (pure output, shell-mutating, interactive) and `ShellPart` (the Bash fragment for the parent-shell effect, if any) | compat generator, `gash init bash` |
| `Grammar` (the 2.x argument grammar, section 3) | parsing, completion, docs |
| `Help` (usage, description, examples: the exact 2.x help text) | `--help`, `gash_help`, docs |
| `Settings`, `Env`, `Files` read | MATRIX cross-check, the `__gash_call` variable list |
| `Errors` (codes it may emit) | contract tests, docs |
| `Handler` | the Go implementation |

Generated from it: the cobra tree, the compat Bash files, `bash_completion`,
the reference card shown by `gash <topic>` and the `gash` function, the 2.x
help registry, `docs/reference/*.md`, the agent catalog in `AGENTS.md`. A unit
test checks that every registry entry has a `MATRIX.md` row and the reverse.
This replaces the 2.x awk parsing of alias sources (PRD 5.5).

## 3. Command line

Two grammars, chosen per command by the registry:

1. **2.x functions** (`gash llm_tree ...`, and every stub): cobra command with
   `DisableFlagParsing`, arguments handed to a per-function 2.x grammar in
   `internal/cli/args`. The grammars are ports of the 2.x parsers, rule for
   rule, because their quirks are contract (`llm-core` audit, section 0.3):
   no `--flag=value`, no `--` terminator, no bundling, a value flag takes the
   next word unconditionally, `-h`/`--help` anywhere is help (only as the sole
   argument for `llm_exec`, only as `$1` for `llm_docker_check`), an empty
   first positional is skipped where 2.x skips it, last value wins, errors in
   argument order. Families share one implementation each: `llm` (the
   `__llm_*` rules), scanners (`__gash_fs_*`), `db` (the `llm_db_*` rules),
   and the hand-written parsers of the human modules, each with a
   table-driven test of its accepted and refused vectors.
2. **New 3.x commands** (`gash init`, `gash update`, `gash version`,
   `gash hte ...`, hidden `gash __compat`, `gash __complete`): ordinary cobra
   flags (`--flag value` and `--flag=value`, `--`), the same error envelope,
   the same `--json` and `--help` conventions.

`gash` with no argument and `gash <topic>` print the 2.x reference card;
cobra's own `help` and `completion` commands are disabled. An unknown word
after `gash` gets what the 2.x `gash` function prints for an unknown topic
(MATRIX row).

## 4. Contract (`internal/contract`)

* **Error envelope**: one line on stderr, keys in the order `error`,
  `details` (only when non-empty), `action`, `recoverable` (false exactly for
  FATAL), `hint` (only when non-empty), plus the extra keys some families add
  (`query_time` for `llm_db_*`). An unknown action becomes STOP. Exit status 1
  unless the 2.x function used another.
* **Code table**: every code with its fixed action, one Go table generated
  into AGENTS.md section 5. A code can only be emitted with its action.
* **JSON encoder**: hand-written, ordered keys, on top of
  `encoding/json/jsontext`. It reproduces what 2.x emits, which is jq output
  in most paths: compact, no HTML escaping (`<`, `>`, `&` raw), U+2028 and
  U+2029 raw, `\"` `\\` `\n` `\r` `\t` `\b` `\f` and `\u00xx` (lowercase hex)
  for the other controls and DEL, non-ASCII raw. Invalid UTF-8 follows the 2.x
  path it replaces (raw bytes where 2.x let them through, U+FFFD where jq
  replaced them): byte-level parity with a LEDGER item for the invalid-JSON
  cases (N15 of the llm audit). Numbers keep the source text (database
  values, `jsontext.Value` pass-through), so `1.0`, big integers and
  PostgreSQL numerics print as the server sent them, as jq 1.7 does.
* **Layout**: compact or pretty exactly where 2.x is (one-line envelopes from
  the `llm_*` family, the layouts of the scanners and of jq-formatted
  output), golden-tested per function.
* **Envelope schemas**: one JSON Schema per envelope under
  `testdata/schema/`, validated in testscript and unit tests
  (`santhosh-tekuri/jsonschema/v6`).

## 5. Terminal and output (`internal/term`, `internal/ui`)

* **Color gating, one function**: colors on only when the target stream's
  check says TTY *and* none of `GASH_HEADLESS=1`, a non-empty `NO_COLOR`,
  `GASH_NO_COLOR` non-empty and not `0`, `--no-color`. Load-time and runtime
  gates of 2.x collapse into this one place; the result is identical because
  the stub passes the caller's non-exported variables and file descriptors.
* **Primitives**: `internal/ui` reimplements the 2.x emitters and layout
  primitives (`__gash_info`, `__gash_error`, `__gash_kv`, `__gash_table`,
  `__gash_box`, headings, lists, marks, bars, wrap, choose, confirm, spinner,
  progress) byte for byte: same palette (`\e[0;31m` ... `\e[38;5;39m`), same
  glyph sets (Unicode or ASCII by locale), same widths counted in characters
  as 2.x counts them, same `COLUMNS` rules. `tests/specs/output_style_render_spec.sh`
  pins the exact SGR set (rule R3: only palette values, attributes and `\e[0m`),
  which a styling library cannot honor (`libraries.md`).
* **Stream policy**: data on stdout; every diagnostic, prompt and progress
  line on stderr; ANSI only when stdout is a TTY, even for stderr text.
* **Prompts**: only on a TTY; headless or without a TTY a prompt that 2.x
  refused is refused the same way, and new commands fail with
  `confirmation_required` / `missing_value` instead of asking.

## 6. Configuration (`internal/config`)

* `~/.gash_env` (or `GASH_ENV_FILE`): the 2.x grammar ported rule for rule
  (first entry wins, `DB:`/`AI:`/`SSH:` entries, URL split at the last `@`,
  user and password at the first `:`, only the password percent-decoded,
  permission warnings), with the 2.x config-spec vectors as unit tests.
* Settings precedence: the caller's shell variable wins (passed by the stub
  through `GASH__SET`), then the file, then the default. Headless applies the
  2.x subset only (`GASH_DB_TIMEOUT`, `GASH_AI_CONFIRM`, `GASH_AI_MODEL_*`).
* File-only settings (`GASH_LLM_EXEC`, `GASH_LLM_EXEC_ALLOW`,
  `GASH_LLM_SANDBOX`): read from the file, never from the binary's
  environment, never passed by the stub.
* Never written by 3.x except by the 2.x functions that already edit it,
  with their 2.x consent rules.

## 7. Processes (`internal/runner`)

The only package that starts processes. Explicit argv (never a shell string),
explicit environment (built from an allowlist, never the full parent
environment unless 2.x passed it through), stdin for secrets and SQL, no
`sh -c` anywhere except `llm_exec` (section 8.3) and the nvm bridge. A
recording fake replaces it in unit tests.

External tool policy (MUST NOT 8: results never depend on which awk, grep or
find is installed). Where a 2.x result depended on an optional tool (`jq`,
`rg`, `gawk`, `yq` flavor, compose v2 with jq), Go always behaves as 2.x did
on the documented path with the tool present, and the degraded path becomes a
LEDGER item (forced divergence or defect):

| Area | 3.0 | Why |
|---|---|---|
| Walks, sizes, counts (`find`, `du`, `stat`, `awk`, `sort`, `numfmt` in 2.x) | Native (`io/fs`, `os.Lstat`, `syscall` for device and block counts) | One behavior on GNU, BSD and BusyBox; tab/newline exclusion kept as contract (PRD 4.3) |
| Content search (`rg` or the GNU grep fallback in 2.x) | Native, one engine with ripgrep's semantics (Go `regexp`, the closest dialect to ripgrep's; `.gitignore`/`.ignore`/`.rgignore`, hidden and binary files skipped as rg does), deterministic path order | The rg/grep split is a forced divergence (LEDGER); rg output order was nondeterministic |
| JSON and YAML (`jq`, `yq` in 2.x) | Native | No silent degradation without jq (LEDGER defects become impossible); YAML output in mikefarah yq's JSON layout |
| Databases (`mysql`, `psql`, `sqlite3` clients) | Native drivers through `database/sql`: `go-sql-driver/mysql`, `pgx/v5` (stdlib), `modernc.org/sqlite` | No client binaries needed, no password in a child environment (`PGPASSWORD`), no psql meta-commands; the 2.x envelopes reproduced (section 9) |
| git | The `git` CLI. `llm_git_*` and `llm_exec` add the 2.x hardening flags; the git module functions (`git_sync`, `git_add_tag`, ...) do not, so they run the user's hooks, signing, credential helpers and `insteadOf` rules as 2.x (LEDGER GIT-10) | Parity with the user's config, credentials and hooks policy (PRD 9) |
| docker, compose | The `docker` CLI for everything 2.x ran through it; registry digests through `go-containerregistry` | `docker compose` has no Go API; the CLI keeps the user's contexts, config and credential helpers. The moby client is not needed (`libraries.md`) |
| ssh, scp, rsync, ssh-add | The CLIs; `ssh-add` driven through a PTY (`creack/pty`) | User's ssh config, agents and keys; private keys never parsed in-process |
| systemctl, journalctl, ps, pgrep, ss/netstat, lsof, df, ip | The CLIs, as 2.x: their output is shown or parsed (in Go, deterministically) and their absence is part of the contract (`ports_tool_not_found`, `port_kill` without lsof). `gopsutil` only where 2.x read `/proc` or `sysctl` itself (`sysinfo` sections) | Same data, same dependency errors, the spec stubs stay valid. Decided per function in MATRIX |
| kill | `kill(2)` | No output of its own to preserve; specs that faked `kill` are rewritten (`testing.md` 2.3) |
| nvm | Bash (it is a shell function), through the generated shim | `nvm use` changes the caller's PATH |
| curl | `net/http` | Secrets (API keys, registry tokens) stay in-process instead of reaching a child through a config descriptor; no dependency on curl or `~/.curlrc`. Proxies and CA from the environment (`HTTPS_PROXY`, `NO_PROXY`, `SSL_CERT_FILE`); 2.x timeouts and error taxonomy kept |

## 8. Safety core

### 8.1 Paths and secrets (`internal/pathguard`)

One implementation of the 2.x rules, used by every caller that 2.x checked:
`..` refusal, the forbidden list (`/proc /sys /dev /boot /root /etc/shadow
/etc/gshadow /etc/passwd /etc/sudoers /etc/sudoers.d`, root's home where it
is not `/root`), whole-component matching, resolved and lexical candidates,
the `/private` prefix on macOS, the secret patterns, exceptions and
directories verbatim, case-insensitive base-name matching, symlink targets.
The tables are data shared with docs generation, so docs cannot drift. Gaps
found by the audit (`/etc/shadow-`, walks entering protected trees, scanners
with `-L`) are LEDGER items: reproduced in rc.A, fixed in 3.0 if accepted.

### 8.2 SQL guard (`internal/sqlguard`)

The per-engine lexers ported state for state, with the 2.x spec vectors plus
the audit's vectors as table tests. The PostgreSQL E-string mislex is a
severe LEDGER defect; it is reproduced in the lexer for parity, but its
impact disappears in rc.A because the native pgx driver has no meta-commands
(a forced consequence of dropping the psql client, recorded in the LEDGER).

### 8.3 `llm_exec` (`internal/llmexec`)

The 2.x pipeline, in Go: blocklist (regexes verbatim), parser (the 2.x
character-level grammar: words, quotes, `\`, `|`, `&&`, `||`, `;`, newline,
`#` comments, the redirection allowlist), allowlist and per-command rules,
walk rules, argument path and secret checks, the restricted child
environment, git hardening, `sed --sandbox`/`gawk --sandbox`, bwrap.

How the checked line is executed is the one design decision this package
needs (D-1 in `README.md`):

* **A (recommended)**: the Go parser's output (a list of simple commands with
  literal words, glob words already expanded by Go against the same cwd
  tracking it validated, operators, allowed redirections) is rendered into a
  canonical script in which every word is single-quoted, and that script runs
  in `bash --noprofile --norc -c` with the 2.x child environment and wrapper
  functions. Bash then has nothing left to parse but quoted words and the
  operators Go emitted, so what runs is exactly what was validated (MUST NOT 7:
  one parser decides). Builtins, `cd`, `test`, `echo`, exit statuses and
  pipeline semantics stay Bash's, as in 2.x. Unrestricted mode passes the raw
  line to `bash -c` as 2.x does (only the blocklist applies, by the user's
  file-only opt-in).
* **B**: no Bash: Go runs the pipeline itself (`os/exec` per simple command,
  pipes, `&&`/`||`/`;` evaluation) and emulates the allowed builtins (`cd`,
  `echo`, `printf`, `pwd`, `test`, `true`, `false`, `type`, `which`). Smaller
  surface, but every builtin's output and exit status must be re-created and
  divergences are likely (`printf` formats, `test` operators, `type` text).

Glob expansion by Go (both options) differs from Bash only in sort order
under non-C collation: a forced divergence (LEDGER), byte order in 3.x.

### 8.4 Other rails

Remote host whitelist and DENY/ASK path classes, compose and git
confirmations, the AI redaction and consent gates: ported unchanged
(MUST NOT 5), each with the 2.x spec vectors as tests; the audit's bypasses
are LEDGER items.

## 9. Databases (`internal/db`)

* Connection resolution, `.target-database`, `-d` and table-name validation,
  row caps, `-n`/`--limit`/`-r`/`--rows`: as 2.x.
* Read-only session per engine: PostgreSQL `default_transaction_read_only=on`
  and `statement_timeout` from `GASH_DB_TIMEOUT` as connection parameters;
  MySQL `SET SESSION TRANSACTION READ ONLY` and `sql_select_limit`; SQLite
  `mode=ro` plus the `-safe` equivalents (no `ATTACH`, no extension loading,
  `trusted_schema=off`, defensive mode).
* Envelope `{"data","rows","query_time"[,"slow_query_explain"]}` in 2.x key
  order; `query_time` in µs, `N.Nms`, `N.NNs` as 2.x formats it.
* Value representation reproduced per engine (audit B.8): PostgreSQL rows as
  `to_jsonb` produces them (the query is still wrapped server-side in
  `COALESCE(jsonb_agg(to_jsonb(...)),'[]')` for wrappable queries, so types
  match exactly), MySQL every value a string and NULL as the string `"NULL"`
  (a LEDGER decision item), SQLite native types, the SQLite `EXPLAIN QUERY
  PLAN` text tree rebuilt from the plan rows in the `sqlite3` CLI layout
  (pinned by `envelope_schema_spec.sh`).
* Timing: 2.x measured client spawn and connect (LEDGER defect); rc.A keeps
  the measurement window equivalent (connect included), 3.0 can measure the
  query alone.
* DB error text comes from the driver instead of the client: the error codes
  and actions are the contract and stay; the `details` text changes (forced
  divergence: no client binary).

## 10. Errors and diagnostics

* A panic in a handler is caught in `internal/app`, printed as one envelope
  `{"error":"internal_error","details":"<message>","action":"STOP",...}` and
  exit 1 (new code, MATRIX row); the stack trace only with `GASH_DEBUG=1`.
* `GASH_DEBUG=1` diagnostics use the 2.x `__gash_debug` line (`Debug: TEXT`
  on stderr, glyph and muted color on a TTY), through `log/slog` with a
  handler that writes that format; no other logging.
* Signals: SIGINT and SIGTERM cancel the handler's context and reach child
  processes (same process group); the exit status is the one the 2.x
  function returned when interrupted (`128+N` for a Bash function killed by
  the signal).

## 11. Testing hooks built into the design

* `platform.Sysroot` maps every absolute system path, so HTE and system
  functions run against a fixture tree.
* `runner.Fake`, `clock.Fake`, `env.Snapshot` and an `afero` or `fstest`
  filesystem are injected through `internal/app`, never through globals.
* `gash __compat stage --out DIR` builds the tree the Bash specs run against
  (`testing.md`).
