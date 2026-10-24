#!/usr/bin/env bash
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# Runs the gash hte end-to-end story TestE2EDistro natively on a macOS
# GitHub Actions runner (there is no Docker there): Homebrew's httpd, php
# and php@8.3, the runner user as the user, its passwordless sudo for the
# root step. The runner is a disposable VM, and this script refuses to run
# anywhere else: install changes the system (the hosts helper, a sudoers
# drop-in, /etc/hosts, the port mechanism). Afterwards the system paths
# gash may touch are compared with their state before install.
#
#   testdata/hte/e2e/macos.sh          # in the hte-macos job of ci.yml
set -euo pipefail

if [ "$(uname -s)" != Darwin ] || [ "${GITHUB_ACTIONS:-}" != true ] || [ "${RUNNER_OS:-}" != macOS ]; then
    echo "macos.sh runs only on a GitHub Actions macOS runner: it changes the system" >&2
    exit 2
fi
here=$(cd "$(dirname "$0")" && pwd)
mod=$(cd "$here/../../.." && pwd)

# Homebrew as the runner image has it: no auto-update (the formulae come
# from the API), no cleanup, no dependents check.
export HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_ENV_HINTS=1
echo "== Homebrew httpd, php and php@8.3"
brew install httpd php php@8.3
prefix=$(brew --prefix)
"$prefix/bin/httpd" -v
"$prefix/opt/php/sbin/php-fpm" -v
"$prefix/opt/php@8.3/sbin/php-fpm" -v
# Nothing of Homebrew's runs: brew services is never used.
if pgrep -x httpd >/dev/null || pgrep -x php-fpm >/dev/null; then
    echo "an httpd or php-fpm runs before the test" >&2
    exit 2
fi

stage=$(mktemp -d)
export CGO_ENABLED=0
(cd "$mod" && go test -c -tags hte_e2e -o "$stage/gash-e2e" ./internal/hte/e2e)
(cd "$mod" && go build -o "$stage/gash-hosts-helper" ./cmd/gash-hosts-helper)

# The marker that lets the test binary (and its root step, which sudo
# starts with a reset environment) change this system: root's, for this
# run only.
marker=/var/run/gash-hte-e2e-ci
sudo sh -c ": >$marker && chmod 0644 $marker"
trap 'sudo rm -f "$marker"' EXIT

# Every system path install and uninstall may touch: kind, mode, owner,
# link target, and the content of each file. A path that does not exist
# is listed as absent, so a directory left behind shows.
roots=(/private/etc/hosts /private/etc/sudoers /private/etc/sudoers.d /private/etc/pf.conf /private/etc/pf.anchors
    /Library/LaunchDaemons /usr/local/libexec /private/var/lib)
snapshot() {
    local r
    for r in "${roots[@]}"; do
        if sudo test -e "$r"; then
            sudo find "$r" -exec stat -f '%N %HT %Lp %Su:%Sg %Y' {} +
            sudo find "$r" -type f -exec shasum -a 256 {} +
        else
            echo "absent $r"
        fi
    done | LC_ALL=C sort
}
out=$(mktemp -d)
snapshot >"$out/before.txt"

echo "== pf probe (temporary)"
"$here/macos-pf-probe.sh" || true

echo "== the test, as $USER"
rc=0
(cd "$HOME" && env -i HOME="$HOME" USER="$USER" LOGNAME="$USER" PATH=/usr/bin:/bin:/usr/sbin:/sbin TMPDIR="${TMPDIR:-/tmp}" \
    GASH_HTE_E2E=1 "GASH_HTE_E2E_LAYOUTS=homebrew homebrew" GASH_HTE_E2E_BLOCKING= \
    "$stage/gash-e2e" -test.v -test.run TestE2EDistro -test.timeout 20m) || rc=$?
if [ "$rc" != 0 ]; then
    echo "== logs"
    find "$HOME/.local/state/gash/hte/logs" -type f -name '*.log' 2>/dev/null | while IFS= read -r f; do
        echo "-- $f"
        tail -n 20 "$f"
    done
fi

echo "== the system paths after uninstall, compared with before install"
snapshot >"$out/after.txt"
if diff -u "$out/before.txt" "$out/after.txt"; then
    echo "identical"
else
    rc=1
fi
exit "$rc"
