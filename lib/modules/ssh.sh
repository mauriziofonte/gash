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

    # One agent per user on a fixed socket, reused by every new shell
    # instead of a new agent per terminal (they were never stopped).
    local dir sock rc=0
    dir="${XDG_RUNTIME_DIR:-$HOME/.ssh}"
    [[ -d "$dir" && -w "$dir" ]] || dir="$HOME/.ssh"
    mkdir -p "$dir" 2>/dev/null && chmod 700 "$dir" 2>/dev/null
    sock="$dir/gash-ssh-agent.sock"

    if [[ -S "$sock" ]]; then
        # ssh-add -l: 0 = keys, 1 = no keys (agent alive), 2 = no agent.
        SSH_AUTH_SOCK="$sock" ssh-add -l >/dev/null 2>&1 || rc=$?
        if ((rc != 2)); then
            export SSH_AUTH_SOCK="$sock"
            return 0
        fi
        rm -f "$sock"
    fi

    local agent_out
    # Strip the trailing "echo Agent pid ..." to avoid noisy startup output.
    agent_out="$(ssh-agent -a "$sock" -s 2>/dev/null | sed '/^echo Agent pid /d' || true)"
    [[ -n "${agent_out-}" ]] || agent_out="$(ssh-agent -s 2>/dev/null | sed '/^echo Agent pid /d' || true)"
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

# Key/passphrase pairs arrive on STDIN as alternating base64 lines.
# argv is deliberately NOT used: /proc/<pid>/cmdline is world-readable,
# so secrets passed as arguments would be visible to any local process.
set pairs {}
while {[gets stdin line] >= 0} {
  if {$line eq ""} { continue }
  lappend pairs $line
}

if {([llength $pairs] % 2) != 0} {
  puts "Invalid input (expected pairs of key/password lines)"
  exit 2
}

for {set i 0} {$i < [llength $pairs]} {incr i 2} {
  set keyfile [b64decode [lindex $pairs $i]]
  set pass [b64decode [lindex $pairs [expr {$i+1}]]]

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
    needs_help "gash_ssh_auto_unlock" "gash_ssh_auto_unlock" "Start ssh-agent and add the keys listed as SSH:path=passphrase in ~/.gash_env (once per shell)." "${1-}" && return
    # Run once per shell session
    if [[ -n "${GASH_SSH_AUTOUNLOCK_RAN:-}" ]]; then
        return 0
    fi

    # Get SSH keys from config
    local ssh_keys
    __gash_load_env
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

    # Secrets travel to expect over STDIN (alternating base64 lines), never
    # in argv: /proc/<pid>/cmdline is world-readable for the lifetime of
    # the expect process.
    local stdin_payload=""
    local IFS=$'\n'
    local row

    # Fingerprints already in the agent: those keys are skipped.
    local loaded="" fp
    [[ "$ssh_add_list" == *SHA256:* || "$ssh_add_list" == *MD5:* ]] && loaded="$ssh_add_list"

    for row in $ssh_keys; do
        [[ -z "$row" ]] && continue

        local keyfile="${row%%$'\t'*}"
        local password="${row#*$'\t'}"

        if [[ -n "$loaded" ]] && command -v ssh-keygen >/dev/null 2>&1; then
            fp=""
            if [[ -f "$keyfile.pub" ]]; then
                fp="$(ssh-keygen -lf "$keyfile.pub" 2>/dev/null)" || fp=""
            else
                fp="$(ssh-keygen -lf "$keyfile" 2>/dev/null)" || fp=""
            fi
            fp="${fp#* }"
            fp="${fp%% *}"
            if [[ -n "$fp" && "$loaded" == *" $fp "* ]]; then
                continue
            fi
        fi

        # Encode values to safely carry special characters (incl. newlines)
        local key_b64 pass_b64
        key_b64="$(printf '%s' "$keyfile" | base64 | tr -d '\n')"
        pass_b64="$(printf '%s' "$password" | base64 | tr -d '\n')"
        stdin_payload+="$key_b64"$'\n'"$pass_b64"$'\n'
    done

    if [[ -z "$stdin_payload" ]]; then
        export GASH_SSH_AUTOUNLOCK_RAN=1
        return 0
    fi

    local expect_script
    expect_script="$(mktemp -t gash-ssh-expect.XXXXXX)"
    __gash_generate_expect_script >"$expect_script"

    local expect_output
    expect_output="$(printf '%s' "$stdin_payload" | expect -f "$expect_script" 2>&1)"
    rm -f "$expect_script" >/dev/null 2>&1

    echo "$expect_output" | __gash_format_expect_output

    # Mark as done only after we actually attempted to unlock
    export GASH_SSH_AUTOUNLOCK_RAN=1

    return 0
}

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

    __gash_register_help "gash_ssh_auto_unlock" \
        --module "ssh" \
        --short "Auto-unlock SSH keys from ~/.gash_env on shell startup" \
        --see-also "gash_env_init" \
        <<'HELP'
USAGE
  gash_ssh_auto_unlock

EXAMPLES
  # Runs automatically on shell startup if SSH keys are configured.
  # Manual invocation:
  gash_ssh_auto_unlock

CONFIGURATION
  Add SSH keys to ~/.gash_env:
    SSH:~/.ssh/id_ed25519=your_passphrase
    SSH:~/.ssh/work_rsa=work-passphrase

REQUIREMENTS
  - expect (for non-interactive passphrase entry)
  - ssh-agent (auto-started if not running)

NOTES
  Runs once per session (tracked by GASH_SSH_AUTOUNLOCK_RAN).
  Only unlocks keys that are not already loaded in the agent.
HELP

fi # end help registration guard
