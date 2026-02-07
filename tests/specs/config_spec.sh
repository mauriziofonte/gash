#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Config Parser - URL Encoding/Decoding"

it "__gash_url_decode decodes simple %XX sequences" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    result="$(__gash_url_decode "hello%20world")"
    [[ "$result" == "hello world" ]]
'

it "__gash_url_decode decodes @ symbol (%40)" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    result="$(__gash_url_decode "p%40ssword")"
    [[ "$result" == "p@ssword" ]]
'

it "__gash_url_decode decodes colon (%3A)" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    result="$(__gash_url_decode "pass%3Aword")"
    [[ "$result" == "pass:word" ]]
'

it "__gash_url_decode handles multiple encoded chars" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    result="$(__gash_url_decode "p%40ss%3Aw%2Frd")"
    [[ "$result" == "p@ss:w/rd" ]]
'

it "__gash_url_decode leaves plain text unchanged" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    result="$(__gash_url_decode "plaintext123")"
    [[ "$result" == "plaintext123" ]]
'

it "__gash_url_encode encodes @ symbol" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    result="$(__gash_url_encode "p@ssword")"
    [[ "$result" == "p%40ssword" ]]
'

it "__gash_url_encode leaves alphanumeric unchanged" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    result="$(__gash_url_encode "abc123XYZ")"
    [[ "$result" == "abc123XYZ" ]]
'

describe "Config Parser - DB URL Parsing"

it "__gash_parse_db_url parses mysql URL" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    test_parse() {
        # Use r_ prefix to avoid nameref conflicts with function internals
        local r_driver="" r_user="" r_pass="" r_host="" r_port="" r_db=""
        __gash_parse_db_url "mysql://root:secret@localhost:3306/myapp" r_driver r_user r_pass r_host r_port r_db

        [[ "$r_driver" == "mysql" ]]
        [[ "$r_user" == "root" ]]
        [[ "$r_pass" == "secret" ]]
        [[ "$r_host" == "localhost" ]]
        [[ "$r_port" == "3306" ]]
        [[ "$r_db" == "myapp" ]]
    }
    test_parse
'

it "__gash_parse_db_url parses pgsql URL" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    test_parse() {
        local r_driver="" r_user="" r_pass="" r_host="" r_port="" r_db=""
        __gash_parse_db_url "pgsql://pguser:pgpass@dbhost:5432/analytics" r_driver r_user r_pass r_host r_port r_db

        [[ "$r_driver" == "pgsql" ]]
        [[ "$r_user" == "pguser" ]]
        [[ "$r_pass" == "pgpass" ]]
        [[ "$r_host" == "dbhost" ]]
        [[ "$r_port" == "5432" ]]
        [[ "$r_db" == "analytics" ]]
    }
    test_parse
'

it "__gash_parse_db_url parses mariadb URL" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    test_parse() {
        local r_driver="" r_user="" r_pass="" r_host="" r_port="" r_db=""
        __gash_parse_db_url "mariadb://admin:pw123@192.168.1.100:3307/prod" r_driver r_user r_pass r_host r_port r_db

        [[ "$r_driver" == "mariadb" ]]
        [[ "$r_user" == "admin" ]]
        [[ "$r_pass" == "pw123" ]]
        [[ "$r_host" == "192.168.1.100" ]]
        [[ "$r_port" == "3307" ]]
        [[ "$r_db" == "prod" ]]
    }
    test_parse
'

it "__gash_parse_db_url handles URL-encoded password" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    test_parse() {
        local r_driver="" r_user="" r_pass="" r_host="" r_port="" r_db=""
        __gash_parse_db_url "mysql://root:p%40ss%3Aword@localhost:3306/app" r_driver r_user r_pass r_host r_port r_db

        [[ "$r_pass" == "p%40ss%3Aword" ]]

        # Decode and verify
        local decoded=""
        decoded="$(__gash_url_decode "$r_pass")"
        [[ "$decoded" == "p@ss:word" ]]
    }
    test_parse
'

