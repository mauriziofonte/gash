#!/usr/bin/env bash

# Gash Module: Core Gash Functions
# Functions for Gash management (help, upgrade, uninstall, unload).
#
# Dependencies: core/output.sh, core/validation.sh, core/utils.sh
#
# Public functions:
#   gash_help             - Display Gash help with all available commands
#   gash_upgrade          - Upgrade Gash to the latest version
#   gash_uninstall        - Uninstall Gash and clean up configurations
#   gash_unload           - Restore shell state saved before Gash was loaded
#   gash_username         - Get the current username
#   gash_inspiring_quote  - Display an inspiring quote

# -----------------------------------------------------------------------------
# User Info
# -----------------------------------------------------------------------------

# Get the current username using multiple fallback methods.
# Usage: gash_username
# Returns: username string (never empty, falls back to UNKNOWN_USERNAME)
gash_username() {
    needs_help "gash_username" "gash_username" "Print the current user name." "${1-}" && return
    local user_name=""

    # Try whoami first (most common)
    if command -v whoami >/dev/null 2>&1; then
        user_name=$(whoami 2>/dev/null) || true
    fi

    # Fallback to id -un
    if [[ -z "$user_name" ]] && command -v id >/dev/null 2>&1; then
        user_name=$(id -un 2>/dev/null) || true
    fi

    # Fallback to /etc/passwd lookup
    if [[ -z "$user_name" ]] && command -v id >/dev/null 2>&1; then
        local user_id
        user_id=$(id -u 2>/dev/null) || true
        if [[ -n "$user_id" && -r /etc/passwd ]]; then
            user_name=$(grep "^[^:]*:[^:]*:${user_id}:" /etc/passwd 2>/dev/null | cut -d':' -f1) || true
        fi
    fi

    # Final fallback
    if [[ -z "$user_name" ]]; then
        user_name="UNKNOWN_USERNAME"
    fi

    printf '%s\n' "$user_name"
}

# -----------------------------------------------------------------------------
# Quotes
# -----------------------------------------------------------------------------

