#!/usr/bin/env bash

# Gash Module: SSH Operations
# Functions for SSH key management and auto-unlock.
#
# Dependencies: core/output.sh, core/validation.sh, core/utils.sh, core/config.sh
#
# SSH keys are configured in ~/.gash_env with format:
#   SSH:~/.ssh/id_ed25519=my passphrase
#   SSH:~/.ssh/work_rsa=work-passphrase

# -----------------------------------------------------------------------------
# Helper Functions
# -----------------------------------------------------------------------------

# Print a best-effort install command for `expect` based on the OS.
# Usage: __gash_os_expect_install_hint
__gash_os_expect_install_hint() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        echo "brew install expect"
        return 0
    fi

    if [[ -r /etc/os-release ]]; then
        # shellcheck disable=SC1091
        . /etc/os-release
        local id="${ID:-}"
        local id_like="${ID_LIKE:-}"

        if [[ "$id" == "debian" || "$id" == "ubuntu" || "$id_like" == *"debian"* ]]; then
            echo "sudo apt update && sudo apt install -y expect"
            return 0
        fi
        if [[ "$id" == "fedora" || "$id_like" == *"fedora"* ]]; then
            echo "sudo dnf install -y expect"
            return 0
        fi
        if [[ "$id" == "centos" || "$id" == "rhel" || "$id_like" == *"rhel"* ]]; then
            echo "sudo yum install -y expect"
            return 0
        fi
        if [[ "$id" == "arch" || "$id_like" == *"arch"* ]]; then
            echo "sudo pacman -S --noconfirm expect"
            return 0
        fi
        if [[ "$id" == "opensuse"* || "$id_like" == *"suse"* ]]; then
            echo "sudo zypper install -y expect"
            return 0
        fi
    fi

    echo "Install 'expect' using your system package manager (e.g. apt/dnf/yum/pacman)."
}

# Format expect output with Gash-style coloring.
# Usage: echo "$output" | __gash_format_expect_output
__gash_format_expect_output() {
    local line
    while IFS= read -r line; do
        line="${line%$'\r'}"
        [[ -z "$line" ]] && continue

        # Filter common noise from expect
        if [[ "$line" == spawn\ ssh-add* ]]; then
            continue
        fi

        if [[ "$line" == Agent\ pid* ]]; then
            __gash_ssh "$line" "info"
        elif [[ "$line" == Identity\ added:* ]]; then
            __gash_ssh "$line" "success"
        elif [[ "$line" == The\ agent\ has\ no\ identities.* ]]; then
            __gash_ssh "$line" "warning"
        elif [[ "$line" == Bad\ passphrase* ]]; then
            __gash_ssh "$line" "error"
        elif [[ "$line" == Could\ not\ open\ a\ connection\ to\ your\ authentication\ agent.* ]]; then
            __gash_ssh "$line" "error"
        else
            __gash_ssh "$line" "info"
        fi
    done
}

# Try to start ssh-agent in the current shell.
# Usage: __gash_try_start_ssh_agent
# Returns: 0 if SSH_AUTH_SOCK becomes available, 1 otherwise
__gash_try_start_ssh_agent() {
    if ! command -v ssh-agent >/dev/null 2>&1; then
        return 1
    fi

    local agent_out
    # Strip the trailing "echo Agent pid ..." to avoid noisy startup output.
    agent_out="$(ssh-agent -s 2>/dev/null | sed '/^echo Agent pid /d' || true)"
    if [[ -z "${agent_out-}" ]]; then
        return 1
    fi

    # shellcheck disable=SC1090
    eval "$agent_out" >/dev/null 2>&1 || true

    if [[ -n "${SSH_AUTH_SOCK-}" ]]; then
        return 0
    fi

    return 1
}

