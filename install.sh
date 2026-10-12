#!/usr/bin/env bash

{ # this ensures the entire script is downloaded #

  # Gash installer: clones https://github.com/mauriziofonte/gash into
  # ~/.gash (or $GASH_DIR), checks out the latest release tag and adds one
  # delimited block to your Bash profile that sources ~/.gashrc.
  #
  # Usage: install.sh [--assume-yes] [--quiet] [--help]
  #
  # Environment:
  #   GASH_DIR                install location (default: ~/.gash)
  #   PROFILE                 profile file to edit (default: detected;
  #                           /dev/null to edit none)
  #   GASH_INSTALL_GIT_REPO   repository to clone (default: GitHub)
  #   GASH_INSTALL_REF        branch, tag or commit to install instead of the
  #                           latest release tag
  #   GASH_REQUIRE_SIGNED=1   abort unless the release tag has a signature
  #                           that `git verify-tag` accepts
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

  # Highest release tag: vX.Y.Z or X.Y (no pre-release suffix), compared as
  # versions. gash_upgrade uses the same rule.
  gashinst_latest_tag() {
    git tag --list --sort=v:refname 2>/dev/null |
      grep -E '^v?[0-9]+(\.[0-9]+)*$' |
      tail -n 1
  }

  # Check the tag signature when there is one. GASH_REQUIRE_SIGNED=1 turns
  # a missing or unverifiable signature into an error.
  gashinst_verify_tag() {
    local TAG="$1"
    if git cat-file -p "refs/tags/$TAG" 2>/dev/null | grep -Eq -- '-----BEGIN (PGP|SSH) SIGNATURE-----'; then
      if git verify-tag "$TAG" >/dev/null 2>&1; then
        gashinst_success "Signature of $TAG verified."
        return 0
      fi
      gashinst_warning "$TAG is signed, but the signature could not be verified (is the key in your keyring?)."
    elif [ "${GASH_REQUIRE_SIGNED-0}" = "1" ]; then
      gashinst_warning "$TAG is not signed."
    else
      return 0
    fi
    if [ "${GASH_REQUIRE_SIGNED-0}" = "1" ]; then
      gashinst_error "GASH_REQUIRE_SIGNED=1 is set; refusing to install $TAG."
      exit 1
    fi
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
      mkdir -p "$INSTALL_DIR"
      if ! command git clone --quiet "$GIT_REPO" "$INSTALL_DIR"; then
        gashinst_error "Failed to clone $GIT_REPO."
        exit 1
      fi
      cd "$INSTALL_DIR" || {
        gashinst_error "Failed to change directory to $INSTALL_DIR"
        exit 1
      }
    fi

    local REF="${GASH_INSTALL_REF-}"
    if [ -n "$REF" ]; then
      if ! git -c advice.detachedHead=false checkout --quiet "$REF" 2>/dev/null &&
        ! git -c advice.detachedHead=false checkout --quiet "origin/$REF" 2>/dev/null; then
        gashinst_error "Cannot check out '$REF'."
        exit 1
      fi
      gashinst_success "Installed Gash at $REF."
      return 0
    fi

    local LATEST_TAG
    LATEST_TAG="$(gashinst_latest_tag)" || LATEST_TAG=""
    if [ -n "$LATEST_TAG" ]; then
      gashinst_verify_tag "$LATEST_TAG"
      if ! git -c advice.detachedHead=false checkout --quiet "$LATEST_TAG"; then
        gashinst_error "Failed to check out $LATEST_TAG."
        exit 1
      fi
      gashinst_success "Installed Gash $LATEST_TAG."
    else
      gashinst_warning "No release tags found, using the latest commit on the default branch."
    fi
  }

  gashinst_do_install() {
    for cmd in git grep sed awk; do
      if ! gashinst_has "$cmd"; then
        gashinst_error "Command '$cmd' is required but not installed."
        exit 1
      fi
    done

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

    gashinst_info "Open a new terminal, or run 'source ${GASH_PROFILE:-~/.bashrc}', to start using Gash."
    gashinst_success "Enjoy using Gash"

    gashinst_reset
  }

  gashinst_reset() {
    unset -f gashinst_has gashinst_echo gashinst_error gashinst_info gashinst_success \
      gashinst_warning gashinst_confirm gashinst_default_install_dir gashinst_install_dir \
      gashinst_try_profile gashinst_detect_profile gashinst_check_bash gashinst_latest_tag \
      gashinst_verify_tag gashinst_install_from_git gashinst_do_install gashinst_reset
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
  for arg in "$@"; do
    case "$arg" in
      --assume-yes | -y) ASSUME_YES=1 ;;
      --quiet | -q) QUIET=1 ;;
      --help | -h)
        sed -n '5,21s/^  # \{0,1\}//p' "${BASH_SOURCE[0]}" 2>/dev/null ||
          echo "Usage: install.sh [--assume-yes] [--quiet] [--help]"
        exit 0
        ;;
      *)
        gashinst_error "Unknown option: $arg (see --help)"
        exit 2
        ;;
    esac
  done

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
