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

# Display an inspiring quote from the quotes file.
# Usage: gash_inspiring_quote
gash_inspiring_quote() {
    local tty_width
    tty_width="$(__gash_tty_width)"

    local gash_dir
    gash_dir="${GASH_DIR:-$HOME/.gash}"
    local quotes_file="$gash_dir/quotes/list.txt"

    if [[ ! -r "$quotes_file" ]]; then
        return 1
    fi

    local -a quotes
    mapfile -t quotes < "$quotes_file"

    if [[ ${#quotes[@]} -eq 0 ]]; then
        return 1
    fi

    local idx=$(( RANDOM % ${#quotes[@]} ))
    local quote="${quotes[$idx]}"

    local visible_prefix="Quote: "
    local prefix_len=${#visible_prefix}
    local wrap_width=$(( tty_width - prefix_len ))
    if (( wrap_width < 20 )); then
        wrap_width=80
    fi

    local first=1
    while IFS= read -r qline; do
        if (( first )); then
            printf '%b%s\n' "${__GASH_CYAN}Quote:${__GASH_COLOR_OFF} " "$qline"
            first=0
        else
            printf '%s%s\n' "       " "$qline"
        fi
    done < <(printf '%s\n' "$quote" | fold -s -w "$wrap_width")
}

# -----------------------------------------------------------------------------
# Upgrade
# -----------------------------------------------------------------------------

# Upgrade Gash to the latest version.
# Usage: gash_upgrade
gash_upgrade() {
    local install_dir="${GASH_DIR:-$HOME/.gash}"

    if [ ! -d "$install_dir" ]; then
        __gash_error "Failed to change directory to $install_dir"
        return 1
    fi

    if [ ! -d "$install_dir/.git" ]; then
        __gash_error "Gash is not installed. Please refer to https://github.com/mauriziofonte/gash."
        return 1
    fi

    local current_dir
    current_dir=$(pwd)

    __gash_info "Upgrading Gash in $install_dir..."
    cd "$install_dir" || { __gash_error "Failed to change directory to $install_dir"; return 1; }

    if ! command git fetch --tags origin; then
        cd "$current_dir" || { __gash_error "Failed to change directory to $current_dir"; return 1; }
        __gash_error "Failed to fetch updates. Please check your network connection and try manually with 'git fetch'."
        return 1
    fi

    local latest_tag
    latest_tag="$(git for-each-ref --sort=-creatordate --format='%(refname:short)' refs/tags | head -n1)"

    if [ -z "$latest_tag" ]; then
        __gash_warning "No tags found. Using the latest commit on the default branch."
        cd "$current_dir" || { __gash_error "Failed to change directory to $current_dir"; return 1; }
        return 0
    fi

    local current_tag
    current_tag="$(git describe --tags --abbrev=0 2>/dev/null)"

    if [ "$current_tag" = "$latest_tag" ]; then
        local release_date
        release_date="$(git log -1 --format=%ai "$current_tag")"
        __gash_success "Gash is already up-to-date ($current_tag, released on $release_date)."
        cd "$current_dir" || { __gash_error "Failed to change directory to $current_dir"; return 1; }
        return 0
    fi

    if ! git checkout "$latest_tag" >/dev/null 2>&1; then
        __gash_error "Failed to checkout tag $latest_tag."
        cd "$current_dir" || { __gash_error "Failed to change directory to $current_dir"; return 1; }
        return 1
    fi

    if ! git reset --hard "$latest_tag" >/dev/null 2>&1; then
        __gash_error "Failed to reset to $latest_tag."
        cd "$current_dir" || { __gash_error "Failed to change directory to $current_dir"; return 1; }
        return 1
    fi

    __gash_success "Upgraded Gash to version $latest_tag."

    __gash_info "Cleaning up Git repository..."
    if ! git reflog expire --expire=now --all; then
        cd "$current_dir" || { __gash_error "Failed to change directory to $current_dir"; return 1; }
        __gash_error "Your version of git is out of date. Please update it!"
    fi
    if ! git gc --auto --aggressive --prune=now; then
        cd "$current_dir" || { __gash_error "Failed to change directory to $current_dir"; return 1; }
        __gash_error "Your version of git is out of date. Please update it!"
    fi

    cd "$current_dir" || { __gash_error "Failed to change directory to $current_dir"; return 1; }
    __gash_success "Gash upgrade completed."
}

# -----------------------------------------------------------------------------
# Uninstall
# -----------------------------------------------------------------------------

# Uninstall Gash and clean up configurations.
# Usage: gash_uninstall
gash_uninstall() {
    local gash_dir="${GASH_DIR:-$HOME/.gash}"

    if [ ! -d "$gash_dir" ]; then
        __gash_error "Gash is not installed on this system."
        return 1
    fi

    __gash_warning "This will remove Gash and its configuration from this account."
    if ! needs_confirm_prompt "Continue?"; then
        __gash_info "Uninstall cancelled."
        return 0
    fi

    local profile_files=( "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.profile" )

    # Remove Gash sourcing lines from profiles
    # Uses unique delimiters to avoid matching other if/fi blocks
    for profile_file in "${profile_files[@]}"; do
        if [ -f "$profile_file" ]; then
            if command grep -qc 'source.*[./]gashrc' "$profile_file"; then
                __gash_info "Removing Gash block from: $profile_file"
                # Try new format with delimiters first
                if grep -q '# >>> GASH START >>>' "$profile_file"; then
                    sed -i.bak '/# >>> GASH START >>>/,/# <<< GASH END <<</d' "$profile_file"
                # Fallback: old format - use more specific pattern to avoid catching other fi's
                elif grep -q '# Load Gash Bash' "$profile_file"; then
                    # Remove exactly: comment + if block + fi (4 lines total)
                    sed -i.bak '/# Load Gash Bash/{N;N;N;N;d}' "$profile_file"
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
        __gash_info "~/.gashrc not found; skipping."
    fi

    # Remove Gash directory
    if [ -d "$gash_dir" ]; then
        __gash_info "Removing Gash at $gash_dir..."
        if ! rm -rf "$gash_dir" 2>/dev/null; then
            __gash_error "Failed to remove $gash_dir; please remove it manually."
        fi
    else
        __gash_info "$gash_dir not found; skipping."
    fi

    # Handle ~/.gash_env (contains credentials - ask user)
    if [ -f "$HOME/.gash_env" ]; then
        __gash_warning "Found ~/.gash_env (may contain database credentials)."
        if needs_confirm_prompt "Remove ~/.gash_env?"; then
            if rm -f "$HOME/.gash_env" 2>/dev/null; then
                __gash_info "Removed ~/.gash_env"
            else
                __gash_error "Failed to remove ~/.gash_env; please remove it manually."
            fi
        else
            __gash_info "Kept ~/.gash_env"
        fi
    fi

    __gash_success "Gash uninstalled."
    __gash_info "Restart your terminal to apply changes."

    # Clean up backup files
    for profile_file in "${profile_files[@]}"; do
        if [ -f "${profile_file}.bak" ]; then
            rm -f "${profile_file}.bak"
        fi
    done
}

# -----------------------------------------------------------------------------
# Unload
# -----------------------------------------------------------------------------

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

    # Restore prompt-related variables
    if [[ "${__GASH_ORIG_PROMPT_COMMAND_SET-0}" -eq 1 ]]; then
        PROMPT_COMMAND="${__GASH_ORIG_PROMPT_COMMAND-}"
    else
        unset PROMPT_COMMAND
    fi

    if [[ "${__GASH_ORIG_PS1_SET-0}" -eq 1 ]]; then
        PS1="${__GASH_ORIG_PS1-}"
    else
        unset PS1
    fi

    if [[ "${__GASH_ORIG_PS2_SET-0}" -eq 1 ]]; then
        PS2="${__GASH_ORIG_PS2-}"
    else
        unset PS2
    fi

    # Restore history-related settings
    if [[ "${__GASH_ORIG_HISTCONTROL_SET-0}" -eq 1 ]]; then
        HISTCONTROL="${__GASH_ORIG_HISTCONTROL-}"
    else
        unset HISTCONTROL
    fi

    if [[ "${__GASH_ORIG_HISTTIMEFORMAT_SET-0}" -eq 1 ]]; then
        HISTTIMEFORMAT="${__GASH_ORIG_HISTTIMEFORMAT-}"
    else
        unset HISTTIMEFORMAT
    fi

    if [[ "${__GASH_ORIG_HISTSIZE_SET-0}" -eq 1 ]]; then
        HISTSIZE="${__GASH_ORIG_HISTSIZE-}"
    else
        unset HISTSIZE
    fi

    if [[ "${__GASH_ORIG_HISTFILESIZE_SET-0}" -eq 1 ]]; then
        HISTFILESIZE="${__GASH_ORIG_HISTFILESIZE-}"
    else
        unset HISTFILESIZE
    fi

    if [[ "${__GASH_ORIG_SHOPT_HISTAPPEND-1}" -eq 0 ]]; then
        shopt -s histappend
    else
        shopt -u histappend
    fi

    if [[ "${__GASH_ORIG_SHOPT_CHECKWINSIZE-1}" -eq 0 ]]; then
        shopt -s checkwinsize
    else
        shopt -u checkwinsize
    fi

    if [[ -n "${__GASH_ORIG_UMASK-}" ]]; then
        umask "${__GASH_ORIG_UMASK}"
    fi

    # Remove aliases introduced by Gash
    if [[ -n "${__GASH_ADDED_ALIASES-}" ]]; then
        local __gash_name
        while IFS= read -r __gash_name; do
            [[ -z "${__gash_name-}" ]] && continue
            unalias "${__gash_name}" >/dev/null 2>&1 || true
        done <<< "${__GASH_ADDED_ALIASES}"
    fi

    # Remove functions introduced by Gash
    if [[ -n "${__GASH_ADDED_FUNCS-}" ]]; then
        local __gash_name
        while IFS= read -r __gash_name; do
            [[ -z "${__gash_name-}" ]] && continue
            [[ "${__gash_name}" == "gash_unload" ]] && continue
            unset -f "${__gash_name}" >/dev/null 2>&1 || true
        done <<< "${__GASH_ADDED_FUNCS}"
    fi

    # Clear internal snapshot variables
    unset __GASH_SNAPSHOT_TAKEN __GASH_PRE_FUNCS __GASH_PRE_ALIASES __GASH_ADDED_FUNCS __GASH_ADDED_ALIASES \
        __GASH_ORIG_PS1_SET __GASH_ORIG_PS1 __GASH_ORIG_PS2_SET __GASH_ORIG_PS2 __GASH_ORIG_PROMPT_COMMAND_SET __GASH_ORIG_PROMPT_COMMAND \
        __GASH_ORIG_HISTCONTROL_SET __GASH_ORIG_HISTCONTROL __GASH_ORIG_HISTTIMEFORMAT_SET __GASH_ORIG_HISTTIMEFORMAT \
        __GASH_ORIG_HISTSIZE_SET __GASH_ORIG_HISTSIZE __GASH_ORIG_HISTFILESIZE_SET __GASH_ORIG_HISTFILESIZE \
        __GASH_ORIG_SHOPT_HISTAPPEND __GASH_ORIG_SHOPT_CHECKWINSIZE __GASH_ORIG_UMASK

    unset __gash_name

    # Finally, remove gash_unload itself
    unset -f gash_unload >/dev/null 2>&1 || true

    return 0
}

# -----------------------------------------------------------------------------
# Health Check
# -----------------------------------------------------------------------------

# Run health checks on the Gash installation.
# Verifies core files, modules, config permissions, and external tools.
# Usage: gash_doctor [--no-color]
gash_doctor() {
    needs_help "gash_doctor" "gash_doctor [--no-color]" "Run health checks on your Gash installation." "${1-}" && return

    local no_color=0
    local a
    for a in "$@"; do
        case "$a" in
            --no-color) no_color=1 ;;
        esac
    done

    # Local color scope (respects env, TTY, --no-color)
    eval "$(__gash_color_scope "$no_color")"

    local gash_dir="${GASH_DIR:-$HOME/.gash}"
    local issues=0

    __gash_info "Gash Doctor — checking installation (v${GASH_VERSION:-unknown})..."
    echo ""

    # 1. Core files
    echo -e "${__GASH_BOLD_WHITE-\e[1;37m}Core Files:${__GASH_COLOR_OFF-\033[0m}"
    local core_files=("$gash_dir/gash.sh" "$gash_dir/lib/core/config.sh" "$gash_dir/lib/core/output.sh" "$gash_dir/lib/core/utils.sh" "$gash_dir/lib/core/validation.sh")
    for f in "${core_files[@]}"; do
        if [[ -f "$f" ]]; then
            __gash_success "  $(basename "$f")"
        else
            __gash_error "  $(basename "$f") — MISSING"
            ((issues++))
        fi
    done

    # 2. Modules
    echo -e "${__GASH_BOLD_WHITE-\e[1;37m}Modules:${__GASH_COLOR_OFF-\033[0m}"
    local expected_modules=(gash git ssh files docker docker-compose system llm ai sysinfo)
    for mod in "${expected_modules[@]}"; do
        if [[ -f "$gash_dir/lib/modules/${mod}.sh" ]]; then
            __gash_success "  ${mod}"
        else
            __gash_warning "  ${mod} — not found"
            ((issues++))
        fi
    done

    # 3. Config file
    echo -e "${__GASH_BOLD_WHITE-\e[1;37m}Configuration:${__GASH_COLOR_OFF-\033[0m}"
    local env_file="${GASH_ENV_FILE:-$HOME/.gash_env}"
    if [[ -f "$env_file" ]]; then
        local perms
        perms=$(stat -c '%a' "$env_file" 2>/dev/null || stat -f '%Lp' "$env_file" 2>/dev/null || echo "unknown")
        if [[ "$perms" == "600" ]]; then
            __gash_success "  $env_file (permissions: $perms)"
        else
            __gash_warning "  $env_file (permissions: $perms — should be 600)"
            ((issues++))
        fi
    else
        __gash_info "  $env_file — not found (optional, run gash_env_init to create)"
    fi

    # 4. External tools
    echo -e "${__GASH_BOLD_WHITE-\e[1;37m}External Tools:${__GASH_COLOR_OFF-\033[0m}"
    local tools=(git curl jq sqlite3 docker)
    for tool in "${tools[@]}"; do
        if type -P "$tool" >/dev/null 2>&1; then
            __gash_success "  $tool ($(type -P "$tool"))"
        else
            __gash_warning "  $tool — not found (some features may be unavailable)"
        fi
    done

    # 5. Summary
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

# Display comprehensive Gash reference card with all functions and aliases.
# Usage: gash [SECTION]
# Sections: git, files, system, docker, nav, llm, all (default: summary)
gash() {
    local section="${1:-}"
    local W='\033[1;37m'    # White bold
    local C='\033[0;36m'    # Cyan
    local Y='\033[0;33m'    # Yellow
    local G='\033[0;32m'    # Green
    local M='\033[0;35m'    # Magenta
    local D='\033[0;90m'    # Dark gray
    local R='\033[0m'       # Reset

    # Header
    __gash_ref_header() {
        echo
        echo -e "${W}╔══════════════════════════════════════════════════════════════════════════════╗${R}"
        echo -e "${W}║${R}  ${C}G${Y}a${M}s${G}h${R} ${W}Reference Card${R}                                        ${D}v${GASH_VERSION:-1.3.3}${R}  ${W}║${R}"
        echo -e "${W}╚══════════════════════════════════════════════════════════════════════════════╝${R}"
    }

    # Section header
    __gash_ref_section() {
        echo
        echo -e "${W}━━━ $1 ━━━${R}"
    }

    # Function entry: name (alias) args - description
    __gash_ref_fn() {
        local name="$1" alias="$2" args="$3" desc="$4"
        if [[ -n "$alias" ]]; then
            printf "  ${Y}%-18s${R} ${G}%-6s${R} ${C}%-20s${R} ${D}%s${R}\n" "$name" "($alias)" "$args" "$desc"
        else
            printf "  ${Y}%-18s${R} ${D}%-6s${R} ${C}%-20s${R} ${D}%s${R}\n" "$name" "" "$args" "$desc"
        fi
    }

    # Alias entry: alias - description
    __gash_ref_alias() {
        local name="$1" desc="$2"
        printf "  ${G}%-12s${R} ${D}%s${R}\n" "$name" "$desc"
    }

    # Git section
    __gash_ref_git() {
        __gash_ref_section "GIT"

        echo -e "  ${W}Functions:${R}"
        __gash_ref_fn "git_list_tags" "glt" "" "List all local and remote tags"
        __gash_ref_fn "git_add_tag" "gat" "TAG [MSG]" "Create and push annotated tag"
        __gash_ref_fn "git_delete_tag" "gdt" "TAG" "Delete tag locally and remote"
        __gash_ref_fn "git_dump_revisions" "gdr" "FILE" "Dump all revisions of a file"
        __gash_ref_fn "git_apply_patch" "gap" "MAIN FEAT COMMIT" "Apply patch from feature branch"

        echo -e "  ${W}Log (run gl --help):${R}"
        __gash_ref_alias "gl" "Compact log with graph"
        __gash_ref_alias "gla" "All branches with graph"
        __gash_ref_alias "glo" "Ultra-compact oneline"
        __gash_ref_alias "glg" "Graph focused (first-parent)"
        __gash_ref_alias "gls" "Log with file statistics"
        __gash_ref_alias "glf FILE" "File history with patches"

        echo -e "  ${W}Status & Diff:${R}"
        __gash_ref_alias "gst" "git status"
        __gash_ref_alias "gs" "git status -sb (short)"
        __gash_ref_alias "gd" "git diff"
        __gash_ref_alias "gds" "git diff --staged"
        __gash_ref_alias "gdw" "git diff --word-diff"

        echo -e "  ${W}Add & Commit:${R}"
        __gash_ref_alias "ga" "git add"
        __gash_ref_alias "gaa" "git add --all"
        __gash_ref_alias "gc" "git commit"
        __gash_ref_alias "gcm" "git commit -m"
        __gash_ref_alias "gca" "git commit --amend"
        __gash_ref_alias "gcan" "git commit --amend --no-edit"

        echo -e "  ${W}Push & Pull:${R}"
        __gash_ref_alias "gp" "git push"
        __gash_ref_alias "gpf" "git push --force-with-lease"
        __gash_ref_alias "gpl" "git pull"
        __gash_ref_alias "gplr" "git pull --rebase"

        echo -e "  ${W}Branch:${R}"
        __gash_ref_alias "gb" "git branch"
        __gash_ref_alias "gba" "git branch -a"
        __gash_ref_alias "gbd" "git branch -d"
        __gash_ref_alias "gbD" "git branch -D (force)"
        __gash_ref_alias "gco" "git checkout"
        __gash_ref_alias "gcb" "git checkout -b"
        __gash_ref_alias "gsw" "git switch"
        __gash_ref_alias "gswc" "git switch -c"

        echo -e "  ${W}Stash:${R}"
        __gash_ref_alias "gsh" "git stash"
        __gash_ref_alias "gshp" "git stash pop"
        __gash_ref_alias "gshl" "git stash list"
        __gash_ref_alias "gsha" "git stash apply"

        echo -e "  ${W}Reset & Rebase:${R}"
        __gash_ref_alias "grh" "git reset HEAD"
        __gash_ref_alias "grh1" "git reset HEAD~1"
        __gash_ref_alias "grhh" "git reset --hard HEAD"
        __gash_ref_alias "grb" "git rebase"
        __gash_ref_alias "grbc" "git rebase --continue"
        __gash_ref_alias "grba" "git rebase --abort"

        echo -e "  ${W}Remote:${R}"
        __gash_ref_alias "gf" "git fetch"
        __gash_ref_alias "gfa" "git fetch --all --prune"
        __gash_ref_alias "gr" "git remote -v"
    }

    # Files section
    __gash_ref_files() {
        __gash_ref_section "FILES"

        __gash_ref_fn "files_largest" "flf" "[PATH]" "List top 100 largest files"
        __gash_ref_fn "dirs_largest" "dld" "[PATH]" "List top 100 largest directories"
        __gash_ref_fn "dirs_find_large" "dfl" "[--size S] [DIR]" "Find dirs larger than SIZE"
        __gash_ref_fn "dirs_list_empty" "dle" "[PATH]" "List all empty directories"
        __gash_ref_fn "archive_extract" "axe" "FILE [DIR]" "Extract archives (tar/zip/gz/...)"
        __gash_ref_fn "file_backup" "fbk" "FILE" "Create timestamped backup"
    }

    # System section
    __gash_ref_system() {
        __gash_ref_section "SYSTEM"

        echo -e "  ${W}Process & Ports:${R}"
        __gash_ref_fn "process_find" "pf" "NAME" "Search for process by name"
        __gash_ref_fn "process_kill" "pk" "NAME" "Kill all processes by name"
        __gash_ref_fn "port_kill" "ptk" "PORT" "Kill processes on port"
        __gash_ref_fn "services_stop" "svs" "[--force]" "Stop Apache/MySQL/Redis/Docker"

        echo -e "  ${W}History:${R}"
        __gash_ref_fn "history_grep" "hg" "PATTERN" "Search history (colored)"
        __gash_ref_fn "hgrep" "" "PATTERN [OPTS]" "Smart search: -n -j -E -c -r"
        echo -e "    ${D}Options: -n LIMIT  -j JSON  -E regex  -c count  -r reverse  -H no-color${R}"

        echo -e "  ${W}Info & Utils:${R}"
        __gash_ref_fn "disk_usage" "du2" "" "Disk usage by filesystem type"
        __gash_ref_fn "ip_public" "myip" "" "Get public IP address"
        __gash_ref_fn "sudo_last" "plz" "[CMD]" "Run last/given command with sudo"
        __gash_ref_fn "mkdir_cd" "mkcd" "DIR" "Create directory and cd into it"
    }

    # Docker section
    __gash_ref_docker() {
        __gash_ref_section "DOCKER"

        echo -e "  ${W}Functions:${R}"
        __gash_ref_fn "docker_stop_all" "dsa" "" "Stop all containers"
        __gash_ref_fn "docker_start_all" "daa" "" "Start all containers"
        __gash_ref_fn "docker_prune_all" "dpa" "" "Remove all resources"

        echo -e "  ${W}Container:${R}"
        __gash_ref_alias "dcls" "docker container ls -a"
        __gash_ref_alias "dclsr" "docker container ls (running)"
        __gash_ref_alias "dstop" "docker stop"
        __gash_ref_alias "dstart" "docker start"
        __gash_ref_alias "dexec" "docker exec -it"
        __gash_ref_alias "drm" "docker rm"
        __gash_ref_alias "dlogs" "docker logs -f"
        __gash_ref_alias "dinspect" "docker inspect"

        echo -e "  ${W}Image:${R}"
        __gash_ref_alias "dils" "docker image ls"
        __gash_ref_alias "drmi" "docker rmi"
        __gash_ref_alias "dirm" "docker image prune -a"

        echo -e "  ${W}Compose (basic):${R}"
        __gash_ref_alias "dc" "docker-compose"
        __gash_ref_alias "dcup" "docker-compose up -d"
        __gash_ref_alias "dcdown" "docker-compose down"
        __gash_ref_alias "dclogs" "docker-compose logs -f"
        __gash_ref_alias "dcps" "docker-compose ps"
        __gash_ref_alias "dcb" "docker-compose build"
        __gash_ref_alias "dcrestart" "docker-compose restart"

        echo -e "  ${W}Compose (smart upgrade):${R}"
        __gash_ref_fn "docker_compose_check" "dcc" "[PATH]" "Check for updates"
        __gash_ref_fn "docker_compose_upgrade" "dcup2" "[PATH] [--dry-run]" "Upgrade services"
        __gash_ref_fn "docker_compose_scan" "dcscan" "[PATH] [--depth N]" "Scan for compose files"
        echo -e "    ${D}Only upgrades mutable tags (latest, main). Use --force for pinned.${R}"
    }

    # Navigation section
    __gash_ref_nav() {
        __gash_ref_section "NAVIGATION"

        echo -e "  ${W}Listing:${R}"
        __gash_ref_alias "ll" "ls -l (long)"
        __gash_ref_alias "la" "ls -la (all)"
        __gash_ref_alias "lash" "ls -lash (detailed)"
        __gash_ref_alias "l" "ls -CF (compact)"
        __gash_ref_alias "sl" "ls (typo fix)"

        echo -e "  ${W}Directory:${R}"
        __gash_ref_alias ".." "cd .."
        __gash_ref_alias "..." "cd ../.."
        __gash_ref_alias "...." "cd ../../.."
        __gash_ref_alias ".4 / .5" "cd up 4/5 levels"
        __gash_ref_alias "cls" "clear"
        __gash_ref_alias "path" "Show PATH entries"

        if grep -qi "microsoft" /proc/version 2>/dev/null; then
            echo -e "  ${W}WSL:${R}"
            __gash_ref_fn "wsl_restart" "wr" "" "Restart WSL (saves history)"
            __gash_ref_fn "wsl_shutdown" "wsd" "" "Shutdown WSL (saves history)"
            __gash_ref_fn "wsl_explorer" "wex" "[PATH]" "Open Windows Explorer"
            __gash_ref_fn "wsl_taskmanager" "wtm" "" "Open Windows Task Manager"
        fi
    }

    # Gash management section
    __gash_ref_gash() {
        __gash_ref_section "GASH MANAGEMENT"

        __gash_ref_fn "gash" "" "[SECTION]" "This reference card"
        __gash_ref_fn "gash_help" "" "[TOPIC]" "Bash help + Gash commands"
        __gash_ref_fn "gash_upgrade" "" "" "Upgrade to latest version"
        __gash_ref_fn "gash_uninstall" "" "" "Uninstall Gash"
        __gash_ref_fn "gash_doctor" "gdoctor" "" "Health check installation"
        __gash_ref_fn "gash_unload" "" "" "Restore pre-Gash shell state"
        __gash_ref_fn "gash_inspiring_quote" "" "" "Display inspiring quote"
        __gash_ref_fn "gash_env_init" "" "" "Create ~/.gash_env template"
        __gash_ref_fn "gash_db_list" "" "" "List database connections"
        __gash_ref_fn "gash_db_test" "" "NAME" "Test database connection"
        __gash_ref_fn "gash_ssh_auto_unlock" "" "" "Auto-unlock SSH keys"
    }

    # LLM section
    __gash_ref_llm() {
        __gash_ref_section "LLM UTILITIES (for AI Agents)"
        echo -e "  ${D}All output JSON. No short aliases. Use via gash-exec.sh${R}"

        echo -e "  ${W}Files:${R}"
        __gash_ref_fn "llm_tree" "" "[--text] [PATH]" "Directory tree (JSON)"
        __gash_ref_fn "llm_find" "" "PATTERN [PATH]" "Find files"
        __gash_ref_fn "llm_grep" "" "PATTERN [PATH]" "Search code"
        __gash_ref_fn "llm_project" "" "[PATH]" "Detect project type"
        __gash_ref_fn "llm_config" "" "[PATH]" "Read config (no .env)"

        echo -e "  ${W}Database:${R}"
        __gash_ref_fn "llm_db_query" "" "\"SQL\" -c CONN" "Read-only SQL"
        __gash_ref_fn "llm_db_tables" "" "-c CONN" "List tables"
        __gash_ref_fn "llm_db_schema" "" "TABLE -c CONN" "Table schema"
        __gash_ref_fn "llm_db_sample" "" "TABLE -c CONN" "Sample rows"

        echo -e "  ${W}Git:${R}"
        __gash_ref_fn "llm_git_status" "" "" "Status (JSON)"
        __gash_ref_fn "llm_git_diff" "" "" "Diff stats (JSON)"
        __gash_ref_fn "llm_git_log" "" "[--limit N]" "Commits (JSON)"

        echo -e "  ${W}System:${R}"
        __gash_ref_fn "llm_exec" "" "COMMAND" "Safe command execution"
        __gash_ref_fn "llm_ports" "" "" "Listening ports"
        __gash_ref_fn "llm_procs" "" "[--name N]" "Processes"
        __gash_ref_fn "llm_env" "" "" "Env vars (no secrets)"

        echo -e "  ${W}Docker:${R}"
        __gash_ref_fn "llm_docker_check" "" "[PATH]" "Compose update check (JSON)"
    }

    # AI section
    __gash_ref_ai() {
        __gash_ref_section "AI-POWERED"

        echo -e "  ${W}Interactive:${R}"
        __gash_ref_fn "ai_ask" "ask" "[provider]" "Interactive AI chat"
        __gash_ref_fn "ai_query" "ask" "[provider] \"query\"" "Non-interactive AI query"

        echo -e "  ${W}System Analysis:${R}"
        __gash_ref_fn "ai_sysinfo" "sysinfo_ai" "[provider] [--raw]" "AI system analysis"
        __gash_ref_fn "sysinfo" "si" "[section] [--llm]" "System enumeration"
        echo -e "    ${D}Sections: identity storage services auth network security webstack mail infra system all${R}"
        echo -e "    ${D}--llm: compact output for AI token minimization${R}"
        echo -e "    ${D}--raw: dump collected data without API call${R}"
    }

    # PHP section
    __gash_ref_php() {
        __gash_ref_section "PHP & COMPOSER"
        echo -e "  ${D}Aliases: php[VERSION] composer[VERSION] (e.g., php81, composer82)${R}"
        echo -e "  ${D}Versions: 5.6, 7.0-7.4, 8.0-8.4 (if installed)${R}"
        echo -e "  ${D}Flags: -d allow_url_fopen=1 -d memory_limit=2048M${R}"

        __gash_ref_alias "composer-packages-update" "Global package update"
        __gash_ref_alias "composer-self-update" "Composer self-update"
    }

    # Summary (default)
    __gash_ref_summary() {
        __gash_ref_header
        echo
        echo -e "  ${W}Usage:${R} gash [section]"
        echo
        echo -e "  ${W}Sections:${R}"
        echo -e "    ${Y}git${R}      Git functions + 40+ aliases"
        echo -e "    ${Y}files${R}    File operations (find, backup, extract)"
        echo -e "    ${Y}system${R}   Process, ports, history, services"
        echo -e "    ${Y}docker${R}   Container management + compose"
        echo -e "    ${Y}nav${R}      Navigation aliases (ls, cd)"
        echo -e "    ${Y}llm${R}      LLM/AI agent utilities"
        echo -e "    ${Y}ai${R}       AI chat + system analysis"
        echo -e "    ${Y}php${R}      PHP & Composer version aliases"
        echo -e "    ${Y}gash${R}     Gash management functions"
        echo -e "    ${Y}all${R}      Show everything"
        echo
        echo -e "  ${W}Quick Reference:${R}"
        echo -e "    ${G}gl${R}       Git log          ${G}gst${R}      Git status"
        echo -e "    ${G}gaa${R}      Git add all      ${G}gcm${R}      Git commit -m"
        echo -e "    ${G}gp${R}       Git push         ${G}gpl${R}      Git pull"
        echo -e "    ${G}hgrep${R}    Smart history    ${G}pf${R}       Process find"
        echo -e "    ${G}flf${R}      Largest files    ${G}axe${R}      Extract archive"
        echo -e "    ${G}dsa${R}      Docker stop all  ${G}dcup${R}     Compose up"
        echo
        echo -e "  ${D}Tip: Run 'gash all' for complete reference${R}"
    }

    # Main dispatch
    case "$section" in
        git)    __gash_ref_header; __gash_ref_git ;;
        files)  __gash_ref_header; __gash_ref_files ;;
        system) __gash_ref_header; __gash_ref_system ;;
        docker) __gash_ref_header; __gash_ref_docker ;;
        nav|navigation) __gash_ref_header; __gash_ref_nav ;;
        llm)    __gash_ref_header; __gash_ref_llm ;;
        ai)     __gash_ref_header; __gash_ref_ai ;;
        php)    __gash_ref_header; __gash_ref_php ;;
        gash|management) __gash_ref_header; __gash_ref_gash ;;
        all)
            __gash_ref_header
            __gash_ref_git
            __gash_ref_files
            __gash_ref_system
            __gash_ref_docker
            __gash_ref_nav
            __gash_ref_php
            __gash_ref_llm
            __gash_ref_ai
            __gash_ref_gash
            ;;
        -h|--help|help)
            __gash_ref_summary
            ;;
        "")
            __gash_ref_summary
            ;;
        *)
            echo -e "${Y}Unknown section:${R} $section"
            echo -e "Run ${G}gash${R} to see available sections."
            return 1
            ;;
    esac

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
            *)          filtered+=("$a") ;;
        esac
    done
    set -- "${filtered[@]}"

    local arg="${1-}"

    # If colors must be suppressed (env, TTY, or --no-color flag), locally
    # shadow the module-wide __GASH_* vars to empty. Propagates via dynamic
    # scoping to __gash_help_display / _search / _list called below.
    eval "$(__gash_color_scope "$no_color")"

    local A="${__GASH_COLOR_ACCENT-\033[38;5;214m}"
    local W="${__GASH_BOLD_WHITE-\033[1;37m}"
    local C="${__GASH_CYAN-\033[0;36m}"
    local G="${__GASH_GREEN-\033[0;32m}"
    local M="${__GASH_COLOR_MUTED-\033[38;5;245m}"
    local Y="${__GASH_YELLOW-\033[0;33m}"
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
        __gash_help_search "$keyword"
        echo
        return $?
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
  gash                 Show summary overview
  gash SECTION         Show a specific section
  gash all             Show everything

SECTIONS
  git, files, system, docker, nav, llm, ai, php, gash

EXAMPLES
  # Quick reminder of all git aliases
  gash git

  # See what Docker functions are available
  gash docker

  # See every function and alias in Gash
  gash all

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
  gash_doctor

EXAMPLES
  # Run a full health check
  gash_doctor

NOTES
  Checks: core files, modules, config file permissions,
  external tools (pydf, most, htop, mtr, colordiff, expect).
  Reports missing or misconfigured components.
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
    --short "Display a random inspiring quote" \
    <<'HELP'
USAGE
  gash_inspiring_quote

EXAMPLES
  gash_inspiring_quote
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

fi  # end help registration guard