it "__gash_parse_db_url handles URL without database" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    test_parse() {
        local r_driver="" r_user="" r_pass="" r_host="" r_port="" r_db=""
        __gash_parse_db_url "mysql://root:pass@localhost:3306" r_driver r_user r_pass r_host r_port r_db

        [[ "$r_driver" == "mysql" ]]
        [[ "$r_user" == "root" ]]
        [[ "$r_host" == "localhost" ]]
        [[ "$r_port" == "3306" ]]
        [[ -z "$r_db" ]]
    }
    test_parse
'

it "__gash_parse_db_url defaults mysql port to 3306" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    test_parse() {
        local r_driver="" r_user="" r_pass="" r_host="" r_port="" r_db=""
        __gash_parse_db_url "mysql://root:pass@localhost/mydb" r_driver r_user r_pass r_host r_port r_db
        [[ "$r_port" == "3306" ]]
    }
    test_parse
'

it "__gash_parse_db_url defaults pgsql port to 5432" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    test_parse() {
        local r_driver="" r_user="" r_pass="" r_host="" r_port="" r_db=""
        __gash_parse_db_url "pgsql://user:pass@localhost/mydb" r_driver r_user r_pass r_host r_port r_db
        [[ "$r_port" == "5432" ]]
    }
    test_parse
'

describe "Config Parser - File Loading"

it "__gash_load_env handles missing file gracefully" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Point to non-existent file
    export GASH_ENV_FILE="/nonexistent/.gash_env"

    # Reset cache
    __GASH_ENV_LOADED=""

    # Should not fail
    __gash_load_env
    [[ "$__GASH_ENV_LOADED" == "1" ]]
    [[ -z "$__GASH_ENV_DB_ENTRIES" ]]
    [[ -z "$__GASH_ENV_SSH_KEYS" ]]
'

it "__gash_load_env parses SSH entries" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    # Create a fake key file
    fake_key="$(mktemp)"
    trap "rm -f $tmp $fake_key" EXIT

    cat > "$tmp" <<EOF
# Test config
SSH:$fake_key=mypassphrase
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    __gash_load_env

    # Should have parsed the SSH key
    keys="$(__gash_get_ssh_keys)"
    [[ "$keys" == *"$fake_key"* ]]
    [[ "$keys" == *"mypassphrase"* ]]
'

it "__gash_load_env parses DB entries" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
# Test config
DB:default=mysql://root:pass@localhost:3306/myapp
DB:postgres=pgsql://pg:secret@dbhost:5432/analytics
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    __gash_load_env

    # Should have parsed the DB entries
    url="$(__gash_get_db_url "default")"
    [[ "$url" == "mysql://root:pass@localhost:3306/myapp" ]]

    url="$(__gash_get_db_url "postgres")"
    [[ "$url" == "pgsql://pg:secret@dbhost:5432/analytics" ]]
'

it "__gash_get_db_url returns error for missing connection" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file with only one connection
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
DB:default=mysql://root:pass@localhost:3306/myapp
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    __gash_load_env

    # Should fail for non-existent connection
    set +e
    url="$(__gash_get_db_url "nonexistent")"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
'

it "__gash_load_env skips comments and empty lines" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
# This is a comment

   # Indented comment

DB:default=mysql://root:pass@localhost:3306/myapp

# Another comment
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    __gash_load_env

    # Should only have the default connection
    url="$(__gash_get_db_url "default")"
    [[ "$url" == "mysql://root:pass@localhost:3306/myapp" ]]
'

it "__gash_load_env trims whitespace" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
   DB:default=mysql://root:pass@localhost:3306/myapp
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    __gash_load_env

    url="$(__gash_get_db_url "default")"
    [[ "$url" == "mysql://root:pass@localhost:3306/myapp" ]]
'

it "__gash_reload_env clears and reloads cache" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
DB:default=mysql://root:pass1@localhost:3306/db1
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    __gash_load_env
    url1="$(__gash_get_db_url "default")"

    # Update file
    cat > "$tmp" <<EOF
