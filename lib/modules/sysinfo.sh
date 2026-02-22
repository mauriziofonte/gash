#!/usr/bin/env bash

# =============================================================================
# Gash Module: System Information Enumeration
# =============================================================================
#
# Comprehensive system enumeration for headless Debian/Ubuntu servers.
# Each section supports two output modes:
#   - verbose (default): colored terminal output for humans
#   - llm (--llm flag): compact section-tagged output for AI token minimization
#
# Dependencies: core/output.sh, core/validation.sh, core/utils.sh
# Runtime: systemctl, ss (optional: sudo for full enumeration)
#
# Public functions:
#   sysinfo (si) [section] [--llm] - System information enumeration
#
# Sections: identity storage services auth network security webstack mail infra system all
#
# =============================================================================

# =============================================================================
# GLOBALS
# =============================================================================

__SYSINFO_MODE="verbose"             # "verbose" or "llm"
__SYSINFO_HAS_SUDO=0                 # 1 if sudo credentials are cached
__SYSINFO_SUDO_KEEPALIVE_PID=""      # PID of sudo keep-alive background process

# =============================================================================
# SUDO CACHING
# =============================================================================

# Acquire sudo credentials once and keep them alive.
# Sets __SYSINFO_HAS_SUDO=1 on success, 0 on failure.
# Starts a background keep-alive process (killed by __sysinfo_release_sudo).
__sysinfo_ensure_sudo() {
    # Already root? No sudo needed.
    if [[ $(id -u) -eq 0 ]]; then
        __SYSINFO_HAS_SUDO=1
        return 0
    fi

    # Already cached from a previous call?
    if sudo -n true 2>/dev/null; then
        __SYSINFO_HAS_SUDO=1
    else
        if [[ "$__SYSINFO_MODE" == "verbose" ]]; then
            __gash_info "Elevated privileges needed for full system enumeration."
        fi
        if sudo -v 2>/dev/null; then
            __SYSINFO_HAS_SUDO=1
        else
            if [[ "$__SYSINFO_MODE" == "verbose" ]]; then
                __gash_warning "Could not obtain sudo. Some data will be limited."
            fi
            __SYSINFO_HAS_SUDO=0
            return 1
        fi
    fi

    # Keep-alive: refresh sudo every 50s (default sudo timeout = 5min)
    (while true; do sudo -n true 2>/dev/null; sleep 50; done) &
    __SYSINFO_SUDO_KEEPALIVE_PID=$!
    disown "$__SYSINFO_SUDO_KEEPALIVE_PID" 2>/dev/null
    return 0
}

# Release sudo keep-alive background process.
__sysinfo_release_sudo() {
    if [[ -n "$__SYSINFO_SUDO_KEEPALIVE_PID" ]]; then
        kill "$__SYSINFO_SUDO_KEEPALIVE_PID" 2>/dev/null || true
        wait "$__SYSINFO_SUDO_KEEPALIVE_PID" 2>/dev/null || true
        __SYSINFO_SUDO_KEEPALIVE_PID=""
    fi
}

# Run a command with sudo if credentials are cached, else without.
# All stderr is suppressed (graceful degradation).
__sysinfo_sudo() {
    if [[ "$__SYSINFO_HAS_SUDO" -eq 1 && $(id -u) -ne 0 ]]; then
        sudo "$@" 2>/dev/null
    else
        "$@" 2>/dev/null
    fi
}

# =============================================================================
# DUAL OUTPUT HELPERS
# =============================================================================

# Print a section header.
__sysinfo_section() {
    local title="$1"
    if [[ "$__SYSINFO_MODE" == "verbose" ]]; then
        __gash_section "$title"
    else
        echo "=== ${title} ==="
    fi
}

# Print a key-value pair. Skipped if value is empty.
__sysinfo_kv() {
    local key="$1" value="$2"
    [[ -z "$value" ]] && return 0
    if [[ "$__SYSINFO_MODE" == "verbose" ]]; then
        printf "  ${__GASH_BOLD_WHITE}%-24s${__GASH_COLOR_OFF} %s\n" "${key}:" "$value"
    else
        echo "${key}=${value}"
    fi
}

# Print a list item.
__sysinfo_item() {
    local text="$1"
    if [[ "$__SYSINFO_MODE" == "verbose" ]]; then
        echo -e "  ${__GASH_CYAN}-${__GASH_COLOR_OFF} ${text}"
    else
        echo "$text"
    fi
}

# Print a sub-header within a section.
__sysinfo_sub() {
    local title="$1"
    if [[ "$__SYSINFO_MODE" == "verbose" ]]; then
        echo -e "  ${__GASH_BOLD_YELLOW}${title}${__GASH_COLOR_OFF}"
    else
        echo "--- ${title}"
    fi
}

# Pipe raw command output (indented in verbose, raw in llm).
__sysinfo_raw() {
    if [[ "$__SYSINFO_MODE" == "verbose" ]]; then
        sed 's/^/    /'
    else
        cat
    fi
}

# =============================================================================
# SECTION 1: IDENTITY (Phase 1)
# =============================================================================

__sysinfo_collect_identity() {
    __sysinfo_section "IDENTITY"

    local pretty_name="" id="" version_id=""
    if [[ -f /etc/os-release ]]; then
        id=$(grep -oP '^ID=\K\w+' /etc/os-release 2>/dev/null || true)
        pretty_name=$(grep -oP '^PRETTY_NAME="\K[^"]+' /etc/os-release 2>/dev/null || true)
        version_id=$(grep -oP '^VERSION_ID="\K[^"]+' /etc/os-release 2>/dev/null || true)
    fi

    __sysinfo_kv "OS" "$pretty_name"
    __sysinfo_kv "OS ID" "$id"
    __sysinfo_kv "Version" "$version_id"
    __sysinfo_kv "Debian Version" "$(cat /etc/debian_version 2>/dev/null)"
    __sysinfo_kv "Kernel" "$(uname -r)"
    __sysinfo_kv "Arch" "$(uname -m)"
    __sysinfo_kv "Hostname" "$(hostname -f 2>/dev/null || hostname)"
    __sysinfo_kv "Machine-ID" "$(cat /etc/machine-id 2>/dev/null)"
    __sysinfo_kv "Uptime" "$(uptime -p 2>/dev/null)"

    # Virtualization detection
    if command -v systemd-detect-virt >/dev/null 2>&1; then
        local virt
        virt=$(systemd-detect-virt 2>/dev/null || echo "none")
        __sysinfo_kv "Virtualization" "$virt"
    fi

    # Container detection via /proc/1/environ (needs sudo)
    local container_info
    container_info=$(__sysinfo_sudo cat /proc/1/environ 2>/dev/null | tr '\0' '\n' | grep -iE "container|vz|lxc|docker" | head -1)
    if [[ -n "$container_info" ]]; then
        __sysinfo_kv "Container" "$container_info"
    elif [[ -d /proc/vz ]]; then
        __sysinfo_kv "Container" "OpenVZ"
    fi

    # WSL detection
    if [[ -f /etc/wsl.conf ]]; then
        __sysinfo_kv "WSL" "yes"
        [[ -f /etc/ubwsl-installed ]] && __sysinfo_kv "WSL Marker" "ubwsl-installed"
        if [[ "$__SYSINFO_MODE" == "llm" ]]; then
            echo "WSL_CONF:"
            cat /etc/wsl.conf 2>/dev/null
        fi
    fi

    # Cloud provider markers
    [[ -f /etc/hetzner-build ]] && __sysinfo_kv "Cloud" "Hetzner"
}

# =============================================================================
# SECTION 2: STORAGE (Phase 2)
# =============================================================================

__sysinfo_collect_storage() {
    __sysinfo_section "STORAGE"

    # fstab (non-comment lines)
    __sysinfo_sub "fstab"
    grep -Ev "^(#|$)" /etc/fstab 2>/dev/null | __sysinfo_raw

    # Encrypted volumes
    if [[ -f /etc/crypttab ]]; then
        __sysinfo_sub "crypttab"
        __sysinfo_sudo grep -v "^#" /etc/crypttab 2>/dev/null | __sysinfo_raw
    fi

    # LVM
    [[ -d /etc/lvm ]] && __sysinfo_kv "LVM" "present"

    # RAID
    if [[ -f /etc/mdadm/mdadm.conf ]]; then
        __sysinfo_sub "mdadm"
        grep "^ARRAY" /etc/mdadm/mdadm.conf 2>/dev/null | __sysinfo_raw
    fi

    # ZFS
    if [[ -d /etc/zfs ]]; then
        __sysinfo_kv "ZFS" "present"
        [[ -f /etc/hostid ]] && __sysinfo_kv "ZFS Host-ID" "present"
        if [[ -f /etc/zfs/zed.rc ]]; then
            __sysinfo_sub "zed.rc"
            grep -Ev "^(#|$)" /etc/zfs/zed.rc 2>/dev/null | __sysinfo_raw
        fi
    fi

    # Sanoid
    if [[ -f /etc/sanoid/sanoid.conf ]]; then
        __sysinfo_sub "sanoid"
        cat /etc/sanoid/sanoid.conf 2>/dev/null | __sysinfo_raw
    fi

    # SMART
    if [[ -f /etc/smartmontools/smartd.conf ]] || [[ -f /etc/smartd.conf ]]; then
        __sysinfo_sub "smartd"
        local smartf="/etc/smartd.conf"
        [[ -f /etc/smartmontools/smartd.conf ]] && smartf="/etc/smartmontools/smartd.conf"
        grep -Ev "^(#|$)" "$smartf" 2>/dev/null | __sysinfo_raw
    fi

    # hdparm
    [[ -f /etc/hdparm.conf ]] && __sysinfo_kv "hdparm" "present"

    # NFS
    if [[ -f /etc/exports ]]; then
        __sysinfo_sub "NFS exports"
        grep -Ev "^(#|$)" /etc/exports 2>/dev/null | __sysinfo_raw
    fi
    [[ -f /etc/nfs.conf ]] && __sysinfo_kv "NFS conf" "present"
}

