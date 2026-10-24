#!/usr/bin/env bash
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# Temporary diagnostics for the hte-macos job: which pf anchor shape lets
# loopback connections through on the runner. Run by macos.sh before the
# story; it leaves pf as it found it (flushed probe anchor, token released).
set -uo pipefail
py=$(command -v python3)
dir=$(mktemp -d)
echo ok >"$dir/index.html"
(cd "$dir" && exec "$py" -m http.server --bind 127.0.0.1 18080) >/dev/null 2>&1 &
srv=$!
sleep 2
try() { printf '%-40s %s\n' "$1" "$(curl -s -m 3 -o /dev/null -w '%{http_code}' "$2"; echo " rc=$?")"; }
counters() { sudo pfctl -s info 2>/dev/null | grep -E 'Status|inserts|state-insert'; }
anchor() {
    echo "-- anchor: $1"
    printf '%s\n' "$2" | sudo pfctl -a com.apple/gash-probe -f - 2>&1 | grep -v ALTQ
    try "  direct 18080" http://127.0.0.1:18080/
    try "  redirected 18081" http://127.0.0.1:18081/
    counters
}
echo "== pf before"
counters
cat /etc/pf.conf
ls -l /etc/pf.anchors
cat /etc/pf.anchors/com.apple
sudo pfctl -sA 2>&1 | grep -v ALTQ
try "pf as found, direct 18080" http://127.0.0.1:18080/
tok=$(sudo pfctl -E 2>&1 | sed -n 's/^Token : //p')
echo "token $tok"
try "pf enabled, direct 18080" http://127.0.0.1:18080/
counters
sudo pfctl -vvs rules 2>&1 | grep -v ALTQ
sudo pfctl -vvs nat 2>&1 | grep -v ALTQ
anchor "rdr pass" "rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 18081 -> 127.0.0.1 port 18080"
sudo pfctl -ss 2>&1 | grep -v ALTQ
anchor "rdr + pass no state" "rdr on lo0 inet proto tcp from any to 127.0.0.1 port 18081 -> 127.0.0.1 port 18080
pass quick on lo0 all no state"
anchor "rdr + pass keep state" "rdr on lo0 inet proto tcp from any to 127.0.0.1 port 18081 -> 127.0.0.1 port 18080
pass in quick on lo0 inet proto tcp from any to 127.0.0.1 port 18080 keep state
pass out quick on lo0 all no state"
anchor "rdr pass + pass out no state" "rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 18081 -> 127.0.0.1 port 18080
pass out quick on lo0 all no state"
echo "-- stock pf.conf reloaded, rdr pass anchor"
sudo pfctl -f /etc/pf.conf 2>&1 | grep -v ALTQ
anchor "rdr pass (after pf.conf)" "rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 18081 -> 127.0.0.1 port 18080"
sudo pfctl -a com.apple/gash-probe -F all 2>&1 | grep -v ALTQ
sudo pfctl -X "$tok" 2>&1 | grep -v ALTQ
counters
kill "$srv"
rm -rf "$dir"
