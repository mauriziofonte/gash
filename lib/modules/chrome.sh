#!/usr/bin/env bash

# Gash Module: Chrome for Testing (headless shell)
# One machine-wide chrome-headless-shell, pinned to the build that the latest
# Playwright release expects, for Playwright and Puppeteer on servers and WSL.
#
# Dependencies: core/output.sh, core/utils.sh, core/validation.sh
#
# Public functions (LONG name + SHORT alias):
#   chrome_headless_update (chri)  - Install or upgrade chrome-headless-shell

# Package that provides a missing shared library (soname), as named on
# Ubuntu 24.04 / Debian 13 ("t64" packages). __gash_chs_pkg_name maps it to
# what this system's apt knows.
__gash_chs_lib_pkg() {
    case "${1-}" in
        libnss3* | libnssutil3* | libsmime3*) echo libnss3 ;;
        libnspr4* | libplc4* | libplds4*) echo libnspr4 ;;
        libatk-bridge-2.0*) echo libatk-bridge2.0-0t64 ;;
        libatk-1.0*) echo libatk1.0-0t64 ;;
        libatspi*) echo libatspi2.0-0t64 ;;
        libcups*) echo libcups2t64 ;;
        libdrm*) echo libdrm2 ;;
        libgbm*) echo libgbm1 ;;
        libxkbcommon*) echo libxkbcommon0 ;;
        libXcomposite*) echo libxcomposite1 ;;
        libXdamage*) echo libxdamage1 ;;
        libXfixes*) echo libxfixes3 ;;
        libXrandr*) echo libxrandr2 ;;
        libXext*) echo libxext6 ;;
        libX11*) echo libx11-6 ;;
        libxcb*) echo libxcb1 ;;
        libasound*) echo libasound2t64 ;;
        libpango*) echo libpango-1.0-0 ;;
        libcairo*) echo libcairo2 ;;
        libdbus*) echo libdbus-1-3 ;;
        libexpat*) echo libexpat1 ;;
        libglib* | libgobject* | libgio*) echo libglib2.0-0t64 ;;
        *) echo "?(${1-})" ;;
    esac
}

# Older releases (Ubuntu 22.04, Debian 12) ship the same libraries without
# the t64 suffix: use whichever name apt knows.
__gash_chs_pkg_name() {
    local pkg="${1-}"
    if [[ "$pkg" == *t64 ]] && command -v apt-cache >/dev/null 2>&1 &&
        ! apt-cache show "$pkg" >/dev/null 2>&1 && apt-cache show "${pkg%t64}" >/dev/null 2>&1; then
        pkg="${pkg%t64}"
    fi
    printf '%s\n' "$pkg"
}

# Delete one directory tree file by file (no recursive rm), as root with
# "sudo" as the second argument.
__gash_chs_delete_tree() {
    local dir="${1-}" use_sudo="${2-}"
    [[ -n "$dir" && "$dir" != "/" && -d "$dir" ]] || return 0
    if [[ "$use_sudo" == "sudo" ]]; then
        sudo find "$dir" -depth -delete
    else
        find "$dir" -depth -delete
    fi
}

# Version of the latest playwright-core on npm, into the caller's pw.
__gash_chs_latest_playwright() {
    pw="$(curl -fsSL https://registry.npmjs.org/playwright-core/latest 2>/dev/null | jq -r '.version // empty' 2>/dev/null)"
    [[ -n "$pw" ]]
}

# Read the chromium-headless-shell entry of playwright-core's browsers.json
# into the variables named target and rev of the caller.
__gash_chs_pinned_build() {
    local pw="$1" cdn browsers
    for cdn in "https://cdn.jsdelivr.net/npm/playwright-core@$pw/browsers.json" \
        "https://unpkg.com/playwright-core@$pw/browsers.json"; do
        browsers="$(curl -fsSL "$cdn" 2>/dev/null)" || continue
        target="$(jq -r '.browsers[] | select(.name == "chromium-headless-shell") | .browserVersion // empty' <<<"$browsers" 2>/dev/null)"
        rev="$(jq -r '.browsers[] | select(.name == "chromium-headless-shell") | .revision // empty' <<<"$browsers" 2>/dev/null)"
        [[ "$target" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] && return 0
    done
    return 1
}

# Download URL of an exact Chrome for Testing version (never "latest
# stable"): the per-version endpoint, then the full index, then Playwright's
# mirror. Sets url and size in the caller.
__gash_chs_locate() {
    local version="$1" sel='.downloads["chrome-headless-shell"][]? | select(.platform == "linux64") | .url'
    url="$(curl -fsSL "https://googlechromelabs.github.io/chrome-for-testing/$version.json" 2>/dev/null | jq -r "$sel" 2>/dev/null)"
    if [[ -z "$url" ]]; then
        url="$(curl -fsSL --compressed https://googlechromelabs.github.io/chrome-for-testing/known-good-versions-with-downloads.json 2>/dev/null |
            jq -r --arg v "$version" ".versions[] | select(.version == \$v) | $sel" 2>/dev/null)"
    fi
    [[ -n "$url" ]] || url="https://cdn.playwright.dev/builds/cft/$version/linux64/chrome-headless-shell-linux64.zip"
    size="$(__gash_remote_size "$url")"
    ((size > 0))
}

