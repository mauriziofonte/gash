#!/usr/bin/env bash
# shellcheck disable=SC2139 # aliases intentionally expand values at definition time

# Gash Aliases: Docker
# Docker and Docker Compose aliases, each only when its program is on PATH
# (type -P: binaries only, not aliases or functions).

if type -P docker >/dev/null 2>&1; then
    # Docker container aliases
    alias dcls="docker container ls -a"
    alias dclsr="docker container ls"
    alias dils="docker image ls"
    alias dirm="docker image prune -a"
    alias dstop="docker stop"
    alias dstart="docker start"
    alias dexec="docker exec -it"
    alias dcrm="docker container rm"
    alias drm="docker rm"
    alias drmi="docker rmi"
    alias dlogs="docker logs -f"
    alias dinspect="docker inspect"
    alias dnetls="docker network ls"

    # Function-based aliases (check if functions exist)
    if declare -F docker_stop_all >/dev/null 2>&1; then
        alias dstopall="docker_stop_all"
    fi
    if declare -F docker_start_all >/dev/null 2>&1; then
        alias dstartall="docker_start_all"
    fi
    if declare -F docker_prune_all >/dev/null 2>&1; then
        alias dpruneall="docker_prune_all"
    fi
fi

# Docker Compose aliases (v1 - docker-compose binary)
if type -P docker-compose >/dev/null 2>&1; then
    alias dc="docker-compose"
    alias dcup="docker-compose up -d"
    alias dcdown="docker-compose down"
    alias dclogs="docker-compose logs -f"
    alias dcb="docker-compose build"
    alias dcrestart="docker-compose restart"
    alias dcps="docker-compose ps"
    alias dcpull="docker-compose pull"
fi

# Docker Compose smart upgrade functions (from docker-compose.sh module)
if declare -F docker_compose_check >/dev/null 2>&1; then
    alias dcc='docker_compose_check'
fi
if declare -F docker_compose_upgrade >/dev/null 2>&1; then
    alias dcup2='docker_compose_upgrade'
fi
if declare -F docker_compose_scan >/dev/null 2>&1; then
    alias dcscan='docker_compose_scan'
fi
