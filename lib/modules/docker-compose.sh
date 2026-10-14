#!/usr/bin/env bash

# Gash Module: Docker Compose Operations
# Smart upgrade system for Docker Compose services with registry version checking.
#
# Dependencies: core/output.sh, core/validation.sh, docker.sh
#
# Public functions (LONG name + SHORT alias):
#   docker_compose_check (dcc)     - Check for available updates
#   docker_compose_upgrade (dcup2) - Upgrade services (only latest/untagged)
#   docker_compose_scan (dcscan)   - Scan directories for compose files
#
# LLM functions (JSON output, no aliases):
#   llm_docker_check               - JSON output for LLM agents

# =============================================================================
# INTERNAL HELPERS
# =============================================================================

# Check if docker compose (v2) or docker-compose (v1) is available.
# Outputs the command to use on stdout.
# Prefers v2 (docker compose) over v1 (docker-compose).
# Usage: __gash_compose_cmd
__gash_compose_cmd() {
    # First check if docker is available
    if ! type -P docker &>/dev/null; then
        return 1
    fi

    # Try docker compose v2 first (integrated into docker CLI)
    if docker compose version &>/dev/null 2>&1; then
        echo "docker compose"
        return 0
    fi

    # Fall back to docker-compose v1 (standalone binary)
    if type -P docker-compose &>/dev/null; then
        echo "docker-compose"
        return 0
    fi

    return 1
}

# Require docker compose to be available.
# Usage: __gash_require_compose
__gash_require_compose() {
    if ! __gash_compose_cmd &>/dev/null; then
        __gash_error "Docker Compose not found. Install docker-compose or Docker Desktop."
        return 1
    fi
    return 0
}

