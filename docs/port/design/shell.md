<!-- Gash 3.0 design: shell layer (gash init bash). Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Shell layer: `gash init bash` (PRD 5.4)

Everything that must change the calling shell stays Bash: `cd`, aliases,
exports, PATH, umask, history, `PROMPT_COMMAND`/PS1, nvm, completion. In 3.x
that Bash is not read from `lib/`: the binary emits it once per shell start,
and `gash.sh` evaluates it. The binary computes everything that depends on
the machine (which programs exist, probes, settings, the banner text), so the
emitted script makes no forks of its own.

## 1. Load sequence (interactive)

The 2.x order (17 steps, audited at S1) is kept. Left: 2.x; right: who does it
in 3.x.

| # | 2.x step | 3.x |
|---|---|---|
| 1 | Bash ≥ 4.4 gate, interactive/headless gate, `GASH_DIR` (path of `gash.sh`, symlinks not resolved), `GASH_VERSION` from `VERSION` | `gash.sh`, same code (generated) |
| 2 | Load bookkeeping, `GASH_TIMING` | `gash.sh`; `GASH_TIMING=1` times the steps below with the same `__GASH_LOAD_TIMES` format |
| 3 | Snapshot for unload (`PS1 PS2 HISTCONTROL HISTTIMEFORMAT HISTSIZE HISTFILESIZE EDITOR PATH`, `histappend`, `checkwinsize`, symbol lists), once per shell | `gash.sh`, same variables and format (`__GASH_SNAPSHOT`), so a 2.x snapshot taken before an upgrade stays valid |
| 4 | Core libraries | Stubs for every public function and the module short aliases (static, generated); the 4 core shims |
| 5 | Settings from `~/.gash_env` (known names only, not exported, the environment wins) | Binary: parsed once, applied in the emitted script as plain assignments guarded by `[[ -v NAME ]] \|\|` |
| 6-9 | terminal, history, prompt, toolchains | Emitted script (sections 2-5) |
| 10 | Modules | Nothing to load: the stubs call the binary |
| 11 | Alias packs | Emitted script (section 6) |
| 12 | Banner, SSH auto-unlock | Emitted script (section 7) |
| 13 | Post symbol list | `gash.sh` |
| 14 | `~/.bash_aliases`, `~/.bash_local` at top level | `gash.sh`, at top level, after the eval (same order) |
| 15 | 1.x migration autostart | Emitted script: interactive, the once-a-day stderr reminder, also on a terminal (the review no longer starts by itself, D-6); nothing in headless mode (forced divergence, LEDGER SHELL-13) |
| 16 | `true` | `gash.sh` |
| 17 | `bash_completion` (from `~/.gashrc`) | Generated, static (section 8) |

The call is one exec:

```bash
[[ -t 1 ]] && __gash_tty_out=1 || __gash_tty_out=0       # no fork
__gash_init=$(GASH__TTY_OUT=$__gash_tty_out GASH__COLUMNS=${COLUMNS-} "$__gash_bin" init bash) \
  && eval "$__gash_init"
```

The binary's stdout is the pipe of `$(...)`, so every decision that 2.x took
on the terminal (`[[ -t 1 ]]`, `COLUMNS`, `tput cols` fallback) is passed in
by the shim. Shell state that 2.x read at load (pre-existing aliases, a set
`EDITOR`, `LESSOPEN`, `LS_COLORS`, the form of `PROMPT_COMMAND`) is tested by
the emitted script itself (`[[ -v BASH_ALIASES[ll] ]]`, `[[ -z ${EDITOR-} ]]`,
`${PROMPT_COMMAND@a}`), so the binary does not need it and the result is the
2.x result. No binary: one stderr line, the stubs stay defined and answer
`binary_not_found` (`compat.md` 2).

The emitted script is quoted with `mvdan.cc/sh/v3/syntax`; every value from
the machine or the user (paths, settings, banner text, alias values) is a
single-quoted literal. It defines `__gash_*` functions only where 2.x had
them as hooks (`__gash_prompt_command`, the history hook, the nvm stub,
`__gash_complete`), leaves no `BASH_REMATCH` and no readonly variables
behind, and returns 0.

## 2. Prompt

The prompt stays a Bash hook, `__gash_prompt_command`, with the 2.x logic
(audited line by line at S1): exit code first, `__AI_LAST_EXIT`, the history
hook, PS1 and PS2 built per prompt, the window title, `return $exit_code` so
later `PROMPT_COMMAND` entries see the user's `$?`. Installation into a
string or array `PROMPT_COMMAND` (prepend, idempotent) and removal are the
2.x code.

