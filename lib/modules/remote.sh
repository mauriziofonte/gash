#!/usr/bin/env bash

# Gash Module: Remote Host Operations
# SSH command execution and scp/rsync file transfer against named remote
# hosts, with multiplexed connections and remote-path safety classification.
#
# Dependencies: core/output.sh, core/validation.sh, core/utils.sh, core/config.sh
#
# Public functions (LONG name + SHORT alias):
#   remote_hosts (rh)             - List configured remote host aliases
#   remote_exec (rex)             - Run a command/script on a remote host
#   remote_push (rpush)           - scp a local file/dir to a remote host
#   remote_pull (rpull)           - scp a remote file/dir to the local machine
#   remote_sync_push (rspush)     - rsync a local dir to a remote host
#   remote_sync_pull (rspull)     - rsync a remote dir to the local machine
#
# Host resolution
#   Named aliases live in ~/.gash_env as HOST:alias=user@host (see
#   __gash_get_host_target in core/config.sh). Interactively, any function
#   here also accepts a raw user@host target directly. Under GASH_HEADLESS=1
#   (an agent/script driving Gash, never a human at a keyboard) ONLY a
#   registered alias is accepted — this is the whitelist that keeps an
#   unsupervised caller confined to hosts the operator explicitly registered.
#
# Remote path safety (scp/rsync destinations/sources)
#   DENY  — always refused, human or headless, no override: irreversible or
#           system-breaking targets (auth databases, boot, kernel interfaces,
#           system binaries/libraries, PAM, SSH server config).
#   ASK   — risky but legitimate (most of /etc, /var, /usr/local, /root,
#           and SSH keys: ~/.ssh, /home/<user>/.ssh):
#           interactively a real y/n prompt (needs_confirm_prompt); headless,
#           refused unless --force is passed explicitly.
#   ALLOW — /tmp, /var/tmp, /home/<user>/*, relative paths, everything else.
# Paths are normalized first (//, /./, trailing /, ~/ and ~user/ prefixes),
# so //etc/shadow is /etc/shadow; any ".." segment is refused.

# -----------------------------------------------------------------------------
# Path safety tables
# -----------------------------------------------------------------------------
__REMOTE_DENY_PREFIXES=(
    "/etc/ssh" "/etc/sudoers" "/etc/sudoers.d"
    "/etc/passwd" "/etc/shadow" "/etc/shadow-" "/etc/gshadow" "/etc/gshadow-"
    "/etc/pam.d"
    "/boot" "/sys" "/proc" "/dev"
    "/bin" "/sbin" "/lib" "/lib64" "/lib32" "/libx32"
    "/usr/bin" "/usr/sbin" "/usr/lib" "/usr/lib64"
)
__REMOTE_ASK_PREFIXES=("/etc" "/var" "/usr/local" "/root")
__REMOTE_DELETE_SAFE_PREFIXES=("/tmp" "/var/tmp")

# -----------------------------------------------------------------------------
# Private helpers
# -----------------------------------------------------------------------------

# Resolve <alias|user@host> to an actual SSH target. See "Host resolution"
# in the module header for the headless-vs-interactive asymmetry.
# Usage: __remote_resolve_target <alias|user@host> OUT_VAR
__remote_resolve_target() {
    local input="${1-}"
    local -n __rrt_out="${2:-__gash_rrt_dummy}"

    local resolved
    __gash_load_env
    if resolved="$(__gash_get_host_target "$input")"; then
        __rrt_out="$resolved"
        return 0
    fi

    if __gash_headless_gate_active; then
        __gash_error "Unknown host alias '$input'. In headless mode only aliases registered in ~/.gash_env (HOST:alias=user@host) are accepted."
        return 1
    fi

    if [[ "$input" =~ ^[^@[:space:]]+@[^@[:space:]]+$ ]]; then
        __rrt_out="$input"
        return 0
    fi

    __gash_error "Unknown host alias '$input' and not a valid user@host target."
    return 1
}