# =============================================================================
# SECTION 3: SERVICES (Phases 3 + 4 + 20)
# =============================================================================

__sysinfo_collect_services() {
    __sysinfo_section "SERVICES"

    # Phase 3: Service detection by /etc/ directories
    __sysinfo_sub "Detected (by /etc/ dirs)"
    local -a service_dirs=(
        # Web
        "apache2" "nginx" "lighttpd"
        # PHP
        "php"
        # Database
        "mysql" "mariadb" "postgresql" "postgresql-common" "redis"
        # Mail
        "postfix" "dovecot" "exim4" "spamassassin" "postgrey"
        # Security
        "fail2ban" "crowdsec" "ufw" "firewalld" "iptables" "apparmor" "apparmor.d"
        # DNS
        "bind" "unbound" "avahi"
        # Containers
        "docker" "containerd" "nvidia-container-runtime" "nvidia-container-toolkit"
        # Storage/NAS
        "openmediavault" "samba" "openvpn" "wireguard"
        # Panels
        "webmin" "usermin"
        # Monitoring
        "monit" "collectd" "snmp" "sysstat"
        # ZFS/Storage
        "zfs" "sanoid" "smartmontools"
        # FTP
        "proftpd" "vsftpd"
        # SSL
        "letsencrypt" "certbot"
        # Time
        "chrony" "ntp"
        # Legacy/Other
        "salt" "runit" "jailkit" "clamav" "logcheck" "awstats"
        # Virtualization
        "qemu" "libvirt"
        # Cloud/Network
        "cloud" "netplan" "NetworkManager" "wpa_supplicant"
        # Enterprise
        "landscape" "ubuntu-advantage" "update-manager" "PackageKit" "apticron"
    )

    for dir in "${service_dirs[@]}"; do
        [[ -d "/etc/${dir}" ]] && __sysinfo_item "/etc/${dir}/"
    done

    # Standalone file markers
    [[ -f /etc/cufile.json ]] && __sysinfo_item "NVIDIA cuFile GDS: present"
    [[ -f /etc/dkim-domains.txt ]] && __sysinfo_item "OpenDKIM: present"
    [[ -f /etc/sasldb2 ]] && __sysinfo_item "SASL: present"
    [[ -f /etc/gprofng.rc ]] && __sysinfo_item "GPROFNG: present"
    [[ -d /etc/boot.d ]] && __sysinfo_item "boot.d hooks: $(ls /etc/boot.d/ 2>/dev/null | paste -sd, -)"
    [[ -d /etc/shutdown.d ]] && __sysinfo_item "shutdown.d hooks: $(ls /etc/shutdown.d/ 2>/dev/null | paste -sd, -)"
    if [[ -f /etc/default/saslauthd ]]; then
        __sysinfo_sub "saslauthd"
        grep -Ev "^(#|$)" /etc/default/saslauthd 2>/dev/null | __sysinfo_raw
    fi

    # Legacy backups
    local legacy_orgs
    legacy_orgs=$(ls /etc/*.org 2>/dev/null)
    [[ -n "$legacy_orgs" ]] && __sysinfo_item "Legacy .org backups found"

    # Phase 4: Running services + ports + timers
    if command -v systemctl >/dev/null 2>&1; then
        __sysinfo_sub "Running services"
        systemctl list-units --type=service --state=running --no-pager --no-legend 2>/dev/null | awk '{print $1}' | __sysinfo_raw

        __sysinfo_sub "Timers"
        systemctl list-timers --no-pager --no-legend 2>/dev/null | awk '{print $NF}' | __sysinfo_raw
    fi

    __sysinfo_sub "Listening ports"
    ss -tlnp 2>/dev/null | tail -n +2 | __sysinfo_raw

    # Phase 20: Custom systemd units
    if [[ -d /etc/systemd/system ]]; then
        __sysinfo_sub "Custom systemd units"
        find /etc/systemd/system -maxdepth 1 \( -name "*.service" -o -name "*.timer" -o -name "*.mount" -o -name "*.socket" \) 2>/dev/null | __sysinfo_raw
        find /etc/systemd/system -name "override.conf" 2>/dev/null | __sysinfo_raw

        # Include key config lines for non-symlink custom units (gives AI actual data)
        local __unit_file
        for __unit_file in /etc/systemd/system/*.service /etc/systemd/system/*.timer; do
            [[ -f "$__unit_file" ]] || continue
            [[ -L "$__unit_file" ]] && continue
            __sysinfo_sub "Unit: $(basename "$__unit_file")"
            __sysinfo_sudo cat "$__unit_file" 2>/dev/null | head -40 | __sysinfo_raw
        done

        __sysinfo_sub "multi-user.target.wants"
        ls /etc/systemd/system/multi-user.target.wants/ 2>/dev/null | __sysinfo_raw

        __sysinfo_sub "timers.target.wants"
        ls /etc/systemd/system/timers.target.wants/ 2>/dev/null | __sysinfo_raw
    fi
}

# =============================================================================
# SECTION 4: AUTH (Phase 5)
# =============================================================================

__sysinfo_collect_auth() {
    __sysinfo_section "AUTH"

    # Users with UID >= 1000
    __sysinfo_sub "Users"
    awk -F: '$3>=1000 && $3<65534{print $1":"$3":"$6":"$7}' /etc/passwd 2>/dev/null | __sysinfo_raw

    # Sudoers (needs sudo)
    __sysinfo_sub "Sudo"
    __sysinfo_sudo grep -Ev "^(#|$)" /etc/sudoers 2>/dev/null | head -20 | __sysinfo_raw
    __sysinfo_sudo find /etc/sudoers.d -type f 2>/dev/null | while IFS= read -r f; do
        __sysinfo_item "--- ${f}:"
        __sysinfo_sudo grep -Ev "^(#|$)" "$f" 2>/dev/null | __sysinfo_raw
    done

    # SSH config (may need sudo)
    __sysinfo_sub "SSH"
    __sysinfo_sudo grep -E "^(Port|PermitRootLogin|PasswordAuthentication|PubkeyAuthentication|AuthorizedKeysFile|AllowUsers|AllowGroups|MaxAuthTries|ClientAliveInterval)" /etc/ssh/sshd_config 2>/dev/null | __sysinfo_raw
    __sysinfo_sudo find /etc/ssh/sshd_config.d -name "*.conf" 2>/dev/null | while IFS= read -r f; do
        __sysinfo_item "--- ${f}:"
        __sysinfo_sudo grep -Ev "^(#|$)" "$f" 2>/dev/null | __sysinfo_raw
    done
}

# =============================================================================
# SECTION 5: NETWORK (Phase 6)
# =============================================================================

__sysinfo_collect_network() {
    __sysinfo_section "NETWORK"

    # resolv.conf
    __sysinfo_sub "resolv.conf"
    grep -v "^#" /etc/resolv.conf 2>/dev/null | __sysinfo_raw

    # hosts
    __sysinfo_sub "hosts"
    grep -Ev "^(#|$)" /etc/hosts 2>/dev/null | __sysinfo_raw

    # interfaces (legacy)
    if [[ -f /etc/network/interfaces ]]; then
        __sysinfo_sub "interfaces"
        grep -Ev "^(#|$)" /etc/network/interfaces 2>/dev/null | __sysinfo_raw
    fi

    # netplan
    if [[ -d /etc/netplan ]]; then
        __sysinfo_sub "netplan"
        find /etc/netplan -name "*.yaml" -exec cat {} \; 2>/dev/null | __sysinfo_raw
    fi

    # DHCP
    if [[ -f /etc/dhcpcd.conf ]]; then
        __sysinfo_sub "dhcpcd"
        grep -Ev "^(#|$)" /etc/dhcpcd.conf 2>/dev/null | __sysinfo_raw
    fi
}

# =============================================================================
# SECTION 6: SECURITY (Phase 7)
# =============================================================================

__sysinfo_collect_security() {
    __sysinfo_section "SECURITY"

    # Fail2ban
    if [[ -d /etc/fail2ban ]]; then
        __sysinfo_sub "fail2ban"
        if [[ -f /etc/fail2ban/jail.local ]]; then
            __sysinfo_sudo grep -Ev "^(#|;|$)" /etc/fail2ban/jail.local 2>/dev/null | __sysinfo_raw
        fi
        __sysinfo_sudo find /etc/fail2ban/jail.d -name "*.conf" 2>/dev/null | while IFS= read -r f; do
            __sysinfo_item "--- ${f}:"
            __sysinfo_sudo grep -Ev "^(#|;|$)" "$f" 2>/dev/null | __sysinfo_raw
        done
    fi

    # CrowdSec
    if [[ -d /etc/crowdsec ]]; then
        __sysinfo_sub "crowdsec"
        [[ -f /etc/crowdsec/config.yaml ]] && __sysinfo_kv "CrowdSec" "installed"
        if [[ -f /etc/crowdsec/acquis.yaml ]]; then
            grep "source:" /etc/crowdsec/acquis.yaml 2>/dev/null | __sysinfo_raw
        fi
        ls /etc/crowdsec/hub/scenarios/ 2>/dev/null | head -20 | __sysinfo_raw
    fi

    # UFW
    if [[ -d /etc/ufw ]]; then
        __sysinfo_sub "ufw"
        [[ -f /etc/ufw/ufw.conf ]] && grep -Ev "^(#|$)" /etc/ufw/ufw.conf 2>/dev/null | __sysinfo_raw
    fi

    # firewalld
    [[ -d /etc/firewalld ]] && __sysinfo_kv "firewalld" "present"

    # iptables
    [[ -d /etc/iptables ]] && __sysinfo_kv "iptables" "present"

    # AppArmor
    if [[ -d /etc/apparmor.d ]]; then
        __sysinfo_kv "AppArmor" "present"
    fi
}

# =============================================================================
# SECTION 7: WEBSTACK (Phases 8 + 9 + 10)
# =============================================================================

__sysinfo_collect_webstack() {
    __sysinfo_section "WEBSTACK"

    # Phase 8: Apache
    if [[ -d /etc/apache2 ]]; then
        __sysinfo_sub "Apache"
        if [[ -f /etc/apache2/ports.conf ]]; then
            __sysinfo_item "ports.conf:"
            grep -Ev "^(#|$)" /etc/apache2/ports.conf 2>/dev/null | __sysinfo_raw
        fi
        if [[ -d /etc/apache2/sites-enabled ]]; then
            __sysinfo_item "sites-enabled:"
            ls /etc/apache2/sites-enabled/ 2>/dev/null | __sysinfo_raw
            find /etc/apache2/sites-enabled -name "*.conf" -exec grep -E "ServerName|ServerAlias|DocumentRoot|ProxyPass |ProxyPassReverse" {} \; 2>/dev/null | __sysinfo_raw
        fi
        if [[ -d /etc/apache2/mods-enabled ]]; then
            __sysinfo_item "mods-enabled:"
            ls /etc/apache2/mods-enabled/*.load 2>/dev/null | xargs -I{} basename {} .load | __sysinfo_raw
        fi
    fi

    # Phase 8: Nginx
    if [[ -d /etc/nginx ]]; then
        __sysinfo_sub "Nginx"
        if [[ -d /etc/nginx/sites-enabled ]]; then
            __sysinfo_item "sites-enabled:"
            ls /etc/nginx/sites-enabled/ 2>/dev/null | __sysinfo_raw
            find /etc/nginx/sites-enabled -type f -exec grep -E "server_name|root |proxy_pass" {} \; 2>/dev/null | __sysinfo_raw
        fi
    fi

    # Phase 9: PHP
    if [[ -d /etc/php ]]; then
        __sysinfo_sub "PHP"
        __sysinfo_item "Versions:"
        find /etc/php -maxdepth 1 -mindepth 1 -type d 2>/dev/null | sort | __sysinfo_raw

        __sysinfo_item "FPM pools:"
        find /etc/php/*/fpm/pool.d -name "*.conf" 2>/dev/null | sort | __sysinfo_raw
        find /etc/php/*/fpm/pool.d -name "*.conf" -exec grep -E '^\[|^(user|listen|pm |pm\.max_children|php_admin_value\[memory_limit\])' {} \; 2>/dev/null | __sysinfo_raw

        __sysinfo_item "Modules:"
        find /etc/php -path "*/mods-available/*.ini" -exec basename {} .ini \; 2>/dev/null | sort -u | paste -sd, - | __sysinfo_raw
    fi

    # Phase 10: MySQL/MariaDB
    if [[ -d /etc/mysql ]]; then
        __sysinfo_sub "MySQL/MariaDB"
        if [[ -d /etc/mysql/mariadb.conf.d ]]; then
            __sysinfo_item "mariadb.conf.d:"
            find /etc/mysql/mariadb.conf.d -name "*.cnf" 2>/dev/null | sort | __sysinfo_raw
            find /etc/mysql/mariadb.conf.d -name "*.cnf" -exec grep -Ev '^(#|$|\[)' {} \; 2>/dev/null | grep -E "(bind|port|max_connections|innodb_buffer_pool_size|innodb_log_file_size|innodb_flush|default_storage|character.set|log_bin|disable_log_bin|galera)" 2>/dev/null | __sysinfo_raw
        fi
        if [[ -d /etc/mysql/conf.d ]]; then
            __sysinfo_item "conf.d:"
            find /etc/mysql/conf.d -name "*.cnf" -exec grep -Ev "^(#|$)" {} \; 2>/dev/null | __sysinfo_raw
        fi
    fi

    # Phase 10: PostgreSQL
    if [[ -d /etc/postgresql ]]; then
        __sysinfo_sub "PostgreSQL"
        ls /etc/postgresql/*/main/*.conf 2>/dev/null | __sysinfo_raw
    fi
}

# =============================================================================
# SECTION 8: MAIL (Phases 11 + 12)
# =============================================================================

__sysinfo_collect_mail() {
    __sysinfo_section "MAIL"

    # Phase 11: Postfix
    if [[ -d /etc/postfix ]]; then
        __sysinfo_sub "Postfix"
        if [[ -f /etc/postfix/main.cf ]]; then
            grep -Ev "^(#|$)" /etc/postfix/main.cf 2>/dev/null | grep -E "(myhostname|mydomain|mydestination|mynetworks|inet_interfaces|relayhost|smtpd_tls|smtpd_sasl|smtp_tls|smtpd_milters|virtual_alias|content_filter|home_mailbox|mailbox_command)" | __sysinfo_raw
        fi
    fi

    # Phase 11: Dovecot
    if [[ -d /etc/dovecot ]]; then
        __sysinfo_sub "Dovecot"
        if [[ -f /etc/dovecot/dovecot.conf ]]; then
            grep -Ev "^(#|$)" /etc/dovecot/dovecot.conf 2>/dev/null | head -30 | __sysinfo_raw
        fi
        find /etc/dovecot/conf.d -name "*.conf" -exec grep -E "^(protocols|ssl_cert|ssl_key|mail_location|auth_mechanisms)" {} \; 2>/dev/null | __sysinfo_raw
    fi

    # Phase 11: Exim4
    if [[ -d /etc/exim4 ]]; then
        __sysinfo_sub "Exim4"
        if [[ -f /etc/exim4/update-exim4.conf.conf ]]; then
            grep -Ev "^(#|$)" /etc/exim4/update-exim4.conf.conf 2>/dev/null | __sysinfo_raw
        fi
    fi

    # Phase 11: SpamAssassin
    if [[ -d /etc/spamassassin ]]; then
        __sysinfo_sub "SpamAssassin"
        if [[ -f /etc/spamassassin/local.cf ]]; then
            grep -Ev "^(#|$)" /etc/spamassassin/local.cf 2>/dev/null | __sysinfo_raw
        fi
        [[ -d /etc/spamassassin/razor ]] && __sysinfo_item "Razor: configured"
        [[ -d /etc/spamassassin/pyzor ]] && __sysinfo_item "Pyzor: configured"
    fi

    # Phase 11: OpenDKIM
    if [[ -f /etc/default/opendkim ]]; then
        __sysinfo_sub "OpenDKIM"
        grep -Ev "^(#|$)" /etc/default/opendkim 2>/dev/null | __sysinfo_raw
    fi

    # Phase 12: BIND
    if [[ -d /etc/bind ]]; then
        __sysinfo_sub "BIND"
        if [[ -f /etc/bind/named.conf.local ]]; then
            grep -E "^zone" /etc/bind/named.conf.local 2>/dev/null | __sysinfo_raw
        fi
        if [[ -f /etc/bind/named.conf.options ]]; then
            grep -Ev '^(//|$)' /etc/bind/named.conf.options 2>/dev/null | head -15 | __sysinfo_raw
        fi
    fi

    # Phase 12: Unbound
    if [[ -d /etc/unbound ]]; then
        __sysinfo_sub "Unbound"
        find /etc/unbound -name "*.conf" -exec cat {} \; 2>/dev/null | __sysinfo_raw
    fi
}

# =============================================================================
# SECTION 9: INFRA (Phases 13 + 14 + 15 + 16 + 17 + 18)
# =============================================================================

__sysinfo_collect_infra() {
    __sysinfo_section "INFRA"

    # Phase 13: FTP
    if [[ -d /etc/proftpd ]]; then
        __sysinfo_sub "ProFTPD"
        if [[ -f /etc/proftpd/proftpd.conf ]]; then
            grep -Ev "^(#|$)" /etc/proftpd/proftpd.conf 2>/dev/null | head -20 | __sysinfo_raw
        fi
        find /etc/proftpd/conf.d -name "*.conf" -exec grep -Ev "^(#|$)" {} \; 2>/dev/null | head -30 | __sysinfo_raw
    fi
    if [[ -d /etc/vsftpd ]] || [[ -f /etc/vsftpd.conf ]]; then
        __sysinfo_sub "vsftpd"
        local vsf="/etc/vsftpd.conf"
        [[ -f /etc/vsftpd/vsftpd.conf ]] && vsf="/etc/vsftpd/vsftpd.conf"
        grep -Ev "^(#|$)" "$vsf" 2>/dev/null | head -15 | __sysinfo_raw
    fi

    # Phase 14: Samba
    if [[ -d /etc/samba ]]; then
        __sysinfo_sub "Samba"
        if [[ -f /etc/samba/smb.conf ]]; then
            grep -Ev "^(#|;|$)" /etc/samba/smb.conf 2>/dev/null | __sysinfo_raw
        fi
    fi

    # Phase 14: OpenMediaVault
    if [[ -d /etc/openmediavault ]]; then
        __sysinfo_sub "OpenMediaVault"
        __sysinfo_kv "OMV" "installed"
        if [[ -f /etc/openmediavault/config.xml ]]; then
            __sysinfo_kv "config.xml" "$(wc -c < /etc/openmediavault/config.xml 2>/dev/null) bytes"
        fi
    fi

    # Phase 15: OpenVPN
    if [[ -d /etc/openvpn ]]; then
        __sysinfo_sub "OpenVPN"
        find /etc/openvpn -name "*.conf" -o -name "*.ovpn" 2>/dev/null | head -20 | __sysinfo_raw
        [[ -f /etc/openvpn/mullvad.conf ]] && __sysinfo_item "Mullvad VPN: configured"
    fi

    # Phase 15: WireGuard
    if [[ -d /etc/wireguard ]]; then
        __sysinfo_sub "WireGuard"
        ls /etc/wireguard/*.conf 2>/dev/null | __sysinfo_raw
    fi

    # Phase 16: Docker
    if [[ -d /etc/docker ]]; then
        __sysinfo_sub "Docker"
        if [[ -f /etc/docker/daemon.json ]]; then
            cat /etc/docker/daemon.json 2>/dev/null | __sysinfo_raw
        fi
    fi
    [[ -d /etc/containerd ]] && __sysinfo_kv "containerd" "present"
    [[ -d /etc/nvidia-container-runtime ]] && __sysinfo_kv "NVIDIA Container Runtime" "present"

    # Phase 17: Monitoring
    if [[ -d /etc/collectd ]]; then
        __sysinfo_sub "Collectd"
        find /etc/collectd/collectd.conf.d -name "*.conf" 2>/dev/null | xargs -I{} basename {} | __sysinfo_raw
    fi
    if [[ -d /etc/monit ]]; then
        __sysinfo_sub "Monit"
        ls /etc/monit/conf.d/ 2>/dev/null | __sysinfo_raw
        ls /etc/monit/conf-available/ 2>/dev/null | __sysinfo_raw
    fi
    [[ -d /etc/snmp ]] && __sysinfo_kv "SNMP" "present"
    [[ -d /etc/sysstat ]] && __sysinfo_kv "sysstat" "present"

    # Phase 18: Control Panels
    if [[ -d /etc/webmin ]]; then
        __sysinfo_sub "Webmin"
        ls /etc/webmin/ 2>/dev/null | head -20 | __sysinfo_raw
        [[ -d /etc/webmin/virtual-server ]] && __sysinfo_item "Virtualmin: installed"
    fi
    [[ -d /etc/usermin ]] && __sysinfo_kv "Usermin" "installed"
}

# =============================================================================
# SECTION 10: SYSTEM (Phases 19 + 21 + 22 + 23 + 24 + 25 + 26)
# =============================================================================

__sysinfo_collect_system() {
    __sysinfo_section "SYSTEM"

    # Phase 19: Cron
    __sysinfo_sub "Cron"
    grep -Ev "^(#|$)" /etc/crontab 2>/dev/null | __sysinfo_raw

    __sysinfo_item "cron.d:"
    ls /etc/cron.d/ 2>/dev/null | __sysinfo_raw
    __sysinfo_item "cron.daily:"
    ls /etc/cron.daily/ 2>/dev/null | __sysinfo_raw
    __sysinfo_item "cron.hourly:"
    ls /etc/cron.hourly/ 2>/dev/null | __sysinfo_raw
    __sysinfo_item "cron.weekly:"
    ls /etc/cron.weekly/ 2>/dev/null | __sysinfo_raw
    __sysinfo_item "cron.monthly:"
    ls /etc/cron.monthly/ 2>/dev/null | __sysinfo_raw

    # Phase 21: SSL/TLS
    if [[ -d /etc/letsencrypt ]]; then
        __sysinfo_sub "Let's Encrypt"
        __sysinfo_sudo ls /etc/letsencrypt/live/ 2>/dev/null | __sysinfo_raw
        __sysinfo_sudo find /etc/letsencrypt/renewal -name "*.conf" 2>/dev/null | xargs -I{} basename {} .conf | __sysinfo_raw
        if [[ -f /etc/letsencrypt/cli.ini ]]; then
            grep -Ev "^(#|$)" /etc/letsencrypt/cli.ini 2>/dev/null | __sysinfo_raw
        fi
    fi
    if [[ -d /etc/ssl/virtualmin ]]; then
        __sysinfo_kv "Virtualmin SSL" "$(ls /etc/ssl/virtualmin/ 2>/dev/null | wc -l) domains"
    fi
    __sysinfo_sudo ls /etc/ssl/private/*.pem /etc/ssl/private/*.key /etc/ssl/private/*.crt 2>/dev/null | head -10 | __sysinfo_raw

    # Phase 22: Kernel
    __sysinfo_sub "Kernel tuning"
    if [[ -d /etc/sysctl.d ]]; then
        find /etc/sysctl.d -name "*.conf" 2>/dev/null | while IFS= read -r f; do
            __sysinfo_item "--- ${f}:"
            grep -Ev "^(#|$)" "$f" 2>/dev/null | __sysinfo_raw
        done
    fi
    grep -Ev "^(#|$)" /etc/sysctl.conf 2>/dev/null | __sysinfo_raw

    __sysinfo_sub "GRUB"
    if [[ -f /etc/default/grub ]]; then
        grep -Ev "^(#|$)" /etc/default/grub 2>/dev/null | __sysinfo_raw
    fi

    __sysinfo_sub "modprobe"
    find /etc/modprobe.d -name "*.conf" 2>/dev/null | while IFS= read -r f; do
        __sysinfo_item "--- ${f}:"
        cat "$f" 2>/dev/null | __sysinfo_raw
    done

    # Phase 23: Logrotate
    __sysinfo_sub "Logrotate"
    ls /etc/logrotate.d/ 2>/dev/null | __sysinfo_raw

    # Phase 24: Defaults
    __sysinfo_sub "Service defaults"
    ls /etc/default/ 2>/dev/null | __sysinfo_raw

    # Phase 25: Profile + Environment
    __sysinfo_sub "Profile"
    ls /etc/profile.d/ 2>/dev/null | __sysinfo_raw
    cat /etc/environment 2>/dev/null | __sysinfo_raw
    __sysinfo_item "update-motd.d:"
    ls /etc/update-motd.d/ 2>/dev/null | __sysinfo_raw

    # Phase 26: Cloud + Misc
    if [[ -d /etc/cloud ]]; then
        __sysinfo_sub "Cloud"
        if [[ -f /etc/cloud/cloud.cfg ]]; then
            grep -E "^(distro|system_info)" /etc/cloud/cloud.cfg 2>/dev/null | __sysinfo_raw
        fi
    fi

    # Chrony
    if [[ -d /etc/chrony ]]; then
        __sysinfo_sub "Chrony"
        if [[ -f /etc/chrony/chrony.conf ]]; then
            grep -Ev "^(#|$)" /etc/chrony/chrony.conf 2>/dev/null | head -10 | __sysinfo_raw
        fi
    fi

    # Etckeeper
    if [[ -d /etc/.git ]]; then
        __sysinfo_kv "etckeeper" "git tracked"
    fi
    [[ -f /etc/.etckeeper ]] && __sysinfo_kv "etckeeper metadata" "present"
}

# =============================================================================
# DEEP COLLECTORS (for interactive drill-down)
# =============================================================================
# These functions collect FULL configs for AI drill-down analysis.
# Each outputs LLM-format data (=== SECTION ===, --- sub, raw lines).
# Called by ai_sysinfo interactive loop, never directly by users.

# Deep: ALL detected services — full configs for every service from prompt.txt.
# Covers phases 3-4, 8-18, 20.
__sysinfo_deep_services() {
    echo "=== SERVICES DEEP ==="

    # --- Web Servers ---
    if [[ -d /etc/apache2 ]]; then
        echo "--- Apache2"
        [[ -f /etc/apache2/ports.conf ]] && {
            echo "--- ports.conf"
            grep -Ev "^(#|$)" /etc/apache2/ports.conf 2>/dev/null
        }
        echo "--- mods-enabled"
        ls /etc/apache2/mods-enabled/*.load 2>/dev/null | xargs -I{} basename {} .load
        if [[ -d /etc/apache2/sites-enabled ]]; then
            for vhost in /etc/apache2/sites-enabled/*.conf; do
                [[ -f "$vhost" ]] || continue
                echo "--- vhost: $(basename "$vhost")"
                grep -Ev "^(#|$|[[:space:]]*#)" "$vhost" 2>/dev/null | head -80
            done
        fi
    fi

    if [[ -d /etc/nginx ]]; then
        echo "--- Nginx"
        if [[ -d /etc/nginx/sites-enabled ]]; then
            for site in /etc/nginx/sites-enabled/*; do
                [[ -f "$site" ]] || continue
                echo "--- site: $(basename "$site")"
                grep -Ev "^(#|$|[[:space:]]*#)" "$site" 2>/dev/null | head -80
            done
        fi
    fi

    if [[ -d /etc/lighttpd ]]; then
        echo "--- Lighttpd"
        [[ -f /etc/lighttpd/lighttpd.conf ]] && grep -Ev "^(#|$)" /etc/lighttpd/lighttpd.conf 2>/dev/null | head -50
        ls /etc/lighttpd/conf-enabled/ 2>/dev/null
    fi

    # --- PHP ---
    if [[ -d /etc/php ]]; then
        for phpver in /etc/php/*/; do
            [[ -d "$phpver" ]] || continue
            local ver
            ver=$(basename "$phpver")
            echo "--- PHP ${ver}"

            # FPM pools
            if [[ -d "${phpver}fpm/pool.d" ]]; then
                for pool in "${phpver}fpm/pool.d/"*.conf; do
                    [[ -f "$pool" ]] || continue
                    echo "--- pool: $(basename "$pool")"
                    grep -Ev "^(;|$)" "$pool" 2>/dev/null | head -50
                done
            fi

            # CLI php.ini key settings
            if [[ -f "${phpver}cli/php.ini" ]]; then
                echo "--- cli/php.ini (key)"
                grep -E "^(memory_limit|upload_max|post_max|max_execution|max_input|date.timezone|error_reporting|display_errors|opcache)" "${phpver}cli/php.ini" 2>/dev/null | head -10
            fi

            # Modules
            if [[ -d "${phpver}mods-available" ]]; then
                echo "--- modules"
                ls "${phpver}mods-available/"*.ini 2>/dev/null | xargs -I{} basename {} .ini | paste -sd, -
            fi
        done
    fi

    # --- Databases ---
    if [[ -d /etc/mysql ]]; then
        echo "--- MySQL/MariaDB"
        if [[ -d /etc/mysql/mariadb.conf.d ]]; then
            for cnf in /etc/mysql/mariadb.conf.d/*.cnf; do
                [[ -f "$cnf" ]] || continue
                echo "--- $(basename "$cnf")"
                grep -Ev "^(#|$)" "$cnf" 2>/dev/null
            done
        fi
        if [[ -d /etc/mysql/conf.d ]]; then
            for cnf in /etc/mysql/conf.d/*.cnf; do
                [[ -f "$cnf" ]] || continue
                echo "--- $(basename "$cnf")"
                grep -Ev "^(#|$)" "$cnf" 2>/dev/null
            done
        fi
    fi

    if [[ -d /etc/postgresql ]]; then
        echo "--- PostgreSQL"
        for pgver in /etc/postgresql/*/; do
            [[ -d "$pgver" ]] || continue
            echo "--- version: $(basename "$pgver")"
            [[ -f "${pgver}main/postgresql.conf" ]] && grep -Ev "^(#|$)" "${pgver}main/postgresql.conf" 2>/dev/null | head -50
            [[ -f "${pgver}main/pg_hba.conf" ]] && {
                echo "--- pg_hba.conf"
                grep -Ev "^(#|$)" "${pgver}main/pg_hba.conf" 2>/dev/null | head -50
            }
        done
    fi

    if [[ -f /etc/redis/redis.conf ]]; then
        echo "--- Redis"
        grep -Ev "^(#|$)" /etc/redis/redis.conf 2>/dev/null | head -50
    fi

    # --- Mail ---
    if [[ -d /etc/postfix ]]; then
        echo "--- Postfix"
        [[ -f /etc/postfix/main.cf ]] && {
            echo "--- main.cf"
            grep -Ev "^(#|$)" /etc/postfix/main.cf 2>/dev/null | head -50
        }
        [[ -f /etc/postfix/master.cf ]] && {
            echo "--- master.cf"
            grep -Ev "^(#|$)" /etc/postfix/master.cf 2>/dev/null | head -50
        }
    fi

    if [[ -d /etc/dovecot ]]; then
        echo "--- Dovecot"
        [[ -f /etc/dovecot/dovecot.conf ]] && grep -Ev "^(#|$)" /etc/dovecot/dovecot.conf 2>/dev/null | head -30
        find /etc/dovecot/conf.d -name "*.conf" 2>/dev/null | sort | while IFS= read -r f; do
            local content
            content=$(grep -Ev "^(#|$)" "$f" 2>/dev/null)
            if [[ -n "$content" ]]; then
                echo "--- $(basename "$f")"
                echo "$content" | head -30
            fi
        done
    fi

    if [[ -d /etc/exim4 ]]; then
        echo "--- Exim4"
        [[ -f /etc/exim4/update-exim4.conf.conf ]] && grep -Ev "^(#|$)" /etc/exim4/update-exim4.conf.conf 2>/dev/null | head -30
    fi

    if [[ -d /etc/spamassassin ]]; then
        echo "--- SpamAssassin"
        [[ -f /etc/spamassassin/local.cf ]] && grep -Ev "^(#|$)" /etc/spamassassin/local.cf 2>/dev/null | head -30
        [[ -d /etc/spamassassin/razor ]] && echo "Razor=configured"
        [[ -d /etc/spamassassin/pyzor ]] && echo "Pyzor=configured"
    fi

    if [[ -f /etc/default/opendkim ]] || [[ -d /etc/opendkim ]]; then
        echo "--- OpenDKIM"
        [[ -f /etc/default/opendkim ]] && grep -Ev "^(#|$)" /etc/default/opendkim 2>/dev/null
        [[ -f /etc/opendkim/TrustedHosts ]] && { echo "--- TrustedHosts"; cat /etc/opendkim/TrustedHosts 2>/dev/null | head -20; }
        [[ -f /etc/opendkim/KeyTable ]] && { echo "--- KeyTable"; cat /etc/opendkim/KeyTable 2>/dev/null | head -20; }
        [[ -f /etc/opendkim/SigningTable ]] && { echo "--- SigningTable"; cat /etc/opendkim/SigningTable 2>/dev/null | head -20; }
    fi

    # --- DNS ---
    if [[ -d /etc/bind ]]; then
        echo "--- BIND"
        [[ -f /etc/bind/named.conf.local ]] && { echo "--- named.conf.local"; cat /etc/bind/named.conf.local 2>/dev/null | head -50; }
        [[ -f /etc/bind/named.conf.options ]] && { echo "--- named.conf.options"; grep -Ev "^(//|$)" /etc/bind/named.conf.options 2>/dev/null | head -50; }
    fi

    if [[ -d /etc/unbound ]]; then
        echo "--- Unbound"
        find /etc/unbound -name "*.conf" -exec cat {} \; 2>/dev/null | head -50
    fi

    if [[ -d /etc/avahi ]]; then
        echo "--- Avahi"
        [[ -f /etc/avahi/avahi-daemon.conf ]] && grep -Ev "^(#|$)" /etc/avahi/avahi-daemon.conf 2>/dev/null | head -20
    fi

    # --- FTP ---
    if [[ -d /etc/proftpd ]]; then
        echo "--- ProFTPD"
        [[ -f /etc/proftpd/proftpd.conf ]] && grep -Ev "^(#|$)" /etc/proftpd/proftpd.conf 2>/dev/null | head -50
        find /etc/proftpd/conf.d -name "*.conf" -exec grep -Ev "^(#|$)" {} \; 2>/dev/null | head -30
    fi

    if [[ -d /etc/vsftpd ]] || [[ -f /etc/vsftpd.conf ]]; then
        echo "--- vsftpd"
        local vsf="/etc/vsftpd.conf"
        [[ -f /etc/vsftpd/vsftpd.conf ]] && vsf="/etc/vsftpd/vsftpd.conf"
        grep -Ev "^(#|$)" "$vsf" 2>/dev/null | head -30
    fi

    # --- NAS / File Sharing ---
    if [[ -d /etc/samba ]]; then
        echo "--- Samba"
        [[ -f /etc/samba/smb.conf ]] && grep -Ev "^(#|;|$)" /etc/samba/smb.conf 2>/dev/null | head -50
    fi

    if [[ -d /etc/openmediavault ]]; then
        echo "--- OpenMediaVault"
        echo "OMV=installed"
        [[ -f /etc/openmediavault/config.xml ]] && echo "config.xml=$(wc -c < /etc/openmediavault/config.xml 2>/dev/null) bytes"
    fi

    if [[ -f /etc/exports ]]; then
        echo "--- NFS"
        cat /etc/exports 2>/dev/null | head -30
        [[ -f /etc/nfs.conf ]] && grep -Ev "^(#|$)" /etc/nfs.conf 2>/dev/null | head -20
    fi

    # --- VPN ---
    if [[ -d /etc/openvpn ]]; then
        echo "--- OpenVPN"
        find /etc/openvpn -name "*.conf" -o -name "*.ovpn" 2>/dev/null | while IFS= read -r f; do
            echo "--- $(basename "$f")"
            grep -Ev "^(#|$)" "$f" 2>/dev/null | head -30
        done
        [[ -f /etc/openvpn/mullvad.conf ]] && echo "Mullvad=configured"
    fi

    if [[ -d /etc/wireguard ]]; then
        echo "--- WireGuard"
        for wg in /etc/wireguard/*.conf; do
            [[ -f "$wg" ]] || continue
            echo "--- $(basename "$wg")"
            # Redact private keys
            grep -Ev "^(#|$)" "$wg" 2>/dev/null | sed 's/PrivateKey = .*/PrivateKey = [REDACTED]/' | head -20
        done
    fi

    # --- Containers ---
    if [[ -d /etc/docker ]]; then
        echo "--- Docker"
        [[ -f /etc/docker/daemon.json ]] && cat /etc/docker/daemon.json 2>/dev/null
        command -v docker >/dev/null 2>&1 && docker info 2>/dev/null | head -50
    fi

    if [[ -d /etc/containerd ]]; then
        echo "--- containerd"
        [[ -f /etc/containerd/config.toml ]] && grep -Ev "^(#|$)" /etc/containerd/config.toml 2>/dev/null | head -30
    fi

    [[ -d /etc/nvidia-container-runtime ]] && {
        echo "--- NVIDIA Container Runtime"
        find /etc/nvidia-container-runtime -type f 2>/dev/null | while IFS= read -r f; do
            echo "--- $(basename "$f")"
            cat "$f" 2>/dev/null | head -20
        done
    }

    # --- Monitoring ---
    if [[ -d /etc/collectd ]]; then
        echo "--- Collectd"
        [[ -f /etc/collectd/collectd.conf ]] && grep -Ev "^(#|$)" /etc/collectd/collectd.conf 2>/dev/null | head -30
        find /etc/collectd/collectd.conf.d -name "*.conf" 2>/dev/null | while IFS= read -r f; do
            echo "--- $(basename "$f")"
            cat "$f" 2>/dev/null | head -20
        done
    fi

    if [[ -d /etc/monit ]]; then
        echo "--- Monit"
        for d in conf.d conf-available; do
            [[ -d "/etc/monit/$d" ]] || continue
            for f in "/etc/monit/$d/"*; do
                [[ -f "$f" ]] || continue
                echo "--- $d/$(basename "$f")"
                cat "$f" 2>/dev/null | head -20
            done
        done
    fi

    if [[ -d /etc/snmp ]]; then
        echo "--- SNMP"
        [[ -f /etc/snmp/snmpd.conf ]] && grep -Ev "^(#|$)" /etc/snmp/snmpd.conf 2>/dev/null | head -30
    fi

    [[ -d /etc/sysstat ]] && {
        echo "--- sysstat"
        [[ -f /etc/default/sysstat ]] && cat /etc/default/sysstat 2>/dev/null | head -10
    }

    if [[ -f /etc/smartd.conf ]] || [[ -f /etc/smartmontools/smartd.conf ]]; then
        echo "--- smartmontools"
        local smartf="/etc/smartd.conf"
        [[ -f /etc/smartmontools/smartd.conf ]] && smartf="/etc/smartmontools/smartd.conf"
        grep -Ev "^(#|$)" "$smartf" 2>/dev/null | head -20
    fi

    # --- Control Panels ---
    if [[ -d /etc/webmin ]]; then
        echo "--- Webmin"
        [[ -f /etc/webmin/miniserv.conf ]] && grep -E "^(port|ssl|listen|session)" /etc/webmin/miniserv.conf 2>/dev/null
        echo "modules:"
        ls /etc/webmin/ 2>/dev/null | head -30
        [[ -d /etc/webmin/virtual-server ]] && echo "Virtualmin=installed"
    fi

    [[ -d /etc/usermin ]] && echo "--- Usermin=installed"

    # --- Time Sync ---
    if [[ -d /etc/chrony ]]; then
        echo "--- Chrony"
        [[ -f /etc/chrony/chrony.conf ]] && grep -Ev "^(#|$)" /etc/chrony/chrony.conf 2>/dev/null | head -20
    fi

    if [[ -f /etc/systemd/timesyncd.conf ]]; then
        echo "--- systemd-timesyncd"
        grep -Ev "^(#|$|\[)" /etc/systemd/timesyncd.conf 2>/dev/null | head -10
    fi

    # --- Custom Systemd Units ---
    echo "--- Custom systemd units"
    for unit_file in /etc/systemd/system/*.service /etc/systemd/system/*.timer; do
        [[ -f "$unit_file" ]] || continue
        # Skip symlinks to /lib/systemd (standard units)
        [[ -L "$unit_file" ]] && continue

        echo "--- Custom: $(basename "$unit_file")"
        cat "$unit_file" 2>/dev/null | head -40

        # Follow EnvironmentFile references
        grep -oP '^EnvironmentFile=-?\K.*' "$unit_file" 2>/dev/null | while IFS= read -r env_file; do
            env_file="${env_file/#-/}"
            if [[ -f "$env_file" ]]; then
                echo "--- Config: $env_file"
                cat "$env_file" 2>/dev/null | head -30
            fi
        done

        # Extract config file paths from ExecStart
        local exec_start
        exec_start=$(grep -oP '^ExecStart=\K.*' "$unit_file" 2>/dev/null || true)
        if [[ -n "$exec_start" ]]; then
            echo "$exec_start" | grep -oE '(/etc/[^ ]+|/opt/[^ ]+\.(conf|yaml|yml|json|ini|cfg))' 2>/dev/null | while IFS= read -r cfg; do
                if [[ -f "$cfg" ]]; then
                    echo "--- Config: $cfg"
                    cat "$cfg" 2>/dev/null | head -30
                fi
            done
        fi

        # WorkingDirectory
        grep -oP '^WorkingDirectory=\K.*' "$unit_file" 2>/dev/null
    done

    # Systemd overrides
    find /etc/systemd/system -name "override.conf" 2>/dev/null | while IFS= read -r f; do
        echo "--- Override: $f"
        cat "$f" 2>/dev/null | head -20
    done

    # --- Runtime State ---
    echo "--- Running services"
    systemctl list-units --type=service --state=running --no-pager --no-legend 2>/dev/null | awk '{print $1}' | head -50

    echo "--- Active timers"
    systemctl list-timers --no-pager --no-legend 2>/dev/null | awk '{print $NF}' | head -30

    echo "--- Listening TCP"
    ss -tlnp 2>/dev/null | tail -n +2 | head -50

    echo "--- Listening UDP"
    ss -ulnp 2>/dev/null | tail -n +2 | head -30
}

# Deep: Security — full firewall rules, IDS configs, auth, logs.
__sysinfo_deep_security() {
    echo "=== SECURITY DEEP ==="

    # Firewall
    if command -v iptables >/dev/null 2>&1; then
        echo "--- iptables"
        __sysinfo_sudo iptables -L -n -v 2>/dev/null | head -50
    fi

    if [[ -d /etc/ufw ]]; then
        echo "--- UFW"
        __sysinfo_sudo ufw status verbose 2>/dev/null | head -30
        [[ -f /etc/ufw/ufw.conf ]] && grep -Ev "^(#|$)" /etc/ufw/ufw.conf 2>/dev/null
    fi

    if command -v firewall-cmd >/dev/null 2>&1; then
        echo "--- firewalld"
        __sysinfo_sudo firewall-cmd --list-all 2>/dev/null | head -30
    fi

    # IDS
    if [[ -d /etc/fail2ban ]]; then
        echo "--- fail2ban"
        __sysinfo_sudo fail2ban-client status 2>/dev/null | head -20
        [[ -f /etc/fail2ban/jail.local ]] && {
            echo "--- jail.local"
            __sysinfo_sudo grep -Ev "^(#|;|$)" /etc/fail2ban/jail.local 2>/dev/null | head -40
        }
        __sysinfo_sudo find /etc/fail2ban/jail.d -name "*.conf" 2>/dev/null | while IFS= read -r f; do
            echo "--- $(basename "$f")"
            __sysinfo_sudo grep -Ev "^(#|;|$)" "$f" 2>/dev/null | head -30
        done
    fi

    if [[ -d /etc/crowdsec ]]; then
        echo "--- CrowdSec"
        [[ -f /etc/crowdsec/config.yaml ]] && grep -Ev "^(#|$)" /etc/crowdsec/config.yaml 2>/dev/null | head -30
        [[ -f /etc/crowdsec/acquis.yaml ]] && { echo "--- acquis.yaml"; cat /etc/crowdsec/acquis.yaml 2>/dev/null | head -30; }
        echo "--- scenarios"
        ls /etc/crowdsec/hub/scenarios/ 2>/dev/null | head -20
    fi

    # SSH
    echo "--- SSH"
    __sysinfo_sudo grep -Ev "^(#|$)" /etc/ssh/sshd_config 2>/dev/null
    __sysinfo_sudo find /etc/ssh/sshd_config.d -name "*.conf" 2>/dev/null | while IFS= read -r f; do
        echo "--- $(basename "$f")"
        __sysinfo_sudo grep -Ev "^(#|$)" "$f" 2>/dev/null
    done

    # PAM
    echo "--- PAM"
    for pam in common-auth common-password common-session; do
        [[ -f "/etc/pam.d/$pam" ]] && {
            echo "--- $pam"
            grep -Ev "^(#|$)" "/etc/pam.d/$pam" 2>/dev/null | head -30
        }
    done

    # AppArmor
    if [[ -d /etc/apparmor.d ]]; then
        echo "--- AppArmor"
        if command -v aa-status >/dev/null 2>&1; then
            __sysinfo_sudo aa-status 2>/dev/null | head -30
        else
            ls /etc/apparmor.d/ 2>/dev/null | head -30
        fi
    fi

    # Auth
    echo "--- Users"
    awk -F: '$3>=1000 && $3<65534{print $1":"$3":"$6":"$7}' /etc/passwd 2>/dev/null

    echo "--- Sudoers"
    __sysinfo_sudo grep -Ev "^(#|$)" /etc/sudoers 2>/dev/null | head -20
    __sysinfo_sudo find /etc/sudoers.d -type f 2>/dev/null | while IFS= read -r f; do
        echo "--- $(basename "$f")"
        __sysinfo_sudo grep -Ev "^(#|$)" "$f" 2>/dev/null
    done

    if [[ -f /etc/login.defs ]]; then
        echo "--- login.defs (key)"
        grep -E "^(PASS_MAX_DAYS|PASS_MIN_DAYS|PASS_MIN_LEN|PASS_WARN_AGE|LOGIN_RETRIES|ENCRYPT_METHOD|UMASK)" /etc/login.defs 2>/dev/null
    fi

    # Logs
    echo "--- Login history"
    last -20 2>/dev/null | head -20

    echo "--- Auth log (recent)"
    __sysinfo_sudo tail -30 /var/log/auth.log 2>/dev/null | head -30
}

# Deep: Network — interfaces, routes, DNS, full configs.
__sysinfo_deep_network() {
    echo "=== NETWORK DEEP ==="

    echo "--- Interfaces"
    ip addr show 2>/dev/null | head -50

    echo "--- Routes"
    ip route show 2>/dev/null | head -20

    echo "--- Neighbors"
    ip neigh show 2>/dev/null | head -20

    echo "--- resolv.conf"
    cat /etc/resolv.conf 2>/dev/null

    if command -v resolvectl >/dev/null 2>&1; then
        echo "--- systemd-resolved"
        resolvectl status 2>/dev/null | head -20
    fi

    echo "--- hosts"
    cat /etc/hosts 2>/dev/null

    if [[ -d /etc/netplan ]]; then
        echo "--- Netplan"
        find /etc/netplan -name "*.yaml" 2>/dev/null | while IFS= read -r f; do
            echo "--- $(basename "$f")"
            cat "$f" 2>/dev/null | head -50
        done
    fi

    if [[ -f /etc/network/interfaces ]]; then
        echo "--- interfaces (legacy)"
        grep -Ev "^(#|$)" /etc/network/interfaces 2>/dev/null
    fi

    if [[ -f /etc/dhcpcd.conf ]]; then
        echo "--- dhcpcd"
        grep -Ev "^(#|$)" /etc/dhcpcd.conf 2>/dev/null
    fi

    echo "--- Listening TCP"
    ss -tlnp 2>/dev/null | tail -n +2 | head -50

    echo "--- Listening UDP"
    ss -ulnp 2>/dev/null | tail -n +2 | head -30
}

# Deep: Storage — disk usage, LVM, RAID, ZFS, SMART, NFS.
__sysinfo_deep_storage() {
    echo "=== STORAGE DEEP ==="

    echo "--- Disk usage"
    df -hT 2>/dev/null | head -30

    echo "--- Block devices"
    lsblk -f 2>/dev/null | head -30

    echo "--- Mounts"
    mount 2>/dev/null | grep -v cgroup | head -30

    echo "--- fstab"
    cat /etc/fstab 2>/dev/null

    if [[ -f /etc/crypttab ]]; then
        echo "--- crypttab"
        __sysinfo_sudo cat /etc/crypttab 2>/dev/null
    fi

    # LVM
    if command -v pvs >/dev/null 2>&1; then
        echo "--- LVM PVs"
        pvs 2>/dev/null | head -20
        echo "--- LVM VGs"
        vgs 2>/dev/null | head -20
        echo "--- LVM LVs"
        lvs 2>/dev/null | head -20
    fi

    # RAID
    if [[ -f /etc/mdadm/mdadm.conf ]]; then
        echo "--- mdadm"
        grep -Ev "^(#|$)" /etc/mdadm/mdadm.conf 2>/dev/null
    fi
    [[ -f /proc/mdstat ]] && { echo "--- mdstat"; cat /proc/mdstat 2>/dev/null | head -30; }

    # ZFS
    if command -v zpool >/dev/null 2>&1; then
        echo "--- ZFS pools"
        zpool status 2>/dev/null | head -30
        echo "--- ZFS datasets"
        zfs list 2>/dev/null | head -30
    fi
    [[ -f /etc/zfs/zed.rc ]] && { echo "--- zed.rc"; grep -Ev "^(#|$)" /etc/zfs/zed.rc 2>/dev/null; }

    # Sanoid
    [[ -f /etc/sanoid/sanoid.conf ]] && { echo "--- Sanoid"; cat /etc/sanoid/sanoid.conf 2>/dev/null | head -30; }

    # SMART
    if command -v smartctl >/dev/null 2>&1; then
        echo "--- SMART"
        for disk in /dev/sd? /dev/nvme?n?; do
            [[ -b "$disk" ]] || continue
            echo "--- $(basename "$disk")"
            __sysinfo_sudo smartctl -H "$disk" 2>/dev/null | head -10
        done
    fi

    # NFS
    [[ -f /etc/exports ]] && { echo "--- NFS exports"; cat /etc/exports 2>/dev/null | head -30; }
    [[ -f /etc/nfs.conf ]] && { echo "--- nfs.conf"; grep -Ev "^(#|$)" /etc/nfs.conf 2>/dev/null | head -20; }

    # hdparm
    [[ -f /etc/hdparm.conf ]] && { echo "--- hdparm"; grep -Ev "^(#|$)" /etc/hdparm.conf 2>/dev/null | head -20; }
}

# Deep: Performance — memory, CPU, processes, DB tuning, PHP FPM, sysctl.
__sysinfo_deep_performance() {
    echo "=== PERFORMANCE DEEP ==="

    echo "--- Memory"
    free -h 2>/dev/null

    echo "--- CPU"
    lscpu 2>/dev/null | grep -iE "model name|cpu\(s\)|thread|core|socket|mhz" | head -10

    echo "--- Load"
    uptime 2>/dev/null

    echo "--- Top by memory"
    ps aux --sort=-%mem 2>/dev/null | head -15

    echo "--- Top by CPU"
    ps aux --sort=-%cpu 2>/dev/null | head -10

    # MySQL/MariaDB tuning
    if [[ -d /etc/mysql ]]; then
        echo "--- MySQL/MariaDB config"
        if [[ -d /etc/mysql/mariadb.conf.d ]]; then
            for cnf in /etc/mysql/mariadb.conf.d/*.cnf; do
                [[ -f "$cnf" ]] || continue
                echo "--- $(basename "$cnf")"
                grep -Ev "^(#|$)" "$cnf" 2>/dev/null
            done
        fi
    fi

    # PHP FPM pool tuning
    if [[ -d /etc/php ]]; then
        echo "--- PHP FPM pools"
        for phpver in /etc/php/*/fpm/pool.d/; do
            [[ -d "$phpver" ]] || continue
            local ver
            ver=$(echo "$phpver" | grep -oP '/etc/php/\K[^/]+')
            for pool in "${phpver}"*.conf; do
                [[ -f "$pool" ]] || continue
                echo "--- PHP ${ver} $(basename "$pool")"
                grep -Ev "^(;|$)" "$pool" 2>/dev/null | head -50
            done
        done
    fi

    # sysctl
    echo "--- sysctl (key active params)"
    sysctl -a 2>/dev/null | grep -E "^(vm\.swappiness|vm\.dirty_ratio|vm\.dirty_background|net\.core\.somaxconn|net\.core\.netdev_max_backlog|net\.ipv4\.tcp_max_syn|net\.ipv4\.tcp_fin_timeout|net\.ipv4\.tcp_tw_reuse|net\.ipv4\.ip_local_port_range|fs\.file-max|fs\.inotify)" 2>/dev/null | head -30

    echo "--- sysctl.d configs"
    find /etc/sysctl.d -name "*.conf" 2>/dev/null | while IFS= read -r f; do
        echo "--- $(basename "$f")"
        grep -Ev "^(#|$)" "$f" 2>/dev/null
    done
    [[ -f /etc/sysctl.conf ]] && {
        local content
        content=$(grep -Ev "^(#|$)" /etc/sysctl.conf 2>/dev/null)
        [[ -n "$content" ]] && { echo "--- sysctl.conf"; echo "$content"; }
    }

    # I/O scheduler
    echo "--- I/O scheduler"
    for sched in /sys/block/*/queue/scheduler; do
        [[ -f "$sched" ]] || continue
        echo "$(dirname "$sched" | xargs basename): $(cat "$sched" 2>/dev/null)"
    done
}

# Deep: Maintenance — cron, logrotate, certs, packages, etckeeper, cloud.
__sysinfo_deep_maintenance() {
    echo "=== MAINTENANCE DEEP ==="

    # Cron
    echo "--- crontab"
    grep -Ev "^(#|$)" /etc/crontab 2>/dev/null | head -20

    echo "--- cron.d contents"
    for f in /etc/cron.d/*; do
        [[ -f "$f" ]] || continue
        echo "--- $(basename "$f")"
        grep -Ev "^(#|$)" "$f" 2>/dev/null | head -15
    done

    echo "--- cron.daily"
    ls /etc/cron.daily/ 2>/dev/null
    echo "--- cron.hourly"
    ls /etc/cron.hourly/ 2>/dev/null
    echo "--- cron.weekly"
    ls /etc/cron.weekly/ 2>/dev/null
    echo "--- cron.monthly"
    ls /etc/cron.monthly/ 2>/dev/null

    echo "--- User crontabs"
    __sysinfo_sudo ls /var/spool/cron/crontabs/ 2>/dev/null | head -10

    # Logrotate
    echo "--- Logrotate configs"
    for f in /etc/logrotate.d/*; do
        [[ -f "$f" ]] || continue
        echo "--- $(basename "$f")"
        cat "$f" 2>/dev/null | head -15
    done

    # Let's Encrypt
    if [[ -d /etc/letsencrypt ]]; then
        echo "--- Let's Encrypt"
        echo "--- Live domains"
        __sysinfo_sudo ls /etc/letsencrypt/live/ 2>/dev/null

        echo "--- Cert expiry"
        __sysinfo_sudo find /etc/letsencrypt/live -name "cert.pem" 2>/dev/null | while IFS= read -r cert; do
            local domain
            domain=$(dirname "$cert" | xargs basename)
            local expiry
            expiry=$(__sysinfo_sudo openssl x509 -enddate -noout -in "$cert" 2>/dev/null)
            echo "${domain}: ${expiry}"
        done

        echo "--- Renewal configs"
        __sysinfo_sudo find /etc/letsencrypt/renewal -name "*.conf" 2>/dev/null | while IFS= read -r f; do
            echo "--- $(basename "$f")"
            __sysinfo_sudo cat "$f" 2>/dev/null | head -20
        done

        [[ -f /etc/letsencrypt/cli.ini ]] && { echo "--- cli.ini"; grep -Ev "^(#|$)" /etc/letsencrypt/cli.ini 2>/dev/null; }
    fi

    # Package management
    echo "--- Upgradable packages"
    apt list --upgradable 2>/dev/null | tail -n +2 | head -30

    echo "--- dpkg log (recent)"
    tail -30 /var/log/dpkg.log 2>/dev/null

    if [[ -f /etc/apt/apt.conf.d/50unattended-upgrades ]]; then
        echo "--- unattended-upgrades"
        grep -Ev "^(//|$)" /etc/apt/apt.conf.d/50unattended-upgrades 2>/dev/null | head -30
    fi

    # Pending config merges
    echo "--- Pending config merges"
    find /etc -name "*.dpkg-old" -o -name "*.dpkg-dist" 2>/dev/null | head -20

    # Legacy backups
    local orgs
    orgs=$(ls /etc/*.org 2>/dev/null)
    [[ -n "$orgs" ]] && { echo "--- Legacy .org backups"; echo "$orgs"; }

    # etckeeper
    if [[ -d /etc/.git ]]; then
        echo "--- etckeeper log"
        git -C /etc log --oneline -20 2>/dev/null
    fi

    # Cloud
    if [[ -f /etc/cloud/cloud.cfg ]]; then
        echo "--- Cloud-init"
        grep -E "^(distro|system_info|manage_etc_hosts|preserve_hostname)" /etc/cloud/cloud.cfg 2>/dev/null | head -20
    fi

    # Profile + Environment
    echo "--- Profile scripts"
    ls /etc/profile.d/ 2>/dev/null
    [[ -f /etc/environment ]] && { echo "--- environment"; cat /etc/environment 2>/dev/null; }

    # Service defaults
    echo "--- Service defaults"
    ls /etc/default/ 2>/dev/null | head -30
}

# =============================================================================
# PUBLIC FUNCTION
# =============================================================================

# System information enumeration.
# Usage: sysinfo [section] [--llm]
# Sections: identity storage services auth network security webstack mail infra system all
# Alias: si
sysinfo() {
    needs_help "sysinfo" "sysinfo [section] [--llm]" \
        "System information enumeration for headless Debian/Ubuntu servers.
Sections: identity storage services auth network security webstack mail infra system all
Flags: --llm (compact token-efficient output for AI). Alias: si" \
        "${1-}" && return

    local section="all"
    __SYSINFO_MODE="verbose"

    # Parse args
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --llm) __SYSINFO_MODE="llm"; shift ;;
            identity|storage|services|auth|network|security|webstack|mail|infra|system|all)
                section="$1"; shift ;;
            *)
                __gash_error "Unknown argument: $1"
                __gash_info "Sections: identity storage services auth network security webstack mail infra system all"
                __gash_info "Flags: --llm"
                return 1
                ;;
        esac
    done

    # Sudo caching (single prompt)
    __sysinfo_ensure_sudo

    # Dispatch
    case "$section" in
        identity)  __sysinfo_collect_identity ;;
        storage)   __sysinfo_collect_storage ;;
        services)  __sysinfo_collect_services ;;
        auth)      __sysinfo_collect_auth ;;
        network)   __sysinfo_collect_network ;;
        security)  __sysinfo_collect_security ;;
        webstack)  __sysinfo_collect_webstack ;;
        mail)      __sysinfo_collect_mail ;;
        infra)     __sysinfo_collect_infra ;;
        system)    __sysinfo_collect_system ;;
        all)
            __sysinfo_collect_identity
            __sysinfo_collect_storage
            __sysinfo_collect_services
            __sysinfo_collect_auth
            __sysinfo_collect_network
            __sysinfo_collect_security
            __sysinfo_collect_webstack
            __sysinfo_collect_mail
            __sysinfo_collect_infra
            __sysinfo_collect_system
            ;;
    esac

    __sysinfo_release_sudo
}

# =============================================================================
# ALIASES
# =============================================================================

alias si='sysinfo'
