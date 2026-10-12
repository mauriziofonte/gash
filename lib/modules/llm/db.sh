#!/usr/bin/env bash

# Gash Module: LLM Utilities: databases
# Read-only queries on SQLite, MySQL/MariaDB and PostgreSQL.
# Sourced by lib/modules/llm.sh.

# -----------------------------------------------------------------------------
# Database Functions (Read-Only)
# -----------------------------------------------------------------------------

# Resolve a named database connection to its components via namerefs.
# Fetches the URL from config, parses it, decodes password, resolves database name.
# Usage: __llm_resolve_db <connection> <database> <out_drv> <out_usr> <out_pw> <out_hst> <out_prt> <out_dbn>
__llm_resolve_db() {
    local conn="$1" db_name="$2"
    local -n _rd_drv="$3" _rd_usr="$4" _rd_pw="$5" _rd_hst="$6" _rd_prt="$7" _rd_dbn="$8"

    local db_url
    __gash_load_env
    db_url=$(__gash_get_db_url "$conn") || {
        __llm_error "no_db_config" "Connection '$conn' not found" "STOP" "true" "Ask user which database connection to use. Connections are configured in ~/.gash_env"
        return 1
    }

    __gash_parse_db_url "$db_url" _rd_drv _rd_usr _rd_pw _rd_hst _rd_prt _rd_dbn
    _rd_pw=$(__gash_url_decode "$_rd_pw")

    # Use explicit database name if provided, otherwise fall back to URL or .target-database
    if [[ -n "$db_name" ]]; then
        _rd_dbn="$db_name"
    elif [[ -z "$_rd_dbn" ]]; then
        if [[ -f ".target-database" ]]; then
            _rd_dbn="$(<.target-database)"
            _rd_dbn="${_rd_dbn%%[[:space:]]*}"
        fi
    fi

    if [[ -z "$_rd_dbn" ]]; then
        __llm_error "no_database" "Database name not specified" "STOP" "true" "Ask user for database name. Use -d option or specify in connection URL"
        return 1
    fi
}

# Check if sqlite3 is available.
# Usage: __llm_has_sqlite
__llm_has_sqlite() {
    type -P sqlite3 >/dev/null 2>&1
}

# Execute a SQLite query with JSON output.
# Usage: __llm_sqlite_query <query> <sqlite_file> [max_rows]
__llm_sqlite_query() {
    local query="${1-}"
    local sqlite_file="${2-}"
    local max_rows="${3-100}"

    __llm_validate_uint "rows" "$max_rows" || return 1

    # Check sqlite3 is installed
    if ! __llm_has_sqlite; then
        __llm_error "sqlite_not_found" "SQLite3 CLI not installed" "FATAL" "false" "sqlite3 binary not found. Install with: sudo apt install sqlite3"
        return 1
    fi

    # Validate file path
    if [[ -z "$sqlite_file" ]]; then
        __llm_error "missing_sqlite_file" "" "RETRY" "true" "Provide SQLite file path with -f option"
        return 1
    fi

    local safe_path
    safe_path="$(__llm_validate_path "$sqlite_file")" || return 1

    if [[ ! -f "$safe_path" ]]; then
        __llm_error "sqlite_file_not_found" "$safe_path" "RETRY" "true" "SQLite file does not exist. Check the path"
        return 1
    fi

    # Verify it's a valid SQLite file
    if ! file "$safe_path" 2>/dev/null | grep -q "SQLite"; then
        __llm_error "invalid_sqlite_file" "$safe_path is not a valid SQLite database" "RETRY" "true" "Provide a valid SQLite database file"
        return 1
    fi

    # Block semicolon (multiple statements)
    if [[ "$query" == *";"* ]]; then
        __llm_error "multiple_statements_blocked" "Only single SQL statements allowed" "FATAL" "false" "Remove semicolons - only one statement per query"
        return 1
    fi

    # Add LIMIT if not present in SELECT queries
    local upper_query
    upper_query="$(echo "$query" | tr '[:lower:]' '[:upper:]')"
    if [[ "$upper_query" =~ ^[[:space:]]*SELECT ]] && [[ ! "$upper_query" =~ LIMIT ]]; then
        query="$query LIMIT $max_rows"
    fi

    # Execute query with JSON output
    local sqlite_output sqlite_stderr sqlite_exit
    sqlite_stderr=$(mktemp)
    sqlite_output=$(sqlite3 -json -readonly "$safe_path" "$query" 2>"$sqlite_stderr")
    sqlite_exit=$?

    # Check for errors
    if [[ $sqlite_exit -ne 0 ]]; then
        local err_msg=""
        if [[ -s "$sqlite_stderr" ]]; then
            err_msg=$(<"$sqlite_stderr")
        fi
        rm -f "$sqlite_stderr"
        err_msg=$(__llm_clean_error_msg "$err_msg")
        if [[ -n "$err_msg" ]]; then
            __llm_error "sqlite_error" "$err_msg" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
        else
            __llm_error "sqlite_error" "Query failed with exit code $sqlite_exit" "RETRY" "true" "Query failed. Check SQL syntax and retry"
        fi
        return 1
    fi
    rm -f "$sqlite_stderr"

    echo "$sqlite_output"
}

