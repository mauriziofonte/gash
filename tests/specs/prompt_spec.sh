#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Prompt"

it "loads gash.sh in an interactive shell and builds PS1" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp"
  ln -s "$ROOT" "$tmp/.gash"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; __gash_ps1_build 0; printf \"%s\\n\" \"\${PROMPT_COMMAND-}\"; printf \"%s\\n\" \"\${PS1-}\"" 2>/dev/null)"

  [[ "$out" == *"__gash_prompt_command"* ]]
  [[ "$out" == *"\\u"* ]]
  [[ "$out" == *"\\w"* ]]
'

it "defines __gash_in_git_repo function" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1 || true
declare -f __gash_in_git_repo >/dev/null 2>&1 && echo "func:defined" || echo "func:missing"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"func:defined"* ]]
'

it "uses __GASH_PS1_* color variables instead of generic names" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # The prompt uses prefixed color variables
    grep -q "__GASH_PS1_OFF" "$ROOT/lib/shell/prompt.sh"
    grep -q "__GASH_PS1_RED" "$ROOT/lib/shell/prompt.sh"
    grep -q "__GASH_PS1_BBLUE" "$ROOT/lib/shell/prompt.sh"
'

it "__gash_in_git_repo returns success in a git repo" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    # Create a git repo in the temp directory
    mkdir -p "$tmp/project"
    (cd "$tmp/project" && git init -q)

    inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1 || true
cd "\$HOME/project"
__gash_in_git_repo && echo "in_repo:yes" || echo "in_repo:no"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"in_repo:yes"* ]]
'

it "__gash_in_git_repo returns failure outside git repo" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    # Create a non-git directory
    mkdir -p "$tmp/notgit"

    inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1 || true
cd "\$HOME/notgit"
__gash_in_git_repo && echo "in_repo:yes" || echo "in_repo:no"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"in_repo:no"* ]]
'

it "__gash_in_git_repo respects GASH_GIT_EXCLUDE" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    # Create a git repo
    mkdir -p "$tmp/excluded_repo"
    (cd "$tmp/excluded_repo" && git init -q)

    inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1 || true
cd "\$HOME/excluded_repo"

# Without exclusion - should return success
unset GASH_GIT_EXCLUDE
__gash_in_git_repo && echo "without_exclude:in_repo" || echo "without_exclude:not_in_repo"

# With exclusion - should return failure
export GASH_GIT_EXCLUDE="\$HOME/excluded_repo"
__gash_in_git_repo && echo "with_exclude:in_repo" || echo "with_exclude:not_in_repo"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"without_exclude:in_repo"* ]]
    [[ "$out" == *"with_exclude:not_in_repo"* ]]
'

it "uses optimized git commands with GIT_OPTIONAL_LOCKS=0" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # The prompt never takes git index locks
    grep -q "GIT_OPTIONAL_LOCKS=0" "$ROOT/lib/shell/prompt.sh"
'

it "git segment shows branch, dirty marker and ahead count from one git status" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    git init -q --bare "$tmp/remote.git"
    git clone -q "$tmp/remote.git" "$tmp/project" 2>/dev/null
    (
        cd "$tmp/project"
        git checkout -q -b feature
        echo one > f && git add f && git commit -qm one
        git push -q -u origin feature 2>/dev/null
        echo two > f && git commit -qam two
        echo dirty >> f
    )

    inner_cmd="source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1 || true
cd \"\$HOME/project\"
__gash_ps1_build 0
printf \"%s\\n\" \"\$PS1\""

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"(feature"* ]]
    [[ "$out" == *"*"* ]]
    [[ "$out" == *"↑1"* || "$out" == *"+1"* ]]
    # one git process per prompt: no symbolic-ref / rev-parse round trips
    ! grep -q "symbolic-ref\|rev-parse" "$ROOT/lib/shell/prompt.sh"
'

it ".gash_env.template documents GASH_GIT_EXCLUDE" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Check that the template includes GASH_GIT_EXCLUDE documentation
    grep -q "GASH_GIT_EXCLUDE" "$ROOT/.gash_env.template"
    grep -q "Shell Settings" "$ROOT/.gash_env.template"
'

