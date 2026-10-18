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

export GOTOOLCHAIN=go1.27.2 CGO_ENABLED=0
(cd "$mod" && go test -c -tags hte_e2e -o "$stage/gash-e2e" ./internal/hte/e2e)
(cd "$mod" && go build -o "$stage/gash-hosts-helper" ./cmd/gash-hosts-helper)
cp "$here/setup.sh" "$stage/setup.sh"
rm -rf "$stage/V2"
cp -a "$mod/testdata/hte/legacy/V2" "$stage/V2"
chmod -R a+rX "$stage"

# systempaths=unconfined: the root step writes
# net.ipv4.ip_unprivileged_port_start, which starts at 1024. /etc/hosts
# stays Docker's bind mount (LEDGER HTE-69).
exec docker run --rm --init --network none \
    --security-opt systempaths=unconfined \
    --sysctl net.ipv4.ip_unprivileged_port_start=1024 \
    -v "$stage:/e2e:ro" "$image" /e2e/setup.sh