# Execute a read-only database query with JSON output.
# Usage: llm_db_query <query> [-d DATABASE] [-c CONNECTION] [-f SQLITE_FILE]
# Example: llm_db_query "SELECT * FROM users LIMIT 5" -c myconn
# Example: llm_db_query "SELECT * FROM users" -f /path/to/db.sqlite
llm_db_query() {
    if needs_help "llm_db_query" "llm_db_query <query> [-c CONNECTION] [-f SQLITE_FILE] [-r ROWS]" "Execute read-only database query (JSON envelope with timing). Use -c for MySQL/PostgreSQL, -f for SQLite" "${1-}"; then
        return 0
    fi

    # Build explain args: copy "$@" but strip -r/--rows (not accepted by explain)
    local __GASH_DB_EXPLAIN_ARGS=()
    local _qw_args=("$@") _qw_i=0
    while [[ $_qw_i -lt ${#_qw_args[@]} ]]; do
        case "${_qw_args[$_qw_i]}" in
            -r | --rows) _qw_i=$((_qw_i + 2)) ;;
            *)
                __GASH_DB_EXPLAIN_ARGS+=("${_qw_args[$_qw_i]}")
                _qw_i=$((_qw_i + 1))
                ;;
        esac
    done
    local __GASH_DB_SLOW_QUERY=1

    __gash_no_history __llm_db_envelope __llm_db_query_impl "$@"
}

__llm_db_query_impl() {
    local query=""
    local database=""
    local connection="default"
    local sqlite_file=""
    local max_rows=100

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d | --database)
                database="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -c | --connection)
                connection="${2-default}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -f | --file)
                sqlite_file="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -r | --rows)
                max_rows="${2-100}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use -d DATABASE, -c CONNECTION, -f SQLITE_FILE, or -r ROWS"
                return 1
                ;;
            *)
                query="$1"
                shift
                ;;
        esac
    done

    if [[ -z "$query" ]]; then
        __llm_error "missing_query" "" "RETRY" "true" "Provide a SQL query as first argument"
        return 1
    fi

    # Security: Only allow read-only queries (validate BEFORE loading config)
    local upper_query
    upper_query="$(echo "$query" | tr '[:lower:]' '[:upper:]')"

    # Block write operations
    if [[ "$upper_query" =~ ^[[:space:]]*(INSERT|UPDATE|DELETE|DROP|CREATE|ALTER|TRUNCATE|GRANT|REVOKE) ]]; then
        __llm_error "write_operation_blocked" "Only SELECT, SHOW, DESCRIBE, EXPLAIN allowed" "FATAL" "false" "Write operations are blocked for safety"
        return 1
    fi

    # Block semicolon (multiple statements)
    if [[ "$query" == *";"* ]]; then
        __llm_error "multiple_statements_blocked" "Only single SQL statements allowed" "FATAL" "false" "Remove semicolons - only one statement per query"
        return 1
    fi

    # -r/--rows is interpolated into the LIMIT clause AFTER the semicolon
    # check above: it must be a plain integer or it becomes an injection.
    __llm_validate_uint "rows" "$max_rows" || return 1

    # SQLite mode: use -f option
    if [[ -n "$sqlite_file" ]]; then
        __llm_sqlite_query "$query" "$sqlite_file" "$max_rows"
        return $?
    fi

    # Resolve database connection
    local db_driver db_user db_pass db_host db_port db_database
    __llm_resolve_db "$connection" "$database" db_driver db_user db_pass db_host db_port db_database || return 1
    database="$db_database"

    # Add LIMIT if not present in SELECT queries
    if [[ "$upper_query" =~ ^[[:space:]]*SELECT ]] && [[ ! "$upper_query" =~ LIMIT ]]; then
        query="$query LIMIT $max_rows"
    fi

    case "$db_driver" in
        mysql | mariadb)
            local mysql_bin
            mysql_bin=$(type -P mariadb 2>/dev/null) || mysql_bin=$(type -P mysql 2>/dev/null) || true
            if [[ -z "$mysql_bin" ]]; then
                __llm_error "mysql_not_found" "MySQL/MariaDB client not installed" "FATAL" "false" "mysql/mariadb binary not found. User must install: sudo apt install mariadb-client"
                return 1
            fi

            # Execute query with TSV output, capturing stderr for errors.
            # Intentionally NOT --raw: mysql --batch without --raw escapes
            # tab/newline/backslash/NUL as \t \n \\ \0 — these are 1:1
            # JSON-compatible (handled by __llm_tsv_to_json mysql mode),
            # so cell values with embedded control chars survive round-trip.
            local mysql_output mysql_stderr mysql_exit
            mysql_stderr=$(mktemp)
            mysql_output=$("$mysql_bin" --defaults-extra-file=<(__llm_mysql_client_ini "$db_user" "$db_pass" "$db_host" "$db_port") "$database" \
                --default-character-set=utf8mb4 \
                -e "$query" \
                --batch 2>"$mysql_stderr")
            mysql_exit=$?

            # Read stderr and clean up temp file
            local err_msg=""
            if [[ -s "$mysql_stderr" ]]; then
                err_msg=$(<"$mysql_stderr")
            fi
            rm -f "$mysql_stderr"

            # Filter out warnings (not errors)
            local filtered_err
            filtered_err=$(__llm_clean_error_msg "$err_msg" --mysql)

            # If mysql exited with error, always return structured error
            if [[ $mysql_exit -ne 0 ]]; then
                if [[ -n "$filtered_err" ]]; then
                    __llm_error "mysql_error" "$filtered_err" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
                else
                    __llm_error "mysql_error" "Query failed with exit code $mysql_exit" "RETRY" "true" "Query failed. Check SQL syntax and retry"
                fi
                return 1
            fi

            # If there's a real error message (not just warnings), report it
            if [[ -n "$filtered_err" ]]; then
                __llm_error "mysql_error" "$filtered_err" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
                return 1
            fi

            # Convert TSV output to JSON array (shared helper, mysql mode
            # preserves \t/\n/\\ pre-escaped sequences from --batch)
            printf '%s\n' "$mysql_output" | __llm_tsv_to_json mysql
            ;;
        pgsql)
            if ! type -P psql >/dev/null 2>&1; then
                __llm_error "psql_not_found" "PostgreSQL client not installed" "FATAL" "false" "psql binary not found. User must install: sudo apt install postgresql-client"
                return 1
            fi

            # Strategy: for SELECT/WITH/VALUES/TABLE (wrappable) queries, wrap
            # with `jsonb_agg(to_jsonb(...))` so PostgreSQL itself serializes
            # the result as native JSON. This preserves PG types (int, bool,
            # null, arrays, nested jsonb) and — crucially — handles tabs,
            # newlines and backslashes inside cell values without any TSV
            # parsing fragility. Non-wrappable statements (SHOW, etc.) fall
            # back to the shared TSV helper.
            local psql_output psql_stderr psql_exit use_jsonb=0 exec_query
            psql_stderr=$(mktemp)

            if [[ "$upper_query" =~ ^[[:space:]]*(SELECT|WITH|VALUES|TABLE)[[:space:]] ]]; then
                use_jsonb=1
                exec_query="SELECT COALESCE(jsonb_agg(to_jsonb(__gash_row)), '[]'::jsonb) FROM (${query}) __gash_row"
                psql_output=$(PGPASSWORD="$db_pass" psql -U "$db_user" -h "$db_host" -p "$db_port" -d "$database" \
                    -At -c "$exec_query" 2>"$psql_stderr")
            else
                # Fallback: header row + no footer, tab-separated.
                psql_output=$(PGPASSWORD="$db_pass" psql -U "$db_user" -h "$db_host" -p "$db_port" -d "$database" \
                    -P footer=off -A -F $'\t' -c "$query" 2>"$psql_stderr")
            fi
            psql_exit=$?

            # Check for PostgreSQL errors. Only exit code failures count as
            # errors — NOTICE/WARNING lines also land on stderr but do not
            # fail the query (e.g. identifier truncation, deprecated feature
            # warnings), so ignore stderr when psql_exit == 0.
            if [[ $psql_exit -ne 0 ]]; then
                local err_msg=""
                [[ -s "$psql_stderr" ]] && err_msg=$(<"$psql_stderr")
                rm -f "$psql_stderr"
                err_msg=$(__llm_clean_error_msg "$err_msg")
                if [[ -n "$err_msg" ]]; then
                    __llm_error "pgsql_error" "$err_msg" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
                else
                    __llm_error "pgsql_error" "Query failed with exit code $psql_exit" "RETRY" "true" "Query failed. Check SQL syntax and retry"
                fi
                return 1
            fi
            rm -f "$psql_stderr"

            if [[ $use_jsonb -eq 1 ]]; then
                # psql -At emitted the jsonb_agg result as raw JSON text
                printf '%s\n' "$psql_output"
            else
                # TSV fallback (shared helper, symmetric with mysql)
                printf '%s\n' "$psql_output" | __llm_tsv_to_json raw
            fi
            ;;
        *)
            __llm_error "unknown_db_type" "$db_driver" "STOP" "false" "Unsupported database driver. Only mysql, mariadb, pgsql are supported"
            return 1
            ;;
    esac
}

