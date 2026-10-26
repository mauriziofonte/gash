<!-- Gash 3.0 shell benchmarks. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Shell benchmarks (prompt latency, startup and per-call cost)

PRD 5.4 and MUST NOT 10: the 3.x prompt must not be slower than 2.x. These
scripts measured 2.x at S1 and 3.2.0 against 2.1.1 below, on the same
machine class, with both trees side by side. They only read the Gash checkout they are given and
use a throwaway `HOME`.

```bash
bash make_fixtures.sh /tmp/fx                       # outside any git repository
bash prompt_bench.sh  ~/src/gash-2.1.0 /tmp/fx /tmp/out-2x 200
bash prompt_bench.sh  ~/src/gash-3     /tmp/fx /tmp/out-3x 200
bash startup_bench.sh ~/src/gash-2.1.0 /tmp/st-2x 30
bash fork_bench.sh    /tmp/fx/bin/true_static_go 200 /tmp/fork.us
```

Both scripts run each measurement under `env -i` with a small fixed
environment. A `GASH_BIN` set in the calling shell is passed through, so a
3.x run can measure a chosen binary (`GASH_BIN=/path/to/gash bash
startup_bench.sh ...`); without it `gash.sh` uses the release binary of the
checkout's `VERSION` under the throwaway `HOME`
(`~/.local/share/gash/versions/`), fetched on first use.

`prompt_bench.sh` calls the hook `__gash_prompt_command` (the 3.x hook keeps
that name) inside `bash --noprofile --norc -i`, 200 times per case, timed with
`$EPOCHREALTIME`.

## 2.x baseline (S1, 2026-10-18)

Machine: 4 vCPU Intel Xeon @ 2.80 GHz VM, Linux 6.18.44, Ubuntu 24.04.5, GNU
bash 5.2.21, git 2.43.0. Gash 2.1.0 (`main`, a4fbc8b). Two runs (run1 | run2).

### Prompt (ms, N=200)

| Case | mean | p50 | p95 |
|---|---|---|---|
| Non-git directory | 1.714 \| 2.437 | 1.638 \| 2.349 | 2.283 \| 3.357 |
| Git repository (6 commits, clean) | 6.697 \| 7.153 | 6.459 \| 6.958 | 8.917 \| 9.056 |
| Same repository in `GASH_GIT_EXCLUDE` | 2.238 \| 2.123 | 2.115 \| 2.054 | 3.043 \| 2.744 |
| Gash checkout (larger tree) | 8.955 \| 7.358 | 7.788 \| 7.080 | 16.230 \| 9.428 |
| `GASH_PROMPT=0` (history hook only) | 0.046 \| 0.027 | 0.042 \| 0.023 | 0.077 \| 0.040 |
| `GASH_HISTORY_SYNC=1`, 10,000-line history | 12.585 \| 11.569 | 11.676 \| 10.884 | 21.245 \| 15.071 |
| PS1 expansion `${PS1@P}` | 0.020 \| 0.018 | 0.016 \| 0.016 | 0.031 \| 0.023 |

Processes per prompt (`strace -f`): non-git directory 1 fork, 0 exec (the
`$(jobs -pr; jobs -ps)` subshell); git repository 2 forks + 1 exec (process
substitution and `git status --porcelain=v2 --branch --untracked-files=no`);
excluded repository 1 fork; `GASH_PROMPT=0` none.

### Reference costs (ms, N=200, mean / p50 / p95)

| Operation | From a shell with Gash 2.x loaded | From a bare `bash -i` |
|---|---|---|
| `/bin/true` | 2.183 / 2.160 / 2.863 | 1.959 / 1.867 / 2.419 |
| Static Go binary (`CGO_ENABLED=0`, empty `main`) | 2.805 / 2.382 / 4.877 | 2.266 / 2.108 / 3.451 |
| Subshell `$(jobs -pr; jobs -ps)` | 1.195 / 1.169 / 1.568 | |
| `git status --porcelain=v2 --branch --untracked-files=no` | 4.161 / 3.762 / 7.146 | |

Fork cost grows with the shell's memory: 13.4 MB RSS with Gash 2.x loaded
against 3.5 MB for a bare shell.

### Startup (ms, 30 runs, warm page cache)

| Case | mean | p50 | p95 |
|---|---|---|---|
| `bash -c true` | 4.5 | 4.1 | 6.2 |
| `bash --noprofile --norc -i -c true` | 4.9 | 4.6 | 6.3 |
| Headless `GASH_HEADLESS=1 bash -c 'source gash.sh; true'` | 90.3 | 81.8 | 116.8 |
| Interactive `gash.sh` + `bash_completion`, probe cache warm | 110.2 | 101.1 | 148.1 |
| Same, `GASH_BANNER=0` | 107.6 | 99.6 | 139.1 |
| Same, probe cache deleted before each run | 138.0 | 133.9 | 177.7 |