# Extract service names and images from a compose file.
# Output: service_name|image_reference (one per line)
# Preferred source: `docker compose config --format json`, which applies
# the compose spec itself (any indentation, anchors, extends, include,
# .env interpolation). Without Compose v2 or jq, or when config fails, a
# YAML-subset reader takes over: block-style services at any indentation.
# Usage: __gash_parse_compose_services <compose_file>
__gash_parse_compose_services() {
    local file="$1"
    [[ ! -f "$file" ]] && return 1

    local compose_cmd json out
    if type -P jq >/dev/null 2>&1 && compose_cmd=$(__gash_compose_cmd 2>/dev/null) && [[ "$compose_cmd" == "docker compose" ]]; then
        if json=$(docker compose -f "$file" --project-directory "$(dirname -- "$file")" config --format json 2>/dev/null) &&
            out=$(jq -r '.services // {} | to_entries[] | select(.value.image != null) | "\(.key)|\(.value.image)"' <<<"$json" 2>/dev/null); then
            [[ -n "$out" ]] && printf '%s\n' "$out"
            return 0
        fi
    fi

    "${__GASH_AWK:-awk}" '
    function indent_of(s) { match(s, /^ */); return RLENGTH }
    function unquote(s) {
        sub(/^[ \t]+/, "", s); sub(/[ \t]+#.*$/, "", s); sub(/[ \t\r]+$/, "", s)
        if (s ~ /^".*"$/ || s ~ /^\047.*\047$/) s = substr(s, 2, length(s) - 2)
        return s
    }
    /^services:[ \t]*(#.*)?\r?$/ { in_services = 1; svc_ind = -1; next }
    in_services && /^[^ \t#\r]/ { in_services = 0 }
    !in_services { next }
    /^[ \t]*(#.*)?\r?$/ { next }
    {
        ind = indent_of($0)
        if (svc_ind < 0) svc_ind = ind
        if (ind == svc_ind) {
            name = $0
            sub(/^ +/, "", name); sub(/:.*$/, "", name)
            current = unquote(name)
            child_ind = -1
            next
        }
        if (current == "" || ind < svc_ind) next
        if (child_ind < 0) child_ind = ind
        if (ind == child_ind && $0 ~ /^ *image:/) {
            img = $0
            sub(/^ *image:/, "", img)
            img = unquote(img)
            if (img != "") print current "|" img
        }
    }
    ' "$file"
}

# Resolve environment variables in a string using .env file.
# Handles ${VAR}, ${VAR:-default}, ${VAR-default} patterns.
# Usage: __gash_resolve_env_vars <string> [env_file]
__gash_resolve_env_vars() {
    local value="$1"
    local env_file="${2:-.env}"
    local result="$value"

    # Load .env file into associative array if it exists
    declare -A env_vars
    if [[ -f "$env_file" ]]; then
        while IFS= read -r line || [[ -n "$line" ]]; do
            # Skip comments and empty lines
            [[ -z "$line" || "$line" == \#* ]] && continue
            # Parse KEY=VALUE
            if [[ "$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
                local key="${BASH_REMATCH[1]}"
                local val="${BASH_REMATCH[2]}"
                # Remove surrounding quotes
                val="${val#\"}" && val="${val%\"}"
                val="${val#\'}" && val="${val%\'}"
                env_vars["$key"]="$val"
            fi
        done <"$env_file"
    fi

    # Replace ${VAR:-default} and ${VAR} patterns.
    # Iteration cap: a self-referential value (VAR=${VAR}) or a circular
    # chain would otherwise loop forever.
    local iterations=0
    local max_iterations=50
    while [[ "$result" =~ \$\{([A-Za-z_][A-Za-z0-9_]*)((:?-)([^}]*))?\} ]]; do
        if ((iterations >= max_iterations)); then
            __gash_warning "Unresolvable variable reference (circular?) in: $value"
            break
        fi
        iterations=$((iterations + 1))

        local var_name="${BASH_REMATCH[1]}"
        local default_val="${BASH_REMATCH[4]:-}"
        local full_match="${BASH_REMATCH[0]}"

        # Try: env_vars from .env, then shell env, then default
        local resolved="${env_vars[$var_name]:-${!var_name:-$default_val}}"
        result="${result//$full_match/$resolved}"
    done

    echo "$result"
}

# Normalize image reference to registry|name|tag format.
# Usage: __gash_normalize_image <image_ref>
# Output: registry|name|tag
__gash_normalize_image() {
    local image="$1"
    local registry="docker.io"
    local name=""
    local tag="latest"

    # Handle digest-pinned images (image@sha256:...)
    if [[ "$image" == *@sha256:* ]]; then
        local digest="${image##*@}"
        image="${image%@*}"
        tag="@$digest"
    fi

    # Extract tag if present: only a colon AFTER the last slash is a tag
    # separator. A registry port (localhost:5000/img, reg.example.com:443/ns/img)
    # must never be mistaken for a tag.
    local last_seg="${image##*/}"
    if [[ "$last_seg" == *:* && ! "$image" == *@* ]]; then
        tag="${image##*:}"
        image="${image%:*}"
    fi

    # Determine registry and name
    if [[ "$image" == */* ]]; then
        local first_part="${image%%/*}"
        # Check if first part looks like a registry (has dot or colon)
        if [[ "$first_part" == *.* || "$first_part" == *:* || "$first_part" == "localhost" ]]; then
            registry="$first_part"
            name="${image#*/}"
        else
            # Docker Hub with namespace (e.g., ollama/ollama)
            name="$image"
        fi
    else
        # Official Docker Hub image (e.g., nginx)
        name="library/$image"
    fi

    echo "$registry|$name|$tag"
}

# Check if an image tag is upgradeable (latest or no specific version).
# Returns 0 if upgradeable, 1 if pinned.
# Usage: __gash_is_upgradeable <tag>
__gash_is_upgradeable() {
    local tag="$1"

    # Digest-pinned: NOT upgradeable
    [[ "$tag" == @sha256:* ]] && return 1

    # Explicit "latest" or common mutable tags: upgradeable
    case "$tag" in
        latest | main | master | dev | develop | edge | nightly | canary)
            return 0
            ;;
    esac

    # Semantic version with only major (e.g., "8", "15"): upgradeable within major
    [[ "$tag" =~ ^[0-9]+$ ]] && return 0

    # Semantic version with major.minor (e.g., "1.25", "8.1"): upgradeable within minor
    [[ "$tag" =~ ^[0-9]+\.[0-9]+$ ]] && return 0

    # Specific version (e.g., "1.25.3", "v2.0.1"): NOT upgradeable
    [[ "$tag" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+ ]] && return 1

    # Tags with suffixes like -alpine, -slim: check base
    local base_tag="${tag%%-*}"
    if [[ "$base_tag" != "$tag" ]]; then
        __gash_is_upgradeable "$base_tag"
        return $?
    fi

    # Default: assume upgradeable for unknown patterns
    return 0
}

# =============================================================================
# REGISTRY API FUNCTIONS
# =============================================================================

# Centralized registry HTTP call with error classification.
# Captures response body/headers and HTTP status code.
# Sets: __gash_registry_curl_body (response), __gash_registry_curl_http_code,
#       __gash_registry_curl_error (error type on failure)
# Returns: 0 on success (2xx), 1 on error
# Usage: __gash_registry_curl [CURL_OPTIONS...] URL
__gash_registry_curl() {
    local tmpfile
    tmpfile=$(mktemp) || return 1
    # shellcheck disable=SC2064
    trap "rm -f '${tmpfile}'; trap - RETURN" RETURN

    # Reset state
    __gash_registry_curl_body=""
    __gash_registry_curl_http_code=""
    __gash_registry_curl_error=""

    local http_code=""
    local _curl_rc=0
    http_code=$(curl -sSL --connect-timeout 10 --max-time 20 \
        -w '%{http_code}' -o "$tmpfile" "$@" 2>/dev/null) || _curl_rc=$?

    if [[ $_curl_rc -ne 0 ]]; then
        case $_curl_rc in
            6) __gash_registry_curl_error="dns_error" ;;
            7) __gash_registry_curl_error="connection_refused" ;;
            28) __gash_registry_curl_error="timeout" ;;
            35 | 60) __gash_registry_curl_error="ssl_error" ;;
            *) __gash_registry_curl_error="curl_error_${_curl_rc}" ;;
        esac
        return 1
    fi

    if [[ -z "$http_code" ]]; then
        __gash_registry_curl_error="no_response"
        return 1
    fi

    __gash_registry_curl_http_code="$http_code"
    __gash_registry_curl_body=$(<"$tmpfile")

    case "$http_code" in
        2??) return 0 ;;
        401) __gash_registry_curl_error="unauthorized" ;;
        403) __gash_registry_curl_error="forbidden" ;;
        404) __gash_registry_curl_error="not_found" ;;
        429) __gash_registry_curl_error="rate_limited" ;;
        5??) __gash_registry_curl_error="server_error" ;;
        *) __gash_registry_curl_error="http_${http_code}" ;;
    esac

    return 1
}

