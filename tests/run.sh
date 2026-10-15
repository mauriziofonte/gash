#!/usr/bin/env bash
#
# Gash test runner.
#
# Usage: tests/run.sh [options] [SPEC...]
#
#   -j, --jobs N       run N spec files in parallel (default: nproc)
#   -f, --filter ERE   only run tests whose "<suite> :: <name>" matches ERE
#   -v, --verbose      stream every test's output
#       --awk NAME     run with NAME (mawk, gawk, busybox awk...) as `awk`
#       --bash PATH    run specs with the bash binary at PATH (version matrix)
#       --slow         also run the slow specs (see below)
#       --no-hermetic  keep the caller's environment (debugging only)
#   -h, --help         show this help
#
# SPEC can be a path or a name relative to tests/ (e.g. specs/files_spec.sh).
# By default every tests/specs/*_spec.sh but the slow ones (below) runs in
# its own process, under a scrubbed environment (env -i, temporary HOME,
# fixed PATH and locale), so results don't depend on the developer's shell
# or tokens. Set GASH_TEST_PATH to change the base PATH of that environment
# (e.g. GNU tools on macOS).
# A spec with a "# gash-test: slow" line (the terminal rendering fixtures)
# runs only with --slow or when named as SPEC; CI runs it in its own job.

set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

jobs="$(nproc 2>/dev/null || getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)"
filter=""
verbose=""
hermetic=1
awk_impl=""
bash_impl=""
slow=0
specs=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    -j | --jobs)
      [[ $# -ge 2 ]] || {
        echo "run.sh: $1 needs a value" >&2
        exit 2
      }
      jobs="$2"
      shift 2
      ;;
    -f | --filter)
      [[ $# -ge 2 ]] || {
        echo "run.sh: $1 needs a value" >&2
        exit 2
      }
      filter="$2"
      shift 2
      ;;
    -v | --verbose)
      verbose=1
      shift
      ;;
    --awk)
      [[ $# -ge 2 ]] || {
        echo "run.sh: $1 needs a value" >&2
        exit 2
      }
      awk_impl="$2"
      shift 2
      ;;
    --bash)
      [[ $# -ge 2 ]] || {
        echo "run.sh: $1 needs a value" >&2
        exit 2
      }
      bash_impl="$2"
      shift 2
      ;;
    --slow)
      slow=1
      shift
      ;;
    --no-hermetic)
      hermetic=0
      shift
      ;;
    -h | --help)
      sed -n '2,23p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    --)
      shift
      specs+=("$@")
      break
      ;;
    -*)
      echo "run.sh: unknown option $1" >&2
      exit 2
      ;;
    *)
      specs+=("$1")
      shift
      ;;
  esac
done

