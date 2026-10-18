#!/usr/bin/env bash

# Minimal bash test framework for Gash.
# Usage: source this file, then write `describe` / `it` blocks.
#
# Conventions:
#   - A test command exiting 0 is a PASS, 77 is a SKIP (automake convention),
#     anything else is a FAIL. Use `gash_skip "reason"` inside `bash -c`
#     bodies, or `exit 77` directly.
#   - Output of each test is captured and only shown when the test fails
#     (set GASH_TEST_VERBOSE=1 to always show it).
#
# Environment knobs:
#   GASH_TEST_FILTER   ERE matched against "<suite> :: <name>"; others are not run
#   GASH_TEST_FORMAT   "pretty" (default) or "tap"
#   GASH_TEST_VERBOSE  1 to stream every test's output
#   GASH_TEST_RESULTS  file to append "total failed skipped" to on summary
#   GASH_TEST_RECORD   file to append one line per test to, tab-separated:
#                      spec, line, name, status (pass, fail, skip), reason
#   NO_COLOR           disable colors (also off when stdout is not a TTY)
#   GASH_TEST_NO_COLOR the same, set by tests/run.sh, which never passes
#                      the caller's NO_COLOR to the specs (TEST-01)

# Fail if we're not sourcing
[[ "${BASH_SOURCE[0]}" != "${0}" ]] || {
  echo "Error: gash-test.sh must be sourced, not executed." >&2
  exit 1
}

set -u

# Initialize counters only once (protect against re-sourcing)
if [[ -z "${__gash_test__initialized-}" ]]; then
  __gash_test__initialized=1
  __gash_test__total=0
  __gash_test__failed=0
  __gash_test__skipped=0
  __gash_test__current_suite=""
  __gash_test__failures=()
  if [[ (-t 1 || -n "${GASH_TEST_COLOR-}") && -z "${NO_COLOR-}" && -z "${GASH_TEST_NO_COLOR-}" &&
    "${GASH_TEST_FORMAT:-pretty}" != "tap" ]]; then
    __gash_test__use_color=1
  else
    __gash_test__use_color=0
  fi
fi

__gash_test__color() {
  local code="$1"
  shift
  if ((__gash_test__use_color)); then
    printf '\033[%sm%s\033[0m' "$code" "$*"
  else
    printf '%s' "$*"
  fi
}

__gash_test__pass() {
  if [[ "${GASH_TEST_FORMAT:-pretty}" == "tap" ]]; then
    printf 'ok %d - %s\n' "$__gash_test__total" "$*"
  else
    printf '%s %s\n' "$(__gash_test__color '1;32' 'PASS')" "$*"
  fi
}

__gash_test__fail() {
  if [[ "${GASH_TEST_FORMAT:-pretty}" == "tap" ]]; then
    printf 'not ok %d - %s\n' "$__gash_test__total" "$*"
  else
    printf '%s %s\n' "$(__gash_test__color '1;31' 'FAIL')" "$*"
  fi
}

__gash_test__skip() {
  local name="$1" reason="${2-}"
  if [[ "${GASH_TEST_FORMAT:-pretty}" == "tap" ]]; then
    printf 'ok %d - %s # SKIP %s\n' "$__gash_test__total" "$name" "$reason"
  else
    printf '%s %s%s\n' "$(__gash_test__color '1;33' 'SKIP')" "$name" \
      "${reason:+ $(__gash_test__color '2' "($reason)")}"
  fi
}

__gash_test__note() {
  if [[ "${GASH_TEST_FORMAT:-pretty}" == "tap" ]]; then
    printf '# %s\n' "$*"
  else
    printf '%s %s\n' "$(__gash_test__color '1;34' 'INFO')" "$*"
  fi
}

describe() {
  __gash_test__current_suite="$1"
  __gash_test__note "Suite: $__gash_test__current_suite"
}

# Skip the current test from inside a `bash -c` body.
gash_skip() {
  [[ -n "${1-}" ]] && printf 'SKIP: %s\n' "$1"
  exit 77
}

# Skip unless every named command is available.
gash_require() {
  local cmd
  for cmd in "$@"; do
    command -v "$cmd" >/dev/null 2>&1 || gash_skip "$cmd not available"
  done
}

# The reason of a skip that printed no SKIP: line (TEST-03): the first
# command that a `command -v NAME ... exit 77` or `type -P NAME ... exit 77`
# line of the test body tests for and that is not available here, else
# "no reason".
__gash_test__skip_reason() {
  local body="${*: -1}" line name
  while IFS= read -r line; do
    [[ "$line" == *"exit 77"* ]] || continue
    while [[ "$line" =~ (command -v|type -P)\ +([A-Za-z0-9_.+-]+)(.*) ]]; do
      name="${BASH_REMATCH[2]}"
      line="${BASH_REMATCH[3]}"
      if ! command -v "$name" >/dev/null 2>&1; then
        printf '%s not available' "$name"
        return 0
      fi
    done
  done <<<"$body"
  printf 'no reason'
}

