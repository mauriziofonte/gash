#!/usr/bin/env bash
# Tests for docker-compose module
# These tests use mocks to avoid requiring real Docker installation.

describe "Docker Compose"

# =============================================================================
# Helper Parsing Tests (no Docker required)
# =============================================================================

it "__gash_parse_compose_services extracts services correctly" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
version: "3"
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
  db:
    image: postgres:15
    environment:
      - POSTGRES_PASSWORD=secret
  redis:
    image: redis:7-alpine
EOF

    out=$(__gash_parse_compose_services "$tmp")
    [[ $(echo "$out" | wc -l) -eq 3 ]]
    [[ "$out" == *"web|nginx:latest"* ]]
    [[ "$out" == *"db|postgres:15"* ]]
    [[ "$out" == *"redis|redis:7-alpine"* ]]
'

it "__gash_parse_compose_services handles quoted images" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
services:
  app:
    image: "ghcr.io/org/app:v1.0"
  api:
    image: '\''my-registry.io/api:latest'\''
EOF

    out=$(__gash_parse_compose_services "$tmp")
    [[ "$out" == *"app|ghcr.io/org/app:v1.0"* ]]
    [[ "$out" == *"api|my-registry.io/api:latest"* ]]
'

it "__gash_parse_compose_services handles services without image (build)" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    cat > "$tmp" << EOF
services:
  app:
    build: .
    ports:
      - "3000:3000"
  db:
    image: postgres:15
EOF

    out=$(__gash_parse_compose_services "$tmp")
    # Only db should be extracted (app has no image)
    [[ $(echo "$out" | grep -c "|") -eq 1 ]]
    [[ "$out" == *"db|postgres:15"* ]]
'

it "__gash_resolve_env_vars expands variables from .env" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    echo "TAG=v2.0" > "$tmpdir/.env"
    echo "REGISTRY=myregistry.io" >> "$tmpdir/.env"

    out=$(__gash_resolve_env_vars "\${REGISTRY}/app:\${TAG}" "$tmpdir/.env")
    [[ "$out" == "myregistry.io/app:v2.0" ]]
'

it "__gash_resolve_env_vars uses defaults when var not set" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT
    echo "" > "$tmp"

    out=$(__gash_resolve_env_vars "nginx:\${TAG:-latest}" "$tmp")
    [[ "$out" == "nginx:latest" ]]

    out=$(__gash_resolve_env_vars "\${IMAGE:-nginx}:1.25" "$tmp")
    [[ "$out" == "nginx:1.25" ]]
'

it "__gash_resolve_env_vars handles quoted values in .env" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    cat > "$tmpdir/.env" << EOF
TAG="v1.0.0"
IMAGE='\''nginx'\''
EOF

    out=$(__gash_resolve_env_vars "\${IMAGE}:\${TAG}" "$tmpdir/.env")
    [[ "$out" == "nginx:v1.0.0" ]]
'

# =============================================================================
# Image Normalization Tests (no Docker required)
# =============================================================================

it "__gash_normalize_image handles official Docker Hub images" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    out=$(__gash_normalize_image "nginx")
    [[ "$out" == "docker.io|library/nginx|latest" ]]

    out=$(__gash_normalize_image "nginx:1.25")
    [[ "$out" == "docker.io|library/nginx|1.25" ]]

    out=$(__gash_normalize_image "redis:7-alpine")
    [[ "$out" == "docker.io|library/redis|7-alpine" ]]
'

it "__gash_normalize_image handles namespaced Docker Hub images" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    out=$(__gash_normalize_image "ollama/ollama:latest")
    [[ "$out" == "docker.io|ollama/ollama|latest" ]]

    out=$(__gash_normalize_image "bitnami/postgresql:15")
    [[ "$out" == "docker.io|bitnami/postgresql|15" ]]
'

it "__gash_normalize_image handles GHCR images" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    out=$(__gash_normalize_image "ghcr.io/open-webui/open-webui:main")
    [[ "$out" == "ghcr.io|open-webui/open-webui|main" ]]
'

it "__gash_normalize_image handles digest-pinned images" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    out=$(__gash_normalize_image "nginx@sha256:abc123def456")
    [[ "$out" == "docker.io|library/nginx|@sha256:abc123def456" ]]
'

