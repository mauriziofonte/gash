#!/usr/bin/env bash

# Gash Module: Node.js / NVM Operations
# Helpers for managing Node.js versions installed via NVM.
#
# Dependencies: core/output.sh, core/validation.sh, core/utils.sh
#
# Public functions (LONG name + SHORT alias):
#   nvm_prune_versions (nvmp)  - Remove duplicate/older NVM patch versions
#   node_detect_version        - Detect the Node.js version a project wants

# -----------------------------------------------------------------------------
# Availability
# -----------------------------------------------------------------------------

# Verify NVM is loaded in the current shell. NVM is a shell function, not a
# PATH binary, so __gash_require_command does not apply here.
__gash_require_nvm() {
    if ! declare -f nvm >/dev/null 2>&1; then
        __gash_error "nvm is not loaded in this shell. Source \$NVM_DIR/nvm.sh first."
        return 1
    fi
    return 0
}

# -----------------------------------------------------------------------------
# Version Cleanup
# -----------------------------------------------------------------------------

# Remove duplicate NVM patch versions, keeping only the latest patch per major
# (the currently active version is never removed). Lists candidates by
# default; pass --force to actually uninstall them.
# Usage: nvm_prune_versions [--force]
# Alias: nvmp
nvm_prune_versions() {
    needs_help "nvm_prune_versions" "nvm_prune_versions [--force]" \
        "Remove duplicate/older NVM patch versions, keeping the latest patch per major. Alias: nvmp" \
        "${1-}" && return

    __gash_require_nvm || return 1

    local force=0 arg
    for arg in "$@"; do
        case "$arg" in
            --force | -f) force=1 ;;
            *)
                __gash_error "Unknown option: $arg (usage: nvm_prune_versions [--force])"
                return 1
                ;;
        esac
    done

    local current
    current="$(nvm current)"

    local -A latest=()
    local duplicates=() ver major

    # Oldest first: each newer patch of a major pushes the previous one out.
    # (grep -E, not -P: BSD and busybox grep have no -P.)
    while read -r ver; do
        [[ "$ver" =~ ^v([0-9]+)\.[0-9]+\.[0-9]+$ ]] || continue
        major="${BASH_REMATCH[1]}"
        if [[ -n "${latest[$major]-}" ]]; then
            duplicates+=("${latest[$major]}")
        fi
        latest[$major]="$ver"
    done < <(nvm ls --no-colors --no-alias | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | sort -u | sort -V)

    if [[ ${#duplicates[@]} -eq 0 ]]; then
        __gash_success "No duplicate NVM versions found."
        return 0
    fi

    for ver in "${duplicates[@]}"; do
        if [[ "$ver" == "$current" ]]; then
            __gash_info "Keeping $ver (currently active)."
            continue
        fi

        if [[ $force -eq 1 ]]; then
            __gash_info "Uninstalling $ver..."
            nvm uninstall "$ver"
        else
            __gash_warning "Dry run: would uninstall $ver. Re-run with --force to apply."
        fi
    done

    if [[ $force -eq 1 ]]; then
        __gash_success "Duplicate NVM versions removed."
    else
        __gash_info "Dry-run complete. Re-run with --force to actually uninstall."
    fi
}

# Installed versions as vX.Y.Z, oldest first (filesystem, no nvm call).
__node_installed_tags() {
    local dir="${NVM_DIR:-$HOME/.nvm}/versions/node" d
    for d in "$dir"/v*; do
        d="${d##*/}"
        [[ "$d" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] && printf '%s\n' "$d"
    done | sort -V
}

# Bring every installed Node.js major to its newest patch, the way apt
# upgrades packages: print the plan, ask once, then install the new patch
# with the global packages of the old one and remove the old patches.
# Majors that are not installed are never added, and a major is never
# removed as a whole.
# Usage: node_upgrade [-y] [--dry-run]
# Alias: nvmc
node_upgrade() {
    needs_help "node_upgrade" "node_upgrade [-y] [--dry-run]" \
        "Upgrade each installed Node.js major to its newest patch and remove the old patches. Alias: nvmc" \
        "${1-}" && return

    local assume_yes=0 dry_run=0 arg
    for arg in "$@"; do
        case "$arg" in
            -y | --yes | --force) assume_yes=1 ;;
            --dry-run) dry_run=1 ;;
            *)
                __gash_error "Unknown option: $arg (usage: node_upgrade [-y] [--dry-run])"
                return 1
                ;;
        esac
    done

    __gash_require_nvm || return 1

    local installed
    installed="$(__node_installed_tags)"
    if [[ -z "$installed" ]]; then
        __gash_info "No Node.js version is installed with nvm."
        return 0
    fi

    local remote
    __gash_task "Reading the remote index from nodejs.org" __node_read_remote_index remote || {
        __gash_error "Unable to read the remote index (network?). Nothing changed."
        return 1
    }

    local latest_lts
    latest_lts="$(grep 'Latest LTS' <<<"$remote" | tail -1 | sed -E 's/.*(v[0-9]+)\.[0-9]+\.[0-9]+.*Latest LTS: ([^)]+)\).*/\1 (\2)/')"

    local -a installs=() sources=() removes=() keeps=()
    local major target ver
    for major in $(sed -E 's/^v([0-9]+)\..*/\1/' <<<"$installed" | sort -un); do
        target="$(grep -oE "v${major}\.[0-9]+\.[0-9]+" <<<"$remote" | sort -V | tail -1)"
        if [[ -z "$target" ]]; then
            __gash_warning "v${major}: no release in the remote index, left as it is."
            continue
        fi
        if grep -qx "$target" <<<"$installed"; then
            keeps+=("$target")
        else
            installs+=("$target")
            sources+=("$(grep -E "^v${major}\." <<<"$installed" | tail -1)")
        fi
        for ver in $(grep -E "^v${major}\." <<<"$installed"); do
            [[ "$ver" != "$target" ]] && removes+=("$ver")
        done
    done

    [[ -n "$latest_lts" ]] && __gash_kv --width 0 "Latest LTS:" "$latest_lts"
    local i
    if ((${#installs[@]})); then
        __gash_heading "The following NEW versions will be installed:"
        for i in "${!installs[@]}"; do
            printf '%s\t(replaces %s, global packages carried over)\n' "${installs[$i]}" "${sources[$i]}"
        done | __gash_table --no-header --indent 2 --color 1:accent --color 2:muted
    fi
    if ((${#removes[@]})); then
        __gash_heading "The following versions will be REMOVED:"
        printf '%s\n' "${removes[*]}" | __gash_table --no-header --indent 2 --color 1:accent
    fi
    if ((${#keeps[@]})); then
        __gash_heading "Already the newest patch:"
        printf '%s\n' "${keeps[*]}" | __gash_table --no-header --indent 2 --color 1:accent
    fi
    printf '%d newly installed, %d to remove, %d up to date.\n' "${#installs[@]}" "${#removes[@]}" "${#keeps[@]}"

    ((${#installs[@]} + ${#removes[@]} == 0)) && return 0
    ((dry_run)) && return 0
    __gash_confirm_plan "$assume_yes" || return 1

    local current failed=" "
    current="$(nvm current 2>/dev/null)"

    for i in "${!installs[@]}"; do
        __gash_info "Installing ${installs[$i]}..."
        if ! nvm install "${installs[$i]}" --reinstall-packages-from="${sources[$i]}"; then
            major="${installs[$i]#v}"
            major="${major%%.*}"
            __gash_error "Installing ${installs[$i]} failed: the v$major patches are kept."
            failed+="$major "
        fi
    done

    # nvm refuses to remove the active version: move to the new patch of the
    # same major first.
    local current_major="${current#v}"
    current_major="${current_major%%.*}"
    if [[ " ${removes[*]} " == *" $current "* && "$failed" != *" $current_major "* ]]; then
        nvm use "$(grep -oE "v${current_major}\.[0-9]+\.[0-9]+" <<<"$remote" | sort -V | tail -1)" >/dev/null
    else
        nvm use "$current" >/dev/null 2>&1 || true
    fi

    local rc=0
    for ver in "${removes[@]}"; do
        major="${ver#v}"
        major="${major%%.*}"
        if [[ "$failed" == *" $major "* ]]; then
            __gash_warning "$ver kept (installing the new v$major failed)."
            continue
        fi
        __gash_info "Removing $ver..."
        nvm uninstall "$ver" >/dev/null || rc=1
    done

    __gash_success "Active version: $(nvm current)"
    return "$rc"
}

# Read `nvm ls-remote` into the variable named by $1; fails when the output
# holds no version (offline, rate limited).
__node_read_remote_index() {
    local -n __nrri_out="$1"
    __nrri_out="$(nvm ls-remote --no-colors 2>/dev/null)"
    [[ "$__nrri_out" =~ v[0-9]+\.[0-9]+\.[0-9]+ ]]
}

# -----------------------------------------------------------------------------
# Version Detection
# -----------------------------------------------------------------------------

# Parse a version string (v20, 20, 20.10.0, >=20, ^18.0.0, lts/iron, ...) to
# its major version number.
__node_parse_version_to_major() {
    local version_str="$1"

    if [[ "$version_str" =~ ^lts/ ]]; then
        # nvm records every LTS line it has seen as alias/lts/<codename>
        # (and lts/* for the newest), so new codenames need no table here.
        local lts_name="${version_str#lts/}" alias_file target
        alias_file="${NVM_DIR:-$HOME/.nvm}/alias/lts/${lts_name,,}"
        if [[ "$lts_name" != */* && -r "$alias_file" ]] && IFS= read -r target <"$alias_file" &&
            [[ "$target" =~ ^v?([0-9]+) ]]; then
            echo "${BASH_REMATCH[1]}"
            return 0
        fi
        case "${lts_name,,}" in
            krypton) echo "24" ;;
            jod) echo "22" ;;
            iron) echo "20" ;;
            hydrogen) echo "18" ;;
            gallium) echo "16" ;;
            fermium) echo "14" ;;
            *)
                local latest
                latest="$(__node_latest_installed)"
                echo "${latest%%.*}"
                ;;
        esac
        return 0
    fi

    local version
    version=$(echo "$version_str" | grep -oE '[0-9]+(\.[0-9]+)*' | head -1)
    [[ -n "$version" ]] && echo "${version%%.*}"
}

__node_installed_versions() {
    local nvm_dir="${NVM_DIR:-$HOME/.nvm}"
    if [[ -d "$nvm_dir/versions/node" ]]; then
        ls -1 "$nvm_dir/versions/node" 2>/dev/null | sed 's/^v//' | sort -V
    fi
}

__node_latest_for_major() {
    local major="$1"
    __node_installed_versions | grep "^${major}\." | tail -1
}

__node_latest_installed() {
    __node_installed_versions | tail -1
}

# Priority: .nvmrc -> .node-version -> package.json engines.node -> latest
# installed. Reads $NVM_DIR/versions/node directly (filesystem), independent
# of whether `nvm` is loaded as a shell function in this process.
__node_detect_version_major() {
    local project_dir="${1:-.}"

    if [[ -f "$project_dir/.nvmrc" ]]; then
        local nvmrc_content
        nvmrc_content=$(head -1 "$project_dir/.nvmrc" | tr -d '[:space:]')
        if [[ -n "$nvmrc_content" ]]; then
            local major
            major=$(__node_parse_version_to_major "$nvmrc_content")
            [[ -n "$major" ]] && {
                echo "$major"
                return 0
            }
        fi
    fi

    if [[ -f "$project_dir/.node-version" ]]; then
        local nodeversion_content
        nodeversion_content=$(head -1 "$project_dir/.node-version" | tr -d '[:space:]')
        if [[ -n "$nodeversion_content" ]]; then
            local major
            major=$(__node_parse_version_to_major "$nodeversion_content")
            [[ -n "$major" ]] && {
                echo "$major"
                return 0
            }
        fi
    fi

    if [[ -f "$project_dir/package.json" ]]; then
        local engines_node
        engines_node=$(jq -r '.engines.node // empty' "$project_dir/package.json" 2>/dev/null || echo "")
        if [[ -n "$engines_node" ]]; then
            local major
            major=$(__node_parse_version_to_major "$engines_node")
            [[ -n "$major" ]] && {
                echo "$major"
                return 0
            }
        fi
    fi

    local latest
    latest="$(__node_latest_installed)"
    if [[ -n "$latest" ]]; then
        echo "${latest%%.*}"
    else
        echo "20"
    fi
}

# Detect the Node.js version a project wants, and resolve the matching
# gash-exec function names. JSON output under --json or GASH_HEADLESS=1,
# human-readable text otherwise.
# Usage: node_detect_version [project_dir] [--json]
node_detect_version() {
    needs_help "node_detect_version" "node_detect_version [project_dir] [--json]" \
        "Detect which Node.js version a project wants (.nvmrc, .node-version, package.json engines.node) and resolve the matching gash-exec function names." \
        "${1-}" && return

    local nvm_dir="${NVM_DIR:-$HOME/.nvm}"
    if [[ ! -d "$nvm_dir" ]]; then
        __gash_error "NVM directory not found ($nvm_dir). Install NVM: curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash"
        return 1
    fi

    local project_dir="." llm_mode=0
    local arg
    for arg in "$@"; do
        case "$arg" in
            --json | --llm) llm_mode=1 ;; # --llm: 1.x spelling, kept as alias
            -*)
                __gash_error "Unknown option: $arg"
                return 1
                ;;
            *) project_dir="$arg" ;;
        esac
    done

    local major_version
    major_version="$(__node_detect_version_major "$project_dir")"

    local full_version
    full_version="$(__node_latest_for_major "$major_version" || true)"

    if [[ -z "$full_version" ]]; then
        local latest
        latest="$(__node_latest_installed)"
        if [[ -z "$latest" ]]; then
            __gash_error "No Node.js versions installed in NVM. Install: nvm install 20"
            return 1
        fi
        __gash_warning "Node $major_version not installed. Using $latest. Run: nvm install $major_version"
        full_version="$latest"
        major_version="${full_version%%.*}"
    fi

    local node_command="node${major_version}"
    local npm_command="npm${major_version}"
    local npx_command="npx${major_version}"

    if [[ $llm_mode -eq 1 ]] || __gash_headless_gate_active; then
        printf '{"version":"%s","full_version":"%s","node_command":"%s","npm_command":"%s","npx_command":"%s"}\n' \
            "$(__gash_json_escape "$major_version")" \
            "$(__gash_json_escape "$full_version")" \
            "$(__gash_json_escape "$node_command")" \
            "$(__gash_json_escape "$npm_command")" \
            "$(__gash_json_escape "$npx_command")"
    else
        __gash_info "Node.js version for '$project_dir': $full_version (major $major_version)"
        __gash_kv --indent 2 --width 15 --value-color accent \
            "node_command" "$node_command" \
            "npm_command" "$npm_command" \
            "npx_command" "$npx_command"
    fi
}

# -----------------------------------------------------------------------------
# Short Aliases
# -----------------------------------------------------------------------------
alias nvmp='nvm_prune_versions'
alias nvmc='node_upgrade'

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

    __gash_register_help "nvm_prune_versions" \
        --aliases "nvmp" \
        --module "node" \
        --short "Remove duplicate/older NVM patch versions" \
        <<'HELP'
USAGE
  nvm_prune_versions [--force]

EXAMPLES
  # Preview what would be removed (default, no changes made)
  nvmp

  # Actually uninstall the duplicate patch versions
  nvmp --force

NOTES
  For each installed major version, keeps only the latest patch.
  The currently active version (nvm current) is never uninstalled,
  even if it is not the latest patch for its major.
HELP

    __gash_register_help "node_detect_version" \
        --module "node" \
        --short "Detect the Node.js version a project wants" \
        <<'HELP'
USAGE
  node_detect_version [project_dir] [--json]

EXAMPLES
  # Human-readable, current directory
  node_detect_version

  # JSON, specific project
  node_detect_version /path/to/project --json

NOTES
  Priority: .nvmrc -> .node-version -> package.json engines.node -> latest
  installed. JSON output is also emitted automatically under
  GASH_HEADLESS=1, --json is not required there.
HELP

    __gash_register_help "node_upgrade" \
        --aliases "nvmc" \
        --module "node" \
        --short "Upgrade each installed Node.js major to its newest patch" \
        --see-also "nvm_prune_versions node_detect_version" \
        <<'HELP'
USAGE
  node_upgrade [-y] [--dry-run]

EXAMPLES
  # See the plan, change nothing
  nvmc --dry-run

  # Upgrade: plan, one question, then install and clean up
  nvmc

  # Unattended
  nvmc -y

NOTES
  For each major already installed (v18, v20, v22...) installs the newest
  published patch with the global packages of the patch it replaces
  (nvm install --reinstall-packages-from), then removes the older patches
  of that major. Majors that are not installed are never added and a
  major is never removed. When an install fails, that major keeps its
  patches. The active version moves to the new patch of its major.
  Needs nvm loaded (GASH_NVM=lazy or on does it) and network access to
  nodejs.org. Without a terminal the question is refused: pass -y.
HELP

fi # end help registration guard
