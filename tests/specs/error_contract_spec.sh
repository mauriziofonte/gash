#!/usr/bin/env bash
#
# The machine error envelope (AGENTS.md, "Error contract") and argument
# parsing edge cases shared by llm_* and the filesystem scanners.

describe "Error contract"

it "__gash_json_error emits the documented shape and derives recoverable" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
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
  command -v jq >/dev/null 2>&1 || exit 77
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
  command -v jq >/dev/null 2>&1 || exit 77
  command -v timeout >/dev/null 2>&1 || exit 77
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
  command -v jq >/dev/null 2>&1 || exit 77
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