it "__gash_normalize_image handles custom registries" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    out=$(__gash_normalize_image "registry.example.com/myapp:v1")
    [[ "$out" == "registry.example.com|myapp|v1" ]]

    out=$(__gash_normalize_image "my.private.registry:5000/org/app:latest")
    [[ "$out" == "my.private.registry:5000|org/app|latest" ]]
'

# =============================================================================
# Upgradeable Tag Tests (no Docker required)
# =============================================================================

it "__gash_is_upgradeable returns true for mutable tags" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    __gash_is_upgradeable "latest" || exit 1
    __gash_is_upgradeable "main" || exit 1
    __gash_is_upgradeable "dev" || exit 1
    __gash_is_upgradeable "edge" || exit 1
    __gash_is_upgradeable "nightly" || exit 1
'

it "__gash_is_upgradeable returns true for partial versions" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    __gash_is_upgradeable "15" || exit 1      # Major only
    __gash_is_upgradeable "1.25" || exit 1    # Major.minor
    __gash_is_upgradeable "8.1" || exit 1
'

it "__gash_is_upgradeable returns false for pinned versions" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    __gash_is_upgradeable "1.25.3" && exit 1
    __gash_is_upgradeable "v2.0.1" && exit 1
    __gash_is_upgradeable "@sha256:abc123" && exit 1
    true
'

it "__gash_is_upgradeable handles tags with suffixes" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    __gash_is_upgradeable "7-alpine" || exit 1       # Base "7" is major only
    __gash_is_upgradeable "1.25-slim" || exit 1     # Base "1.25" is major.minor
    __gash_is_upgradeable "1.25.3-alpine" && exit 1 # Base is pinned
    true
'

# =============================================================================
# Compose Command Detection Tests (uses mocks)
# =============================================================================

it "__gash_compose_cmd detects docker compose v2" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"

    out=$(__gash_compose_cmd)
    [[ "$out" == "docker compose" ]]
'

it "__gash_compose_cmd falls back to docker-compose v1" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    # Create temp bin with only docker-compose, not docker compose
    tmpbin=$(mktemp -d)
    trap "rm -rf $tmpbin" EXIT

    # Create a docker that does not support "compose" subcommand
    cat > "$tmpbin/docker" << '\''MOCK'\''
#!/bin/bash
if [[ "$1" == "compose" ]]; then
    exit 1
fi
exit 0
MOCK
    chmod +x "$tmpbin/docker"

    # Link docker-compose mock
    ln -s "$ROOT/tests/mocks/bin/docker-compose" "$tmpbin/docker-compose"

    export PATH="$tmpbin:$PATH"

    out=$(__gash_compose_cmd)
    [[ "$out" == "docker-compose" ]]
'

it "__gash_require_compose fails without docker" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    # Empty PATH - no docker
    export PATH="/nonexistent"

    set +e
    __gash_require_compose 2>/dev/null
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
'

# =============================================================================
# docker_compose_check Tests (uses mocks)
# =============================================================================

it "docker_compose_check shows help with -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    out=$(docker_compose_check -h 2>&1)
    [[ "$out" == *"docker_compose_check"* ]]
    [[ "$out" == *"--json"* ]]
    [[ "$out" == *"USAGE"* ]]
'

it "docker_compose_check errors on missing compose file" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    set +e
    out=$(docker_compose_check "$tmpdir" --json 2>&1)
    set -e

    [[ "$out" == *"compose_not_found"* ]]
'

it "docker_compose_check --json returns valid JSON structure" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    cat > "$tmpdir/docker-compose.yml" << EOF
services:
  web:
    image: nginx:latest
  db:
    image: postgres:15.3
EOF

    out=$(docker_compose_check "$tmpdir" --json 2>/dev/null) || true

    # Check JSON structure
    [[ "$out" == "{"* ]]
    [[ "$out" == *"\"services\":"* ]]
    [[ "$out" == *"\"summary\":"* ]]
    [[ "$out" == *"\"total\":"* ]]
'

it "docker_compose_check correctly identifies pinned vs upgradeable" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    cat > "$tmpdir/docker-compose.yml" << EOF
services:
  upgradeable:
    image: nginx:latest
  pinned:
    image: postgres:15.3.1
