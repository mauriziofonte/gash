#!/usr/bin/env bash

# Gash Module: LLM Utilities: files
# llm_tree, llm_find, llm_grep, llm_config.
# Sourced by lib/modules/llm.sh.

# -----------------------------------------------------------------------------
# Directory & File Navigation
# -----------------------------------------------------------------------------

# Output a compact directory tree in JSON or text format.
# JSON mode (default) builds a NESTED tree up to --depth N with proper JSON
# escaping. Adding --stats enriches each node with size (cumulative for
# directories) and children_count.
# Text mode emits an indented listing capped at 200 entries total.
#
# Usage: llm_tree [--text] [--depth N] [--stats] [PATH]
# Example: llm_tree src/
# Example: llm_tree --text --depth 2 .
# Example: llm_tree --depth 3 --stats lib/    # nested JSON with per-node size
llm_tree() {
    if needs_help "llm_tree" "llm_tree [--text] [--depth N] [--stats] [PATH]" "Compact directory tree (JSON default, --text for indented, --stats for size/count)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_tree_impl "$@"
}

__llm_tree_impl() {
    local text_mode=0
    local max_depth="" # empty = default per mode (text=3, JSON=1)
    local with_stats=0
    local target_path="."

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --text)
                text_mode=1
                shift
                ;;
            --depth | --max-depth)
                max_depth="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            --stats)
                with_stats=1
                shift
                ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use --text, --depth N, or --stats"
                return 1
                ;;
            *)
                target_path="$1"
                shift
                ;;
        esac
    done

    # Apply per-mode depth defaults (JSON: 1 for backward-compat; text: 3)
    if [[ -z "$max_depth" ]]; then
        [[ $text_mode -eq 1 ]] && max_depth=3 || max_depth=1
    fi
    if ! [[ "$max_depth" =~ ^[0-9]+$ ]] || [[ "$max_depth" -lt 1 ]]; then
        __llm_error "invalid_depth" "$max_depth" "RETRY" "true" "Provide a positive integer"
        return 1
    fi

    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    if [[ ! -d "$safe_path" ]]; then
        __llm_error "not_a_directory" "$safe_path" "RETRY" "true" "Provide a valid directory path"
        return 1
    fi

    if [[ $text_mode -eq 1 ]]; then
        # Text mode: find with indentation, 200-entry cap
        find "$safe_path" -maxdepth "$max_depth" \
            -name 'node_modules' -prune -o \
            -name 'vendor' -prune -o \
            -name '.git' -prune -o \
            -name '__pycache__' -prune -o \
            -name '.cache' -prune -o \
            -type f -print -o -type d -print 2>/dev/null |
            sort |
            head -n 200 |
            while IFS= read -r item; do
                local rel="${item#$safe_path}"
                rel="${rel#/}"
                local depth
                depth=$(echo "$rel" | tr -cd '/' | wc -c)
                local indent=""
                for ((i = 0; i < depth; i++)); do indent+="  "; done
                local name
                name="$(basename "$item")"
                if [[ -d "$item" ]]; then
                    echo "${indent}${name}/"
                else
                    echo "${indent}${name}"
                fi
            done
        return 0
    fi

    # JSON mode: nested tree via awk aggregation.
    # find emits `<relpath>\t<type>\t<size>` lines; awk lists the entries
    # down to max_depth (at most 500 of them, flagged with "truncated") and
    # nests them in one END-phase recursive emission.
    # With --stats the walk has no depth limit, so a directory's size and
    # children_count are its real cumulative size and direct-child count,
    # also where the listing stops (depth limit or entry cap).
    # Paths containing tab/newline would corrupt the TSV transport (wrong
    # names silently attributed): exclude them instead of emitting bad data.
    # `|| true`: an unreadable subdir must not kill a `set -euo pipefail`
    # caller.
    local nl_glob tab_glob
    nl_glob=$(printf '*\n*')
    tab_glob=$(printf '*\t*')
    local -a depth_args=(-maxdepth "$max_depth")
    [[ $with_stats -eq 1 ]] && depth_args=()

    local esc_root
    esc_root=$(__gash_json_escape "$safe_path")

    {
        find "$safe_path" -mindepth 1 "${depth_args[@]}" \
            -name 'node_modules' -prune -o \
            -name 'vendor' -prune -o \
            -name '.git' -prune -o \
            -name '__pycache__' -prune -o \
            -name '.cache' -prune -o \
            \( -type f -o -type d -o -type l \) \
            ! -path "$nl_glob" ! -path "$tab_glob" \
            -printf '%P\t%y\t%s\n' 2>/dev/null || true
    } | "${__GASH_AWK:-awk}" -F'\t' \
        -v root="$esc_root" \
        -v max_depth="$max_depth" \
        -v with_stats="$with_stats" \
        -v cap=500 \
        '
        function esc(s,   r) {
            r = s
            gsub(/\\/, "\\\\", r)
            gsub(/"/,  "\\\"", r)
            gsub(/\n/, "\\n",  r)
            gsub(/\r/, "\\r",  r)
            gsub(/\t/, "\\t",  r)
            return r
        }
        function basename_of(p,   n, a) {
            if (p == "") return ""
            n = split(p, a, "/")
            return a[n]
        }
        NF < 3 { next }
        {
            rel = $1
            n = split(rel, part, "/")
            par = ""
            for (i = 1; i < n; i++) par = (i == 1 ? part[1] : par "/" part[i])

            # Real totals: every non-directory adds its size to each listed
            # ancestor; every entry counts as a child of its parent.
            if ($2 != "d") {
                sz = $3 + 0
                total[""] += sz
                anc = ""
                for (i = 1; i < n && i <= max_depth; i++) {
                    anc = (i == 1 ? part[1] : anc "/" part[i])
                    total[anc] += sz
                }
            }
            nchild[par]++

            if (n > max_depth) next
            if (listed >= cap) { truncated = 1; next }
            listed++
            # %y: f/d/l (anything else is reported as a file)
            types[rel] = ($2 == "d" ? "directory" : ($2 == "l" ? "link" : "file"))
            sizes[rel] = $3 + 0
            child_list[par, ++child_cnt[par]] = rel
        }
        function emit_node(rel, depth_rem,   i, n, first) {
            printf "{\"name\":\"%s\",\"type\":\"%s\"", esc(basename_of(rel)), types[rel]
            if (with_stats) {
                if (types[rel] == "directory") {
                    printf ",\"size\":%.0f,\"children_count\":%d", total[rel], nchild[rel]
                } else {
                    printf ",\"size\":%.0f", sizes[rel]
                }
            }
            if (types[rel] == "directory" && depth_rem > 1 && child_cnt[rel] > 0) {
                printf ",\"children\":["
                first = 1
                n = child_cnt[rel]
                for (i = 1; i <= n; i++) {
                    if (!first) printf ","
                    first = 0
                    emit_node(child_list[rel, i], depth_rem - 1)
                }
                printf "]"
            }
            printf "}"
        }
        END {
            printf "{\"path\":\"%s\",\"type\":\"directory\"", root
            if (with_stats) {
                printf ",\"size\":%.0f,\"children_count\":%d", total[""], nchild[""]
            }
            if (truncated) printf ",\"truncated\":true"
            printf ",\"children\":["
            n = child_cnt[""]
            first = 1
            for (i = 1; i <= n; i++) {
                if (!first) printf ","
                first = 0
                emit_node(child_list["", i], max_depth)
            }
            printf "]}\n"
        }
        '
}

