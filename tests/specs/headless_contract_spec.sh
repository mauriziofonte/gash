#!/usr/bin/env bash

# Headless contract: every public function must survive `set -euo pipefail`
# (the environment of LLM wrappers and CI scripts), return 0 on legitimately
# empty results, emit parseable JSON where JSON is promised, and never leak
# diagnostics into values captured via $(...).
#
# This is the regression gate for the errexit/JSON/locale/stderr fix waves:
# fixtures include hostile filenames (space, tab, newline, quotes, dash,
# glob, unicode) and an unreadable directory.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Headless contract (set -euo pipefail)"

# Common preamble for every test: source core+modules, THEN enable strict
# mode (mirrors gash-exec.sh wrappers).
__hc_preamble='
  ROOT="${GASH_TEST_ROOT}"
  export GASH_HEADLESS=1
  source "$ROOT/tests/gash-test.sh"
  gash_source_all "$ROOT" >/dev/null 2>&1
  set -euo pipefail
'

# Builds a sandbox with hostile filenames + unreadable dir in $sandbox.
__hc_sandbox='
  sandbox="$(mktemp -d)"
  trap "chmod -R u+rwx \"$sandbox\" 2>/dev/null; rm -rf \"$sandbox\"" EXIT
  mkdir -p "$sandbox/sub" "$sandbox/noperm" "$sandbox/empty dir"
  printf x > "$sandbox/normal.txt"
  printf x > "$sandbox/with space.txt"
  printf x > "$sandbox/-leading-dash.txt"
  printf x > "$sandbox/g*l(o)b?.txt"
  printf x > "$sandbox/quote\"double.txt"
  printf x > "$sandbox/quote'"'"'single.txt"
  printf x > "$sandbox/più_unicode_è.txt"
  printf x > "$(printf "%s/tab\there.txt" "$sandbox")"
  printf x > "$(printf "%s/new\nline.txt" "$sandbox")"
  chmod 000 "$sandbox/noperm"
'

# -----------------------------------------------------------------------------
# llm_* search functions: empty results are rc=0, never a crash
# -----------------------------------------------------------------------------

it "llm_grep with zero matches returns 0 under pipefail" bash -c "
  $__hc_preamble
  llm_grep 'PATTERN_THAT_DOES_NOT_EXIST_XYZ' . >/dev/null
  echo alive >/dev/null
"

it "llm_find with zero matches returns 0 under pipefail" bash -c "
  $__hc_preamble
  llm_find '*.ext_that_does_not_exist_xyz' >/dev/null
"

it "llm_grep/llm_find reject non-integer --limit (C8 family)" bash -c "
  $__hc_preamble
  set +e
  llm_grep 'x' --limit 'evil; payload' >/dev/null 2>&1; rc1=\$?
  llm_find '*.x' --limit '5; DROP' >/dev/null 2>&1; rc2=\$?
  set -e
  [[ \$rc1 -ne 0 && \$rc2 -ne 0 ]]
"

# -----------------------------------------------------------------------------
# llm_* JSON emitters: output must parse with jq, even with hostile inputs
# -----------------------------------------------------------------------------

it "llm_tree emits valid JSON on hostile sandbox" bash -c "
  $__hc_preamble
  $__hc_sandbox
  llm_tree --depth 3 \"\$sandbox\" | jq -e . >/dev/null
"

it "llm_project emits valid JSON" bash -c "
  $__hc_preamble
  $__hc_sandbox
  llm_project \"\$sandbox\" | jq -e . >/dev/null
"

it "llm_env emits valid JSON (multi-line env value)" bash -c "
  $__hc_preamble
  export HC_MULTILINE=\$'line1\nline2\"quoted'
  llm_env | jq -e . >/dev/null
"

it "llm_procs emits valid JSON under it-style locale input" bash -c "
  $__hc_preamble
  llm_procs | jq -e . >/dev/null
"

it "llm_ports emits valid JSON" bash -c "
  $__hc_preamble
  PATH=\"\$ROOT/tests/mocks/bin:\$PATH\" llm_ports | jq -e . >/dev/null
"

