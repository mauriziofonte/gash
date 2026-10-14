#!/usr/bin/env bash

# Gash Module: LLM Utilities: databases
# Read-only queries on SQLite, MySQL/MariaDB and PostgreSQL.
# Sourced by lib/modules/llm.sh.
#
# Read-only is enforced in three layers, so that no single parser mistake
# turns into a write:
#
#   1. __llm_sql_guard: one statement, an allowed first keyword, no
#      client commands (\! in mysql/psql, dot-commands in sqlite3), no
#      write keywords or file/program functions anywhere outside string
#      literals and comments.
#   2. The client runs read-only: sqlite3 -readonly -safe, MySQL
#      `SET SESSION TRANSACTION READ ONLY`, PostgreSQL
#      default_transaction_read_only=on (plus a statement timeout).
#   3. Row caps are applied by the engine (wrapping or sql_select_limit),
#      never by appending text to the query.

# First keywords a query may start with.
__LLM_SQL_ALLOWED_FIRST=" SELECT WITH VALUES TABLE SHOW DESCRIBE DESC EXPLAIN PRAGMA "

# Words refused anywhere in the statement (outside strings and comments):
# data and schema changes, file and program access, session/server control.
__LLM_SQL_FORBIDDEN_WORDS=" INSERT UPDATE DELETE MERGE UPSERT DROP CREATE ALTER TRUNCATE GRANT REVOKE RENAME ATTACH DETACH VACUUM REINDEX COPY CALL HANDLER LOCK UNLOCK INTO OUTFILE DUMPFILE LOAD_FILE SYS_EXEC SYS_EVAL GET_LOCK PG_READ_FILE PG_READ_BINARY_FILE PG_LS_DIR PG_STAT_FILE PG_FILE_WRITE PG_FILE_RENAME PG_FILE_UNLINK LO_IMPORT LO_EXPORT LO_UNLINK LO_PUT LO_FROM_BYTEA DBLINK DBLINK_EXEC DBLINK_CONNECT DBLINK_SEND_QUERY QUERY_TO_XML QUERY_TO_XMLSCHEMA QUERY_TO_XML_AND_XMLSCHEMA SET_CONFIG PG_TERMINATE_BACKEND PG_CANCEL_BACKEND PG_RELOAD_CONF PG_ROTATE_LOGFILE PG_PROMOTE PG_NOTIFY LOAD_EXTENSION READFILE WRITEFILE EDIT SLEEP PG_SLEEP PG_SLEEP_FOR PG_SLEEP_UNTIL BENCHMARK "

# -----------------------------------------------------------------------------
# SQL guard
# -----------------------------------------------------------------------------

