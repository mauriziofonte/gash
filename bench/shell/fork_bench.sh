#!/usr/bin/env bash
# Author: Maurizio Fonte <fonte.maurizio@gmail.com>
# Fork+exec cost of BIN from a bare interactive bash (no Gash loaded), N runs.
# Usage: bash fork_bench.sh BIN [N] OUTFILE
BIN=${1:?binary} N=${2:-200} OUT=${3:?out file}
env -i HOME=/nonexistent PATH=/usr/bin:/bin TERM=xterm-256color LANG=C.UTF-8 bash --noprofile --norc -i -c "
: > '$OUT'
for ((i=0;i<$N;i++)); do t0=\${EPOCHREALTIME//[!0-9]/}; '$BIN'; t1=\${EPOCHREALTIME//[!0-9]/}; echo \$((10#\$t1-10#\$t0)) >> '$OUT'; done" </dev/null >/dev/null 2>&1
python3 -I -c "
import statistics as st
v=sorted(int(x) for x in open('$OUT'));n=len(v)
print(f'n={n} mean={st.mean(v)/1000:.3f} ms p50={v[int(.5*(n-1))]/1000:.3f} ms p95={v[int(.95*(n-1))]/1000:.3f} ms')"
