#!/usr/bin/env bash

# Gash Core: Help System
# Rich, example-driven help registry for all Gash public functions.
#
# Provides:
#   - Associative-array registry for help text, aliases, modules, see-also
#   - __gash_register_help: register help for a function (called by modules)
#   - __gash_help_display: render colorized help for a single function
#   - __gash_help_find_by_alias: reverse-lookup alias -> function name
#   - __gash_help_search: search registry by keyword
#   - __gash_help_list: grouped listing by module
#
# The public gash_help() function lives in lib/modules/gash.sh.
#
# Dependencies: none at load time (colors from output.sh used at display time)

# -----------------------------------------------------------------------------
# Help Registry (global associative arrays)
# -----------------------------------------------------------------------------

# NOTE: We guard each declaration to avoid the bash 5.2 "pop_var_context" bug
# that triggers when `declare -gA` runs inside a sourced file within a function
# context (e.g. gash_source_all in test harness).

if ! declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then
    declare -gA __GASH_HELP_REGISTRY=() # func_name -> full help body
fi
if ! declare -p __GASH_HELP_SHORT &>/dev/null 2>&1; then
    declare -gA __GASH_HELP_SHORT=() # func_name -> one-line description
fi
if ! declare -p __GASH_HELP_ALIASES &>/dev/null 2>&1; then
    declare -gA __GASH_HELP_ALIASES=() # func_name -> comma-separated aliases
fi
if ! declare -p __GASH_HELP_MODULE &>/dev/null 2>&1; then
    declare -gA __GASH_HELP_MODULE=() # func_name -> module name
fi
if ! declare -p __GASH_HELP_SEE_ALSO &>/dev/null 2>&1; then
    declare -gA __GASH_HELP_SEE_ALSO=() # func_name -> space-separated related funcs
fi

# -----------------------------------------------------------------------------
# Registration
# -----------------------------------------------------------------------------

# Register help for a public function.
# Usage:
#   __gash_register_help "function_name" \
#       --aliases "alias1,alias2" \
#       --module "module_name" \
#       --short "One-line description" \
#       --see-also "func1 func2" \
#       <<'HELP'
#   Help body with USAGE and EXAMPLES sections.
#   HELP
__gash_register_help() {
    local func_name="${1-}"
    [[ -z "$func_name" ]] && return 1
    shift

    local aliases="" module="" short="" see_also=""

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --aliases)
                aliases="${2-}"
                shift 2 || break
                ;;
            --module)
                module="${2-}"
                shift 2 || break
                ;;
            --short)
                short="${2-}"
                shift 2 || break
                ;;
            --see-also)
                see_also="${2-}"
                shift 2 || break
                ;;
            *) shift ;;
        esac
    done

    # Read help body from stdin (heredoc) without forking `cat`: Gash
    # registers ~80 entries at load time, one subprocess each was the
    # single largest startup cost.
    local body=""
    IFS= read -r -d '' body || true
    while [[ "$body" == *$'\n' ]]; do body="${body%$'\n'}"; done

    __GASH_HELP_REGISTRY["$func_name"]="$body"
    [[ -n "$short" ]] && __GASH_HELP_SHORT["$func_name"]="$short"
    [[ -n "$aliases" ]] && __GASH_HELP_ALIASES["$func_name"]="$aliases"
    [[ -n "$module" ]] && __GASH_HELP_MODULE["$func_name"]="$module"
    [[ -n "$see_also" ]] && __GASH_HELP_SEE_ALSO["$func_name"]="$see_also"
    return 0
}

# -----------------------------------------------------------------------------
# Display
# -----------------------------------------------------------------------------