# One line of GASH_TEST_RECORD (tabs and newlines in fields become spaces).
__gash_test__record() {
  [[ -n "${GASH_TEST_RECORD-}" ]] || return 0
  local spec="$1" lineno="$2" name="$3" status="$4" reason="${5-}"
  name="${name//[$'\t\n']/ }" reason="${reason//[$'\t\n']/ }"
  printf '%s\t%s\t%s\t%s\t%s\n' "$spec" "$lineno" "$name" "$status" "$reason" >>"$GASH_TEST_RECORD"
}

it() {
  local name="$1"
  shift
  local full="$__gash_test__current_suite :: $name"
  local spec_file="${BASH_SOURCE[1]-}" spec_line="${BASH_LINENO[0]-0}"

  if [[ -n "${GASH_TEST_FILTER-}" ]] && ! [[ "$full" =~ $GASH_TEST_FILTER ]]; then
    return 0
  fi

  __gash_test__total=$((__gash_test__total + 1))

  local out rc
  out="$(mktemp "${TMPDIR:-/tmp}/gash-test.XXXXXX")"
  if [[ "${GASH_TEST_VERBOSE-}" == "1" ]]; then
    "$@" 2>&1 | tee "$out"
    rc=${PIPESTATUS[0]}
  else
    "$@" >"$out" 2>&1
    rc=$?
  fi

  case "$rc" in
    0)
      __gash_test__pass "$full"
      __gash_test__record "$spec_file" "$spec_line" "$name" pass
      ;;
    77)
      __gash_test__skipped=$((__gash_test__skipped + 1))
      local reason
      reason="$(grep -m1 -oE 'SKIP:? .*' "$out" 2>/dev/null | sed -E 's/^SKIP:? //')"
      [[ -n "$reason" ]] || reason="$(__gash_test__skip_reason "$@")"
      __gash_test__skip "$full" "$reason"
      __gash_test__record "$spec_file" "$spec_line" "$name" skip "$reason"
      ;;
    *)
      __gash_test__failed=$((__gash_test__failed + 1))
      __gash_test__failures+=("$full")
      __gash_test__fail "$full (exit $rc)"
      __gash_test__record "$spec_file" "$spec_line" "$name" fail "exit $rc"
      if [[ "${GASH_TEST_VERBOSE-}" != "1" && -s "$out" ]]; then
        tail -n 25 "$out" | sed 's/^/    │ /'
      fi
      ;;
  esac
  rm -f "$out"
  [[ $rc -eq 0 || $rc -eq 77 ]]
}

# expects <actual> <op> <expected>
# ops: ==, !=, =~
expects() {
  local actual="$1"
  local op="$2"
  local expected="$3"

  case "$op" in
    '==') [[ "$actual" == "$expected" ]] ;;
    '!=') [[ "$actual" != "$expected" ]] ;;
    '=~') [[ "$actual" =~ $expected ]] ;;
    *)
      __gash_test__fail "Unknown expects operator: $op"
      return 2
      ;;
  esac
}

# expects_status <status> <cmd...>
expects_status() {
  local expected_status="$1"
  shift
  "$@" >/dev/null 2>&1
  local status=$?
  [[ $status -eq $expected_status ]]
}

# expects_contains <haystack> <needle>
expects_contains() {
  local haystack="$1"
  local needle="$2"
  [[ "$haystack" == *"$needle"* ]]
}

gash_test_summary() {
  local passed=$((__gash_test__total - __gash_test__failed - __gash_test__skipped))

  if [[ -n "${GASH_TEST_RESULTS-}" ]]; then
    printf '%d %d %d\n' "$__gash_test__total" "$__gash_test__failed" "$__gash_test__skipped" >>"$GASH_TEST_RESULTS"
    [[ $__gash_test__failed -eq 0 ]]
    return
  fi

  if [[ "${GASH_TEST_FORMAT:-pretty}" == "tap" ]]; then
    printf '1..%d\n' "$__gash_test__total"
    [[ $__gash_test__failed -eq 0 ]]
    return
  fi

  echo
  local summary="$passed passed, $__gash_test__failed failed, $__gash_test__skipped skipped ($__gash_test__total total)"
  if [[ $__gash_test__failed -eq 0 ]]; then
    __gash_test__pass "$summary"
    return 0
  fi

  __gash_test__fail "$summary"
  local f
  for f in "${__gash_test__failures[@]}"; do
    printf '  - %s\n' "$f"
  done
  return 1
}

# Helper to source all Gash modules (core + functional)
# Usage: gash_source_all "$ROOT"
gash_source_all() {
  local root="${1:-.}"

  # A Gash 3 tree (the generated shims): its help registry is in the
  # binary, loaded here as the 2.x glob loaded lib/core/help.sh.
  [[ -r "$root/lib/compat/bootstrap.sh" ]] && __GASH_HELP_LOADED=1

  # Source core modules first
  for __gash_core_file in "$root"/lib/core/*.sh; do
    [ -f "$__gash_core_file" ] && source "$__gash_core_file"
  done
  unset __gash_core_file

  # Source functional modules
  for __gash_module_file in "$root"/lib/modules/*.sh; do
    [ -f "$__gash_module_file" ] && source "$__gash_module_file"
  done
  unset __gash_module_file
}
