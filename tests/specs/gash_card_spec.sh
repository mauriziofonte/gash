#!/usr/bin/env bash
#
# The gash reference card: generated from the help registry and the alias
# files, boxed, aligned, and plain when piped.

describe "Gash reference card"

it "gash all lists every registered function" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1 LC_ALL=C COLUMNS=200
  source "$GASH_TEST_ROOT/gash.sh"
  card="$(gash all --no-color)"
  missing=""
  for fn in "${!__GASH_HELP_REGISTRY[@]}"; do
    grep -qE "^  $fn( |\$)" <<<"$card" || missing+=" $fn"
  done
  [[ -z "$missing" ]] || { echo "missing:$missing" >&2; exit 1; }
'

it "the overview counts add up to the registry" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1 LC_ALL=C COLUMNS=200
  source "$GASH_TEST_ROOT/gash.sh"
  sum=$(gash --no-color | awk "/^  [a-z]+ +[0-9]+ +[0-9]+ /{s+=\$2} END{print s}")
  [[ "$sum" == "${#__GASH_HELP_REGISTRY[@]}" ]]
'

it "gash topics show aliases with their expansion, from the alias files" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1 LC_ALL=C COLUMNS=200
  source "$GASH_TEST_ROOT/gash.sh"
  out="$(gash git)"
  grep -qE "^  +git_add_tag +[^ ]" <<<"$out"
  grep -q "Aliases" <<<"$out"
  grep -qE "(^|  )gl +Compact log with graph" <<<"$out"
  # Internal variables never leak into the card.
  [[ "$(gash all)" != *"__gash_"* ]]
  # Identity aliases (ls -> ls) are left out.
  ! grep -qE "(^|  )ls +ls( |\$)" <<<"$(gash nav)"
'

it "gash accepts the old section names and refuses unknown topics" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1 LC_ALL=C COLUMNS=120
  source "$GASH_TEST_ROOT/gash.sh"
  [[ "$(gash navigation)" == *"NAV"* ]]
  [[ "$(gash management)" == *"GASH"* ]]
  [[ "$(gash php)" == *"php_detect_version"* ]]
  ! gash bogus >/dev/null 2>&1
  [[ "$(gash bogus 2>&1)" == *"Topics: files system nav"* ]]
  [[ "$(gash --help)" == *"TOPICS"* ]]
'

it "gash writes no escape bytes when piped, and the box lines line up" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1 LC_ALL=C COLUMNS=100
  source "$GASH_TEST_ROOT/gash.sh"
  [[ "$(gash all)" != *$'"'"'\033'"'"'* ]]
  # Box: every line has the same width.
  widths=$(gash | sed -n "2,5p" | awk "{print length(\$0)}" | sort -u | wc -l)
  [[ "$widths" -eq 1 ]]
  # Table rows fit the terminal.
  [[ $(gash all | awk "length(\$0) > 100" | wc -l) -eq 0 ]]
'

it "__gash_box keeps its border aligned with colored and over-wide lines" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1 LC_ALL=C COLUMNS=40
  source "$GASH_TEST_ROOT/gash.sh"
  __gash_use_color() { return 0; }
  out="$(__gash_box --color "$__GASH_CYAN" "${__GASH_BOLD}bold${__GASH_COLOR_OFF} text" "plain" "$(printf "%060d" 0)")"
  plain="$out" re=$'"'"'\e'"'"'"\\[[0-9;]*m"
  while [[ "$plain" =~ $re ]]; do plain="${plain//"${BASH_REMATCH[0]}"/}"; done
  [[ $(awk "{print length(\$0)}" <<<"$plain" | sort -u | wc -l) -eq 1 ]]
  [[ $(head -n 1 <<<"$plain" | awk "{print length(\$0)}") -le 40 ]]
  grep -q "~  |" <<<"$plain"
'

it "__gash_table --fit truncates the last column and --color skips the header" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1 LC_ALL=C COLUMNS=30
  source "$GASH_TEST_ROOT/gash.sh"
  out="$(printf "A\tB\nx\t%s\n" "$(printf "%050d" 0)" | __gash_table --fit)"
  [[ $(awk "length(\$0) > 30" <<<"$out" | wc -l) -eq 0 ]]
  [[ "$(tail -n 1 <<<"$out")" == *"~" ]]
  __gash_use_color() { return 0; }
  out="$(printf "A\tB\nx\ty\n" | __gash_table --color 1:"$__GASH_CYAN")"
  [[ "$(tail -n 1 <<<"$out")" == "${__GASH_CYAN}x${__GASH_COLOR_OFF}  y" ]]
'
