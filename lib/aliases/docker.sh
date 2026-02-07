#!/usr/bin/env bash

# Gash Aliases: Docker
# Docker and Docker Compose aliases.

# Helper function for adding aliases if binary exists
# Uses type -P to get only external binary path (not aliases/functions)
__gash_add_docker_alias() {
    local binary_name="$1"
    local alias_name="$2"
    local alias_command="$3"

    local BINARY
    BINARY=$(type -P "$binary_name" 2>/dev/null) || true

    if [[ -n "$BINARY" ]]; then
        alias "$alias_name"="$alias_command"
    fi
}

# Docker container aliases
__gash_add_docker_alias "docker" "dcls" "docker container ls -a"
__gash_add_docker_alias "docker" "dclsr" "docker container ls"
__gash_add_docker_alias "docker" "dils" "docker image ls"
__gash_add_docker_alias "docker" "dirm" "docker image prune -a"
__gash_add_docker_alias "docker" "dstop" "docker stop"
__gash_add_docker_alias "docker" "dstart" "docker start"
__gash_add_docker_alias "docker" "dexec" "docker exec -it"
__gash_add_docker_alias "docker" "dcrm" "docker container rm"
__gash_add_docker_alias "docker" "drm" "docker rm"
__gash_add_docker_alias "docker" "drmi" "docker rmi"
__gash_add_docker_alias "docker" "dlogs" "docker logs -f"
__gash_add_docker_alias "docker" "dinspect" "docker inspect"
__gash_add_docker_alias "docker" "dnetls" "docker network ls"

# Function-based aliases (check if functions exist)
if declare -f docker_stop_all >/dev/null 2>&1; then
    __gash_add_docker_alias "docker" "dstopall" "docker_stop_all"
fi
if declare -f docker_start_all >/dev/null 2>&1; then
    __gash_add_docker_alias "docker" "dstartall" "docker_start_all"
fi
if declare -f docker_prune_all >/dev/null 2>&1; then
    __gash_add_docker_alias "docker" "dpruneall" "docker_prune_all"
fi

# Docker Compose aliases (v1 - docker-compose binary)
__gash_add_docker_alias "docker-compose" "dc" "docker-compose"
__gash_add_docker_alias "docker-compose" "dcup" "docker-compose up -d"
__gash_add_docker_alias "docker-compose" "dcdown" "docker-compose down"
__gash_add_docker_alias "docker-compose" "dclogs" "docker-compose logs -f"
__gash_add_docker_alias "docker-compose" "dcb" "docker-compose build"
__gash_add_docker_alias "docker-compose" "dcrestart" "docker-compose restart"
__gash_add_docker_alias "docker-compose" "dcps" "docker-compose ps"
__gash_add_docker_alias "docker-compose" "dcpull" "docker-compose pull"

# Docker Compose smart upgrade functions (from docker-compose.sh module)
if declare -f docker_compose_check >/dev/null 2>&1; then
    alias dcc='docker_compose_check'
fi
if declare -f docker_compose_upgrade >/dev/null 2>&1; then
    alias dcup2='docker_compose_upgrade'
fi
if declare -f docker_compose_scan >/dev/null 2>&1; then
    alias dcscan='docker_compose_scan'
fi

# Cleanup helper function
unset -f __gash_add_docker_alias
