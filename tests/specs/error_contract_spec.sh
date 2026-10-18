#!/usr/bin/env bash
#
# The machine error envelope (AGENTS.md, "Error contract") and argument
# parsing edge cases shared by llm_* and the filesystem scanners.

describe "Error contract"

it "__gash_json_error emits the documented shape and derives recoverable" bash -c '
  set -euo pipefail
  gash_require jq
  source "$GASH_TEST_ROOT/lib/core/output.sh"
  source "$GASH_TEST_ROOT/lib/core/utils.sh"

  out="$(__gash_json_error "bad_flag" "--x \"q\"" "RETRY" "use --help" 2>&1 || true)"
  jq -e ". == {error:\"bad_flag\",details:\"--x \\\"q\\\"\",action:\"RETRY\",recoverable:true,hint:\"use --help\"}" <<<"$out" >/dev/null

  out="$(__gash_json_error "blocked" "" "FATAL" 2>&1 || true)"
  jq -e ". == {error:\"blocked\",action:\"FATAL\",recoverable:false}" <<<"$out" >/dev/null

  # Unknown actions degrade to STOP rather than inventing a new verb.
  out="$(__gash_json_error "x" "" "MAYBE" 2>&1 || true)"
  [[ "$(jq -r .action <<<"$out")" == "STOP" ]]
'

it "llm_* and scanners agree: recoverable is false exactly for FATAL" bash -c '
  set -euo pipefail
  gash_require jq
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"

  for call in "llm_grep" "llm_db_query \"DROP TABLE x\" -f /nonexistent" "files_largest --json --bogus" "files_largest --json /" "llm_config $HOME/.env"; do
    err="$(eval "$call" 2>&1 >/dev/null || true)"
    action="$(jq -r .action <<<"$err")"
    recoverable="$(jq -r .recoverable <<<"$err")"
    if [[ "$action" == "FATAL" ]]; then want=false; else want=true; fi
    [[ "$recoverable" == "$want" ]] || { echo "$call -> $err"; exit 1; }
  done
'

it "a value flag at the end of the line errors instead of hanging" bash -c '
  set -euo pipefail
  gash_require jq
  gash_require timeout
  cd "$(mktemp -d)"

  for call in "files_largest --json --limit" "dirs_largest --json --depth" "dirs_find_large --json --size" \
      "dirs_list_empty --json --exclude" "tree_stats --json --top" "llm_find x --limit" "llm_grep x --ext" \
      "llm_db_query \"SELECT 1\" -c"; do
    rc=0
    err="$(GASH_HEADLESS=1 timeout 10 bash -c "source \"$GASH_TEST_ROOT/gash.sh\"; $call" 2>&1 >/dev/null)" || rc=$?
    [[ $rc -ne 124 ]] || { echo "hang: $call"; exit 1; }
    [[ $rc -ne 0 ]] || { echo "no error: $call"; exit 1; }
    [[ "$(jq -r .error <<<"$err")" == "missing_value" ]] || { echo "$call -> $err"; exit 1; }
  done
'

it "flags mean one thing: -n is a limit, --json is JSON, unknown flags fail" bash -c '
  set -uo pipefail
  gash_require jq
  tmp="$(mktemp -d)"; trap "rm -rf \"$tmp\"" EXIT
  for i in 1 2 3; do head -c $((i * 100)) /dev/zero >"$tmp/f$i"; done
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  [[ "$(files_largest -n 2 --json "$tmp" | jq .count)" == 2 ]] || exit 1
  [[ "$(dirs_largest -n 1 --json "$tmp" | jq .count)" -le 1 ]] || exit 1
  printf "8.3\n" >"$tmp/.target-php-version"
  php_detect_version "$tmp" --json | jq -e .version >/dev/null || exit 1
  php_detect_version "$tmp" --llm | jq -e .version >/dev/null || exit 1
  php_detect_version "$tmp" --bogus >/dev/null 2>&1 && exit 1
  printf "services:\n  web:\n    image: nginx\n" >"$tmp/compose.yml"
  docker_compose_upgrade "$tmp" -n >/dev/null 2>&1 && exit 1
  exit 0
'

