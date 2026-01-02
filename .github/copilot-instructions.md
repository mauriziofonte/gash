# Copilot instructions (Gash)

## Project overview

-   Gash is a Bash configuration framework intended to be **sourced** (not executed) in interactive shells.
-   Entry flow: user shell sources `~/.gashrc` (template in [.gashrc](../.gashrc)), which sources `~/.gash/gash.sh` and loads completion from [bash_completion](../bash_completion).
-   `gash.sh` is the main orchestrator: it exits early for non-interactive shells and if `$HOME/.gash` is missing, then sources:
    -   Helpers/functions: [lib/functions.sh](../lib/functions.sh)
    -   Aliases/env: [lib/aliases.sh](../lib/aliases.sh)
    -   Greeting/extras: [lib/prompt.sh](../lib/prompt.sh)
    -   User hooks: `~/.bash_aliases` then `~/.bash_local`

Additional runtime features:

-   `gash.sh` snapshots the original shell state before mutating it (prompt/history/shopt/umask plus pre-existing functions and aliases). This snapshot is used by `gash_unload`.
-   `gash.sh` computes `__GASH_ADDED_FUNCS` and `__GASH_ADDED_ALIASES` (best-effort) so we can:
    -   Unload only what Gash introduced (without touching user shell symbols)
    -   Drive completion from a stable list of “public” Gash functions

## Development workflow (how to safely test changes)

-   Preferred: install to `~/.gash` and test by opening a fresh terminal (matches real usage).
    -   Install/update uses tags via git: [install.sh](../install.sh) and `gash_upgrade` in [lib/functions.sh](../lib/functions.sh).
-   If testing from a working copy, mirror the real layout (because [gash.sh](../gash.sh) hard-codes `GASH_DIR="$HOME/.gash"` and returns if it doesn’t exist).
    -   Common approach: `ln -s "$PWD" "$HOME/.gash"` (or copy) then restart shell.

## Codebase conventions (Bash-specific)

-   **Never `exit` from sourced code** (it would terminate the user’s shell). Prefer `return` from functions and guard blocks. Example: [gash.sh](../gash.sh) uses `return` for early exits.
-   Be strict-mode friendly:
    -   Assume callers/tests may use `set -euo pipefail`.
    -   Under `set -u`, never read raw positional parameters (`$1`, `$2`, …) unless you’ve validated the argument count; prefer `${1-}`, `${2-}`, etc.
    -   Prefer explicit “missing argument” errors (via `print_error`) over letting tools fail with cryptic messages.
-   Avoid leaking globals into the user shell:
    -   Prefer `local` variables inside functions.
    -   Don’t leave temporary variables defined at file scope unless they are intended configuration.
-   Keep startup safe/fast:
    -   Guard optional tooling with `command -v …` (pattern used throughout [lib/aliases.sh](../lib/aliases.sh) and [lib/functions.sh](../lib/functions.sh)).
    -   Avoid output in non-interactive contexts; `gash.sh` already checks `[[ $- != *i* ]] && return`.
-   Help/UX pattern for new functions:
    -   Use `needs_help "name" "usage" "description" "${1-}" && return` and print errors via `print_error` (see [lib/functions.sh](../lib/functions.sh)).
-   Cross-platform behavior is intentional:
    -   macOS tweaks live in [lib/aliases.sh](../lib/aliases.sh) (`LSCOLORS`, BSD `ls`).
    -   WSL-only aliases are gated by `/proc/version` and `$WSLENV`.

## Key integration points

-   Installer modifies user profiles to source `~/.gashrc`: [install.sh](../install.sh).
-   Completion relies on functions being loaded first (it checks `declare -f gadd_tag`): [bash_completion](../bash_completion).
    -   Completion prefers `__GASH_ADDED_FUNCS` (computed by [gash.sh](../gash.sh)) to suggest all public Gash functions and avoid user-defined ones.
    -   Internal helpers are filtered (name prefix `__*`/`_*` plus a small denylist like `needs_help`).
-   Quotes are data-driven from [quotes/list.txt](../quotes/list.txt) and rendered by `gash_inspiring_quote`.
-   SSH auto-unlock is invoked during prompt startup (in [lib/prompt.sh](../lib/prompt.sh)) if `gash_ssh_auto_unlock` is defined.
    -   Credentials file: defaults to `~/.gash_ssh_credentials`, configurable via `GASH_SSH_CREDENTIALS_FILE`.
    -   The function is designed to be safe at shell startup (no `exit`, tolerant parsing, early return if `ssh-agent`/`expect` aren’t available).
-   `gash_unload` is implemented in [lib/functions.sh](../lib/functions.sh) and restores the snapshot created by [gash.sh](../gash.sh) (best-effort).

## Making changes

