#!/usr/bin/env bash
#
# llm_* argument handling: --help anywhere, unknown options, missing and
# extra arguments, integer caps; llm_find PATTERN --contains; compact
# llm_procs; the llm_env secret filter.

describe "LLM flags, llm_find --contains and llm_env"

it "--help and -h work anywhere on the line" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  for call in "llm_tree . --help" "llm_find *.php . -h" "llm_grep x . --help" "llm_config x.json --help" \
      "llm_git_status . -h" "llm_git_log -n 3 --help" "llm_project . --help" "llm_deps . -h" \
      "llm_db_query SELECT --help" "llm_env --filter X -h" "llm_procs --name x --help" "llm_ports --help"; do
    set -f
    out="$($call 2>&1)" || { echo "rc!=0: $call"; exit 1; }
    set +f
    fn="${call%% *}"
    [[ "$out" == *"$fn"* ]] || { echo "no help: $call: $out"; exit 1; }
  done
'

it "functions without options reject them; extra arguments are refused" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  printf "{}\n" >c.json
  verdict() { "$@" 2>&1 >/dev/null | jq -r .error; }
  [[ "$(verdict llm_git_status --bogus)" == unknown_option ]]
  [[ "$(verdict llm_git_status . other)" == unexpected_argument ]]
  [[ "$(verdict llm_project --bogus)" == unknown_option ]]
  [[ "$(verdict llm_project "$tmp/nonexistent")" == path_not_found ]]
  [[ "$(verdict llm_project "$tmp/c.json")" == not_a_directory ]]
  [[ "$(verdict llm_config --bogus)" == unknown_option ]]
  [[ "$(verdict llm_config c.json other.json)" == unexpected_argument ]]
  [[ "$(verdict llm_config c.json --max-bytes)" == missing_value ]]
  [[ "$(verdict llm_find)" == missing_pattern ]]
  [[ "$(verdict llm_find "*" . extra)" == unexpected_argument ]]
  for fn in llm_tree "llm_grep x" llm_git_diff llm_git_log llm_deps; do
    [[ "$(verdict $fn . other)" == unexpected_argument ]] || { echo "second path accepted: $fn"; exit 1; }
  done
  [[ "$(verdict llm_find "*" --type x)" == invalid_type ]]
  [[ "$(verdict llm_find "*" --type d --contains x)" == invalid_type ]]
  # Integers are capped at 9 digits (no arithmetic overflow).
  [[ "$(verdict llm_tree --depth 99999999999999999999)" == invalid_depth ]]
  [[ "$(verdict llm_find "*" -n 1234567890)" == invalid_limit ]]
  [[ "$(verdict llm_grep x -n 99999999999999999999)" == invalid_limit ]]
  llm_tree --depth 007 . | jq -e ".type == \"directory\"" >/dev/null
'

it "llm_find PATTERN --contains honours PATTERN, with rg and with grep alike" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  p="$tmp/p"
  mkdir -p "$p/src" "$p/.hidden" "$p/node_modules" "$p/__pycache__" "$p/vendor"
  for f in src/a.php c.md .hidden/x.php ignored.php node_modules/y.php __pycache__/z.php vendor/v.php secrets.php; do
    printf "needle\n" >"$p/$f"
  done
  printf "nothing\n" >"$p/src/b.php"
  printf "ignored.php\n" >"$p/.gitignore"
  expected="$(printf "%s\n" "$p/.hidden/x.php" "$p/ignored.php" "$p/src/a.php")"
  engines=(grep)
  command -v rg >/dev/null 2>&1 && engines+=(rg)
  for engine in "${engines[@]}"; do
    if [[ $engine == grep ]]; then __llm_has_rg() { return 1; }; else unset -f __llm_has_rg; source "$GASH_TEST_ROOT/lib/modules/llm/core.sh"; fi
    got="$(llm_find "*.php" "$p" --contains needle | sort)"
    [[ "$got" == "$expected" ]] || { echo "$engine: $got"; exit 1; }
    [[ -z "$(llm_find "*.nomatch" "$p" --contains needle)" ]] || { echo "$engine: nomatch"; exit 1; }
    [[ "$(llm_find "*.md" "$p" --contains needle)" == "$p/c.md" ]] || { echo "$engine: md"; exit 1; }
    [[ "$(llm_find "a.*" "$p/src/a.php" --contains needle)" == "$p/src/a.php" ]] || { echo "$engine: file"; exit 1; }
    [[ -z "$(llm_find "b.*" "$p/src/a.php" --contains needle)" ]] || { echo "$engine: file pattern"; exit 1; }
  done
  # The name search lists the same files.
  [[ "$(llm_find "*.php" "$p" | sort)" == "$(printf "%s\n" "$p/.hidden/x.php" "$p/ignored.php" "$p/secrets.php" "$p/src/a.php" "$p/src/b.php")" ]]
'

it "llm_procs prints one compact JSON array" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  out="$(llm_procs)"
  [[ "$(wc -l <<<"$out")" -eq 1 ]]
  jq -e "type == \"array\"" <<<"$out" >/dev/null
  name="no-such-$((RANDOM + 100000))-x"
  [[ "$(llm_procs --name "$name")" == "[]" ]]
  [[ "$(llm_procs --port 1x 2>&1 >/dev/null | jq -r .error)" == invalid_port ]]
'

it "llm_env hides secret names and masks token values" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  export MYSQL_PWD=a GH_PAT=b PASSPHRASE=c MY_JWT=d SLACK_WEBHOOK_URL=e SESSION_ID=f BEARER=g APP_SALT=h ENCRYPTION_PASS=i PRIVATE_THING=j
  export NOTES="key AKIAABCDEFGHIJKLMNOP and ghp_abcdefghijklmnopqrstuvwxyz0123 end"
  export API_BASE="https://u:pw@example.com/x" SAFE_VALUE=plain PATCH_LEVEL=3
  export PEMDATA="-----BEGIN OPENSSH PRIVATE KEY-----
abc"
  out="$(llm_env)"
  for n in MYSQL_PWD GH_PAT PASSPHRASE MY_JWT SLACK_WEBHOOK_URL SESSION_ID BEARER APP_SALT ENCRYPTION_PASS PRIVATE_THING; do
    jq -e --arg n "$n" "has(\$n) | not" <<<"$out" >/dev/null || { echo "shown: $n"; exit 1; }
  done
  jq -e ".NOTES == \"key *** and *** end\" and .API_BASE == \"https://u:***@example.com/x\" and .PEMDATA == \"***\"" <<<"$out" >/dev/null
  jq -e ".SAFE_VALUE == \"plain\" and .PATCH_LEVEL == \"3\" and .PWD == \"$PWD\"" <<<"$out" >/dev/null
'
