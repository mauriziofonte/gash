#!/bin/sh
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# Runs in an official php:X.Y-fpm image (Dockerfile.multiphp): copies its
# php-fpm, every shared library it loads (the C library and its loader
# included, so the copy does not depend on the glibc of the image it lands
# in) and its development php.ini into /opt/php/X.Y.
set -eu
v=$1
dst=/opt/php/$v
mkdir -p "$dst/sbin" "$dst/lib" "$dst/etc"
cp /usr/local/sbin/php-fpm "$dst/sbin/php-fpm"
ldd /usr/local/sbin/php-fpm | awk '$2 == "=>" && $3 ~ /^\// { print $3 } $1 ~ /^\/.*ld-linux/ { print $1 }' | while read -r lib; do
    cp -L "$lib" "$dst/lib/"
done
# NSS modules a glibc before 2.34 loads at run time (getpwuid).
for lib in /lib/x86_64-linux-gnu/libnss_files.so.2 /lib/x86_64-linux-gnu/libnss_compat.so.2; do
    if [ -e "$lib" ]; then
        cp -L "$lib" "$dst/lib/"
    fi
done
cp /usr/local/etc/php/php.ini-development "$dst/etc/php.ini"
"$dst/sbin/php-fpm" -v | head -n 1
