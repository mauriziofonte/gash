#!/usr/bin/env bash
# Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# The generated Bash loads on a given Bash as the 2.x gash.sh does: a staged
# tree (the checkout with the generated gash.sh, shims and bash_completion,
# plus the binary) sourced headless under `set -euo pipefail` in a scrubbed
# environment prints nothing, returns 0, and defines the same public
# functions and aliases as the 2.x gash.sh of this checkout:
#
#   tests/ci/compat-load.sh BASH GASH_BIN
#
# BASH is the bash to load with (the CI version matrix), GASH_BIN the Gash 3
# binary (go build ./cmd/gash). Exit status 1, with the difference, when a
# check fails.

set -euo pipefail

bash_bin="${1:?usage: compat-load.sh BASH GASH_BIN}"
gash_bin="${2:?usage: compat-load.sh BASH GASH_BIN}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir "$work/home"

stage_out="$("$gash_bin" __compat stage --out "$work/stage" --root "$ROOT_DIR")"
staged_bin="${stage_out#GASH_BIN=}"

# load TREE NAME: sources TREE/gash.sh; NAME.out holds what sourcing
# printed, NAME.functions the public function names, NAME.aliases the
# aliases.
load() {
  env -i HOME="$work/home" PATH="/usr/bin:/bin" LANG=C.UTF-8 TERM=dumb \
    GASH_HEADLESS=1 GASH_BIN="$staged_bin" \
    "$bash_bin" --noprofile --norc -c '
      set -euo pipefail
      source "$1/gash.sh" >"$2.out" 2>&1
      declare -F | while read -r _ _ f; do
        [[ "$f" == _* ]] || printf "%s\n" "$f"
      done | LC_ALL=C sort >"$2.functions"
      alias | LC_ALL=C sort >"$2.aliases"
    ' _ "$1" "$work/$2"
}

status=0
"$bash_bin" --version | head -n 1
load "$ROOT_DIR" 2x || {
  echo "the 2.x gash.sh failed to load (status $?)"
  exit 1
}
if ! load "$work/stage" 3x; then
  echo "FAIL: the generated gash.sh returned non-zero under set -euo pipefail"
  sed 's/^/  /' "$work/3x.out"
  exit 1
fi
if [[ -s "$work/3x.out" ]]; then
  echo "FAIL: sourcing the generated gash.sh printed:"
  sed 's/^/  /' "$work/3x.out"
  status=1
fi
for kind in functions aliases; do
  if ! diff -u --label "2.x $kind" --label "generated $kind" "$work/2x.$kind" "$work/3x.$kind"; then
    echo "FAIL: the generated gash.sh defines other $kind than 2.x"
    status=1
  fi
done
((status)) || echo "OK: $(wc -l <"$work/3x.functions") functions and $(wc -l <"$work/3x.aliases") aliases, silent load"
exit "$status"
