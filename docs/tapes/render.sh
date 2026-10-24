#!/usr/bin/env bash
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# render.sh [TAPE...]: records docs/tapes/*.tape (or the named ones) into
# docs/assets/*.gif with vhs, against gash and gash-hosts-helper built from
# this checkout (stamped with VERSION, as a release build). Needs go, vhs,
# ttyd, ffmpeg, git, jq and sqlite3; hte.tape also needs Apache, PHP-FPM
# and passwordless sudo, and changes the machine (gash hte install), so it
# is recorded only where GASH_DEMO_HTE=1 (the Demos workflow sets it).
#
#   docs/tapes/render.sh                 # every tape
#   docs/tapes/render.sh demo agents     # only these
set -euo pipefail

repo=$(cd "$(dirname "$0")/../.." && pwd)
cd "$repo"
# GASH_BIN set: use that build (gash-hosts-helper next to it).
if [ -z "${GASH_BIN:-}" ]; then
    bin=$(mktemp -d)
    trap 'rm -rf "$bin"' EXIT
    version=$(cat VERSION)
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X github.com/mauriziofonte/gash/internal/buildinfo.Version=$version" -o "$bin/gash" ./cmd/gash
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$bin/gash-hosts-helper" ./cmd/gash-hosts-helper
    export GASH_BIN=$bin/gash
fi

tapes=()
if (($#)); then
    for t in "$@"; do tapes+=("docs/tapes/${t%.tape}.tape"); done
else
    for t in docs/tapes/*.tape; do
        if [ "$t" = docs/tapes/hte.tape ] && [ "${GASH_DEMO_HTE:-}" != 1 ]; then
            echo "skip $t (set GASH_DEMO_HTE=1 on a disposable machine)"
            continue
        fi
        tapes+=("$t")
    done
fi
for t in "${tapes[@]}"; do
    echo "== $t"
    rm -f /tmp/gash-demo-hte.check
    vhs "$t"
    # vhs records whatever the commands print, errors included: for the
    # HTE tape, check that the site really answered.
    if [ "$t" = docs/tapes/hte.tape ] && ! grep -q '^Acme shop: PHP' /tmp/gash-demo-hte.check 2>/dev/null; then
        echo "render.sh: $t: https://shop.test/ did not answer during the recording" >&2
        exit 1
    fi
done
