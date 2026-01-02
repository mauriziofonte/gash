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

  # Mock ls so PHP version detection is deterministic and does not touch /usr/bin.
  cat > "$tmp/bin/ls" <<"EOF"
#!/usr/bin/env bash

if [[ "${1-}" == "-1" && "${2-}" == "/usr/bin/php*" ]]; then
  printf '%s\n' "/usr/bin/php8.4" "/usr/bin/php8.3"
  exit 0
fi

exec /bin/ls "$@"
EOF
  chmod +x "$tmp/bin/ls"

  export PATH="$tmp/bin:$ROOT/tests/mocks/bin:/usr/bin:/bin"

  # Simulate the user having defined composer as an alias.
  alias composer="echo should-not-be-used"

  # Disable glob expansion so /usr/bin/php* is passed literally to our mocked ls.
  set -f

  # Keep LS_COLORS empty so aliases.sh initializes it.
  export LS_COLORS=""

  # shellcheck source=lib/aliases.sh
  source "$ROOT/lib/aliases.sh"

  set +f

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
