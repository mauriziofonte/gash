#!/usr/bin/env bash
# Author: Maurizio Fonte <fonte.maurizio@gmail.com>
# Prompt latency benchmark for Gash 2.x (read-only on the Gash checkout).
#
# Usage: bash prompt_bench.sh GASH_DIR FIXTURE_DIR OUT_DIR [N]
#   FIXTURE_DIR must contain plain/sub (no .git above it) and repo/ (a git
#   repository with a few commits, clean tree).
# Every measurement runs inside `bash --noprofile --norc -i` (Gash only
# loads interactively) with a throwaway HOME and a scrubbed environment.
# Timings come from $EPOCHREALTIME around each call (microseconds).
set -u
GASH=${1:?gash dir}
FX=${2:?fixture dir}
OUT=${3:?out dir}
N=${4:-200}
mkdir -p "$OUT"
H="$(mktemp -d "$OUT/home.XXXX")"

inner() {
    # $1 = label, $2 = dir, $3 = extra env assignments (eval'd), $4 = what to time
    cat <<EOF
source "$GASH/gash.sh" >/dev/null 2>&1
set -o history; HISTFILE="$H/.bash_history"
$3
cd "$2" || exit 9
__gash_prompt_command   # warm-up (page cache, hash table)
: > "$OUT/$1.us"
for ((i = 0; i < $N; i++)); do
    t0=\${EPOCHREALTIME//[!0-9]/}
    $4
    t1=\${EPOCHREALTIME//[!0-9]/}
    printf '%d\n' \$((10#\$t1 - 10#\$t0)) >> "$OUT/$1.us"
done
EOF
}

run() {
    local label="$1" dir="$2" pre="$3" what="$4"
    env -i HOME="$H" PATH=/usr/local/bin:/usr/bin:/bin TERM=xterm-256color LANG=C.UTF-8 \
        USER="${USER:-root}" LOGNAME="${USER:-root}" GASH_BANNER=0 \
        bash --noprofile --norc -i -c "$(inner "$label" "$dir" "$pre" "$what")" </dev/null >/dev/null 2>&1
}

# (1) non-git directory, (2) git repo, (3) git repo excluded by GASH_GIT_EXCLUDE
run prompt_plain "$FX/plain/sub" "" "__gash_prompt_command"
run prompt_repo "$FX/repo/deep/er" "" "__gash_prompt_command"
run prompt_excluded "$FX/repo/deep/er" "GASH_GIT_EXCLUDE='/nonexistent:$FX/repo'" "__gash_prompt_command"
# PS1 expansion (the prompt string has no command substitution or functions;
# this times bash expanding the escapes, as readline would)
run ps1_expand_repo "$FX/repo/deep/er" "__gash_prompt_command" ': "${PS1@P}"'
# GASH_PROMPT=0 (hook still runs: history -a, exit status)
run prompt_off_repo "$FX/repo/deep/er" "GASH_PROMPT=0" "__gash_prompt_command"
# GASH_HISTORY_SYNC=1 with a 10,000-line history file
run prompt_histsync_plain "$FX/plain/sub" "for ((k=0;k<10000;k++)); do echo \"echo line \$k\"; done > \$HISTFILE; history -c; history -r; GASH_HISTORY_SYNC=1" "__gash_prompt_command"
# References: fork+exec of a static-ish trivial binary, and git invocations
run ref_true "$FX/plain/sub" "" "/bin/true"
BIN="$FX/bin"
[[ -x "$BIN/true_static_c" ]] && run ref_true_static_c "$FX/plain/sub" "" "'$BIN/true_static_c'"
[[ -x "$BIN/true_static_go" ]] && run ref_true_static_go "$FX/plain/sub" "" "'$BIN/true_static_go'"
run ref_subshell_builtin "$FX/plain/sub" "" 'x=$(jobs -pr; jobs -ps)'
run ref_git_revparse "$FX/repo/deep/er" "" "git rev-parse --abbrev-ref HEAD >/dev/null"
run ref_git_status_v2 "$FX/repo/deep/er" "" "GIT_OPTIONAL_LOCKS=0 git -C '$FX/repo' status --porcelain=v2 --branch --untracked-files=no >/dev/null"
# A larger work tree: the Gash checkout itself
run prompt_gash_checkout "$GASH/lib" "" "__gash_prompt_command"

python3 -I - "$OUT" <<'PY'
import sys, os, statistics as st
out = sys.argv[1]
rows = []
for name in ["prompt_plain", "prompt_repo", "prompt_excluded", "ps1_expand_repo", "prompt_off_repo",
             "prompt_histsync_plain", "ref_true", "ref_true_static_c", "ref_true_static_go", "ref_subshell_builtin", "ref_git_revparse",
             "ref_git_status_v2", "prompt_gash_checkout"]:
    p = os.path.join(out, name + ".us")
    if not os.path.exists(p):
        rows.append((name, "missing")); continue
    v = sorted(int(x) for x in open(p) if x.strip())
    if not v:
        rows.append((name, "empty")); continue
    n = len(v)
    p50 = v[int(0.50 * (n - 1))]
    p95 = v[int(0.95 * (n - 1))]
    rows.append((name, f"n={n} mean={st.mean(v)/1000:.3f} ms p50={p50/1000:.3f} ms p95={p95/1000:.3f} ms min={v[0]/1000:.3f} max={v[-1]/1000:.3f}"))
for r in rows:
    print(f"{r[0]:24s} {r[1]}")
PY
command rm -rf "$H"