# Validate a query for read-only execution.
# DRIVER selects the lexical rules of the engine that will run the query:
#   mysql   '..' ".." `..` (a backslash inside any of them is refused: client,
#           server and sql_mode disagree on what it escapes), # and -- comments
#   pgsql   '..' E'..' (backslash escapes) ".." and $tag$..$tag$ strings
#   sqlite  '..' ".." `..` [..]
# On success stores the query without a trailing ";" in OUT_QUERY and its
# first keyword (upper case) in OUT_KIND. On failure prints the error
# envelope and returns 1.
# Usage: __llm_sql_guard DRIVER QUERY OUT_QUERY OUT_KIND
__llm_sql_guard() {
    local driver="$1" query="$2"
    local -n __sg_out_query="$3" __sg_out_kind="$4"

    local code="" c next state="code" quote="" i n=${#query} esc=0 last_semicolon=-1
    local tag rest
    for ((i = 0; i < n; i++)); do
        c="${query:i:1}"
        next="${query:i+1:1}"
        if [[ "$state" == "code" ]]; then
            case "$c" in
                "'" | '"' | '`')
                    state="quoted"
                    quote="$c"
                    esc=0
                    if [[ "$driver" == "pgsql" && "$c" == "'" && $i -gt 0 && "${query:i-1:1}" == [Ee] ]]; then
                        esc=1
                    fi
                    code+=" "
                    ;;
                "[")
                    if [[ "$driver" == "sqlite" ]]; then
                        state="quoted"
                        quote="]"
                        esc=0
                        code+=" "
                    else
                        code+="$c"
                    fi
                    ;;
                "$")
                    # PostgreSQL dollar quoting: $$..$$ or $tag$..$tag$ (not
                    # after an identifier character: a$b, $1).
                    if [[ "$driver" == "pgsql" && ($i -eq 0 || ! "${query:i-1:1}" =~ ^[A-Za-z0-9_$]$) && "${query:i}" =~ ^(\$([A-Za-z_][A-Za-z0-9_]*)?\$) ]]; then
                        tag="${BASH_REMATCH[1]}"
                        rest="${query:i+${#tag}}"
                        if [[ "$rest" != *"$tag"* ]]; then
                            __llm_error "unterminated_literal" "Dollar-quoted string $tag is not closed" "RETRY" "true" "Close every quote and /* comment */"
                            return 1
                        fi
                        rest="${rest%%"$tag"*}"
                        ((i += ${#tag} + ${#rest} + ${#tag} - 1))
                        code+=" "
                    else
                        code+="$c"
                    fi
                    ;;
                "-")
                    if [[ "$next" == "-" ]] && { [[ "$driver" != "mysql" ]] || [[ "${query:i+2:1}" =~ ^[[:space:]]?$ ]]; }; then
                        state="line"
                        code+=" "
                    else
                        code+="$c"
                    fi
                    ;;
                "#")
                    if [[ "$driver" == "mysql" ]]; then
                        state="line"
                        code+=" "
                    else
                        code+="$c"
                    fi
                    ;;
                "/")
                    if [[ "$next" == "*" ]]; then
                        if [[ "${query:i+2:1}" == "!" || "${query:i+2:2}" == [Mm]"!" ]]; then
                            __llm_error "executable_comment_blocked" "/*! ... */" "RETRY" "true" "Remove MySQL/MariaDB executable comments (/*! ... */, /*M! ... */)"
                            return 1
                        fi
                        state="block"
                        code+=" "
                        ((i++))
                    else
                        code+="$c"
                    fi
                    ;;
                "\\")
                    __llm_error "client_command_blocked" "Backslash outside a string literal" "FATAL" "false" "Client commands such as \\! are not allowed; use plain SQL"
                    return 1
                    ;;
                ";")
                    last_semicolon=$i
                    code+=";"
                    ;;
                *) code+="$c" ;;
            esac
            continue
        fi

        case "$state" in
            quoted)
                if [[ "$c" == "\\" && "$driver" == "mysql" ]]; then
                    __llm_error "backslash_in_literal" "Backslash inside a quoted string or identifier" "RETRY" "true" "MySQL clients and servers disagree on backslash escapes: write the value without backslashes (e.g. CHAR(92))"
                    return 1
                elif [[ "$c" == "\\" && $esc -eq 1 ]]; then
                    ((i++))
                elif [[ "$c" == "$quote" ]]; then
                    if [[ "$next" == "$quote" ]]; then
                        ((i++))
                    else
                        state="code"
                    fi
                fi
                ;;
            line)
                [[ "$c" == $'\n' ]] && state="code" && code+=" "
                ;;
            block)
                if [[ "$c" == "*" && "$next" == "/" ]]; then
                    state="code"
                    ((i++))
                fi
                ;;
        esac
    done

    if [[ "$state" == "quoted" || "$state" == "block" ]]; then
        __llm_error "unterminated_literal" "A string, identifier or comment is not closed" "RETRY" "true" "Close every quote and /* comment */"
        return 1
    fi

    # One statement: a single ";" is accepted only at the very end.
    local trimmed="$code"
    trimmed="${trimmed%"${trimmed##*[![:space:]]}"}"
    if [[ "$trimmed" == *";" ]]; then
        trimmed="${trimmed%;}"
        query="${query:0:last_semicolon}"
    fi
    if [[ "$trimmed" == *";"* ]]; then
        __llm_error "multiple_statements_blocked" "Only single SQL statements allowed" "FATAL" "false" "Send one statement per call (a trailing semicolon is fine)"
        return 1
    fi

    # sqlite3 runs lines starting with "." as dot-commands (.shell, .system),
    # and sqlite3 and mysql end a statement at a line holding only "go" or "/".
    local line
    while IFS= read -r line; do
        line="${line#"${line%%[![:space:]]*}"}"
        if [[ "$line" == .* ]]; then
            __llm_error "dot_command_blocked" "${line:0:24}" "FATAL" "false" "sqlite3 dot-commands are not allowed; use plain SQL"
            return 1
        fi
        line="${line%"${line##*[![:space:]]}"}"
        if [[ "${line,,}" == "go" || "$line" == "/" ]]; then
            __llm_error "multiple_statements_blocked" "Line '$line' ends a statement in the database client" "FATAL" "false" "Send one statement per call"
            return 1
        fi
    done <<<"$query"

    local upper="${trimmed^^}"
    local words=" ${upper//[^A-Z0-9_]/ } "
    local first="${upper#"${upper%%[![:space:](]*}"}"
    first="${first%%[^A-Z_]*}"

    if [[ -z "$first" || "$__LLM_SQL_ALLOWED_FIRST" != *" $first "* ]]; then
        __llm_error "write_operation_blocked" "Statement starts with '${first:-?}'" "FATAL" "false" "Only SELECT, WITH, VALUES, TABLE, SHOW, DESCRIBE, EXPLAIN and PRAGMA are allowed"
        return 1
    fi

    local w
    for w in $__LLM_SQL_FORBIDDEN_WORDS; do
        if [[ "$words" == *" $w "* ]]; then
            __llm_error "write_operation_blocked" "Statement contains '$w'" "FATAL" "false" "Only read-only queries are allowed; data-changing and file/program functions are refused"
            return 1
        fi
    done
    if [[ "$upper" =~ LOAD[[:space:]]+(DATA|XML) ]]; then
        __llm_error "write_operation_blocked" "Statement contains 'LOAD DATA'" "FATAL" "false" "Only read-only queries are allowed"
        return 1
    fi

    __sg_out_query="$query"
    __sg_out_kind="$first"
    return 0
}

# Fail fast, before any connection is resolved, on a query that no engine's
# rules accept. The authoritative check runs again once the driver is known.
# Usage: __llm_sql_precheck QUERY
__llm_sql_precheck() {
    local _pc_query _pc_kind
    __llm_sql_guard mysql "$1" _pc_query _pc_kind 2>/dev/null && return 0
    __llm_sql_guard pgsql "$1" _pc_query _pc_kind 2>/dev/null && return 0
    __llm_sql_guard sqlite "$1" _pc_query _pc_kind
}

# -----------------------------------------------------------------------------
# Connections and clients
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

    case "$_rd_drv" in
        mysql | mariadb | pgsql) ;;
        *)
            __llm_error "unknown_db_type" "$_rd_drv" "STOP" "true" "Unsupported database driver. Only mysql, mariadb, pgsql are supported"
            return 1
            ;;
    esac
}