DB:default=mysql://root:pass2@localhost:3306/db2
EOF

    # Without reload, should return cached value
    url_cached="$(__gash_get_db_url "default")"
    [[ "$url_cached" == "$url1" ]]

    # After reload, should return new value
    __gash_reload_env
    url2="$(__gash_get_db_url "default")"
    [[ "$url2" == "mysql://root:pass2@localhost:3306/db2" ]]
'

describe "Config Parser - Security"

it "__gash_load_env skips SSH keys for non-existent files" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
SSH:/nonexistent/key=passphrase
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    # Should not fail, just warn and skip
    __gash_load_env 2>/dev/null

    # SSH keys should be empty
    keys="$(__gash_get_ssh_keys)"
    [[ -z "$keys" ]]
'

it "__gash_load_env skips invalid DB URL formats" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
DB:invalid=sqlite://invalid
DB:default=mysql://root:pass@localhost:3306/myapp
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    # Should not fail, just warn and skip invalid
    __gash_load_env 2>/dev/null

    # Valid connection should work
    url="$(__gash_get_db_url "default")"
    [[ "$url" == "mysql://root:pass@localhost:3306/myapp" ]]

    # Invalid should not be found
    set +e
    url="$(__gash_get_db_url "invalid")"
    rc=$?
    set -e
    [[ $rc -ne 0 ]]
'

describe "Config Parser - Public Functions"

it "gash_db_list shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    out="$(gash_db_list -h)"
    [[ "$out" == *"gash_db_list"* ]]
'

it "gash_db_test shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    out="$(gash_db_test -h)"
    [[ "$out" == *"gash_db_test"* ]]
'

it "gash_env_init shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    out="$(gash_env_init -h)"
    [[ "$out" == *"gash_env_init"* ]]
'

it "gash_env_init refuses to overwrite without --force" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp home dir
    tmp_home="$(mktemp -d)"
    trap "rm -rf $tmp_home" EXIT

    # Create existing config
    touch "$tmp_home/.gash_env"

    export HOME="$tmp_home"
    export GASH_DIR="$ROOT"

    set +e
    gash_env_init 2>/dev/null
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
'

it "gash_env_init creates config with --force" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp home dir
    tmp_home="$(mktemp -d)"
    trap "rm -rf $tmp_home" EXIT

    # Create existing config
    touch "$tmp_home/.gash_env"

    export HOME="$tmp_home"
    export GASH_DIR="$ROOT"

    gash_env_init --force

    # File should exist with correct permissions
    [[ -f "$tmp_home/.gash_env" ]]
    perms="$(stat -c %a "$tmp_home/.gash_env")"
    [[ "$perms" == "600" ]]
'

# =============================================================================
# DB URL Parsing Edge Cases
# =============================================================================

it "__gash_parse_db_url handles @ in password (URL-encoded)" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    t_drv="" t_usr="" t_pw="" t_hst="" t_prt="" t_dbn=""
    __gash_parse_db_url "mysql://admin:p%40ss%3Aword@dbhost:3306/mydb" \
        t_drv t_usr t_pw t_hst t_prt t_dbn

    [[ "$t_drv" == "mysql" ]]
    [[ "$t_usr" == "admin" ]]
    [[ "$t_hst" == "dbhost" ]]
    [[ "$t_prt" == "3306" ]]
    [[ "$t_dbn" == "mydb" ]]

    # Decode password and verify
    decoded="$(__gash_url_decode "$t_pw")"
    [[ "$decoded" == "p@ss:word" ]]
'

it "__gash_parse_db_url handles raw @ in password (split on last @)" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    t_drv="" t_usr="" t_pw="" t_hst="" t_prt="" t_dbn=""
    __gash_parse_db_url "mysql://user:p@ss@localhost:3306/db" \
        t_drv t_usr t_pw t_hst t_prt t_dbn

    [[ "$t_drv" == "mysql" ]]
    [[ "$t_usr" == "user" ]]
    [[ "$t_pw" == "p@ss" ]]
    [[ "$t_hst" == "localhost" ]]
    [[ "$t_prt" == "3306" ]]
    [[ "$t_dbn" == "db" ]]
'