# List all tables in the database.
# Usage: llm_db_tables [-c CONNECTION] [-f SQLITE_FILE]
llm_db_tables() {
    if needs_help "llm_db_tables" "llm_db_tables [-c CONNECTION] [-f SQLITE_FILE]" "List database tables (JSON array). Use -c for MySQL/PostgreSQL, -f for SQLite" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_db_envelope __llm_db_tables_impl "$@"
}

__llm_db_tables_impl() {
    local database=""
    local connection="default"
    local sqlite_file=""

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d | --database)
                database="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -c | --connection)
                connection="${2-default}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -f | --file)
                sqlite_file="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            *) shift ;;
        esac
    done

    # SQLite mode
    if [[ -n "$sqlite_file" ]]; then
        local result
        result=$(__llm_sqlite_query "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name" "$sqlite_file" 1000) || return 1
        if __llm_has_jq; then
            printf '%s' "$result" | jq -c '[.[].name]'
        else
            printf '%s\n' "$result"
        fi
        return 0
    fi

    # Get driver from connection to determine query type
    local db_url
    __gash_load_env
    db_url=$(__gash_get_db_url "$connection") || {
        __llm_error "no_db_config" "Connection '$connection' not found" "STOP" "true" "Ask user which database connection to use"
        return 1
    }

    local db_driver _u _p _h _port _db
    __gash_parse_db_url "$db_url" db_driver _u _p _h _port _db

    local args=()
    [[ -n "$database" ]] && args+=(-d "$database")
    args+=(-c "$connection")

    local result
    case "$db_driver" in
        mysql | mariadb)
            result=$(__llm_db_query_impl "SHOW TABLES" "${args[@]}" 2>/dev/null) || return 1
            if __llm_has_jq; then
                printf '%s' "$result" | jq -c '[.[].[] | values]'
            else
                # Fallback: extract table names without jq
                printf '%s' "$result" | sed 's/^\[{[^}]*:"\([^"]*\)"}\(,\|]\)/"\1"\2/g'
            fi
            ;;
        pgsql)
            result=$(__llm_db_query_impl "SELECT tablename FROM pg_tables WHERE schemaname='public'" "${args[@]}" 2>/dev/null) || return 1
            if __llm_has_jq; then
                printf '%s' "$result" | jq -c '[.[].tablename]'
            else
                printf '%s\n' "$result"
            fi
            ;;
        *)
            __llm_error "unknown_db_type" "$db_driver" "STOP" "false" "Unsupported database driver"
            return 1
            ;;
    esac
}

