# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
# shellcheck shell=bash
#
# Sourced by every tape, hidden, from the repository root:
#
#   source docs/tapes/env.sh [--headless]
#
# It builds a fresh demo home (sandbox.sh) and makes it HOME, so the
# prompt reads ~/projects/shop and nothing of the recording machine shows.
# Then it loads this checkout's gash.sh, with the gash binary named by
# GASH_BIN (render.sh and the Demos workflow build it from this checkout).
# --headless loads nothing and links this checkout at ~/.gash: the tape
# loads Gash the way an agent does.
if [ -z "${GASH_BIN:-}" ] || [ ! -x "$GASH_BIN" ]; then
    echo "env.sh: set GASH_BIN to a gash binary built from this checkout (docs/tapes/render.sh does)" >&2
    return 1
fi
GASH_DEMO_REPO=$PWD
export GASH_DEMO_REPO
GASH_DEMO_HOME=/tmp/gash-demo
bash "$GASH_DEMO_REPO/docs/tapes/sandbox.sh" "$GASH_DEMO_HOME" || return 1
export HOME=$GASH_DEMO_HOME
export GIT_CONFIG_GLOBAL=$HOME/.gitconfig GIT_CONFIG_NOSYSTEM=1
export LANG=C.UTF-8 LC_ALL=C.UTF-8 TZ=Europe/Rome
export HISTFILE=$HOME/.bash_history GASH_BANNER=0
cd "$HOME/projects/shop" || return 1
export GASH_SH=$GASH_DEMO_REPO/gash.sh
if [ "${1:-}" = --headless ]; then
    # What an agent sees: a plain prompt, Gash at ~/.gash, nothing loaded yet.
    ln -s "$GASH_DEMO_REPO" "$HOME/.gash"
    PS1='$ '
else
    # shellcheck source=/dev/null
    source "$GASH_SH"
fi
clear