# Check if sqlite3 is available.
# Usage: __llm_has_sqlite
__llm_has_sqlite() {
    type -P sqlite3 >/dev/null 2>&1
}

# Validate an SQLite database path and store the resolved path in OUT.
# Usage: __llm_sqlite_file FILE OUT
__llm_sqlite_file() {
    local -n __sf_out="$2"
    local file="${1-}" safe_path magic=""

    if ! __llm_has_sqlite; then
        __llm_error "sqlite_not_found" "SQLite3 CLI not installed" "FATAL" "false" "sqlite3 binary not found. Install with: sudo apt install sqlite3"
        return 1
    fi
    if [[ -z "$file" ]]; then
        __llm_error "missing_sqlite_file" "" "RETRY" "true" "Provide SQLite file path with -f option"
        return 1
    fi
    safe_path="$(__llm_validate_path "$file")" || return 1
    if [[ ! -f "$safe_path" ]]; then
        __llm_error "sqlite_file_not_found" "$safe_path" "RETRY" "true" "SQLite file does not exist. Check the path"
        return 1
    fi
    # Magic header "SQLite format 3\0" (no dependency on file(1)).
    IFS= read -r -n 15 magic <"$safe_path" 2>/dev/null || true
    if [[ "$magic" != "SQLite format 3" ]]; then
        __llm_error "invalid_sqlite_file" "$safe_path is not a valid SQLite database" "FATAL" "false" "Provide a valid SQLite database file"
        return 1
    fi
    __sf_out="$safe_path"
}