EOF

    out=$(docker_compose_check "$tmpdir" --json 2>/dev/null) || true

    # upgradeable service should have upgradeable:true
    [[ "$out" == *"\"name\":\"upgradeable\""*"\"upgradeable\":true"* ]]
    # pinned service should have upgradeable:false
    [[ "$out" == *"\"name\":\"pinned\""*"\"upgradeable\":false"* ]]
'

it "docker_compose_check handles compose files with env vars" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    cat > "$tmpdir/docker-compose.yml" << EOF
services:
  app:
    image: \${REGISTRY:-docker.io}/myapp:\${TAG:-latest}
EOF

    cat > "$tmpdir/.env" << EOF
REGISTRY=ghcr.io/myorg
TAG=v1.0
EOF

    out=$(docker_compose_check "$tmpdir" --json 2>/dev/null) || true

    # Should resolve to ghcr.io/myorg/myapp:v1.0
    [[ "$out" == *"ghcr.io/myorg/myapp:v1.0"* ]]
'

# =============================================================================
# docker_compose_upgrade Tests (uses mocks)
# =============================================================================

it "docker_compose_upgrade shows help with -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    out=$(docker_compose_upgrade -h 2>&1)
    [[ "$out" == *"docker_compose_upgrade"* ]]
    [[ "$out" == *"--dry-run"* ]]
    [[ "$out" == *"--force"* ]]
'

it "docker_compose_upgrade errors on missing compose file" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    set +e
    out=$(docker_compose_upgrade "$tmpdir" 2>&1)
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"No docker-compose.yml"* ]]
'

it "docker_compose_upgrade --dry-run shows planned actions" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    cat > "$tmpdir/docker-compose.yml" << EOF
services:
  web:
    image: nginx:latest
  db:
    image: postgres:15.3.1
EOF

    out=$(docker_compose_upgrade "$tmpdir" --dry-run 2>&1)

    # Should show web as upgradeable
    [[ "$out" == *"[UPGRADE]"*"web"* ]]
    # Should show db as skipped (pinned)
    [[ "$out" == *"[SKIP]"*"db"* ]]
    # Should show "Would run" (dry-run)
    [[ "$out" == *"Would run"* ]]
    [[ "$out" == *"pull"* ]]
'

it "docker_compose_upgrade skips all pinned versions" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    cat > "$tmpdir/docker-compose.yml" << EOF
services:
  web:
    image: nginx:1.25.3
  db:
    image: postgres:15.3.1
EOF

    out=$(docker_compose_upgrade "$tmpdir" --dry-run 2>&1)

    # Both should be skipped
    [[ "$out" == *"No services to upgrade"* ]]
'

it "docker_compose_upgrade --force includes pinned versions" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    cat > "$tmpdir/docker-compose.yml" << EOF
services:
  web:
    image: nginx:1.25.3
EOF

    out=$(docker_compose_upgrade "$tmpdir" --dry-run --force 2>&1)

    # Should show web as upgradeable with --force
    [[ "$out" == *"[UPGRADE]"*"web"* ]]
    [[ "$out" == *"Would run"* ]]
'

it "docker_compose_upgrade executes commands in correct order" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"
    export GASH_HEADLESS=1  # Skip confirmation prompt

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    # Log file to track mock calls
    export MOCK_DOCKER_LOG="$tmpdir/docker.log"

    cat > "$tmpdir/docker-compose.yml" << EOF
services:
  web:
    image: nginx:latest
EOF

    # Run upgrade
    docker_compose_upgrade "$tmpdir" >/dev/null 2>&1 || true

    # Check that commands were called in order
    [[ -f "$MOCK_DOCKER_LOG" ]] || exit 1
    log=$(cat "$MOCK_DOCKER_LOG")

    [[ "$log" == *"compose pull"* ]] || exit 1
    [[ "$log" == *"compose stop"* ]] || exit 1
    [[ "$log" == *"compose up"* ]] || exit 1
'

# =============================================================================
# docker_compose_scan Tests
# =============================================================================

it "docker_compose_scan shows help with -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    out=$(docker_compose_scan -h 2>&1)
    [[ "$out" == *"docker_compose_scan"* ]]
    [[ "$out" == *"--depth"* ]]
    [[ "$out" == *"--json"* ]]
'