-   Prompt behavior is split:
    -   PS1 construction/title logic is in [gash.sh](../gash.sh) (`PROMPT_COMMAND`, `__construct_ps1`).
    -   The greeting banner + quote runs at load time in [lib/prompt.sh](../lib/prompt.sh) (interactive-only).
        -   Optional system info is gated by `GASH_SHOW_INXI=1`.
-   Aliases and env defaults belong in [lib/aliases.sh](../lib/aliases.sh); reusable commands belong in [lib/functions.sh](../lib/functions.sh).

## Unit tests (Bash)

### How to run

-   Run the whole suite: `bash tests/run.sh`
-   Specs live in `tests/specs/*_spec.sh` and are sourced by the runner.

### Test harness DSL

-   The minimal test framework is in `tests/gash-test.sh`.
-   Core primitives:
    -   `describe "Suite name"` prints a suite header.
    -   `it "test name" <command...>` runs a single test and records pass/fail.
    -   `expects <actual> <op> <expected>` supports `==`, `!=`, `=~`.
    -   `expects_contains <haystack> <needle>` and `expects_status <code> <cmd...>`.

Most tests are written as `it "..." bash -c '...'
` so they run in a clean subshell with strict mode.

### Conventions used by specs

-   Prefer `set -euo pipefail` inside each `bash -c` body.
-   When a spec needs nested shells (especially `bash -i -c`), build the inner script via heredoc/variables to avoid quoting pitfalls and “wrong-shell” expansions.
-   Use `GASH_TEST_ROOT` (exported by `tests/run.sh`) to locate the repo root inside subshells.
-   Use temporary directories for filesystem effects:
    -   `tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT`
-   Avoid depending on user state (real `~/.gash`, real history, real ssh-agent, etc).
-   Prefer edge tests that verify “safe failure” behavior:
    -   Missing args should return non-zero and print a helpful error.
    -   Destructive operations should avoid side effects when invoked incorrectly.

### Mocking strategy (critical)

-   Deterministic tests rely on mock binaries in `tests/mocks/bin/`.
-   Specs commonly do: `export PATH="$ROOT/tests/mocks/bin:$PATH"` or even set a minimal PATH like `...:/usr/bin:/bin`.
-   If you add a new mock file, ensure it is executable.
-   Bash builtins cannot be overridden via PATH; override them with shell functions inside the test when needed.
    -   Example: `pskill` uses the `kill` builtin, so tests override `kill()` to record calls.

### Unload behavior (important)

-   `gash_unload` must never `exit` (it’s sourced code) and should be safe under `set -u`.
-   If you add new aliases/functions during startup, they should be discoverable via `__GASH_ADDED_FUNCS` / `__GASH_ADDED_ALIASES` so unload and completion stay correct.

### Writing tests that “break” things (then fixing)

-   Prefer exercising error paths first (missing binaries, missing config files, invalid args).
-   When a test reveals a real robustness issue, fix production code (not just the test).
    -   Example: prompt helpers returning non-zero can trip `set -e` callers; use `if ! needs_confirm_prompt ...; then ...; fi` to be errexit-safe.
-   Keep fixes minimal and consistent with Gash conventions:
    -   No `exit` from sourced code; use `return`.
    -   Guard optional tools with `command -v`.
    -   Keep messages in English and in Gash style.

### Testing install/uninstall/upgrade (happy paths)

-   Tests for installer/upgrader should be network-free and must not touch the user’s real shell config.
-   Pattern used in this repo:
    -   Set `HOME` to a temp dir.
    -   Set `PROFILE` to a temp profile file (e.g. `$HOME/.bashrc`).
    -   Point `GASH_INSTALL_GIT_REPO` to a local path (no network).
    -   Use `--assume-yes` for `install.sh`.
-   If you add `--quiet` coverage, ensure “data-returning” helpers are not silenced by QUIET (profile detection must still return a filepath).

### Debugging workflow for failures

-   Re-run the suite: `bash tests/run.sh`.
-   If a spec is flaky or hard to debug, run only the suite by sourcing a single spec:
    -   `bash -c 'set -uo pipefail; export GASH_TEST_ROOT="$PWD"; source tests/gash-test.sh; source tests/specs/<file>_spec.sh; gash_test_summary'`
-   Watch for common Bash pitfalls:
    -   Single quotes inside `bash -c '...'` strings can terminate the script unexpectedly.
    -   `grep -q "-pattern"` treats `-pattern` as an option; use `grep -q -- "-pattern"`.
    -   Under `set -u`, always use `${1-}` (not `$1`) when args may be missing.

## Practical patterns already used in this repo

-   For user-facing commands, prefer the pattern:
    -   `needs_help ... "${1-}" && return`
    -   Then validate args (`[[ $# -eq 0 ]]` or `[[ -z "${1-}" ]]`) and call `print_error` with a clear usage hint.
-   When wrapping tools that might prompt or return non-zero in normal flows (e.g. confirmation prompts), make the code `errexit`-safe using `if ! ...; then ...; fi` blocks.