# sqlite3 -safe exists since 3.37; checked once.
__llm_sqlite_safe_flag() {
    if [[ -z "${__LLM_SQLITE_SAFE-}" ]]; then
        if sqlite3 -safe :memory: 'SELECT 1' >/dev/null 2>&1; then
            __LLM_SQLITE_SAFE="-safe"
        else
            __LLM_SQLITE_SAFE="none"
        fi
    fi
    [[ "$__LLM_SQLITE_SAFE" == "-safe" ]] && printf '%s' "-safe"
    return 0
}

# Run SQL read-only on an SQLite file; print a JSON array.
# Usage: __llm_sqlite_run FILE SQL
__llm_sqlite_run() {
    local file="$1" sql="$2" out err rc=0 safe
    safe="$(__llm_sqlite_safe_flag)"
    err="$(mktemp)"
    # SQL on stdin, never argv: an argument starting with "." would be a
    # dot-command.
    # shellcheck disable=SC2086 # $safe is empty or one flag
    out="$(printf '%s\n;\n' "$sql" | sqlite3 -bail -readonly $safe -json "$file" 2>"$err")" || rc=$?
    if ((rc != 0)) || [[ -s "$err" ]]; then
        local msg
        msg="$(__llm_clean_error_msg "$(<"$err")")"
        rm -f "$err"
        __llm_error "sqlite_error" "${msg:-sqlite3 exited with status $rc}" "RETRY" "true" "Fix the SQL and retry"
        return 1
    fi
    rm -f "$err"
    printf '%s\n' "${out:-[]}"
}

# mysql or mariadb client path.
__llm_mysql_bin() {
    type -P mariadb 2>/dev/null || type -P mysql 2>/dev/null || {
        __llm_error "mysql_not_found" "MySQL/MariaDB client not installed" "FATAL" "false" "mysql/mariadb binary not found. User must install: sudo apt install mariadb-client"
        return 1
    }
}

# Run SQL on MySQL/MariaDB in a read-only session. Connection details come
# from the caller's db_user, db_pass, db_host, db_port, database.
# MODE tsv: JSON array from --batch output (row cap via sql_select_limit).
# MODE raw: --batch --raw output without the header row (EXPLAIN JSON).
# Usage: __llm_mysql_run MODE SQL [ROW_CAP]
__llm_mysql_run() {
    local mode="$1" sql="$2" cap="${3-}" bin out err rc=0
    bin="$(__llm_mysql_bin)" || return 1

    local script="SET SESSION TRANSACTION READ ONLY;"$'\n'
    [[ -n "$cap" ]] && script+="SET SESSION sql_select_limit = ${cap};"$'\n'
    script+="${sql}"$'\n;\n'

    local -a flags=(--batch --default-character-set=utf8mb4)
    [[ "$mode" == "raw" ]] && flags+=(--raw --skip-column-names)

    err="$(mktemp)"
    # Credentials through a process-substitution file, never argv.
    out="$(printf '%s' "$script" | "$bin" --defaults-extra-file=<(__llm_mysql_client_ini "$db_user" "$db_pass" "$db_host" "$db_port") \
        "${flags[@]}" "$database" 2>"$err")" || rc=$?
    local msg
    msg="$(__llm_clean_error_msg "$(<"$err")" --mysql)"
    rm -f "$err"
    if ((rc != 0)) || [[ -n "$msg" ]]; then
        __llm_error "mysql_error" "${msg:-mysql exited with status $rc}" "RETRY" "true" "Fix the SQL and retry"
        return 1
    fi

    if [[ "$mode" == "raw" ]]; then
        printf '%s\n' "$out"
    else
        printf '%s\n' "$out" | __llm_tsv_to_json mysql
    fi
}

