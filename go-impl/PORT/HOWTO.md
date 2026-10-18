<!-- Gash 3.0 port: how to port a function. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# How to port a 2.x function

rc.A is a pure port: the Go handler prints the 2.x bytes, returns the 2.x
status and keeps the 2.x defects, except the forced divergences listed in
`LEDGER.md`. New defects go to the report, not into the code.

All commands run in `go-impl/` with `GOTOOLCHAIN=go1.27.2 CGO_ENABLED=0`.

## Where things are

| Path | What |
|---|---|
| `internal/registry` | Every 2.x function, alias, setting and topic, generated from the 2.x sources (`go generate ./internal/registry`, output committed, cross-checked against MATRIX by its tests) |
| `internal/invoke` | `Call` (one invocation) and `Table` (name to handler) |
| `internal/handlers` | `Table()`: the one list of module `Register` functions |
| `internal/<module>` | The handlers of one module, e.g. `internal/selfmgmt` |
| `internal/help` | `Display` (the registered help page), `Needs`/`NeedsOf` (needs_help) |
| `internal/ui` | 2.x output primitives byte for byte (`Printer` for Info/Error/Success/Warning/Hint, `Heading`, `ColorChart`) |
| `internal/term` | The color gate and the TTY state (`Term`) |
| `internal/contract` | The JSON envelope and the jq-compatible encoder |
| `internal/cli` | The cobra root: one command per delegated function, built from the registry (nothing to add per function) |
| `compat/*.tmpl` | Templates of the generated Bash (gash.sh, shims, bootstrap, completion, bin/gash); `compat/stubs.tmpl` holds stub overrides |
| `testdata/compat/` | Golden copies of the generated Bash |
| `internal/tools/parity` | Writes Go-mode results into the Go column of `PARITY.md` |

## Steps

1. Read the function's rows in `MATRIX.md` (class, notes, specs), its
   LEDGER items, its PARITY rows and the 2.x source.
2. In `internal/<module>`, write the handler and register it:

   ```go
   func Register(t *invoke.Table) {
       t.Handle("gash_username", username)
   }

   func username(c *invoke.Call) int {
       if help.NeedsOf(c, c.Arg(0)) {
           return 0
       }
       ...
       return 0
   }
   ```

   `Handle` panics on a name that is not in the registry or that is
   registered twice. A new module adds one line to `handlers.Table`.
   Nothing registers itself in `init()`.
3. Write Go unit tests that pin the exact 2.x bytes (stdout, stderr, status)
   with fakes for processes and files, e.g.
   `internal/selfmgmt/username_test.go`. Where the 2.x output can be
   produced in the test, compare with it live, as `internal/help` does with
   `__gash_help_display`.
4. Run the function's specs in Go mode (below) until they pass, then update
   the PARITY Go column with the parity tool.
5. A shell-mutating function (MATRIX class `shell-mutating`) also needs a
   stub override (below).
6. Gate: `gofmt -l .` empty, `go vet ./...`, `go test ./...`, and
   `bash tests/run.sh` (Bash reference) still 0 failed.

## The handler contract

- `c.Args` are the raw 2.x arguments: cobra does not parse them
  (`DisableFlagParsing`). Parse them in the 2.x order, with the 2.x
  messages. `c.Arg(i)` is Bash `${N-}`.
