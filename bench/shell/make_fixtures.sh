#!/usr/bin/env bash
# Author: Maurizio Fonte <fonte.maurizio@gmail.com>
# Builds the fixture tree used by prompt_bench.sh.
# Usage: bash make_fixtures.sh FIXTURE_DIR
#   FIXTURE_DIR/plain/sub      a directory with no .git above it (pick a FIXTURE_DIR outside any repository)
#   FIXTURE_DIR/repo           a git repository with 6 commits, clean tree, and repo/deep/er
#   FIXTURE_DIR/bin            true_static_go (and true_static_c when gcc can link statically)
set -eu
FX=${1:?fixture dir}
mkdir -p "$FX/plain/sub" "$FX/repo/deep/er" "$FX/bin"
cd "$FX/repo"
git init -q .
for i in 1 2 3 4 5 6; do
    if ((i <= 5)); then echo "$i" >"f$i.txt"; else echo "$i" >>f1.txt; fi
    git add -A
    GIT_AUTHOR_DATE="2026-01-0${i}T00:00:00Z" GIT_COMMITTER_DATE="2026-01-0${i}T00:00:00Z" \
        git -c user.name=bench -c user.email=bench@example.invalid commit -q -m "c$i"
done
tmp=$(mktemp -d)
printf 'package main\n\nfunc main() {}\n' >"$tmp/t.go"
printf 'int main(void) { return 0; }\n' >"$tmp/t.c"
if command -v go >/dev/null; then
    (cd "$tmp" && CGO_ENABLED=0 go build -o "$FX/bin/true_static_go" t.go) || true
fi
if command -v gcc >/dev/null; then
    gcc -static -O2 -o "$FX/bin/true_static_c" "$tmp/t.c" 2>/dev/null || true
fi
rm -rf "$tmp"
