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
#       --impl IMPL    bash (default: the 2.x sources of this checkout), go
#                      (the Gash 3 binary behind the generated shims, in a
#                      staged copy of the checkout) or both
#       --results FILE also write one line per test, tab-separated: spec,
#                      line, name, status (pass, fail, skip), skip reason;
#                      go mode writes tests/.results/go.tsv by default
#       --gash-bin PATH  go mode: use this binary instead of building one
#       --no-isolate   go mode: do not run in a PID namespace
#       --no-network   go mode: also run in a network namespace
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
#
# Go mode builds go-impl/cmd/gash (or takes --gash-bin), stages the
# checkout with `gash __compat stage` and runs the unchanged specs from the
# staged tree. When unshare(1) allows it, the run happens in its own PID
# namespace, so no test can signal a process it did not start; update the
# PARITY Go column with `go run ./internal/tools/parity` in go-impl.

set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

jobs="$(nproc 2>/dev/null || getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)"
filter=""
verbose=""
hermetic=1
awk_impl=""
bash_impl=""
slow=0
impl=bash
results_file=""
gash_bin=""
isolate=1
no_network=0
in_namespace=0
specs=()
# The arguments as given, for the go part of --impl both and the
# namespace re-exec.
orig_args=("$@")

# A job count: an integer >= 1 (TEST-02).
check_jobs() {
  [[ "$1" =~ ^[0-9]+$ ]] && (($1 >= 1)) || {
    echo "run.sh: --jobs needs an integer >= 1, got: $1" >&2
    exit 2
  }
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -j | --jobs)
      [[ $# -ge 2 ]] || {
        echo "run.sh: $1 needs a value" >&2
        exit 2
      }
      check_jobs "$2"
      jobs="$2"
      shift 2
      ;;
    -j?*)
      check_jobs "${1#-j}"
      jobs="${1#-j}"
      shift
      ;;
    --jobs=*)
      check_jobs "${1#--jobs=}"
      jobs="${1#--jobs=}"
      shift
      ;;
    --impl | --impl=*)
      if [[ "$1" == --impl=* ]]; then
        impl="${1#--impl=}"
        shift
      else
        [[ $# -ge 2 ]] || {
          echo "run.sh: $1 needs a value" >&2
          exit 2
        }
        impl="$2"
        shift 2
      fi
      case "$impl" in
        bash | go | both) ;;
        *)
          echo "run.sh: --impl must be bash, go or both, got: $impl" >&2
          exit 2
          ;;
      esac
      ;;
    --results)
      [[ $# -ge 2 ]] || {
        echo "run.sh: $1 needs a value" >&2
        exit 2
      }
      results_file="$2"
      shift 2
      ;;
    --gash-bin)
      [[ $# -ge 2 ]] || {
        echo "run.sh: $1 needs a value" >&2
        exit 2
      }
      gash_bin="$2"
      shift 2
      ;;
    --no-isolate)
      isolate=0
      shift
      ;;
    --no-network)
      no_network=1
      shift
      ;;
    --in-namespace)
      in_namespace=1
      shift
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
      sed -n '2,38p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
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

# --impl both: the bash run, then the go run, each with its own summary.
if [[ "$impl" == both ]]; then
  rest=() skip_next=0
  for a in "${orig_args[@]}"; do
    if ((skip_next)); then
      skip_next=0
      continue
    fi
    case "$a" in
      --impl) skip_next=1 ;;
      --impl=*) ;;
      *) rest+=("$a") ;;
    esac
  done
  bash "${BASH_SOURCE[0]}" --impl bash ${rest[@]+"${rest[@]}"}
  s_bash=$?
  echo
  bash "${BASH_SOURCE[0]}" --impl go ${rest[@]+"${rest[@]}"}
  s_go=$?
  ((s_bash > s_go)) && exit "$s_bash"
  exit "$s_go"
fi

