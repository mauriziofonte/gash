#!/usr/bin/env bash

# Minimal bash test framework for Gash.
# Usage: source this file, then write `describe` / `it` blocks.

set -u

__gash_test__total=0
__gash_test__failed=0
__gash_test__current_suite=""

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
  echo
  if [[ $__gash_test__failed -eq 0 ]]; then
    __gash_test__pass "All $__gash_test__total tests passed"
    return 0
  fi

  __gash_test__fail "$__gash_test__failed / $__gash_test__total tests failed"
  return 1
}