# Generate the expect script content for SSH auto-unlock.
# Usage: __gash_generate_expect_script
__gash_generate_expect_script() {
    cat <<'EOF'
#!/usr/bin/expect -f

proc b64decode {s} {
  return [binary decode base64 $s]
}

set timeout 20

if {([llength $argv] % 2) != 0} {
  puts "Invalid arguments (expected pairs of key/password)"
  exit 2
}

for {set i 0} {$i < [llength $argv]} {incr i 2} {
  set key_b64 [lindex $argv $i]
  set pass_b64 [lindex $argv [expr {$i+1}]]
  set keyfile [b64decode $key_b64]
  set pass [b64decode $pass_b64]

  spawn ssh-add -- $keyfile
  expect {
    -re {Enter passphrase.*:} {
      send -- "$pass\r"
      exp_continue
    }
    -re {Identity added:.*} { }
    -re {Bad passphrase.*} { }
    -re {Could not open a connection to your authentication agent.*} { }
    eof { }
    timeout { }
  }
}
EOF
}

# -----------------------------------------------------------------------------
# Main Function
# -----------------------------------------------------------------------------

# Auto-add SSH keys listed in ~/.gash_env using `expect`.
# This is designed to be safe when sourced at shell startup.
#
# Configuration in ~/.gash_env:
#   SSH:~/.ssh/id_ed25519=my passphrase
#   SSH:~/.ssh/work_rsa=work-passphrase
#
# Behavior:
#   - Automatically starts ssh-agent if not running and SSH keys are configured
#
# Usage: gash_ssh_auto_unlock
gash_ssh_auto_unlock() {
    # Run once per shell session
    if [[ -n "${GASH_SSH_AUTOUNLOCK_RAN:-}" ]]; then
        return 0
    fi

    # Get SSH keys from config
    local ssh_keys
    ssh_keys="$(__gash_get_ssh_keys)"

    if [[ -z "$ssh_keys" ]]; then
        # No SSH keys configured
        export GASH_SSH_AUTOUNLOCK_RAN=1
        return 0
    fi

    if ! command -v ssh-add >/dev/null 2>&1; then
        __gash_error "ssh-add is not available; cannot auto-unlock SSH keys."
        return 0
    fi

    # Detect ssh-agent not running early
    local ssh_add_list
    ssh_add_list="$(ssh-add -l 2>&1 || true)"

    if [[ "$ssh_add_list" == *"Could not open a connection to your authentication agent"* ]]; then
        # Auto-start ssh-agent when SSH keys are configured
        __gash_try_start_ssh_agent || true
        ssh_add_list="$(ssh-add -l 2>&1 || true)"

        if [[ "$ssh_add_list" == *"Could not open a connection to your authentication agent"* ]]; then
            __gash_error "ssh-agent is not running and could not be started automatically."
            return 0
        fi
    fi

    if ! command -v expect >/dev/null 2>&1; then
        __gash_warning "'expect' is not installed."
        echo -e "${__GASH_BOLD_WHITE}Install it to enable SSH key auto-unlock:${__GASH_COLOR_OFF} ${__GASH_CYAN}$(__gash_os_expect_install_hint)${__GASH_COLOR_OFF}"
        return 0
    fi

    local -a args=()
    local IFS=$'\n'
    local row

    for row in $ssh_keys; do
        [[ -z "$row" ]] && continue

        local keyfile="${row%%$'\t'*}"
        local password="${row#*$'\t'}"

        # Encode values to safely pass special characters through argv
        local key_b64 pass_b64
        key_b64="$(printf '%s' "$keyfile" | base64 | tr -d '\n')"
        pass_b64="$(printf '%s' "$password" | base64 | tr -d '\n')"
        args+=("$key_b64" "$pass_b64")
    done

    if [[ ${#args[@]} -eq 0 ]]; then
        export GASH_SSH_AUTOUNLOCK_RAN=1
        return 0
    fi

    local expect_script
    expect_script="$(mktemp -t gash-ssh-expect.XXXXXX)"
    __gash_generate_expect_script > "$expect_script"

    local expect_output
    expect_output="$(expect -f "$expect_script" -- "${args[@]}" 2>&1)"
    rm -f "$expect_script" >/dev/null 2>&1

    echo "$expect_output" | __gash_format_expect_output

    # Mark as done only after we actually attempted to unlock
    export GASH_SSH_AUTOUNLOCK_RAN=1

    return 0
}
