#!/usr/bin/env bash
#
# Extract a specific Bash release from the official `bash:VERSION` Docker
# image so the test suite can run against it on the host:
#
#   tests/ci/fetch-bash.sh 4.4 /opt/bash-4.4   # -> /opt/bash-4.4/bin/bash
#   tests/run.sh --bash /opt/bash-4.4/bin/bash
#
# The image ships a musl-linked binary; we copy it together with its loader
# and libncursesw, then point its ELF interpreter/rpath at the copies with
# patchelf. Requires: docker, patchelf.

set -euo pipefail

version="${1:?usage: fetch-bash.sh VERSION DEST_DIR}"
dest="${2:?usage: fetch-bash.sh VERSION DEST_DIR}"

mkdir -p "$dest/bin"
dest="$(cd "$dest" && pwd)"

cid="$(docker create "bash:$version")"
trap 'docker container rm --force "$cid" >/dev/null 2>&1 || true' EXIT

docker cp -L "$cid:/usr/local/bin/bash" "$dest/bash"
docker cp -L "$cid:/lib/ld-musl-x86_64.so.1" "$dest/ld-musl.so"
docker cp -L "$cid:/usr/lib/libncursesw.so.6" "$dest/libncursesw.so.6"

patchelf --set-interpreter "$dest/ld-musl.so" --set-rpath "$dest" "$dest/bash"
ln -sf "$dest/bash" "$dest/bin/bash"

"$dest/bin/bash" -c 'printf "bash %s ready at %s\n" "$BASH_VERSION" "$0"' "$dest/bin/bash"