# Show schema for a table.
# Usage: llm_db_schema <table> [-c CONNECTION] [-f SQLITE_FILE]
llm_db_schema() {
    if needs_help "llm_db_schema" "llm_db_schema <table> [-c CONNECTION] [-f SQLITE_FILE]" "Show table schema (JSON). Use -c for MySQL/PostgreSQL, -f for SQLite" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_db_envelope __llm_db_schema_impl "$@"
}

__llm_db_schema_impl() {
    local table=""
    local database=""
    local connection="default"
    local sqlite_file=""

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d | --database)
                database="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -c | --connection)
                connection="${2-default}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -f | --file)
                sqlite_file="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -*) shift ;;
            *)
                table="$1"
                shift
                ;;
        esac
    done

    if [[ -z "$table" ]]; then
        __llm_error "missing_table" "" "RETRY" "true" "Provide table name as first argument"
        return 1
    fi

    # Sanitize table name (alphanumeric and underscore only)
    if [[ ! "$table" =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
        __llm_error "invalid_table_name" "Table name must be alphanumeric with underscores" "RETRY" "true" "Use only letters, numbers, and underscores"
        return 1
    fi

    # SQLite mode
    if [[ -n "$sqlite_file" ]]; then
        __llm_sqlite_query "SELECT cid, name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('$table')" "$sqlite_file" 1000
        return $?
    fi

    # Get driver from connection
    local db_url
    __gash_load_env
    db_url=$(__gash_get_db_url "$connection") || {
        __llm_error "no_db_config" "Connection '$connection' not found" "STOP" "true" "Ask user which database connection to use"
        return 1
    }

    local db_driver _u _p _h _port _db
    __gash_parse_db_url "$db_url" db_driver _u _p _h _port _db

    local args=()
    [[ -n "$database" ]] && args+=(-d "$database")
    args+=(-c "$connection")

    case "$db_driver" in
        mysql | mariadb)
            __llm_db_query_impl "DESCRIBE $table" "${args[@]}"
            ;;
        pgsql)
            __llm_db_query_impl "SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_name='$table' AND table_schema='public' ORDER BY ordinal_position" "${args[@]}"
            ;;
        *)
            __llm_error "unknown_db_type" "$db_driver" "STOP" "false" "Unsupported database driver"
            return 1
            ;;
    esac
}

