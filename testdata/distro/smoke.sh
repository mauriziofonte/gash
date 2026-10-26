#!/bin/sh
# Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# The distro smoke test, inside a disposable container of a bare distro
# image (run.sh starts it):
#
#   1. install.sh --check names what the image lacks (git, tar, curl...)
#      and the command that installs it on this distro;
#   2. install.sh --assume-yes installs those packages, clones the Gash
#      tree of this commit (/smoke/tree, tagged with its VERSION) and
#      fetches the release archive from file:///smoke/rel, checked against
#      the pins of release/SHA256SUMS;
#   3. gash_doctor and a few llm_* reads run in an interactive Bash;
#   4. gash_doctor --install --all --yes installs every package the
#      prerequisites catalog names for this distro, and each must provide
#      its tool (the catalog's package names, checked on the real distro).
#
# Only bash itself is installed by hand, where the image lacks it: the
# installer is a Bash script.
set -eu

. /etc/os-release
echo "== ${PRETTY_NAME:-$ID} ($(uname -m))"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

if ! command -v bash >/dev/null 2>&1; then
    case "$ID" in
        alpine) apk add --no-cache bash >/dev/null ;;
        *) fail "no bash in this image" ;;
    esac
fi
bash --version | head -n 1

export HOME=/root
mkdir -p "$HOME"
: >"$HOME/.bashrc"
cd "$HOME"
export GASH_RELEASE_URL=file:///smoke/rel GASH_INSTALL_GIT_REPO=/smoke/tree
# The tree belongs to the user who built it, not to root.
export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0='*'

echo "== install.sh --check"
check_rc=0
bash /smoke/tree/install.sh --check </dev/null || check_rc=$?
echo "(status $check_rc)"

echo "== install.sh --assume-yes"
bash /smoke/tree/install.sh --assume-yes </dev/null || fail "install.sh --assume-yes"
bash /smoke/tree/install.sh --check </dev/null || fail "install.sh --check after the install"
test -f "$HOME/.gash/gash.sh" || fail "no ~/.gash/gash.sh"
grep -q '>>> GASH START >>>' "$HOME/.bashrc" || fail "no Gash block in ~/.bashrc"

# The binary comes from the release archive, verified against the pins.
out=$(bash --norc -i -c 'source "$HOME/.gash/gash.sh"; gash version' </dev/null 2>/dev/null) || fail "gash version: $out"
# An interactive shell prints its banner first: the version is the last line.
out=$(printf '%s\n' "$out" | tail -n 1)
echo "$out"
case "$out" in
    "gash 3."*" linux/"*) ;;
    *) fail "unexpected gash version output: $out" ;;
esac

doctor=$(bash --norc -i -c 'source "$HOME/.gash/gash.sh"; gash_doctor --no-color' </dev/null 2>&1) || true
echo "$doctor" | grep -E '^(OK|Error|Warning): +(bash|gash|linux_|gash-hosts-helper)' || true
echo "$doctor" | grep -q "matches release/SHA256SUMS" || fail "gash_doctor did not verify the binary: $doctor"
if echo "$doctor" | grep -q '^Error:'; then
    fail "gash_doctor reports an error: $doctor"
fi

# A few reads, through the sourced shell as an agent calls them.
reads=$(GASH_HEADLESS=1 bash --norc -c '
    source "$HOME/.gash/gash.sh"
    llm_sysinfo
    llm_tree /usr/share --depth 1 --top 2
    llm_find "*.conf" /etc --type f -n 2
    llm_git_status /tmp
' 2>&1) || true
echo "$reads" | cut -c1-160
echo "$reads" | grep -q '"kernel"' || fail "llm_sysinfo: $reads"
echo "$reads" | grep -q '"top":\[' || fail "llm_tree: $reads"
echo "$reads" | grep -q '^/etc/.*\.conf$' || fail "llm_find: $reads"
echo "$reads" | grep -q '"error":"not_a_git_repo"' || fail "llm_git_status: $reads"

# Every package of the catalog for this distro.
version=$(cat "$HOME/.gash/VERSION")
bin="$HOME/.local/share/gash/versions/$version/gash"
echo "== gash_doctor --install --all --yes"
"$bin" gash_doctor --install --all --yes --no-color </dev/null || true
json=$("$bin" gash_doctor --json) || true
# One tool per line; the ones still missing that had an install command
# (a non-free one may need a repository the image does not enable).
bad=$(printf '%s\n' "$json" | sed 's/},{/}\n{/g' | grep '"found":false' | grep '"install":' | grep -v '"non_free":true' || true)
printf '%s\n' "$json" | sed 's/},{/}\n{/g' | grep '"found":false' | cut -c1-200 || true
[ -z "$bad" ] || fail "packages that do not provide their tool: $bad"
echo "== OK"