# Get authentication token for Docker Hub.
# Sets: __gash_auth_token (token string on success)
# Usage: __gash_dockerhub_token <image_name>
__gash_dockerhub_token() {
    local image="$1"
    __gash_auth_token=""

    __gash_registry_curl \
        "https://auth.docker.io/token?service=registry.docker.io&scope=repository:${image}:pull" ||
        return 1

    local token=""
    if command -v jq >/dev/null 2>&1; then
        # Robust extraction: handles "token" or "access_token", any spacing
        token=$(jq -r '.token // .access_token // empty' <<<"$__gash_registry_curl_body" 2>/dev/null) || token=""
    else
        # Fallback: parameter expansion (assumes compact "token":"..." form)
        token="${__gash_registry_curl_body}"
        token="${token##*\"token\":\"}"
        token="${token%%\"*}"
        [[ "$token" == "$__gash_registry_curl_body" || "$token" == "{" ]] && token=""
    fi

    if [[ -n "$token" ]]; then
        __gash_auth_token="$token"
        echo "$token"
        return 0
    fi

    __gash_registry_curl_error="malformed_token"
    return 1
}

# Get authentication token for GHCR (GitHub Container Registry).
# GHCR requires a token even for public images.
# Sets: __gash_auth_token (token string on success)
# Usage: __gash_ghcr_token <image_name>
__gash_ghcr_token() {
    local image="$1"
    __gash_auth_token=""

    # If user has GITHUB_TOKEN, use it; otherwise get anonymous token
    if [[ -n "${GITHUB_TOKEN:-}" ]]; then
        __gash_auth_token="$GITHUB_TOKEN"
        echo "$GITHUB_TOKEN"
        return 0
    fi

    __gash_registry_curl \
        "https://ghcr.io/token?service=ghcr.io&scope=repository:${image}:pull" ||
        return 1

    local token=""
    if command -v jq >/dev/null 2>&1; then
        token=$(jq -r '.token // .access_token // empty' <<<"$__gash_registry_curl_body" 2>/dev/null) || token=""
    else
        token="${__gash_registry_curl_body}"
        token="${token##*\"token\":\"}"
        token="${token%%\"*}"
        [[ "$token" == "$__gash_registry_curl_body" || "$token" == "{" ]] && token=""
    fi

    if [[ -n "$token" ]]; then
        __gash_auth_token="$token"
        echo "$token"
        return 0
    fi

    __gash_registry_curl_error="malformed_token"
    return 1
}

# Manifest media types accepted from registries. The index/list types come
# first: a multi-arch image pulled by tag records the digest of its index
# in RepoDigests, so asking only for a single-platform manifest yields a
# different digest and a permanent false "update available".
__GASH_MANIFEST_ACCEPT=(
    -H "Accept: application/vnd.oci.image.index.v1+json"
    -H "Accept: application/vnd.docker.distribution.manifest.list.v2+json"
    -H "Accept: application/vnd.oci.image.manifest.v1+json"
    -H "Accept: application/vnd.docker.distribution.manifest.v2+json"
)

# Get remote image digest from registry.
# Usage: __gash_get_remote_digest <registry> <name> <tag>
# Output: sha256:... digest on success (stdout)
# Returns: 0 on success, 1 on error (check __gash_registry_curl_error)
__gash_get_remote_digest() {
    local registry="$1"
    local name="$2"
    local tag="$3"
    __gash_remote_digest=""

    # Skip digest-pinned images
    [[ "$tag" == @sha256:* ]] && {
        __gash_remote_digest="${tag#@}"
        echo "${tag#@}"
        return 0
    }

    case "$registry" in
        docker.io)
            __gash_dockerhub_token "$name" >/dev/null 2>&1 || return 1
            [[ -z "$__gash_auth_token" ]] && return 1

            # Bearer token via --config fd, never in argv (/proc/<pid>/cmdline)
            __gash_registry_curl -I \
                --config <(printf 'header = "Authorization: Bearer %s"\n' "$__gash_auth_token") \
                "${__GASH_MANIFEST_ACCEPT[@]}" \
                "https://registry-1.docker.io/v2/${name}/manifests/${tag}" ||
                return 1
            ;;

        ghcr.io)
            __gash_ghcr_token "$name" >/dev/null 2>&1 || return 1
            [[ -z "$__gash_auth_token" ]] && return 1

            # Bearer token via --config fd, never in argv (/proc/<pid>/cmdline)
            __gash_registry_curl -I \
                --config <(printf 'header = "Authorization: Bearer %s"\n' "$__gash_auth_token") \
                "${__GASH_MANIFEST_ACCEPT[@]}" \
                "https://ghcr.io/v2/${name}/manifests/${tag}" ||
                return 1
            ;;

        *)
            # Generic registry - try without auth
            __gash_registry_curl -I \
                "${__GASH_MANIFEST_ACCEPT[@]}" \
                "https://${registry}/v2/${name}/manifests/${tag}" ||
                return 1
            ;;
    esac

    # Extract digest from Docker-Content-Digest header in response
    local digest
    digest=$(echo "$__gash_registry_curl_body" | grep -i "docker-content-digest:" | awk '{print $2}' | tr -d '\r\n')

    if [[ -n "$digest" ]]; then
        __gash_remote_digest="$digest"
        echo "$digest"
        return 0
    fi

    __gash_registry_curl_error="no_digest_in_response"
    return 1
}