# Canonical spelling of a remote path for the safety tables: repeated
# slashes collapsed, "." segments and trailing slashes dropped, "~/" and
# "~user/" home prefixes reduced to a relative path ("~" alone -> ".").
# Usage: __remote_normalize_path PATH   (prints the normalized path)
__remote_normalize_path() {
    local p="${1-}"
    if [[ "$p" == "~" || "$p" =~ ^~[^/]*$ ]]; then
        printf '.'
        return
    fi
    [[ "$p" =~ ^~[^/]*/ ]] && p="${p#"${BASH_REMATCH[0]}"}"
    while [[ "$p" == *//* ]]; do p="${p//\/\//\/}"; done
    while [[ "$p" == */./* ]]; do p="${p//\/.\//\/}"; done
    [[ "$p" == ./* ]] && p="${p#./}"
    [[ "$p" == */. ]] && p="${p%/.}"
    [[ "$p" == /. ]] && p="/"
    while [[ "$p" != "/" && "$p" == */ ]]; do p="${p%/}"; done
    printf '%s' "${p:-.}"
}

# Classify a remote path: prints "allow", "deny:<prefix>", or "ask:<prefix>".
__remote_classify_path() {
    local raw="$1"
    local path
    path="$(__remote_normalize_path "$raw")"

    if [[ "/$path/" == *"/../"* ]]; then
        printf 'deny:path_traversal'
        return
    fi
    # SSH keys and authorized_keys, in the remote home or any user's.
    if [[ "$path" == ".ssh" || "$path" == .ssh/* || "$path" =~ ^/home/[^/]+/\.ssh(/|$) ]]; then
        printf 'ask:~/.ssh'
        return
    fi
    if [[ "$path" != /* ]]; then
        printf 'allow'
        return
    fi
    if [[ "$path" == "/" ]]; then
        printf 'deny:/'
        return
    fi

    local prefix
    for prefix in "${__REMOTE_DENY_PREFIXES[@]}"; do
        if [[ "$path" == "$prefix" || "$path" == "$prefix"/* ]]; then
            printf 'deny:%s' "$prefix"
            return
        fi
    done

    for prefix in "${__REMOTE_DELETE_SAFE_PREFIXES[@]}"; do
        if [[ "$path" == "$prefix" || "$path" == "$prefix"/* ]]; then
            printf 'allow'
            return
        fi
    done

    for prefix in "${__REMOTE_ASK_PREFIXES[@]}"; do
        if [[ "$path" == "$prefix" || "$path" == "$prefix"/* ]]; then
            printf 'ask:%s' "$prefix"
            return
        fi
    done

    printf 'allow'
}

# Enforce path safety for a remote path. DENY is absolute (no --force, no
# interactive override). ASK is a real y/n prompt interactively, or a refusal
# under GASH_HEADLESS=1 unless force=1.
# Usage: __remote_check_safety <remote_path> <operation_label> <force 0|1>
__remote_check_safety() {
    local remote_path="$1" operation="$2" force="${3:-0}"
    local verdict kind detail
    verdict="$(__remote_classify_path "$remote_path")"
    kind="${verdict%%:*}"
    detail="${verdict#*:}"

    case "$kind" in
        allow) return 0 ;;
        deny)
            __gash_error "Remote path '$remote_path' is in the DENY list (matched: $detail). This is never allowed, not even with --force. Refusing $operation."
            return 1
            ;;
        ask)
            [[ "$force" -eq 1 ]] && return 0
            if __gash_headless_gate_active; then
                __gash_error "Remote path '$remote_path' is sensitive (matched: $detail). $operation refused in headless mode without --force."
                return 1
            fi
            if needs_confirm_prompt "Remote path '$remote_path' is sensitive (matched: $detail). Proceed with $operation?"; then
                return 0
            fi
            __gash_info "$operation cancelled."
            return 1
            ;;
    esac
}

# Returns 0 if a path is safe for rsync --delete without extra confirmation.
__remote_is_safe_for_delete() {
    local path
    path="$(__remote_normalize_path "$1")"

    [[ "/$path/" == *"/../"* ]] && return 1
    [[ "$path" == ".ssh" || "$path" == .ssh/* || "$path" =~ ^/home/[^/]+/\.ssh(/|$) ]] && return 1
    # The home directory itself: --delete there would mirror over all of it.
    [[ "$path" == "." ]] && return 1
    [[ "$path" != /* ]] && return 0

    local prefix
    for prefix in "${__REMOTE_DELETE_SAFE_PREFIXES[@]}"; do
        if [[ "$path" == "$prefix" || "$path" == "$prefix"/* ]]; then
            return 0
        fi
    done

    [[ "$path" =~ ^/home/[^/]+(/|$) ]] && return 0

    return 1
}

# Enforce delete safety for rsync --delete. Mirrors __remote_check_safety's
# headless/interactive asymmetry, narrower safe set.
__remote_check_delete_safety() {
    local remote_path="$1" force="${2:-0}"

    __remote_is_safe_for_delete "$remote_path" && return 0
    [[ "$force" -eq 1 ]] && return 0

    if __gash_headless_gate_active; then
        __gash_error "rsync --delete targeting '$remote_path' is outside the safe set (/tmp, /var/tmp, /home/<user>/). Refused in headless mode without --force."
        return 1
    fi
    if needs_confirm_prompt "rsync --delete targeting '$remote_path' can remove files on the remote. Proceed?"; then
        return 0
    fi
    __gash_info "Operation cancelled."
    return 1
}

__remote_require_scp() {
    command -v scp >/dev/null 2>&1 || {
        __gash_error "scp is not installed."
        return 1
    }
}

__remote_require_rsync() {
    command -v rsync >/dev/null 2>&1 || {
        __gash_error "rsync is not installed."
        return 1
    }
}

# -----------------------------------------------------------------------------
# Host registry
# -----------------------------------------------------------------------------

# List configured remote host aliases.
# Usage: remote_hosts
# Alias: rh
remote_hosts() {
    needs_help "remote_hosts" "remote_hosts" \
        "List configured remote host aliases from ~/.gash_env. Alias: rh" \
        "${1-}" && return

    local entries
    __gash_load_env
    entries="$(__gash_get_host_entries)"

    if [[ -z "$entries" ]]; then
        __gash_info "No remote hosts configured in ~/.gash_env"
        __gash_info "Add: HOST:alias=user@host"
        return 0
    fi

    __gash_info "Configured remote hosts:"
    echo

    local line
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        local alias_name="${line%%$'\t'*}"
        local target="${line#*$'\t'}"
        printf "  %-15s %s\n" "$alias_name" "$target"
    done <<<"$entries"
}

# -----------------------------------------------------------------------------
# Remote execution
# -----------------------------------------------------------------------------

# Execute a command (or a whole script) on a remote host via multiplexed SSH.
# Usage: remote_exec <alias|user@host> "command"
#        remote_exec <alias|user@host> --script FILE [-- arg1 arg2 ...]
#        remote_exec <alias|user@host> --script -   (script from stdin)
# Alias: rex
remote_exec() {
    needs_help "remote_exec" "remote_exec <alias|user@host> \"command\" | --script FILE [-- args...]" \
        "Execute a command or script on a remote host via multiplexed SSH. Alias: rex" \
        "${1-}" && return

    local target_input="${1-}"
    __gash_require_arg "$target_input" "host alias/target" "remote_exec <alias|user@host> \"command\"" || return 1
    shift

    local ssh_target
    __remote_resolve_target "$target_input" ssh_target || return 1

    local script_file=""
    if [[ "${1-}" == "--script" ]]; then
        script_file="${2-}"
        __gash_require_arg "$script_file" "script file" "remote_exec <alias> --script FILE|-" || return 1
        shift 2
    fi

    local script_mode=0 remote_cmd="" script_content="" script_args=()
    if [[ -n "$script_file" ]]; then
        script_mode=1
        if [[ "$script_file" == "-" ]]; then
            script_content="$(cat)"
        else
            __gash_require_file "$script_file" "Script file not readable: $script_file" || return 1
            script_content="$(cat "$script_file")"
        fi
        if [[ -z "${script_content//[[:space:]]/}" ]]; then
            __gash_error "Script is empty: $script_file"
            return 1
        fi
        [[ "${1-}" == "--" ]] && shift
        script_args=("$@")
    else
        remote_cmd="${1-}"
        __gash_require_arg "$remote_cmd" "remote command" "remote_exec <alias> \"command\"" || return 1
        if [[ $# -gt 1 ]]; then
            __gash_error "Pass the remote command as a SINGLE quoted string: remote_exec <alias> \"cmd arg1 arg2\". Refusing to silently drop extra args. For multi-line commands use --script."
            return 1
        fi
    fi

    mkdir -p "$HOME/.ssh" 2>/dev/null || true
    local ssh_opts=(
        -o ConnectTimeout=10
        -o BatchMode=yes
        -o ServerAliveInterval=15
        -o ServerAliveCountMax=4
        -o ControlMaster=auto
        -o ControlPath="$HOME/.ssh/cm-%C"
        -o ControlPersist=60s
    )

    local exit_code=0
    if [[ $script_mode -eq 1 ]]; then
        local remote_args=""
        if [[ ${#script_args[@]} -gt 0 ]]; then
            remote_args=" $(printf '%q ' "${script_args[@]}")"
        fi
        ssh "${ssh_opts[@]}" "$ssh_target" "unset HISTFILE HISTSIZE; bash -s --${remote_args}" <<<"$script_content" || exit_code=$?
    else
        ssh "${ssh_opts[@]}" "$ssh_target" "unset HISTFILE HISTSIZE; $remote_cmd" || exit_code=$?
    fi

    # Exit 255 is ambiguous (SSH transport failure vs. remote command killed by
    # a signal/exit 255). Disambiguate with a light reconnection probe reusing
    # the multiplexed master.
    if [[ $exit_code -eq 255 ]]; then
        if ssh -o ConnectTimeout=5 "${ssh_opts[@]}" "$ssh_target" 'exit 0' 2>/dev/null; then
            __gash_error "Remote command exited 255 (killed by a signal or explicit exit 255); the SSH connection is healthy."
            return 255
        fi
        __gash_error "Failed to connect to $ssh_target. Verify SSH connectivity and key configuration."
        return 1
    fi

    return $exit_code
}

# -----------------------------------------------------------------------------
# File transfer
# -----------------------------------------------------------------------------

# Copy a local file/dir to a remote host via scp.
# Usage: remote_push <alias|user@host> <local-src> <remote-dst> [-r] [-p] [--no-progress] [--force]
# Alias: rpush
remote_push() {
    needs_help "remote_push" "remote_push <alias|user@host> <local-src> <remote-dst> [-r] [-p] [--no-progress] [--force]" \
        "Copy a local file/dir to a remote host via scp. Alias: rpush" \
        "${1-}" && return

    local target_input="${1-}" src="${2-}" dst="${3-}"
    __gash_require_arg "$target_input" "host alias/target" "remote_push <alias> <src> <dst>" || return 1
    __gash_require_arg "$src" "local source" "remote_push <alias> <src> <dst>" || return 1
    __gash_require_arg "$dst" "remote destination" "remote_push <alias> <src> <dst>" || return 1
    shift 3

    local recursive=0 preserve=0 no_progress=0 force=0
    local arg
    for arg in "$@"; do
        case "$arg" in
            -r | --recursive) recursive=1 ;;
            -p | --preserve) preserve=1 ;;
            --no-progress) no_progress=1 ;;
            --force) force=1 ;;
        esac
    done

    local ssh_target
    __remote_resolve_target "$target_input" ssh_target || return 1
    __remote_check_safety "$dst" "push" "$force" || return 1
    __remote_require_scp || return 1

    local scp_args=(-o ConnectTimeout=10 -o BatchMode=yes)
    [[ $recursive -eq 1 ]] && scp_args+=(-r)
    [[ $preserve -eq 1 ]] && scp_args+=(-p)
    [[ $no_progress -eq 1 ]] && scp_args+=(-q)

    scp "${scp_args[@]}" -- "$src" "${ssh_target}:${dst}"
}

# Copy a remote file/dir to the local machine via scp.
# Usage: remote_pull <alias|user@host> <remote-src> <local-dst> [-r] [-p] [--no-progress] [--force]
# Alias: rpull
remote_pull() {
    needs_help "remote_pull" "remote_pull <alias|user@host> <remote-src> <local-dst> [-r] [-p] [--no-progress] [--force]" \
        "Copy a remote file/dir to the local machine via scp. Alias: rpull" \
        "${1-}" && return

    local target_input="${1-}" src="${2-}" dst="${3-}"
    __gash_require_arg "$target_input" "host alias/target" "remote_pull <alias> <src> <dst>" || return 1
    __gash_require_arg "$src" "remote source" "remote_pull <alias> <src> <dst>" || return 1
    __gash_require_arg "$dst" "local destination" "remote_pull <alias> <src> <dst>" || return 1
    shift 3

    local recursive=0 preserve=0 no_progress=0 force=0
    local arg
    for arg in "$@"; do
        case "$arg" in
            -r | --recursive) recursive=1 ;;
            -p | --preserve) preserve=1 ;;
            --no-progress) no_progress=1 ;;
            --force) force=1 ;;
        esac
    done

    local ssh_target
    __remote_resolve_target "$target_input" ssh_target || return 1
    __remote_check_safety "$src" "pull (source path access)" "$force" || return 1
    __remote_require_scp || return 1

    local scp_args=(-o ConnectTimeout=10 -o BatchMode=yes)
    [[ $recursive -eq 1 ]] && scp_args+=(-r)
    [[ $preserve -eq 1 ]] && scp_args+=(-p)
    [[ $no_progress -eq 1 ]] && scp_args+=(-q)

    scp "${scp_args[@]}" -- "${ssh_target}:${src}" "$dst"
}

# Sync a local directory to a remote host via rsync.
# Usage: remote_sync_push <alias|user@host> <local-dir> <remote-dir> [--delete] [--exclude=PAT] [--dry-run] [--no-progress] [--force]
# Alias: rspush
remote_sync_push() {
    needs_help "remote_sync_push" "remote_sync_push <alias|user@host> <local-dir> <remote-dir> [--delete] [--exclude=PAT] [--dry-run] [--no-progress] [--force]" \
        "Sync a local directory to a remote host via rsync. Alias: rspush" \
        "${1-}" && return

    local target_input="${1-}" src="${2-}" dst="${3-}"
    __gash_require_arg "$target_input" "host alias/target" "remote_sync_push <alias> <src> <dst>" || return 1
    __gash_require_arg "$src" "local source" "remote_sync_push <alias> <src> <dst>" || return 1
    __gash_require_arg "$dst" "remote destination" "remote_sync_push <alias> <src> <dst>" || return 1
    shift 3

    local delete=0 dry_run=0 no_progress=0 force=0
    local excludes=()
    local arg
    for arg in "$@"; do
        case "$arg" in
            --delete) delete=1 ;;
            --dry-run) dry_run=1 ;;
            --no-progress) no_progress=1 ;;
            --force) force=1 ;;
            --exclude=*) excludes+=("${arg#--exclude=}") ;;
        esac
    done

    local ssh_target
    __remote_resolve_target "$target_input" ssh_target || return 1
    __remote_check_safety "$dst" "sync-push" "$force" || return 1
    [[ $delete -eq 1 ]] && { __remote_check_delete_safety "$dst" "$force" || return 1; }
    __remote_require_rsync || return 1

    local rsync_rsh="ssh -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=4 -o ControlMaster=auto -o ControlPath=${HOME}/.ssh/cm-%C -o ControlPersist=60s"
    local rsync_args=(-az -e "$rsync_rsh")
    [[ $no_progress -eq 1 ]] || rsync_args+=(--info=progress2)
    [[ $delete -eq 1 ]] && rsync_args+=(--delete)
    [[ $dry_run -eq 1 ]] && rsync_args+=(--dry-run)
    local pat
    for pat in "${excludes[@]}"; do rsync_args+=(--exclude="$pat"); done

    rsync "${rsync_args[@]}" -- "$src" "${ssh_target}:${dst}"
}

# Sync a remote directory to the local machine via rsync.
# Usage: remote_sync_pull <alias|user@host> <remote-dir> <local-dir> [--delete] [--exclude=PAT] [--dry-run] [--no-progress] [--force]
# Alias: rspull
remote_sync_pull() {
    needs_help "remote_sync_pull" "remote_sync_pull <alias|user@host> <remote-dir> <local-dir> [--delete] [--exclude=PAT] [--dry-run] [--no-progress] [--force]" \
        "Sync a remote directory to the local machine via rsync. Alias: rspull" \
        "${1-}" && return

    local target_input="${1-}" src="${2-}" dst="${3-}"
    __gash_require_arg "$target_input" "host alias/target" "remote_sync_pull <alias> <src> <dst>" || return 1
    __gash_require_arg "$src" "remote source" "remote_sync_pull <alias> <src> <dst>" || return 1
    __gash_require_arg "$dst" "local destination" "remote_sync_pull <alias> <src> <dst>" || return 1
    shift 3

    local delete=0 dry_run=0 no_progress=0 force=0
    local excludes=()
    local arg
    for arg in "$@"; do
        case "$arg" in
            --delete) delete=1 ;;
            --dry-run) dry_run=1 ;;
            --no-progress) no_progress=1 ;;
            --force) force=1 ;;
            --exclude=*) excludes+=("${arg#--exclude=}") ;;
        esac
    done

    local ssh_target
    __remote_resolve_target "$target_input" ssh_target || return 1
    __remote_check_safety "$src" "sync-pull (source path access)" "$force" || return 1
    # --delete here removes files under the LOCAL destination. Local paths are
    # not otherwise checked; this is a light defense (relative/tmp/home pass
    # without --force), matching the historical harness contract.
    [[ $delete -eq 1 ]] && { __remote_check_delete_safety "$dst" "$force" || return 1; }
    __remote_require_rsync || return 1

    local rsync_rsh="ssh -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=4 -o ControlMaster=auto -o ControlPath=${HOME}/.ssh/cm-%C -o ControlPersist=60s"
    local rsync_args=(-az -e "$rsync_rsh")
    [[ $no_progress -eq 1 ]] || rsync_args+=(--info=progress2)
    [[ $delete -eq 1 ]] && rsync_args+=(--delete)
    [[ $dry_run -eq 1 ]] && rsync_args+=(--dry-run)
    local pat
    for pat in "${excludes[@]}"; do rsync_args+=(--exclude="$pat"); done

    rsync "${rsync_args[@]}" -- "${ssh_target}:${src}" "$dst"
}

# -----------------------------------------------------------------------------
# Short Aliases
# -----------------------------------------------------------------------------
alias rh='remote_hosts'
alias rex='remote_exec'
alias rpush='remote_push'
alias rpull='remote_pull'
alias rspush='remote_sync_push'
alias rspull='remote_sync_pull'

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

    __gash_register_help "remote_hosts" \
        --aliases "rh" \
        --module "remote" \
        --short "List configured remote host aliases" \
        --see-also "remote_exec remote_push remote_pull" \
        <<'HELP'
USAGE
  remote_hosts

EXAMPLES
  # See which aliases are registered
  rh

NOTES
  Reads HOST:alias=user@host entries from ~/.gash_env.
HELP

    __gash_register_help "remote_exec" \
        --aliases "rex" \
        --module "remote" \
        --short "Run a command or script on a remote host via multiplexed SSH" \
        --see-also "remote_hosts remote_push remote_pull" \
        <<'HELP'
USAGE
  remote_exec <alias|user@host> "command"
  remote_exec <alias|user@host> --script FILE [-- arg1 arg2 ...]
  remote_exec <alias|user@host> --script -

EXAMPLES
  # Run a single command
  rex hnas "ls -la /root"

  # Run a local script on the remote (no quoting hell)
  rex myserver --script /tmp/provision.sh -- --verbose

NOTES
  Connections are multiplexed (ControlMaster, 60s persist): repeated calls to
  the same host reuse one session. Under GASH_HEADLESS=1 only a registered
  alias (~/.gash_env HOST:) is accepted, never a raw user@host target.
HELP

    __gash_register_help "remote_push" \
        --aliases "rpush" \
        --module "remote" \
        --short "Copy a local file/dir to a remote host via scp" \
        --see-also "remote_pull remote_sync_push" \
        <<'HELP'
USAGE
  remote_push <alias|user@host> <local-src> <remote-dst> [-r] [-p] [--no-progress] [--force]

EXAMPLES
  # Push a config file
  rpush hnas ./nginx.conf /tmp/nginx.conf

  # Push a directory
  rpush hnas ./dist /tmp/dist -r
HELP

    __gash_register_help "remote_pull" \
        --aliases "rpull" \
        --module "remote" \
        --short "Copy a remote file/dir to the local machine via scp" \
        --see-also "remote_push remote_sync_pull" \
        <<'HELP'
USAGE
  remote_pull <alias|user@host> <remote-src> <local-dst> [-r] [-p] [--no-progress] [--force]

EXAMPLES
  # Pull a single file
  rpull viper /tmp/backup.tar.gz ./backup.tar.gz
HELP

    __gash_register_help "remote_sync_push" \
        --aliases "rspush" \
        --module "remote" \
        --short "Sync a local directory to a remote host via rsync" \
        --see-also "remote_sync_pull remote_push" \
        <<'HELP'
USAGE
  remote_sync_push <alias|user@host> <local-dir> <remote-dir> [--delete] [--exclude=PAT] [--dry-run] [--no-progress] [--force]

EXAMPLES
  # Sync a build directory
  rspush hnas ./build/ /tmp/build/ --delete --exclude='*.log'
HELP

    __gash_register_help "remote_sync_pull" \
        --aliases "rspull" \
        --module "remote" \
        --short "Sync a remote directory to the local machine via rsync" \
        --see-also "remote_sync_push remote_pull" \
        <<'HELP'
USAGE
  remote_sync_pull <alias|user@host> <remote-dir> <local-dir> [--delete] [--exclude=PAT] [--dry-run] [--no-progress] [--force]

EXAMPLES
  # Sync a remote project directory locally
  rspull viper /home/maurizio/projects/foo/ ./foo/
HELP

fi # end help registration guard
