#!/usr/bin/env bash
# Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# Runs the distro smoke test (smoke.sh) in a disposable container of IMAGE,
# on the architecture of this machine: builds the release archive of this
# commit as release.yml does, a Gash repository from `git archive` that pins
# those binaries, tagged with the VERSION, and mounts both.
#
#   testdata/distro/run.sh mirror.gcr.io/library/rockylinux:8
set -euo pipefail

image=${1:?usage: $0 IMAGE}
here=$(cd "$(dirname "$0")" && pwd)
mod=$(cd "$here/../.." && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *)
        echo "unsupported machine $(uname -m)" >&2
        exit 2
        ;;
esac
version=$(cat "$mod/VERSION")
mkdir -p "$stage/rel" "$stage/bin" "$stage/tree"
export GOOS=linux GOARCH=$arch CGO_ENABLED=0
(cd "$mod" && go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X github.com/mauriziofonte/gash/internal/buildinfo.Version=$version" \
    -o "$stage/bin/gash" ./cmd/gash)
(cd "$mod" && go build -trimpath -buildvcs=false -ldflags "-s -w" \
    -o "$stage/bin/gash-hosts-helper" ./cmd/gash-hosts-helper)
asset="gash_${version}_linux_${arch}.tar.gz"
tar -czf "$stage/rel/$asset" -C "$stage/bin" gash gash-hosts-helper
(cd "$stage/rel" && sha256sum "$asset" >checksums.txt)
(cd "$mod" && git archive HEAD) | tar -x -C "$stage/tree"
# The smoke checks the installer, not the pins: pin the binaries just built
# (release/SHA256SUMS goes stale between release bumps).
for b in gash gash-hosts-helper; do
    sum=$(sha256sum "$stage/bin/$b" | cut -d' ' -f1)
    sed -i "s|^[0-9a-f]*  linux_${arch}/$b\$|$sum  linux_${arch}/$b|" "$stage/tree/release/SHA256SUMS"
done
grep "linux_${arch}/" "$stage/tree/release/SHA256SUMS"
# The installer clones it and checks out its newest release tag.
git -C "$stage/tree" init -q
git -C "$stage/tree" add -A
git -C "$stage/tree" -c user.name=smoke -c user.email=smoke@localhost commit -q -m smoke
git -C "$stage/tree" tag "v$version"
cp "$here/smoke.sh" "$stage/smoke.sh"
chmod -R a+rX "$stage"

docker run --rm -v "$stage:/smoke:ro" "$image" /bin/sh /smoke/smoke.sh
