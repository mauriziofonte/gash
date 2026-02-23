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
    declare -gA __GASH_HELP_REGISTRY=()   # func_name -> full help body
fi
if ! declare -p __GASH_HELP_SHORT &>/dev/null 2>&1; then
    declare -gA __GASH_HELP_SHORT=()      # func_name -> one-line description
fi
if ! declare -p __GASH_HELP_ALIASES &>/dev/null 2>&1; then
    declare -gA __GASH_HELP_ALIASES=()    # func_name -> comma-separated aliases
fi
if ! declare -p __GASH_HELP_MODULE &>/dev/null 2>&1; then
    declare -gA __GASH_HELP_MODULE=()     # func_name -> module name
fi
if ! declare -p __GASH_HELP_SEE_ALSO &>/dev/null 2>&1; then
    declare -gA __GASH_HELP_SEE_ALSO=()   # func_name -> space-separated related funcs
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
            --aliases)  aliases="${2-}"; shift 2 ;;
            --module)   module="${2-}"; shift 2 ;;
            --short)    short="${2-}"; shift 2 ;;
            --see-also) see_also="${2-}"; shift 2 ;;
            *) shift ;;
        esac
    done

    # Read help body from stdin (heredoc)
    local body=""
    body="$(cat)"

    __GASH_HELP_REGISTRY["$func_name"]="$body"
    [[ -n "$short" ]]    && __GASH_HELP_SHORT["$func_name"]="$short"
    [[ -n "$aliases" ]]  && __GASH_HELP_ALIASES["$func_name"]="$aliases"
    [[ -n "$module" ]]   && __GASH_HELP_MODULE["$func_name"]="$module"
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

    # Colors (defined in output.sh, available at display time)
    local A="${__GASH_COLOR_ACCENT-\033[38;5;214m}"   # Orange - function name
    local W="${__GASH_BOLD_WHITE-\033[1;37m}"          # Bold white - section headers
    local G="${__GASH_GREEN-\033[0;32m}"               # Green - aliases
    local M="${__GASH_COLOR_MUTED-\033[38;5;245m}"     # Dim gray - comments/module
    local C="${__GASH_CYAN-\033[0;36m}"                # Cyan - commands
    local R="${__GASH_COLOR_OFF-\033[0m}"               # Reset

    # Header: function name + module tag
    echo
    if [[ -n "$module" ]]; then
        echo -e "  ${A}${func_name}${R}${M}  (module: ${module})${R}"
    else
        echo -e "  ${A}${func_name}${R}"
    fi

    # Short description
    if [[ -n "$short" ]]; then
        echo
        echo -e "  ${W}${short}${R}"
    fi

    # Aliases
    if [[ -n "$aliases" ]]; then
        echo
        echo -e "  ${W}ALIASES${R}"
        local IFS=','
        local alias_list=""
        for a in $aliases; do
            [[ -n "$alias_list" ]] && alias_list+=", "
            alias_list+="${G}${a}${R}"
        done
        echo -e "    ${alias_list}"
    fi

    # Body (colorize sections and examples)
    if [[ -n "$body" ]]; then
        echo
        local line
        while IFS= read -r line; do
            # Section headers: USAGE, EXAMPLES, OPTIONS, NOTES, etc.
            if [[ "$line" =~ ^[A-Z][A-Z\ ]+$ ]]; then
                echo -e "  ${W}${line}${R}"
            # Comment lines (# ...)
            elif [[ "$line" =~ ^[[:space:]]*# ]]; then
                echo -e "  ${M}${line}${R}"
            # Empty lines
            elif [[ -z "$line" ]]; then
                echo
            # Regular content (commands/text)
            else
                echo -e "  ${C}${line}${R}"
            fi
        done <<< "$body"
    fi

    # See Also
    if [[ -n "$see_also" ]]; then
        echo
        echo -e "  ${W}SEE ALSO${R}"
        local sa_list=""
        for sa in $see_also; do
            [[ -n "$sa_list" ]] && sa_list+=", "
            sa_list+="${A}${sa}${R}"
        done
        echo -e "    ${sa_list}"
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

    local func_name aliases
    for func_name in "${!__GASH_HELP_ALIASES[@]}"; do
        aliases="${__GASH_HELP_ALIASES[$func_name]}"
        local IFS=','
        for a in $aliases; do
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

# Search registry for keyword matches (case-insensitive).
# Searches: function names, aliases, short descriptions, full body.
# Usage: __gash_help_search "keyword"
__gash_help_search() {
    local keyword="${1-}"
    [[ -z "$keyword" ]] && return 1

    local A="${__GASH_COLOR_ACCENT-\033[38;5;214m}"
    local G="${__GASH_GREEN-\033[0;32m}"
    local W="${__GASH_BOLD_WHITE-\033[1;37m}"
    local M="${__GASH_COLOR_MUTED-\033[38;5;245m}"
    local R="${__GASH_COLOR_OFF-\033[0m}"

    local lc_keyword
    lc_keyword="$(printf '%s' "$keyword" | tr '[:upper:]' '[:lower:]')"

    local found=0
    local func_name

    # Collect and sort function names
    local sorted_funcs
    sorted_funcs="$(printf '%s\n' "${!__GASH_HELP_REGISTRY[@]}" | sort)"

    while IFS= read -r func_name; do
        [[ -z "$func_name" ]] && continue

        local lc_name lc_aliases lc_short lc_body
        lc_name="$(printf '%s' "$func_name" | tr '[:upper:]' '[:lower:]')"
        lc_aliases="$(printf '%s' "${__GASH_HELP_ALIASES[$func_name]-}" | tr '[:upper:]' '[:lower:]')"
        lc_short="$(printf '%s' "${__GASH_HELP_SHORT[$func_name]-}" | tr '[:upper:]' '[:lower:]')"
        lc_body="$(printf '%s' "${__GASH_HELP_REGISTRY[$func_name]-}" | tr '[:upper:]' '[:lower:]')"

        if [[ "$lc_name" == *"$lc_keyword"* ]] || \
           [[ "$lc_aliases" == *"$lc_keyword"* ]] || \
           [[ "$lc_short" == *"$lc_keyword"* ]] || \
           [[ "$lc_body" == *"$lc_keyword"* ]]; then

            local aliases="${__GASH_HELP_ALIASES[$func_name]-}"
            local short="${__GASH_HELP_SHORT[$func_name]-}"
            local alias_display=""

            if [[ -n "$aliases" ]]; then
                alias_display=" ${G}(${aliases})${R}"
            fi

            printf '  %b%-24s%b %b%s%b\n' "$A" "$func_name" "$R${alias_display}" "$M" "$short" "$R"
            found=1
        fi
    done <<< "$sorted_funcs"

    if [[ $found -eq 0 ]]; then
        echo -e "  ${M}No results for '${keyword}'.${R}"
        return 1
    fi

    return 0
}

# -----------------------------------------------------------------------------
# List
# -----------------------------------------------------------------------------

# List all registered functions grouped by module.
# Usage: __gash_help_list
__gash_help_list() {
    local A="${__GASH_COLOR_ACCENT-\033[38;5;214m}"
    local G="${__GASH_GREEN-\033[0;32m}"
    local W="${__GASH_BOLD_WHITE-\033[1;37m}"
    local M="${__GASH_COLOR_MUTED-\033[38;5;245m}"
    local R="${__GASH_COLOR_OFF-\033[0m}"

    # Collect unique module names, sorted
    local -A seen_modules=()
    local func_name module
    for func_name in "${!__GASH_HELP_MODULE[@]}"; do
        module="${__GASH_HELP_MODULE[$func_name]}"
        seen_modules["$module"]=1
    done

    # Define display order for modules
    local -a module_order=(files system git docker docker-compose ai sysinfo gash ssh llm utils)

    # Add any modules not in the predefined order
    for module in "${!seen_modules[@]}"; do
        local in_order=0
        local m
        for m in "${module_order[@]}"; do
            [[ "$m" == "$module" ]] && in_order=1 && break
        done
        [[ $in_order -eq 0 ]] && module_order+=("$module")
    done

    local displayed=0

    for module in "${module_order[@]}"; do
        [[ -z "${seen_modules[$module]+x}" ]] && continue

        # Collect functions for this module
        local -a module_funcs=()
        for func_name in "${!__GASH_HELP_MODULE[@]}"; do
            if [[ "${__GASH_HELP_MODULE[$func_name]}" == "$module" ]]; then
                module_funcs+=("$func_name")
            fi
        done

        # Sort functions
        local sorted
        sorted="$(printf '%s\n' "${module_funcs[@]}" | sort)"

        # Print module header
        local uc_module
        uc_module="$(printf '%s' "$module" | tr '[:lower:]' '[:upper:]')"
        echo
        echo -e "  ${W}${uc_module}${R}"

        # Print each function
        while IFS= read -r func_name; do
            [[ -z "$func_name" ]] && continue

            local aliases="${__GASH_HELP_ALIASES[$func_name]-}"
            local short="${__GASH_HELP_SHORT[$func_name]-}"
            local alias_display=""

            if [[ -n "$aliases" ]]; then
                alias_display=" ${G}(${aliases})${R}"
            fi

            printf '    %b%-24s%b %b%s%b\n' "$A" "$func_name" "$R${alias_display}" "$M" "$short" "$R"
            displayed=1
        done <<< "$sorted"
    done

    # Handle functions with no module
    local -a no_module_funcs=()
    for func_name in "${!__GASH_HELP_REGISTRY[@]}"; do
        if [[ -z "${__GASH_HELP_MODULE[$func_name]+x}" ]] || [[ -z "${__GASH_HELP_MODULE[$func_name]}" ]]; then
            no_module_funcs+=("$func_name")
        fi
    done

    if [[ ${#no_module_funcs[@]} -gt 0 ]]; then
        echo
        echo -e "  ${W}OTHER${R}"
        local sorted
        sorted="$(printf '%s\n' "${no_module_funcs[@]}" | sort)"
        while IFS= read -r func_name; do
            [[ -z "$func_name" ]] && continue
            local aliases="${__GASH_HELP_ALIASES[$func_name]-}"
            local short="${__GASH_HELP_SHORT[$func_name]-}"
            local alias_display=""
            if [[ -n "$aliases" ]]; then
                alias_display=" ${G}(${aliases})${R}"
            fi
            printf '    %b%-24s%b %b%s%b\n' "$A" "$func_name" "$R${alias_display}" "$M" "$short" "$R"
        done <<< "$sorted"
    fi

    if [[ $displayed -eq 0 && ${#no_module_funcs[@]} -eq 0 ]]; then
        echo -e "  ${M}No help entries registered.${R}"
    fi
}
