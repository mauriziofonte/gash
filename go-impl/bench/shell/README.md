<!-- Gash 3.0 shell benchmarks. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Shell benchmarks (prompt latency and startup)

PRD 5.4 and MUST NOT 10: the 3.x prompt must not be slower than 2.x. These
scripts measured 2.x at S1 and measure 3.x at rc.A, on the same machine, with
both trees side by side. They only read the Gash checkout they are given and
use a throwaway `HOME`.

```bash
bash make_fixtures.sh /tmp/fx                       # outside any git repository
bash prompt_bench.sh  ~/src/gash-2.1.0 /tmp/fx /tmp/out-2x 200
bash prompt_bench.sh  ~/src/gash-3     /tmp/fx /tmp/out-3x 200
bash startup_bench.sh ~/src/gash-2.1.0 /tmp/st-2x 30
bash fork_bench.sh    /tmp/fx/bin/true_static_go 200 /tmp/fork.us
```

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

## 3.x (rc.A)

To be filled at rc.A with the same scripts and machine class.