Per step (`GASH_TIMING=1`, mean): `gash.sh` 98.97 ms in total, of which
module parsing is about 80 ms (llm.sh 14.00, files.sh 8.60, ai.sh 7.32,
system.sh 6.69, ...); alias packs 5.11; banner 4.23; `bash_completion` 5.93.

## 3.2.0 against 2.1.1 (2026-10-25)

Same machine class as the baseline: 4 vCPU Intel Xeon @ 2.80 GHz VM, Linux
6.18.44, Ubuntu 24.04.5, GNU bash 5.2.21, git 2.43.0. Gash 2.1.1 (tag
`v2.1.1`) and Gash 3.2.0 (tag `v3.2.0`, binary built from the tag with
`CGO_ENABLED=0 -trimpath` and passed as `GASH_BIN`), measured one after the
other in the same session, each from a git checkout of its own tag:

```bash
git worktree add /tmp/g2 v2.1.1 && git worktree add /tmp/g3 v3.2.0
bash prompt_bench.sh  /tmp/g2 /tmp/fx /tmp/p2 200
GASH_BIN=/tmp/gash-3.2.0 bash prompt_bench.sh /tmp/g3 /tmp/fx /tmp/p3 200
bash startup_bench.sh /tmp/g2 /tmp/s2 30
GASH_BIN=/tmp/gash-3.2.0 bash startup_bench.sh /tmp/g3 /tmp/s3 30
bash ../perf/funcs_bench.sh /tmp/g2 ../perf/cmds.tsv /tmp/g3 /tmp/f2.tsv 50
GASH_BIN=/tmp/gash-3.2.0 bash ../perf/funcs_bench.sh /tmp/g3 ../perf/cmds.tsv /tmp/g3 /tmp/f3.tsv 50
GASH_BIN=/tmp/gash-3.2.0 BENCH_ENV=GASH_SERVER=0 bash ../perf/funcs_bench.sh /tmp/g3 ../perf/cmds.tsv /tmp/g3 /tmp/f3n.tsv 50
```

### Prompt (ms, N=200, mean / p50 / p95)

| Case | 2.1.1 | 3.2.0 | Ratio (mean) |
|---|---|---|---|
| Non-git directory | 1.434 / 1.378 / 1.815 | 0.375 / 0.310 / 0.568 | 3.8x |
| Git repository (6 commits, clean) | 5.858 / 5.475 / 7.886 | 3.870 / 3.750 / 4.757 | 1.5x |
| Same repository in `GASH_GIT_EXCLUDE` | 1.560 / 1.477 / 1.904 | 0.329 / 0.299 / 0.426 | 4.7x |
| Gash checkout of the same tag (larger tree) | 16.593 / 15.591 / 22.676 | 7.119 / 6.841 / 8.951 | 2.3x |
| `GASH_PROMPT=0` (history hook only) | 0.025 / 0.022 / 0.041 | 0.054 / 0.049 / 0.092 | 0.5x |
| `GASH_HISTORY_SYNC=1`, 10,000-line history | 10.584 / 10.315 / 12.806 | 8.599 / 8.254 / 10.703 | 1.2x |
| PS1 expansion `${PS1@P}` | 0.017 / 0.013 / 0.048 | 0.014 / 0.013 / 0.017 | 1.2x |

The non-git prompt forks nothing in 3.2.0 (the jobs count is `\j`, no
subshell); a git prompt still runs one `git status`, which is most of its
cost (`git status --porcelain=v2`: 3.2 ms alone in the reference costs of
this run). `GASH_PROMPT=0` is 0.03 ms slower in 3.2.0, far below anything
a user sees.

Reference costs of the same run (ms, mean): `/bin/true` 2.057 from a shell
with 2.1.1 loaded and 1.645 with 3.2.0 loaded; the subshell
`$(jobs -pr; jobs -ps)` 1.062 and 0.917. A fork costs less in a 3.2.0 shell,
consistent with the smaller shell (most functions are stand-ins until their
first call; see the fork cost note of the baseline).

### Startup (ms, 30 runs, warm page cache, mean / p50 / p95)

