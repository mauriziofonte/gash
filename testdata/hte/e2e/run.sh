#!/usr/bin/env bash
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# Runs the gash hte end-to-end test in a disposable container: the system
# Apache and PHP-FPM of the image serve the hte-cli V2 tree, then the test
# binary (built with -tags hte_e2e) installs gash hte as an unprivileged
# user, migrates the sites, exercises the stack and uninstalls; setup.sh
# then compares /etc with its state before install. Nothing runs on the
# host system: the host only builds the binaries and starts docker.
#
#   testdata/hte/e2e/run.sh            # Ubuntu 24.04
#   GASH_HTE_E2E_BASE=... run.sh       # another apt-based image
#   GASH_HTE_E2E_SYSTEMD=1 run.sh      # systemd as PID 1, autostart systemd
#   GASH_HTE_E2E_MULTIPHP=1 run.sh     # PHP 7.4, 8.2 and 8.4 next to the distro 8.3
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
mod=$(cd "$here/../../.." && pwd)
base=${GASH_HTE_E2E_BASE:-mirror.gcr.io/library/ubuntu:24.04}
image=${GASH_HTE_E2E_IMAGE:-gash-hte-e2e-base:$(basename "$base" | tr ':' '-')}
stage=${GASH_HTE_E2E_STAGE:-$(mktemp -d)}
mkdir -p "$stage"

if ! docker image inspect "$image" >/dev/null 2>&1; then
    docker build --build-arg BASE="$base" -t "$image" -f "$here/Dockerfile.base" "$here"
fi

if [ "${GASH_HTE_E2E_MULTIPHP:-}" = 1 ]; then
    # The official php:X.Y-fpm builds side by side (Dockerfile.multiphp).
    mp=${image/gash-hte-e2e-base/gash-hte-e2e-multiphp}
    if ! docker image inspect "$mp" >/dev/null 2>&1; then
        docker build --build-arg BASE="$image" -t "$mp" -f "$here/Dockerfile.multiphp" "$here"
    fi
    image=$mp
fi

export GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0
(cd "$mod" && go test -c -tags hte_e2e -o "$stage/gash-e2e" ./internal/hte/e2e)
(cd "$mod" && go build -o "$stage/gash-hosts-helper" ./cmd/gash-hosts-helper)
cp "$here/setup.sh" "$stage/setup.sh"
rm -rf "$stage/V2"
cp -a "$mod/testdata/hte/legacy/V2" "$stage/V2"
chmod -R a+rX "$stage"

if [ "${GASH_HTE_E2E_SYSTEMD:-}" = 1 ]; then
    # systemd as PID 1 (LEDGER HTE-73): it mounts its cgroup hierarchy
    # itself, which needs CAP_SYS_ADMIN; the private cgroup namespace
    # scopes that mount to the container's own cgroup. The story uses the
    # high ports, so /proc/sys stays read-only.
    sd=${image/gash-hte-e2e-base/gash-hte-e2e-systemd}
    if ! docker image inspect "$sd" >/dev/null 2>&1; then
        docker build --build-arg BASE="$image" -t "$sd" -f "$here/Dockerfile.systemd" "$here"
    fi
    name=gash-hte-e2e-systemd-$$
    docker run -d --rm --name "$name" --network none --cgroupns=private --cap-add SYS_ADMIN \
        --tmpfs /run --tmpfs /run/lock -v "$stage:/e2e:ro" "$sd" >/dev/null
    trap 'docker rm -f "$name" >/dev/null 2>&1' EXIT
    for _ in $(seq 50); do
        state=$(docker exec "$name" systemctl is-system-running 2>/dev/null || true)
        case "$state" in
            running | degraded) break ;;
        esac
        sleep 0.2
    done
    echo "== systemd: $state"
    rc=0
    docker exec -e GASH_HTE_E2E_SYSTEMD=1 "$name" /e2e/setup.sh || rc=$?
    exit "$rc"
fi

# --privileged: the root step lowers net.ipv4.ip_unprivileged_port_start
# (which --sysctl starts at 1024) to 80. systempaths=unconfined unmasks the
# read-only /proc/sys mount, but on a host whose Docker enforces the
# docker-default AppArmor profile (the GitHub Actions runners) that profile
# still denies the write: sysctl reports "Read-only file system", exits 0,
# and the value stays 1024. A privileged throwaway --network none container
# lifts both; it only ever touches its own disposable namespaces. /etc/hosts
# stays Docker's bind mount (LEDGER HTE-69).
exec docker run --rm --init --network none --privileged \
    --sysctl net.ipv4.ip_unprivileged_port_start=1024 \
    -e GASH_HTE_E2E_MULTIPHP="${GASH_HTE_E2E_MULTIPHP:-}" \
    -v "$stage:/e2e:ro" "$image" /e2e/setup.sh
