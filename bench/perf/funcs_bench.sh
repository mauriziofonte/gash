#!/usr/bin/env bash
# Author: Maurizio Fonte <fonte.maurizio@gmail.com>
# Per-call latency of Gash functions, in a headless shell with Gash loaded.
#
# Usage: bash funcs_bench.sh GASH_DIR CMDS_FILE WORKDIR OUT_TSV [N]
#
# CMDS_FILE holds one command per line ("label<TAB>command"); each runs N
# times (after one warm-up call) in WORKDIR, output discarded, timed with
# $EPOCHREALTIME. OUT_TSV gets "label<TAB>mean_us<TAB>p50_us<TAB>p95_us<TAB>status".
# The environment is scrubbed (env -i) and HOME is a throwaway directory
# unless BENCH_HOME is set; GASH_BIN is passed through, and BENCH_ENV adds
# NAME=VALUE words (e.g. BENCH_ENV=GASH_SERVER=0 for the exec path).
set -u
GASH=${1:?gash dir}
CMDS=${2:?commands file}
WORK=${3:?workdir}
OUT=${4:?out tsv}
N=${5:-50}
H=${BENCH_HOME:-$(mktemp -d)}
: >"$OUT"
while IFS=$'\t' read -r label cmd; do
    [[ -z "$label" || "$label" == \#* ]] && continue
    env -i HOME="$H" PATH=/usr/local/bin:/usr/bin:/bin TERM=xterm-256color LANG=C.UTF-8 \
        USER="${USER:-root}" LOGNAME="${USER:-root}" GASH_HEADLESS=1 \
        ${GASH_BIN:+"GASH_BIN=$GASH_BIN"} ${BENCH_ENV-} bash -c '
source "$1/gash.sh" >/dev/null 2>&1
cd "$2" || exit 9
eval "$3" >/dev/null 2>&1 </dev/null
st=$?
ts=()
for ((i = 0; i < $4; i++)); do
    t0=${EPOCHREALTIME//[!0-9]/}
    eval "$3" >/dev/null 2>&1 </dev/null
    t1=${EPOCHREALTIME//[!0-9]/}
    ts+=($((10#$t1 - 10#$t0)))
done
printf "%s\n" "${ts[@]}" | sort -n | awk -v st="$st" -v l="$5" "{a[NR]=\$1; s+=\$1} END {printf \"%s\t%d\t%d\t%d\t%s\n\", l, s/NR, a[int((NR+1)/2)], a[int(NR*0.95+0.5)], st}"
' _ "$GASH" "$WORK" "$cmd" "$N" "$label" >>"$OUT"
done <"$CMDS"