it "llm_git_status emits valid JSON with hostile filenames (C5)" bash -c "
  $__hc_preamble
  repo=\"\$(mktemp -d)\"
  trap 'rm -rf \"\$repo\"' EXIT
  git -C \"\$repo\" init -q
  printf x > \"\$repo/evil\\\"quote.txt\"
  printf x > \"\$repo/back\\\\slash.txt\"
  out=\$(llm_git_status \"\$repo\")
  printf '%s' \"\$out\" | jq -e . >/dev/null
  # empty staged array must be [], never [\"\"]
  [[ \$(printf '%s' \"\$out\" | jq -r '.staged | length') == 0 ]]
"

it "llm_git_log emits valid JSON with quotes in subject (C6)" bash -c "
  $__hc_preamble
  repo=\"\$(mktemp -d)\"
  trap 'rm -rf \"\$repo\"' EXIT
  git -C \"\$repo\" init -q
  git -C \"\$repo\" -c user.email=t@t -c user.name=t commit -q --allow-empty -m 'subject with \"quotes\" and \\\\backslash'
  llm_git_log --limit 5 \"\$repo\" | jq -e . >/dev/null
"

# -----------------------------------------------------------------------------
# SQL guards (C8)
# -----------------------------------------------------------------------------

it "llm_db_query -r injection payload is rejected before the engine" bash -c "
  $__hc_preamble
  gash_require sqlite3
  db=\"\$(mktemp -u).db\"
  trap 'rm -f \"\$db\"' EXIT
  sqlite3 \"\$db\" 'CREATE TABLE t(x); INSERT INTO t VALUES (1)'
  set +e
  llm_db_query 'SELECT * FROM t' -f \"\$db\" -r '5; DROP TABLE t' >/dev/null 2>&1
  rc=\$?
  set -e
  [[ \$rc -ne 0 ]]
  [[ \$(sqlite3 \"\$db\" 'SELECT count(*) FROM t') == 1 ]]
"

it "llm_db_sample rejects non-integer --limit" bash -c "
  $__hc_preamble
  gash_require sqlite3
  db=\"\$(mktemp -u).db\"
  trap 'rm -f \"\$db\"' EXIT
  sqlite3 \"\$db\" 'CREATE TABLE t(x)'
  set +e
  llm_db_sample t -f \"\$db\" --limit '1; ATTACH' >/dev/null 2>&1
  rc=\$?
  set -e
  [[ \$rc -ne 0 ]]
"

# -----------------------------------------------------------------------------
# fs scanners: hostile sandbox + unreadable dirs + SIGPIPE-prone limits
# -----------------------------------------------------------------------------

it "files_largest survives unreadable dir and tiny --limit (C4)" bash -c "
  $__hc_preamble
  $__hc_sandbox
  files_largest \"\$sandbox\" --limit 2 >/dev/null 2>&1
  files_largest \"\$sandbox\" --json | jq -e . >/dev/null
"

it "dirs_largest emits valid JSON on hostile sandbox" bash -c "
  $__hc_preamble
  $__hc_sandbox
  dirs_largest \"\$sandbox\" --json | jq -e . >/dev/null
"

it "dirs_find_large emits valid JSON and excludes the root (M15)" bash -c "
  $__hc_preamble
  $__hc_sandbox
  out=\$(dirs_find_large \"\$sandbox\" --size 1 --json)
  printf '%s' \"\$out\" | jq -e . >/dev/null
  root=\$(cd \"\$sandbox\" && pwd -P)
  [[ \$(printf '%s' \"\$out\" | jq -r --arg r \"\$root\" '[.data[] | select(.path == \$r)] | length') == 0 ]]
"

it "dirs_list_empty and tree_stats emit valid JSON on hostile sandbox" bash -c "
  $__hc_preamble
  $__hc_sandbox
  dirs_list_empty \"\$sandbox\" --json | jq -e . >/dev/null
  tree_stats \"\$sandbox\" --json | jq -e . >/dev/null
"

it "files_largest --null emits intact NUL records (A10)" bash -c "
  $__hc_preamble
  $__hc_sandbox
  out_file=\"\$(mktemp)\"
  trap 'rm -f \"\$out_file\"' EXIT
  files_largest \"\$sandbox\" --null > \"\$out_file\"
  # every NUL-delimited record must be an existing file path
  while IFS= read -r -d '' p; do
    [[ -f \"\$p\" ]] || exit 1
  done < \"\$out_file\"
"

it "files_largest honors PATH after -- separator (M17)" bash -c "
  $__hc_preamble
  $__hc_sandbox
  files_largest -- \"\$sandbox\" 2>/dev/null | grep -q 'normal.txt'
"

# -----------------------------------------------------------------------------
# sysinfo collectors (C3) and misc errexit traps
# -----------------------------------------------------------------------------

it "sysinfo identity/storage/all --llm survive pipefail (C3)" bash -c "
  $__hc_preamble
  sysinfo identity --llm >/dev/null
  sysinfo storage --llm >/dev/null
  sysinfo all --llm >/dev/null
"

it "history_grep with zero matches returns 0 (M4)" bash -c "
  $__hc_preamble
  history_grep 'PATTERN_THAT_DOES_NOT_EXIST_XYZ' >/dev/null
"

it "__gash_url_decode survives errexit and keeps newlines (A6/M9)" bash -c "
  $__hc_preamble
  out=\$(__gash_url_decode 'a%0Ab%20c')
  [[ \"\$out\" == \$'a\nb c' ]]
"

it "__gash_url_encode is UTF-8 byte-wise (M6)" bash -c "
  $__hc_preamble
  [[ \$(__gash_url_encode 'è') == '%C3%A8' ]]
  [[ \$(__gash_url_encode '€') == '%E2%82%AC' ]]
"

# -----------------------------------------------------------------------------
# Stream policy (C12): captured values must never contain diagnostics
# -----------------------------------------------------------------------------

it "config warnings never contaminate captured DB URLs (C12)" bash -c "
  $__hc_preamble
  envf=\"\$(mktemp)\"
  trap 'rm -f \"\$envf\"' EXIT
  printf 'garbage line that triggers a warning\nDB:default=mysql://u:p@h:3306/db\n' > \"\$envf\"
  chmod 644 \"\$envf\"  # wrong perms -> warning at load time
  export GASH_ENV_FILE=\"\$envf\"
  __gash_reload_env 2>/dev/null || true
  url=\$(__gash_get_db_url default 2>/dev/null)
  [[ \"\$url\" == 'mysql://u:p@h:3306/db' ]]
"

it "needs_confirm_prompt refuses in headless mode, no ghost confirm (C13)" bash -c "
  $__hc_preamble
  export REPLY=y
  set +e
  needs_confirm_prompt 'Destroy everything?' </dev/null >/dev/null 2>&1
  rc=\$?
  set -e
  [[ \$rc -ne 0 ]]
"