What it costs is what 2.x costs, by construction: the same subshell for the
job count, the same walk up `$PWD` for `.git` without forks, the same single
`GIT_OPTIONAL_LOCKS=0 git -C ROOT status --porcelain=v2 --branch
--untracked-files=no`, no exec outside repositories. A Go prompt helper was
measured and rejected: one exec of a static Go binary costs 2.1-2.8 ms (mean)
from the shell, more than the whole 2.x prompt in a non-git directory
(1.7-2.4 ms), and inside a repository it would still need `git status` or an
index walk to know the dirty state.

Expected effect of 3.x (inferred, measured at rc.A): forks get cheaper,
because the shell no longer holds 457 Gash functions and about 200
variables (13.4 MB RSS with 2.x loaded against 3.5 MB bare; fork cost grows
with it, `go-impl/bench/shell/README.md`).

One divergence, forced: 2.x splices the git branch name, the `VIRTUAL_ENV`
basename, `debian_chroot` and the screen session name raw into PS1, and Bash
re-expands PS1 before each prompt, so text in a branch name of a cloned
repository runs as code when the user enters it (LEDGER, Security). The 3.x
hook stores each of them in a variable and PS1 references the variable
(`\${__GASH_PS1_BRANCH}`): expansion happens once and is never re-scanned, so
ordinary names render byte for byte as in 2.x and no text is ever executed
(MUST NOT 7).

Latency proof at rc.A: `go-impl/bench/shell/prompt_bench.sh` on 2.1.0 and on
the branch, same machine, every case not slower (mean and p95). In CI, a
deterministic proxy: `strace -f -c` counts per prompt (non-git 1 fork and 0
exec, repository 2 forks and 1 exec, excluded 1 fork, `GASH_PROMPT=0` none).

## 3. History

Emitted as in 2.x (`GASH_HISTORY` on by default): `HISTCONTROL=ignoreboth`,
`histappend`, `HISTSIZE=-1`, `HISTFILESIZE=-1`, `export HISTTIMEFORMAT='%F %T '`,
set unconditionally (the 2.x overwrite of a user's `HISTCONTROL` is a LEDGER
defect, reproduced). The hook runs `history -a`, and `history -c; history -r`
with `GASH_HISTORY_SYNC=1`.

## 4. Terminal

`checkwinsize`; `EDITOR` only when unset (nano, else vim, resolved by the
binary on PATH); `LESSOPEN`/`LESSCLOSE` only when `LESSOPEN` is unset and
`lesspipe` exists (path resolved by the binary); `debian_chroot` from
`/etc/debian_chroot` when unset; `umask` from a valid `GASH_UMASK` (validated
by the binary, no regex in the shell); `LS_COLORS` only when unset, from
`dircolors -b` run by the binary with the same input file choice
(`~/.dircolors`, `/etc/DIR_COLORS`).

## 5. Toolchains

* `GASH_PATH_ADD`: the binary splits the value and emits `path_add 'a' 'b'`.
  2.x split it on the caller's `IFS` and glob-expanded it (it was unquoted);
  3.x splits on space, tab and newline and never globs (forced divergence,
  LEDGER). `path_add` keeps its 2.x semantics (existing directories only,
  `~` expanded, prepended, no duplicates).
* `GASH_NVM=on|1`: export `NVM_DIR`, source `nvm.sh` and nvm's completion in
  the shell (they define shell functions), as 2.x.
* `GASH_NVM=lazy`: the binary resolves the default version directory (the
  `alias/default` chain, up to 5 hops, `node`/`stable` = newest, exact
  `vX.Y.Z`, else newest matching prefix with `sort -V` order, ported), the
  script prepends its `bin` to PATH and defines the 2.x `nvm` stub.

## 6. Alias packs

The binary evaluates the 2.x packs (`colors navigation safety git docker php
general`, same order, same conditions, same values) from the registry and
emits the result. The probes 2.x ran in the shell (`ls --color=auto`, `rm
--version`, the grep color probe, `dircolors`) run in the binary with the
same commands and the same interpretation; the 2.x probe cache
(`~/.cache/gash/probes`) is moot and neither read nor written (left as it is
for a rollback to 2.x).