# Run SQL on PostgreSQL in a read-only transaction with a statement timeout.
# Connection details come from the caller (see __llm_mysql_run).
# MODE json: SQL already returns one JSON value; MODE tsv: table output.
# Usage: __llm_pgsql_run MODE SQL
__llm_pgsql_run() {
    local mode="$1" sql="$2" out err rc=0
    if ! type -P psql >/dev/null 2>&1; then
        __llm_error "psql_not_found" "PostgreSQL client not installed" "FATAL" "false" "psql binary not found. User must install: sudo apt install postgresql-client"
        return 1
    fi

    # Statement timeout in seconds (GASH_DB_TIMEOUT, default 30).
    local timeout="${GASH_DB_TIMEOUT:-30}"
    [[ "$timeout" =~ ^[0-9]+$ ]] || timeout=30

    local -a flags=(-X -v ON_ERROR_STOP=1)
    if [[ "$mode" == "json" ]]; then
        flags+=(-At)
    else
        flags+=(-P footer=off -A -F $'\t')
    fi

    err="$(mktemp)"
    # SQL on stdin (-f -), never argv: /proc/<pid>/cmdline is world-readable.
    # A ";" on its own line ends the statement even after a -- comment.
    out="$(printf '%s\n;\n' "$sql" | PGPASSWORD="$db_pass" PGCONNECT_TIMEOUT=10 \
        PGOPTIONS="-c default_transaction_read_only=on -c standard_conforming_strings=on -c statement_timeout=${timeout}s" \
        psql -U "$db_user" -h "$db_host" -p "$db_port" -d "$database" "${flags[@]}" -f - 2>"$err")" || rc=$?
    # NOTICE/WARNING lines also land on stderr: only the exit status counts.
    if ((rc != 0)); then
        local msg
        msg="$(__llm_clean_error_msg "$(<"$err")")"
        rm -f "$err"
        __llm_error "pgsql_error" "${msg:-psql exited with status $rc}" "RETRY" "true" "Fix the SQL and retry"
        return 1
    fi
    rm -f "$err"

    if [[ "$mode" == "json" ]]; then
        printf '%s\n' "${out:-[]}"
    else
        printf '%s\n' "$out" | __llm_tsv_to_json raw
    fi
}

# Run a guarded query on the backend picked by the caller's sqlite_file or
# db_driver, capping result rows at CAP. Prints a JSON array.
# Usage: __llm_db_run KIND SQL CAP
__llm_db_run() {
    local kind="$1" sql="$2" cap="$3" nl=$'\n'
    local wrappable=0
    [[ " SELECT WITH VALUES TABLE " == *" $kind "* ]] && wrappable=1

    if [[ -n "${sqlite_file-}" ]]; then
        ((wrappable)) && sql="SELECT * FROM (${nl}${sql}${nl}) LIMIT ${cap}"
        __llm_sqlite_run "$sqlite_file" "$sql"
        return
    fi

    case "$db_driver" in
        mysql | mariadb)
            __llm_mysql_run tsv "$sql" "$cap"
            ;;
        pgsql)
            if ((wrappable)); then
                # PostgreSQL serializes the rows itself: native types, and
                # tabs or newlines inside values survive.
                __llm_pgsql_run json "SELECT COALESCE(jsonb_agg(to_jsonb(__gash_row)), '[]'::jsonb) FROM (SELECT * FROM (${nl}${sql}${nl}) __gash_q LIMIT ${cap}) __gash_row"
            else
                __llm_pgsql_run tsv "$sql"
            fi
            ;;
    esac
}

# Parse the flags shared by llm_db_* into the caller's locals: positional,
# database, connection, sqlite_file, rows, analyze. ALLOWED lists the flags
# this function accepts besides -d/-c/-f.
# Usage: __llm_db_parse_args "ALLOWED" "$@"
__llm_db_parse_args() {
    local allowed=" -d --database -c --connection -f --file $1 "
    shift
    while (($# > 0)); do
        case "$1" in
            -*)
                if [[ "$allowed" != *" $1 "* ]]; then
                    __llm_error "unknown_option" "$1" "RETRY" "true" "Valid options:${allowed}"
                    return 1
                fi
                if [[ "$1" == "--analyze" ]]; then
                    analyze=1
                    shift
                    continue
                fi
                if (($# < 2)); then
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                fi
                case "$1" in
                    -d | --database) database="$2" ;;
                    -c | --connection) connection="$2" ;;
                    -f | --file) sqlite_file="$2" ;;
                    -r | --rows | -n | --limit) rows="$2" ;;
                esac
                shift 2
                ;;
            *)
                if [[ -n "$positional" ]]; then
                    __llm_error "unexpected_argument" "$1" "RETRY" "true" "Quote the whole query as one argument"
                    return 1
                fi
                positional="$1"
                shift
                ;;
        esac
    done
}

