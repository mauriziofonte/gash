#!/usr/bin/env bash

describe "Aliases Refactoring"

it "loads all alias files via _loader.sh" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
export GASH_DIR="\$HOME/.gash"
source "\$HOME/.gash/lib/aliases/_loader.sh"

# Navigation aliases should be defined
alias ll >/dev/null 2>&1; echo "ll:\$?"
alias la >/dev/null 2>&1; echo "la:\$?"
alias .. >/dev/null 2>&1; echo "dotdot:\$?"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"ll:0"* ]]
    [[ "$out" == *"la:0"* ]]
    [[ "$out" == *"dotdot:0"* ]]
'

it "colors.sh sets LS_COLORS" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
unset LS_COLORS
export GASH_DIR="\$HOME/.gash"
source "\$HOME/.gash/lib/aliases/colors.sh"
[[ -n "\${LS_COLORS-}" ]] && echo "LS_COLORS:set" || echo "LS_COLORS:empty"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"LS_COLORS:set"* ]]
'

it "navigation.sh defines ls and cd aliases" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
export GASH_DIR="\$HOME/.gash"
source "\$HOME/.gash/lib/aliases/navigation.sh"

alias lash >/dev/null 2>&1; echo "lash:\$?"
alias ... >/dev/null 2>&1; echo "threedots:\$?"
alias cls >/dev/null 2>&1; echo "cls:\$?"
alias path >/dev/null 2>&1; echo "path:\$?"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"lash:0"* ]]
    [[ "$out" == *"threedots:0"* ]]
    [[ "$out" == *"cls:0"* ]]
    [[ "$out" == *"path:0"* ]]
'

it "safety.sh defines safe file operation aliases" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
export GASH_DIR="\$HOME/.gash"
source "\$HOME/.gash/lib/aliases/safety.sh"

alias cp >/dev/null 2>&1; echo "cp:\$?"
alias mv >/dev/null 2>&1; echo "mv:\$?"
alias rm >/dev/null 2>&1; echo "rm:\$?"
alias mkdir >/dev/null 2>&1; echo "mkdir:\$?"
alias ports >/dev/null 2>&1; echo "ports:\$?"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"cp:0"* ]]
    [[ "$out" == *"mv:0"* ]]
    [[ "$out" == *"rm:0"* ]]
    [[ "$out" == *"mkdir:0"* ]]
    [[ "$out" == *"ports:0"* ]]
'

it "git.sh defines git aliases and functions when git is available" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Skip if git not available
    command -v git >/dev/null 2>&1 || { echo "SKIP: git not available"; exit 0; }

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
export GASH_DIR="\$HOME/.gash"
source "\$HOME/.gash/lib/aliases/git.sh"

# Check aliases
alias gst >/dev/null 2>&1; echo "gst:\$?"
alias ga >/dev/null 2>&1; echo "ga:\$?"
alias gc >/dev/null 2>&1; echo "gc:\$?"
alias gp >/dev/null 2>&1; echo "gp:\$?"

# Check log functions (gl is now a function, not alias)
declare -f gl >/dev/null 2>&1; echo "gl_func:\$?"
declare -f gla >/dev/null 2>&1; echo "gla_func:\$?"
declare -f glo >/dev/null 2>&1; echo "glo_func:\$?"
declare -f gls >/dev/null 2>&1; echo "gls_func:\$?"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"gst:0"* ]]
    [[ "$out" == *"ga:0"* ]]
    [[ "$out" == *"gc:0"* ]]
    [[ "$out" == *"gp:0"* ]]
    [[ "$out" == *"gl_func:0"* ]]
    [[ "$out" == *"gla_func:0"* ]]
    [[ "$out" == *"glo_func:0"* ]]
    [[ "$out" == *"gls_func:0"* ]]
'

it "gl --help shows usage information" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Skip if git not available
    command -v git >/dev/null 2>&1 || { echo "SKIP: git not available"; exit 0; }

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
export GASH_DIR="\$HOME/.gash"
source "\$HOME/.gash/lib/aliases/git.sh"
gl --help
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"Git Log Aliases"* ]]
    [[ "$out" == *"gla"* ]]
    [[ "$out" == *"glo"* ]]
    [[ "$out" == *"gls"* ]]
    [[ "$out" == *"EXAMPLES"* ]]
'

it "docker.sh defines docker aliases when docker is available" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Skip if docker not available
    type -P docker >/dev/null 2>&1 || { echo "SKIP: docker not available"; exit 0; }

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
export GASH_DIR="\$HOME/.gash"
source "\$HOME/.gash/lib/aliases/docker.sh"

alias dcls >/dev/null 2>&1; echo "dcls:\$?"
alias dils >/dev/null 2>&1; echo "dils:\$?"
alias dstop >/dev/null 2>&1; echo "dstop:\$?"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"dcls:0"* ]]
    [[ "$out" == *"dils:0"* ]]
    [[ "$out" == *"dstop:0"* ]]
'

it "general.sh defines help and quit aliases" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
export GASH_DIR="\$HOME/.gash"

# Create dummy functions that general.sh references
gash_help() { :; }
stop_services() { :; }

source "\$HOME/.gash/lib/aliases/general.sh"

alias help >/dev/null 2>&1; echo "help:\$?"
alias quit >/dev/null 2>&1; echo "quit:\$?"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"help:0"* ]]
    [[ "$out" == *"quit:0"* ]]
'

it "backward compatible with monolithic aliases.sh" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Verify monolithic file still exists as fallback
    [[ -f "$ROOT/lib/aliases.sh" ]]
'

it "gash.sh prefers _loader.sh over aliases.sh" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1

# Both loader systems should result in aliases being defined
alias ll >/dev/null 2>&1; echo "ll:\$?"
alias gst >/dev/null 2>&1 || echo "gst:skipped"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"ll:0"* ]]
'

# =============================================================================
# Alias Conflict & Bug Fix Tests
# =============================================================================

it "gap alias resolves to git add --patch (not git_apply_patch)" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<\EOF
set -u
source "$HOME/.gash/gash.sh" >/dev/null 2>&1
alias gap 2>/dev/null
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"git add --patch"* ]]
'

it "gpatch alias resolves to git_apply_patch" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<\EOF
set -u
source "$HOME/.gash/gash.sh" >/dev/null 2>&1
alias gpatch 2>/dev/null
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"git_apply_patch"* ]]
'

it "quit alias resolves to services_stop --force" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<\EOF
set -u
source "$HOME/.gash/gash.sh" >/dev/null 2>&1
alias quit 2>/dev/null
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"services_stop --force"* ]]
'

it "dcrm alias is defined when docker is available" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    MOCK_BIN="$ROOT/tests/mocks/bin"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<\EOF
set -u
source "$HOME/.gash/gash.sh" >/dev/null 2>&1
alias dcrm 2>/dev/null
EOF
)"

    out="$(HOME="$tmp" PATH="$MOCK_BIN:$PATH" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"docker container rm"* ]]
'