# Get local image digest.
# Usage: __gash_get_local_digest <image_full_ref>
# Output: sha256:... or empty if not found
__gash_get_local_digest() {
    local image="$1"

    # RepoDigests: prefer the entry of this repository (an image tagged
    # under several names carries one per repository), else the first.
    local repo="${image%@*}" digests digest=""
    [[ "${repo##*/}" == *:* ]] && repo="${repo%:*}"
    digests=$(docker inspect --format '{{range .RepoDigests}}{{.}}{{"\n"}}{{end}}' "$image" 2>/dev/null) || digests=""
    local line
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        if [[ "${line%@*}" == "$repo" || "${line%@*}" == "docker.io/$repo" || "${line%@*}" == "docker.io/library/$repo" ]]; then
            digest="${line#*@}"
            break
        fi
        [[ -z "$digest" ]] && digest="${line#*@}"
    done <<<"$digests"
    [[ "$digest" =~ ^sha256:[a-f0-9]{64}$ ]] || digest=""

    [[ -n "$digest" ]] && echo "$digest"
}

# =============================================================================
# PUBLIC FUNCTIONS
# =============================================================================

# Check for available updates in docker-compose services.
# Usage: docker_compose_check [PATH] [--json]
# Options:
#   --json    Output in JSON format (for scripting/LLM)
#   -h        Show help
docker_compose_check() {
    # Help
    if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
        cat <<'EOF'
docker_compose_check - Check for Docker Compose image updates

USAGE:
  docker_compose_check [PATH] [--json] [--no-color]

OPTIONS:
  PATH        Directory containing docker-compose.yml (default: current dir)
  --json      Output in JSON format for scripting
  --no-color  Disable ANSI colors (auto-off on pipe / NO_COLOR / GASH_HEADLESS)
  -h          Show this help

OUTPUT:
  Shows each service with current local digest, remote digest, and update status.
  Services with pinned versions show "PINNED" and won't be upgraded.

EXAMPLES:
  docker_compose_check                    Check current directory
  docker_compose_check /path/to/project   Check specific project
  docker_compose_check --json             JSON output for scripting
EOF
        return 0
    fi

    __gash_require_docker || return 1
    __gash_require_compose || return 1

    local path="."
    local json_output=0
    local no_color=0

    # Parse arguments
    for arg in "$@"; do
        case "$arg" in
            --json | -j) json_output=1 ;;
            --no-color) no_color=1 ;;
            -*)
                __gash_error "Unknown option: $arg"
                return 1
                ;;
            *) path="$arg" ;;
        esac
    done

    # Local color scope (ignored when --json since JSON path doesn't emit ANSI)
    eval "$(__gash_color_scope "$no_color")"

    # Resolve path
    path="$(realpath -m "$path" 2>/dev/null)" || path="$PWD"

    # Find compose file
    local compose_file=""
    for f in docker-compose.yml docker-compose.yaml compose.yml compose.yaml; do
        [[ -f "$path/$f" ]] && {
            compose_file="$path/$f"
            break
        }
    done

    if [[ -z "$compose_file" ]]; then
        if [[ $json_output -eq 1 ]]; then
            echo '{"error":"compose_not_found","path":"'"$(__gash_json_escape "$path")"'"}'
        else
            __gash_error "No docker-compose.yml found in $path"
        fi
        return 1
    fi

    # Parse services
    local services
    services=$(__gash_parse_compose_services "$compose_file")

    if [[ -z "$services" ]]; then
        if [[ $json_output -eq 1 ]]; then
            echo '{"error":"no_services","path":"'"$(__gash_json_escape "$path")"'"}'
        else
            __gash_error "No services with images found in $compose_file"
        fi
        return 1
    fi

    # Resolve .env file path
    local env_file="$path/.env"

    # Collect results
    local results=()
    local total=0 updates=0 current=0 pinned=0 errors=0

    while IFS='|' read -r service_name image_ref; do
        total=$((total + 1))

        # Resolve env vars in image reference
        local resolved_image
        resolved_image=$(__gash_resolve_env_vars "$image_ref" "$env_file")

        # Normalize image
        local normalized
        normalized=$(__gash_normalize_image "$resolved_image")
        IFS='|' read -r registry name tag <<<"$normalized"

        # Check if upgradeable
        local upgradeable=0
        __gash_is_upgradeable "$tag" && upgradeable=1

        # Get local digest
        local local_digest=""
        local remote_digest=""
        local status="UNKNOWN"
        local error_msg=""

        # For digest-pinned images, extract digest directly (no registry call needed)
        if [[ "$tag" == @sha256:* ]]; then
            status="PINNED"
            local_digest="${tag#@}"
            remote_digest="$local_digest"
            pinned=$((pinned + 1))
        else
            # Get local and remote digests
            local_digest=$(__gash_get_local_digest "$resolved_image")
            __gash_remote_digest=""
            __gash_get_remote_digest "$registry" "$name" "$tag" >/dev/null 2>&1 || true
            remote_digest="$__gash_remote_digest"

            if [[ -z "$remote_digest" ]]; then
                status="ERROR"
                error_msg="${__gash_registry_curl_error:-registry_unreachable}"
                errors=$((errors + 1))
            elif [[ -z "$local_digest" ]]; then
                status="NOT_PULLED"
                updates=$((updates + 1))
            elif [[ "$upgradeable" -eq 0 ]]; then
                status="PINNED"
                pinned=$((pinned + 1))
            elif [[ "$local_digest" == "$remote_digest" ]]; then
                status="CURRENT"
                current=$((current + 1))
            else
                status="UPDATE"
                updates=$((updates + 1))
            fi
        fi

        # Store result
        results+=("$service_name|$resolved_image|$local_digest|$remote_digest|$status|$upgradeable|$error_msg")

    done <<<"$services"

    # Output
    if [[ $json_output -eq 1 ]]; then
        # JSON output
        echo -n '{"path":"'"$(__gash_json_escape "$path")"'","compose_file":"'"$(__gash_json_escape "$(basename "$compose_file")")"'","services":['
        local first=1
        for r in "${results[@]}"; do
            IFS='|' read -r svc img local_d remote_d stat upgr err <<<"$r"
            [[ $first -eq 0 ]] && echo -n ','
            first=0
            echo -n '{"name":"'"$(__gash_json_escape "$svc")"'","image":"'"$(__gash_json_escape "$img")"'"'
            echo -n ',"local_digest":"'"${local_d:-null}"'"'
            echo -n ',"remote_digest":"'"${remote_d:-null}"'"'
            echo -n ',"status":"'"$stat"'"'
            echo -n ',"upgradeable":'"$([[ $upgr -eq 1 ]] && echo 'true' || echo 'false')"
            [[ -n "$err" ]] && echo -n ',"error":"'"$(__gash_json_escape "$err")"'"'
            echo -n '}'
        done
        echo '],"summary":{"total":'"$total"',"updates":'"$updates"',"current":'"$current"',"pinned":'"$pinned"',"errors":'"$errors"'}}'
    else
        # Human-readable output
        echo ""
        printf "${__GASH_BOLD_WHITE}Checking ${__GASH_BOLD_CYAN}%s${__GASH_COLOR_OFF}\n" "$compose_file"
        echo ""

        # Table header
        printf "${__GASH_BOLD_WHITE}%-15s %-40s %-12s${__GASH_COLOR_OFF}\n" "SERVICE" "IMAGE" "STATUS"
        printf "%s\n" "────────────────────────────────────────────────────────────────────"

        for r in "${results[@]}"; do
            IFS='|' read -r svc img local_d remote_d stat upgr err <<<"$r"

            # Truncate long image names
            local display_img="$img"
            [[ ${#display_img} -gt 38 ]] && display_img="${display_img:0:35}..."

            # Color-coded status
            local status_color=""
            case "$stat" in
                UPDATE | NOT_PULLED) status_color="${__GASH_BOLD_YELLOW}" ;;
                CURRENT) status_color="${__GASH_BOLD_GREEN}" ;;
                PINNED) status_color="${__GASH_BOLD_CYAN}" ;;
                ERROR | UNKNOWN) status_color="${__GASH_BOLD_RED}" ;;
            esac

            printf "%-15s %-40s ${status_color}%-12s${__GASH_COLOR_OFF}\n" "$svc" "$display_img" "$stat"
        done

        echo ""
        printf "Summary: "
        [[ $updates -gt 0 ]] && printf "${__GASH_BOLD_YELLOW}%d update(s)${__GASH_COLOR_OFF} " "$updates"
        [[ $current -gt 0 ]] && printf "${__GASH_BOLD_GREEN}%d current${__GASH_COLOR_OFF} " "$current"
        [[ $pinned -gt 0 ]] && printf "${__GASH_BOLD_CYAN}%d pinned${__GASH_COLOR_OFF} " "$pinned"
        [[ $errors -gt 0 ]] && printf "${__GASH_BOLD_RED}%d error(s)${__GASH_COLOR_OFF}" "$errors"
        echo ""

        # Show error details if any
        if [[ $errors -gt 0 ]]; then
            echo ""
            printf "${__GASH_BOLD_RED}Errors:${__GASH_COLOR_OFF}\n"
            for r in "${results[@]}"; do
                IFS='|' read -r svc img local_d remote_d stat upgr err <<<"$r"
                if [[ "$stat" == "ERROR" ]]; then
                    # Parse registry info for better error message
                    local normalized
                    normalized=$(__gash_normalize_image "$img")
                    IFS='|' read -r reg _ _ <<<"$normalized"

                    printf "  ${__GASH_BOLD_WHITE}%s${__GASH_COLOR_OFF} (%s)\n" "$svc" "$img"
                    printf "    Registry: %s\n" "$reg"

                    case "$err" in
                        timeout)
                            printf "    Error: Connection timed out\n"
                            printf "    Hint: Registry may be slow or unreachable\n"
                            ;;
                        dns_error)
                            printf "    Error: DNS resolution failed\n"
                            printf "    Hint: Check network connectivity and registry hostname\n"
                            ;;
                        connection_refused)
                            printf "    Error: Connection refused\n"
                            printf "    Hint: Registry may be down or port blocked\n"
                            ;;
                        ssl_error)
                            printf "    Error: SSL/TLS handshake failed\n"
                            printf "    Hint: Check certificate validity\n"
                            ;;
                        unauthorized)
                            printf "    Error: Authentication required (401)\n"
                            case "$reg" in
                                ghcr.io)
                                    printf "    Hint: Set GITHUB_TOKEN for authenticated access\n"
                                    ;;
                                *)
                                    printf "    Hint: Registry requires authentication\n"
                                    ;;
                            esac
                            ;;
                        forbidden)
                            printf "    Error: Access denied (403)\n"
                            printf "    Hint: Check credentials and repository permissions\n"
                            ;;
                        not_found)
                            printf "    Error: Image not found (404)\n"
                            printf "    Hint: Check image name and tag are correct\n"
                            ;;
                        rate_limited)
                            printf "    Error: Rate limit exceeded (429)\n"
                            case "$reg" in
                                docker.io)
                                    printf "    Hint: Docker Hub limits unauthenticated pulls. Try logging in.\n"
                                    ;;
                                *)
                                    printf "    Hint: Wait and retry, or authenticate for higher limits\n"
                                    ;;
                            esac
                            ;;
                        server_error)
                            printf "    Error: Registry server error (5xx)\n"
                            printf "    Hint: Temporary issue, retry later\n"
                            ;;
                        registry_unreachable | *)
                            printf "    Error: Could not reach registry or get manifest\n"
                            case "$reg" in
                                ghcr.io)
                                    printf "    Hint: Try setting GITHUB_TOKEN for authenticated access\n"
                                    ;;
                                docker.io)
                                    printf "    Hint: Check network or Docker Hub rate limits\n"
                                    ;;
                                *)
                                    printf "    Hint: Registry may require authentication\n"
                                    ;;
                            esac
                            ;;
                    esac
                fi
            done
        fi

        if [[ $updates -gt 0 ]]; then
            echo ""
            printf "${__GASH_BOLD_WHITE}Run 'docker_compose_upgrade' to update services with mutable tags.${__GASH_COLOR_OFF}\n"
        fi
    fi
}

