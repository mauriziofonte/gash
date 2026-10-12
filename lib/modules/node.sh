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

    local force=0
    for arg in "$@"; do
        case "$arg" in
            --force | -f) force=1 ;;
        esac
    done

    local current
    current="$(nvm current)"

    local -A latest=()
    local duplicates=()

    while read -r ver; do
        [[ "$ver" =~ ^v([0-9]+)\.[0-9]+\.[0-9]+$ ]] || continue
        local major="${BASH_REMATCH[1]}"
        if [[ -n "${latest[$major]-}" ]]; then
            duplicates+=("${latest[$major]}")
        fi
        latest[$major]="$ver"
    done < <(nvm ls --no-colors --no-alias | grep -oP 'v\d+\.\d+\.\d+' | sort -t. -k1,1n -k2,2n -k3,3n)

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
            __gash_warning "[DRY-RUN] Would uninstall $ver. Re-run with --force to apply."
        fi
    done

    if [[ $force -eq 1 ]]; then
        __gash_success "Duplicate NVM versions removed."
    else
        __gash_info "Dry-run complete. Re-run with --force to actually uninstall."
    fi
}

# -----------------------------------------------------------------------------
# Version Detection
# -----------------------------------------------------------------------------

# Parse a version string (v20, 20, 20.10.0, >=20, ^18.0.0, lts/iron, ...) to
# its major version number.
__node_parse_version_to_major() {
    local version_str="$1"

    if [[ "$version_str" =~ ^lts/ ]]; then
        local lts_name="${version_str#lts/}"
        case "$lts_name" in
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
# gash-exec function names. JSON output under --llm or GASH_HEADLESS=1,
# human-readable text otherwise.
# Usage: node_detect_version [project_dir] [--llm]
node_detect_version() {
    needs_help "node_detect_version" "node_detect_version [project_dir] [--llm]" \
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
            --llm) llm_mode=1 ;;
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
        printf "  %-15s %s\n" "node_command" "$node_command"
        printf "  %-15s %s\n" "npm_command" "$npm_command"
        printf "  %-15s %s\n" "npx_command" "$npx_command"
    fi
}

# -----------------------------------------------------------------------------
# Short Aliases
# -----------------------------------------------------------------------------
alias nvmp='nvm_prune_versions'

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
  node_detect_version [project_dir] [--llm]

EXAMPLES
  # Human-readable, current directory
  node_detect_version

  # JSON, specific project
  node_detect_version /path/to/project --llm

NOTES
  Priority: .nvmrc -> .node-version -> package.json engines.node -> latest
  installed. JSON output is also emitted automatically under
  GASH_HEADLESS=1, --llm is not required there.
HELP

fi # end help registration guard