slow_skipped=()
if [[ ${#specs[@]} -eq 0 ]]; then
  for spec in "$ROOT_DIR"/tests/specs/*_spec.sh; do
    if ((! slow)) && grep -q '^# gash-test: slow$' "$spec"; then
      slow_skipped+=("${spec#"$ROOT_DIR"/tests/}")
      continue
    fi
    specs+=("$spec")
  done
else
  for i in "${!specs[@]}"; do
    if [[ ! -f "${specs[$i]}" && -f "$ROOT_DIR/tests/${specs[$i]}" ]]; then
      specs[$i]="$ROOT_DIR/tests/${specs[$i]}"
    fi
    [[ -f "${specs[$i]}" ]] || {
      echo "run.sh: spec not found: ${specs[$i]}" >&2
      exit 2
    }
  done
fi

# Physical path: on macOS /var is a symlink to /private/var, and tests
# compare $PWD with resolved paths.
workdir="$(cd "$(mktemp -d "${TMPDIR:-/tmp}/gash-run.XXXXXX")" && pwd -P)"
trap 'rm -rf "$workdir"' EXIT
results="$workdir/results"
: >"$results"

# Optional awk override: a shim dir placed first on PATH.
path_prefix=""
if [[ -n "$awk_impl" ]]; then
  awk_bin="$(command -v "$awk_impl")" || {
    echo "run.sh: awk implementation not found: $awk_impl" >&2
    exit 2
  }
  mkdir -p "$workdir/awkbin"
  ln -s "$awk_bin" "$workdir/awkbin/awk"
  path_prefix="$workdir/awkbin:"
  # Gash prefers gawk when installed; GASH_AWK pins the one under test.
  export GASH_TEST_AWK="$awk_bin"
fi
if [[ -n "$bash_impl" ]]; then
  [[ -x "$bash_impl" ]] || {
    echo "run.sh: not an executable: $bash_impl" >&2
    exit 2
  }
  mkdir -p "$workdir/bashbin"
  ln -s "$(cd "$(dirname "$bash_impl")" && pwd)/$(basename "$bash_impl")" "$workdir/bashbin/bash"
  path_prefix="$workdir/bashbin:$path_prefix"
fi

# Run one spec file in its own (optionally scrubbed) process.
run_spec() {
  local spec="$1" home
  home="$(mktemp -d "$workdir/home.XXXXXX")"
  local -a envv=(
    "GASH_TEST_ROOT=$ROOT_DIR"
    "GASH_TEST_RESULTS=$results"
    "GASH_TEST_FILTER=$filter"
    "GASH_TEST_VERBOSE=$verbose"
  )
  [[ -n "${NO_COLOR-}" ]] && envv+=("NO_COLOR=1")
  [[ -t 1 || -n "${GASH_TEST_COLOR-}" ]] && [[ -z "${NO_COLOR-}" ]] && envv+=("GASH_TEST_COLOR=1")
  [[ -n "${GASH_TEST_AWK-}" ]] && envv+=("GASH_AWK=$GASH_TEST_AWK")

  local script='source "$GASH_TEST_ROOT/tests/gash-test.sh"; source "$1"; gash_test_summary'
  if ((hermetic)); then
    env -i \
      HOME="$home" \
      USER="${USER:-$(id -un)}" \
      LOGNAME="${USER:-$(id -un)}" \
      PATH="${path_prefix}${GASH_TEST_PATH:-/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin}" \
      LANG="C.UTF-8" LC_ALL="C.UTF-8" \
      TERM="xterm-256color" \
      GIT_AUTHOR_NAME="Gash Test" GIT_AUTHOR_EMAIL="test@gash.invalid" \
      GIT_COMMITTER_NAME="Gash Test" GIT_COMMITTER_EMAIL="test@gash.invalid" \
      TMPDIR="$workdir" \
      "${envv[@]}" \
      bash -c "$script" _ "$spec"
  else
    env "${envv[@]}" PATH="${path_prefix}$PATH" bash -c "$script" _ "$spec"
  fi
}
export -f run_spec

start=$SECONDS
status=0

if [[ "$jobs" -le 1 ]]; then
  for spec in "${specs[@]}"; do
    run_spec "$spec" || status=1
  done
else
  # Run in parallel, buffer each spec's log, print logs in spec order.
  pids=()
  idx=0
  for spec in "${specs[@]}"; do
    while [[ $(jobs -rp | wc -l) -ge $jobs ]]; do
      wait -n 2>/dev/null || true
    done
    run_spec "$spec" >"$workdir/log.$idx" 2>&1 &
    pids+=($!)
    idx=$((idx + 1))
  done
  for i in "${!pids[@]}"; do
    wait "${pids[$i]}" || status=1
    cat "$workdir/log.$i"
  done
fi

total=0 failed=0 skipped=0
while read -r t f s; do
  total=$((total + t))
  failed=$((failed + f))
  skipped=$((skipped + s))
done <"$results"
passed=$((total - failed - skipped))
elapsed=$((SECONDS - start))

echo
for spec in "${slow_skipped[@]}"; do
  printf 'INFO Not run (slow, use --slow): %s\n' "$spec"
done
if [[ $failed -eq 0 && $status -eq 0 ]]; then
  printf 'PASS %d passed, %d failed, %d skipped (%d total) in %ds\n' "$passed" "$failed" "$skipped" "$total" "$elapsed"
else
  printf 'FAIL %d passed, %d failed, %d skipped (%d total) in %ds\n' "$passed" "$failed" "$skipped" "$total" "$elapsed"
  status=1
fi

exit "$status"
