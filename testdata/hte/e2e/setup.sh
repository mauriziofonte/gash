#!/bin/bash
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# The container side of run.sh, as root: on Debian and Ubuntu the hte-cli
# V2 tree served by the system Apache and PHP-FPM; on Fedora and Arch the
# distro httpd and PHP-FPM packages as installed, never started. Then the
# user dev, the test binary as dev, and /etc compared with its state
# before install.
set -euo pipefail
if [ ! -f /.dockerenv ]; then
    echo "setup.sh runs only inside the E2E container (run.sh)" >&2
    exit 2
fi
out=/tmp/e2e
mkdir -p "$out"

useradd -m -s /bin/bash dev
# pam_unix's account check fails (AUTHINFO_UNAVAIL) for a user without a
# shadow entry, which a container image without /etc/shadow leaves (seen
# on Fedora); sudo then asks for a password even under NOPASSWD.
pwconv
if ! getent shadow dev >/dev/null; then
    echo "the user dev has no shadow entry" >&2
    exit 2
fi
# Fedora ships /etc/shadow as mode 0000, readable only with
# CAP_DAC_OVERRIDE; on the GitHub runners the setuid-root sudo and
# unix_chkpwd in this container lack it (unix_chkpwd chkexpiry returns
# AUTHINFO_UNAVAIL), so pam_unix refuses dev's account. Mode 0400 lets the
# owner root read it without the capability.
if [ "$(stat -c %a /etc/shadow)" = 0 ]; then
    chmod 0400 /etc/shadow
fi
install -m 0755 /e2e/gash-e2e /usr/local/bin/gash-e2e
install -m 0755 /e2e/gash-hosts-helper /usr/local/bin/gash-hosts-helper
# The test binary is also the root step (gash hte __root).
printf 'dev ALL=(root) NOPASSWD: /usr/local/bin/gash-e2e hte __root\n' >/etc/sudoers.d/e2e
chmod 0440 /etc/sudoers.d/e2e
visudo -c >/dev/null
# The root step must be reachable as dev before the stories start; when it
# is not, show what sudo's PAM stack sees instead of failing inside a story.
if ! runuser -u dev -- sudo -n -l /usr/local/bin/gash-e2e hte __root >/dev/null 2>"$out/sudo.err"; then
    echo "== sudo as dev refused" >&2
    cat "$out/sudo.err" >&2
    for f in /etc/pam.d/sudo /etc/pam.d/system-auth /etc/nsswitch.conf; do
        echo "-- $f" >&2
        grep -v '^#' "$f" >&2 || true
    done
    getent passwd dev >&2 || true
    getent shadow dev >&2 || true
    ls -l /etc/shadow /usr/bin/sudo /usr/sbin/unix_chkpwd >&2 || true
    command -v selinuxenabled >/dev/null && { selinuxenabled && echo "selinux enabled" >&2 || echo "selinux disabled" >&2; }
    echo "-- as dev through runuser" >&2
    runuser -u dev -- sh -c 'id; grep -E "^(Cap|NoNewPrivs|Seccomp)" /proc/self/status; /usr/sbin/unix_chkpwd dev chkexpiry </dev/null; echo "unix_chkpwd chkexpiry: $?"' >&2 || true
    echo "-- sudo through su" >&2
    su -s /bin/sh dev -c 'sudo -n -l /usr/local/bin/gash-e2e hte __root' >&2 || true
    rpm -V pam sudo util-linux-core >&2 || true
    exit 1
fi

# Docker bind-mounts /etc/hosts, so the helper cannot rename over it and
# writes in place (LEDGER HTE-69). The test runs on that mount.
echo "== hosts helper on the bind-mounted /etc/hosts"
grep -q ' /etc/hosts ' /proc/self/mountinfo
cp /etc/hosts /tmp/hosts.orig
printf '127.0.0.1 repro.test\n' | /usr/local/bin/gash-hosts-helper apply
grep -qx '127.0.0.1 repro.test' /etc/hosts
grep -q ' /etc/hosts ' /proc/self/mountinfo
if ls /etc/.hosts.gash-hte-* >/dev/null 2>&1; then
    echo "the helper left a temporary file" >&2
    exit 1