# Install or upgrade the machine-wide chrome-headless-shell to the build of
# the latest Playwright release, apt-style: plan, one question, then the
# download, checks and install. The outgoing version is kept for rollback;
# older ones are removed.
# Usage: chrome_headless_update [-y] [--dry-run]
# Alias: chri
chrome_headless_update() {
    needs_help "chrome_headless_update" "chrome_headless_update [-y] [--dry-run]" \
        "Install or upgrade chrome-headless-shell to the build the latest Playwright uses. Alias: chri" \
        "${1-}" && return

    local assume_yes=0 dry_run=0 arg
    for arg in "$@"; do
        case "$arg" in
            -y | --yes) assume_yes=1 ;;
            --dry-run) dry_run=1 ;;
            *)
                __gash_error "Unknown option: $arg (usage: chrome_headless_update [-y] [--dry-run])"
                return 1
                ;;
        esac
    done

    if [[ "$(uname -s)" != "Linux" || "$(uname -m)" != "x86_64" ]]; then
        __gash_error "Chrome for Testing publishes chrome-headless-shell for Linux x86_64 only (this is $(uname -s) $(uname -m))."
        return 1
    fi
    local tool
    for tool in curl jq unzip ldd; do
        __gash_require_command "$tool" "chrome_headless_update needs '$tool'." || return 1
    done

    local root="${GASH_CHROME_ROOT:-/opt/chrome-headless-shell}"
    local link="${GASH_CHROME_LINK:-/usr/local/bin/chrome-headless-shell}"
    local vre='^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$'

    # Installed state: the symlink names the current version; the
    # directories under $root are the versions kept.
    local current="" d
    local -a installed=()
    if [[ -L "$link" ]]; then
        current="$(readlink -f "$link")"
        current="${current#"$root"/}"
        current="${current%%/*}"
        [[ "$current" =~ $vre ]] || current=""
    fi
    for d in "$root"/*; do
        [[ -d "$d" && "${d##*/}" =~ $vre ]] && installed+=("${d##*/}")
    done

    local pw="" target="" rev=""
    __gash_task "Resolving the latest Playwright release" __gash_chs_latest_playwright || {
        __gash_error "Unable to read playwright-core from the npm registry (network?). Nothing changed."
        return 1
    }
    __gash_task "Reading the pinned browser build" __gash_chs_pinned_build "$pw" || {
        __gash_error "No chromium-headless-shell entry in browsers.json of playwright-core $pw. Nothing changed."
        return 1
    }

    printf 'Latest Playwright: playwright-core %s (chromium-headless-shell %s, revision %s)\n' "$pw" "$target" "${rev:-?}"
    printf 'Installed: %s\n' "${current:-none}"

    if [[ "$current" == "$target" ]]; then
        printf 'chrome-headless-shell is already the newest version (%s).\n' "$target"
        return 0
    fi

    local url="" size=0
    __gash_task "Locating the Chrome for Testing download" __gash_chs_locate "$target" || {
        __gash_error "No download found for chrome-headless-shell $target (linux64). Nothing changed."
        return 1
    }

    # Rollback policy: keep the outgoing version, remove every other old one.
    local -a prune=()
    local ver
    for ver in "${installed[@]}"; do
        [[ "$ver" == "$target" || "$ver" == "$current" ]] || prune+=("$ver")
    done

    if [[ -z "$current" ]]; then
        printf 'The following NEW packages will be installed:\n  chrome-headless-shell %s (from playwright-core %s)\n' "$target" "$pw"
    else
        printf 'The following packages will be upgraded:\n  chrome-headless-shell (%s => %s, from playwright-core %s)\n' "$current" "$target" "$pw"
        printf 'Kept for rollback: %s\n' "$current"
    fi
    if ((${#prune[@]})); then
        printf 'The following versions will be REMOVED:\n  %s\n' "${prune[*]}"
    fi
    printf 'Need to get %s of archives from %s\n' "$(__gash_human_size "$size")" "$url"

    ((dry_run)) && return 0
    __gash_confirm_plan "$assume_yes" || return 1

    local tmp
    tmp="$(mktemp -d "${TMPDIR:-/tmp}/chrome-headless-shell.XXXXXX")" || return 1
    local zip="$tmp/chrome-headless-shell-linux64.zip"
    local bin_rel=chrome-headless-shell-linux64/chrome-headless-shell

    if ! __gash_download "$url" "$zip"; then
        __gash_chs_delete_tree "$tmp"
        return 1
    fi
    if ! unzip -Z1 "$zip" 2>/dev/null | grep -qx "$bin_rel"; then
        __gash_error "The archive does not contain $bin_rel."
        __gash_chs_delete_tree "$tmp"
        return 1
    fi
    __gash_task "Unpacking chrome-headless-shell $target" unzip -q "$zip" -d "$tmp/x" || {
        __gash_chs_delete_tree "$tmp"
        return 1
    }

    # Missing shared libraries: name the packages instead of failing later
    # with a loader error inside Playwright.
    local missing
    missing="$(ldd "$tmp/x/$bin_rel" 2>/dev/null | awk '/not found/ { print $1 }' | sort -u)"
    if [[ -n "$missing" ]]; then
        local lib pkgs
        pkgs="$(while read -r lib; do __gash_chs_pkg_name "$(__gash_chs_lib_pkg "$lib")"; done <<<"$missing" | sort -u | tr '\n' ' ')"
        __gash_error "Missing shared libraries: $(tr '\n' ' ' <<<"$missing")"
        __gash_info "Install them, then run chrome_headless_update again:  sudo apt-get install ${pkgs% }"
        __gash_chs_delete_tree "$tmp"
        return 1
    fi

    local got
    got="$("$tmp/x/$bin_rel" --version 2>/dev/null)"
    if [[ "$got" != *"$target"* ]]; then
        __gash_error "Version check failed: expected $target, the binary says '${got:-nothing}'."
        __gash_chs_delete_tree "$tmp"
        return 1
    fi
    __gash_info "Verified: $got"

    local rc=0
    __gash_info "Installing into $root/$target (sudo)..."
    if [[ ! -x "$root/$target/$bin_rel" ]]; then
        sudo mkdir -p "$root/$target" &&
            sudo cp -R --no-preserve=ownership "$tmp/x/chrome-headless-shell-linux64" "$root/$target/" ||
            rc=1
    fi
    ((rc == 0)) && { sudo ln -sfn "$root/$target/$bin_rel" "$link" || rc=1; }
    __gash_chs_delete_tree "$tmp"
    if ((rc != 0)); then
        __gash_error "Installation into $root failed."
        return 1
    fi

    for ver in "${prune[@]}"; do
        __gash_info "Removing chrome-headless-shell $ver..."
        __gash_chs_delete_tree "$root/$ver" sudo || rc=1
    done

    __gash_success "Installed: $("$link" --version 2>/dev/null) -> $(readlink -f "$link")"
    return "$rc"
}

# -----------------------------------------------------------------------------
# Short Aliases
# -----------------------------------------------------------------------------
alias chri='chrome_headless_update'

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

    __gash_register_help "chrome_headless_update" \
        --aliases "chri" \
        --module "chrome" \
        --short "Install or upgrade chrome-headless-shell (Chrome for Testing)" \
        --see-also "node_upgrade" \
        <<'HELP'
USAGE
  chrome_headless_update [-y] [--dry-run]

EXAMPLES
  # Show what would change, touch nothing
  chri --dry-run

  # After upgrading Playwright: bring the shared browser to the same build
  chri

  # Unattended (CI, provisioning scripts)
  chri -y

NOTES
  The version is the chromium-headless-shell build listed in browsers.json
  of the latest playwright-core release, downloaded from Chrome for Testing
  (exact version, never "latest stable"). Linux x86_64 only.
  Layout: /opt/chrome-headless-shell/<version>/ with the symlink
  /usr/local/bin/chrome-headless-shell (GASH_CHROME_ROOT and
  GASH_CHROME_LINK change them). The outgoing version is kept for
  rollback, older ones are removed. Installing needs sudo.
  Missing shared libraries are reported with the apt packages to install.
  Point your tools at the shared binary and stop npm from downloading
  browsers of its own:
    export PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 PUPPETEER_SKIP_DOWNLOAD=1
    Playwright: launchOptions: { executablePath: '/usr/local/bin/chrome-headless-shell' }
    Puppeteer:  { headless: 'shell', executablePath: '/usr/local/bin/chrome-headless-shell' }
  Without a terminal the question is refused: pass -y.
HELP

fi # end help registration guard