# Render colorized help for a single function.
# Usage: __gash_help_display "function_name"
__gash_help_display() {
    local func_name="${1-}"
    [[ -z "$func_name" ]] && return 1

    # Check if function exists in registry
    if [[ -z "${__GASH_HELP_REGISTRY[$func_name]+x}" ]]; then
        return 1
    fi

    local body="${__GASH_HELP_REGISTRY[$func_name]}"
    local short="${__GASH_HELP_SHORT[$func_name]-}"
    local aliases="${__GASH_HELP_ALIASES[$func_name]-}"
    local module="${__GASH_HELP_MODULE[$func_name]-}"
    local see_also="${__GASH_HELP_SEE_ALSO[$func_name]-}"

    # Semantic palette, gated through __gash_use_color at runtime: piping
    # help output must never leak escape sequences. Names (function,
    # aliases, see also) are accent, headings bold, comments muted.
    local A='' B='' M='' R=''
    if __gash_use_color; then
        A="${__GASH_COLOR_ACCENT}"
        B="${__GASH_BOLD}"
        M="${__GASH_COLOR_MUTED}"
        R="${__GASH_COLOR_OFF}"
    fi

    # Header: function name + module tag
    echo
    if [[ -n "$module" ]]; then
        printf '%s\n' "  ${A}${func_name}${R}${M}  (module: ${module})${R}"
    else
        printf '%s\n' "  ${A}${func_name}${R}"
    fi

    # Short description
    if [[ -n "$short" ]]; then
        echo
        printf '%s\n' "  ${B}${short}${R}"
    fi

    # Aliases
    if [[ -n "$aliases" ]]; then
        echo
        __gash_heading --indent 2 "ALIASES"
        local alias_list="" a
        local -a alias_arr=()
        IFS=',' read -r -a alias_arr <<<"$aliases"
        for a in "${alias_arr[@]}"; do
            [[ -n "$alias_list" ]] && alias_list+=", "
            alias_list+="${A}${a}${R}"
        done
        printf '%s\n' "    ${alias_list}"
    fi

    # Body (colorize sections and examples)
    if [[ -n "$body" ]]; then
        echo
        local line
        while IFS= read -r line; do
            # Section headers: USAGE, EXAMPLES, OPTIONS, NOTES, etc.
            if [[ "$line" =~ ^[A-Z][A-Z\ ]+$ ]]; then
                __gash_heading --indent 2 "$line"
            # Comment lines (# ...)
            elif [[ "$line" =~ ^[[:space:]]*# ]]; then
                printf '%s\n' "  ${M}${line}${R}"
            # Empty lines
            elif [[ -z "$line" ]]; then
                echo
            # Regular content (commands/text)
            else
                printf '%s\n' "  ${line}"
            fi
        done <<<"$body"
    fi

    # See Also
    if [[ -n "$see_also" ]]; then
        echo
        __gash_heading --indent 2 "SEE ALSO"
        local sa_list="" sa
        local -a sa_arr=()
        read -r -a sa_arr <<<"$see_also"
        for sa in "${sa_arr[@]}"; do
            [[ -n "$sa_list" ]] && sa_list+=", "
            sa_list+="${A}${sa}${R}"
        done
        printf '%s\n' "    ${sa_list}"
    fi

    echo
}

# -----------------------------------------------------------------------------
# Reverse Lookup
# -----------------------------------------------------------------------------

# Find function name from alias.
# Usage: __gash_help_find_by_alias "alias_name"
# Prints function name to stdout, returns 1 if not found.
__gash_help_find_by_alias() {
    local target="${1-}"
    [[ -z "$target" ]] && return 1

    local func_name a
    local -a alias_arr=()
    for func_name in "${!__GASH_HELP_ALIASES[@]}"; do
        IFS=',' read -r -a alias_arr <<<"${__GASH_HELP_ALIASES[$func_name]}"
        for a in "${alias_arr[@]}"; do
            if [[ "$a" == "$target" ]]; then
                printf '%s' "$func_name"
                return 0
            fi
        done
    done

    return 1
}

# -----------------------------------------------------------------------------
# Search
# -----------------------------------------------------------------------------

# One table row per function: name, aliases in parentheses, short
# description (tab separated, for __gash_help_rows).
# Usage: __gash_help_row FUNCTION
__gash_help_row() {
    local aliases="${__GASH_HELP_ALIASES[$1]-}"
    printf '%s\t%s\t%s\n' "$1" "${aliases:+($aliases)}" "${__GASH_HELP_SHORT[$1]-}"
}

# Render rows from __gash_help_row as a table: names accent, aliases accent,
# descriptions muted, first two columns at least WIDTH1 and WIDTH2 wide (so
# that consecutive tables line up).
# Usage: ... | __gash_help_rows INDENT WIDTH1 WIDTH2
__gash_help_rows() {
    __gash_table --no-header --indent "${1-2}" --min "1:${2-24}" --min "2:${3-0}" --fit \
        --color 1:accent --color 2:accent --color 3:muted
}

# Widths of the name and alias columns over every registered function,
# into the variables named NAMEVAR and ALIASVAR.
# Usage: __gash_help_widths NAMEVAR ALIASVAR
__gash_help_widths() {
    local -n __ghw_n="$1" __ghw_a="$2"
    local f a
    __ghw_n=0 __ghw_a=0
    for f in "${!__GASH_HELP_REGISTRY[@]}"; do
        ((${#f} > __ghw_n)) && __ghw_n=${#f}
        a="${__GASH_HELP_ALIASES[$f]-}"
        [[ -n "$a" ]] && ((${#a} + 2 > __ghw_a)) && __ghw_a=$((${#a} + 2))
    done
    return 0
}

# Search registry for keyword matches (case-insensitive).
# Searches: function names, aliases, short descriptions, full body.
# Usage: __gash_help_search "keyword"
__gash_help_search() {
    local keyword="${1-}"
    [[ -z "$keyword" ]] && return 1

    local lc_keyword="${keyword,,}" func_name rows="" wn wa
    local lc_name lc_aliases lc_short lc_body sorted_funcs
    sorted_funcs="$(printf '%s\n' "${!__GASH_HELP_REGISTRY[@]}" | sort)"

    while IFS= read -r func_name; do
        [[ -z "$func_name" ]] && continue
        lc_name="${func_name,,}"
        lc_aliases="${__GASH_HELP_ALIASES[$func_name]-}"
        lc_short="${__GASH_HELP_SHORT[$func_name]-}"
        lc_body="${__GASH_HELP_REGISTRY[$func_name]-}"
        lc_aliases="${lc_aliases,,}" lc_short="${lc_short,,}" lc_body="${lc_body,,}"
        if [[ "$lc_name" == *"$lc_keyword"* || "$lc_aliases" == *"$lc_keyword"* ||
            "$lc_short" == *"$lc_keyword"* || "$lc_body" == *"$lc_keyword"* ]]; then
            rows+="$(__gash_help_row "$func_name")"$'\n'
        fi
    done <<<"$sorted_funcs"

    if [[ -z "$rows" ]]; then
        if __gash_use_color; then
            printf '  %s%s%s\n' "$__GASH_COLOR_MUTED" "No results for '${keyword}'." "$__GASH_COLOR_OFF"
        else
            printf '  %s\n' "No results for '${keyword}'."
        fi
        return 1
    fi
    __gash_help_widths wn wa
    printf '%s' "$rows" | __gash_help_rows 2 "$wn" "$wa"
    return 0
}

# -----------------------------------------------------------------------------
# List
# -----------------------------------------------------------------------------

# List all registered functions grouped by module.
# Usage: __gash_help_list
__gash_help_list() {
    # Collect unique module names, sorted
    local -A seen_modules=()
    local func_name module wn wa
    for func_name in "${!__GASH_HELP_MODULE[@]}"; do
        module="${__GASH_HELP_MODULE[$func_name]}"
        seen_modules["$module"]=1
    done
    __gash_help_widths wn wa

    # Define display order for modules
    local -a module_order=(files system git docker docker-compose ai sysinfo gash ssh llm utils)

    # Add any modules not in the predefined order
    local in_order m
    for module in "${!seen_modules[@]}"; do
        in_order=0
        for m in "${module_order[@]}"; do
            [[ "$m" == "$module" ]] && in_order=1 && break
        done
        ((in_order)) || module_order+=("$module")
    done

    local displayed=0 sorted
    local -a module_funcs=()
    for module in "${module_order[@]}"; do
        [[ -z "${seen_modules[$module]+x}" ]] && continue

        module_funcs=()
        for func_name in "${!__GASH_HELP_MODULE[@]}"; do
            if [[ "${__GASH_HELP_MODULE[$func_name]}" == "$module" ]]; then
                module_funcs+=("$func_name")
            fi
        done
        sorted="$(printf '%s\n' "${module_funcs[@]}" | sort)"

        echo
        __gash_heading --indent 2 "${module^^}"
        while IFS= read -r func_name; do
            [[ -n "$func_name" ]] && __gash_help_row "$func_name"
        done <<<"$sorted" | __gash_help_rows 4 "$wn" "$wa"
        displayed=1
    done

    # Handle functions with no module
    local -a no_module_funcs=()
    for func_name in "${!__GASH_HELP_REGISTRY[@]}"; do
        if [[ -z "${__GASH_HELP_MODULE[$func_name]-}" ]]; then
            no_module_funcs+=("$func_name")
        fi
    done

    if ((${#no_module_funcs[@]} > 0)); then
        echo
        __gash_heading --indent 2 "OTHER"
        sorted="$(printf '%s\n' "${no_module_funcs[@]}" | sort)"
        while IFS= read -r func_name; do
            [[ -n "$func_name" ]] && __gash_help_row "$func_name"
        done <<<"$sorted" | __gash_help_rows 4 "$wn" "$wa"
    fi

    if ((displayed == 0 && ${#no_module_funcs[@]} == 0)); then
        if __gash_use_color; then
            printf '  %s%s%s\n' "$__GASH_COLOR_MUTED" "No help entries registered." "$__GASH_COLOR_OFF"
        else
            printf '  %s\n' "No help entries registered."
        fi
    fi
    return 0
}