# Upgrade docker-compose services (only those with mutable tags).
# Usage: docker_compose_upgrade [PATH] [--dry-run] [--force]
# Options:
#   --dry-run   Show what would be done without executing
#   --force     Force pull even for pinned versions
#   -h          Show help
docker_compose_upgrade() {
    # Help
    if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
        cat <<'EOF'
docker_compose_upgrade - Upgrade Docker Compose services

USAGE:
  docker_compose_upgrade [PATH] [OPTIONS]

OPTIONS:
  PATH        Directory containing docker-compose.yml (default: current dir)
  --dry-run   Show what would be done without executing
  --force     Force upgrade even for pinned versions
  --yes       Skip the interactive confirmation (REQUIRED in headless mode)
  --no-color  Disable ANSI colors
  -h          Show this help

BEHAVIOR:
  Only upgrades services with mutable tags (latest, main, dev, etc.).
  Services with pinned versions (e.g., nginx:1.25.3) are skipped.
  Use --force to override and upgrade all services.
  In headless mode (GASH_HEADLESS=1) the upgrade refuses to run without
  an explicit --yes: pulling images and restarting a stack must never
  happen as a silent side effect of automation.

EXAMPLES:
  docker_compose_upgrade                  Upgrade in current directory
  docker_compose_upgrade /path/to/app     Upgrade specific project
  docker_compose_upgrade --dry-run        Preview changes
EOF
        return 0
    fi

    __gash_require_docker || return 1
    __gash_require_compose || return 1

    local path="."
    local dry_run=0
    local force=0
    local assume_yes=0
    local no_color=0

    # Parse arguments
    for arg in "$@"; do
        case "$arg" in
            --dry-run) dry_run=1 ;;
            -n)
                # -n means "limit N" across Gash 2.x; dry-run is spelled out.
                __gash_error "-n is not --dry-run anymore; use --dry-run"
                return 1
                ;;
            --force | -f) force=1 ;;
            --yes | -y) assume_yes=1 ;;
            --no-color) no_color=1 ;;
            -*)
                __gash_error "Unknown option: $arg"
                return 1
                ;;
            *) path="$arg" ;;
        esac
    done

    # Local color scope
    eval "$(__gash_color_scope "$no_color")"

    # Resolve path
    path="$(realpath -m "$path" 2>/dev/null)" || path="$PWD"

    # Find compose file
    local compose_file=""
    for f in docker-compose.yml docker-compose.yaml compose.yml compose.yaml; do
        [[ -f "$path/$f" ]] && {
            compose_file="$path/$f"
            break
        }
    done

    if [[ -z "$compose_file" ]]; then
        __gash_error "No docker-compose.yml found in $path"
        return 1
    fi

    local compose_cmd
    compose_cmd=$(__gash_compose_cmd)
    # Every compose call names the project explicitly: no cd in the
    # caller's shell.
    local -a dc
    read -ra dc <<<"$compose_cmd"
    dc+=(-f "$compose_file" --project-directory "$path")

    echo ""
    printf "${__GASH_BOLD_WHITE}=== Docker Compose Upgrade ===${__GASH_COLOR_OFF}\n"
    printf "Path: ${__GASH_BOLD_CYAN}%s${__GASH_COLOR_OFF}\n" "$path"
    echo ""

    if [[ $dry_run -eq 1 ]]; then
        printf "${__GASH_BOLD_YELLOW}[DRY-RUN] No changes will be made${__GASH_COLOR_OFF}\n\n"
    fi

    # Check what needs updating
    local services upgradeable_services=()
    services=$(__gash_parse_compose_services "$compose_file")
    local env_file="$path/.env"

    while IFS='|' read -r service_name image_ref; do
        local resolved_image
        resolved_image=$(__gash_resolve_env_vars "$image_ref" "$env_file")

        local normalized
        normalized=$(__gash_normalize_image "$resolved_image")
        IFS='|' read -r registry name tag <<<"$normalized"

        if [[ $force -eq 1 ]] || __gash_is_upgradeable "$tag"; then
            upgradeable_services+=("$service_name")
            printf "  ${__GASH_BOLD_GREEN}[UPGRADE]${__GASH_COLOR_OFF} %s (%s)\n" "$service_name" "$resolved_image"
        else
            printf "  ${__GASH_BOLD_CYAN}[SKIP]${__GASH_COLOR_OFF} %s (%s) - pinned version\n" "$service_name" "$resolved_image"
        fi
    done <<<"$services"

    echo ""

    if [[ ${#upgradeable_services[@]} -eq 0 ]]; then
        printf "${__GASH_BOLD_YELLOW}No services to upgrade (all pinned).${__GASH_COLOR_OFF}\n"
        printf "Use --force to upgrade pinned versions.\n"
        return 0
    fi

    if [[ $dry_run -eq 1 ]]; then
        printf "Would run:\n"
        printf "  %s pull %s\n" "${dc[*]}" "${upgradeable_services[*]}"
        printf "  %s stop %s\n" "${dc[*]}" "${upgradeable_services[*]}"
        printf "  %s up -d --remove-orphans %s\n" "${dc[*]}" "${upgradeable_services[*]}"
        printf "  docker image prune -f\n"
        return 0
    fi

    # Safety confirmation. --yes skips it explicitly; in headless mode the
    # upgrade REFUSES without --yes (an agent must opt in, never restart a
    # stack as a silent default).
    if [[ $assume_yes -ne 1 ]]; then
        if [[ "${GASH_HEADLESS:-}" == "1" ]]; then
            __gash_error "Refusing to upgrade in headless mode without --yes (pull + restart is destructive)."
            return 1
        fi
        printf "\n${__GASH_BOLD_YELLOW}This will:${__GASH_COLOR_OFF}\n"
        printf "  - pull new images for: %s\n" "${upgradeable_services[*]}"
        printf "  - stop and recreate those containers\n"
        printf "  - remove containers of this project no longer in %s (--remove-orphans)\n" "${compose_file##*/}"
        printf "  - remove dangling images on this host, from any project (docker image prune -f)\n"
        if ! needs_confirm_prompt "Proceed with upgrade?"; then
            printf "Upgrade cancelled.\n"
            return 0
        fi
    fi

    local pull_failed=0
    __gash_step 1 4 "Pulling updated images..."
    if ! "${dc[@]}" pull "${upgradeable_services[@]}"; then
        __gash_error "Failed to pull images. Aborting upgrade."
        pull_failed=1
    fi

    # Only proceed if pull succeeded
    if [[ $pull_failed -eq 0 ]]; then
        __gash_step 2 4 "Stopping old containers..."
        "${dc[@]}" stop "${upgradeable_services[@]}" 2>/dev/null || true

        __gash_step 3 4 "Starting updated containers..."
        if ! "${dc[@]}" up -d --remove-orphans "${upgradeable_services[@]}"; then
            __gash_error "Failed to start containers. Check logs with: ${dc[*]} logs"
            return 1
        fi

        __gash_step 4 4 "Cleaning old images..."
        docker image prune -f >/dev/null 2>&1 || true

        echo ""
        printf "${__GASH_BOLD_GREEN}=== Upgrade Complete ===${__GASH_COLOR_OFF}\n"
        echo ""

        # Show status
        "${dc[@]}" ps
    else
        return 1
    fi
}

# Scan directories for docker-compose files.
# Usage: docker_compose_scan [BASEDIR] [--depth N] [--json]
# Options:
#   --depth N   Maximum search depth (default: 3)
#   --json      Output in JSON format
#   -h          Show help
docker_compose_scan() {
    # Help
    if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
        cat <<'EOF'
docker_compose_scan - Scan for Docker Compose projects

USAGE:
  docker_compose_scan [BASEDIR] [OPTIONS]

OPTIONS:
  BASEDIR     Base directory to scan (default: current dir)
  --depth N   Maximum search depth (default: 3)
  --json      Output in JSON format
  --no-color  Disable ANSI colors
  -h          Show this help

EXAMPLES:
  docker_compose_scan                     Scan current directory
  docker_compose_scan ~ --depth 2         Scan home with depth 2
  docker_compose_scan /projects --json    JSON output
EOF
        return 0
    fi

    local basedir="."
    local depth=3
    local json_output=0
    local no_color=0

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --depth | -d)
                depth="$2"
                shift 2 || {
                    __gash_error "$1 needs a value"
                    return 1
                }
                ;;
            --json | -j)
                json_output=1
                shift
                ;;
            --no-color)
                no_color=1
                shift
                ;;
            -*) shift ;;
            *)
                basedir="$1"
                shift
                ;;
        esac
    done

    # Local color scope
    eval "$(__gash_color_scope "$no_color")"

    # Resolve basedir
    basedir="$(realpath -m "$basedir" 2>/dev/null)" || basedir="$PWD"

    # Find compose files
    local compose_files
    compose_files=$(find "$basedir" -maxdepth "$depth" -type f \
        \( -name "docker-compose.yml" -o -name "docker-compose.yaml" \
        -o -name "compose.yml" -o -name "compose.yaml" \) \
        2>/dev/null | sort)

    if [[ -z "$compose_files" ]]; then
        if [[ $json_output -eq 1 ]]; then
            echo '{"base_path":"'"$(__gash_json_escape "$basedir")"'","compose_files":[],"total_found":0}'
        else
            echo "No docker-compose files found in $basedir (depth: $depth)"
        fi
        return 0
    fi

    if [[ $json_output -eq 1 ]]; then
        # JSON output
        echo -n '{"base_path":"'"$(__gash_json_escape "$basedir")"'","depth":'"$depth"',"compose_files":['
        local first=1
        local total=0
        while IFS= read -r file; do
            total=$((total + 1))
            local dir
            dir=$(dirname "$file")
            local fname
            fname=$(basename "$file")

            # Get service names
            local services
            services=$(__gash_parse_compose_services "$file" | cut -d'|' -f1 | tr '\n' ',' | sed 's/,$//')

            [[ $first -eq 0 ]] && echo -n ','
            first=0
            echo -n '{"path":"'"$(__gash_json_escape "$dir")"'","file":"'"$(__gash_json_escape "$fname")"'","services":["'
            echo -n "${services//,/\",\"}"
            echo -n '"]}'
        done <<<"$compose_files"
        echo '],"total_found":'"$total"'}'
    else
        # Human-readable output
        echo ""
        printf "${__GASH_BOLD_WHITE}Scanning for Docker Compose files in %s (depth: %d)${__GASH_COLOR_OFF}\n" "$basedir" "$depth"
        echo ""

        local total=0
        while IFS= read -r file; do
            total=$((total + 1))
            local dir
            dir=$(dirname "$file")

            printf "${__GASH_BOLD_CYAN}%s${__GASH_COLOR_OFF}\n" "$dir"

            # List services
            local services
            services=$(__gash_parse_compose_services "$file")
            while IFS='|' read -r svc img; do
                printf "  - %s: %s\n" "$svc" "$img"
            done <<<"$services"
            echo ""
        done <<<"$compose_files"

        printf "Found ${__GASH_BOLD_WHITE}%d${__GASH_COLOR_OFF} compose file(s)\n" "$total"
    fi
}

