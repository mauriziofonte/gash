#!/bin/sh
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# Runs in the multi-PHP image (Dockerfile.multiphp) for each version that
# bundle.sh copied into /opt/php/X.Y: the copy loads its own loader and
# libraries (patchelf), and is installed as the Debian packages install
# a php-fpm: /usr/sbin/php-fpmX.Y, /etc/php/X.Y/fpm/{php.ini,php-fpm.conf},
# the www pool and the SysV service phpX.Y-fpm (enabled, not started).
set -eu
for v in "$@"; do
    d=/opt/php/$v
    patchelf --set-interpreter "$d/lib/$(cd "$d/lib" && ls ld-linux-*.so.*)" --force-rpath --set-rpath "$d/lib" "$d/sbin/php-fpm"
    ln -s "$d/sbin/php-fpm" "/usr/sbin/php-fpm$v"
    etc=/etc/php/$v/fpm
    mkdir -p "$etc/pool.d"
    cp "$d/etc/php.ini" "$etc/php.ini"
    cat >"$etc/php-fpm.conf" <<CONF
[global]
pid = /run/php/php$v-fpm.pid
error_log = /var/log/php$v-fpm.log
include = $etc/pool.d/*.conf
CONF
    cat >"$etc/pool.d/www.conf" <<CONF
[www]
user = www-data
group = www-data
listen = /run/php/php$v-fpm.sock
listen.owner = www-data
listen.group = www-data
pm = dynamic
pm.max_children = 5
pm.start_servers = 2
pm.min_spare_servers = 1
pm.max_spare_servers = 3
CONF
    cat >"/etc/init.d/php$v-fpm" <<INIT
#!/bin/sh
### BEGIN INIT INFO
# Provides:          php$v-fpm
# Required-Start:    \$remote_fs \$network
# Required-Stop:     \$remote_fs \$network
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: PHP $v FastCGI Process Manager (official php:$v-fpm build)
### END INIT INFO
pid=/run/php/php$v-fpm.pid
running() { [ -f "\$pid" ] && kill -0 "\$(cat "\$pid" 2>/dev/null)" 2>/dev/null; }
case "\$1" in
    start)
        running && exit 0
        mkdir -p /run/php
        exec /usr/sbin/php-fpm$v --fpm-config $etc/php-fpm.conf -c $etc/php.ini
        ;;
    stop)
        if running; then
            kill -QUIT "\$(cat "\$pid")"
            for _ in \$(seq 50); do
                running || break
                sleep 0.1
            done
        fi
        rm -f "\$pid"
        ;;
    reload) running && kill -USR2 "\$(cat "\$pid")" ;;
    restart)
        "\$0" stop
        "\$0" start
        ;;
    status) running ;;
    *)
        echo "usage: \$0 start|stop|reload|restart|status" >&2
        exit 2
        ;;
esac
INIT
    chmod 0755 "/etc/init.d/php$v-fpm"
    update-rc.d "php$v-fpm" defaults >/dev/null
    "/usr/sbin/php-fpm$v" -v | head -n 1
done
