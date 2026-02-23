#!/usr/bin/env bash

# Gash Module: Docker Operations
# Functions for managing Docker containers, images, and cleanup.
#
# Dependencies: core/output.sh, core/validation.sh, core/utils.sh
#
# Public functions (LONG name + SHORT alias):
#   docker_stop_all (dsa)   - Stop all running containers
#   docker_start_all (daa)  - Start all stopped containers
#   docker_prune_all (dpa)  - Remove all containers, images, volumes, networks

# -----------------------------------------------------------------------------
# Container Management
# -----------------------------------------------------------------------------

# Stop all running Docker containers.
# Usage: docker_stop_all
docker_stop_all() {
    __gash_require_docker || return 1

    local containers
    containers=$(docker ps -q 2>/dev/null)

    if [[ -z "$containers" ]]; then
        __gash_info "No running containers to stop."
        return 0
    fi

    __gash_info "Stopping Docker containers..."
    docker stop $containers 2>/dev/null || true
}

# Start all stopped Docker containers.
# Usage: docker_start_all
docker_start_all() {
    __gash_require_docker || return 1

    local containers
    containers=$(docker ps -aq --filter "status=exited" --filter "status=created" 2>/dev/null)

    if [[ -z "$containers" ]]; then
        __gash_info "No stopped containers to start."
        return 0
    fi

    __gash_info "Starting Docker containers..."
    docker start $containers 2>/dev/null || true
}

# -----------------------------------------------------------------------------
# Cleanup
# -----------------------------------------------------------------------------

# Remove all Docker containers, images, volumes, and networks.
# Usage: docker_prune_all
# Warning: This is a destructive operation!
docker_prune_all() {
    __gash_require_docker || return 1

    if ! needs_confirm_prompt "${__GASH_BOLD_YELLOW}Warning:${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}Remove all Docker containers, images, volumes, and networks?${__GASH_COLOR_OFF}"; then
        return 0
    fi

    local ids

    __gash_step 1 6 "Stopping Docker containers..."
    ids=$(docker ps -q 2>/dev/null)
    [[ -n "$ids" ]] && docker stop $ids 2>/dev/null || true

    __gash_step 2 6 "Removing Docker containers..."
    ids=$(docker ps -aq 2>/dev/null)
    [[ -n "$ids" ]] && docker rm $ids 2>/dev/null || true

    __gash_step 3 6 "Removing Docker images..."
    ids=$(docker images -q 2>/dev/null)
    [[ -n "$ids" ]] && docker rmi $ids 2>/dev/null || true

    __gash_step 4 6 "Removing Docker volumes..."
    ids=$(docker volume ls -q 2>/dev/null)
    [[ -n "$ids" ]] && docker volume rm $ids 2>/dev/null || true

    __gash_step 5 6 "Removing Docker networks..."
    docker network ls --format '{{.Name}}' 2>/dev/null | grep -ivF -e bridge -e host -e none | while IFS= read -r net; do
        docker network rm "$net" 2>/dev/null || true
    done

    __gash_step 6 6 "Cleaning Docker environment..."
    docker system prune --volumes -a --force
}

# -----------------------------------------------------------------------------
# Short Aliases
# -----------------------------------------------------------------------------
alias dsa='docker_stop_all'
alias daa='docker_start_all'
alias dpa='docker_prune_all'

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

__gash_register_help "docker_stop_all" \
    --aliases "dsa" \
    --module "docker" \
    --short "Stop all running Docker containers" \
    --see-also "docker_start_all docker_prune_all" \
    <<'HELP'
USAGE
  docker_stop_all

EXAMPLES
  # Stop every running container
  dsa

  # Restart all containers: stop then start
  dsa && daa
HELP

__gash_register_help "docker_start_all" \
    --aliases "daa" \
    --module "docker" \
    --short "Start all stopped Docker containers" \
    --see-also "docker_stop_all" \
    <<'HELP'
USAGE
  docker_start_all

EXAMPLES
  # Start all stopped containers
  daa

  # Restart everything
  dsa && daa
HELP

__gash_register_help "docker_prune_all" \
    --aliases "dpa" \
    --module "docker" \
    --short "Remove ALL Docker containers, images, volumes, and networks" \
    --see-also "docker_stop_all" \
    <<'HELP'
USAGE
  docker_prune_all

EXAMPLES
  # Nuclear cleanup (interactive confirmation)
  dpa

NOTES
  WARNING: This is destructive. It will:
    1. Stop all running containers
    2. Remove all containers
    3. Remove all images
    4. Remove all volumes (DATA LOSS)
    5. Remove all networks
    6. Run docker system prune
  You will be asked for confirmation before proceeding.
HELP

fi  # end help registration guard
