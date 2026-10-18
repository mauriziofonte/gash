#!/usr/bin/env bash
#
# llm_db_*: database names from -d and from .target-database never reach a
# client as an option or a connection string; MySQL TSV escapes decode right.

describe "LLM database names and values"

# Mock mysql/mariadb and psql: record argv and PGDATABASE, answer one row.
__dbn_mocks='
  mkdir -p "$tmp/bin"
  cat >"$tmp/bin/mysql" <<"MOCK"
#!/usr/bin/env bash
printf "%s\n" "$@" >"$MOCK_DIR/mysql.args"
cat >/dev/null
printf "id\n1\n"
MOCK
  cp "$tmp/bin/mysql" "$tmp/bin/mariadb"
  cat >"$tmp/bin/psql" <<"MOCK"
#!/usr/bin/env bash
printf "%s\n" "$@" >"$MOCK_DIR/psql.args"
printf "%s\n" "${PGDATABASE-unset}" >"$MOCK_DIR/psql.db"
cat >/dev/null
printf "[{\"id\": 1}]\n"
MOCK
  chmod +x "$tmp/bin/mysql" "$tmp/bin/mariadb" "$tmp/bin/psql"
  printf "%s\n" "DB:my=mysql://u:secret@127.0.0.1:3306/app" "DB:pg=pgsql://u:secret@127.0.0.1:5432/app" \
    "DB:myn=mysql://u:secret@127.0.0.1:3306" "DB:pgn=pgsql://u:secret@127.0.0.1:5432" >"$tmp/.gash_env"
  export PATH="$tmp/bin:$PATH" GASH_HEADLESS=1 HOME="$tmp" MOCK_DIR="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  # Pin the clock: on a slow runner the 100 ms slow-query rule (DB-06) runs the mock a second time (TEST-10).
  unset EPOCHREALTIME; EPOCHREALTIME="1792007131.734850"
'

it "-d must be a plain database name" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  '"$__dbn_mocks"'
  for conn in my pg; do
    for name in "--host=attacker.example" "-hattacker" "host=attacker.example options=-cx=y" \
        "postgresql://attacker.example/x" "a b" "x;y" "$(printf "%065d" 0)"; do
      err="$(llm_db_query "SELECT 1" -c "$conn" -d "$name" 2>&1 >/dev/null || true)"
      jq -e ".error == \"invalid_database_name\" and .action == \"RETRY\"" <<<"$err" >/dev/null ||
        { echo "accepted for $conn: $name: $err"; exit 1; }
    done
  done
  [[ ! -e "$tmp/mysql.args" && ! -e "$tmp/psql.args" ]]
'

it "MySQL gets --database=NAME, PostgreSQL gets PGDATABASE (never -d)" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  '"$__dbn_mocks"'
  llm_db_query "SELECT 1" -c my -d "other_db-2.x" >/dev/null
  grep -qx -- "--database=other_db-2.x" "$tmp/mysql.args"
  ! grep -qx -- "other_db-2.x" "$tmp/mysql.args"
  llm_db_query "SELECT 1" -c pg -d "other_db" >/dev/null
  [[ "$(cat "$tmp/psql.db")" == other_db ]]
  ! grep -qx -- "-d" "$tmp/psql.args"
  ! grep -q "other_db" "$tmp/psql.args"
  # The URL database is used when -d is absent.
  llm_db_tables -c pg >/dev/null
  [[ "$(cat "$tmp/psql.db")" == app ]]
'

it ".target-database is read only when it holds a plain name" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  '"$__dbn_mocks"'
  mkdir "$tmp/repo"
  cd "$tmp/repo"
  printf "  shop_db  # comment\n" >.target-database
  llm_db_query "SELECT 1" -c myn >/dev/null
  grep -qx -- "--database=shop_db" "$tmp/mysql.args"
  llm_db_query "SELECT 1" -c pgn >/dev/null
  [[ "$(cat "$tmp/psql.db")" == shop_db ]]
  rm -f "$tmp/mysql.args" "$tmp/psql.args"
  for bad in "postgresql://attacker.example/x" "--host=attacker.example" "host=evil"; do
    printf "%s\n" "$bad" >.target-database
    for conn in myn pgn; do
      err="$(llm_db_query "SELECT 1" -c "$conn" 2>&1 >/dev/null || true)"
      jq -e ".error == \"invalid_database_name\" and (.details | startswith(\".target-database\"))" <<<"$err" >/dev/null ||
        { echo "accepted for $conn: $bad: $err"; exit 1; }
    done
  done
  [[ ! -e "$tmp/mysql.args" && ! -e "$tmp/psql.args" ]]
  # -d still wins over a bad file.
  llm_db_query "SELECT 1" -c myn -d good >/dev/null
  grep -qx -- "--database=good" "$tmp/mysql.args"
'

it "MySQL TSV escapes decode left to right (an escaped backslash before 0)" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  # mysql --batch writes C:\0dir as C:\\0dir, a NUL as \0, a newline as \n.
  out="$(printf "%s\t%s\t%s\n" a b c "C:\\\\0dir" "x\\0y\\Zz" "l1\\nl2\"q\\\\" | __llm_tsv_to_json mysql)"
  jq -e ".[0].a == \"C:\\\\0dir\" and .[0].b == \"x\u0000y\u001az\" and .[0].c == \"l1\nl2\\\"q\\\\\"" <<<"$out" >/dev/null ||
    { echo "$out"; exit 1; }
  # A backslash before any other letter stays a literal backslash.
  out="$(printf "%s\n%s\n" a "\\q\\u0041" | __llm_tsv_to_json mysql)"
  [[ "$(jq -r ".[0].a" <<<"$out")" == "\\q\\u0041" ]] || { echo "$out"; exit 1; }
'
