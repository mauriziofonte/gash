# Contributing to Gash

Thanks for helping. Gash is a Bash library that runs inside people's
interactive shells and inside their agents' tool calls, so a change is
judged by two questions: does it keep the user's shell intact, and does it
keep the public contract (functions, `--json` envelopes, error contract,
`GASH_*` settings) stable?

## Getting started

```bash
git clone https://github.com/mauriziofonte/gash.git
cd gash
bash tests/run.sh                 # the whole suite, in parallel
GASH_HEADLESS=1 bash -c 'source ./gash.sh; files_largest --json .'
```

To try a working copy as your shell, point `~/.gashrc` at it with
`GASH_DIR=/path/to/gash` or run `bash --rcfile <(echo "source $PWD/gash.sh")`.

## Before you open a pull request

```bash
bash tests/run.sh                                   # all specs
bash tests/run.sh -f 'files_largest'                # only matching tests
bash tests/run.sh --awk mawk                        # with mawk as awk
bash tests/run.sh --bash /path/to/bash-4.4/bin/bash # an older Bash
shellcheck -s bash -S warning $(git ls-files '*.sh' bash_completion .gashrc)
shfmt -d $(git ls-files '*.sh' bash_completion .gashrc tests/mocks/bin)
tools/gen-docs.sh                                   # when help text changed
```

CI runs the same commands on Ubuntu 22.04 and 24.04 (gawk and mawk), on
Bash 4.4, 5.0, 5.1 and 5.3, and on macOS with Homebrew Bash and GNU tools
(best effort). `tests/ci/fetch-bash.sh` builds the older Bash versions
locally.

## Conventions

- **Bash 4.4+, GNU or BSD userland.** Only features every Bash in the
  matrix has, no `\b` in regexes (macOS lacks it), no gawk-only awk
  (`__GASH_AWK` prefers gawk where it is only a nicety).
- **Safe under `set -euo pipefail`.** Callers source Gash from strict
  scripts: avoid `[[ … ]] && cmd` as the last command of a function or loop,
  and never leave `BASH_REMATCH` or other globals behind at load time.
- **Every public function** starts with
  `needs_help name "usage" "description" "${1-}" && return`, registers its
  help with `__gash_register_help` (usage, options, examples, see also),
  answers `--help` without side effects, and fails on unknown options.
- **Agent-facing functions** print JSON on stdout and errors through
  `__gash_json_error` on stderr, with an `action` the agent can branch on.
  New keys may be added to an envelope; existing keys never change type.
- **Output** goes through the helpers in `lib/core/output.sh`: the semantic
  palette (`__GASH_COLOR_PRIMARY` for structure, `ACCENT` for the names the
  user looks for, `MUTED` for secondary data, the status colors for status
  only), emitters for diagnostics (`__gash_info`, `__gash_error`,
  `__gash_usage`, `__gash_hint`, ...), `__gash_table` for columns, `__gash_kv`
  for key/value lines, `__gash_section` or `__gash_heading` for titles,
  `__gash_list`, `__gash_box`, `__gash_bar`, and `needs_confirm_prompt` or
  `__gash_confirm_plan` for questions. Colors only when `__gash_use_color`
  says so, glyphs with an ASCII fallback, outside text with `printf '%s'`.
  `tests/specs/output_style_lint_spec.sh` enforces these rules on the source;
  `tests/specs/output_style_render_spec.sh` runs every public function on a
  real terminal, piped and with `NO_COLOR`, so a new function needs a
  fixture there.
- **Settings** are `GASH_*` lines in `~/.gash_env`, listed in
  `.gash_env.template` and in `__GASH_SETTINGS_KNOWN`; the environment
  wins over the file, except for the `llm_exec` permissions.
- **Traps** clean up with `command rm` and remove themselves.
- **Tests** live in `tests/specs/*_spec.sh`: `it "behaviour" bash -c '…'`,
  hermetic (temporary `HOME`, mocks in `tests/mocks/bin`), exit 77 to skip
  when a tool is missing.
- **Docs**: `docs/reference/` is generated; a test fails when it is stale or
  when a page cites a function or setting that does not exist.

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
as disputed or misattributed, leave it out. `bash tests/run.sh -f Quotes`
checks the format and duplicates.

## Commits and pull requests

Small, focused pull requests are easiest to review. Describe what a user
sees before and after, add a line to `CHANGELOG.md` under the unreleased
version, and say how you tested it. Breaking a documented function,
envelope or setting needs a major version; adding one needs a minor.

## Reporting security issues

Please do not open a public issue; see [SECURITY.md](SECURITY.md).

By contributing you agree that your contributions are licensed under the
[MIT License](LICENSE).
