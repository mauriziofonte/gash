#!/usr/bin/env bash
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# run.sh VERSION [GO TEST ARGS...]: the compat Bash matrix of
# design/testing.md 5 and 7 (compat.md 1: the generated Bash must load and
# behave on Bash 4.4, 5.0, 5.1 and 5.3). It puts Bash VERSION first on PATH
# (extract.sh: the official bash:VERSION image's binary on the host's GNU
# userland) and runs the Go suite with it: the testscript black-box suite
# (every script sources the generated gash.sh, the shims and
# bash_completion through `bash`, interactive and headless, and the 2.1.0
# state-parity script compares both loads on that Bash) and the unit tests
# that start bash (compatgen, shellinit, selfmgmt, llmexec, ...).
# GASH_EXPECT_BASH makes the suite check that the bash it starts is VERSION.
#
#   bash testdata/bash/run.sh 4.4
#   bash testdata/bash/run.sh 5.3 ./cmd/gash -run TestScript
set -euo pipefail

ver=${1:?usage: run.sh VERSION [GO TEST ARGS...]}
shift
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
dir="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/gash-bash-$ver"

bash "$root/testdata/bash/extract.sh" "$ver" "$dir"
(($#)) || set -- -p 2 ./...
cd "$root"
PATH="$dir/bin:$PATH" GASH_EXPECT_BASH="$ver" CGO_ENABLED=0 go test -count=1 "$@"