| Case | 2.1.1 | 3.2.0 | Ratio (mean) |
|---|---|---|---|
| `bash -c true` | 4.3 / 3.9 / 5.9 | 4.3 / 4.1 / 4.7 | |
| `bash --noprofile --norc -i -c true` | 4.4 / 4.1 / 5.7 | 4.4 / 4.2 / 5.0 | |
| Headless `GASH_HEADLESS=1 bash -c 'source gash.sh; true'` | 79.3 / 75.3 / 92.3 | 11.5 / 9.9 / 15.4 | 6.9x |
| Interactive `gash.sh` + `bash_completion`, probe cache warm | 104.5 / 97.4 / 137.0 | 30.9 / 29.8 / 36.0 | 3.4x |
| Same, `GASH_BANNER=0` | 100.3 / 98.3 / 128.8 | 30.8 / 30.1 / 36.4 | 3.3x |
| Same, probe cache deleted before each run | 122.7 / 118.2 / 143.1 | 34.7 / 34.0 / 40.1 | 3.5x |

Per step in 3.2.0 (`GASH_TIMING=1`, mean): `gash.sh` 24.78 ms in total, of
which `gash init bash` (the binary building the interactive layer) is
14.72, the bootstrap 1.89 and the prompt block 0.90; every module file is a
list of stand-ins that loads in 0.02 to 0.19 ms (2.1.1 parsed each module:
llm.sh 13.19, files.sh 8.13, ai.sh 6.69); `bash_completion` 3.32 (5.96).

### Per call, in a shell with Gash loaded (ms, N=50 after one warm-up)

`funcs_bench.sh` with [`../perf/cmds.tsv`](../perf/cmds.tsv): each command
runs 50 times in one headless shell, in a checkout of Gash 3.2.0. "Server"
is the 3.2.0 default (the per-shell server runs the call); "exec" is
`GASH_SERVER=0` (the binary is started for every call, as in 3.0 and 3.1).

| Command | 2.1.1 | 3.2.0 server | Ratio | 3.2.0 exec | Ratio |
|---|---|---|---|---|---|
| `llm_config go.mod` | 24.8 | 0.66 | 37.6x | 6.50 | 3.8x |
| `llm_project` | 15.9 | 0.50 | 31.5x | 7.15 | 2.2x |
| `llm_env PATH` | 11.2 | 0.51 | 22.0x | 6.88 | 1.6x |
| `dirs_find_large --json internal` | 75.2 | 3.86 | 19.5x | 10.88 | 6.9x |
| `tree_stats internal` | 62.2 | 3.65 | 17.0x | 10.93 | 5.7x |
| `llm_find '*.go' internal -n 50` | 15.5 | 1.05 | 14.7x | 7.29 | 2.1x |
| `dirs_largest -n 10 internal` | 44.4 | 3.74 | 11.9x | 10.38 | 4.3x |
| `gash files` (card topic) | 31.2 | 3.00 | 10.4x | 12.06 | 2.6x |
| `llm_exec 'ls internal'` | 132.3 | 14.58 | 9.1x | 19.81 | 6.7x |
| `llm_tree --depth 2 internal` | 21.4 | 3.04 | 7.0x | 8.33 | 2.6x |
| `files_largest -n 10 .` | 53.3 | 11.62 | 4.6x | 20.46 | 2.6x |
| `gash_help files_largest` | 3.0 | 0.71 | 4.2x | 8.41 | 0.4x |
| `dirs_list_empty --json .` | 40.4 | 10.47 | 3.9x | 17.25 | 2.3x |
| `disk_usage` | 11.6 | 3.25 | 3.6x | 9.87 | 1.2x |
| `llm_git_log -n 10` | 22.2 | 6.45 | 3.4x | 14.21 | 1.6x |
| `llm_git_diff` | 23.7 | 7.68 | 3.1x | 13.96 | 1.7x |
| `llm_git_status` | 37.4 | 12.40 | 3.0x | 18.81 | 2.0x |
| `llm_grep 'func Register' internal -n 50` | 33.8 | 13.84 | 2.4x | 22.34 | 1.5x |
| `process_find bash` | 17.7 | 9.41 | 1.9x | 13.97 | 1.3x |
| `llm_procs` | 10.4 | 8.60 | 1.2x | 13.52 | 0.8x |
| `git_list_tags` | 171.7 | 168.68 | 1.0x | 164.18 | 1.0x |

`llm_ports` is left out: neither `ss` nor `netstat` is installed on this
machine, so both versions return `ports_tool_not_found` (2.1.1 in 5.4 ms,
3.2.0 in 0.64). The calls bound by an external program (`git` for the
`llm_git_*` functions and `git_list_tags`, `ps` for `llm_procs` and
`process_find`) gain the least: the program's own run is the floor. Without
the server, a call pays the start of the binary (about 6 ms here), which is
why `gash_help` is slower than 2.1.1's in-shell function on the exec path.
