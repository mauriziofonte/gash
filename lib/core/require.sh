#!/usr/bin/env bash

# Gash Core: Requirements
# The message gash.sh shows when the shell cannot run Gash. Plain sh syntax:
# it is sourced before gash.sh knows that it runs in a Bash it supports, so
# Bash 3.2 (macOS /bin/bash) and other shells reach the message instead of a
# parse error. Nothing else may live here.

# Usage: __gash_require_bash_message
__gash_require_bash_message() {
    echo "Gash requires Bash 4.4 or newer (this is ${BASH_VERSION:-not Bash})." >&2
    echo "On macOS: brew install bash, then use $(brew --prefix 2>/dev/null || echo /opt/homebrew)/bin/bash." >&2
}
