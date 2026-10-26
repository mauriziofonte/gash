#!/bin/sh
# Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# The distro smoke test, inside a disposable distro container (run.sh
# starts it): a Gash tree in ~/.gash whose bootstrap fetches the release
# archive of this commit from file:///smoke/rel, checks it against the
# pins of release/SHA256SUMS, then gash_doctor and a few llm_* reads run
# in an interactive Bash. What the image lacks to fetch the binary (tar,
# gzip, curl) is installed first with its package manager.
set -eu

. /etc/os-release
echo "== ${PRETTY_NAME:-$ID} ($(uname -m))"

pm=""
for p in apt-get dnf microdnf yum zypper pacman apk; do
    if command -v "$p" >/dev/null 2>&1; then
        pm=$p
        break
    fi
done
missing=""
command -v bash >/dev/null 2>&1 || missing="$missing bash"
command -v tar >/dev/null 2>&1 || missing="$missing tar"
command -v gzip >/dev/null 2>&1 || missing="$missing gzip"
command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 || missing="$missing curl"
if [ -n "$missing" ]; then
    echo "== installing:$missing ($pm)"
    # shellcheck disable=SC2086 # one word per package
    case "$pm" in
        apt-get) apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq $missing >/dev/null ;;
        dnf | microdnf | yum) "$pm" install -y $missing >/dev/null ;;
        zypper) zypper -n install $missing >/dev/null ;;
        pacman) pacman -Sy --noconfirm $missing >/dev/null ;;
        apk) apk add --no-cache $missing >/dev/null ;;
        *)
            echo "no package manager for:$missing" >&2
            exit 1
            ;;
    esac
fi

bash --version | head -n 1
export HOME=/root
mkdir -p "$HOME"
cp -R /smoke/tree "$HOME/.gash"
export GASH_RELEASE_URL=file:///smoke/rel

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

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
echo "$reads" | grep -q '"error":"not_a_git_repo"\|"error":"git_not_found"' || fail "llm_git_status: $reads"
echo "== OK"
