#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

describe "Chrome headless shell (chri)"

# curl answering the npm registry, jsDelivr, Chrome for Testing and HEAD
# requests; uname reporting Linux x86_64; unzip/ldd present.
MOCKS='
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/bin" "$tmp/opt"
  cat > "$tmp/bin/curl" <<EOF
#!/usr/bin/env bash
url="" head=0
for a in "\$@"; do case "\$a" in -fsIL) head=1 ;; http*) url="\$a" ;; esac; done
if ((head)); then printf "HTTP/1.1 200 OK\r\nContent-Length: 104857600\r\n\r\n"; exit 0; fi
case "\$url" in
  *registry.npmjs.org/playwright-core/latest) echo "{\"version\":\"1.56.0\"}" ;;
  *jsdelivr*1.56.0/browsers.json) echo "{\"browsers\":[{\"name\":\"chromium\",\"browserVersion\":\"1.0.0.0\"},{\"name\":\"chromium-headless-shell\",\"revision\":\"1194\",\"browserVersion\":\"141.0.7390.37\"}]}" ;;
  *chrome-for-testing/141.0.7390.37.json) echo "{\"downloads\":{\"chrome-headless-shell\":[{\"platform\":\"linux64\",\"url\":\"https://storage.example/141.0.7390.37/linux64/chrome-headless-shell-linux64.zip\"}]}}" ;;
  *) exit 22 ;;
esac
EOF
  printf "#!/bin/sh\ncase \"\$1\" in -m) echo \${MOCK_ARCH:-x86_64} ;; *) echo Linux ;; esac\n" > "$tmp/bin/uname"
  printf "#!/bin/sh\nexit 0\n" > "$tmp/bin/unzip"
  printf "#!/bin/sh\nexit 0\n" > "$tmp/bin/ldd"
  chmod +x "$tmp/bin"/*
  PATH="$tmp/bin:$PATH"
  export GASH_CHROME_ROOT="$tmp/opt/chs" GASH_CHROME_LINK="$tmp/bin/chrome-headless-shell"
'

it "rejects unknown options" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  out="$(chrome_headless_update --bogus 2>&1)" && exit 1
  [[ "$out" == *"Unknown option: --bogus"* ]]
'

it "refuses on a platform Chrome for Testing does not publish" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  '"$MOCKS"'
  out="$(MOCK_ARCH=aarch64 chrome_headless_update --dry-run 2>&1)" && exit 1
  [[ "$out" == *"Linux x86_64 only"* ]]
'

it "--dry-run resolves the build pinned by the latest Playwright" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  '"$MOCKS"'
  out="$(chrome_headless_update --dry-run 2>&1)"
  [[ "$out" == *"playwright-core 1.56.0 (chromium-headless-shell 141.0.7390.37, revision 1194)"* ]]
  [[ "$out" == *"Installed: none"* && "$out" == *"NEW packages will be installed"* ]]
  [[ "$out" == *"Need to get 100.0 MiB"*"storage.example/141.0.7390.37"* ]]
  [[ ! -e "$GASH_CHROME_ROOT" ]]
'

it "plans an upgrade that keeps the current version and prunes older ones" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  '"$MOCKS"'
  mkdir -p "$GASH_CHROME_ROOT"/{139.0.7258.5,140.0.7339.16}/chrome-headless-shell-linux64
  touch "$GASH_CHROME_ROOT/140.0.7339.16/chrome-headless-shell-linux64/chrome-headless-shell"
  ln -s "$GASH_CHROME_ROOT/140.0.7339.16/chrome-headless-shell-linux64/chrome-headless-shell" "$GASH_CHROME_LINK"
  out="$(chrome_headless_update --dry-run 2>&1)"
  [[ "$out" == *"(140.0.7339.16 => 141.0.7390.37"* ]]
  [[ "$out" == *"Kept for rollback: 140.0.7339.16"* ]]
  [[ "$out" == *"REMOVED:"*"139.0.7258.5"* ]]
'

it "does nothing when the pinned build is installed" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  '"$MOCKS"'
  mkdir -p "$GASH_CHROME_ROOT/141.0.7390.37/chrome-headless-shell-linux64"
  touch "$GASH_CHROME_ROOT/141.0.7390.37/chrome-headless-shell-linux64/chrome-headless-shell"
  ln -s "$GASH_CHROME_ROOT/141.0.7390.37/chrome-headless-shell-linux64/chrome-headless-shell" "$GASH_CHROME_LINK"
  out="$(chrome_headless_update 2>&1 </dev/null)"
  [[ "$out" == *"already the newest version (141.0.7390.37)"* ]]
'

it "fails cleanly when the npm registry is unreachable" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  '"$MOCKS"'
  printf "#!/bin/sh\nexit 6\n" > "$tmp/bin/curl"
  out="$(chrome_headless_update -y 2>&1)" && exit 1
  [[ "$out" == *"Nothing changed"* && ! -e "$GASH_CHROME_ROOT" ]]
'

it "maps missing libraries to packages, without t64 where apt lacks it" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/bin"
  # An Ubuntu 22.04-like apt: only the names without t64 exist.
  printf "#!/bin/sh\ncase \"\$2\" in *t64) exit 100 ;; *) exit 0 ;; esac\n" > "$tmp/bin/apt-cache"
  chmod +x "$tmp/bin/apt-cache"
  PATH="$tmp/bin:$PATH"
  [[ "$(__gash_chs_lib_pkg libnss3.so)" == "libnss3" ]]
  [[ "$(__gash_chs_pkg_name "$(__gash_chs_lib_pkg libasound.so.2)")" == "libasound2" ]]
  [[ "$(__gash_chs_pkg_name libgbm1)" == "libgbm1" ]]
'