# LLM-optimized version of docker_compose_check.
# Always outputs JSON, no colors, minimal tokens.
# Usage: llm_docker_check [PATH]
llm_docker_check() {
    docker_compose_check "${1:-.}" --json
}

# -----------------------------------------------------------------------------
# Short Aliases
# -----------------------------------------------------------------------------
alias dcc='docker_compose_check'
alias dcup2='docker_compose_upgrade'
alias dcscan='docker_compose_scan'

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

    __gash_register_help "docker_compose_check" \
        --aliases "dcc" \
        --module "docker-compose" \
        --short "Check for available Docker Compose image updates" \
        --see-also "docker_compose_upgrade docker_compose_scan" \
        <<'HELP'
USAGE
  docker_compose_check [PATH] [--json]

EXAMPLES
  # Check for updates in current directory
  dcc

  # Check a specific project
  dcc /opt/myapp

  # JSON output for scripting
  dcc --json

STATUS CODES
  UPDATE     New image available on registry
  UP-TO-DATE Image matches remote digest
  PINNED     Version is pinned (e.g. nginx:1.25.3), skipped
  ERROR      Could not reach registry

NOTES
  Queries Docker Hub and GHCR registries to compare
  local image digests with remote ones.
HELP

    __gash_register_help "docker_compose_upgrade" \
        --aliases "dcup2" \
        --module "docker-compose" \
        --short "Upgrade Docker Compose services to latest images" \
        --see-also "docker_compose_check docker_compose_scan" \
        <<'HELP'
