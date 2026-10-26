#!/usr/bin/env bash

{ # this ensures the entire script is downloaded #

  # Gash installer: clones https://github.com/mauriziofonte/gash into
  # ~/.gash (or $GASH_DIR), checks out the latest release tag, adds one
  # delimited block to your Bash profile that sources ~/.gashrc, and fetches
  # and verifies (SHA-256) the release binary for your platform.
  #
  # Usage: install.sh [--assume-yes] [--quiet] [--check] [--repair] [--help]
  #
  #   --assume-yes  install without asking; when a tool Gash needs is
  #                 missing, install it with the package manager too (only
  #                 those: the tools single functions use are left to
  #                 gash_doctor --install)
  #   --check       check what Gash needs (Bash 4.4+, git, curl or wget, tar,
  #                 gzip, sha256sum or shasum, grep, sed), print the command
  #                 that installs what is missing, and change nothing (status
  #                 1 when something is missing)
  #   --repair      fetch and verify the gash binary for the VERSION the
  #                 install has checked out, and nothing else: no git command,
  #                 no profile or ~/.gashrc edit (non-interactive; status 1
  #                 when it fails)
  #
  # Environment:
  #   GASH_DIR                install location (default: ~/.gash)
  #   PROFILE                 profile file to edit (default: detected;
  #                           /dev/null to edit none)
  #   GASH_INSTALL_GIT_REPO   repository to clone (default: GitHub)
  #   GASH_INSTALL_REF        branch, tag or commit to install instead of the
  #                           latest release tag
  #   GASH_RELEASE_URL        base URL of the release assets (default: the
  #                           GitHub release of the installed tag)
  #   GASH_REQUIRE_SIGNED=1   abort unless the release tag has a signature
  #                           that `git verify-tag` accepts, or its release a
  #                           Sigstore signature that cosign verifies for the
  #                           tag's commit, and accept only a binary pinned in
  #                           release/SHA256SUMS
  #
  # MIT License · https://github.com/mauriziofonte/gash

  gashinst_has() {
    type "$1" >/dev/null 2>&1
  }

  gashinst_echo() {
    if [ "${QUIET-0}" -eq 1 ]; then
      return 0
    fi
    command printf '%b\n' "${1-}"
  }

  gashinst_error() {
    command printf '%b\n' "${RED}Error:${NC} $1" >&2
  }

  gashinst_info() {
    gashinst_echo "${CYAN}Info:${NC} $1"
  }

  gashinst_success() {
    gashinst_echo "${GREEN}OK:${NC} $1"
  }

  gashinst_warning() {
    gashinst_echo "${YELLOW}Warning:${NC} $1"
  }

  # Returns 0 for yes, 1 for no, 2 when there is no terminal to ask on.
  gashinst_confirm() {
    if [ "${ASSUME_YES-0}" -eq 1 ]; then
      return 0
    fi
    command printf '%b' " $1 ${BOLD}(y/N):${NC} "
    if ! { read -r REPLY </dev/tty; } 2>/dev/null; then
      command printf '\n'
      return 2
    fi
    if [[ "$REPLY" =~ ^[Yy]$ ]]; then
      return 0
    fi
    return 1
  }

  gashinst_default_install_dir() {
    printf %s "${HOME}/.gash"
  }

  gashinst_install_dir() {
    if [ -n "${GASH_DIR-}" ]; then
      printf %s "${GASH_DIR}"
    else
      gashinst_default_install_dir
    fi
  }

  gashinst_try_profile() {
    if [ -z "${1-}" ] || [ ! -f "${1}" ]; then
      return 1
    fi
    command printf '%s\n' "${1}"
  }

  gashinst_detect_profile() {
    if [ "${PROFILE-}" = '/dev/null' ]; then
      return
    fi

    if [ -n "${PROFILE-}" ] && [ -f "${PROFILE}" ]; then
      command printf '%s\n' "${PROFILE}"
      return
    fi

    local DETECTED_PROFILE=''
    if [ -f "$HOME/.bashrc" ]; then
      DETECTED_PROFILE="$HOME/.bashrc"
    elif [ -f "$HOME/.bash_profile" ]; then
      DETECTED_PROFILE="$HOME/.bash_profile"
    fi

    if [ -z "$DETECTED_PROFILE" ]; then
      local EACH_PROFILE
      for EACH_PROFILE in ".profile" ".bashrc" ".bash_profile"; do
        if DETECTED_PROFILE="$(gashinst_try_profile "${HOME}/${EACH_PROFILE}")"; then
          break
        fi
      done
    fi

    if [ -n "$DETECTED_PROFILE" ]; then
      command printf '%s\n' "$DETECTED_PROFILE"
    fi
  }

  # Gash needs Bash 4.4+. The installer itself may run under an older bash
  # (macOS /bin/bash is 3.2), so look for a newer one before giving up.
  gashinst_check_bash() {
    if [ "${BASH_VERSINFO[0]}" -gt 4 ] || { [ "${BASH_VERSINFO[0]}" -eq 4 ] && [ "${BASH_VERSINFO[1]}" -ge 4 ]; }; then
      return 0
    fi
    local candidate version
    for candidate in $(type -aP bash 2>/dev/null) /opt/homebrew/bin/bash /usr/local/bin/bash; do
      [ -x "$candidate" ] || continue
      version="$("$candidate" -c 'echo "${BASH_VERSINFO[0]}.${BASH_VERSINFO[1]}"' 2>/dev/null)" || continue
      case "$version" in
        [5-9].* | 4.[4-9]*)
          gashinst_warning "This bash is ${BASH_VERSION}; Gash will need ${candidate} (${version}) as your login shell."
          gashinst_warning "Set it with: chsh -s ${candidate}"
          return 0
          ;;
      esac
    done
    gashinst_error "Gash requires Bash 4.4 or newer (found ${BASH_VERSION})."
    gashinst_error "On macOS: brew install bash, then chsh -s \"\$(brew --prefix)/bin/bash\"."
    exit 1
  }

  # >>> generated by internal/prereq (go test ./internal/prereq -update) >>>
  # The package manager of this system (apt, dnf, microdnf, yum, pacman,
  # zypper, apk, brew), empty when none is known. Generated from
  # internal/prereq: do not edit.
  gashinst_pkg_manager() {
    local f key value id="" like="" v m=""
    if [[ "${OSTYPE-}" == darwin* ]]; then
      printf '%s\n' brew
      return 0
    fi
    for f in "${GASH__SYSROOT-}/etc/os-release" "${GASH__SYSROOT-}/usr/lib/os-release"; do
      [[ -r "$f" ]] || continue
      while IFS='=' read -r key value; do
        value="${value#\"}"
        value="${value%\"}"
        value="${value#\'}"
        value="${value%\'}"
        case "$key" in
          ID) id="$value" ;;
          ID_LIKE) like="$value" ;;
        esac
      done <"$f"
      break
    done
    for v in $id $like; do
      case "$v" in
        debian | ubuntu | linuxmint | pop | raspbian | kali) m=apt ;;
        fedora | rhel | centos | rocky | almalinux | ol | amzn) m=dnf ;;
        arch | archarm | manjaro | endeavouros) m=pacman ;;
        opensuse | opensuse-leap | opensuse-tumbleweed | suse | sles) m=zypper ;;
        alpine) m=apk ;;
      esac
      [[ -n "$m" ]] && break
    done
    if [[ "$m" == dnf ]] && ! type -P dnf >/dev/null 2>&1; then
      if type -P microdnf >/dev/null 2>&1; then
        m=microdnf
      elif type -P yum >/dev/null 2>&1; then
        m=yum
      fi
    fi
    printf '%s\n' "$m"
  }

  # Usage: gashinst_install_hint [--run] TOOL...
  # The command that installs TOOL... on this system (each package once),
  # nothing when no package is known for any of them; --run runs it instead,
  # after fetching the package lists where the manager needs it. Generated
  # from internal/prereq: do not edit.
  gashinst_install_hint() {
    local m names t pkg pkgs="" run=0 IFS=$' \t\n'
    local -a cmd update
    if [[ "${1-}" == --run ]]; then
      run=1
      shift
    fi
    m="$(gashinst_pkg_manager)"
    names="$m"
    [[ "$m" == microdnf || "$m" == yum ]] && names=dnf
    for t in "$@"; do
      pkg=""
      case "$names:$t" in
        apk:bash) pkg='bash' ;;
        apk:curl) pkg='curl' ;;
        apk:git) pkg='git' ;;
        apk:grep) pkg='grep' ;;
        apk:gzip) pkg='gzip' ;;
        apk:sed) pkg='sed' ;;
        apk:sha256sum) pkg='coreutils' ;;
        apk:tar) pkg='tar' ;;
        apt:bash) pkg='bash' ;;
        apt:curl) pkg='curl' ;;
        apt:git) pkg='git' ;;
        apt:grep) pkg='grep' ;;
        apt:gzip) pkg='gzip' ;;
        apt:sed) pkg='sed' ;;
        apt:sha256sum) pkg='coreutils' ;;
        apt:tar) pkg='tar' ;;
        brew:bash) pkg='bash' ;;
        brew:git) pkg='git' ;;
        dnf:bash) pkg='bash' ;;
        dnf:curl) pkg='curl' ;;
        dnf:git) pkg='git' ;;
        dnf:grep) pkg='grep' ;;
        dnf:gzip) pkg='gzip' ;;
        dnf:sed) pkg='sed' ;;
        dnf:sha256sum) pkg='coreutils' ;;
        dnf:tar) pkg='tar' ;;
        pacman:bash) pkg='bash' ;;
        pacman:curl) pkg='curl' ;;
        pacman:git) pkg='git' ;;
        pacman:grep) pkg='grep' ;;
        pacman:gzip) pkg='gzip' ;;
        pacman:sed) pkg='sed' ;;
        pacman:sha256sum) pkg='coreutils' ;;
        pacman:tar) pkg='tar' ;;
        zypper:bash) pkg='bash' ;;
        zypper:curl) pkg='curl' ;;
        zypper:git) pkg='git' ;;
        zypper:grep) pkg='grep' ;;
        zypper:gzip) pkg='gzip' ;;
        zypper:sed) pkg='sed' ;;
        zypper:sha256sum) pkg='coreutils' ;;
        zypper:tar) pkg='tar' ;;
      esac
      [[ -z "$pkg" || " $pkgs " == *" $pkg "* ]] || pkgs="$pkgs $pkg"
    done
    [[ -n "$pkgs" ]] || return 0
    case "$m" in
      apt) cmd=(apt-get install -y) ;;
      dnf | microdnf | yum) cmd=("$m" install -y) ;;
      pacman) cmd=(pacman -S --needed --noconfirm) ;;
      zypper) cmd=(zypper --non-interactive install) ;;
      apk) cmd=(apk add) ;;
      brew)
        type -P brew >/dev/null 2>&1 || return 0
        cmd=(brew install)
        ;;
      *) return 0 ;;
    esac
    # The package lists: apt and apk always, pacman when it has none.
    case "$m" in
      apt) update=(apt-get update) ;;
      apk) update=(apk update) ;;
      pacman) [[ -n "$(ls -A "${GASH__SYSROOT-}/var/lib/pacman/sync" 2>/dev/null)" ]] || update=(pacman -Sy) ;;
    esac
    if [[ "$m" != brew && "${EUID-}" != 0 ]] && type -P sudo >/dev/null 2>&1; then
      cmd=(sudo "${cmd[@]}")
      [[ ${#update[@]} -eq 0 ]] || update=(sudo "${update[@]}")
    fi
    # shellcheck disable=SC2206 # one word per package
    cmd+=($pkgs)
    if [[ "$run" == 0 ]]; then
      printf '%s\n' "${cmd[*]}"
      return 0
    fi
    if [[ ${#update[@]} -gt 0 ]]; then
      "${update[@]}" || return
    fi
    "${cmd[@]}"
  }
  # <<< generated by internal/prereq <<<

  # Whether the program NAME is a macOS stub that only offers to install
  # the Xcode Command Line Tools (/usr/bin/git on a fresh Mac).
  gashinst_xcode_stub() {
    [[ "${OSTYPE-}" == darwin* ]] || return 1
    [ "$(type -P "$1" 2>/dev/null)" = "/usr/bin/$1" ] || return 1
    ! xcode-select -p >/dev/null 2>&1
  }

  # The tools Gash needs that this system lacks, separated by spaces: git,
  # curl (or wget), tar, gzip, sha256sum (or shasum), grep and sed. Bash is
  # gashinst_check_bash's.
  gashinst_missing_tools() {
    local tool missing=""
    for tool in git curl tar gzip sha256sum grep sed; do
      case "$tool" in
        curl) type -P wget >/dev/null 2>&1 && continue ;;
        # The macOS tar reads gzip itself.
        gzip) [[ "${OSTYPE-}" == darwin* ]] && continue ;;
        sha256sum) type -P shasum >/dev/null 2>&1 && continue ;;
      esac
      if type -P "$tool" >/dev/null 2>&1 && ! gashinst_xcode_stub "$tool"; then
        continue
      fi
      missing="$missing $tool"
    done
    command printf '%s' "${missing# }"
  }

  # Before anything changes: the tools Gash needs. The missing ones are
  # named with the command that installs them on this system, and installed
  # after a yes (--assume-yes: without asking); --check stops at the report.
  # Status 0 when every tool is there.
  gashinst_preflight() {
    local missing cmd rc
    missing="$(gashinst_missing_tools)"
    if [ -z "$missing" ]; then
      gashinst_success "Gash finds everything it needs (Bash ${BASH_VERSINFO[0]}.${BASH_VERSINFO[1]}, git, curl or wget, tar, gzip, sha256sum or shasum, grep, sed)."
      return 0
    fi
    gashinst_error "Gash needs these tools, which are not installed: ${missing// /, }."
    # shellcheck disable=SC2086 # one word per tool
    cmd="$(gashinst_install_hint $missing)"
    if [ -z "$cmd" ]; then
      if [[ "${OSTYPE-}" == darwin* ]]; then
        gashinst_error "Install the Xcode Command Line Tools (xcode-select --install) or Homebrew (https://brew.sh), then run the installer again."
      else
        gashinst_error "Install them with your package manager, then run the installer again."
      fi
      return 1
    fi
    command printf '%b\n' "To install them: ${BOLD}${cmd}${NC}" >&2
    if [ "${CHECK-0}" -eq 1 ]; then
      return 1
    fi
    if [ "${EUID-}" != 0 ] && [[ "$cmd" != sudo\ * && "$cmd" != brew\ * ]]; then
      gashinst_error "Installing packages needs root, and sudo is not installed: run that command as root, then the installer again."
      return 1
    fi
    rc=0
    gashinst_confirm "Install them now?" || rc=$?
    if [ "$rc" -eq 2 ]; then
      gashinst_error "No terminal to ask on: run that command, or the installer with --assume-yes."
      return 1
    elif [ "$rc" -ne 0 ]; then
      gashinst_info "Install them, then run the installer again."
      return 1
    fi
    # shellcheck disable=SC2086 # one word per tool
    if ! gashinst_install_hint --run $missing; then
      gashinst_error "The install command failed: $cmd"
      return 1
    fi
    hash -r
    missing="$(gashinst_missing_tools)"
    if [ -n "$missing" ]; then
      gashinst_error "Still not installed after the install command: ${missing// /, }."
      return 1
    fi
    gashinst_success "Installed what Gash needs."
  }

  # After the install: the tools single functions use (rsync, 7z, docker,
  # ...), from the binary's own check (gash_doctor --install), which asks
  # before it installs anything. Not with --assume-yes, --quiet, an agent
  # (GASH_HEADLESS=1) or no terminal: then only a hint.
  gashinst_feature_tools() {
    declare -F __gash_bin_resolve >/dev/null 2>&1 || return 0
    __gash_bin_resolve 2>/dev/null || return 0
    if [ "${QUIET-0}" -eq 1 ]; then
      return 0
    fi
    if [ "${ASSUME_YES-0}" -eq 1 ] || [ "${GASH_HEADLESS-}" = 1 ] || ! { : </dev/tty; } 2>/dev/null; then
      gashinst_info "gash_doctor --prereqs lists the tools single functions use; gash_doctor --install installs the missing ones."
      return 0
    fi
    gashinst_echo
    gashinst_info "The tools single Gash functions use (gash_doctor --install):"
    "$__GASH_BIN" gash_doctor --install </dev/tty || true
  }

  # Highest release tag: vX.Y.Z or X.Y (no pre-release suffix), compared as
  # versions. gash_upgrade uses the same rule.
  gashinst_latest_tag() {
    git tag --list --sort=v:refname 2>/dev/null |
      grep -E '^v?[0-9]+(\.[0-9]+)*$' |
      tail -n 1
  }

  # Whether TAG is an annotated tag whose object carries a signature: after
  # the header block a line equal to the BEGIN marker and, as the last line,
  # the matching END marker (SELF-10). A lightweight tag (cat-file tag
  # fails) and a message that only quotes the marker are not signed.
  gashinst_tag_signed() {
    local obj kind
    obj="$(git cat-file tag "refs/tags/$1" 2>/dev/null)" || return 1
    [[ "$obj" == *$'\n\n'* ]] || return 1
    obj=$'\n'"${obj#*$'\n\n'}"
    for kind in PGP SSH; do
      if [[ "$obj" == *$'\n'"-----BEGIN $kind SIGNATURE-----"$'\n'* &&
        "$obj" == *$'\n'"-----END $kind SIGNATURE-----" ]]; then
        return 0
      fi
    done
    return 1
  }

  # The program git verify-tag runs for the signature of TAG: for an SSH
  # signature gpg.ssh.program, else ssh-keygen; for a PGP one
  # gpg.openpgp.program, else gpg.program, else gpg.
  gashinst_tag_verifier() {
    local obj prog
    obj="$(git cat-file tag "refs/tags/$1" 2>/dev/null)" || return 1
    if [[ "$obj" == *$'\n'"-----BEGIN SSH SIGNATURE-----"$'\n'* ]]; then
      prog="$(git config --get gpg.ssh.program 2>/dev/null)" || prog=ssh-keygen
    else
      prog="$(git config --get gpg.openpgp.program 2>/dev/null)" ||
        prog="$(git config --get gpg.program 2>/dev/null)" || prog=gpg
    fi
    command printf '%s' "$prog"
  }

  # Download URL to FILE with curl or wget.
  gashinst_get() {
    if gashinst_has curl; then
      curl -fsSL --max-time 120 -o "$2" "$1" 2>/dev/null
    elif gashinst_has wget; then
      wget -q -T 120 -O "$2" "$1" 2>/dev/null
    else
      return 1
    fi
  }

  # The Sigstore signature of the release of TAG: the release workflow signs
  # checksums.txt keyless (checksums.txt.sigstore.json), and the certificate
  # names the workflow, the tag and the commit. cosign must accept it for
  # the commit TAG points at in this clone. Status 0 verified, 1 it does not
  # verify (cosign's last error line in the caller's GASHINST_COSIGN_ERROR),
  # 2 no cosign, 3 no signature to check.
  gashinst_verify_release() {
    local TAG="$1" COMMIT BASE TMP OUT RC REPO="mauriziofonte/gash"
    gashinst_has cosign || return 2
    COMMIT="$(git rev-parse --verify --quiet "refs/tags/$TAG^{commit}" 2>/dev/null)" || return 3
    BASE="${GASH_RELEASE_URL:-https://github.com/$REPO/releases/download/$TAG}"
    BASE="${BASE%/}"
    TMP="$(mktemp -d 2>/dev/null)" || return 3
    if ! gashinst_get "$BASE/checksums.txt" "$TMP/checksums.txt" ||
      ! gashinst_get "$BASE/checksums.txt.sigstore.json" "$TMP/checksums.txt.sigstore.json"; then
      command rm -rf -- "$TMP"
      return 3
    fi
    RC=0
    OUT="$(cosign verify-blob --bundle "$TMP/checksums.txt.sigstore.json" \
      --certificate-identity "https://github.com/$REPO/.github/workflows/release.yml@refs/tags/$TAG" \
      --certificate-oidc-issuer https://token.actions.githubusercontent.com \
      --certificate-github-workflow-repository "$REPO" \
      --certificate-github-workflow-ref "refs/tags/$TAG" \
      --certificate-github-workflow-sha "$COMMIT" \
      "$TMP/checksums.txt" 2>&1)" || RC=1
    command rm -rf -- "$TMP"
    if [ "$RC" -ne 0 ]; then
      OUT="$(printf '%s\n' "$OUT" | tail -n 1)"
      GASHINST_COSIGN_ERROR="${OUT#Error: }"
    fi
    return "$RC"
  }

  # Check the tag signature when there is one, else the Sigstore signature
  # of its release (with cosign on PATH). GASH_REQUIRE_SIGNED=1 turns a
  # missing or unverifiable signature into a refusal: status 1, the caller
  # stops.
  gashinst_verify_tag() {
    local TAG="$1" RC PROG GASHINST_COSIGN_ERROR=""
    if gashinst_tag_signed "$TAG"; then
      if git verify-tag "$TAG" >/dev/null 2>&1; then
        gashinst_success "Signature of $TAG verified."
        return 0
      fi
      # A verifier that is not installed is named, not blamed on the keyring.
      PROG="$(gashinst_tag_verifier "$TAG")" || PROG=""
      if [ -n "$PROG" ] && ! type -P "$PROG" >/dev/null 2>&1; then
        gashinst_warning "$TAG is signed, but its signature cannot be checked: $PROG is not installed."
      else
        gashinst_warning "$TAG is signed, but the signature could not be verified (is the key in your keyring?)."
      fi
    else
      RC=0
      gashinst_verify_release "$TAG" || RC=$?
      case "$RC" in
        0)
          gashinst_success "Release signature of $TAG verified (Sigstore)."
          return 0
          ;;
        1) gashinst_warning "The release signature of $TAG does not verify: $GASHINST_COSIGN_ERROR." ;;
        2)
          [ "${GASH_REQUIRE_SIGNED-0}" = "1" ] || return 0
          gashinst_warning "$TAG is not signed; install cosign to check its Sigstore release signature (https://docs.sigstore.dev/cosign/system_config/installation/)."
          ;;
        *)
          [ "${GASH_REQUIRE_SIGNED-0}" = "1" ] || return 0
          gashinst_warning "$TAG is not signed, and its release has no Sigstore signature (checksums.txt.sigstore.json)."
          ;;
      esac
    fi
    if [ "${GASH_REQUIRE_SIGNED-0}" = "1" ]; then
      gashinst_error "GASH_REQUIRE_SIGNED=1 is set; refusing to install $TAG."
      return 1
    fi
    return 0
  }

  # A new install is cloned, checked out and verified in a temporary
  # directory next to INSTALL_DIR, and moved into place only once it passed
  # (SELF-07): a refusal or a failure leaves nothing behind.
  gashinst_tmp_clone_cleanup() {
    if [ -n "${GASHINST_TMP_CLONE-}" ]; then
      command rm -rf -- "$GASHINST_TMP_CLONE"
      GASHINST_TMP_CLONE=""
    fi
  }

  # Stop after a refusal or failure: remove a temporary clone, say so.
  gashinst_abort() {
    if [ -n "${GASHINST_TMP_CLONE-}" ]; then
      gashinst_tmp_clone_cleanup
      gashinst_info "Nothing was installed."
    fi
    exit 1
  }

  gashinst_install_from_git() {
    local INSTALL_DIR
    INSTALL_DIR="$(gashinst_install_dir)"
    local GIT_REPO="${GASH_INSTALL_GIT_REPO:-https://github.com/mauriziofonte/gash.git}"

    if [ -d "$INSTALL_DIR/.git" ]; then
      gashinst_info "Gash is already installed in $INSTALL_DIR, updating it."
      cd "$INSTALL_DIR" || {
        gashinst_error "Failed to change directory to $INSTALL_DIR"
        exit 1
      }
      # The checkout below would overwrite local edits: refuse instead.
      if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
        gashinst_error "$INSTALL_DIR has local modifications."
        gashinst_error "Keep them with 'git -C $INSTALL_DIR stash', or discard them with 'git -C $INSTALL_DIR checkout -- .', then run the installer again."
        exit 1
      fi
      if ! command git fetch --quiet --tags origin; then
        gashinst_error "Failed to update Gash, run 'git fetch' in $INSTALL_DIR yourself."
        exit 1
      fi
    elif [ -e "$INSTALL_DIR" ] && [ -n "$(ls -A "$INSTALL_DIR" 2>/dev/null)" ]; then
      gashinst_error "$INSTALL_DIR exists and is not a Gash checkout. Move it away or set GASH_DIR."
      exit 1
    else
      gashinst_info "Cloning Gash into '$INSTALL_DIR'."
      local PARENT
      PARENT="$(dirname -- "$INSTALL_DIR")"
      mkdir -p -- "$PARENT"
      GASHINST_TMP_CLONE="$(mktemp -d "$PARENT/.gash-install.XXXXXX")" || {
        GASHINST_TMP_CLONE=""
        gashinst_error "Failed to create a temporary directory in $PARENT."
        exit 1
      }
      trap gashinst_tmp_clone_cleanup EXIT
      if ! command git clone --quiet "$GIT_REPO" "$GASHINST_TMP_CLONE"; then
        gashinst_error "Failed to clone $GIT_REPO."
        gashinst_abort
      fi
      cd "$GASHINST_TMP_CLONE" || {
        gashinst_error "Failed to change directory to $GASHINST_TMP_CLONE"
        gashinst_abort
      }
    fi

    local REF="${GASH_INSTALL_REF-}"
    local LATEST_TAG=""
    if [ -n "$REF" ]; then
      if ! git -c advice.detachedHead=false checkout --quiet "$REF" 2>/dev/null &&
        ! git -c advice.detachedHead=false checkout --quiet "origin/$REF" 2>/dev/null; then
        gashinst_error "Cannot check out '$REF'."
        gashinst_abort
      fi
    else
      LATEST_TAG="$(gashinst_latest_tag)" || LATEST_TAG=""
      if [ -n "$LATEST_TAG" ]; then
        gashinst_verify_tag "$LATEST_TAG" || gashinst_abort
        if ! git -c advice.detachedHead=false checkout --quiet "$LATEST_TAG"; then
          gashinst_error "Failed to check out $LATEST_TAG."
          gashinst_abort
        fi
      fi
    fi

    # Move a verified new clone into place: INSTALL_DIR is absent, or an
    # empty directory (possibly a link to one), which receives its contents.
    if [ -n "${GASHINST_TMP_CLONE-}" ]; then
      cd / || exit 1
      if [ -d "$INSTALL_DIR" ]; then
        if ! (shopt -s dotglob && command mv -- "$GASHINST_TMP_CLONE"/* "$INSTALL_DIR"/); then
          gashinst_error "Failed to move the new checkout into $INSTALL_DIR."
          gashinst_abort
        fi
      elif ! command mv -- "$GASHINST_TMP_CLONE" "$INSTALL_DIR"; then
        gashinst_error "Failed to move the new checkout into $INSTALL_DIR."
        gashinst_abort
      fi
      gashinst_tmp_clone_cleanup
      trap - EXIT
      cd "$INSTALL_DIR" || {
        gashinst_error "Failed to change directory to $INSTALL_DIR"
        exit 1
      }
    fi

    if [ -n "$REF" ]; then
      gashinst_success "Installed Gash at $REF."
    elif [ -n "$LATEST_TAG" ]; then
      gashinst_success "Installed Gash $LATEST_TAG."
    else
      gashinst_warning "No release tags found, using the latest commit on the default branch."
    fi
  }

  gashinst_do_install() {
    gashinst_preflight || exit 1

    if [ ! -w "$HOME" ]; then
      gashinst_error "Cannot write to your home directory ($HOME)."
      exit 1
    fi

    gashinst_install_from_git

    local GASH_PROFILE
    GASH_PROFILE="$(gashinst_detect_profile)"

    SOURCE_STR="\n# >>> GASH START >>>\n# Load Gash Bash - Do not edit this block, it is managed by Gash installer\nif [ -f \"\$HOME/.gashrc\" ]; then\n  source \"\$HOME/.gashrc\"\nfi\n# <<< GASH END <<<\n"

    if [ -z "$GASH_PROFILE" ]; then
      gashinst_info "Profile not found. Tried ~/.bashrc, ~/.bash_profile, and ~/.profile."
      gashinst_info "Create one of them and run this script again, or append these lines yourself:"
      command printf '%b' "${SOURCE_STR}"
    else
      # Anchor to a NON-commented source line: a commented-out
      # '# source ~/.gashrc' must not skip installation.
      if ! command grep -Eq '^[[:space:]]*source.*[/.]gashrc' "$GASH_PROFILE"; then
        gashinst_info "Adding the Gash block to $GASH_PROFILE"
        command printf '%b' "${SOURCE_STR}" >>"$GASH_PROFILE"
      else
        gashinst_info "Gash block already in ${GASH_PROFILE}"
      fi
    fi

    # ~/.gashrc: created once, never overwritten (yours to keep). A custom
    # install location is written into it so new shells find Gash.
    if [ ! -f "$HOME/.gashrc" ]; then
      local INSTALL_DIR TEMPLATE_DIR_LINE
      INSTALL_DIR="$(gashinst_install_dir)"
      if [ "$INSTALL_DIR" != "$(gashinst_default_install_dir)" ]; then
        TEMPLATE_DIR_LINE="__gash_home=\"\${GASH_DIR:-$(printf '%q' "$INSTALL_DIR")}\""
        sed "s|^__gash_home=.*|$TEMPLATE_DIR_LINE|" "$INSTALL_DIR/.gashrc" >"$HOME/.gashrc" || {
          gashinst_error "Failed to create ~/.gashrc"
          exit 1
        }
      else
        cp "$INSTALL_DIR/.gashrc" "$HOME/.gashrc" || {
          gashinst_error "Failed to create ~/.gashrc"
          exit 1
        }
      fi
      gashinst_success "Created ~/.gashrc"
    fi

    if ! gashinst_fetch_binary; then
      gashinst_info "Open a new terminal, or run 'source ${GASH_PROFILE:-~/.bashrc}', to load Gash."
      gashinst_warning "Gash cannot run until its binary is installed: bash $(gashinst_install_dir)/install.sh --repair"
      gashinst_reset
      return 0
    fi
    gashinst_feature_tools

    gashinst_info "Open a new terminal, or run 'source ${GASH_PROFILE:-~/.bashrc}', to start using Gash."
    gashinst_success "Enjoy using Gash"

    gashinst_reset
  }

  # Fetch and verify the release binary for this platform, reusing the
  # checkout's own compat bootstrap (SHA-256 checked). Best-effort: a
  # failure is reported, not fatal — the shim fetches on first use, and
  # 'install.sh --repair' retries. Status 1 when no binary is in place.
  gashinst_fetch_binary() {
    local INSTALL_DIR BOOT MODE
    INSTALL_DIR="$(gashinst_install_dir)"
    BOOT="$INSTALL_DIR/lib/compat/bootstrap.sh"
    if [ ! -r "$BOOT" ]; then
      gashinst_warning "No compat bootstrap in $INSTALL_DIR; the binary will be fetched on first use."
      return 0
    fi
    # shellcheck source=/dev/null
    if ! source "$BOOT" 2>/dev/null; then
      gashinst_warning "Could not load the compat bootstrap; the binary will be fetched on first use."
      return 0
    fi
    MODE=loud
    [ "${QUIET-0}" -eq 1 ] && MODE=quiet
    __gash_fetch "$MODE"
  }

  # --repair (upgrade.md 3.7): download and verify the binary of the
  # VERSION this install has checked out, replacing the one in place; the
  # checkout, the profile and ~/.gashrc are not touched. The binary in
  # place is set aside first and put back when the fetch fails. Status 0
  # when the verified binary is installed, 1 otherwise.
  gashinst_repair() {
    local INSTALL_DIR BOOT VDIR BACKUP="" MODE=loud
    INSTALL_DIR="$(gashinst_install_dir)"
    BOOT="$INSTALL_DIR/lib/compat/bootstrap.sh"
    if [ ! -r "$BOOT" ]; then
      gashinst_error "No Gash 3 install in $INSTALL_DIR (lib/compat/bootstrap.sh is missing); run the installer without --repair."
      return 1
    fi
    # shellcheck source=/dev/null
    if ! source "$BOOT" 2>/dev/null || [ -z "${__GASH_BOOT_VERSION-}" ]; then
      gashinst_error "Cannot read the version of $INSTALL_DIR (VERSION file)."
      return 1
    fi
    VDIR="${XDG_DATA_HOME:-$HOME/.local/share}/gash/versions/${__GASH_BOOT_VERSION}"
    if [ -e "$VDIR" ]; then
      BACKUP="$VDIR.repair.$$"
      command rm -rf -- "$BACKUP" 2>/dev/null || true
      if ! command mv -- "$VDIR" "$BACKUP"; then
        gashinst_error "Cannot move $VDIR aside."
        return 1
      fi
    fi
    [ "${QUIET-0}" -eq 1 ] && MODE=quiet
    if __gash_fetch "$MODE"; then
      [ -n "$BACKUP" ] && command rm -rf -- "$BACKUP"
      return 0
    fi
    command rm -rf -- "$VDIR" 2>/dev/null || true
    if [ -n "$BACKUP" ]; then
      command mv -- "$BACKUP" "$VDIR" || true
    fi
    if [ "$MODE" = quiet ]; then
      gashinst_error "gash ${__GASH_BOOT_VERSION} is not installed and could not be downloaded (${__GASH_FETCH_REASON})."
    fi
    return 1
  }

  gashinst_reset() {
    unset -f gashinst_has gashinst_echo gashinst_error gashinst_info gashinst_success \
      gashinst_warning gashinst_confirm gashinst_default_install_dir gashinst_install_dir \
      gashinst_try_profile gashinst_detect_profile gashinst_check_bash gashinst_latest_tag \
      gashinst_verify_tag gashinst_tag_signed gashinst_tag_verifier gashinst_get gashinst_verify_release \
      gashinst_tmp_clone_cleanup gashinst_abort gashinst_install_from_git gashinst_fetch_binary gashinst_repair \
      gashinst_pkg_manager gashinst_install_hint gashinst_xcode_stub gashinst_missing_tools gashinst_preflight \
      gashinst_feature_tools gashinst_do_install gashinst_reset
  }

  # Main script execution starts here

  set -e

  if [ -z "${BASH_VERSION-}" ] || [ -n "${ZSH_VERSION-}" ]; then
    echo "Error: run the Gash installer with bash (curl ... | bash)." >&2
    exit 1
  fi

  export LC_ALL=C

  if [[ ! -t 1 ]]; then
    NO_COLOR=1
  fi
  NO_COLOR=${NO_COLOR:-}

  if [ "$NO_COLOR" ]; then
    RED='' GREEN='' YELLOW='' CYAN='' BOLD='' NC=''
  else
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    CYAN='\033[0;36m'
    BOLD='\033[1;37m'
    NC='\033[0m'
  fi

  ASSUME_YES=0
  QUIET=0
  CHECK=0
  REPAIR=0
  for arg in "$@"; do
    case "$arg" in
      --assume-yes | -y) ASSUME_YES=1 ;;
      --quiet | -q) QUIET=1 ;;
      --check) CHECK=1 ;;
      --repair) REPAIR=1 ;;
      --help | -h)
        sed -n '5,39s/^  # \{0,1\}//p' "${BASH_SOURCE[0]}" 2>/dev/null ||
          echo "Usage: install.sh [--assume-yes] [--quiet] [--check] [--repair] [--help]"
        exit 0
        ;;
      *)
        gashinst_error "Unknown option: $arg (see --help)"
        exit 2
        ;;
    esac
  done

  # --check reports what is missing and changes nothing.
  if [ "$CHECK" -eq 1 ]; then
    gashinst_check_bash
    check_rc=0
    gashinst_preflight || check_rc=$?
    gashinst_reset
    exit "$check_rc"
  fi

  # --repair fetches the binary and does nothing else.
  if [ "$REPAIR" -eq 1 ]; then
    gashinst_check_bash
    repair_rc=0
    gashinst_repair || repair_rc=$?
    gashinst_reset
    exit "$repair_rc"
  fi

  gashinst_echo
  gashinst_echo "${CYAN}Welcome to the Gash installation script${NC}"
  gashinst_echo

  gashinst_check_bash

  gashinst_warning "This script will install Gash. This will modify your shell configuration files."
  # Capture the status: under `set -e` a bare non-zero return would abort
  # before the check.
  confirm_rc=0
  gashinst_confirm "${YELLOW}Do you want to continue?${NC}" || confirm_rc=$?
  if [ "$confirm_rc" -eq 2 ]; then
    gashinst_error "No terminal to ask for confirmation. Re-run with --assume-yes to install non-interactively."
    exit 1
  elif [ "$confirm_rc" -ne 0 ]; then
    gashinst_info "Installation aborted, nothing was changed."
    exit 0
  fi
  gashinst_echo

  gashinst_do_install

} # this ensures the entire script is downloaded #
