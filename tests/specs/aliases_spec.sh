#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Aliases"

it "defines core navigation and safety aliases" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  export PATH="$ROOT/tests/mocks/bin:$PATH"

  # Keep LS_COLORS empty so aliases.sh initializes it.
  export LS_COLORS=""

  # Ensure the helper does not leak globals.
  unset BINARY 2>/dev/null || true

  # shellcheck source=lib/aliases.sh
  source "$ROOT/lib/aliases.sh"

  a1="$(alias ..)"
  aDots2="$(alias ...)"
  aDots3="$(alias ....)"
  aDots4="$(alias .....)"
  a2="$(alias ll)"
  a3="$(alias rm)"
  a4="$(alias help)"

  [[ "$a1" == *"cd .."* ]]
  [[ "$aDots2" == *"cd ../.."* ]]
  [[ "$aDots3" == *"cd ../../.."* ]]
  [[ "$aDots4" == *"cd ../../../.."* ]]
  [[ "$a2" == *"ls"* ]]
  [[ "$a3" == *"--preserve-root"* ]]
  [[ "$a4" == *"gash_help"* ]]

  # Must remain unset (no pollution).
  [[ -z "${BINARY+x}" ]]
'

it "does not inject alias definitions into composer aliases" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp="$(mktemp -d)"
  trap "/bin/rm -rf \"$tmp\"" EXIT

  mkdir -p "$tmp/bin" "$tmp/home"
  export HOME="$tmp/home"

  # Provide a mock php so the PHP/composer alias block runs.
  cat > "$tmp/bin/php" <<"EOF"
#!/usr/bin/env bash
exit 0
EOF
  chmod +x "$tmp/bin/php"

  # Provide an external composer executable.
  cat > "$tmp/bin/composer" <<"EOF"
#!/usr/bin/env bash
echo composer-mock
EOF
  chmod +x "$tmp/bin/composer"

  # Provide versioned PHP binaries so detection is deterministic and
  # does not depend on what is installed under /usr/bin.
  for v in 8.4 8.3; do
    printf "#!/usr/bin/env bash\nexit 0\n" > "$tmp/bin/php$v"
    chmod +x "$tmp/bin/php$v"
  done

  export PATH="$tmp/bin:$ROOT/tests/mocks/bin:/usr/bin:/bin"

  # Simulate the user having defined composer as an alias.
  alias composer="echo should-not-be-used"

  # Keep LS_COLORS empty so aliases.sh initializes it.
  export LS_COLORS=""

  # shellcheck source=lib/aliases.sh
  source "$ROOT/lib/aliases.sh"

    q="$(printf '\\047')"
    line="$(alias composer)"
    body="${line#alias composer=}"
    body="${body#${q}}"
    body="${body%${q}}"
  [[ "$body" == *"$tmp/bin/composer"* ]]

  # The alias body must never contain an embedded alias definition.
  if echo "$body" | grep -qE "(^|[[:space:]])alias($|[[:space:]])"; then
    exit 1
  fi

    line2="$(alias composer-packages-update)"
    body2="${line2#alias composer-packages-update=}"
    body2="${body2#${q}}"
    body2="${body2%${q}}"
  [[ "$body2" == *"$tmp/bin/composer"* ]]
'