# Display a random quote from quotes/list.txt. Each quote is one line,
# `"Text." — Author`, under a `# source` comment line naming where it was
# said or written; --source prints that line too.
# Usage: gash_inspiring_quote [--source]
gash_inspiring_quote() {
    needs_help "gash_inspiring_quote" "gash_inspiring_quote [--source]" "Print a random quote (with --source: where it comes from)." "${1-}" && return

    local show_source=0
    case "${1-}" in
        "") ;;
        --source) show_source=1 ;;
        *)
            __gash_error "Unknown option: $1 (usage: gash_inspiring_quote [--source])"
            return 1
            ;;
    esac

    local quotes_file="${GASH_DIR:-$HOME/.gash}/quotes/list.txt"
    [[ -r "$quotes_file" ]] || return 1

    # No subprocess on this path: it runs in every new shell (banner).
    local IFS=$' \t\n'
    local -a lines=() quotes=() sources=()
    local line src="" n try
    mapfile -t lines <"$quotes_file"
    n=${#lines[@]}
    # Without --source: random lines until one is a quote, which picks
    # each quote with the same chance and reads no other line.
    if ((show_source == 0 && n > 0)); then
        for ((try = 0; try < 64; try++)); do
            line="${lines[RANDOM % n]}"
            [[ -z "$line" || "$line" == "#"* ]] && continue
            quotes=("$line")
            break
        done
    fi
    if ((${#quotes[@]} == 0)); then
        for line in ${lines[@]+"${lines[@]}"}; do
            case "$line" in
                "") src="" ;;
                "#"*) src="${line#\#}" src="${src# }" ;;
                *)
                    quotes+=("$line")
                    sources+=("$src")
                    src=""
                    ;;
            esac
        done
    fi
    ((${#quotes[@]})) || return 1

    local idx=$((RANDOM % ${#quotes[@]}))
    local wrap_width=""
    # A glob, not =~: BASH_REMATCH would be left behind in the shell.
    [[ -n "${COLUMNS-}" && "$COLUMNS" != *[!0-9]* ]] && ((10#$COLUMNS > 0)) && wrap_width="$COLUMNS"
    [[ -n "$wrap_width" ]] || wrap_width="$(__gash_tty_width)"
    wrap_width=$((wrap_width - 7))
    ((wrap_width < 20)) && wrap_width=80

    local label="Quote:" on="" off="" first=1 qline
    local -a wrapped=()
    if __gash_use_color; then
        on="$__GASH_CYAN" off="$__GASH_COLOR_OFF"
    fi
    __gash_wrap wrapped "$wrap_width" "${quotes[$idx]}"
    for qline in "${wrapped[@]}"; do
        if ((first)); then
            printf '%s%s%s %s\n' "$on" "$label" "$off" "$qline"
            first=0
        else
            printf '       %s\n' "$qline"
        fi
    done

    if ((show_source)) && [[ -n "${sources[$idx]}" ]]; then
        __gash_wrap wrapped "$wrap_width" "Source: ${sources[$idx]}"
        for qline in "${wrapped[@]}"; do
            printf '       %s\n' "$qline"
        done
    fi
    return 0
}

# -----------------------------------------------------------------------------
# Upgrade
# -----------------------------------------------------------------------------

# Highest release tag in a checkout: vX.Y.Z or X.Y, no pre-release suffix,
# compared as versions (same rule as install.sh).
# Usage: __gash_latest_tag DIR
__gash_latest_tag() {
    git -C "${1:-.}" tag --list --sort=v:refname 2>/dev/null |
        grep -E '^v?[0-9]+(\.[0-9]+)*$' |
        tail -n 1
}

# Report a release tag's signature when it has one. With
# GASH_REQUIRE_SIGNED=1 an unsigned or unverifiable tag returns 1.
# Usage: __gash_verify_tag DIR TAG
__gash_verify_tag() {
    local dir="$1" tag="$2"
    if git -C "$dir" cat-file -p "refs/tags/$tag" 2>/dev/null | grep -Eq -- '-----BEGIN (PGP|SSH) SIGNATURE-----'; then
        if git -C "$dir" verify-tag "$tag" >/dev/null 2>&1; then
            __gash_success "Signature of $tag verified."
            return 0
        fi
        __gash_warning "$tag is signed, but the signature could not be verified (is the key in your keyring?)."
    elif [[ "${GASH_REQUIRE_SIGNED-0}" == "1" ]]; then
        __gash_warning "$tag is not signed."
    else
        return 0
    fi
    [[ "${GASH_REQUIRE_SIGNED-0}" == "1" ]] && return 1
    return 0
}

# Upgrade Gash to the latest release tag.
# Usage: gash_upgrade
gash_upgrade() {
    needs_help "gash_upgrade" "gash_upgrade" "Upgrade Gash to the latest release (refuses to touch local modifications)." "${1-}" && return

    local install_dir="${GASH_DIR:-$HOME/.gash}"

    if [[ ! -d "$install_dir/.git" ]]; then
        __gash_error "Gash is not installed in $install_dir as a git checkout. See https://github.com/mauriziofonte/gash."
        return 1
    fi

    # The checkout below would overwrite local edits: refuse instead.
    if [[ -n "$(git -C "$install_dir" status --porcelain 2>/dev/null)" ]]; then
        __gash_error "Working tree at $install_dir has local modifications. Commit, stash, or discard them before upgrading."
        return 1
    fi

    __gash_info "Checking for a new release in $install_dir..."
    if ! git -C "$install_dir" fetch --quiet --tags origin; then
        __gash_error "Failed to fetch updates. Check your network connection, or run 'git fetch' in $install_dir."
        return 1
    fi

    local latest_tag current_tag
    latest_tag="$(__gash_latest_tag "$install_dir")"
    if [[ -z "$latest_tag" ]]; then
        __gash_warning "No release tags found; leaving $install_dir as it is."
        return 0
    fi

    current_tag="$(git -C "$install_dir" describe --tags --exact-match HEAD 2>/dev/null)" || current_tag=""
    if [[ "$current_tag" == "$latest_tag" ]]; then
        local release_date
        release_date="$(git -C "$install_dir" log -1 --format=%as "$current_tag" 2>/dev/null)"
        __gash_success "Gash is already up-to-date ($current_tag, released on $release_date)."
        return 0
    fi

    __gash_verify_tag "$install_dir" "$latest_tag" || {
        __gash_error "GASH_REQUIRE_SIGNED=1 is set; not upgrading to $latest_tag."
        return 1
    }

    if ! git -C "$install_dir" -c advice.detachedHead=false checkout --quiet "$latest_tag"; then
        __gash_error "Failed to check out $latest_tag."
        return 1
    fi

    __gash_success "Upgraded Gash ${current_tag:+from $current_tag }to $latest_tag."
    __gash_info "Open a new terminal (or run 'source ~/.gashrc') to load it."
}

# -----------------------------------------------------------------------------
# Uninstall
# -----------------------------------------------------------------------------

# Uninstall Gash and clean up configurations.
# Usage: gash_uninstall
gash_uninstall() {
    needs_help "gash_uninstall" "gash_uninstall" "Remove Gash and its startup block from this account (asks first)." "${1-}" && return
    local gash_dir="${GASH_DIR:-$HOME/.gash}"

    if [ ! -d "$gash_dir" ]; then
        __gash_error "Gash is not installed on this system."
        return 1
    fi
    # rm -rf only a directory that is unmistakably a Gash checkout, never
    # / or $HOME (a stray GASH_DIR must not turn into a wipe).
    local real_dir real_home
    real_dir="$(cd -- "$gash_dir" 2>/dev/null && pwd -P)" || real_dir=""
    real_home="$(cd -- "$HOME" 2>/dev/null && pwd -P)" || real_home="$HOME"
    if [[ -z "$real_dir" || "$real_dir" == "/" || "$real_dir" == "$real_home" ||
        ! -f "$real_dir/gash.sh" || ! -d "$real_dir/lib/core" ]]; then
        __gash_error "$gash_dir does not look like a Gash installation (no gash.sh and lib/core); refusing to remove it."
        return 1
    fi

    __gash_warning "This will remove Gash ($real_dir) and its configuration from this account."
    if ! needs_confirm_prompt "Continue?"; then
        __gash_info "Uninstall cancelled."
        return 0
    fi

    local profile_files=("$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.profile")

    # Remove Gash sourcing lines from profiles
    # Uses unique delimiters to avoid matching other if/fi blocks
    for profile_file in "${profile_files[@]}"; do
        if [ -f "$profile_file" ]; then
            if command grep -q 'source.*[./]gashrc' "$profile_file"; then
                __gash_info "Removing Gash block from: $profile_file"
                # Try new format with delimiters first
                if grep -q '# >>> GASH START >>>' "$profile_file"; then
                    sed -i.gash-uninstall.bak '/# >>> GASH START >>>/,/# <<< GASH END <<</d' "$profile_file"
                # Fallback: old format - use more specific pattern to avoid catching other fi's
                elif grep -q '# Load Gash Bash' "$profile_file"; then
                    # Remove exactly: comment + if block + fi (4 lines total)
                    sed -i.gash-uninstall.bak '/# Load Gash Bash/{N;N;N;N;d}' "$profile_file"
                fi
            else
                __gash_info "No Gash block found in: $profile_file; skipping."
            fi
        fi
    done

    # Remove ~/.gashrc
    if [ -f "$HOME/.gashrc" ]; then
        __gash_info "Removing ~/.gashrc..."
        if ! rm -f "$HOME/.gashrc" 2>/dev/null; then
            __gash_error "Failed to remove ~/.gashrc; please remove it manually."
        fi
    else
        __gash_info "$HOME/.gashrc not found; skipping."
    fi

    # Remove Gash directory (verified above)
    __gash_info "Removing Gash at $real_dir..."
    if ! rm -rf -- "$real_dir" 2>/dev/null; then
        __gash_error "Failed to remove $real_dir; please remove it manually."
    fi

    # Handle the settings file, ~/.gash_env or GASH_ENV_FILE (contains
    # credentials - ask user)
    local env_file="${GASH_ENV_FILE:-$HOME/.gash_env}"
    local env_shown="${env_file/#$HOME/\~}"
    if [ -f "$env_file" ]; then
        __gash_warning "Found $env_shown (may contain database credentials)."
        if needs_confirm_prompt "Remove $env_shown?"; then
            if rm -f -- "$env_file" 2>/dev/null; then
                __gash_info "Removed $env_shown"
            else
                __gash_error "Failed to remove $env_shown; please remove it manually."
            fi
        else
            __gash_info "Kept $env_shown"
        fi
    fi

    __gash_success "Gash uninstalled."
    __gash_info "Restart your terminal to apply changes."

    # Clean up the backups sed made above (only ours: a user's own
    # ~/.bashrc.bak is left alone).
    for profile_file in "${profile_files[@]}"; do
        rm -f -- "${profile_file}.gash-uninstall.bak"
    done
}

# -----------------------------------------------------------------------------
# Unload
# -----------------------------------------------------------------------------

# From a list made by __gash_symbols_now (gash.sh), the part that holds
# functions (public: those not starting with _) or aliases, into VAR.
# Usage: __gash_symbols_pick VAR LIST functions|aliases [public]
__gash_symbols_pick() {
    # Bytes, not characters (\x01 never occurs inside a UTF-8 character),
    # and offsets instead of ${list#*X}, which is quadratic in bash.
    local LC_ALL=C
    local list="$2" public internal aliases
    public="${list%%$'\001'*}"
    list="${list:${#public}+1}"
    internal="${list%%$'\001'*}"
    aliases="${list:${#internal}+1}"
    case "$3|${4-}" in
        aliases*) printf -v "$1" '%s' "$aliases" ;;
        *"|public") printf -v "$1" '%s' "$public" ;;
        *) printf -v "$1" '%s\n%s' "$public" "$internal" ;;
    esac
}

# Names Gash defined while loading, into the array VAR: present right after
# it loaded, absent from the snapshot taken before (interactive mode only;
# gash.sh records both lists). "public" leaves out names starting with _.
# Usage: __gash_added_symbols_to VAR functions|aliases [public]
__gash_added_symbols_to() {
    local -n __gas_out="$1"
    local - __gas_name __gas_pre __gas_post
    set -f
    local IFS=$'\n' LC_ALL=C
    __gash_symbols_pick __gas_pre "${__GASH_PRE_SYMBOLS-}" "${2-}" "${3-}"
    __gash_symbols_pick __gas_post "${__GASH_POST_SYMBOLS-}" "${2-}" "${3-}"
    # shellcheck disable=SC2206 # split on newlines, globbing is off
    __gas_out=($__gas_post)
    # Usually little or nothing existed before: one lookup per name.
    [[ -n "${__gas_pre//$'\n'/}" && ${#__gas_out[@]} -gt 0 ]] || return 0
    local -A __gas_before=()
    for __gas_name in $__gas_pre; do
        __gas_before["$__gas_name"]=1
    done
    local -a __gas_kept=()
    for __gas_name in "${__gas_out[@]}"; do
        [[ -n "${__gas_before["$__gas_name"]+x}" ]] || __gas_kept+=("$__gas_name")
    done
    __gas_out=(${__gas_kept[@]+"${__gas_kept[@]}"})
    return 0
}

# The same names, one per line.
# Usage: __gash_added_symbols functions|aliases
__gash_added_symbols() {
    local -a names=()
    __gash_added_symbols_to names "${1-functions}"
    ((${#names[@]})) && printf '%s\n' "${names[@]}"
    return 0
}

# Restore the shell state saved before Gash was loaded (best-effort).
# Usage: gash_unload
gash_unload() {
    needs_help "gash_unload" "gash_unload" \
        "Restores the shell state saved before Gash was loaded (best-effort)." \
        "${1-}" && return

    if [[ -z "${__GASH_SNAPSHOT_TAKEN-}" ]]; then
        __gash_error "Gash snapshot not found. Source gash.sh first."
        return 1
    fi

    # Remove the prompt hook from PROMPT_COMMAND (other entries stay).
    if declare -f __gash_prompt_uninstall >/dev/null 2>&1; then
        __gash_prompt_uninstall
    fi

    # Restore variables, shell options and umask from the load-time snapshot.
    local __gash_entry __gash_key
    for __gash_entry in "${__GASH_SNAPSHOT[@]}"; do
        case "$__gash_entry" in
            shopt:*)
                __gash_key="${__gash_entry#shopt:}"
                if [[ "$__gash_entry" == *=1 ]]; then
                    shopt -s "${__gash_key%=*}" 2>/dev/null
                else
                    shopt -u "${__gash_key%=*}" 2>/dev/null
                fi
                ;;
            umask=*) umask "${__gash_entry#umask=}" ;;
            *=*)
                __gash_key="${__gash_entry%%=*}"
                printf -v "$__gash_key" '%s' "${__gash_entry#*=}"
                ;;
            *) unset "$__gash_entry" ;;
        esac
    done

    # Remove aliases and functions introduced by Gash.
    local __gash_name
    local -a __gash_names=()
    __gash_added_symbols_to __gash_names aliases
    for __gash_name in ${__gash_names[@]+"${__gash_names[@]}"}; do
        unalias "$__gash_name" >/dev/null 2>&1 || true
    done
    __gash_added_symbols_to __gash_names functions
    for __gash_name in ${__gash_names[@]+"${__gash_names[@]}"}; do
        [[ "$__gash_name" == "gash_unload" ]] && continue
        unset -f "$__gash_name" >/dev/null 2>&1 || true
    done

    # Per-prefix function sweep: the load-time snapshot misses functions
    # defined AFTER it was taken (alias loader, prompt.sh, completion,
    # helpers created at runtime). Anything gash_* / __gash_* goes.
    local __gash_fn
    while IFS= read -r __gash_fn; do
        [[ -z "${__gash_fn}" ]] && continue
        [[ "${__gash_fn}" == "gash_unload" ]] && continue
        unset -f "${__gash_fn}" >/dev/null 2>&1 || true
    done < <(declare -F 2>/dev/null | awk '{print $3}' | grep -E '^(gash_|__gash_)' || true)

    # Per-prefix variable sweep: includes the cached secrets
    # (__GASH_ENV_SSH_KEYS, __GASH_ENV_AI_PROVIDERS) which must never
    # survive an unload, plus colors, PS1 caches, help registry.
    local __gash_var
    while IFS= read -r __gash_var; do
        [[ -z "${__gash_var}" ]] && continue
        unset "${__gash_var}" >/dev/null 2>&1 || true
    done < <(compgen -v 2>/dev/null | grep -E '^(GASH_|__GASH_)' || true)
    unset __AI_LAST_EXIT >/dev/null 2>&1 || true

    unset __gash_name

    # Finally, remove gash_unload itself
    unset -f gash_unload >/dev/null 2>&1 || true

    return 0
}

# -----------------------------------------------------------------------------
# Health Check
# -----------------------------------------------------------------------------

# Microseconds as milliseconds with one decimal, into VAR (no floats: any
# locale). Usage: __gash_us_to_ms VAR MICROSECONDS
__gash_us_to_ms() {
    printf -v "$1" '%d.%d' "$(($2 / 1000))" "$(($2 % 1000 / 100))"
}

# Startup cost, for `gash_doctor --timing`. A new interactive shell loads
# Gash from GASH_DIR as ~/.gashrc does, with GASH_TIMING=1, and hands back
# __GASH_LOAD_TIMES on fd 3 (its banner and other output are dropped; SSH
# key unlocking is marked done so it never starts an agent). Then whole new
# shells are timed: `bash -i` with your ~/.bashrc, and bash alone (median of
# 3). Needs $EPOCHREALTIME (Bash 5.0+).
__gash_doctor_timing() {
    if [[ -z "${EPOCHREALTIME-}" ]]; then
        __gash_error "gash_doctor --timing needs Bash 5.0 or newer (\$EPOCHREALTIME); this is ${BASH_VERSION}."
        return 1
    fi
    local gash_dir="${GASH_DIR:-$HOME/.gash}" bash_bin="${BASH:-bash}"
    local H="${__GASH_BOLD_WHITE-}" W="${__GASH_YELLOW-}" O="${__GASH_COLOR_OFF-}"
    local out us label ms on i t0 t1
    # shellcheck disable=SC2016 # expanded by the child shell
    out="$(GASH_TIMING=1 GASH_HEADLESS='' GASH_SSH_AUTOUNLOCK_RAN=1 "$bash_bin" --noprofile --norc -i -c '
        source "$1/gash.sh"
        [[ -f "$1/bash_completion" ]] && source "$1/bash_completion"
        printf "%s\n" "${__GASH_LOAD_TIMES[@]}" >&3
    ' gash-timing "$gash_dir" 3>&1 >/dev/null 2>&1 </dev/null)"
    if [[ -z "$out" ]]; then
        __gash_error "No timings: a new shell did not load $gash_dir/gash.sh."
        return 1
    fi

    printf '%s\n' "${H}Startup timing${O} — Gash ${GASH_VERSION:-unknown}, bash ${BASH_VERSION}, in a new interactive shell"
    printf '  %7s  %s\n' "ms" "step"
    while read -r us label; do
        [[ "$us" =~ ^[0-9]+$ ]] || continue
        __gash_us_to_ms ms "$us"
        on=""
        [[ "$label" == *"(total)" ]] && on="$H"
        ((us >= 5000)) && on="$W"
        printf '  %s%7s%s  %s\n' "$on" "$ms" "$O" "$label"
    done <<<"$out"

    # Whole shells, median of three runs each.
    local -a whole=() bare=()
    for i in 1 2 3; do
        t0="$EPOCHREALTIME"
        GASH_SSH_AUTOUNLOCK_RAN=1 "$bash_bin" -i -c exit </dev/null >/dev/null 2>&1
        t1="$EPOCHREALTIME"
        whole+=("$((${t1//[!0-9]/} - ${t0//[!0-9]/}))")
        t0="$EPOCHREALTIME"
        "$bash_bin" --noprofile --norc -i -c exit </dev/null >/dev/null 2>&1
        t1="$EPOCHREALTIME"
        bare+=("$((${t1//[!0-9]/} - ${t0//[!0-9]/}))")
    done
    local whole_ms bare_ms
    __gash_us_to_ms whole_ms "$(__gash_median3 "${whole[@]}")"
    __gash_us_to_ms bare_ms "$(__gash_median3 "${bare[@]}")"
    printf '\n  New shell with your ~/.bashrc: %s ms (bash alone: %s ms; median of 3)\n' "$whole_ms" "$bare_ms"
    printf '  GASH_TIMING=1 when a shell starts keeps its own figures in __GASH_LOAD_TIMES.\n'
    return 0
}

# Usage: __gash_median3 A B C  (prints the middle value)
__gash_median3() {
    local a="$1" b="$2" c="$3" lo hi
    lo=$((a < b ? a : b)) lo=$((lo < c ? lo : c))
    hi=$((a > b ? a : b)) hi=$((hi > c ? hi : c))
    printf '%s' "$((a + b + c - lo - hi))"
}

# Run health checks on the Gash installation.
# Verifies core files, modules, config permissions, and external tools.
# Usage: gash_doctor [--no-color]
gash_doctor() {
    needs_help "gash_doctor" "gash_doctor [--timing] [--no-color]" "Run health checks on your Gash installation (--timing: what each file costs at startup)." "${1-}" && return

    local no_color=0 timing=0
    local a
    for a in "$@"; do
        case "$a" in
            --no-color) no_color=1 ;;
            --timing) timing=1 ;;
        esac
    done

    # Local color scope (respects env, TTY, --no-color)
    __gash_want_colors "$no_color" || eval "$__GASH_COLOR_OFF_LOCALS"

    if ((timing)); then
        __gash_doctor_timing
        return
    fi

    local gash_dir="${GASH_DIR:-$HOME/.gash}"
    local issues=0 f mod tool name
    local H="${__GASH_BOLD_WHITE-}" O="${__GASH_COLOR_OFF-}"

    __gash_info "Gash Doctor — checking installation (v${GASH_VERSION:-unknown})..."
    echo ""

    # 1. Runtime
    printf '%s\n' "${H}Runtime:${O}"
    if ((BASH_VERSINFO[0] > 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] >= 4))); then
        __gash_success "  bash ${BASH_VERSION}"
    else
        __gash_error "  bash ${BASH_VERSION} — Gash needs 4.4 or newer"
        issues=$((issues + 1))
    fi
    local -a load_errors=(${__GASH_LOAD_ERRORS[@]+"${__GASH_LOAD_ERRORS[@]}"})
    if ((${#load_errors[@]} == 0)); then
        __gash_success "  all files loaded"
    else
        for f in "${load_errors[@]}"; do
            __gash_error "  failed to load ${f}"
            issues=$((issues + 1))
        done
    fi

    # 2. Core files
    printf '%s\n' "${H}Core Files:${O}"
    local core_files=(gash.sh lib/core/output.sh lib/core/help.sh lib/core/utils.sh lib/core/validation.sh lib/core/config.sh
        lib/shell/terminal.sh lib/shell/history.sh lib/shell/prompt.sh lib/shell/banner.sh lib/aliases/_loader.sh)
    for f in "${core_files[@]}"; do
        if [[ -f "$gash_dir/$f" ]]; then
            __gash_success "  $f"
        else
            __gash_error "  $f — MISSING"
            issues=$((issues + 1))
        fi
    done

    # 3. Modules
    printf '%s\n' "${H}Modules:${O}"
    local expected_modules=(gash git ssh files docker docker-compose system llm ai sysinfo remote node php)
    for mod in "${expected_modules[@]}"; do
        if [[ -f "$gash_dir/lib/modules/${mod}.sh" ]]; then
            __gash_success "  ${mod}"
        else
            __gash_warning "  ${mod} — not found"
            issues=$((issues + 1))
        fi
    done

    # 4. Config file and settings
    printf '%s\n' "${H}Configuration:${O}"
    local env_file="${GASH_ENV_FILE:-$HOME/.gash_env}"
    if [[ -f "$env_file" ]]; then
        local perms
        perms=$(stat -L -c '%a' "$env_file" 2>/dev/null || stat -L -f '%Lp' "$env_file" 2>/dev/null || echo "unknown")
        if [[ "$perms" == "600" || "$perms" == "400" ]]; then
            __gash_success "  $env_file (permissions: $perms)"
        else
            __gash_warning "  $env_file (permissions: $perms — should be 600)"
            issues=$((issues + 1))
        fi
    else
        __gash_info "  $env_file — not found (optional, run gash_env_init to create)"
    fi
    for name in ${__GASH_SETTINGS_KNOWN-}; do
        [[ -n "${!name+x}" ]] && printf '    %s=%s\n' "$name" "${!name}"
    done

    # 5. External tools
    printf '%s\n' "${H}External Tools:${O}"
    local tools=(git curl jq sqlite3 docker)
    for tool in "${tools[@]}"; do
        if type -P "$tool" >/dev/null 2>&1; then
            __gash_success "  $tool ($(type -P "$tool"))"
        else
            __gash_warning "  $tool — not found (some features may be unavailable)"
        fi
    done

    # 6. Summary
    echo ""
    if [[ $issues -eq 0 ]]; then
        __gash_success "All checks passed."
    else
        __gash_warning "$issues issue(s) found."
    fi
}

alias gdoctor='gash_doctor'

# -----------------------------------------------------------------------------
# Reference Card
# -----------------------------------------------------------------------------

# Topics of the reference card, generated from the help registry (functions)
# and from lib/aliases/*.sh (aliases), so the card cannot drift from the code.
# Format: topic|help modules|alias files|description
__GASH_CARD_TOPICS=(
    "files|files||Largest files and folders, tree stats, archives, backups"
    "system|system sysinfo utils|safety colors general|Processes, ports, services, disks, history, WSL"
    "nav||navigation|Directory navigation and listings"
    "git|git|git|Sync, tags, patches, repository repair, git shortcuts"
    "docker|docker docker-compose|docker|Containers and Compose stacks"
    "remote|remote ssh||Hosts from ~/.gash_env: exec, push, pull, sync"
    "dev|php node chrome|php|PHP and Node.js versions, headless Chrome"
    "llm|llm||Read-only JSON tools for AI agents"
    "ai|ai||Ask Claude or Gemini from the terminal"
    "gash|gash||Help, doctor, configuration, upgrade"
)

# Static alias list of one lib/aliases file: "name<TAB>expansion" lines.
# The expansion of an alias defined in this shell is taken from the shell
# (it shows the real flags); functions use the comment line above them.
__gash_card_aliases() {
    local file="${GASH_DIR:-$HOME/.gash}/lib/aliases/${1-}.sh"
    [[ -r "$file" ]] || return 0
    local name value
    while IFS=$'\t' read -r name value; do
        [[ -n "${BASH_ALIASES[$name]+x}" ]] && value="${BASH_ALIASES[$name]}"
        # "ls -> ls" (color flags off) teaches nothing.
        [[ "$value" == "$name" ]] && continue
        printf '%s\t%s\n' "$name" "$value"
    done < <("${__GASH_AWK:-awk}" '
        function unq(s) {
            if (s ~ /^".*"$/ || s ~ /^\047.*\047$/) s = substr(s, 2, length(s) - 2)
            return s
        }
        /^[ \t]*#/ { comment = $0; sub(/^[ \t]*#[ \t]*/, "", comment); next }
        /^[ \t]*function [A-Za-z0-9_.-]+ \{/ {
            n = $0; sub(/^[ \t]*function /, "", n); sub(/ .*/, "", n)
            if (!(n in seen) && n != "help") { seen[n] = 1; print n "\t" comment }
            comment = ""; next
        }
        /^[ \t]*alias [^$"\047=]+=/ {
            line = $0; sub(/^[ \t]*alias /, "", line)
            n = line; sub(/=.*/, "", n)
            v = line; sub(/^[^=]*=/, "", v); sub(/[ \t]+#.*$/, "", v)
            v = unq(v)
            # Variables the file sets at load time, shown by what they hold.
            gsub(/\$__gash_hte/, "sudo phpX.Y hte-cli", v)
            gsub(/\$__gash_composer1/, "composer1", v)
            gsub(/\$__gash_composer/, "composer", v)
            gsub(/\$__gash_(php_latest|bin)/, "phpX.Y", v)
            gsub(/ ?\$__gash_[A-Za-z_]+/, "", v)
            if (n ~ /\$/ || v == "") next
            if (!(n in seen)) { seen[n] = 1; print n "\t" v }
            comment = ""; next
        }
        /__gash_add_docker_alias "[^"]*" "[^"]*" "[^"]*"/ {
            split($0, q, "\"")
            if (!(q[4] in seen)) { seen[q[4]] = 1; print q[4] "\t" q[6] }
            next
        }
        { comment = "" }
    ' "$file")
}

# Usage line of a registered function, without the function name.
__gash_card_usage() {
    local fn="$1" body="${__GASH_HELP_REGISTRY[$1]-}" line found=0
    while IFS= read -r line; do
        if ((found)); then
            line="${line#"${line%%[![:space:]]*}"}"
            [[ -z "$line" ]] && continue
            [[ "$line" == "$fn"* ]] && line="${line#"$fn"}" && line="${line# }"
            printf '%s' "$line"
            return 0
        fi
        [[ "$line" == USAGE* ]] && found=1
    done <<<"$body"
}

# Functions of the help modules MODULES (space separated), sorted.
__gash_card_functions() {
    local mod fn
    for fn in "${!__GASH_HELP_MODULE[@]}"; do
        for mod in $1; do
            if [[ "${__GASH_HELP_MODULE[$fn]}" == "$mod" ]]; then printf '%s\n' "$fn"; fi
        done
    done | sort
}

# Print one topic: functions table, then the alias grid.
__gash_card_topic() {
    local topic="$1" modules="$2" alias_files="$3" desc="$4"
    local A="${__GASH_COLOR_ACCENT}" G="${__GASH_GREEN}" M="${__GASH_COLOR_MUTED}" B="${__GASH_BOLD}" R="${__GASH_COLOR_OFF}"
    __gash_use_color || { A="" G="" M="" B="" R=""; }

    printf '\n  %s%s%s  %s%s%s\n\n' "$B" "${topic^^}" "$R" "$M" "$desc" "$R"

    local fns fn usage al
    fns="$(__gash_card_functions "$modules")"
    if [[ -n "$fns" ]]; then
        {
            printf 'FUNCTION\tALIAS\tUSAGE\tDESCRIPTION\n'
            while IFS= read -r fn; do
                usage="$(__gash_card_usage "$fn")"
                ((${#usage} > 30)) && usage="${usage:0:29}${__GASH_GLYPH_ELLIPSIS}"
                al="${__GASH_HELP_ALIASES[$fn]-}"
                printf '%s\t%s\t%s\t%s\n' "$fn" "${al// /, }" "$usage" "${__GASH_HELP_SHORT[$fn]-}"
            done <<<"$fns"
        } | __gash_table --indent 2 --fit --color 1:"$A" --color 2:"$G" --color 3:"$M"
    fi

    local f entries=() name value
    for f in $alias_files; do
        while IFS=$'\t' read -r name value; do
            [[ -n "$name" ]] && entries+=("$name"$'\t'"$value")
        done < <(__gash_card_aliases "$f")
    done
    ((${#entries[@]} > 0)) || return 0

    # Grid: as many "name  expansion" columns as the terminal holds.
    local namew=0 valw=0 e
    for e in "${entries[@]}"; do
        name="${e%%$'\t'*}" value="${e#*$'\t'}"
        ((${#name} > namew)) && namew=${#name}
        ((${#value} > valw)) && valw=${#value}
    done
    ((valw > 26)) && valw=26
    local cellw=$((namew + 2 + valw + 3)) cols
    cols=$((($(__gash_tty_width) - 2) / cellw))
    ((cols < 1)) && cols=1
    ((cols > 3)) && cols=3

    [[ -n "$fns" ]] && echo
    printf '  %sAliases%s\n' "$B" "$R"
    local i=0
    for e in "${entries[@]}"; do
        name="${e%%$'\t'*}" value="${e#*$'\t'}"
        ((${#value} > valw)) && value="${value:0:valw-1}${__GASH_GLYPH_ELLIPSIS}"
        ((i % cols == 0)) && printf '  '
        i=$((i + 1))
        if ((i % cols == 0 || i == ${#entries[@]})); then
            printf '%s%-*s%s  %s%s%s\n' "$G" "$namew" "$name" "$R" "$M" "$value" "$R"
        else
            printf '%s%-*s%s  %s%-*s%s   ' "$G" "$namew" "$name" "$R" "$M" "$valw" "$value" "$R"
        fi
    done
}

# Gash reference card, generated from the help registry and the alias files.
# Usage: gash [TOPIC|all] [--no-color]
gash() {
    if [[ "${1-}" == migrate ]]; then
        shift
        gash_migrate "$@"
        return
    fi
    local no_color=0 topic="" a
    for a in "$@"; do
        case "$a" in
            --no-color) no_color=1 ;;
            -h | --help) topic="--help" ;;
            *) topic="$a" ;;
        esac
    done
    __gash_want_colors "$no_color" || eval "$__GASH_COLOR_OFF_LOCALS"
    [[ "$topic" == "--help" ]] && {
        __gash_help_display gash
        return 0
    }
    case "$topic" in
        navigation) topic="nav" ;;
        management) topic="gash" ;;
        php | node) topic="dev" ;;
    esac

    local P="${__GASH_COLOR_PRIMARY}" B="${__GASH_BOLD}" M="${__GASH_COLOR_MUTED}" G="${__GASH_GREEN}" A="${__GASH_COLOR_ACCENT}" R="${__GASH_COLOR_OFF}"
    __gash_use_color || { P="" B="" M="" G="" A="" R=""; }
    local dot="$__GASH_GLYPH_DOT" entry t mods files desc

    if [[ -n "$topic" && "$topic" != all ]]; then
        for entry in "${__GASH_CARD_TOPICS[@]}"; do
            IFS='|' read -r t mods files desc <<<"$entry"
            [[ "$t" == "$topic" ]] && break
            t=""
        done
        if [[ -z "$t" ]]; then
            local names=() e
            for e in "${__GASH_CARD_TOPICS[@]}"; do names+=("${e%%|*}"); done
            __gash_error "Unknown topic: $topic. Topics: ${names[*]}, all"
            return 1
        fi
    fi

    local nfn=${#__GASH_HELP_REGISTRY[@]}
    echo
    __gash_box --color "$P" \
        "${B}Gash${R} ${GASH_VERSION-} ${M}${dot} Bash for humans and AI agents${R}" \
        "${M}${nfn} functions ${dot} gash <topic> ${dot} gash_help <name>${R}"

    if [[ -n "$topic" && "$topic" != all ]]; then
        __gash_card_topic "$t" "$mods" "$files" "$desc"
        echo
        return 0
    fi
    if [[ "$topic" == all ]]; then
        for entry in "${__GASH_CARD_TOPICS[@]}"; do
            IFS='|' read -r t mods files desc <<<"$entry"
            __gash_card_topic "$t" "$mods" "$files" "$desc"
        done
        echo
        return 0
    fi

    # Overview: one row per topic, then where to go next.
    echo
    {
        printf 'TOPIC\tFUNCTIONS\tALIASES\tWHAT\n'
        local nf na f
        for entry in "${__GASH_CARD_TOPICS[@]}"; do
            IFS='|' read -r t mods files desc <<<"$entry"
            nf=$(__gash_card_functions "$mods" | grep -c .)
            na=0
            for f in $files; do na=$((na + $(__gash_card_aliases "$f" | grep -c .))); done
            printf '%s\t%s\t%s\t%s\n' "$t" "$nf" "$na" "$desc"
        done
    } | __gash_table --indent 2 --right 2,3 --fit --color 1:"$A"
    echo
    printf '  %sgash <topic>%s        functions and aliases of a topic, e.g. %sgash git%s\n' "$G" "$R" "$G" "$R"
    printf '  %sgash all%s            the whole card\n' "$G" "$R"
    printf '  %sgash_help <name>%s    full help with examples, e.g. %sgash_help flf%s\n' "$G" "$R" "$G" "$R"
    printf '  %sgash_help --search%s  search every help page\n' "$G" "$R"
    printf '  %sgash_doctor%s         check the installation\n' "$G" "$R"
    echo
}

# -----------------------------------------------------------------------------
# Help
# -----------------------------------------------------------------------------

# Gash help system with examples, search, and discovery.
# Usage: gash_help [--no-color] [FUNCTION|ALIAS|--list|--search KEYWORD|--short FUNCTION|--bash TOPIC]
gash_help() {
    # Extract --no-color from anywhere in the arg list; leave other args intact.
    local no_color=0
    local -a filtered=()
    local a
    for a in "$@"; do
        case "$a" in
            --no-color) no_color=1 ;;
            *) filtered+=("$a") ;;
        esac
    done
    set -- "${filtered[@]}"

    local arg="${1-}"

    # If colors must be suppressed (env, TTY, or --no-color flag), locally
    # shadow the module-wide __GASH_* vars to empty. Propagates via dynamic
    # scoping to __gash_help_display / _search / _list called below.
    __gash_want_colors "$no_color" || eval "$__GASH_COLOR_OFF_LOCALS"

    local A="${__GASH_COLOR_ACCENT-\033[38;5;214m}"
    local W="${__GASH_BOLD_WHITE-\033[1;37m}"
    local C="${__GASH_CYAN-\033[0;36m}"
    local G="${__GASH_GREEN-\033[0;32m}"
    local M="${__GASH_COLOR_MUTED-\033[38;5;245m}"
    local R="${__GASH_COLOR_OFF-\033[0m}"

    # --bash: delegate to builtin help
    if [[ "$arg" == "--bash" ]]; then
        shift
        builtin help "$@"
        return $?
    fi

    # --list: grouped listing
    if [[ "$arg" == "--list" ]]; then
        echo
        echo -e "  ${W}Gash Functions${R} ${M}(use 'gash_help FUNCTION' for details)${R}"
        __gash_help_list
        echo
        return 0
    fi

    # --search: keyword search
    if [[ "$arg" == "--search" ]]; then
        local keyword="${2-}"
        if [[ -z "$keyword" ]]; then
            __gash_error "Usage: gash_help --search KEYWORD"
            return 1
        fi
        echo
        echo -e "  ${W}Search results for '${keyword}':${R}"
        echo
        local rc=0
        __gash_help_search "$keyword" || rc=$?
        echo
        return "$rc"
    fi

    # --short: one-line description
    if [[ "$arg" == "--short" ]]; then
        local func="${2-}"
        if [[ -z "$func" ]]; then
            __gash_error "Usage: gash_help --short FUNCTION"
            return 1
        fi
        # Try direct lookup
        if [[ -n "${__GASH_HELP_SHORT[$func]+x}" ]]; then
            printf '%s\n' "${__GASH_HELP_SHORT[$func]}"
            return 0
        fi
        # Try alias reverse lookup
        local resolved
        resolved="$(__gash_help_find_by_alias "$func")" || true
        if [[ -n "$resolved" && -n "${__GASH_HELP_SHORT[$resolved]+x}" ]]; then
            printf '%s\n' "${__GASH_HELP_SHORT[$resolved]}"
            return 0
        fi
        __gash_error "No help found for '$func'."
        return 1
    fi

    # -h / --help: show self-help
    if [[ "$arg" == "-h" || "$arg" == "--help" ]]; then
        echo
        echo -e "  ${A}gash_help${R}${M}  (module: gash)${R}"
        echo
        echo -e "  ${W}Gash help system with examples, search, and discovery.${R}"
        echo
        echo -e "  ${W}USAGE${R}"
        echo -e "    ${C}gash_help${R}                   ${M}Overview of all functions${R}"
        echo -e "    ${C}gash_help FUNCTION${R}          ${M}Detailed help with examples${R}"
        echo -e "    ${C}gash_help ALIAS${R}             ${M}Lookup by short alias name${R}"
        echo -e "    ${C}gash_help --list${R}            ${M}List all functions by module${R}"
        echo -e "    ${C}gash_help --search KEYWORD${R}  ${M}Search across all help text${R}"
        echo -e "    ${C}gash_help --short FUNCTION${R}  ${M}One-line description only${R}"
        echo -e "    ${C}gash_help --bash TOPIC${R}      ${M}Bash builtin help${R}"
        echo
        return 0
    fi

    # No args: show overview
    if [[ -z "$arg" ]]; then
        echo
        echo -e "  ${W}Gash${R} ${M}v${GASH_VERSION:-?}${R} ${M}- Gash, Another SHell!${R}"
        echo
        echo -e "  ${C}gash_help FUNCTION${R}          ${M}Detailed help with examples${R}"
        echo -e "  ${C}gash_help ALIAS${R}             ${M}Lookup by short alias (e.g. flf, dsa, ask)${R}"
        echo -e "  ${C}gash_help --list${R}            ${M}List all functions by module${R}"
        echo -e "  ${C}gash_help --search KEYWORD${R}  ${M}Search across all help text${R}"
        echo -e "  ${C}gash${R}                        ${M}Reference card with aliases${R}"
        echo -e "  ${C}FUNCTION --help${R}             ${M}Quick usage reminder${R}"

        # Show grouped list
        __gash_help_list
        echo
        echo -e "  ${M}Tip: Every function supports -h/--help for a quick usage reminder.${R}"
        echo -e "  ${M}     Use gash_help FUNCTION for detailed help with real-world examples.${R}"
        echo
        return 0
    fi

    # Specific function/alias lookup
    # Try direct lookup first
    if [[ -n "${__GASH_HELP_REGISTRY[$arg]+x}" ]]; then
        __gash_help_display "$arg"
        return 0
    fi

    # Try alias reverse lookup
    local resolved
    resolved="$(__gash_help_find_by_alias "$arg")" || true
    if [[ -n "$resolved" ]]; then
        echo -e "\n  ${M}'${arg}' is an alias for '${resolved}'${R}"
        __gash_help_display "$resolved"
        return 0
    fi

    # Not found
    __gash_error "No help found for '$arg'."
    echo -e "  ${M}Try: gash_help --list  or  gash_help --search ${arg}${R}" >&2
    return 1
}

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

    __gash_register_help "gash" \
        --module "gash" \
        --short "Gash reference card with all functions and aliases" \
        --see-also "gash_help gash_doctor" \
        <<'HELP'
USAGE
  gash [TOPIC|all] [--no-color]
  gash migrate [...]   Same as gash_migrate (Gash 1.x configuration)

  gash                 Overview: topics with function and alias counts
  gash TOPIC           Functions (alias, usage, description) and aliases of a topic
  gash all             The whole card, every topic

TOPICS
  files, system, nav, git, docker, remote, dev, llm, ai, gash
  (navigation, management, php and node still work as old names)

  The card is built from the help registry and the alias files, so it
  always lists exactly what this Gash ships.

EXAMPLES
  # Quick reminder of all git aliases
  gash git

  # See what Docker functions are available
  gash docker

  # See every function and alias in Gash, plain text for a file
  gash all --no-color > gash-card.txt

  # Check AI-related functions
  gash ai
HELP

    __gash_register_help "gash_help" \
        --module "gash" \
        --short "Help system with examples, search, and discovery" \
        --see-also "gash" \
        <<'HELP'
USAGE
  gash_help                    Overview of all functions
  gash_help FUNCTION           Detailed help with real-world examples
  gash_help ALIAS              Lookup by short alias name
  gash_help --list             List all functions grouped by module
  gash_help --search KEYWORD   Search across all help text
  gash_help --short FUNCTION   One-line description (for scripting)
  gash_help --bash TOPIC       Bash builtin help (e.g. cd, export)

EXAMPLES
  # Get detailed help for a function
  gash_help ai_query

  # Don't remember the long name? Use the alias
  gash_help flf

  # Find all Docker-related functions
  gash_help --search docker

  # Find functions that work with pipes
  gash_help --search pipe

  # Get just the description for scripting
  gash_help --short files_largest

  # Access bash builtin help
  gash_help --bash cd
HELP

    __gash_register_help "gash_doctor" \
        --aliases "gdoctor" \
        --module "gash" \
        --short "Run health checks on the Gash installation" \
        --see-also "gash_upgrade gash" \
        <<'HELP'
USAGE
  gash_doctor [--timing] [--no-color]

OPTIONS
  --timing    What each file and step costs when a shell starts
  --no-color  Plain output

EXAMPLES
  # Run a full health check
  gash_doctor

  # Find what makes new shells slow
  gash_doctor --timing

NOTES
  Checks: core files, modules, config file permissions,
  external tools (git, curl, jq, sqlite3, docker).
  Reports missing or misconfigured components.

  --timing loads Gash in a new interactive shell and lists the
  milliseconds each file and step takes (5 ms or more stands out),
  then times whole new shells: with your ~/.bashrc, and bash alone.
  Needs Bash 5.0 or newer. Starting a shell with GASH_TIMING=1 keeps
  the same figures in its __GASH_LOAD_TIMES array.
HELP

    __gash_register_help "gash_upgrade" \
        --module "gash" \
        --short "Upgrade Gash to the latest version" \
        --see-also "gash_doctor gash_uninstall" \
        <<'HELP'
USAGE
  gash_upgrade

EXAMPLES
  # Upgrade to latest version
  gash_upgrade
  # Then restart your terminal to apply changes

NOTES
  Fetches the latest git tag, checks out that version,
  and cleans up. Restart your shell after upgrading.
HELP

    __gash_register_help "gash_uninstall" \
        --module "gash" \
        --short "Uninstall Gash and clean up configurations" \
        --see-also "gash_upgrade gash_unload" \
        <<'HELP'
USAGE
  gash_uninstall

EXAMPLES
  # Remove Gash completely (interactive confirmation)
  gash_uninstall

NOTES
  Removes the Gash sourcing block from your shell profile,
  deletes the installation directory, and cleans up.
  Restart your terminal after uninstalling.
HELP

    __gash_register_help "gash_unload" \
        --module "gash" \
        --short "Restore shell state to before Gash was loaded" \
        --see-also "gash_uninstall" \
        <<'HELP'
USAGE
  gash_unload

EXAMPLES
  # Temporarily disable Gash in the current terminal
  gash_unload

  # Re-enable Gash without restarting
  source ~/.gashrc

NOTES
  Removes all functions, aliases, and settings introduced by Gash.
  Restores PS1, PROMPT_COMMAND, history settings, and shopt options
  to the state captured before Gash loaded. Session-only: does not
  modify any files.
HELP

    __gash_register_help "gash_inspiring_quote" \
        --module "gash" \
        --short "Display a random quote, optionally with its source" \
        <<'HELP'
USAGE
  gash_inspiring_quote [--source]

EXAMPLES
  # The quote shown under the banner
  gash_inspiring_quote

  # Where it was said or written
  gash_inspiring_quote --source

NOTES
  Quotes live in quotes/list.txt: one `"Text." — Author` line each,
  under a `# ` line with the source (book, essay, talk, mail; a link
  where one exists). New quotes need a source that can be checked;
  see CONTRIBUTING.md.
HELP

    __gash_register_help "gash_env_init" \
        --module "gash" \
        --short "Create ~/.gash_env configuration template" \
        --see-also "gash_db_list gash_ai_list gash_ssh_auto_unlock" \
        <<'HELP'
USAGE
  gash_env_init [--force]

EXAMPLES
  # Create config from template (will not overwrite existing)
  gash_env_init

  # Overwrite existing config with fresh template
  gash_env_init --force

NOTES
  Creates ~/.gash_env with template sections for:
    SSH:~/.ssh/id_ed25519=your_passphrase
    DB:default=mysql://user:pass@host:port/db
    AI:claude=sk-ant-api03-xxxxx
  Always set permissions: chmod 600 ~/.gash_env
HELP

    __gash_register_help "gash_db_list" \
        --module "gash" \
        --short "List configured database connections" \
        --see-also "gash_db_test gash_env_init" \
        <<'HELP'
USAGE
  gash_db_list

EXAMPLES
  # Show all database connections from ~/.gash_env
  gash_db_list

NOTES
  Passwords are masked in output.
  Configure connections in ~/.gash_env with:
    DB:name=driver://user:pass@host:port/database
  Supported drivers: mysql, mariadb, pgsql
HELP

    __gash_register_help "gash_db_test" \
        --module "gash" \
        --short "Test a database connection" \
        --see-also "gash_db_list gash_env_init" \
        <<'HELP'
USAGE
  gash_db_test [CONNECTION_NAME]

EXAMPLES
  # Test the default connection
  gash_db_test

  # Test a named connection
  gash_db_test postgres
  gash_db_test legacy
HELP

    __gash_register_help "gash_ai_list" \
        --module "gash" \
        --short "List configured AI providers" \
        --see-also "ai_ask ai_query gash_env_init" \
        <<'HELP'
USAGE
  gash_ai_list

EXAMPLES
  # Show configured AI providers
  gash_ai_list

NOTES
  Configure providers in ~/.gash_env with:
    AI:claude=sk-ant-api03-xxxxx
    AI:gemini=AIzaSyxxxxx
  The first configured provider is used by default.
HELP

fi # end help registration guard
