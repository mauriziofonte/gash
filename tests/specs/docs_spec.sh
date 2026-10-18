#!/usr/bin/env bash
#
# The published documentation matches the code: the generated function
# reference is current, and every function and setting the hand-written
# pages cite exists.

describe "Documentation"

# The hand-written pages (the reference is generated, checked separately).
__docs_pages='
  pages=("$GASH_TEST_ROOT/README.md" "$GASH_TEST_ROOT/AGENTS.md" "$GASH_TEST_ROOT/CHANGELOG.md")
  for p in "$GASH_TEST_ROOT"/docs/*.md; do [[ -f "$p" ]] && pages+=("$p"); done
'

it "docs/reference is generated from the current help registry" bash -c '
  set -euo pipefail
  gash_require diff
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  bash "$GASH_TEST_ROOT/tools/gen-docs.sh" "$tmp" >/dev/null 2>&1
  diff -ru "$GASH_TEST_ROOT/docs/reference" "$tmp" >&2 || {
    echo "docs/reference is stale: run tools/gen-docs.sh" >&2
    exit 1
  }
'

it "every Gash function cited in the documentation exists" bash -c "$__docs_pages"'
  set -euo pipefail
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  # Names with a Gash prefix; wildcards (llm_*) and file names are not names.
  prefixes="llm|gash|remote|git|docker|wsl|dirs|files|tree|ai|process|sysinfo|port|disk|archive|services|history"
  missing=""
  while IFS= read -r name; do
    declare -F "$name" >/dev/null && continue
    alias "$name" >/dev/null 2>&1 && continue
    # An error code ("git_not_found") is a name too, emitted by the code.
    grep -rqF -- "\"$name\"" "$GASH_TEST_ROOT/lib" && continue
    missing+=" $name"
  done < <(
    grep -ohE "(^|[^[:alnum:]_./\"{-])($prefixes)_[a-z0-9_]*[a-z0-9]([^[:alnum:]_*./\"-]|\$)" "${pages[@]}" |
      grep -oE "($prefixes)_[a-z0-9_]*[a-z0-9]" | sort -u
  )
  [[ -z "$missing" ]] || { echo "not defined:$missing" >&2; exit 1; }
'

it "every GASH_ setting cited in the documentation is read by the code" bash -c "$__docs_pages"'
  set -euo pipefail
  cd "$GASH_TEST_ROOT"
  missing=""
  while IFS= read -r name; do
    grep -rqw -- "$name" gash.sh lib install.sh .gash_env.template bash_completion 2>/dev/null || missing+=" $name"
  done < <(grep -ohE "\bGASH_[A-Z0-9_]*[A-Z0-9]\b" "${pages[@]}" | sort -u)
  [[ -z "$missing" ]] || { echo "not in the code:$missing" >&2; exit 1; }
'

it "every page linked from the documentation exists" bash -c "$__docs_pages"'
  set -euo pipefail
  missing=""
  for p in "${pages[@]}" "$GASH_TEST_ROOT"/docs/reference/*.md; do
    dir="${p%/*}"
    while IFS= read -r target; do
      target="${target%%#*}"
      [[ -z "$target" ]] && continue
      [[ -e "$dir/$target" ]] || missing+=" ${p#"$GASH_TEST_ROOT"/}->$target"
    done < <(grep -oE "\]\([^)#:]+(#[^)]*)?\)" "$p" | sed -E "s/^\]\(//; s/\)\$//" | grep -v "^https\?:" || true)
  done
  [[ -z "$missing" ]] || { echo "broken links:$missing" >&2; exit 1; }
'
