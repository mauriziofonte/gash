#!/usr/bin/env bash
# Author: Maurizio Fonte <fonte.maurizio@gmail.com>
# Startup cost of Gash 2.x, 30 runs each (read-only on the Gash checkout).
#
# Usage: bash startup_bench.sh GASH_DIR OUT_DIR [RUNS]
#
#   bare_i        bash --noprofile --norc -i -c true          (baseline)
#   bare_c        bash -c true                                (baseline)
#   inter_warm    bash -i: source gash.sh + bash_completion (as ~/.gashrc does), probe cache warm
#   inter_cold    same, probe cache deleted before each run
#   inter_nobanner same as inter_warm with GASH_BANNER=0
#   headless      GASH_HEADLESS=1 bash -c 'source gash.sh; true'
#   timing        GASH_TIMING=1 interactive load: per-step __GASH_LOAD_TIMES, averaged
#
# Wall time is measured by the parent with $EPOCHREALTIME around the child.
set -u
GASH=${1:?gash dir}
OUT=${2:?out dir}
RUNS=${3:-30}
mkdir -p "$OUT"
H="$(mktemp -d "$OUT/home.XXXX")"
E=(env -i HOME="$H" PATH=/usr/local/bin:/usr/bin:/bin TERM=xterm-256color LANG=C.UTF-8 USER="${USER:-root}" LOGNAME="${USER:-root}")

t() { # label cmd...
    local label="$1" t0 t1
    shift
    t0=${EPOCHREALTIME//[!0-9]/}
    "$@" </dev/null >/dev/null 2>&1
    t1=${EPOCHREALTIME//[!0-9]/}
    printf '%d\n' $((10#$t1 - 10#$t0)) >>"$OUT/$label.us"
}

for f in bare_i bare_c inter_warm inter_cold inter_nobanner headless; do : >"$OUT/$f.us"; done
: >"$OUT/timing.raw"

# warm the probe cache and the page cache once
"${E[@]}" bash --noprofile --norc -i -c "source '$GASH/gash.sh'; source '$GASH/bash_completion'" </dev/null >/dev/null 2>&1

for ((r = 0; r < RUNS; r++)); do
    t bare_i "${E[@]}" bash --noprofile --norc -i -c true
    t bare_c "${E[@]}" bash -c true
    t inter_warm "${E[@]}" bash --noprofile --norc -i -c "source '$GASH/gash.sh'; source '$GASH/bash_completion'"
    t inter_nobanner "${E[@]}" GASH_BANNER=0 bash --noprofile --norc -i -c "source '$GASH/gash.sh'; source '$GASH/bash_completion'"
    rm -rf "$H/.cache/gash"
    t inter_cold "${E[@]}" bash --noprofile --norc -i -c "source '$GASH/gash.sh'; source '$GASH/bash_completion'"
    t headless "${E[@]}" GASH_HEADLESS=1 bash -c "source '$GASH/gash.sh'; true"
    # per-step timings (cache warm again after the cold run above)
    "${E[@]}" GASH_TIMING=1 bash --noprofile --norc -i -c "source '$GASH/gash.sh'; source '$GASH/bash_completion'; printf '%s\n' \"\${__GASH_LOAD_TIMES[@]}\" >&3" \
        3>>"$OUT/timing.raw" </dev/null >/dev/null 2>&1
done

python3 -I - "$OUT" <<'PY'
import sys, os, statistics as st, collections
out = sys.argv[1]
def stats(v):
    v = sorted(v); n = len(v)
    return f"n={n} mean={st.mean(v)/1000:.1f} ms p50={v[int(.5*(n-1))]/1000:.1f} ms p95={v[int(.95*(n-1))]/1000:.1f} ms min={v[0]/1000:.1f} max={v[-1]/1000:.1f}"
for name in ["bare_c", "bare_i", "headless", "inter_warm", "inter_nobanner", "inter_cold"]:
    v = [int(x) for x in open(os.path.join(out, name + ".us")) if x.strip()]
    print(f"{name:16s} {stats(v)}")
steps = collections.OrderedDict()
for line in open(os.path.join(out, "timing.raw")):
    line = line.rstrip("\n")
    if not line: continue
    us, _, label = line.partition(" ")
    steps.setdefault(label, []).append(int(us))
print("\nGASH_TIMING=1 per-step (mean / p50 / p95 ms over runs):")
for label, v in steps.items():
    v2 = sorted(v); n = len(v2)
    print(f"  {st.mean(v2)/1000:7.2f} {v2[int(.5*(n-1))]/1000:7.2f} {v2[int(.95*(n-1))]/1000:7.2f}  {label}")
PY
command rm -rf "$H"