it "docker_compose_scan finds compose files" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    mkdir -p "$tmpdir/project1" "$tmpdir/project2"
    echo -e "services:\n  app:\n    image: nginx" > "$tmpdir/project1/docker-compose.yml"
    echo -e "services:\n  db:\n    image: postgres" > "$tmpdir/project2/compose.yaml"

    out=$(docker_compose_scan "$tmpdir" --json 2>/dev/null)
    [[ "$out" == *"\"total_found\":2"* ]]
    [[ "$out" == *"project1"* ]]
    [[ "$out" == *"project2"* ]]
'

it "docker_compose_scan respects depth limit" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    mkdir -p "$tmpdir/level1/level2/level3"
    echo -e "services:\n  a:\n    image: nginx" > "$tmpdir/level1/docker-compose.yml"
    echo -e "services:\n  b:\n    image: nginx" > "$tmpdir/level1/level2/level3/docker-compose.yml"

    # Depth 2 should only find level1
    out=$(docker_compose_scan "$tmpdir" --depth 2 --json 2>/dev/null)
    [[ "$out" == *"\"total_found\":1"* ]]
'

it "docker_compose_scan handles empty directory" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    out=$(docker_compose_scan "$tmpdir" --json 2>/dev/null)
    [[ "$out" == *"\"total_found\":0"* ]]
    [[ "$out" == *"\"compose_files\":[]"* ]]
'

it "docker_compose_scan finds all naming variants" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    mkdir -p "$tmpdir/p1" "$tmpdir/p2" "$tmpdir/p3" "$tmpdir/p4"
    echo -e "services:\n  a:\n    image: nginx" > "$tmpdir/p1/docker-compose.yml"
    echo -e "services:\n  a:\n    image: nginx" > "$tmpdir/p2/docker-compose.yaml"
    echo -e "services:\n  a:\n    image: nginx" > "$tmpdir/p3/compose.yml"
    echo -e "services:\n  a:\n    image: nginx" > "$tmpdir/p4/compose.yaml"

    out=$(docker_compose_scan "$tmpdir" --json 2>/dev/null)
    [[ "$out" == *"\"total_found\":4"* ]]
'

# =============================================================================
# Alias Tests
# =============================================================================

it "docker compose aliases are defined" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    alias dcc >/dev/null 2>&1 || exit 1
    alias dcup2 >/dev/null 2>&1 || exit 1
    alias dcscan >/dev/null 2>&1 || exit 1
'

# =============================================================================
# LLM Function Tests
# =============================================================================

it "llm_docker_check outputs JSON" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    cat > "$tmpdir/docker-compose.yml" << EOF
services:
  web:
    image: nginx:latest
EOF

    out=$(llm_docker_check "$tmpdir" 2>/dev/null) || true

    # Must be valid JSON
    [[ "$out" == "{"* ]]
    [[ "$out" == *"}"* ]]
'

# =============================================================================
# Edge Cases and Error Handling
# =============================================================================

it "handles compose file with syntax edge cases" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    tmp=$(mktemp)
    trap "rm -f $tmp" EXIT

    # Various edge cases in image specifications
    cat > "$tmp" << EOF
services:
  # Comment before service
  web:
    image: nginx:latest # inline comment
    ports:
      - "80:80"
  db:
    # Comment inside service
    image: postgres:15
  cache:
    image: redis  # No tag
  custom:
    image: "my.registry.io:5000/namespace/app:v1.0.0"
EOF

    out=$(__gash_parse_compose_services "$tmp")

    [[ "$out" == *"web|nginx:latest"* ]]
    [[ "$out" == *"db|postgres:15"* ]]
    [[ "$out" == *"cache|redis"* ]]
    [[ "$out" == *"custom|my.registry.io:5000/namespace/app:v1.0.0"* ]]
'

it "handles missing .env file gracefully" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    # .env file does not exist
    out=$(__gash_resolve_env_vars "nginx:\${TAG:-latest}" "/nonexistent/.env")

    # Should use default
    [[ "$out" == "nginx:latest" ]]
'

it "docker_compose_check handles empty services gracefully" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    # Compose file with no services
    cat > "$tmpdir/docker-compose.yml" << EOF
version: "3"
# No services defined
EOF

    set +e
    out=$(docker_compose_check "$tmpdir" --json 2>&1)
    set -e

    [[ "$out" == *"no_services"* ]]
'