# Find files by pattern, optimized for common use cases.
# Usage: llm_find <pattern> [PATH] [--type f|d] [--contains REGEX]
# Example: llm_find "*.php" src/
# Example: llm_find --contains "class.*Controller" .
llm_find() {
    if needs_help "llm_find" "llm_find <pattern> [PATH] [--type f|d] [--contains REGEX]" "Find files by pattern (ignores node_modules, vendor, .git)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_find_impl "$@"
}

__llm_find_impl() {
    local pattern=""
    local target_path="."
    local file_type="f"
    local contains_regex=""
    local limit=100

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --type)
                file_type="${2-f}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            --contains)
                contains_regex="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -n | --limit)
                limit="${2-100}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use --type, --contains, or --limit"
                return 1
                ;;
            *)
                if [[ -z "$pattern" ]]; then
                    pattern="$1"
                else
                    target_path="$1"
                fi
                shift
                ;;
        esac
    done

    __llm_validate_uint "limit" "$limit" || return 1

    # Validate path
    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    # Zero matches (or SIGPIPE from head) is a valid outcome, not an error:
    # never let pipefail+errexit kill a headless caller.
    if [[ -n "$contains_regex" ]]; then
        # Search by content; secret files are never opened.
        if __llm_is_secret_file "$safe_path"; then
            __llm_error "secret_file_blocked" "$safe_path" "FATAL" "false" "This file contains secrets and cannot be searched"
            return 1
        fi
        local -a secret_ex=()
        if __llm_has_rg; then
            __llm_secret_excludes rg secret_ex
            rg -l --no-heading --color=never \
                --glob '!node_modules' --glob '!vendor' --glob '!.git' "${secret_ex[@]}" \
                -e "$contains_regex" -- "$safe_path" 2>/dev/null | head -n "$limit" || true
        else
            __llm_secret_excludes grep secret_ex
            grep -rl --include='*.php' --include='*.js' --include='*.ts' \
                --include='*.py' --include='*.go' --include='*.java' \
                --exclude-dir=node_modules --exclude-dir=vendor --exclude-dir=.git "${secret_ex[@]}" \
                -e "$contains_regex" -- "$safe_path" 2>/dev/null | head -n "$limit" || true
        fi
    else
        # Search by filename pattern. Paths with embedded newlines would be
        # unparseable in line-oriented output: exclude them.
        local nl_glob
        nl_glob=$(printf '*\n*')
        find "$safe_path" \
            -name 'node_modules' -prune -o \
            -name 'vendor' -prune -o \
            -name '.git' -prune -o \
            -name '__pycache__' -prune -o \
            -type "$file_type" -name "$pattern" ! -path "$nl_glob" -print 2>/dev/null |
            head -n "$limit" || true
    fi
    return 0
}