# Get sample rows from a table.
# Usage: llm_db_sample <table> [-c CONNECTION] [-f SQLITE_FILE] [--limit N]
llm_db_sample() {
    if needs_help "llm_db_sample" "llm_db_sample <table> [-c CONNECTION] [-f SQLITE_FILE] [--limit N]" "Get sample rows from table (default 5 rows). Use -c for MySQL/PostgreSQL, -f for SQLite" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_db_envelope __llm_db_sample_impl "$@"
}

__llm_db_sample_impl() {
    local table=""
    local database=""
    local connection="default"
    local sqlite_file=""
    local limit=5

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d | --database)
                database="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -c | --connection)
                connection="${2-default}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -f | --file)
                sqlite_file="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            --limit)
                limit="${2-5}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -*) shift ;;
            *)
                table="$1"
                shift
                ;;
        esac
    done

    if [[ -z "$table" ]]; then
        __llm_error "missing_table" "" "RETRY" "true" "Provide table name as first argument"
        return 1
    fi

    # Sanitize table name
    if [[ ! "$table" =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
        __llm_error "invalid_table_name" "Table name must be alphanumeric with underscores" "RETRY" "true" "Use only letters, numbers, and underscores"
        return 1
    fi

    # --limit is interpolated into the LIMIT clause: integers only
    __llm_validate_uint "limit" "$limit" || return 1

    # SQLite mode
    if [[ -n "$sqlite_file" ]]; then
        __llm_sqlite_query "SELECT * FROM $table LIMIT $limit" "$sqlite_file" "$limit"
        return $?
    fi

    local args=()
    [[ -n "$database" ]] && args+=(-d "$database")
    args+=(-c "$connection")

    __llm_db_query_impl "SELECT * FROM $table LIMIT $limit" "${args[@]}"
}

# Explain a query execution plan.
# Usage: llm_db_explain <QUERY> [-c CONNECTION] [-f SQLITE_FILE] [--analyze]
llm_db_explain() {
    if needs_help "llm_db_explain" "llm_db_explain <QUERY> [-c CONNECTION] [-f SQLITE_FILE] [--analyze]" "Analyze query execution plan (EXPLAIN). Use -c for MySQL/PostgreSQL, -f for SQLite" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_db_envelope __llm_db_explain_impl "$@"
}

__llm_db_explain_impl() {
    local query=""
    local database=""
    local connection="default"
    local sqlite_file=""
    local analyze=0

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d | --database)
                database="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -c | --connection)
                connection="${2-default}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -f | --file)
                sqlite_file="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            --analyze)
                analyze=1
                shift
                ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use -c CONNECTION, -f SQLITE_FILE, or --analyze"
                return 1
                ;;
            *)
                query="$1"
                shift
                ;;
        esac
    done

    if [[ -z "$query" ]]; then
        __llm_error "missing_query" "" "RETRY" "true" "Provide a SQL query to analyze as first argument"
        return 1
    fi

    # Security: Only allow read-only queries
    local upper_query
    upper_query="$(echo "$query" | tr '[:lower:]' '[:upper:]')"

    # Block write operations
    if [[ "$upper_query" =~ ^[[:space:]]*(INSERT|UPDATE|DELETE|DROP|CREATE|ALTER|TRUNCATE|GRANT|REVOKE) ]]; then
        __llm_error "write_operation_blocked" "Only SELECT queries can be explained" "FATAL" "false" "EXPLAIN only works with SELECT queries for safety"
        return 1
    fi

    # Block semicolon (multiple statements)
    if [[ "$query" == *";"* ]]; then
        __llm_error "multiple_statements_blocked" "Only single SQL statements allowed" "FATAL" "false" "Remove semicolons - only one statement per query"
        return 1
    fi

    # SQLite mode
    if [[ -n "$sqlite_file" ]]; then
        # Check sqlite3 is installed
        if ! __llm_has_sqlite; then
            __llm_error "sqlite_not_found" "SQLite3 CLI not installed" "FATAL" "false" "sqlite3 binary not found. Install with: sudo apt install sqlite3"
            return 1
        fi

        # Validate file path
        local safe_path
        safe_path="$(__llm_validate_path "$sqlite_file")" || return 1

        if [[ ! -f "$safe_path" ]]; then
            __llm_error "sqlite_file_not_found" "$safe_path" "RETRY" "true" "SQLite file does not exist. Check the path"
            return 1
        fi

        # SQLite EXPLAIN QUERY PLAN is always available
        local explain_cmd="EXPLAIN QUERY PLAN"

        local sqlite_output sqlite_stderr sqlite_exit
        sqlite_stderr=$(mktemp)
        sqlite_output=$(sqlite3 -json -readonly "$safe_path" "$explain_cmd $query" 2>"$sqlite_stderr")
        sqlite_exit=$?

        if [[ $sqlite_exit -ne 0 ]]; then
            local err_msg=""
            if [[ -s "$sqlite_stderr" ]]; then
                err_msg=$(<"$sqlite_stderr")
            fi
            rm -f "$sqlite_stderr"
            err_msg=$(__llm_clean_error_msg "$err_msg")
            if [[ -n "$err_msg" ]]; then
                __llm_error "sqlite_error" "$err_msg" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
            else
                __llm_error "sqlite_error" "Query failed with exit code $sqlite_exit" "RETRY" "true" "Query failed. Check SQL syntax and retry"
            fi
            return 1
        fi
        rm -f "$sqlite_stderr"

        echo "$sqlite_output"
        return 0
    fi

    # Resolve database connection
    local db_driver db_user db_pass db_host db_port db_database
    __llm_resolve_db "$connection" "$database" db_driver db_user db_pass db_host db_port db_database || return 1
    database="$db_database"

    # Build EXPLAIN prefix based on driver and options
    local explain_prefix

    case "$db_driver" in
        mysql | mariadb)
            local mysql_bin
            mysql_bin=$(type -P mariadb 2>/dev/null) || mysql_bin=$(type -P mysql 2>/dev/null) || true
            if [[ -z "$mysql_bin" ]]; then
                __llm_error "mysql_not_found" "MySQL/MariaDB client not installed" "FATAL" "false" "mysql/mariadb binary not found. User must install: sudo apt install mariadb-client"
                return 1
            fi

            # Detect if it's MariaDB or MySQL (MariaDB uses ANALYZE, MySQL uses EXPLAIN ANALYZE)
            local is_mariadb=0
            local version_output
            version_output=$("$mysql_bin" --defaults-extra-file=<(__llm_mysql_client_ini "$db_user" "$db_pass" "$db_host" "$db_port") "$database" \
                --default-character-set=utf8mb4 -N -e "SELECT VERSION()" 2>/dev/null)
            [[ "$version_output" == *"MariaDB"* ]] && is_mariadb=1

            if [[ $analyze -eq 1 ]]; then
                if [[ $is_mariadb -eq 1 ]]; then
                    # MariaDB: ANALYZE FORMAT=JSON <query>
                    explain_prefix="ANALYZE FORMAT=JSON"
                else
                    # MySQL 8.0.18+: EXPLAIN ANALYZE <query>
                    explain_prefix="EXPLAIN ANALYZE"
                fi
            else
                explain_prefix="EXPLAIN FORMAT=JSON"
            fi

            # Execute EXPLAIN query. Use --batch --raw here: EXPLAIN FORMAT=JSON
            # returns multiline JSON, and --batch (without --raw) would escape
            # newlines to literal \n, corrupting the JSON for downstream parsers.
            # This is safe because EXPLAIN output goes directly to jq, not
            # through the TSV helper.
            local mysql_output mysql_stderr mysql_exit
            mysql_stderr=$(mktemp)
            mysql_output=$("$mysql_bin" --defaults-extra-file=<(__llm_mysql_client_ini "$db_user" "$db_pass" "$db_host" "$db_port") "$database" \
                --default-character-set=utf8mb4 \
                -e "$explain_prefix $query" \
                --batch --raw 2>"$mysql_stderr")
            mysql_exit=$?

            # Check for errors (filter out password warning)
            if [[ $mysql_exit -ne 0 ]]; then
                local err_msg
                err_msg=$(<"$mysql_stderr")
                rm -f "$mysql_stderr"
                err_msg=$(__llm_clean_error_msg "$err_msg" --mysql)
                if [[ -n "$err_msg" ]]; then
                    __llm_error "mysql_error" "$err_msg" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
                    return 1
                fi
            fi
            rm -f "$mysql_stderr"

            # Output result (skip header line for JSON format)
            echo "$mysql_output" | tail -n +2
            ;;

        pgsql)
            if ! type -P psql >/dev/null 2>&1; then
                __llm_error "psql_not_found" "PostgreSQL client not installed" "FATAL" "false" "psql binary not found. User must install: sudo apt install postgresql-client"
                return 1
            fi

            if [[ $analyze -eq 1 ]]; then
                explain_prefix="EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)"
            else
                explain_prefix="EXPLAIN (FORMAT JSON)"
            fi

            # Execute EXPLAIN query.
            # -A unaligned + -t tuples-only: emit raw JSON text with no
            # column wrapping, no "+" continuations and no trailing footer —
            # required for the output to parse as valid JSON.
            local psql_output psql_stderr psql_exit
            psql_stderr=$(mktemp)
            psql_output=$(PGPASSWORD="$db_pass" psql -U "$db_user" -h "$db_host" -p "$db_port" -d "$database" \
                -At -c "$explain_prefix $query" 2>"$psql_stderr")
            psql_exit=$?

            # Check for errors. Only exit code failures count — NOTICE/WARNING
            # on stderr (identifier truncation, etc.) are ignored when exit=0.
            if [[ $psql_exit -ne 0 ]]; then
                local err_msg=""
                [[ -s "$psql_stderr" ]] && err_msg=$(<"$psql_stderr")
                rm -f "$psql_stderr"
                err_msg=$(__llm_clean_error_msg "$err_msg")
                if [[ -n "$err_msg" ]]; then
                    __llm_error "pgsql_error" "$err_msg" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
                else
                    __llm_error "pgsql_error" "EXPLAIN failed with exit code $psql_exit" "RETRY" "true" "Query failed. Check SQL syntax and retry"
                fi
                return 1
            fi
            rm -f "$psql_stderr"

            echo "$psql_output"
            ;;

        *)
            __llm_error "unknown_db_type" "$db_driver" "STOP" "false" "Unsupported database driver. Only mysql, mariadb, pgsql are supported"
            return 1
            ;;
    esac
}
