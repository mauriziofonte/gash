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
# Help
# -----------------------------------------------------------------------------

# Display Gash help with all available commands.
# Usage: gash_help [TOPIC]
gash_help() {
    # Display the built-in Bash help
    builtin help "$@"

    # If no specific help topic is requested, show Gash-specific help
    if [[ -z "${1-}" ]]; then
        echo
        echo -e "\e[1;37m===\033[0m \033[0;36mG\033[0;33ma\033[38;5;214ms\033[0;32mh \033[1;37mGash, Another SHell!\033[0m - \e[1;37mCustom Commands ===\033[0m"
        echo -e "\e[0;36mFormat: LONG_NAME (SHORT_ALIAS)\033[0m"
        echo

        # File operations
        echo -e "\e[1;37m--- File Operations ---\033[0m"
        echo -e " > \e[0;33mfiles_largest\033[0m (\e[0;32mflf\033[0m) \e[0;36m[PATH]\033[0m - \e[1;37mLists the top 100 largest files.\033[0m"
        echo -e " > \e[0;33mdirs_largest\033[0m (\e[0;32mdld\033[0m) \e[0;36m[PATH]\033[0m - \e[1;37mLists the top 100 largest directories.\033[0m"
        echo -e " > \e[0;33mdirs_find_large\033[0m (\e[0;32mdfl\033[0m) \e[0;36m[--size SIZE] [DIR]\033[0m - \e[1;37mFinds directories larger than SIZE.\033[0m"
        echo -e " > \e[0;33mdirs_list_empty\033[0m (\e[0;32mdle\033[0m) \e[0;36m[PATH]\033[0m - \e[1;37mList all empty directories.\033[0m"
        echo -e " > \e[0;33marchive_extract\033[0m (\e[0;32maxe\033[0m) \e[0;36mFILE [DIR]\033[0m - \e[1;37mExtracts the archive file.\033[0m"
        echo -e " > \e[0;33mfile_backup\033[0m (\e[0;32mfbk\033[0m) \e[0;36mFILE\033[0m - \e[1;37mCreates a backup with timestamp.\033[0m"
        echo

        # System operations
        echo -e "\e[1;37m--- System Operations ---\033[0m"
        echo -e " > \e[0;33mdisk_usage\033[0m (\e[0;32mdu2\033[0m) - \e[1;37mDisplays disk usage for specific filesystem types.\033[0m"
        echo -e " > \e[0;33mhistory_grep\033[0m (\e[0;32mhg\033[0m) \e[0;36mPATTERN\033[0m - \e[1;37mSearches bash history for PATTERN.\033[0m"
        echo -e " > \e[0;33mip_public\033[0m (\e[0;32mmyip\033[0m) - \e[1;37mGet your public IP address.\033[0m"
        echo -e " > \e[0;33mprocess_find\033[0m (\e[0;32mpf\033[0m) \e[0;36mNAME\033[0m - \e[1;37mSearch for a process by name.\033[0m"
        echo -e " > \e[0;33mprocess_kill\033[0m (\e[0;32mpk\033[0m) \e[0;36mNAME\033[0m - \e[1;37mKill all processes by name.\033[0m"
        echo -e " > \e[0;33mport_kill\033[0m (\e[0;32mptk\033[0m) \e[0;36mPORT\033[0m - \e[1;37mKill all processes by port.\033[0m"
        echo -e " > \e[0;33mservices_stop\033[0m (\e[0;32msvs\033[0m) \e[0;36m[--force]\033[0m - \e[1;37mStop well-known services.\033[0m"
        echo -e " > \e[0;33msudo_last\033[0m (\e[0;32mplz\033[0m) - \e[1;37mRe-runs the previous command with sudo.\033[0m"
        echo -e " > \e[0;33mmkdir_cd\033[0m (\e[0;32mmkcd\033[0m) \e[0;36mDIRECTORY\033[0m - \e[1;37mCreates a directory and cd into it.\033[0m"
        echo

        # Git operations
        echo -e "\e[1;37m--- Git Functions ---\033[0m"
        echo -e " > \e[0;33mgit_list_tags\033[0m (\e[0;32mglt\033[0m) - \e[1;37mLists all local and remote tags.\033[0m"
        echo -e " > \e[0;33mgit_add_tag\033[0m (\e[0;32mgat\033[0m) \e[0;36m<tag> \"<msg>\"\033[0m - \e[1;37mCreates and pushes a tag.\033[0m"
        echo -e " > \e[0;33mgit_delete_tag\033[0m (\e[0;32mgdt\033[0m) \e[0;36m<tag>\033[0m - \e[1;37mDeletes a tag locally and on remote.\033[0m"
        echo -e " > \e[0;33mgit_dump_revisions\033[0m (\e[0;32mgdr\033[0m) \e[0;36mFILE\033[0m - \e[1;37mDump all revisions of a file.\033[0m"
        echo -e " > \e[0;33mgit_apply_patch\033[0m (\e[0;32mgap\033[0m) \e[0;36mMAIN FEAT COMMIT\033[0m - \e[1;37mApply a feature patch.\033[0m"
        echo

        # Docker operations
        echo -e "\e[1;37m--- Docker Functions ---\033[0m"
        echo -e " > \e[0;33mdocker_stop_all\033[0m (\e[0;32mdsa\033[0m) - \e[1;37mStop all Docker containers.\033[0m"
        echo -e " > \e[0;33mdocker_start_all\033[0m (\e[0;32mdaa\033[0m) - \e[1;37mStart all Docker containers.\033[0m"
        echo -e " > \e[0;33mdocker_prune_all\033[0m (\e[0;32mdpa\033[0m) - \e[1;37mRemove all Docker resources.\033[0m"
        echo

        # Gash management
        echo -e "\e[1;37m--- Gash Management ---\033[0m"
        echo -e " > \e[0;33mgash_help\033[0m - \e[1;37mDisplay this help.\033[0m"
        echo -e " > \e[0;33mgash_upgrade\033[0m - \e[1;37mUpgrade Gash to the latest version.\033[0m"
        echo -e " > \e[0;33mgash_uninstall\033[0m - \e[1;37mUninstall Gash.\033[0m"
        echo -e " > \e[0;33mgash_unload\033[0m - \e[1;37mRestore shell state before Gash.\033[0m"
        echo -e " > \e[0;33mgash_inspiring_quote\033[0m - \e[1;37mDisplay an inspiring quote.\033[0m"
        echo -e " > \e[0;33mgash_env_init\033[0m - \e[1;37mCreate ~/.gash_env from template.\033[0m"
        echo -e " > \e[0;33mgash_db_list\033[0m - \e[1;37mList configured database connections.\033[0m"
        echo -e " > \e[0;33mgash_db_test\033[0m \e[0;36mNAME\033[0m - \e[1;37mTest a database connection.\033[0m"
        echo -e " > \e[0;33mgash_ssh_auto_unlock\033[0m - \e[1;37mAuto-unlock SSH keys from ~/.gash_env.\033[0m"
        echo

        # Listing aliases
        echo -e "\e[1;37m--- Listing Aliases ---\033[0m"
        echo -e " > \e[0;33mll\033[0m - \e[1;37mLong listing (ls -l).\033[0m"
        echo -e " > \e[0;33mla\033[0m - \e[1;37mList all including hidden (ls -la).\033[0m"
        echo -e " > \e[0;33mlash\033[0m - \e[1;37mDetailed listing with sizes (ls -lash).\033[0m"
        echo -e " > \e[0;33ml\033[0m - \e[1;37mCompact listing (ls -CF).\033[0m"
        echo -e " > \e[0;33msl\033[0m - \e[1;37mTypo correction for ls.\033[0m"
        echo

        # Navigation aliases
        echo -e "\e[1;37m--- Navigation Aliases ---\033[0m"
        echo -e " > \e[0;33m..\033[0m, \e[0;33m...\033[0m, \e[0;33m....\033[0m, \e[0;33m.....\033[0m, \e[0;33m.4\033[0m, \e[0;33m.5\033[0m - \e[1;37mChange up 1-5 directories.\033[0m"
        echo -e " > \e[0;33mports\033[0m - \e[1;37mDisplay listening ports.\033[0m"
        echo -e " > \e[0;33mpath\033[0m - \e[1;37mShow PATH entries one per line.\033[0m"
        echo -e " > \e[0;33mcls\033[0m - \e[1;37mClear the screen.\033[0m"

        # WSL-specific aliases
        if grep -qi "microsoft" /proc/version 2>/dev/null && [ -n "${WSLENV-}" ]; then
            echo -e " > \e[0;33mwslrestart\033[0m, \e[0;33mwslshutdown\033[0m - \e[1;37mWSL restart/shutdown.\033[0m"
            echo -e " > \e[0;33mexplorer\033[0m - \e[1;37mOpen current directory in Windows Explorer.\033[0m"
        fi
        echo

        # Git aliases
        if command -v git >/dev/null 2>&1; then
            echo -e "\e[1;37m--- Git Log (run 'gl --help' for details) ---\033[0m"
            echo -e " > \e[0;33mgl\033[0m - \e[1;37mCompact log with graph (current branch).\033[0m"
            echo -e " > \e[0;33mgla\033[0m - \e[1;37mAll branches with graph.\033[0m"
            echo -e " > \e[0;33mglo\033[0m - \e[1;37mUltra-compact oneline format.\033[0m"
            echo -e " > \e[0;33mglg\033[0m - \e[1;37mGraph focused (first-parent only).\033[0m"
            echo -e " > \e[0;33mgls\033[0m - \e[1;37mLog with file statistics.\033[0m"
            echo -e " > \e[0;33mglf\033[0m \e[0;36mFILE\033[0m - \e[1;37mFile history with patches.\033[0m"
            echo
            echo -e "\e[1;37m--- Git Aliases ---\033[0m"
            echo -e " > \e[0;33mStatus:\033[0m gst (full), gs (short with branch)"
            echo -e " > \e[0;33mAdd:\033[0m ga, gaa (all), gap (interactive patch)"
            echo -e " > \e[0;33mCommit:\033[0m gc, gcm (with msg), gca (amend), gcan (amend no-edit)"
            echo -e " > \e[0;33mPush/Pull:\033[0m gp, gpf (force-with-lease), gpl, gplr (rebase)"
            echo -e " > \e[0;33mBranch:\033[0m gb, gba (all), gcb (create+switch), gbd/gbD (delete)"
            echo -e " > \e[0;33mCheckout:\033[0m gco, gsw (switch), gswc (switch -c)"
            echo -e " > \e[0;33mDiff:\033[0m gd, gds (staged), gdw (word-diff)"
            echo -e " > \e[0;33mStash:\033[0m gsh, gshp (pop), gshl (list), gsha (apply)"
            echo -e " > \e[0;33mRemote:\033[0m gf (fetch), gfa (fetch all+prune), gr (remote -v)"
            echo -e " > \e[0;33mReset:\033[0m grh, grh1 (undo last commit), grhh (hard)"
            echo -e " > \e[0;33mRebase:\033[0m grb, grbc (continue), grba (abort)"
            echo
        fi

        # Docker aliases
        if command -v docker >/dev/null 2>&1; then
            echo -e "\e[1;37m--- Docker Aliases ---\033[0m"
            echo -e " > \e[0;33mContainers:\033[0m dcls (list), dclsr (running), dstop, dstart, dexec, drm"
            echo -e " > \e[0;33mImages:\033[0m dils (list), drmi (remove)"
            echo -e " > \e[0;33mLogs/Info:\033[0m dlogs, dinspect, dnetls (networks)"
            echo
        fi

        # LLM Utilities (for AI agents)
        echo -e "\e[1;37m--- LLM Utilities (for AI agents) ---\033[0m"
        echo -e "\e[0;36mNo short aliases. Commands excluded from bash history.\033[0m"
        echo -e " > \e[0;33mllm_exec\033[0m \e[0;36mCMD\033[0m - \e[1;37mExecute command safely.\033[0m"
        echo -e " > \e[0;33mllm_tree\033[0m \e[0;36m[PATH]\033[0m - \e[1;37mDirectory tree (JSON).\033[0m"
        echo -e " > \e[0;33mllm_find\033[0m \e[0;36mPATTERN [PATH]\033[0m - \e[1;37mFind files by pattern.\033[0m"
        echo -e " > \e[0;33mllm_grep\033[0m \e[0;36mPATTERN [PATH]\033[0m - \e[1;37mSearch code (file:line:content).\033[0m"
        echo -e " > \e[0;33mllm_db_query\033[0m \e[0;36mSQL -c CONN\033[0m - \e[1;37mRead-only DB query (JSON).\033[0m"
        echo -e " > \e[0;33mllm_db_tables\033[0m \e[0;36m-c CONN\033[0m - \e[1;37mList database tables.\033[0m"
        echo -e " > \e[0;33mllm_db_schema\033[0m \e[0;36mTABLE -c CONN\033[0m - \e[1;37mShow table schema.\033[0m"
        echo -e " > \e[0;33mllm_db_sample\033[0m \e[0;36mTABLE -c CONN\033[0m - \e[1;37mSample rows from table.\033[0m"
        echo -e " > \e[0;33mllm_project\033[0m \e[0;36m[PATH]\033[0m - \e[1;37mDetect project type (JSON).\033[0m"
        echo -e " > \e[0;33mllm_deps\033[0m \e[0;36m[PATH]\033[0m - \e[1;37mList dependencies (JSON).\033[0m"
        echo -e " > \e[0;33mllm_config\033[0m \e[0;36m[PATH]\033[0m - \e[1;37mRead config files (JSON).\033[0m"
        echo -e " > \e[0;33mllm_git_status\033[0m - \e[1;37mCompact git status (JSON).\033[0m"
        echo -e " > \e[0;33mllm_git_log\033[0m \e[0;36m[--limit N]\033[0m - \e[1;37mRecent commits (JSON).\033[0m"
        echo -e " > \e[0;33mllm_git_diff\033[0m - \e[1;37mDiff with stats (JSON).\033[0m"
        echo -e " > \e[0;33mllm_ports\033[0m - \e[1;37mList ports in use (JSON).\033[0m"
        echo -e " > \e[0;33mllm_procs\033[0m \e[0;36m[--name N]\033[0m - \e[1;37mList processes (JSON).\033[0m"
        echo -e " > \e[0;33mllm_env\033[0m - \e[1;37mFiltered env vars (no secrets).\033[0m"
    fi
}