it "__gash_in_git_repo returns failure when git root is HOME" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    # Initialize HOME as a git repo (simulating the problem scenario)
    (cd "$tmp" && git init -q)

    # Create a subdirectory (not a git repo itself)
    mkdir -p "$tmp/projects/myapp"

    inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1 || true

# Test from HOME itself - should skip
cd "\$HOME"
__gash_in_git_repo && echo "home:in_repo" || echo "home:not_in_repo"

# Test from subdirectory - should also skip since git root is HOME
cd "\$HOME/projects/myapp"
__gash_in_git_repo && echo "subdir:in_repo" || echo "subdir:not_in_repo"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    # Both should report not_in_repo because git root is HOME
    [[ "$out" == *"home:not_in_repo"* ]]
    [[ "$out" == *"subdir:not_in_repo"* ]]
'

it "__gash_in_git_repo works correctly with nested repos" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    # Initialize HOME as a git repo
    (cd "$tmp" && git init -q)

    # Create a REAL project with its own git repo inside HOME
    mkdir -p "$tmp/projects/realproject"
    (cd "$tmp/projects/realproject" && git init -q)

    inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1 || true

# Test from realproject - should show git info (has its own .git)
cd "\$HOME/projects/realproject"
__gash_in_git_repo && echo "realproject:in_repo" || echo "realproject:not_in_repo"

# Test from projects dir (no .git, parent is HOME) - should NOT show git info
cd "\$HOME/projects"
__gash_in_git_repo && echo "projects:in_repo" || echo "projects:not_in_repo"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    # realproject has its own .git - should show
    [[ "$out" == *"realproject:in_repo"* ]]
    # projects dir inherits from HOME - should NOT show
    [[ "$out" == *"projects:not_in_repo"* ]]
'

it "the job counter shows running and stopped jobs, not finished ones" bash -c '
  set -euo pipefail
  command -v script >/dev/null 2>&1 && script -qc true /dev/null >/dev/null 2>&1 || exit 77
  cd "$GASH_TEST_ROOT"
  run() { printf "export GASH_BANNER=0; source ./gash.sh\n%s\ntrue\nexit\n" "$1" | script -qc "bash --norc -i" /dev/null 2>&1; }
  # bash < 5.3 keeps a finished job for a program run by a RETURN trap.
  out="$(run "f() { trap \"env true; trap - RETURN\" RETURN; return 0; }; f")"
  [[ "$out" != *"[j1]"* ]]
  out="$(run "sleep 3 &")"
  [[ "$out" == *"[j1]"* ]]
'

it "NO_COLOR / GASH_NO_COLOR drop the prompt colors at the next prompt, same layout" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$ROOT" "$tmp/.gash"
  git init -q "$tmp/project"
  cat >"$tmp/inner.sh" <<"EOF"
source "$HOME/.gash/gash.sh" >/dev/null 2>&1 || true
cd "$HOME/project"
SSH_CONNECTION="1 2 3 4" VIRTUAL_ENV=/x/venv TERM=dumb debian_chroot=
esc="$(printf "\033")"
# One prompt per call, after a failing command; ESC shown as <E>.
show() { false || __gash_prompt_command; printf "%s=%s\n" "$1" "${PS1//$esc/<E>}"; }
show color
NO_COLOR=1 show nocolor
GASH_NO_COLOR=1 show gashnocolor
GASH_NO_COLOR=0 show gashzero
show again
EOF
  out="$(HOME="$tmp" bash --noprofile --norc -i "$tmp/inner.sh" 2>/dev/null)"
  get() { sed -n "s/^$1=//p" <<<"$out"; }
  color="$(get color)"
  [[ "$color" == *"<E>["* && "$color" == *"\\["* ]]
  [[ "$(get gashzero)" == "$color" && "$(get again)" == "$color" ]]
  # The colored prompt with every \[...\] removed is the plain prompt.
  stripped="$(sed "s/\\\\\\[[^]]*\\\\\\]//g" <<<"$color")"
  [[ "$stripped" == "(venv) \\u@\\h:\\w ("*") [!1] \\\$ " ]] || { echo "layout: [$stripped]"; exit 1; }
  for v in nocolor gashnocolor; do
    [[ "$(get "$v")" == "$stripped" ]] || { echo "$v: [$(get "$v")] vs [$stripped]"; exit 1; }
  done
'
