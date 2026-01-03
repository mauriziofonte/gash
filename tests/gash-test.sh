#!/usr/bin/env bash

# Minimal bash test framework for Gash.
# Usage: source this file, then write `describe` / `it` blocks.

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
    __gash_test__current_suite=""
fi

__gash_test__color() {
  local code="$1"; shift
  printf '\033[%sm%s\033[0m' "$code" "$*"
}

__gash_test__pass() {
  printf '%s %s\n' "$(__gash_test__color '1;32' 'PASS')" "$*"
}

__gash_test__fail() {
  printf '%s %s\n' "$(__gash_test__color '1;31' 'FAIL')" "$*"
}

__gash_test__note() {
  printf '%s %s\n' "$(__gash_test__color '1;34' 'INFO')" "$*"
}

describe() {
  __gash_test__current_suite="$1"
  __gash_test__note "Suite: $__gash_test__current_suite"
}

it() {
  local name="$1"; shift
  __gash_test__total=$((__gash_test__total + 1))

  if "$@"; then
    __gash_test__pass "$__gash_test__current_suite :: $name"
    return 0
  fi

  __gash_test__failed=$((__gash_test__failed + 1))
  __gash_test__fail "$__gash_test__current_suite :: $name"
  return 1
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

# expects_status <cmd...> <status>
expects_status() {
  local expected_status="$1"; shift
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
  local passed=$((__gash_test__total - __gash_test__failed))
  echo
  if [[ $__gash_test__failed -eq 0 ]]; then
    __gash_test__pass "$passed/$__gash_test__total tests passed"
    return 0
  fi

  __gash_test__fail "$passed/$__gash_test__total tests passed ($__gash_test__failed failed)"
  return 1
}

# Helper to source all Gash modules (core + functional)
# Usage: gash_source_all "$ROOT"
gash_source_all() {
  local root="${1:-.}"

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