fi
cat /tmp/hosts.orig >/etc/hosts
echo "the helper wrote the bind mount in place"

distro=${GASH_HTE_E2E_DISTRO:-ubuntu}
multiphp=${GASH_HTE_E2E_MULTIPHP:-}
# The environment of the test binary, as dev.
testenv=(GASH_HTE_E2E=1)
case "$distro" in
    debian | ubuntu) legacy=1 ;;
    fedora | alma)
        legacy=0
        # The distro php-fpm and Remi's php83; mod_ssl's ssl.conf carries
        # the stock _default_:443 vhost. AlmaLinux (the RHEL family) has
        # Fedora's layout.
        testenv+=("GASH_HTE_E2E_LAYOUTS=fedora remi" "GASH_HTE_E2E_BLOCKING=/etc/httpd/conf.d/ssl.conf")
        ;;
    arch)
        legacy=0
        testenv+=("GASH_HTE_E2E_LAYOUTS=arch arch-legacy" "GASH_HTE_E2E_BLOCKING=")
        ;;
    *)
        echo "unknown distro $distro" >&2
        exit 2
        ;;
esac

# After an Apache or PHP-FPM (re)start the FPM pool can need a moment before
# proxy_fcgi reaches it, so a one-shot request catches a transient 503. Retry
# briefly: the smoke checks test that the site serves, not that it serves on
# the first try.
curl_serves() {
    local out _
    for _ in $(seq 1 20); do
        if out=$(curl -fsS "$@" 2>/dev/null); then
            printf '%s\n' "$out"
            return 0
        fi
        sleep 0.5
    done
    curl -fsS "$@"
}

if [ "$legacy" = 1 ]; then
    # The PHP-FPM of the distro packages (8.3 on Ubuntu 24.04, 8.2 on
    # Debian 12), which the hte-cli tree's 8.3 sites move to.
    php=$(dpkg-query -W -f='${Depends}' php-fpm | sed -n 's/^php\([0-9]*\.[0-9]*\)-fpm.*/\1/p')
    if [ -z "$php" ] || [ ! -d "/etc/php/$php/fpm/pool.d" ]; then
        echo "no distro PHP-FPM version from the php-fpm package" >&2
        exit 2
    fi
    testenv+=("GASH_HTE_E2E_PHP=$php")
    echo "== the distro PHP-FPM is $php"

    # The V2 hte-cli tree. The multi-PHP image has its system PHP 7.4; in
    # the base image https.test moves to the distro PHP.
    cp -a /e2e/V2/etc/apache2/. /etc/apache2/
    for f in /e2e/V2/etc/php/8.3/fpm/pool.d/*.conf; do
        sed "s/php8\.3/php$php/g" "$f" >"/etc/php/$php/fpm/pool.d/${f##*/}"
    done
    sed -i "s/php8\.3/php$php/g" /etc/apache2/sites-available/*.conf
    if [ "$multiphp" = 1 ]; then
        cp /e2e/V2/etc/php/7.4/fpm/pool.d/https.test.conf /etc/php/7.4/fpm/pool.d/
    else
        sed "s/php7\.4/php$php/g" /e2e/V2/etc/php/7.4/fpm/pool.d/https.test.conf >"/etc/php/$php/fpm/pool.d/https.test.conf"
        sed -i "s/php7\.4/php$php/g" /etc/apache2/sites-available/002-https.test.conf
    fi
    grep 'hte-cli' /e2e/V2/etc/hosts >>/etc/hosts
    chown root:root /etc/apache2/certs-selfsigned/*
    chmod 0644 /etc/apache2/certs-selfsigned/*.crt
    chmod 0400 /etc/apache2/certs-selfsigned/*.key
    for s in http https forced; do
        mkdir -p "/home/dev/sites/$s/public"
        # shellcheck disable=SC2016 # PHP source: $_SERVER is PHP's
        printf '<?php echo "legacy:", $_SERVER["HTTP_HOST"], ":", PHP_SAPI, "\\n";\n' >"/home/dev/sites/$s/public/index.php"
        if [ "$multiphp" = 1 ]; then
            printf '<?php echo PHP_VERSION, " ", ini_get("max_input_vars"), "\\n";\n' >"/home/dev/sites/$s/public/info.php"
        fi
    done
    chown -R dev:dev /home/dev/sites
    # As on hte-cli machines, the system Apache runs as the user (the pool
    # sockets are dev:dev 0660).
    sed -i -e 's/^export APACHE_RUN_USER=.*/export APACHE_RUN_USER=dev/' -e 's/^export APACHE_RUN_GROUP=.*/export APACHE_RUN_GROUP=dev/' /etc/apache2/envvars
    a2enmod -q ssl proxy_fcgi rewrite http2 headers >/dev/null
    # restart: under systemd the units already run, on the default tree.
    service "php$php-fpm" restart >/dev/null
    if [ "$multiphp" = 1 ]; then
        service php7.4-fpm restart >/dev/null
    fi
    service apache2 restart >/dev/null
    echo "== the system Apache serves the legacy sites"
    curl_serves --resolve http.test:80:127.0.0.1 http://http.test/
    curl_serves -k --resolve https.test:443:127.0.0.1 https://https.test/
