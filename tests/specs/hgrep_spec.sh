#!/usr/bin/env bash
# Tests for hgrep function

describe "hgrep"

it "hgrep deduplicates keeping last execution" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
#1700000000
echo hello
#1700000100
echo world
#1700000200
echo hello
EOF

    # Use JSON mode for timezone-agnostic test
    HISTFILE="$tmp" out="$(hgrep echo -j 2>/dev/null)"

    # Should have 2 items (hello deduplicated)
    # Count JSON objects by counting "epoch" occurrences
    [[ $(echo "$out" | grep -o "\"epoch\"" | wc -l) -eq 2 ]]
    # "hello" should have epoch 1700000200 (later one wins)
    [[ "$out" == *"\"epoch\":1700000200"*"echo hello"* ]]
'

it "hgrep respects --limit option" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
#1700000000
cmd1
#1700000100
cmd2
#1700000200
cmd3
EOF

    HISTFILE="$tmp" out="$(hgrep cmd -n 2 -H 2>/dev/null)"
    [[ $(echo "$out" | wc -l) -eq 2 ]]
    # Should have cmd2 and cmd3 (last 2)
    [[ "$out" == *"cmd2"* ]]
    [[ "$out" == *"cmd3"* ]]
'

it "hgrep excludes self-references by default" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
#1700000000
hgrep test
#1700000100
echo test
EOF

    HISTFILE="$tmp" out="$(hgrep test -H 2>/dev/null)"
    [[ $(echo "$out" | wc -l) -eq 1 ]]
    [[ "$out" == *"echo test"* ]]
'

it "hgrep includes self-references with -a flag" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
#1700000000
hgrep test
#1700000100
echo test
EOF

    HISTFILE="$tmp" out="$(hgrep test -a -H 2>/dev/null)"
    [[ $(echo "$out" | wc -l) -eq 2 ]]
'

it "hgrep --reverse shows oldest first" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
#1700000000
old_cmd
#1700000200
new_cmd
EOF

    HISTFILE="$tmp" out="$(hgrep cmd -r -H 2>/dev/null)"
    # First line should be new_cmd (reversed)
    first_line=$(echo "$out" | head -1)
    [[ "$first_line" == *"new_cmd"* ]]
'

it "hgrep --json outputs valid JSON" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
#1700000000
git commit
#1700000100
git push
EOF

    HISTFILE="$tmp" out="$(hgrep git -j 2>/dev/null)"
    # Should be valid JSON array
    [[ "$out" == "["* ]]
    [[ "$out" == *"]" ]]
    [[ "$out" == *"\"timestamp\":"* ]]
    [[ "$out" == *"\"command\":"* ]]
    [[ "$out" == *"\"epoch\":"* ]]
'

it "hgrep --count returns correct count" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
#1700000000
git status
#1700000100
git push
#1700000200
git status
EOF

    HISTFILE="$tmp" out="$(hgrep git -c 2>/dev/null)"
    # Should be 2 (git status deduplicated)
    [[ "$out" == "2" ]]
'

it "hgrep --regex enables extended regex" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
#1700000000
git push
#1700000100
git pull
#1700000200
git status
EOF

    HISTFILE="$tmp" out="$(hgrep -E "^git (push|pull)" -H 2>/dev/null)"
    [[ $(echo "$out" | wc -l) -eq 2 ]]
    [[ "$out" != *"status"* ]]
'

it "hgrep fails gracefully on missing pattern" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    set +e
    out="$(hgrep 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"Missing"* ]]
'

it "hgrep shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    out="$(hgrep -h 2>&1)"
    [[ "$out" == *"hgrep PATTERN"* ]]
    [[ "$out" == *"--limit"* ]]
    [[ "$out" == *"--json"* ]]
'

it "hgrep rejects unknown options" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    set +e
    out="$(hgrep test --invalid 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"Unknown option"* ]]
'

it "hgrep returns empty JSON array when no matches" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
#1700000000
git status
EOF

    HISTFILE="$tmp" out="$(hgrep nonexistent -j 2>/dev/null)"
    [[ "$out" == "[]" ]]
'

it "hgrep returns 0 for count when no matches" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
#1700000000
git status
EOF

    HISTFILE="$tmp" out="$(hgrep nonexistent -c 2>/dev/null)"
    [[ "$out" == "0" ]]
'

it "hgrep handles commands with special characters" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
#1700000000
echo "hello world" | grep -E "test.*pattern"
EOF

    HISTFILE="$tmp" out="$(hgrep grep -H 2>/dev/null)"
    [[ $(echo "$out" | wc -l) -eq 1 ]]
'

it "hgrep JSON escapes special characters" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
#1700000000
echo "test with \"quotes\""
EOF

    HISTFILE="$tmp" out="$(hgrep echo -j 2>/dev/null)"
    # Should have escaped quotes
    [[ "$out" == *"\\\""* ]]
'
