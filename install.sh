#!/usr/bin/env bash

{ # this ensures the entire script is downloaded #

# GASH (Gash, Another SHell) – your terminal's new best friend!
# Installation script for Gash. It clones the Gash repository and updates the shell configuration files.
#
# Author: Maurizio Fonte (https://www.mauriziofonte.it)
# Version: 1.3.1
# Release Date: 2024-10-24
# Last Update: 2026-01-06
# License: Apache License
#
# If you find any issue, please report it on GitHub: https://github.com/mauriziofonte/gash/issues
#

gashinst_has() {
  type "$1" > /dev/null 2>&1
}

gashinst_echo() {
  if [ "${QUIET-0}" -eq 1 ]; then
    return 0
  fi

  command printf '%b\n' "$1"
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

gashinst_confirm() {
  if [ "${ASSUME_YES-0}" -eq 1 ]; then
    return 0
  fi

  command printf '%b' " $1 \e[1;37m(y/N):\033[0m "
  read -r REPLY < /dev/tty
  if [[ "$REPLY" =~ ^[Yy]$ ]]; then
    return 0
  fi
  return 1
}

gashinst_default_install_dir() {
  printf %s "${HOME}/.gash"
}

gashinst_install_dir() {
  if [ -n "$GASH_DIR" ]; then
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

  if [ -n "${PROFILE}" ] && [ -f "${PROFILE}" ]; then
    command printf '%s\n' "${PROFILE}"
    return
  fi

  local DETECTED_PROFILE
  DETECTED_PROFILE=''

  if [ -n "$BASH_VERSION" ]; then
    if [ -f "$HOME/.bashrc" ]; then
      DETECTED_PROFILE="$HOME/.bashrc"
    elif [ -f "$HOME/.bash_profile" ]; then
      DETECTED_PROFILE="$HOME/.bash_profile"
    fi
  fi

  if [ -z "$DETECTED_PROFILE" ]; then
    for EACH_PROFILE in ".profile" ".bashrc" ".bash_profile"
    do
      if DETECTED_PROFILE="$(gashinst_try_profile "${HOME}/${EACH_PROFILE}")"; then
        break
      fi
    done
  fi

  if [ -n "$DETECTED_PROFILE" ]; then
    command printf '%s\n' "$DETECTED_PROFILE"
  fi
}

gashinst_install_from_git() {
  local INSTALL_DIR
  INSTALL_DIR="$(gashinst_install_dir)"
  local GIT_REPO
  GIT_REPO="${GASH_INSTALL_GIT_REPO:-https://github.com/mauriziofonte/gash.git}"
  local FETCH_ERROR

  if [ -d "$INSTALL_DIR/.git" ]; then
    gashinst_info "Gash is already installed in $INSTALL_DIR, trying to update using git."
    FETCH_ERROR="Failed to update Gash, run 'git fetch' in $INSTALL_DIR yourself."
    cd "$INSTALL_DIR" || { gashinst_error "Failed to change directory to $INSTALL_DIR"; exit 1; }
    if ! command git fetch --tags origin; then
      gashinst_error "$FETCH_ERROR"
      exit 1
    fi
  else
    gashinst_info "Cloning Gash from git to '$INSTALL_DIR'."
    mkdir -p "$INSTALL_DIR"
    if ! command git clone "$GIT_REPO" "$INSTALL_DIR"; then
      gashinst_error "Failed to clone Gash repo. Please report this!"
      exit 1
    fi
    cd "$INSTALL_DIR" || { gashinst_error "Failed to change directory to $INSTALL_DIR"; exit 1; }
  fi

  local LATEST_TAG
  LATEST_TAG="$(git describe --tags "$(git rev-list --tags --max-count=1)" 2>/dev/null)"
  if [ -n "$LATEST_TAG" ]; then
    if ! git checkout "$LATEST_TAG" >/dev/null 2>&1; then
      gashinst_error "Failed to checkout tag $LATEST_TAG"
      exit 1
    fi
    if ! git reset --hard "$LATEST_TAG" >/dev/null 2>&1; then
      gashinst_error "Failed to reset to $LATEST_TAG"
      exit 1
    fi
    gashinst_success "Installed Gash version $LATEST_TAG."
  else
    gashinst_warning "No tags found, using the latest commit on the default branch."
  fi

  gashinst_info "Compressing and cleaning up git repository."
  if ! git reflog expire --expire=now --all; then
    gashinst_error "Your version of git is out of date. Please update it!"
  fi
  if ! git gc --auto --aggressive --prune=now; then
    gashinst_error "Your version of git is out of date. Please update it!"
  fi
}

gashinst_do_install() {
  # Ensure the script is run with Bash and not Zsh or another shell
  if [ -z "$BASH_VERSION" ]; then
    gashinst_error "Gash is designed for Bash and may not work properly with other shells. Please run the script with Bash."
    exit 1
  fi

  if [ -n "$ZSH_VERSION" ]; then
    gashinst_error "It looks like you're using Zsh. Gash is designed for Bash and may not work properly with Zsh. Please switch to Bash before proceeding."
    exit 1
  fi

  # Check for required commands
  for cmd in git grep sed awk; do
    if ! gashinst_has "$cmd"; then
      gashinst_error "Command '$cmd' is required but not installed."
      exit 1
    fi
  done

  # Check for write permissions in the home directory
  if [ ! -w "$HOME" ]; then
    gashinst_error "Insufficient permissions to write to the home directory. Please check your permissions or try running the script with elevated privileges."
    exit 1
  fi

  # Install Gash from Git
  gashinst_install_from_git

  # Detect profile file
  local GASH_PROFILE
  GASH_PROFILE="$(gashinst_detect_profile)"

  local PROFILE_INSTALL_DIR
  PROFILE_INSTALL_DIR="$(gashinst_install_dir | command sed "s:^$HOME:\$HOME:")"

  SOURCE_STR="\n# >>> GASH START >>>\n# Load Gash Bash - Do not edit this block, it is managed by Gash installer\nif [ -f \"\$HOME/.gashrc\" ]; then\n  source \"\$HOME/.gashrc\"\nfi\n# <<< GASH END <<<\n"

  if [ -z "$GASH_PROFILE" ]; then
    gashinst_info "Profile not found. Tried ~/.bashrc, ~/.bash_profile, and ~/.profile."
    gashinst_info "Create one of them and run this script again."
    gashinst_info "OR"
    gashinst_info "Append the following lines to the correct file yourself:"
    command printf "${SOURCE_STR}"
  else
    if ! command grep -qc 'source.*\.gashrc' "$GASH_PROFILE"; then
      gashinst_info "Appending Gash source string to $GASH_PROFILE"
      command printf "${SOURCE_STR}" >> "$GASH_PROFILE"
    else
      gashinst_info "Gash source string already in ${GASH_PROFILE}"
    fi
  fi

  # Ensure we have a ".gashrc" file in the home directory
  if [ ! -f "$HOME/.gashrc" ]; then
    gashinst_info "Creating a new .gashrc file in your home directory..."
    cp "$(gashinst_install_dir)/.gashrc" "$HOME/.gashrc" || { gashinst_error "Failed to copy .gashrc"; exit 1; }
    gashinst_success "Created .gashrc file."
  fi

  gashinst_info "Please restart your terminal or run 'source $GASH_PROFILE' to start using Gash."
  gashinst_success "Enjoy using Gash"

  gashinst_reset
}

gashinst_reset() {
  unset -f gashinst_has gashinst_echo gashinst_error gashinst_info gashinst_success \
    gashinst_warning gashinst_confirm gashinst_default_install_dir gashinst_install_dir \
    gashinst_try_profile gashinst_detect_profile gashinst_install_from_git gashinst_do_install \
    gashinst_reset
}

# Main script execution starts here

# Exit immediately if a command exits with a non-zero status
set -e

# Ensure the script is run with Bash
if [ -z "$BASH_VERSION" ]; then
  echo -e "Error: Gash is not compatible with the current shell. Please run the script with Bash."
  exit 1
fi

# Export LC_ALL to ensure consistent behavior across locales
export LC_ALL=C

# Color codes for colored output (with option to disable)
if [[ ! -t 1 ]]; then
  NO_COLOR=1
fi

NO_COLOR=${NO_COLOR:-}

if [ "$NO_COLOR" ]; then
  RED=''
  GREEN=''
  YELLOW=''
  CYAN=''
  NC=''
else
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  CYAN='\033[0;36m'
  NC='\033[0m' # No Color
fi

# Parse command-line arguments
ASSUME_YES=0
QUIET=0

for arg in "$@"; do
  case "$arg" in
    --assume-yes)
      ASSUME_YES=1
      ;;
    --quiet)
      QUIET=1
      ;;
  esac
done

gashinst_echo
gashinst_echo "${CYAN}Welcome to the Gash installation script${NC}"

# Prompt user for confirmation before proceeding
gashinst_echo
gashinst_warning "This script will install Gash. This will modify your shell configuration files."
gashinst_confirm "${YELLOW}Do you want to continue?${NC}"
if [ $? -eq 1 ]; then
  gashinst_info "Sorry to see you go... Installation aborted."
  exit 0
fi
gashinst_echo

# Run the installer
gashinst_do_install

} # this ensures the entire script is downloaded #