- Help: call `help.NeedsOf(c, arg)` where 2.x calls `needs_help`, with the
  argument 2.x passes; it prints the registered page (or the 2.x three-line
  fallback when the caller's shell had no help registry, `GASH__HELP=0`).
- Color and TTY: `c.Term.UseColor()` (environment allows color and stdout
  is a terminal), `c.Term.WantColors(noColor)` for a `--no-color` flag,
  `c.Term.Palette()`, `c.Term.Glyphs()`. Never test a terminal yourself.
- Messages: `ui.NewPrinter(c.Stderr, c.Term, noColor).Error(msg)` and
  friends, which reproduce `__gash_error` and the other emitters.
- Envelopes: `return c.Fail(contract.New(code, details, hint))` only where
  2.x prints `__gash_json_error`; the code sets action and recoverable.
- Use `c.Env`, `c.Stdin`, `c.Stdout`, `c.Stderr` and `c.Ctx`: never
  `os.Getenv`, `os.Stdout` or `os.Exit` in a handler. Return the 2.x status.
- A handler may run another function through `c.Handlers.Run(&invoke.Call{...})`
  (the gash card runs `gash_migrate` this way).
- A panic becomes the `internal_error` envelope (stack with `GASH_DEBUG=1`).
  A delegated function without a handler answers
  `{"error":"internal_error","details":"NAME is not implemented in this gash build",...}`.

## Shell state and child processes

The stub runs `gash NAME ARGS...` in a subshell that exports the shell
variables the 2.x code read: the common ones (`registry.CommonShellVars`),
the function's `ShellVars` and its `Settings`. `GASH__SET` lists the
settings that are set (set-empty and unset stay apart: use
`c.Env.Lookup`). If the 2.x code reads a shell variable the registry does
not list, fix the extractor in `internal/registry/gen`, run
`go generate ./internal/registry` and refresh the goldens; never read it
from a file instead.

A command the handler runs gets `c.Env.Child()`: the caller's exported
environment as a 2.x child saw it, without the `GASH__` protocol variables
and without the shell variables the stub exported only for this call
(`GASH__ADDED`). Never pass `c.Env.Environ()` to a child: it holds
settings and tokens that 2.x kept in unexported shell variables. Secrets
never go in argv, a child environment, a file or a log. Commands are found
on the caller's `PATH` (`c.Env.Get("PATH")`), not the binary's.

## Shell-mutating functions: stub overrides

A function that changes the calling shell (cd, history, exports, aliases)
cannot do that part in a child process. Keep that part in Bash: define
`stub/NAME` in `compat/stubs.tmpl`, which replaces the one-line stub in
every generated file that defines NAME:

```
{{% define "stub/mkdir_cd" %}}
mkdir_cd() {
    ...parent-shell part, as in 2.x...
    __gash_call {{% .Name %}} "$@"
}
{{% end %}}
```

The binary may hand state back as quoted assignments that the override
evaluates (as `gash_ssh_auto_unlock` will): quote every value with
`mvdan.cc/sh/v3/syntax.Quote` and emit only names you validated; never
evaluate anything else. MATRIX says which part stays Bash for each one.

## Compat operations

The shims call hidden operations, `gash __compat OP ARGS...`, through
`__gash_compat`, which exports the common state and returns 127 without a
binary. Without a binary every call site but `__gash_download` (which
answers `binary_not_found`) has a Bash fallback. Register one with
`t.HandleCompat("output table", handler)`; the handler gets the words after
the operation in `c.Args`.

| Operation | Called by | Protocol | In M0 |
|---|---|---|---|
| `help NAME` | `needs_help` | the registered page of NAME on stdout; status 1, no output, when none | done |
| `output wrap WIDTH` | `__gash_wrap` | text on stdin; one line per wrapped line on stdout | not ported |
| `output choose DEFAULT OPTION...` | `__gash_choose` | menu on the terminal; the 1-based index on fd 3 | not ported |
| `output box [--color ESC]` | `__gash_box` | lines on stdin, each NUL-terminated | not ported |
| `output table [ARGS...]` | `__gash_table` | 2.x arguments, rows on stdin, `GASH__TABLE_COLOR` | not ported |
| `output download URL DEST` | `__gash_download` | as 2.x | not ported |
| `json str` | `__gash_json_str` | raw value on stdin; the JSON string on stdout | not ported |
| `json arr` | `__gash_json_arr` | values on stdin, NUL-separated; the JSON array on stdout | not ported |
| `env load` | `__gash_load_env` | parses the settings file (warnings on stderr), prints quoted assignments of the caches that the shim evaluates | not ported |
| `generate --out DIR [--check] [--stage]` | developers, CI | writes or compares the generated Bash | done |
| `stage --out DIR [--root ROOT] [--bin PATH]` | `tests/run.sh --impl go` | staged checkout; prints `GASH_BIN=PATH` | done |

Until an operation is ported, its call prints the not-implemented
envelope on stderr and fails; `__gash_wrap` and `__gash_load_env` then
fall back as without a binary, the other call sites return the failure.

## Changing the generated Bash

Edit `compat/*.tmpl` (delimiters `{{%` and `%}}`), then:

```bash
go test ./internal/compatgen -update      # rewrite testdata/compat
git diff testdata/compat                  # review every changed byte
go test ./internal/compatgen              # goldens, bash -n, shellcheck, shfmt, sh -n,
                                          # quiet and fork-free sourcing, state passing
```

Rules: Bash 4.4+; nothing printed, prompted, forked or failing while
sourcing headless; functions that stay Bash keep their 2.x bytes; messages
go through the core emitters (static lint rules S10 and S11).

## Running the specs in Go mode

```bash
bash tests/run.sh --impl go                           # all specs; writes tests/.results/go.tsv
bash tests/run.sh --impl go specs/core_utils_spec.sh  # one spec
bash tests/run.sh --impl go -f 'gash_username'        # tests whose "<suite> :: <name>" matches
bash tests/run.sh --impl both                         # Bash reference, then Go (--results F: F.bash, F.go)
```

Go mode builds `go-impl/cmd/gash` (or takes `--gash-bin PATH`), stages
the checkout without its 2.x implementation files (`gash __compat stage`)
and runs the unchanged specs there, in PID and network namespaces when
unshare allows it (`--network` keeps the host network, `--no-isolate`
turns both off): a fake a binary bypasses fails instead of reaching the
network or signalling a real process. Then:

```bash
go run ./internal/tools/parity            # write the Go column of PORT/PARITY.md
go run ./internal/tools/parity -check     # status 1 when the column is out of date
```

The tool writes `pass`, `fail` or `not run` (for a skip), keeps `FD ...`
and `n/a ...` cells unless `-force`, and lists results without a PARITY
row. Run it after a full run: a partial run overwrites `go.tsv`.
