#!/usr/bin/env bash
#
# The JSON envelopes AGENTS.md documents as stable API, checked key by key
# and type by type (valid JSON alone is covered by headless_contract_spec).

describe "Envelope schemas"

# Shared fixture: t/{a/{x.txt, b/big.bin}, empty/} and a sqlite database.
__envelope_fixture='
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf \"$tmp\"" EXIT
  mkdir -p "$tmp/t/a/b" "$tmp/t/empty"
  head -c 70000 /dev/zero >"$tmp/t/a/b/big.bin"
  printf "hi\n" >"$tmp/t/a/x.txt"
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  # schema NAME JQ_EXPR: the expression must be true for stdin.
  schema() { local in; in="$(cat)"; jq -e "$2" <<<"$in" >/dev/null || { echo "schema $1 failed on: $in" >&2; return 1; }; }
'

it "size-list envelope: files_largest, dirs_largest, dirs_find_large" bash -c "$__envelope_fixture"'
  set -euo pipefail
  common="(keys == [\"count\",\"data\",\"errors\",\"kind\",\"scan_time\",\"total_size\",\"total_size_human\"])
    and (.data | type == \"array\") and (.count == (.data | length))
    and (.total_size == ([.data[].size] | add // 0))
    and (.total_size_human | type == \"string\")
    and (.scan_time | test(\"^[0-9]+ms$\")) and (.errors | type == \"number\")
    and all(.data[]; (.size | type == \"number\") and (.size_human | type == \"string\")
                     and (.path | startswith(\"/\")))"

  files_largest --json "$tmp/t" | schema files "$common and .kind == \"files\" and .count == 2
    and all(.data[]; (.mtime | test(\"^[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}$\")))
    and .data[0].size == 70000"
  dirs_largest --json "$tmp/t" | schema dirs_largest "$common and .kind == \"directories\"
    and all(.data[]; has(\"mtime\") | not)"
  dirs_find_large --size 1 --json "$tmp/t" | schema dirs_find_large "$common and .kind == \"directories\""
'

it "empty-dirs and tree-stats envelopes" bash -c "$__envelope_fixture"'
  set -euo pipefail
  root="$(cd "$tmp/t" && pwd -P)"
  dirs_list_empty --json "$tmp/t" | schema empty "keys == [\"count\",\"data\",\"errors\",\"path\"]
    and .data == [\"$root/empty\"] and .count == 1 and .path == \"$root\" and .errors == 0"

  tree_stats --json "$tmp/t" | schema tree_stats "(keys == [\"data\",\"errors\",\"scan_time\"])
    and (.data | keys == [\"avg_file_size\",\"dirs_total\",\"empty_dirs\",\"files_total\",\"max_depth\",\"path\",
                          \"size_total\",\"size_total_human\",\"top_by_count\",\"top_by_size\"])
    and .data.files_total == 2 and .data.size_total == 70003 and .data.empty_dirs == 1
    and all(.data.top_by_count[]; keys == [\"count\",\"ext\",\"size\"])
    and all(.data.top_by_size[]; keys == [\"ext\",\"size\"])
    and (.scan_time | test(\"^[0-9]+ms$\"))"
'

it "llm_tree --stats: cumulative sizes and real child counts past the depth limit" bash -c "$__envelope_fixture"'
  set -euo pipefail
  llm_tree --depth 2 --stats "$tmp/t" | schema llm_tree "
    (keys == [\"children\",\"children_count\",\"path\",\"size\",\"type\"])
    and .type == \"directory\" and .size == 70003 and .children_count == 2
    and ((.children[] | select(.name == \"a\")) as \$a
         | \$a.size == 70003 and \$a.children_count == 2
         and (\$a.children[] | select(.name == \"b\") | .size == 70000 and .children_count == 1 and (has(\"children\") | not)))"
  # Without --stats: names and types only, default depth 1.
  llm_tree "$tmp/t" | schema llm_tree_plain "(keys == [\"children\",\"path\",\"type\"])
    and all(.children[]; keys == [\"name\",\"type\"])"
'

it "llm_tree flags a listing cut at 500 entries" bash -c "$__envelope_fixture"'
  set -euo pipefail
  mkdir "$tmp/many"
  (cd "$tmp/many" && for i in $(seq 1 510); do : >"f$i"; done)
  llm_tree --stats "$tmp/many" | schema capped ".truncated == true and (.children | length) == 500 and .children_count == 510"
  llm_tree "$tmp/t" | schema uncapped "has(\"truncated\") | not"
'

it "database envelope: every llm_db_* returns {data, rows, query_time}" bash -c "$__envelope_fixture"'
  set -euo pipefail
  command -v sqlite3 >/dev/null 2>&1 || exit 77
  db="$tmp/app.db"
  sqlite3 "$db" "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT); INSERT INTO users(name) VALUES (\"alice\"), (\"bob\");"
  env="(keys == [\"data\",\"query_time\",\"rows\"]) and (.data | type == \"array\")
    and .rows == (.data | length) and (.query_time | test(\"^[0-9]+(\\\\.[0-9]+)?(µs|ms|s)$\") and . != \"0µs\")"

  llm_db_tables -f "$db" | schema tables "$env and .data == [\"users\"]"
  llm_db_schema users -f "$db" | schema schema "$env and ([.data[].name] == [\"id\",\"name\"])"
  llm_db_sample users -f "$db" --limit 1 | schema sample "$env and .data == [{id: 1, name: \"alice\"}]"
  llm_db_query "SELECT id, name FROM users ORDER BY id" -f "$db" | schema query "$env and .rows == 2 and .data[1].name == \"bob\""
  llm_db_explain "SELECT * FROM users" -f "$db" | schema explain "$env and .rows >= 1"
'

it "git envelopes: llm_git_status keys and llm_git_log records" bash -c "$__envelope_fixture"'
  set -euo pipefail
  command -v git >/dev/null 2>&1 || exit 77
  repo="$tmp/repo"; mkdir "$repo"; cd "$repo"
  git init -q -b main . && git config user.email t@t && git config user.name T
  echo a >a && git add a && git commit -qm "first \"quoted\""
  echo b >>a && echo n >new

  llm_git_status "$repo" | schema status "keys == [\"ahead\",\"behind\",\"branch\",\"conflicted\",\"modified\",\"staged\",\"untracked\"]
    and .branch == \"main\" and .ahead == 0 and .behind == 0
    and .modified == [\"a\"] and .untracked == [\"new\"] and .staged == [] and .conflicted == []"
  llm_git_log --limit 5 "$repo" | schema log "length == 1
    and (.[0] | keys == [\"author\",\"date\",\"hash\",\"subject\"]) and .[0].subject == \"first \\\"quoted\\\"\""
'

it "system and project envelopes: llm_project, llm_procs, llm_ports" bash -c "$__envelope_fixture"'
  set -euo pipefail
  printf "{\"name\":\"x\",\"dependencies\":{\"left-pad\":\"1\"},\"devDependencies\":{\"jest\":\"1\"}}" >"$tmp/package.json"
  llm_project "$tmp" | schema project "keys == [\"entry_point\",\"framework\",\"has_tests\",\"package_manager\",\"path\",\"type\"]
    and .type == \"javascript\" and .package_manager == \"npm\" and (.has_tests | type == \"boolean\")"
  llm_deps "$tmp" | schema deps "keys == [\"dependencies\",\"manager\"] and .dependencies == [\"left-pad\"]"
  llm_deps "$tmp" --dev | schema deps_dev "keys == [\"dev\",\"manager\",\"prod\"] and .dev == [\"jest\"]"
  llm_procs --name bash | schema procs "type == \"array\" and all(.[]; (.pid | type == \"number\") and (.name | type == \"string\"))"
  PATH="$GASH_TEST_ROOT/tests/mocks/bin:$PATH" llm_ports | schema ports "type == \"array\" and all(.[]; (.port | type == \"number\") and (.proto | type == \"string\"))"
'
