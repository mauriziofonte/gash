#!/usr/bin/env bash

describe "Unload"

it "gash_unload restores prompt, options, and removes gash symbols" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$ROOT" "$tmp/.gash"

  inner_cmd="$(cat <<EOF
set -u

PS1=ORIG_PS1
PS2=ORIG_PS2
PROMPT_COMMAND=ORIG_PC

umask 077
shopt -u histappend
shopt -u checkwinsize

alias prealias=pre
prefunc() { :; }

source "\$HOME/.gash/gash.sh" >/dev/null 2>&1

# sanity: gash-defined symbols exist after load
type largest_files >/dev/null 2>&1
alias sl >/dev/null 2>&1

gash_unload >/dev/null 2>&1

echo "PS1:\${PS1-}"
echo "PS2:\${PS2-}"
echo "PROMPT_COMMAND:\${PROMPT_COMMAND-}"

shopt -q histappend; echo "histappend:\$?"
shopt -q checkwinsize; echo "checkwinsize:\$?"
echo "umask:\$(umask)"

# gash symbols removed
type largest_files >/dev/null 2>&1; echo "largest_files:\$?"
alias sl >/dev/null 2>&1; echo "sl:\$?"

# pre-existing symbols preserved
alias prealias >/dev/null 2>&1; echo "prealias:\$?"
type prefunc >/dev/null 2>&1; echo "prefunc:\$?"

# unload removes itself
type gash_unload >/dev/null 2>&1; echo "gash_unload:\$?"
EOF
)"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

  [[ "$out" == *"PS1:ORIG_PS1"* ]]
  [[ "$out" == *"PS2:ORIG_PS2"* ]]
  [[ "$out" == *"PROMPT_COMMAND:ORIG_PC"* ]]

  # restored shopt/umask
  [[ "$out" == *"histappend:1"* ]]
  [[ "$out" == *"checkwinsize:1"* ]]
  [[ "$out" == *"umask:0077"* ]]

  # removed gash symbols
  [[ "$out" == *"largest_files:1"* ]]
  [[ "$out" == *"sl:1"* ]]

  # preserved pre-existing
  [[ "$out" == *"prealias:0"* ]]
  [[ "$out" == *"prefunc:0"* ]]

  # removed itself
  [[ "$out" == *"gash_unload:1"* ]]
'
