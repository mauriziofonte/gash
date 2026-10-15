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
    trap "command rm -f '${tmpfile}'; trap - RETURN" RETURN

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

    if [[ -n "$digest" ]]; then echo "$digest"; fi
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

    local path="."
    local json_output=0
    local no_color=0

    # Parse arguments
    local arg
    for arg in "$@"; do
        case "$arg" in
            --json | -j) json_output=1 ;;
            --no-color) no_color=1 ;;
            -*)
                if [[ " $* " == *" --json "* || " $* " == *" -j "* ]]; then
                    __gash_json_error "unknown_option" "$arg" "RETRY" "Use a path and --json"
                else
                    __gash_error "Unknown option: $arg"
                fi
                return 1
                ;;
            *) path="$arg" ;;
        esac
    done

    # --json errors follow the agent error contract (stderr envelope).
    if [[ $json_output -eq 1 ]]; then
        if ! type -P docker >/dev/null 2>&1; then
            __gash_json_error "docker_not_found" "Docker is not installed" "CONTINUE" "Skip the image update check"
            return 1
        fi
        if ! __gash_compose_cmd &>/dev/null; then
            __gash_json_error "compose_not_installed" "Docker Compose is not installed" "CONTINUE" "Skip the image update check"
            return 1
        fi
    else
        __gash_require_docker || return 1
        __gash_require_compose || return 1
    fi

    # Local color scope (ignored when --json since JSON path doesn't emit ANSI)
    __gash_want_colors "$no_color" || eval "$__GASH_COLOR_OFF_LOCALS"

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
            __gash_json_error "compose_not_found" "$path" "RETRY" "Pass the directory that holds docker-compose.yml, docker-compose.yaml, compose.yml or compose.yaml"
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
            __gash_json_error "no_services" "$compose_file" "CONTINUE" "The compose file has no service with an image: nothing to check"
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
            # Unknown digests are JSON null, never the string "null".
            if [[ -n "$local_d" ]]; then
                echo -n ',"local_digest":"'"$(__gash_json_escape "$local_d")"'"'
            else
                echo -n ',"local_digest":null'
            fi
            if [[ -n "$remote_d" ]]; then
                echo -n ',"remote_digest":"'"$(__gash_json_escape "$remote_d")"'"'
            else
                echo -n ',"remote_digest":null'
            fi
            echo -n ',"status":"'"$stat"'"'
            echo -n ',"upgradeable":'"$([[ $upgr -eq 1 ]] && echo 'true' || echo 'false')"
            [[ -n "$err" ]] && echo -n ',"error":"'"$(__gash_json_escape "$err")"'"'
            echo -n '}'
        done
        echo '],"summary":{"total":'"$total"',"updates":'"$updates"',"current":'"$current"',"pinned":'"$pinned"',"errors":'"$errors"'}}'
    else
        # Human-readable output
        local W="" S="" I="" E="" B="" M="" A="" R=""
        if __gash_use_color; then
            W="$__GASH_COLOR_WARNING" S="$__GASH_COLOR_SUCCESS" I="$__GASH_COLOR_INFO"
            E="$__GASH_COLOR_ERROR" B="$__GASH_BOLD" M="$__GASH_COLOR_MUTED"
            A="$__GASH_COLOR_ACCENT" R="$__GASH_COLOR_OFF"
        fi
        echo ""
        __gash_heading "Checking $compose_file"
        echo ""

        # One row per service; the status takes its status color (hidden
        # column 4).
        local style
        {
            printf 'SERVICE\tIMAGE\tSTATUS\t\n'
            for r in "${results[@]}"; do
                IFS='|' read -r svc img local_d remote_d stat upgr err <<<"$r"
                case "$stat" in
                    UPDATE | NOT_PULLED) style=warning ;;
                    CURRENT) style=success ;;
                    PINNED) style=info ;;
                    *) style=error ;;
                esac
                printf '%s\t%s\t%s\t%s\n' "$svc" "$img" "$stat" "$style"
            done
        } | __gash_table --gap 1 --min 1:15 --min 2:40 --max 2:38 --color 1:accent --cell-color 3:4

        echo ""
        printf '%sSummary:%s ' "$B" "$R"
        if ((updates > 0)); then printf '%s%d update(s)%s ' "$W" "$updates" "$R"; fi
        if ((current > 0)); then printf '%s%d current%s ' "$S" "$current" "$R"; fi
        if ((pinned > 0)); then printf '%s%d pinned%s ' "$I" "$pinned" "$R"; fi
        if ((errors > 0)); then printf '%s%d error(s)%s' "$E" "$errors" "$R"; fi
        echo ""

        # Show error details if any
        if [[ $errors -gt 0 ]]; then
            echo ""
            __gash_heading "Errors:"
            local reg msg hint
            for r in "${results[@]}"; do
                IFS='|' read -r svc img local_d remote_d stat upgr err <<<"$r"
                [[ "$stat" == "ERROR" ]] || continue
                # Parse registry info for better error message
                local normalized
                normalized=$(__gash_normalize_image "$img")
                IFS='|' read -r reg _ _ <<<"$normalized"

                case "$err" in
                    timeout)
                        msg="Connection timed out"
                        hint="Registry may be slow or unreachable"
                        ;;
                    dns_error)
                        msg="DNS resolution failed"
                        hint="Check network connectivity and registry hostname"
                        ;;
                    connection_refused)
                        msg="Connection refused"
                        hint="Registry may be down or port blocked"
                        ;;
                    ssl_error)
                        msg="SSL/TLS handshake failed"
                        hint="Check certificate validity"
                        ;;
                    unauthorized)
                        msg="Authentication required (401)"
                        case "$reg" in
                            ghcr.io) hint="Set GITHUB_TOKEN for authenticated access" ;;
                            *) hint="Registry requires authentication" ;;
                        esac
                        ;;
                    forbidden)
                        msg="Access denied (403)"
                        hint="Check credentials and repository permissions"
                        ;;
                    not_found)
                        msg="Image not found (404)"
                        hint="Check image name and tag are correct"
                        ;;
                    rate_limited)
                        msg="Rate limit exceeded (429)"
                        case "$reg" in
                            docker.io) hint="Docker Hub limits unauthenticated pulls. Try logging in." ;;
                            *) hint="Wait and retry, or authenticate for higher limits" ;;
                        esac
                        ;;
                    server_error)
                        msg="Registry server error (5xx)"
                        hint="Temporary issue, retry later"
                        ;;
                    registry_unreachable | *)
                        msg="Could not reach registry or get manifest"
                        case "$reg" in
                            ghcr.io) hint="Try setting GITHUB_TOKEN for authenticated access" ;;
                            docker.io) hint="Check network or Docker Hub rate limits" ;;
                            *) hint="Registry may require authentication" ;;
                        esac
                        ;;
                esac

                printf '  %s%s%s (%s)\n' "$A" "$svc" "$R" "$img"
                __gash_kv --indent 4 "Registry:" "$reg"
                __gash_mark --indent 4 error "$msg"
                printf '    %sHint: %s%s\n' "$M" "$hint" "$R"
            done
        fi

        if [[ $updates -gt 0 ]]; then
            echo ""
            __gash_print bold "Run 'docker_compose_upgrade' to update services with mutable tags."
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
    __gash_want_colors "$no_color" || eval "$__GASH_COLOR_OFF_LOCALS"

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

    __gash_section "Docker Compose Upgrade"
    __gash_kv --value-color accent "Path:" "$path"
    echo ""

    if [[ $dry_run -eq 1 ]]; then
        __gash_print warning "[DRY-RUN] No changes will be made"
        echo ""
    fi

    # Check what needs updating: one plan row per service (label, name,
    # image; the label's status color in hidden column 4).
    local services upgradeable_services=() plan_rows=()
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
            plan_rows+=("[UPGRADE]"$'\t'"$service_name"$'\t'"($resolved_image)"$'\t'"success")
        else
            plan_rows+=("[SKIP]"$'\t'"$service_name"$'\t'"($resolved_image) - pinned version"$'\t'"info")
        fi
    done <<<"$services"
    printf '%s\n' "${plan_rows[@]}" |
        __gash_table --no-header --indent 2 --gap 1 --cell-color 1:4 --color 2:accent --color 3:muted

    echo ""

    if [[ ${#upgradeable_services[@]} -eq 0 ]]; then
        __gash_print warning "No services to upgrade (all pinned)."
        __gash_print muted "Use --force to upgrade pinned versions."
        return 0
    fi

    if [[ $dry_run -eq 1 ]]; then
        __gash_heading "Would run:"
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
        echo ""
        __gash_heading "This will:"
        __gash_list "pull new images for: ${upgradeable_services[*]}" \
            "stop and recreate those containers" \
            "remove containers of this project no longer in ${compose_file##*/} (--remove-orphans)" \
            "remove dangling images on this host, from any project (docker image prune -f)"
        if ! needs_confirm_prompt "Proceed with upgrade?"; then
            __gash_info "Upgrade cancelled."
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
        __gash_mark ok "Upgrade Complete"
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
    __gash_want_colors "$no_color" || eval "$__GASH_COLOR_OFF_LOCALS"

    # Resolve basedir
    basedir="$(realpath -m "$basedir" 2>/dev/null)" || basedir="$PWD"

    # Find compose files
    local compose_files
    compose_files=$(find "$basedir" -maxdepth "$depth" -type f \
        \( -name "docker-compose.yml" -o -name "docker-compose.yaml" \
        -o -name "compose.yml" -o -name "compose.yaml" \) \
        2>/dev/null | sort) || compose_files=""

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
        local B="" R=""
        if __gash_use_color; then
            B="$__GASH_BOLD" R="$__GASH_COLOR_OFF"
        fi
        echo ""
        __gash_heading "Scanning for Docker Compose files in $basedir (depth: $depth)"
        echo ""

        local total=0
        while IFS= read -r file; do
            total=$((total + 1))
            local dir
            dir=$(dirname "$file")

            __gash_print accent "$dir"

            # List services ("name: image"), none for a file without images.
            local services svc img
            local -a items=()
            services=$(__gash_parse_compose_services "$file")
            while IFS='|' read -r svc img; do
                if [[ -n "$svc" ]]; then items+=("$svc: $img"); fi
            done <<<"$services"
            if ((${#items[@]} > 0)); then
                __gash_list "${items[@]}"
            fi
            echo ""
        done <<<"$compose_files"

        printf 'Found %s%d%s compose file(s)\n' "$B" "$total" "$R"
    fi
}

# LLM-optimized version of docker_compose_check.
# Always outputs JSON, no colors, minimal tokens.
# Usage: llm_docker_check [PATH]
llm_docker_check() {
    needs_help "llm_docker_check" "llm_docker_check [PATH]" "Compose image update check as JSON" "${1-}" && return
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