The shadow rule is exact without forks: for every alias that would hide a
program on PATH (the binary checks PATH, honoring the fixed exemption list
`ls dir vdir sl cp mv rm mkdir chown chmod chgrp grep bc reboot shutdown less
tail df top traceroute tracepath diff php composer php[0-9]* composer[0-9]*
1composer[0-9]*`), the script defines it only when it already existed before
Gash (`[[ -v 'BASH_ALIASES[gp]' ]] && alias gp='git push'`), which is what 2.x
did (the redefinition of a pre-existing alias is itself a LEDGER defect,
reproduced). Aliases that hide no program are defined unconditionally.
Module short aliases are defined by `gash.sh` in both modes (agent contract)
and, as in 2.x, are outside the shadow rule.

The `gl gla glo glg gls glf` git log functions and `help` are emitted as Bash
functions that call the binary (their output is a `git log` or the help
text); `quit` stays an alias.

## 7. Banner and SSH

`mesg n` when stdin is a terminal (in the script); `inxi -IpRS -v0 -c5` when
`GASH_SHOW_INXI=1` (in the script: it writes to the terminal); the banner
line and the quote are rendered by the binary with the color decision of
the shell's stdout (`GASH__TTY_OUT`) and the width rule of 2.x, and emitted
as one quoted `printf`. SSH auto-unlock: when keys are configured, the
script runs `eval "$("$__gash_bin" __compat ssh-unlock)"`, which drives
`ssh-add` through a PTY (secrets on the PTY only) and prints the quoted
`SSH_AUTH_SOCK`/`SSH_AGENT_PID` exports; otherwise only
`export GASH_SSH_AUTOUNLOCK_RAN=1`, as 2.x.

## 8. Completion

`bash_completion` is generated from the registry: the 2.x `__gash_complete`
function with the per-command flag lists and topic lists as a `case`
statement (no fork per TAB beyond the 2.x alias resolution), registered with
one `complete -o default -F __gash_complete NAMES` for every public function
and every registry alias that is live in the shell (tested with
`BASH_ALIASES`, no fork). The `gash` topic list keeps the 2.x `help`
candidate in rc.A (LEDGER defect).

## 9. Unload

`gash_unload` stays a shell function (it must change the calling shell). It
replays `__GASH_SNAPSHOT`, removes the aliases and functions Gash added, and
removes the hook, with the 2.x behavior including its leftovers (completion
specs, some exports, the user's own `GASH_*` settings removed, overwritten
aliases not restored: LEDGER defects with an exact 3.0 recipe where the init
script records every variable, alias and completion it touches with its
prior state). A 2.x snapshot taken before `source ~/.gashrc` re-sourced 3.x
over a 2.x session is honored, so unload returns to the pre-2.x state.

## 10. Startup

2.x: 90 ms headless, 110 ms interactive (mean; module parsing is about 80
ms). 3.x, expected (inferred): headless is `gash.sh` with about 100 one-line
stubs and no exec (a few ms); interactive adds one `gash init bash` exec
(binary start 2-7 ms, the probes the binary runs, about 1-2 ms each without
a cache) and the parse of a short script. Measured at rc.A with
`startup_bench.sh`; if the probes dominate, a cache keyed like 2.x's (PATH,
TERM, OSTYPE, dircolors file, version) under `$XDG_CACHE_HOME/gash/3/` is
added, with no change in behavior.

## 11. Tests

* State parity: `declare -p` (minus dynamic variables and the allowed
  internals), `alias -p`, `complete -p`, `shopt -p`, `set -o`, `trap -p`,
  `umask` after an interactive 2.1.0 load and after a 3.x load in the same
  fixture HOME: identical, for a matrix of settings (defaults, every setting
  off, `GASH_REPLACE_TOOLS=1`, `GASH_NVM=lazy`, `GASH_PATH_ADD`, pre-existing
  aliases, string and array `PROMPT_COMMAND`, docker and php fixtures on
  PATH). The same diff after `gash_unload` must equal 2.x's.
* Prompt: golden PS1 strings for every segment (chroot, venv, user colors,
  SSH host color, cwd, git clean/dirty/ahead/behind/detached/unborn, jobs,
  screen, exit code, color gate, UTF-8 and ASCII glyphs), byte-equal to 2.x;
  hostile branch names render literally and create no file.
* The existing `startup_spec.sh`, `hygiene_spec.sh`, `prompt_spec.sh`,
  `completion_spec.sh`, `unload_spec.sh`, `toolchains_spec.sh`,
  `settings_spec.sh` and alias specs run unchanged in Go mode.