# Go mode runs in its own PID namespace (and, with --no-network, network
# namespace) when unshare allows it: as root directly, otherwise in a user
# namespace that maps the caller's own uid.
if [[ "$impl" == go ]] && ((isolate && ! in_namespace)) && command -v unshare >/dev/null 2>&1; then
  ns=(--pid --fork --mount-proc)
  ((no_network)) && ns+=(--net)
  if [[ "$(id -u)" != 0 ]]; then
    ns=(--user --map-current-user "${ns[@]}")
  fi
  if unshare "${ns[@]}" true >/dev/null 2>&1; then
    exec unshare "${ns[@]}" bash "${BASH_SOURCE[0]}" --in-namespace "${orig_args[@]}"
  fi
  echo "INFO unshare cannot create a PID namespace here: go mode runs without isolation" >&2
fi

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

# Per-test records: --results FILE, and always in go mode.
if [[ "$impl" == go && -z "$results_file" ]]; then
  results_file="$ROOT_DIR/tests/.results/go.tsv"
fi
records=""
if [[ -n "$results_file" ]]; then
  mkdir -p "$(dirname "$results_file")" || exit 2
  records="$workdir/records"
  : >"$records"
fi

# Go mode: the binary, and the staged tree the specs run from.
test_root="$ROOT_DIR"
gash_env=()
if [[ "$impl" == go ]]; then
  if [[ -z "$gash_bin" ]]; then
    command -v go >/dev/null 2>&1 || {
      echo "run.sh: --impl go needs Go (or --gash-bin PATH)" >&2
      exit 2
    }
    gash_bin="$workdir/gash"
    echo "INFO Building go-impl/cmd/gash"
    (cd "$ROOT_DIR/go-impl" && CGO_ENABLED=0 go build -o "$gash_bin" ./cmd/gash) || {
      echo "run.sh: go build failed" >&2
      exit 2
    }
  fi
  [[ -x "$gash_bin" ]] || {
    echo "run.sh: not an executable: $gash_bin" >&2
    exit 2
  }
  stage_out="$("$gash_bin" __compat stage --out "$workdir/stage" --root "$ROOT_DIR")" || {
    echo "run.sh: staging failed" >&2
    exit 2
  }
  gash_env=("${stage_out%%$'\n'*}")
  test_root="$workdir/stage"
  for i in "${!specs[@]}"; do
    case "${specs[$i]}" in
      "$ROOT_DIR"/*) specs[$i]="$test_root/${specs[$i]#"$ROOT_DIR"/}" ;;
    esac
  done
  ((in_namespace)) && ns_note=", PID namespace" || ns_note=""
  echo "INFO Implementation: go (${gash_env[0]}${ns_note})"
fi

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
    "GASH_TEST_ROOT=$test_root"
    "GASH_TEST_RESULTS=$results"
    "GASH_TEST_FILTER=$filter"
    "GASH_TEST_VERBOSE=$verbose"
  )
  # The caller's NO_COLOR only turns the runner's own colors off: specs
  # never inherit it (TEST-01).
  [[ -n "${NO_COLOR-}" ]] && envv+=("GASH_TEST_NO_COLOR=1")
  [[ -t 1 || -n "${GASH_TEST_COLOR-}" ]] && [[ -z "${NO_COLOR-}" ]] && envv+=("GASH_TEST_COLOR=1")
  [[ -n "${GASH_TEST_AWK-}" ]] && envv+=("GASH_AWK=$GASH_TEST_AWK")
  [[ -n "$records" ]] && envv+=("GASH_TEST_RECORD=$records")
  envv+=(${gash_env[@]+"${gash_env[@]}"})

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

if [[ -n "$records" ]]; then
  # Spec paths relative to tests/specs, whatever tree they ran from.
  sed -e "s#^$test_root/tests/specs/##" -e "s#^$ROOT_DIR/tests/specs/##" "$records" >"$results_file"
fi

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
[[ -n "$records" ]] && printf 'INFO Per-test results: %s\n' "$results_file"

exit "$status"
