#!/usr/bin/env bash
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# extract.sh VERSION DEST: put GNU Bash VERSION on this (Linux x86_64) host
# as DEST/bin/bash, for the compat Bash matrix (testdata/bash/run.sh,
# design/testing.md 5 and 7, compat.md 1).
#
# The binary is the one of the official bash:VERSION image (Alpine, built
# from the GNU sources, linked to musl), copied out with docker; nothing is
# compiled. Its ELF interpreter is /lib/ld-musl-x86_64.so.1, so the image's
# musl loader is installed there and /etc/ld-musl-x86_64.path points it at
# DEST/lib (libncursesw): only musl programs read either file, so the
# host's glibc programs are untouched, and Bash runs with the host's GNU
# userland exactly as the generated Bash meets it on a user's machine.
# Needs docker and root (sudo when not root). GASH_BASH_IMAGE overrides
# the image repository (default mirror.gcr.io/library/bash).
set -euo pipefail

ver=${1:?usage: extract.sh VERSION DEST}
dest=${2:?usage: extract.sh VERSION DEST}
image="${GASH_BASH_IMAGE:-mirror.gcr.io/library/bash}:$ver"

case "$(uname -s)/$(uname -m)" in
    Linux/x86_64) ;;
    *)
        echo "extract.sh: Linux x86_64 only" >&2
        exit 1
        ;;
esac
sudo=""
[[ $(id -u) == 0 ]] || sudo=sudo

docker pull -q "$image" >/dev/null
cid=$(docker create "$image")
trap 'docker rm -f "$cid" >/dev/null' EXIT

rm -rf "$dest"
mkdir -p "$dest/bin" "$dest/lib"
docker cp "$cid:/usr/local/bin/bash" "$dest/bin/bash"
docker cp -L "$cid:/lib/ld-musl-x86_64.so.1" "$dest/lib/ld-musl-x86_64.so.1"
docker cp -L "$cid:/usr/lib/libncursesw.so.6" "$dest/lib/libncursesw.so.6"

$sudo install -m 0755 "$dest/lib/ld-musl-x86_64.so.1" /lib/ld-musl-x86_64.so.1
printf '%s\n' "$dest/lib" | $sudo tee /etc/ld-musl-x86_64.path >/dev/null

# shellcheck disable=SC2016 # expanded by the extracted bash
got=$("$dest/bin/bash" -c 'echo "${BASH_VERSINFO[0]}.${BASH_VERSINFO[1]}"')
if [[ $got != "$ver" ]]; then
    echo "extract.sh: $image gave Bash $got, not $ver" >&2
    exit 1
fi
# shellcheck disable=SC2016 # expanded by the extracted bash
echo "Bash $("$dest/bin/bash" -c 'echo "$BASH_VERSION"') in $dest/bin"
