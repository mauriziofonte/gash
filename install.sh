#!/usr/bin/env bash

# GASH (Gash, Another SHell) – your terminal's new best friend!
# Installation script for Gash. It clones the Gash repository and updates the shell configuration files.
# 
# Author: Maurizio Fonte (https://www.mauriziofonte.it)
# Version: 1.0.2
# Release Date: 2024-10-24
# Last Update: 2024-10-26
# License: Apache License
#
# If you find any issue, please report it on GitHub: https://github.com/mauriziofonte/gash/issues
#

{ # this ensures the entire script is downloaded #

    GIT_REPO="https://github.com/mauriziofonte/gash.git"
    INSTALL_DIR="$HOME/.gash"

    # Exit immediately if a command exits with a non-zero status
    set -e

    # Ensure the script is run with bash
    if [ -z "$BASH_VERSION" ]; then
        echo -e " ⛔ Gash is not compatible with the current shell. Please run the script with bash."
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

    gashinst_echo () {
        if [ "$QUIET" -eq 1 ]; then
            return
        fi

        echo -e "$1"
    }
    
    gashinst_info() {
        gashinst_echo "${CYAN} 💡 $1${NC}"
    }

    gashinst_success() {
        gashinst_echo "${GREEN} ✅ $1${NC}"
    }

    gashinst_warning() {
        gashinst_echo "${YELLOW} ⚠️ $1${NC}"
    }

    gashinst_error() {
        gashinst_echo "${RED} ⛔ $1${NC}"
    }

    gashinst_confirm() {
        if [ "$ASSUME_YES" -eq 1 ]; then
            return 0
        fi

        echo -ne "$@ \e[1;37m(y/N):\033[0m "
        read -n 1 -r < /dev/tty
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            return 0
        fi

        return 1
    }

    # Function to check if the script is running in a supported environment
    gashinst_check_supported_environment() {
        # Check if the OS is Linux, macOS, or Windows WSL
        OS=$(uname -s)
        case "$OS" in
            Linux)
                if grep -qi "microsoft" /proc/sys/kernel/osrelease 2>/dev/null; then
                    gashinst_info "Detected Windows Subsystem for Linux (WSL)."
                else
                    gashinst_info "Detected Linux OS."
                fi
                ;;
            Darwin)
                gashinst_info "Detected macOS."
                ;;
            *)
                gashinst_error "Unsupported OS: $OS. This script only supports Linux, macOS, and Windows WSL."
                exit 1
                ;;
        esac

        # Check for required commands
        for cmd in git grep ln mv cp date; do
            if ! command -v "$cmd" &>/dev/null; then
                gashinst_error "Command '$cmd' is required but not installed."
                exit 1
            fi
        done

        # Check for write permissions in the home directory
        if [ ! -w "$HOME" ]; then
            gashinst_error "Insufficient permissions to write to the home directory. Please check your permissions or try running the script with elevated privileges."
            exit 1
        fi
    }

    # Function to clone or update the Gash repository
    gashinst_install_or_update() {
        if [ -d "$INSTALL_DIR" ]; then
            gashinst_info "Gash is already installed. Checking for updates..."
            cd "$INSTALL_DIR" || { error "Failed to change directory to $INSTALL_DIR"; exit 1; }
            git fetch --tags >/dev/null 2>&1 || { error "Failed to fetch updates from the repository."; exit 1; }
            LATEST_TAG=$(git describe --tags "$(git rev-list --tags --max-count=1)")
            CURRENT_TAG=$(git describe --tags)

            if [ "$CURRENT_TAG" != "$LATEST_TAG" ]; then
                gashinst_info "Updating to the latest Gash release ($LATEST_TAG)..."
                git checkout "$LATEST_TAG" >/dev/null 2>&1 || { error "Failed to checkout tag $LATEST_TAG"; exit 1; }
                git reset --hard "$LATEST_TAG" >/dev/null 2>&1  || { error "Failed to reset to $LATEST_TAG"; exit 1; }
                gashinst_success "Updated to version $LATEST_TAG."
            else
                gashinst_success "You are already using the latest version ($CURRENT_TAG)."
            fi
        else
            gashinst_info "Cloning the Gash repository..."
            git clone "$GIT_REPO" "$INSTALL_DIR" >/dev/null 2>&1 || { error "Failed to clone the Gash repository."; exit 1; }
            cd "$INSTALL_DIR" || { error "Failed to change directory to $INSTALL_DIR"; exit 1; }
            git fetch --tags >/dev/null 2>&1 || { error "Failed to fetch tags from the repository."; exit 1; }
            LATEST_TAG=$(git describe --tags "$(git rev-list --tags --max-count=1)")
            git checkout "$LATEST_TAG" >/dev/null 2>&1 || { error "Failed to checkout tag $LATEST_TAG"; exit 1; }
            gashinst_success "Installed Gash version $LATEST_TAG."
        fi

        # Ensure we have a ".gashrc" file in our home directory. If not, create it (it's on .gash/.gashrc to be copied)
        if [ ! -f "$HOME/.gashrc" ]; then
            gashinst_info "Creating a new .gashrc file in your home directory..."
            touch "$HOME/.gashrc" || { error "Failed to create .gashrc"; exit 1; }
            cp "$INSTALL_DIR/.gashrc" "$HOME/.gashrc" || { error "Failed to copy .gashrc"; exit 1; }
            gashinst_success "Created .gashrc file."
        fi

        # Backup and modify the .bash_profile file to source the .gashrc file
        if [ -f "$HOME/.bash_profile" ]; then
            cp "$HOME/.bash_profile" "$HOME/.bash_profile.bak_$(date +%Y%m%d%H%M%S)"
            gashinst_info "Backup of your current .bash_profile created."
        else
            gashinst_info "Creating a new .bash_profile file in your home directory..."
            touch "$HOME/.bash_profile" || { error "Failed to create .bash_profile"; exit 1; }
        fi

        # Check if .bash_profile sources .gashrc, if not, add it
        if ! grep -Fq "if [ -f ~/.gashrc ]; then" "$HOME/.bash_profile"; then
            gashinst_info "Modifying the .bash_profile file to source the .gashrc file..."
            echo -e "\n# Load the Gash Environment\nif [ -f ~/.gashrc ]; then\n    source ~/.gashrc\nfi" >> "$HOME/.bash_profile"
        fi
    }

    # Function ro run the installation script
    gashinst_run () {
        gashinst_echo
        gashinst_echo "${CYAN}🚀 Welcome to the Gash installation script! 🚀${NC}"

        # Check for supported environment
        gashinst_check_supported_environment

        # Prompt user for confirmation before proceeding
        gashinst_echo
        gashinst_echo " ⚠️ ${YELLOW}This script will install Gash. This will modify your shell configuration files.${NC}"
        gashinst_confirm " ⚠️ ${YELLOW}Do you want to continue?${NC}"
        if [ $? -eq 1 ]; then
            info "Sorry to see you go... Installation aborted."
            exit 0
        fi
        gashinst_echo

        # Install or update Gash
        gashinst_install_or_update

        # Source the updated .bash_profile
        [ -f "$HOME/.bash_profile" ] && source "$HOME/.bash_profile"

        gashinst_success "Done! Enjoy using Gash 🚀"

        gashinst_reset
    }

    #
    # Unsets the various functions defined
    # during the execution of the install script
    gashinst_reset () {
        unset -f gashinst_echo gashinst_info gashinst_success gashinst_warning gashinst_error gashinst_confirm gashinst_reset
    }

    #
    # Runs the installer
    gashinst_run
    
} # this ensures the entire script is downloaded #