# Grep with structured output, optimized for code search.
# Usage: llm_grep <pattern> [PATH] [--ext EXT1,EXT2] [--context N]
# Example: llm_grep "TODO|FIXME" src/ --ext php,js
llm_grep() {
    if needs_help "llm_grep" "llm_grep <pattern> [PATH] [--ext EXT1,EXT2] [--context N]" "Search code with file:line:content output" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_grep_impl "$@"
}

__llm_grep_impl() {
    local pattern=""
    local target_path="."
    local extensions=""
    local context=0
    local limit=100

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --ext)
                extensions="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            --context | -C)
                context="${2-0}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -n | --limit)
                limit="${2-100}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use --ext, --context, or --limit"
                return 1
                ;;
            *)
                if [[ -z "$pattern" ]]; then
                    pattern="$1"
                else
                    target_path="$1"
                fi
                shift
                ;;
        esac
    done

    if [[ -z "$pattern" ]]; then
        __llm_error "missing_pattern" "" "RETRY" "true" "Provide a search pattern as first argument"
        return 1
    fi

    __llm_validate_uint "limit" "$limit" || return 1

    # Validate path
    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1
    if __llm_is_secret_file "$safe_path"; then
        __llm_error "secret_file_blocked" "$safe_path" "FATAL" "false" "This file contains secrets and cannot be searched"
        return 1
    fi

    if __llm_has_rg; then
        local rg_args=(-n --no-heading --color=never)
        rg_args+=(--glob '!node_modules' --glob '!vendor' --glob '!.git')
        local -a secret_ex=()
        __llm_secret_excludes rg secret_ex
        rg_args+=("${secret_ex[@]}")

        if [[ -n "$extensions" ]]; then
            IFS=',' read -ra exts <<<"$extensions"
            for ext in "${exts[@]}"; do
                rg_args+=(--glob "*.$ext")
            done
        fi

        [[ $context -gt 0 ]] && rg_args+=(-C "$context")

        # Zero matches (or SIGPIPE from head) is a valid outcome, not an
        # error: never let pipefail+errexit kill a headless caller.
        rg "${rg_args[@]}" -e "$pattern" -- "$safe_path" 2>/dev/null | head -n "$limit" || true
        return 0
    else
        local grep_args=(-rn --color=never)
        grep_args+=(--exclude-dir=node_modules --exclude-dir=vendor --exclude-dir=.git)
        local -a secret_ex=()
        __llm_secret_excludes grep secret_ex
        grep_args+=("${secret_ex[@]}")

        if [[ -n "$extensions" ]]; then
            IFS=',' read -ra exts <<<"$extensions"
            for ext in "${exts[@]}"; do
                grep_args+=(--include="*.$ext")
            done
        fi

        [[ $context -gt 0 ]] && grep_args+=(-C "$context")

        grep "${grep_args[@]}" -e "$pattern" -- "$safe_path" 2>/dev/null | head -n "$limit" || true
        return 0
    fi
}

# Read a config file (NOT .env for security).
# Usage: llm_config <file>
# Supports: .json, .yaml, .yml
llm_config() {
    if needs_help "llm_config" "llm_config <file>" "Read config file (JSON/YAML, NOT .env)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_config_impl "$@"
}

__llm_config_impl() {
    local file="${1-}"

    if [[ -z "$file" ]]; then
        __llm_error "missing_file" "" "RETRY" "true" "Provide a file path as argument"
        return 1
    fi

    # Security: Block .env files
    if __llm_is_secret_file "$file"; then
        __llm_error "secret_file_blocked" "Cannot read .env or credential files" "FATAL" "false" "This file contains secrets and cannot be read"
        return 1
    fi

    local safe_path
    safe_path="$(__llm_validate_path "$file")" || return 1

    if [[ ! -f "$safe_path" ]]; then
        __llm_error "file_not_found" "$safe_path" "RETRY" "true" "Check the file path and try again"
        return 1
    fi

    case "$safe_path" in
        *.json)
            cat "$safe_path"
            ;;
        *.yaml | *.yml)
            if __llm_has_jq && type -P yq >/dev/null 2>&1; then
                yq -o=json "$safe_path"
            else
                cat "$safe_path"
            fi
            ;;
        *)
            cat "$safe_path"
            ;;
    esac
}
