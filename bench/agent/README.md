<!-- Gash 3.4 agent benchmark kit. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Agent benchmark kit (raw shell shapes vs gash functions)

The kit of the 3.4 agent-toolkit plan (section 12): each case times the raw
shell shape an agent types today (the baseline), sometimes an ideal shell
form, and the gash function meant to replace it (the target), warm and
cold, and checks the target against a relative rule. `gen.py` and
`agentbench.py` are kept here verbatim; nothing in the repository runs
them (no Go test, no lint, no CI job).

| Step | Command |
| --- | --- |
| Pick a disk-backed directory outside the repository | `B=$HOME/gash-bench; mkdir -p $B/kit; df -T $B` (must not be `tmpfs`) |
| Copy the two scripts | `cp bench/agent/gen.py bench/agent/agentbench.py $B/kit/` |
| Requirements | Python 3.8+, git, curl; optional: php, node, jq, sqlite3, rg, dig, tac (a variant whose tool is missing is skipped) |
| Build the corpus (~600 MB, ~40 s, deterministic) | `python3 -I $B/kit/gen.py $B` |
| Build gash as a release builds it (static, stripped: a cgo build starts about 1 ms slower) | `CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o $B/gash ./cmd/gash` (from the repository root) |
| Run every case, or the ones named | `GASH_BENCH_BIN=$B/gash python3 -I $B/kit/agentbench.py --root $B [case ...]` |
| Read | `$B/results/AGENTBENCH.md` (per-variant timings, PASS / FAIL / NA), `$B/results/agentbench.json` |

The corpus lives outside the working tree on purpose: it is a git
repository of ~34k files with vendor/ and node_modules/ trees, a 150 MB log
and a 50k-row database, which no repository walk must ever see.

Verdicts are relative. PASS or FAIL comes from the `ACCEPT` table in
`agentbench.py`: the target's time as a fraction of the baseline's on the
same machine, and a ceiling on its output bytes. NA means the target is
not implemented yet (detected from gash's unknown-function or
unknown-option envelopes) or a tool it needs is missing. Absolute numbers
vary with the machine, its load and its caches; compare runs of the same
host only.

Caveats: cold runs evict data pages with `posix_fadvise(DONTNEED)`, inode
and dentry caches stay warm. `wait-file-match` and `replace-20-files`
modify corpus files (restored, or harmless); `git-commit-push` adds
commits to the corpus repository and its bare remote. Results are written
to `$B/results/`, shared by every run on that root: copy them before the
next run.
