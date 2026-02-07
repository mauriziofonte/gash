#!/usr/bin/env bash
# Tests for docker.sh module (container management + cleanup)
# Uses mock docker binary for all tests.

describe "Docker"

# =============================================================================
# docker_stop_all Tests
# =============================================================================

it "docker_stop_all stops running containers" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"
    export MOCK_DOCKER_CONTAINERS=1

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT
    export MOCK_DOCKER_LOG="$tmpdir/docker.log"

    docker_stop_all 2>/dev/null

    log=$(cat "$MOCK_DOCKER_LOG")
    # Should use ps -q (running only, not -aq)
    [[ "$log" == *"docker ps -q"* ]]
    # Should stop the running container
    [[ "$log" == *"docker stop abc123"* ]]
'

it "docker_stop_all is no-op when no running containers" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"
    # MOCK_DOCKER_CONTAINERS not set - docker ps returns empty

    out=$(docker_stop_all 2>&1)
    [[ "$out" == *"No running containers"* ]]
'

it "docker_stop_all errors without docker" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="/nonexistent"

    set +e
    docker_stop_all 2>/dev/null
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
'

# =============================================================================
# docker_start_all Tests
# =============================================================================

it "docker_start_all starts only stopped containers" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"
    export MOCK_DOCKER_CONTAINERS=1

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT
    export MOCK_DOCKER_LOG="$tmpdir/docker.log"

    docker_start_all 2>/dev/null

    log=$(cat "$MOCK_DOCKER_LOG")
    # Should use --filter for stopped containers only
    [[ "$log" == *"--filter"* ]]
    # Should start only the stopped container
    [[ "$log" == *"docker start stopped123"* ]]
'

it "docker_start_all is no-op when no stopped containers" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"
    # No MOCK_DOCKER_CONTAINERS - docker ps returns empty

    out=$(docker_start_all 2>&1)
    [[ "$out" == *"No stopped containers"* ]]
'

it "docker_start_all errors without docker" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="/nonexistent"

    set +e
    docker_start_all 2>/dev/null
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
'

# =============================================================================
# docker_prune_all Tests
# =============================================================================

it "docker_prune_all filters networks by name not ID" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"
    export MOCK_DOCKER_CONTAINERS=1
    export MOCK_DOCKER_CUSTOM_NETWORKS=1

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT
    export MOCK_DOCKER_LOG="$tmpdir/docker.log"

    # Override confirmation prompt to auto-accept
    needs_confirm_prompt() { return 0; }

    docker_prune_all 2>/dev/null || true

    log=$(cat "$MOCK_DOCKER_LOG")
    # Must use --format (names), not -q (IDs)
    [[ "$log" == *"network ls --format"* ]]
'

it "docker_prune_all removes custom networks but preserves built-in" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"
    export MOCK_DOCKER_CONTAINERS=1
    export MOCK_DOCKER_CUSTOM_NETWORKS=1

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT
    export MOCK_DOCKER_LOG="$tmpdir/docker.log"

    needs_confirm_prompt() { return 0; }

    docker_prune_all 2>/dev/null || true

    log=$(cat "$MOCK_DOCKER_LOG")
    # Should remove custom networks
    [[ "$log" == *"docker network rm my_app_network"* ]]
    [[ "$log" == *"docker network rm test_network"* ]]
    # Should NOT remove built-in networks
    [[ "$log" != *"docker network rm bridge"* ]]
    [[ "$log" != *"docker network rm host"* ]]
    [[ "$log" != *"docker network rm none"* ]]
'

it "docker_prune_all guards empty container lists" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    export PATH="$ROOT/tests/mocks/bin:$PATH"
    # No MOCK_DOCKER_CONTAINERS - empty lists

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT
    export MOCK_DOCKER_LOG="$tmpdir/docker.log"

    needs_confirm_prompt() { return 0; }

    docker_prune_all 2>/dev/null || true

    log=$(cat "$MOCK_DOCKER_LOG")
    # Should still call system prune
    [[ "$log" == *"system prune"* ]]
    # Should NOT call docker stop (no containers)
    [[ "$log" != *"docker stop "*  ]]
'

# =============================================================================
# Alias Tests
# =============================================================================

it "docker aliases are defined" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    alias dsa >/dev/null 2>&1 || exit 1
    alias daa >/dev/null 2>&1 || exit 1
    alias dpa >/dev/null 2>&1 || exit 1
'