# Resolve the target of a llm_db_* call: SQLite file (sqlite_file) or named
# connection (sets db_driver, db_user, db_pass, db_host, db_port, database
# in the caller).
__llm_db_target() {
    if [[ -n "$sqlite_file" ]]; then
        __llm_sqlite_file "$sqlite_file" sqlite_file || return 1
        db_driver="sqlite"
        return 0
    fi
    local _resolved_db
    __llm_resolve_db "$connection" "$database" db_driver db_user db_pass db_host db_port _resolved_db || return 1
    database="$_resolved_db"
}

# Guard driver name for a resolved target.
__llm_db_guard_driver() {
    case "$db_driver" in
        mysql | mariadb) printf 'mysql' ;;
        pgsql) printf 'pgsql' ;;
        *) printf 'sqlite' ;;
    esac
}

# -----------------------------------------------------------------------------
# Public functions
# -----------------------------------------------------------------------------

# Execute a read-only database query with JSON output.
# Usage: llm_db_query <query> [-d DATABASE] [-c CONNECTION] [-f SQLITE_FILE] [-r|-n|--limit ROWS]
# Example: llm_db_query "SELECT * FROM users" -c myconn
# Example: llm_db_query "SELECT * FROM users" -f /path/to/db.sqlite
llm_db_query() {
    if needs_help "llm_db_query" "llm_db_query <query> [-c CONNECTION] [-f SQLITE_FILE] [-n ROWS]" "Execute read-only database query (JSON envelope with timing). Use -c for MySQL/PostgreSQL, -f for SQLite" "${1-}"; then
        return 0
    fi

    # Build explain args: copy "$@" but strip the row cap (not accepted by explain)
    local __GASH_DB_EXPLAIN_ARGS=()
    local _qw_args=("$@") _qw_i=0
    while [[ $_qw_i -lt ${#_qw_args[@]} ]]; do
        case "${_qw_args[$_qw_i]}" in
            -r | --rows | -n | --limit) _qw_i=$((_qw_i + 2)) ;;
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
    local positional="" database="" connection="default" sqlite_file="" rows=100 analyze=0
    __llm_db_parse_args "-r --rows -n --limit" "$@" || return 1

    if [[ -z "$positional" ]]; then
        __llm_error "missing_query" "" "RETRY" "true" "Provide a SQL query as first argument"
        return 1
    fi
    __llm_validate_uint "rows" "$rows" || return 1

    # Validate with the strictest literal rules before touching any config,
    # then again with the driver's own rules once it is known.
    local sql sql_kind
    __llm_sql_precheck "$positional" || return 1

    local db_driver="" db_user="" db_pass="" db_host="" db_port=""
    __llm_db_target || return 1
    __llm_sql_guard "$(__llm_db_guard_driver)" "$positional" sql sql_kind || return 1

    __llm_db_run "$sql_kind" "$sql" "$rows"
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
    local positional="" database="" connection="default" sqlite_file="" rows=0 analyze=0
    __llm_db_parse_args "" "$@" || return 1

    local db_driver="" db_user="" db_pass="" db_host="" db_port=""
    __llm_db_target || return 1

    local result
    case "$db_driver" in
        sqlite)
            result="$(__llm_db_run SELECT "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name" 10000)" || return 1
            ;;
        mysql | mariadb)
            result="$(__llm_db_run SELECT "SELECT table_name AS name FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_name" 10000)" || return 1
            ;;
        pgsql)
            result="$(__llm_db_run SELECT "SELECT tablename AS name FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename" 10000)" || return 1
            ;;
    esac
    printf '%s' "$result" | jq -c '[.[] | (.name // .NAME // .TABLE_NAME // .table_name)]'
}

# Show schema for a table.
# Usage: llm_db_schema <table> [-c CONNECTION] [-f SQLITE_FILE]
llm_db_schema() {
    if needs_help "llm_db_schema" "llm_db_schema <table> [-c CONNECTION] [-f SQLITE_FILE]" "Show table schema (JSON). Use -c for MySQL/PostgreSQL, -f for SQLite" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_db_envelope __llm_db_schema_impl "$@"
}