USAGE
  docker_compose_upgrade [PATH] [--dry-run] [--force]

EXAMPLES
  # Upgrade services with mutable tags (latest, main, dev)
  dcup2

  # Preview what would be upgraded without doing it
  dcup2 --dry-run

  # Force upgrade even for pinned versions
  dcup2 --force

  # Upgrade a specific project
  dcup2 /opt/myapp

NOTES
  Only upgrades services with mutable tags (latest, main, dev, etc.).
  Pinned versions (e.g. nginx:1.25.3) are skipped unless --force is used.
  Pulls new images, recreates affected containers, removes old images.
HELP

    __gash_register_help "docker_compose_scan" \
        --aliases "dcscan" \
        --module "docker-compose" \
        --short "Scan directories for Docker Compose projects" \
        --see-also "docker_compose_check docker_compose_upgrade" \
        <<'HELP'
USAGE
  docker_compose_scan [BASEDIR] [--depth N] [--json]

EXAMPLES
  # Scan current directory for compose files
  dcscan

  # Scan /opt with limited depth
  dcscan /opt --depth 2

  # JSON output for scripting
  dcscan /home/deploy --json

  # Full audit: scan then check each project
  dcscan /opt --depth 3
HELP

    __gash_register_help "llm_docker_check" \
        --module "docker-compose" \
        --short "Docker Compose update check (JSON output for AI agents)" \
        --see-also "docker_compose_check" \
        <<'HELP'
USAGE
  llm_docker_check [PATH]

EXAMPLES
  # Check for updates (JSON output)
  llm_docker_check

  # Check a specific path
  llm_docker_check /opt/myapp

NOTES
  Wrapper around docker_compose_check --json.
  Designed for LLM/AI agent consumption.
HELP

fi # end help registration guard
