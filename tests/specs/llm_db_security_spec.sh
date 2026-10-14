#!/usr/bin/env bash
#
# llm_db_* read-only guarantees: the SQL guard, sqlite3 -readonly -safe, and
# the read-only session settings handed to the mysql and psql clients.

describe "LLM database security"

# Guard verdicts that need no database at all.
it "the SQL guard refuses client commands, comment tricks and second statements" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  verdict() { local q k; __llm_sql_guard "$1" "$2" q k 2>&1 >/dev/null | jq -r .error; }

  # mysql client: \! runs a shell command; a backslash in a backtick
  # identifier desyncs the client from the server.
  [[ "$(verdict mysql "SELECT 1 \\! id")" == client_command_blocked ]]
  [[ "$(verdict mysql "SELECT \`a\\\` , '"'"'x\` \\! id #'"'"'")" == backslash_in_literal ]]
  [[ "$(verdict mysql "/*!50000 DROP TABLE t */ SELECT 1")" == executable_comment_blocked ]]
  [[ "$(verdict mysql "/*M!100101 DROP TABLE t */ SELECT 1")" == executable_comment_blocked ]]
  [[ "$(verdict mysql "SELECT 1; SELECT 2")" == multiple_statements_blocked ]]
  [[ "$(verdict mysql "SELECT * FROM t INTO OUTFILE '"'"'/tmp/x'"'"'")" == write_operation_blocked ]]
  [[ "$(verdict mysql "SELECT * FROM t FOR UPDATE")" == write_operation_blocked ]]

  # PostgreSQL dollar quotes hide nothing from the guard.
  [[ "$(verdict pgsql "SELECT \$\$'"'"'\$\$, pg_read_file('"'"'/etc/passwd'"'"') -- '"'"'")" == write_operation_blocked ]]
  [[ "$(verdict pgsql "SELECT pg_terminate_backend(1)")" == write_operation_blocked ]]
  [[ "$(verdict pgsql "SELECT \$t\$ unterminated")" == unterminated_literal ]]

  # sqlite3 shell: dot-commands and line terminators.
  [[ "$(verdict sqlite $'"'"'SELECT 1\n.shell id'"'"')" == dot_command_blocked ]]
  [[ "$(verdict sqlite $'"'"'SELECT 1\ngo\nSELECT 2'"'"')" == multiple_statements_blocked ]]
  [[ "$(verdict sqlite "ATTACH '"'"'/tmp/x.db'"'"' AS x")" == write_operation_blocked ]]
'

it "the SQL guard accepts ordinary read queries" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  ok() { local q k; __llm_sql_guard "$1" "$2" q k; }
  ok mysql "SELECT '"'"'it'"''"'s'"'"' -- a comment"
  ok mysql "SELECT 1;"
  ok pgsql "SELECT E'"'"'a\\'"'"'b'"'"', '"'"'C:\\'"'"'"
  ok pgsql "SELECT \$\$ DELETE ; \$\$ AS literal"
  ok sqlite "SELECT replace(name, '"'"'a'"'"', '"'"'b'"'"') FROM [odd'"'"'name]"
  ok sqlite "WITH a AS (SELECT 1) SELECT * FROM a"
  # A trailing semicolon is dropped from the stored query.
  q="" k=""
  __llm_sql_guard sqlite "SELECT 1 ;  " q k
  [[ "$q" == "SELECT 1 " && "$k" == SELECT ]]
'

it "llm_db_query on SQLite caps rows, keeps comments harmless and returns empty envelopes" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  command -v sqlite3 >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  sqlite3 "$tmp/t.db" "CREATE TABLE u(id INTEGER, name TEXT); INSERT INTO u VALUES (1,'"'"'a'"'"'),(2,'"'"'b'"'"'),(3,'"'"'c'"'"');"
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"

  out="$(llm_db_query "SELECT * FROM u -- trailing comment" -f "$tmp/t.db")"
  jq -e ".rows == 3" <<<"$out" >/dev/null
  out="$(llm_db_query "SELECT * FROM u;" -f "$tmp/t.db" -r 2)"
  jq -e ".rows == 2 and .data[1].name == \"b\"" <<<"$out" >/dev/null
  out="$(llm_db_query "SELECT * FROM u WHERE id > 9" -f "$tmp/t.db")"
  jq -e ".data == [] and .rows == 0" <<<"$out" >/dev/null
  out="$(llm_db_explain "SELECT * FROM u WHERE id = 1" -f "$tmp/t.db")"
  jq -e ".rows == 1 and (.data[0].plan | test(\"u\"))" <<<"$out" >/dev/null
'

it "llm_db_query on SQLite never writes, even past the guard" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  command -v sqlite3 >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  sqlite3 "$tmp/t.db" "CREATE TABLE u(id INTEGER);"
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  # Straight to the runner, skipping the guard: -readonly still refuses.
  ! __llm_sqlite_run "$tmp/t.db" "INSERT INTO u VALUES (1)" 2>/dev/null
  [[ "$(sqlite3 "$tmp/t.db" "SELECT count(*) FROM u")" == 0 ]]
  # .shell is refused by the guard and, if -safe exists, by sqlite3 too.
  ! llm_db_query $'"'"'SELECT 1\n.shell touch '"'"'"$tmp/pwned" -f "$tmp/t.db" 2>/dev/null
  [[ ! -e "$tmp/pwned" ]]
'

it "llm_db_query sends MySQL a read-only session with a row cap, SQL on stdin" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/bin"
  cat >"$tmp/bin/mysql" <<"MOCK"
#!/usr/bin/env bash
printf "%s\n" "$@" >"$MOCK_DIR/args"
# Read the credentials file like the real client does (it is a pipe).
for a in "$@"; do
  case "$a" in --defaults-extra-file=*) cat "${a#*=}" >"$MOCK_DIR/ini" ;; esac
done
cat >"$MOCK_DIR/stdin"
printf "id\tname\n1\ta\n"
MOCK
  chmod +x "$tmp/bin/mysql"
  # mariadb is preferred when installed (macOS runners have it): mock both.
  cp "$tmp/bin/mysql" "$tmp/bin/mariadb"
  printf "DB:default=mysql://u:p%%40ss@127.0.0.1:3306/app\n" >"$tmp/.gash_env"
  export PATH="$tmp/bin:$PATH" GASH_HEADLESS=1 HOME="$tmp" MOCK_DIR="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"

  # On failure, show what the client saw.
  trap '"'"'echo "out: ${out-}"; for f in args ini stdin; do echo "--- $f"; cat "$tmp/$f" 2>&1; done'"'"' ERR
  out="$(llm_db_query "SELECT id, name FROM t -- note" -r 7)"
  jq -e ".rows == 1 and .data[0].name == \"a\"" <<<"$out" >/dev/null
  grep -q "p@ss" "$tmp/ini"
  grep -qx "SET SESSION TRANSACTION READ ONLY;" "$tmp/stdin"
  grep -qx "SET SESSION sql_select_limit = 7;" "$tmp/stdin"
  grep -qx "SELECT id, name FROM t -- note" "$tmp/stdin"
  [[ "$(tail -n 1 "$tmp/stdin")" == ";" ]]
  # Neither the SQL nor the password ever reach argv.
  ! grep -q "SELECT" "$tmp/args"
  ! grep -q "p@ss" "$tmp/args"
'

it "llm_db_query runs PostgreSQL read-only with a timeout and standard strings" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/bin"
  cat >"$tmp/bin/psql" <<"MOCK"
#!/usr/bin/env bash
printf "%s\n" "$@" >"$MOCK_DIR/args"
printf "%s\n" "$PGOPTIONS" >"$MOCK_DIR/pgoptions"
printf "[{\"id\": 1, \"ok\": true}]\n"
MOCK
  chmod +x "$tmp/bin/psql"
  printf "DB:default=pgsql://u:secret@127.0.0.1:5432/app\n" >"$tmp/.gash_env"
  export PATH="$tmp/bin:$PATH" GASH_HEADLESS=1 HOME="$tmp" MOCK_DIR="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"

  out="$(llm_db_query "SELECT 1 AS id, true AS ok" -r 3)"
  jq -e ".data[0].ok == true and .rows == 1" <<<"$out" >/dev/null
  grep -q "default_transaction_read_only=on" "$tmp/pgoptions"
  grep -q "standard_conforming_strings=on" "$tmp/pgoptions"
  grep -q "statement_timeout=30s" "$tmp/pgoptions"
  grep -q "LIMIT 3" "$tmp/args"
  grep -qx "ON_ERROR_STOP=1" "$tmp/args"
  ! grep -q "secret" "$tmp/args"
'

it "llm_db_* reject unknown options and stray arguments" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  [[ "$(llm_db_query "SELECT 1" --bogus 2>&1 >/dev/null | jq -r .error)" == unknown_option ]]
  [[ "$(llm_db_query SELECT 1 2>&1 >/dev/null | jq -r .error)" == unexpected_argument ]]
  [[ "$(llm_db_tables -c 2>&1 >/dev/null | jq -r .error)" == missing_value ]]
'
