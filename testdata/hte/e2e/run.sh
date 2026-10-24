#!/usr/bin/env bash
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# Runs the gash hte end-to-end test in a disposable container: the system
# Apache and PHP-FPM of a distro image, then the test binary (built with
# -tags hte_e2e) installs gash hte as an unprivileged user, exercises the
# stack and uninstalls; setup.sh then compares /etc with its state before
# install. Nothing runs on the host system: the host only builds the
# binaries and starts docker.
#
#   testdata/hte/e2e/run.sh [ubuntu]   # Ubuntu 24.04: the hte-cli tree migrated (PHP 8.3)
#   testdata/hte/e2e/run.sh debian     # Debian 12: the same story on its PHP 8.2
#   testdata/hte/e2e/run.sh fedora     # Fedora 44: httpd, the distro php-fpm and Remi's php83
#   testdata/hte/e2e/run.sh arch       # Arch: httpd, php-fpm and php-legacy-fpm
#   GASH_HTE_E2E_BASE=... run.sh D     # another image of the same family as D
#   GASH_HTE_E2E_SYSTEMD=1 run.sh D    # systemd as PID 1, autostart systemd (debian, ubuntu)
#   GASH_HTE_E2E_MULTIPHP=1 run.sh     # PHP 7.4, 8.2 and 8.4 next to the distro 8.3 (ubuntu)
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
mod=$(cd "$here/../../.." && pwd)
distro=${1:-ubuntu}
case "$distro" in
    debian) def=mirror.gcr.io/library/debian:12 dockerfile=Dockerfile.base ;;
    ubuntu) def=mirror.gcr.io/library/ubuntu:24.04 dockerfile=Dockerfile.base ;;
    fedora) def=mirror.gcr.io/library/fedora:44 dockerfile=Dockerfile.fedora ;;
    arch) def=mirror.gcr.io/library/archlinux:latest dockerfile=Dockerfile.arch ;;
    *)
        echo "usage: $0 [debian|ubuntu|fedora|arch]" >&2
        exit 2
        ;;
esac
# The multi-PHP image adds 7.4, 8.2 and 8.4 next to Ubuntu's 8.3; Fedora
# (Remi) and Arch (php-legacy) run two versions in their own story. The
# systemd image is apt-based.
if [ "${GASH_HTE_E2E_MULTIPHP:-}" = 1 ] && [ "$distro" != ubuntu ]; then
    echo "GASH_HTE_E2E_MULTIPHP=1 runs on ubuntu only" >&2
    exit 2
fi
if [ "${GASH_HTE_E2E_SYSTEMD:-}" = 1 ] && [ "$distro" != ubuntu ] && [ "$distro" != debian ]; then
    echo "GASH_HTE_E2E_SYSTEMD=1 runs on debian and ubuntu only" >&2
    exit 2
fi
base=${GASH_HTE_E2E_BASE:-$def}
image=${GASH_HTE_E2E_IMAGE:-gash-hte-e2e-base:$(basename "$base" | tr ':' '-')}
stage=${GASH_HTE_E2E_STAGE:-$(mktemp -d)}
mkdir -p "$stage"

if ! docker image inspect "$image" >/dev/null 2>&1; then
    docker build --build-arg BASE="$base" -t "$image" -f "$here/$dockerfile" "$here"
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
    # systemd as PID 1 (LEDGER HTE-73), in a private cgroup namespace, so
    # its cgroup mount stays the container's own. --privileged, as for the
    # story below: the docker-default AppArmor profile of the GitHub
    # Actions runners denies that mount even with CAP_SYS_ADMIN. The
    # container has no network and masks every unit that touches the
    # kernel or devices (Dockerfile.systemd); the story uses the high
    # ports, so /proc/sys is never written.
    sd=${image/gash-hte-e2e-base/gash-hte-e2e-systemd}
    if ! docker image inspect "$sd" >/dev/null 2>&1; then
        docker build --build-arg BASE="$image" -t "$sd" -f "$here/Dockerfile.systemd" "$here"
    fi
    name=gash-hte-e2e-systemd-$$
    docker run -d --rm --name "$name" --network none --privileged --cgroupns=private \
        --tmpfs /run --tmpfs /run/lock -v "$stage:/e2e:ro" "$sd" >/dev/null
    trap 'docker rm -f "$name" >/dev/null 2>&1' EXIT
    state=
    for _ in $(seq 100); do
        state=$(docker exec "$name" systemctl is-system-running 2>/dev/null || true)
        case "$state" in
            running | degraded) break ;;
        esac
        sleep 0.2
    done
    echo "== systemd: $state"
    if [ "$state" = degraded ]; then
        docker exec "$name" systemctl --failed --no-pager || true
    fi
    rc=0
    docker exec -e GASH_HTE_E2E_SYSTEMD=1 -e GASH_HTE_E2E_DISTRO="$distro" "$name" /e2e/setup.sh || rc=$?
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
    -e GASH_HTE_E2E_MULTIPHP="${GASH_HTE_E2E_MULTIPHP:-}" -e GASH_HTE_E2E_DISTRO="$distro" \
    -v "$stage:/e2e:ro" "$image" /e2e/setup.sh
