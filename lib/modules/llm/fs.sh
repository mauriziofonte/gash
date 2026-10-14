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
    if __llm_needs_help "llm_tree" "llm_tree [--text] [--depth N] [--stats] [PATH]" "Compact directory tree (JSON default, --text for indented, --stats for size/count)" "$@"; then
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
    __llm_validate_uint "depth" "$max_depth" || return 1
    if ((10#$max_depth < 1)); then
        __llm_error "invalid_depth" "$max_depth" "RETRY" "true" "Provide a positive integer"
        return 1
    fi
    max_depth=$((10#$max_depth))

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

# Find files by name pattern, optionally only those whose content matches.
# Usage: llm_find <pattern> [PATH] [--type f|d] [--contains REGEX]
# Example: llm_find "*.php" src/
# Example: llm_find "*.php" . --contains "class.*Controller"
llm_find() {
    if __llm_needs_help "llm_find" "llm_find <pattern> [PATH] [--type f|d] [--contains REGEX]" "Find files by pattern (ignores node_modules, vendor, .git)" "$@"; then
        return 0
    fi

    __gash_no_history __llm_find_impl "$@"
}

__llm_find_impl() {
    local pattern=""
    local target_path="." path_set=""
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
                elif [[ -z "$path_set" ]]; then
                    target_path="$1"
                    path_set=1
                else
                    __llm_error "unexpected_argument" "$1" "RETRY" "true" "Pass one PATTERN and at most one PATH"
                    return 1
                fi
                shift
                ;;
        esac
    done

    if [[ -z "$pattern" ]]; then
        __llm_error "missing_pattern" "" "RETRY" "true" "Provide a name pattern as first argument, e.g. '*.php' or '*'"
        return 1
    fi
    if [[ "$file_type" != f && "$file_type" != d ]]; then
        __llm_error "invalid_type" "$file_type" "RETRY" "true" "Use --type f (files) or --type d (directories)"
        return 1
    fi
    if [[ -n "$contains_regex" && "$file_type" == d ]]; then
        __llm_error "invalid_type" "--type d with --contains" "RETRY" "true" "--contains searches file contents; drop --type d"
        return 1
    fi
    __llm_validate_uint "limit" "$limit" || return 1

    # Validate path
    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    # Zero matches (or SIGPIPE from head) is a valid outcome, not an error:
    # never let pipefail+errexit kill a headless caller.
    if [[ -n "$contains_regex" ]]; then
        # Search by content, in the files the name search would list: the
        # same pattern, prunes and policy (hidden and gitignored files
        # included) with rg and with the grep fallback. Secret files are
        # never opened.
        if __llm_is_secret_file "$safe_path"; then
            __llm_error "secret_file_blocked" "$safe_path" "FATAL" "false" "This file contains secrets and cannot be searched"
            return 1
        fi
        if [[ -f "$safe_path" ]]; then
            # shellcheck disable=SC2053 # PATTERN is a glob on purpose
            [[ "${safe_path##*/}" == $pattern ]] || return 0
        fi
        local -a secret_ex=()
        if __llm_has_rg; then
            __llm_secret_excludes rg secret_ex
            rg -l --no-heading --color=never --no-messages --no-ignore --hidden --glob "$pattern" \
                --glob '!node_modules' --glob '!vendor' --glob '!.git' --glob '!__pycache__' "${secret_ex[@]}" \
                -e "$contains_regex" -- "$safe_path" 2>/dev/null | head -n "$limit" || true
        elif [[ -d "$safe_path" ]]; then
            __llm_secret_excludes grep secret_ex
            __llm_grep_tree "$safe_path" "" "$pattern" -l -I "${secret_ex[@]}" \
                -e "$contains_regex" | head -n "$limit" || true
        else
            grep -l -I -e "$contains_regex" -- "$safe_path" 2>/dev/null | head -n "$limit" || true
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
    if __llm_needs_help "llm_grep" "llm_grep <pattern> [PATH] [--ext EXT1,EXT2] [--context N]" "Search code with file:line:content output" "$@"; then
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

        if [[ -d "$safe_path" ]]; then
            local -a tree_args=(-n --color=never "${secret_ex[@]}")
            [[ $context -gt 0 ]] && tree_args+=(-C "$context")
            __llm_grep_tree "$safe_path" "$extensions" "" "${tree_args[@]}" -e "$pattern" | head -n "$limit" || true
        else
            grep "${grep_args[@]}" -e "$pattern" -- "$safe_path" 2>/dev/null | head -n "$limit" || true
        fi
        return 0
    fi
}

# grep fallback for a directory when rg is missing. find walks the tree and
# prunes noise and secret directories by path: grep --exclude-dir matches
# one name only, so a rule such as ".config/gcloud" needs find. Secret file
# names are skipped here and by the caller's --exclude arguments. NAME is
# a -name glob (llm_find), which also prunes __pycache__ as its name
# search does.
# Usage: __llm_grep_tree DIR EXTS(comma list, may be empty) NAME(may be empty) GREP_ARG...
__llm_grep_tree() {
    local root="$1" ext_csv="$2" name_glob="$3" dir pattern ext
    shift 3
    local -a prune=(-name node_modules -o -name vendor -o -name .git) names=() skip=() list=()
    if [[ -n "$name_glob" ]]; then
        prune+=(-o -name __pycache__)
        names=(-name "$name_glob")
    fi
    for dir in "${__LLM_SECRET_DIRS[@]}"; do
        prune+=(-o -ipath "*/$dir")
    done
    for pattern in "${__LLM_SECRET_PATTERNS[@]}"; do
        skip+=(! -iname "$pattern")
    done
    if [[ -n "$ext_csv" ]]; then
        IFS=',' read -ra list <<<"$ext_csv"
        for ext in "${list[@]}"; do
            ((${#names[@]} == 0)) || names+=(-o)
            names+=(-name "*.$ext")
        done
        names=(\( "${names[@]}" \))
    fi
    # /dev/null keeps grep from reading stdin when nothing was found and
    # makes it print file names (as grep -r does).
    find "$root" -mindepth 1 \( "${prune[@]}" \) -prune -o -type f ${names[@]+"${names[@]}"} "${skip[@]}" -print0 2>/dev/null |
        xargs -0 grep "$@" -- /dev/null 2>/dev/null
}

# Read a config file (NOT .env for security).
# Usage: llm_config <file> [--max-bytes N]
# Supports: .json, .yaml, .yml (YAML as JSON when yq is installed)
llm_config() {
    if __llm_needs_help "llm_config" "llm_config <file> [--max-bytes N]" "Read config file (JSON/YAML, NOT .env); files over 1 MiB need --max-bytes" "$@"; then
        return 0
    fi

    __gash_no_history __llm_config_impl "$@"
}

__llm_config_impl() {
    local file="" max_bytes=1048576

    while (($# > 0)); do
        case "$1" in
            --max-bytes)
                if (($# < 2)); then
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                fi
                max_bytes="$2"
                shift 2
                ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use a file path and --max-bytes N"
                return 1
                ;;
            *)
                if [[ -n "$file" ]]; then
                    __llm_error "unexpected_argument" "$1" "RETRY" "true" "Pass one file per call"
                    return 1
                fi
                file="$1"
                shift
                ;;
        esac
    done

    if [[ -z "$file" ]]; then
        __llm_error "missing_file" "" "RETRY" "true" "Provide a file path as argument"
        return 1
    fi
    __llm_validate_uint "size" "$max_bytes" || return 1

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
    if [[ ! -r "$safe_path" ]]; then
        __llm_error "path_unreadable" "$safe_path" "RETRY" "true" "The file is not readable by this user"
        return 1
    fi

    local size
    size="$(wc -c <"$safe_path" 2>/dev/null)" || size=""
    size="${size//[[:space:]]/}"
    if [[ "$size" =~ ^[0-9]+$ ]] && ((size > 10#$max_bytes)); then
        __llm_error "file_too_large" "$safe_path is $size bytes" "RETRY" "true" "Over the --max-bytes limit ($max_bytes): raise --max-bytes, or read part of it with llm_exec \"head -c 65536 FILE\""
        return 1
    fi

    # YAML as JSON when yq can convert it; the raw text otherwise (no yq,
    # an unknown yq, or a file it cannot parse).
    local json=""
    case "$safe_path" in
        *.yaml | *.yml)
            json="$(__llm_yaml_to_json "$safe_path")" || json=""
            ;;
    esac
    if [[ -n "$json" ]]; then
        printf '%s\n' "$json"
        return 0
    fi
    if ! cat -- "$safe_path" 2>/dev/null; then
        __llm_error "path_unreadable" "$safe_path" "RETRY" "true" "The file could not be read"
        return 1
    fi
}

# Convert a YAML file to JSON with whichever yq is installed: mikefarah's
# (Go, `yq eval -o=json`) or kislyuk's (Python, a jq wrapper: `yq .`, the
# Debian/Ubuntu package). Prints nothing and fails for anything else.
# Usage: __llm_yaml_to_json FILE
__llm_yaml_to_json() {
    type -P yq >/dev/null 2>&1 || return 1
    if [[ -z "${__LLM_YQ_FLAVOUR-}" ]]; then
        local version
        version="$(yq --version 2>&1 </dev/null)" || version=""
        case "$version" in
            *mikefarah*) __LLM_YQ_FLAVOUR=mikefarah ;;
            "yq "[0-9]*) __LLM_YQ_FLAVOUR=kislyuk ;;
            *) __LLM_YQ_FLAVOUR=none ;;
        esac
    fi
    case "$__LLM_YQ_FLAVOUR" in
        mikefarah) yq eval -o=json . "$1" </dev/null 2>/dev/null ;;
        kislyuk) __llm_has_jq && yq . "$1" </dev/null 2>/dev/null ;;
        *) return 1 ;;
    esac
}