# Table names are interpolated into SQL: letters, digits and underscores only.
__llm_db_check_table() {
    if [[ -z "${1-}" ]]; then
        __llm_error "missing_table" "" "RETRY" "true" "Provide table name as first argument"
        return 1
    fi
    if [[ ! "$1" =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
        __llm_error "invalid_table_name" "Table name must be alphanumeric with underscores" "RETRY" "true" "Use only letters, numbers, and underscores"
        return 1
    fi
}

__llm_db_schema_impl() {
    local positional="" database="" connection="default" sqlite_file="" rows=0 analyze=0
    __llm_db_parse_args "" "$@" || return 1
    __llm_db_check_table "$positional" || return 1
    local table="$positional"

    local db_driver="" db_user="" db_pass="" db_host="" db_port=""
    __llm_db_target || return 1

    case "$db_driver" in
        sqlite)
            __llm_db_run SELECT "SELECT cid, name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('$table')" 10000
            ;;
        mysql | mariadb)
            __llm_db_run SELECT "SELECT column_name AS name, column_type AS type, is_nullable AS nullable, column_key AS \`key\`, column_default AS \`default\` FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = '$table' ORDER BY ordinal_position" 10000
            ;;
        pgsql)
            __llm_db_run SELECT "SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_name = '$table' AND table_schema = 'public' ORDER BY ordinal_position" 10000
            ;;
    esac
}

# Get sample rows from a table.
# Usage: llm_db_sample <table> [-c CONNECTION] [-f SQLITE_FILE] [-n|--limit N]
llm_db_sample() {
    if needs_help "llm_db_sample" "llm_db_sample <table> [-c CONNECTION] [-f SQLITE_FILE] [-n N]" "Get sample rows from table (default 5 rows). Use -c for MySQL/PostgreSQL, -f for SQLite" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_db_envelope __llm_db_sample_impl "$@"
}

__llm_db_sample_impl() {
    local positional="" database="" connection="default" sqlite_file="" rows=5 analyze=0
    __llm_db_parse_args "-n --limit" "$@" || return 1
    __llm_db_check_table "$positional" || return 1
    __llm_validate_uint "limit" "$rows" || return 1

    local db_driver="" db_user="" db_pass="" db_host="" db_port=""
    __llm_db_target || return 1

    local quoted="\"$positional\""
    [[ "$db_driver" == mysql || "$db_driver" == mariadb ]] && quoted="\`$positional\`"
    __llm_db_run SELECT "SELECT * FROM $quoted" "$rows"
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
    local positional="" database="" connection="default" sqlite_file="" rows=0 analyze=0
    __llm_db_parse_args "--analyze" "$@" || return 1

    if [[ -z "$positional" ]]; then
        __llm_error "missing_query" "" "RETRY" "true" "Provide a SQL query to analyze as first argument"
        return 1
    fi

    local sql sql_kind
    __llm_sql_precheck "$positional" || return 1

    local db_driver="" db_user="" db_pass="" db_host="" db_port=""
    __llm_db_target || return 1
    __llm_sql_guard "$(__llm_db_guard_driver)" "$positional" sql sql_kind || return 1

    local plan
    case "$db_driver" in
        sqlite)
            # sqlite3 renders EXPLAIN QUERY PLAN as a text tree in every mode.
            plan="$(__llm_sqlite_run "$sqlite_file" "EXPLAIN QUERY PLAN $sql")" || return 1
            ;;
        mysql | mariadb)
            local prefix="EXPLAIN FORMAT=JSON"
            if ((analyze)); then
                # MariaDB: ANALYZE FORMAT=JSON; MySQL 8.0.18+: EXPLAIN ANALYZE
                local version
                version="$(__llm_mysql_run raw "SELECT VERSION()")" || return 1
                if [[ "$version" == *MariaDB* ]]; then
                    prefix="ANALYZE FORMAT=JSON"
                else
                    prefix="EXPLAIN ANALYZE"
                fi
            fi
            plan="$(__llm_mysql_run raw "$prefix $sql")" || return 1
            ;;
        pgsql)
            local prefix="EXPLAIN (FORMAT JSON)"
            ((analyze)) && prefix="EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)"
            plan="$(__llm_pgsql_run json "$prefix $sql")" || return 1
            ;;
    esac

    # Always an array: JSON plans as they come, text trees as [{plan: TEXT}].
    if jq -e . >/dev/null 2>&1 <<<"$plan"; then
        jq -c 'if type == "array" then . else [.] end' <<<"$plan"
    else
        jq -cRs '[{plan: (. | rtrimstr("\n"))}]' <<<"$plan"
    fi
}