it "the DB functions take -n as the row limit too" bash -c '
  set -euo pipefail
  gash_require jq
  gash_require sqlite3
  tmp="$(mktemp -d)"; trap "rm -rf \"$tmp\"" EXIT
  sqlite3 "$tmp/t.db" "CREATE TABLE t (id INTEGER); WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c WHERE x < 10) INSERT INTO t SELECT x FROM c;"
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  [[ "$(llm_db_query "SELECT id FROM t" -f "$tmp/t.db" -n 3 | jq .rows)" == 3 ]]
  [[ "$(llm_db_query "SELECT id FROM t" -f "$tmp/t.db" --limit 2 | jq .rows)" == 2 ]]
  [[ "$(llm_db_query "SELECT id FROM t" -f "$tmp/t.db" -r 4 | jq .rows)" == 4 ]]
  [[ "$(llm_db_query "SELECT id FROM t" -f "$tmp/t.db" --rows 5 | jq .rows)" == 5 ]]
  [[ "$(llm_db_sample t -f "$tmp/t.db" -n 2 | jq .rows)" == 2 ]]
  [[ "$(llm_db_sample t -f "$tmp/t.db" --limit 6 | jq .rows)" == 6 ]]
  [[ "$(llm_db_query "SELECT id FROM t" -f "$tmp/t.db" -n x 2>&1 >/dev/null | jq -r .error)" == invalid_rows ]]
  [[ "$(llm_db_sample t -f "$tmp/t.db" -n 2>&1 >/dev/null | jq -r .error)" == missing_value ]]
  [[ "$(llm_db_query -h)" == *"-n, --limit N"* ]]
  [[ "$(llm_db_sample -h)" == *"-n, --limit N"* ]]
'

it "llm_procs rejects unknown options with the unknown_option envelope" bash -c '
  set -euo pipefail
  gash_require jq
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  for args in "--bogus" "--pid 1" "nginx"; do
    rc=0
    out="$(llm_procs $args 2>"$HOME/err")" || rc=$?
    [[ $rc -eq 1 && -z "$out" ]] || { echo "$args: rc=$rc out=$out"; exit 1; }
    jq -e ".error == \"unknown_option\" and .action == \"RETRY\" and .recoverable == true" "$HOME/err" >/dev/null
  done
'

it "llm_git_status, llm_git_diff and llm_git_log agree outside a repository" bash -c '
  set -euo pipefail
  gash_require jq
  gash_require git
  tmp="$(mktemp -d)"; trap "rm -rf \"$tmp\"" EXIT
  export GASH_HEADLESS=1 HOME="$tmp" GIT_CEILING_DIRECTORIES="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  mkdir "$tmp/plain"; cd "$tmp/plain"
  for fn in llm_git_status llm_git_diff "llm_git_diff --staged" llm_git_log "llm_git_log --limit 3"; do
    rc=0
    out="$($fn 2>"$tmp/err")" || rc=$?
    [[ $rc -eq 1 && -z "$out" ]] || { echo "$fn: rc=$rc out=$out"; exit 1; }
    jq -e ".error == \"not_a_git_repo\" and .action == \"CONTINUE\" and .recoverable == true
      and .details == \"$tmp/plain is not a git repository\"" "$tmp/err" >/dev/null || { echo "$fn:"; cat "$tmp/err"; exit 1; }
  done
'

it "llm_docker_check errors follow the contract and unknown digests are null" bash -c '
  set -euo pipefail
  gash_require jq
  tmp="$(mktemp -d)"; trap "rm -rf \"$tmp\"" EXIT
  export GASH_HEADLESS=1 HOME="$tmp" PATH="$GASH_TEST_ROOT/tests/mocks/bin:$PATH"
  source "$GASH_TEST_ROOT/gash.sh"
  mkdir "$tmp/none" "$tmp/nosvc" "$tmp/app"
  check() { # DIR ERROR ACTION
    local rc=0 out
    out="$(llm_docker_check "$1" 2>"$tmp/err")" || rc=$?
    [[ $rc -eq 1 && -z "$out" ]] || { echo "$1: rc=$rc out=$out"; return 1; }
    jq -e ".error == \"$2\" and .action == \"$3\" and .recoverable == true and (.details | length > 0)
      and (keys - [\"error\",\"action\",\"recoverable\",\"details\",\"hint\"] == [])" "$tmp/err" >/dev/null ||
      { cat "$tmp/err"; return 1; }
  }
  check "$tmp/none" compose_not_found RETRY
  printf "services:\n  x:\n    build: .\n" >"$tmp/nosvc/compose.yml"
  check "$tmp/nosvc" no_services CONTINUE

  # An image that was never pulled has no local digest: null, not "null".
  printf "services:\n  app:\n    image: ghcr.io/nobody/none:dev\n" >"$tmp/app/compose.yml"
  out="$(llm_docker_check "$tmp/app")"
  jq -e ".services[0].local_digest == null and .services[0].status == \"NOT_PULLED\"
    and (.services[0].remote_digest | type == \"string\")" <<<"$out" >/dev/null
'