else
    # Nothing of the system httpd or PHP-FPM runs: the packages as a
    # fresh install leaves them.
    if pgrep -x httpd >/dev/null || pgrep -f 'php-fpm: master' >/dev/null; then
        echo "a system httpd or PHP-FPM runs" >&2
        exit 2
    fi
fi

snapshot() {
    local roots=(/etc /usr/local)
    [ -d /var/lib/gash-hte ] && roots+=(/var/lib/gash-hte)
    find "${roots[@]}" -xdev | LC_ALL=C sort | while IFS= read -r p; do
        if [ -L "$p" ]; then
            echo "$p -> $(readlink "$p")"
        elif [ -d "$p" ]; then
            echo "$p/ $(stat -c '%a %U:%G' "$p")"
        else
            echo "$p $(stat -c '%a %U:%G' "$p") $(sha256sum <"$p" | cut -c1-16)"
        fi
    done
}
snapshot >"$out/before.txt"

echo "== the test, as dev"
cd /home/dev
set +e
if [ "${GASH_HTE_E2E_SYSTEMD:-}" = 1 ]; then
    # A login session (pam_systemd): XDG_RUNTIME_DIR and the user manager.
    su -l dev -c "env $(printf '%q ' "${testenv[@]}")GASH_HTE_E2E_SYSTEMD=1 /usr/local/bin/gash-e2e -test.v -test.run TestE2ESystemd -test.timeout 20m"
else
    if [ "$multiphp" = 1 ]; then
        testenv+=(GASH_HTE_E2E_MULTIPHP=1)
        run=TestE2EMultiPHP
    elif [ "$legacy" = 1 ]; then
        run='TestE2E$|TestE2EReinstall'
    else
        run=TestE2EDistro
    fi
    runuser -u dev -- env -i HOME=/home/dev USER=dev LOGNAME=dev \
        PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin "${testenv[@]}" \
        /usr/local/bin/gash-e2e -test.v -test.run "$run" -test.timeout 20m
fi
rc=$?
set -e
if [ "$rc" != 0 ]; then
    echo "== logs"
    for f in /home/dev/.local/state/gash/hte/logs/*.log /home/dev/.local/state/gash/hte/logs/*/*.log /var/log/apache2/error.log /var/log/php*-fpm.log /var/log/httpd/*; do
        [ -f "$f" ] && {
            echo "-- $f"
            tail -n 20 "$f"
        }
    done
fi

echo "== /etc after uninstall, compared with before install"
snapshot >"$out/after.txt"
if diff -u "$out/before.txt" "$out/after.txt"; then
    echo "identical"
else
    rc=1
fi
if [ "$legacy" = 0 ]; then
    : # no system Apache ran before install
elif pgrep -u root -x apache2 >/dev/null; then
    echo "the system Apache runs again"
    curl_serves --resolve http.test:80:127.0.0.1 http://http.test/ || rc=1
else
    echo "the system Apache does not run"
    rc=1
fi
exit "$rc"
