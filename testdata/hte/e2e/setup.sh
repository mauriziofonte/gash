#!/bin/bash
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# The container side of run.sh, as root: the hte-cli V2 tree served by
# the system Apache and PHP-FPM, the user dev, then the test binary as
# dev, then /etc compared with its state before install.
set -euo pipefail
if [ ! -f /.dockerenv ]; then
    echo "setup.sh runs only inside the E2E container (run.sh)" >&2
    exit 2
fi
out=/tmp/e2e
mkdir -p "$out"

useradd -m -s /bin/bash dev
install -m 0755 /e2e/gash-e2e /usr/local/bin/gash-e2e
install -m 0755 /e2e/gash-hosts-helper /usr/local/bin/gash-hosts-helper
# The test binary is also the root step (gash hte __root).
printf 'dev ALL=(root) NOPASSWD: /usr/local/bin/gash-e2e hte __root\n' >/etc/sudoers.d/e2e
chmod 0440 /etc/sudoers.d/e2e
visudo -c >/dev/null

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

# The V2 hte-cli tree. The multi-PHP image has its system PHP 7.4; in
# the base image https.test moves to PHP 8.3.
multiphp=${GASH_HTE_E2E_MULTIPHP:-}
cp -a /e2e/V2/etc/apache2/. /etc/apache2/
cp /e2e/V2/etc/php/8.3/fpm/pool.d/*.conf /etc/php/8.3/fpm/pool.d/
if [ "$multiphp" = 1 ]; then
    cp /e2e/V2/etc/php/7.4/fpm/pool.d/https.test.conf /etc/php/7.4/fpm/pool.d/
else
    sed 's/php7\.4/php8.3/g' /e2e/V2/etc/php/7.4/fpm/pool.d/https.test.conf >/etc/php/8.3/fpm/pool.d/https.test.conf
    sed -i 's/php7\.4/php8.3/g' /etc/apache2/sites-available/002-https.test.conf
fi
grep 'hte-cli' /e2e/V2/etc/hosts >>/etc/hosts
chown root:root /etc/apache2/certs-selfsigned/*
chmod 0644 /etc/apache2/certs-selfsigned/*.crt
chmod 0400 /etc/apache2/certs-selfsigned/*.key
for s in http https forced; do
    mkdir -p "/home/dev/sites/$s/public"
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
service php8.3-fpm restart >/dev/null
if [ "$multiphp" = 1 ]; then
    service php7.4-fpm restart >/dev/null
fi
service apache2 restart >/dev/null
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

echo "== the system Apache serves the legacy sites"
curl_serves --resolve http.test:80:127.0.0.1 http://http.test/
curl_serves -k --resolve https.test:443:127.0.0.1 https://https.test/

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
    su -l dev -c 'env GASH_HTE_E2E=1 GASH_HTE_E2E_SYSTEMD=1 /usr/local/bin/gash-e2e -test.v -test.run TestE2ESystemd -test.timeout 20m'
elif [ "$multiphp" = 1 ]; then
    runuser -u dev -- env -i HOME=/home/dev USER=dev LOGNAME=dev \
        PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin GASH_HTE_E2E=1 GASH_HTE_E2E_MULTIPHP=1 \
        /usr/local/bin/gash-e2e -test.v -test.run TestE2EMultiPHP -test.timeout 20m
else
    runuser -u dev -- env -i HOME=/home/dev USER=dev LOGNAME=dev \
        PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin GASH_HTE_E2E=1 \
        /usr/local/bin/gash-e2e -test.v -test.timeout 20m
fi
rc=$?
set -e
if [ "$rc" != 0 ]; then
    echo "== logs"
    for f in /home/dev/.local/state/gash/hte/logs/*.log /home/dev/.local/state/gash/hte/logs/*/*.log /var/log/apache2/error.log /var/log/php*-fpm.log; do
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
if pgrep -u root -x apache2 >/dev/null; then
    echo "the system Apache runs again"
    curl_serves --resolve http.test:80:127.0.0.1 http://http.test/ || rc=1
else
    echo "the system Apache does not run"
    rc=1
fi
exit "$rc"
