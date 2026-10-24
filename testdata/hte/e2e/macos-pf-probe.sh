#!/usr/bin/env bash
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# Temporary diagnostics for the hte-macos job: which pf anchor shape lets
# loopback connections through on the runner, direct and redirected, after
# earlier connections left states. Run by macos.sh before the story; it
# leaves pf as it found it (flushed probe anchor, token released).
set -uo pipefail
py=$(command -v python3)
dir=$(mktemp -d)
echo ok >"$dir/index.html"
(cd "$dir" && exec "$py" -m http.server --bind 127.0.0.1 18080) >/dev/null 2>&1 &
s4=$!
(cd "$dir" && exec "$py" -m http.server --bind ::1 18080) >/dev/null 2>&1 &
s6=$!
sleep 2
code() { curl -s -m 3 -o /dev/null -w '%{http_code}' "$1"; }
inserts() { sudo pfctl -s info 2>/dev/null | awk '/state-insert/ {print $2}'; }
variant() {
    local name=$1 rules=$2 line="" _
    sudo pfctl -F states >/dev/null 2>&1
    printf '%s\n' "$rules" | sudo pfctl -a com.apple/gash-probe -f - 2>&1 | grep -vE 'ALTQ|Use of -f|present in the main|See /etc/pf.conf|^$'
    local before
    before=$(inserts)
    for _ in 1 2 3 4; do
        line+=" r4=$(code http://127.0.0.1:18081/) d4=$(code http://127.0.0.1:18080/) r6=$(code 'http://[::1]:18081/') d6=$(code 'http://[::1]:18080/') |"
    done
    echo "$name:$line state-insert +$(($(inserts) - before))"
}
tok=$(sudo pfctl -E 2>&1 | sed -n 's/^Token : //p')
variant "old rdr pass" "rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 18081 -> 127.0.0.1 port 18080
rdr pass on lo0 inet6 proto tcp from any to ::1 port 18081 -> ::1 port 18080"
variant "new (production shape)" "rdr on lo0 inet proto tcp from any to 127.0.0.1 port 18081 -> 127.0.0.1 port 18080
rdr on lo0 inet6 proto tcp from any to ::1 port 18081 -> ::1 port 18080
pass in quick on lo0 inet proto tcp from any to 127.0.0.1 port { 18080 18443 } keep state
pass in quick on lo0 inet6 proto tcp from any to ::1 port { 18080 18443 } keep state
pass out quick on lo0 proto tcp from any to any port { 18081 18444 18080 18443 } no state
pass out quick on lo0 proto tcp from any port { 18081 18444 18080 18443 } to any no state"
variant "broad pass out" "rdr on lo0 inet proto tcp from any to 127.0.0.1 port 18081 -> 127.0.0.1 port 18080
rdr on lo0 inet6 proto tcp from any to ::1 port 18081 -> ::1 port 18080
pass in quick on lo0 proto tcp from any to any port 18080 keep state
pass out quick on lo0 all no state"
variant "old rdr pass again" "rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 18081 -> 127.0.0.1 port 18080
rdr pass on lo0 inet6 proto tcp from any to ::1 port 18081 -> ::1 port 18080"
sudo pfctl -a com.apple/gash-probe -F all >/dev/null 2>&1
sudo pfctl -X "$tok" 2>&1 | grep -v ALTQ
kill "$s4" "$s6"
wait 2>/dev/null
rm -rf "$dir"
