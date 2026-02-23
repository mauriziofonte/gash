#!/usr/bin/env bash

# =============================================================================
# Gash Module: AI Chat Integration
# =============================================================================
#
# Interactive chat with Claude/Gemini APIs directly from terminal.
#
# Dependencies: core/output.sh, core/config.sh, core/utils.sh
# Runtime: curl, jq
#
# Configuration in ~/.gash_env:
#   AI:claude=YOUR_CLAUDE_API_KEY
#   AI:gemini=YOUR_GEMINI_API_KEY
#
# Public functions:
#   ai_ask [provider]          - Interactive chat (alias: ask)
#   ai_query [provider] query  - Non-interactive query
#   gash_ai_list               - List configured providers (in config.sh)
#
# =============================================================================

# =============================================================================
# CONSTANTS
# =============================================================================

# Guard against re-sourcing (readonly variables cannot be redefined)
if [[ -z "${__AI_CLAUDE_API:-}" ]]; then
    # API Endpoints
    readonly __AI_CLAUDE_API="https://api.anthropic.com/v1/messages"
    readonly __AI_GEMINI_API="https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"

    # Models
    readonly __AI_CLAUDE_MODEL="claude-haiku-4-5-20251001"
    readonly __AI_GEMINI_MODEL="gemini-2.0-flash"

    # Timeouts (seconds)
    readonly __AI_CONNECT_TIMEOUT=5   # Connection timeout (quick fail if unreachable)
    readonly __AI_RESPONSE_TIMEOUT=60 # Response timeout (allow LLM time to think)

    # System prompt for structured JSON output
    readonly __AI_SYSTEM_PROMPT='You are a bash/linux terminal assistant. Respond ONLY with valid JSON.

RESPONSE TYPES (select in order of priority):
1. "troubleshoot": Content provided via pipe → analyze, identify issue, suggest fix
2. "command": User asks HOW TO DO something → provide bash command + explanation
3. "explanation": User asks WHAT something IS → provide conceptual explanation
4. "code": User asks to WRITE a script/config → provide code snippet
5. "fallback": Everything else (greetings, unclassifiable) → brief direct answer

RULES:
1. Match user language in text field
2. For "command": command = exact bash command, text = brief explanation
3. For "code": code = complete snippet, lang = language name
4. For "troubleshoot": issue = problem found, suggestion = how to fix, text = summary
5. For "fallback": only text field needed
6. Keep all fields concise (1-3 sentences max)
7. NEVER use markdown formatting in any field'

    # JSON Schema for structured responses (5 types)
    readonly __AI_RESPONSE_SCHEMA='{
  "type": "object",
  "properties": {
    "type": {
      "type": "string",
      "enum": ["command", "explanation", "code", "troubleshoot", "fallback"],
      "description": "Response type"
    },
    "command": {
      "type": "string",
      "description": "Bash command. Only when type=command."
    },
    "text": {
      "type": "string",
      "description": "Main response text. Required for all types."
    },
    "code": {
      "type": "string",
      "description": "Code snippet. Only when type=code."
    },
    "lang": {
      "type": "string",
      "description": "Code language (bash, python, etc). Only when type=code."
    },
    "issue": {
      "type": "string",
      "description": "Problem identified. Only when type=troubleshoot."
    },
    "suggestion": {
      "type": "string",
      "description": "How to fix/resolve. Only when type=troubleshoot."
    }
  },
  "required": ["type", "text"],
  "additionalProperties": false
}'

fi

# Sysinfo constants: guard on drilldown schema (v2 marker — recreates all on upgrade)
if [[ -z "${__AI_SYSINFO_DRILLDOWN_SCHEMA:-}" ]]; then
    # Unset old readonly constants if they exist (will fail silently for readonly)
    unset __AI_SYSINFO_MAX_TOKENS __AI_SYSINFO_RESPONSE_TIMEOUT __AI_SYSINFO_SYSTEM_PROMPT __AI_SYSINFO_RESPONSE_SCHEMA 2>/dev/null || true

    readonly __AI_SYSINFO_MAX_TOKENS=8192
    readonly __AI_SYSINFO_RESPONSE_TIMEOUT=120

    # System prompt for sysinfo analysis (embedded KB + analysis framework)
    readonly __AI_SYSINFO_SYSTEM_PROMPT='You are a headless Debian/Ubuntu server analyst. Given system enumeration data, analyze services, security, and configuration. Respond ONLY with valid JSON matching the provided schema.

SERVICE DETECTION MAP:
Core: systemd/ (units, timers, overrides .d/), init.d/ rc*.d/ (SysV), cloud/ (cloud-init), wsl.conf (WSL2)
Web: apache2/ (sites-available/enabled, mods-available/enabled, conf-available/enabled, ports.conf), nginx/ (sites-enabled/), lighttpd/
PHP: php/{ver}/ (fpm/pool.d/*.conf, cli/php.ini, mods-available/, conf.d/)
DB: mysql/ (mariadb.conf.d/{50-server,50-client,60-galera,99-custom}.cnf), postgresql/, redis/
Mail: postfix/ (main.cf, master.cf, virtual, bcc, sni_map, sasl/), dovecot/ (dovecot.conf, conf.d/*.conf), exim4/ (update-exim4.conf.conf), spamassassin/ (local.cf, razor/, pyzor/), dkim-domains.txt sasldb2 (OpenDKIM/SASL)
DNS: bind/ (named.conf.local/options, zones, rndc.key), unbound/ (unbound.conf.d/)
FTP: proftpd/ (proftpd.conf, conf.d/), vsftpd/
Security: crowdsec/ (config.yaml, acquis.yaml, hub/scenarios), fail2ban/ (jail.local, jail.d/, filter.d/, action.d/), ufw/ (ufw.conf, applications.d/), firewalld/ (zones, services), iptables/, apparmor.d/
Storage: openmediavault/ (config.xml), samba/ (smb.conf), zfs/ (zed.rc, zed.d/), sanoid/ (sanoid.conf), lvm/, mdadm/, NFS (exports, nfs.conf), smartmontools/ (smartd.conf)
VPN: openvpn/ (*.conf, custom-config/ Mullvad), wireguard/
Containers: docker/ (daemon.json), containerd/, nvidia-container-runtime/ nvidia-container-toolkit/ (GPU)
Monitoring: collectd/ (collectd.conf.d/*.conf), monit/ (monitrc, conf.d/, conf-available/), snmp/, sysstat/
Panels: webmin/ (modules, ACLs, virtual-server/ = Virtualmin), usermin/
SSL: letsencrypt/ (live/, renewal/, cli.ini), ssl/ (certs/, private/, openssl.cnf)
Cron: crontab, cron.d/, cron.{daily,hourly,weekly,monthly}/
Auth: pam.d/ (common-auth/session/password), security/ (limits.conf, limits.d/), sudoers sudoers.d/, ssh/ (sshd_config, sshd_config.d/), ldap/, cracklib/
Network: network/interfaces, netplan/, resolv.conf, hostname, hosts, dhcp/ dhcpcd.conf, avahi/, NetworkManager/, wpa_supplicant/, wsdd-server/
Time: chrony/ (chrony.conf), systemd/timesyncd.conf
Runtimes: python*, java-*-openjdk/, groovy/
GPU: OpenCL/, vulkan/, glvnd/, cufile.json (NVIDIA GDS)
Enterprise: landscape/, ubuntu-advantage/ (uaclient.conf), update-manager/, apticron/
Legacy: xinetd.d/, runit/, salt/ (minion), ImageMagick-{6,7}/, awstats/, jailkit/, logcheck/, john/
Markers: wsl.conf ubwsl-installed (WSL), hetzner-build (Hetzner), hostid (ZFS), boot.d/ shutdown.d/ (custom hooks)

FILE PATTERNS:
- .d/ dirs: numeric prefix ordering (10-*, 20-*, 99-*). available/enabled: symlink pattern (Apache, PHP mods)
- Package traces: .dpkg-old .dpkg-dist .dpkg-bak .ucf-dist = pending manual merges after upgrades
- Skip binary: ld.so.cache, alternatives/*, *.db, .etckeeper (>50KB), .git/

ANALYSIS PRIORITIES:
1. Identity: OS version, kernel, cloud provider (Hetzner/AWS), container type (bare-metal/VM/OpenVZ/LXC/Docker), WSL
2. Services: Cross-ref /etc/ dirs with running services. Flag: dir present but service NOT running (disabled?), running but no config dir (config in /usr/lib/?). Include custom systemd units.
3. Security RED FLAGS:
   - SSH: PermitRootLogin yes, PasswordAuthentication yes = warning; non-standard port = hardening
   - No fail2ban AND no crowdsec = CRITICAL (brute-force unprotected)
   - Open ports without matching service = investigate
   - No firewall (ufw/firewalld/iptables) = CRITICAL on public server
   - .dpkg-old on security files (pam.d/, ssh/, sudoers) = failed security upgrade
   - sudo NOPASSWD = note
4. Network: resolv.conf (stub vs direct), static vs DHCP, netplan vs ifupdown vs NetworkManager
5. Service Deep Dives:
   - Web: vhost count, domains, reverse proxies, SSL, enabled modules
   - PHP: versions, FPM pools, pm strategy, memory limits
   - DB: engine, InnoDB buffer pool, max_connections, binary log, Galera
   - Mail: role (origin/relay/full MTA), DKIM, TLS, virtual domains
   - DNS: zone count, DNSSEC, recursion policy (authoritative vs recursive)
   - Storage: fs types, RAID, LVM, NFS exports, Samba shares, SMART schedules, ZFS/sanoid
   - Docker: daemon config, GPU passthrough (nvidia-container-runtime)
   - Monitoring: Collectd plugins, Monit services, SMART disk schedules
   - Cron: task count, notable custom jobs (OMV automation, backups)
   - Custom systemd: purpose (from Description/ExecStart), config, issues
6. Performance: sysctl custom params (vm.*, net.*), InnoDB vs RAM, FPM children, I/O (SMR disk, NVMe, RAID)
7. Maintenance: .dpkg-old/.dpkg-dist = pending merges, etckeeper, LE renewal, *.org = old backups, systemd timer overrides (apt-daily delays)

RULES:
- In "Services Detected": list EVERY detected service as a separate finding with key config params.
  Examples: "Apache2: mpm_event, 13 vhosts, Listen localhost:80/443, mods: rewrite/ssl/proxy_fcgi"
  "PHP-FPM 8.1: pool domain1, pm=dynamic, max_children=5, memory_limit=256M"
  "MariaDB: InnoDB buffer_pool=4G, max_connections=20, bind=127.0.0.1, binary_log=disabled"
  List each PHP-FPM version separately. List each database separately. List each custom systemd
  service separately with its purpose (inferred from ExecStart/Description).
  Include ALL: web, PHP, databases, mail, DNS, FTP, NAS, VPN, containers, monitoring, panels,
  time sync, and any custom/non-standard services.
- In other sections: one concise sentence per finding
- Severity: critical (immediate action), warning (should address), info (notable), ok (confirmed good)
- Be specific: mention service names, config files, actual values'

    # JSON Schema for sysinfo analysis response
    readonly __AI_SYSINFO_RESPONSE_SCHEMA='{
  "type": "object",
  "properties": {
    "hostname": { "type": "string" },
    "platform": { "type": "string", "description": "OS version | kernel | provider/type" },
    "sections": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "title": {
            "type": "string",
            "enum": ["Services Detected", "Security Posture", "Network", "Storage", "Performance Tuning", "Maintenance", "Recommendations"]
          },
          "findings": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "severity": { "type": "string", "enum": ["critical", "warning", "info", "ok"] },
                "text": { "type": "string" }
              },
              "required": ["severity", "text"],
              "additionalProperties": false
            }
          }
        },
        "required": ["title", "findings"],
        "additionalProperties": false
      }
    }
  },
  "required": ["hostname", "platform", "sections"],
  "additionalProperties": false
}'

    # Drilldown system prompt (focused, for per-section deep analysis)
    readonly __AI_SYSINFO_DRILLDOWN_PROMPT='You are analyzing one section of a Debian/Ubuntu server in detail. You receive:
1. Initial analysis findings (from a previous overview pass)
2. Detailed configuration data (full config files, runtime state)

For each service/component detected:
- Report its status (running, installed, stopped, error, eol)
- List ALL non-default configuration values as config_highlights
- Flag security issues, performance concerns, EOL software, misconfigurations as issues with severity
- Be thorough: list every relevant parameter, not just summaries
- Group related settings under the same component (e.g., "PHP 8.1 FPM" not "PHP")

For CUSTOM systemd units (services you do not recognize by name):
- Infer the service purpose from Description, ExecStart, WorkingDirectory
- Analyze referenced config files (EnvironmentFile, config paths in ExecStart)
- Flag any issues: missing restart policy, no resource limits, running as root unnecessarily
- Report what the service does, how it is configured, and key files involved

Respond ONLY with valid JSON matching the provided schema.'

    # Drilldown JSON Schema (component-based for per-service detail)
    readonly __AI_SYSINFO_DRILLDOWN_SCHEMA='{
  "type": "object",
  "properties": {
    "title": { "type": "string" },
    "summary": { "type": "string", "description": "2-3 sentence overview of this section" },
    "components": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string", "description": "Service or component name" },
          "status": { "type": "string", "enum": ["running", "installed", "stopped", "error", "eol"] },
          "config_highlights": {
            "type": "array",
            "items": { "type": "string" },
            "description": "Key config values, especially non-default"
          },
          "issues": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "severity": { "type": "string", "enum": ["critical", "warning", "info", "ok"] },
                "text": { "type": "string" }
              },
              "required": ["severity", "text"],
              "additionalProperties": false
            }
          }
        },
        "required": ["name", "status", "config_highlights", "issues"],
        "additionalProperties": false
      }
    }
  },
  "required": ["title", "summary", "components"],
  "additionalProperties": false
}'
fi

# =============================================================================
# INTERNAL HELPERS
# =============================================================================

# Debug output (enabled with GASH_AI_DEBUG=1)
# Usage: __ai_debug "label" "content"
__ai_debug() {
    [[ "${GASH_AI_DEBUG:-0}" != "1" ]] && return 0
    local label="${1-}"
    local content="${2-}"
    echo -e "\n${__GASH_CYAN}[AI DEBUG] ${label}:${__GASH_COLOR_OFF}" >&2
    echo "$content" >&2
}

# Check if jq is available
__ai_require_jq() {
    if ! type -P jq >/dev/null 2>&1; then
        __gash_error "jq is required for AI module. Install with: sudo apt install jq"
        return 1
    fi
    return 0
}

# Check if curl is available
__ai_require_curl() {
    if ! type -P curl >/dev/null 2>&1; then
        __gash_error "curl is required for AI module. Install with: sudo apt install curl"
        return 1
    fi
    return 0
}

# Handle curl exit codes and HTTP errors for AI API calls.
# Returns: 0 if no error, 1 if fatal error, 2 if retryable (429 rate-limit, already slept).
# Usage: __ai_handle_curl_error <provider> <curl_exit> <http_code> <elapsed_time> <response> <curl_error>
__ai_handle_curl_error() {
    local provider="$1" curl_exit="${2:-}" http_code="$3"
    local elapsed_time="$4" response="$5" curl_error="${6:-}"

    if [[ -n "$curl_exit" ]]; then
        case "$curl_exit" in
            5)  __gash_error "$provider API: Couldn't resolve proxy" ;;
            6)  __gash_error "$provider API: Could not resolve host (check DNS/internet)" ;;
            7)  __gash_error "$provider API: Failed to connect (server unreachable)" ;;
            18) __gash_error "$provider API: Partial response received (transfer interrupted)" ;;
            22) __gash_error "$provider API: HTTP error returned" ;;
            28)
                if [[ $elapsed_time -le $(( __AI_CONNECT_TIMEOUT + 2 )) ]]; then
                    __gash_error "$provider API: Connection timed out after ${elapsed_time}s (server unreachable)"
                else
                    __gash_error "$provider API: Response timed out after ${elapsed_time}s (max: ${__AI_RESPONSE_TIMEOUT}s)"
                fi
                ;;
            35) __gash_error "$provider API: SSL/TLS handshake failed" ;;
            47) __gash_error "$provider API: Too many redirects" ;;
            52) __gash_error "$provider API: Server returned empty response" ;;
            55) __gash_error "$provider API: Failed sending data to server" ;;
            56) __gash_error "$provider API: Failed receiving data from server" ;;
            60) __gash_error "$provider API: SSL certificate problem (verify failed)" ;;
            *)  __gash_error "$provider API: curl failed (exit $curl_exit)${curl_error:+: $curl_error}" ;;
        esac
        return 1
    fi

    if [[ -z "$http_code" || "$http_code" == "000" ]]; then
        __gash_error "$provider API: No response received (network error)"
        return 1
    fi

    if [[ "$http_code" != "200" ]]; then
        # 429 rate-limited: extract retry delay, let caller decide
        if [[ "$http_code" == "429" ]]; then
            local retry_delay=""
            # Gemini: .error.details[].retryDelay ("30s", "3.9s", "633.4ms")
            retry_delay=$(echo "$response" | jq -r '
                .error.details[]? | select(."@type" // "" | contains("RetryInfo")) | .retryDelay // empty
            ' 2>/dev/null | head -1)
            # Parse duration: "30s" → 30, "633ms" → 0.633
            if [[ "$retry_delay" =~ ^([0-9.]+)ms$ ]]; then
                retry_delay=$(awk "BEGIN{printf \"%.1f\", ${BASH_REMATCH[1]}/1000}")
            else
                retry_delay="${retry_delay%s}"
            fi
            # Validate numeric, default 5s
            if ! [[ "${retry_delay:-}" =~ ^[0-9]+\.?[0-9]*$ ]]; then
                retry_delay=5
            fi
            # Floor: at least 1s to avoid rapid-fire retries
            if [[ ${retry_delay%%.*} -lt 1 ]]; then
                retry_delay=1
            fi
            # Store for caller to sleep/retry
            __AI_RETRY_DELAY="$retry_delay"
            __AI_RETRY_PROVIDER="$provider"
            return 2
        fi

        local error_msg
        error_msg=$(echo "$response" | jq -r '.error.message // .error // "Unknown error"' 2>/dev/null)
        __gash_error "$provider API error ($http_code): $error_msg"
        return 1
    fi

    return 0
}

# Gather context information (~80 tokens)
# Returns JSON: {"cwd":"/path","shell":"bash","distro":"ubuntu","pkg":"apt","git":"branch","files":["a","b"],"exit":0}
__ai_gather_context() {
    local cwd shell_name distro pkg_mgr git_branch exit_code files_json

    cwd="$(pwd)"

    # Detect shell
    if [[ -n "${BASH_VERSION:-}" ]]; then
        shell_name="bash"
    elif [[ -n "${ZSH_VERSION:-}" ]]; then
        shell_name="zsh"
    else
        shell_name="sh"
    fi

    # Last exit code (captured by PROMPT_COMMAND hook)
    exit_code="${__AI_LAST_EXIT:-0}"

    # Detect distro and package manager
    if [[ -f /etc/os-release ]]; then
        distro=$(grep -oP '^ID=\K\w+' /etc/os-release 2>/dev/null || echo "linux")
    elif [[ "$(uname)" == "Darwin" ]]; then
        distro="macos"
    else
        distro="linux"
    fi

    case "$distro" in
        ubuntu|debian|pop|mint) pkg_mgr="apt" ;;
        fedora|rhel|centos|rocky|alma) pkg_mgr="dnf" ;;
        arch|manjaro|endeavouros) pkg_mgr="pacman" ;;
        opensuse*|suse) pkg_mgr="zypper" ;;
        alpine) pkg_mgr="apk" ;;
        macos) pkg_mgr="brew" ;;
        *) pkg_mgr="unknown" ;;
    esac

    # Git branch (null if not in repo)
    git_branch=""
    if type -P git >/dev/null 2>&1; then
        git_branch=$(git branch --show-current 2>/dev/null) || true
    fi

    # Top 8 files/dirs in cwd (names only)
    files_json=$(ls -1 2>/dev/null | head -8 | jq -R -s -c 'split("\n") | map(select(. != ""))')
    [[ -z "$files_json" ]] && files_json="[]"

    # Build JSON using jq for proper escaping
    jq -n -c \
        --arg cwd "$cwd" \
        --arg shell "$shell_name" \
        --arg distro "$distro" \
        --arg pkg "$pkg_mgr" \
        --arg git "$git_branch" \
        --argjson files "$files_json" \
        --argjson exit "$exit_code" \
        '{cwd:$cwd,shell:$shell,distro:$distro,pkg:$pkg,git:($git|if . == "" then null else . end),files:$files,exit:$exit}'
}

# Format and display AI response based on type
# Usage: __ai_format_response "json_response"
__ai_format_response() {
    local json="${1-}"

    local resp_type text command code lang issue suggestion
    resp_type=$(echo "$json" | jq -r '.type // "fallback"' 2>/dev/null)
    text=$(echo "$json" | jq -r '.text // ""' 2>/dev/null)
    command=$(echo "$json" | jq -r '.command // ""' 2>/dev/null)
    code=$(echo "$json" | jq -r '.code // ""' 2>/dev/null)
    lang=$(echo "$json" | jq -r '.lang // "bash"' 2>/dev/null)
    issue=$(echo "$json" | jq -r '.issue // ""' 2>/dev/null)
    suggestion=$(echo "$json" | jq -r '.suggestion // ""' 2>/dev/null)

    case "$resp_type" in
        command)
            [[ -n "$command" ]] && echo -e "${__GASH_CYAN}Command:${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}\`${command}\`${__GASH_COLOR_OFF}"
            [[ -n "$text" ]] && echo -e "${__GASH_GREEN}Explanation:${__GASH_COLOR_OFF} ${text}"
            ;;
        explanation)
            [[ -n "$text" ]] && echo -e "${__GASH_GREEN}Explanation:${__GASH_COLOR_OFF} ${text}"
            ;;
        troubleshoot)
            [[ -n "$issue" ]] && echo -e "${__GASH_RED}Issue:${__GASH_COLOR_OFF} ${issue}"
            [[ -n "$suggestion" ]] && echo -e "${__GASH_YELLOW}Suggestion:${__GASH_COLOR_OFF} ${suggestion}"
            # Show text only if no issue (as summary)
            { [[ -n "$text" && -z "$issue" ]] && echo "$text"; } || true
            ;;
        code)
            [[ -n "$text" ]] && echo -e "${__GASH_GREEN}${text}${__GASH_COLOR_OFF}"
            echo -e "${__GASH_CYAN}# ${lang}${__GASH_COLOR_OFF}"
            [[ -n "$code" ]] && echo "$code"
            ;;
        fallback|*)
            # Direct output, no labels
            [[ -n "$text" ]] && echo "$text"
            ;;
    esac
}

# =============================================================================
# API CALL FUNCTIONS
# =============================================================================

# Call Claude API with structured output
# Usage: __ai_call_claude <token> <query>
# Returns: JSON response with command and explanation
__ai_call_claude() {
    local token="${1-}"
    local query="${2-}"

    local context
    context="$(__ai_gather_context)"

    # Build request body with structured output using jq for proper escaping
    local body
    body=$(jq -n \
        --arg model "$__AI_CLAUDE_MODEL" \
        --arg system "$__AI_SYSTEM_PROMPT" \
        --arg context "Context: $context" \
        --arg query "$query" \
        --argjson schema "$__AI_RESPONSE_SCHEMA" \
        '{
            model: $model,
            max_tokens: 1024,
            system: $system,
            messages: [{role: "user", content: ($context + "\n\n" + $query)}],
            output_config: {
                format: {
                    type: "json_schema",
                    schema: $schema
                }
            }
        }')

    __ai_debug "Claude Request Body" "$(echo "$body" | jq -C . 2>/dev/null || echo "$body")"

    # Make API call
    local response http_code curl_exit
    local tmp_file tmp_err
    tmp_file=$(mktemp)
    tmp_err=$(mktemp)
    trap 'rm -f "$tmp_file" "$tmp_err" >/dev/null 2>&1' RETURN

    # Track start time for timeout differentiation
    local start_time elapsed_time curl_error __retry_count=0 __max_retries=3 __rc
    while true; do
        curl_exit=""
        start_time=$(date +%s)

        http_code=$(curl -s -w "%{http_code}" -o "$tmp_file" \
            --connect-timeout "$__AI_CONNECT_TIMEOUT" \
            --max-time "$__AI_RESPONSE_TIMEOUT" \
            -H "Content-Type: application/json" \
            -H "x-api-key: $token" \
            -H "anthropic-version: 2023-06-01" \
            -d "$body" \
            "$__AI_CLAUDE_API" 2>"$tmp_err") || curl_exit=$?

        elapsed_time=$(( $(date +%s) - start_time ))
        response=$(<"$tmp_file")
        curl_error=$(<"$tmp_err")

        __ai_debug "Claude HTTP Code" "$http_code (elapsed: ${elapsed_time}s)"
        __ai_debug "Claude Raw Response" "$(echo "$response" | jq -C . 2>/dev/null || echo "$response")"
        [[ -n "$curl_error" ]] && __ai_debug "Claude Curl Stderr" "$curl_error"

        # Handle curl/HTTP errors (rc=2 means 429 rate-limited, retry once)
        __ai_handle_curl_error "Claude" "${curl_exit:-}" "$http_code" "$elapsed_time" "$response" "${curl_error:-}"
        __rc=$?
        if [[ $__rc -eq 0 ]]; then break
        elif [[ $__rc -eq 2 && $__retry_count -lt $__max_retries ]]; then
            (( __retry_count++ ))
            __gash_warning "${__AI_RETRY_PROVIDER} API: Rate limited, waiting ${__AI_RETRY_DELAY:-5}s (retry ${__retry_count}/${__max_retries})..."
            sleep "${__AI_RETRY_DELAY:-5}"
            continue
        elif [[ $__rc -eq 2 ]]; then
            __gash_error "${__AI_RETRY_PROVIDER} API: Rate limit exceeded after ${__max_retries} retries"
            return 1
        else return 1; fi
    done

    # Extract response text (contains structured JSON)
    local text
    text=$(echo "$response" | jq -r '.content[0].text // empty' 2>/dev/null)

    __ai_debug "Claude Extracted JSON" "$(echo "$text" | jq -C . 2>/dev/null || echo "$text")"

    if [[ -z "$text" ]]; then
        __gash_error "Failed to parse Claude response"
        __ai_debug "Claude Parse Error" "Could not extract .content[0].text from response"
        return 1
    fi

    printf '%s' "$text"
}

# Call Gemini API with structured output
# Usage: __ai_call_gemini <token> <query>
# Returns: JSON response with command and explanation
__ai_call_gemini() {
    local token="${1-}"
    local query="${2-}"

    local context
    context="$(__ai_gather_context)"

    # Build request body with structured output using jq for proper escaping
    local body
    body=$(jq -n \
        --arg system "$__AI_SYSTEM_PROMPT" \
        --arg context "Context: $context" \
        --arg query "$query" \
        --argjson schema "$__AI_RESPONSE_SCHEMA" \
        '{
            contents: [{
                parts: [{text: ($system + "\n\n" + $context + "\n\n" + $query)}]
            }],
            generationConfig: {
                maxOutputTokens: 1024,
                responseMimeType: "application/json",
                responseJsonSchema: $schema
            }
        }')

    __ai_debug "Gemini Request Body" "$(echo "$body" | jq -C . 2>/dev/null || echo "$body")"

    # Make API call (token in query param)
    local response http_code curl_exit
    local tmp_file tmp_err
    tmp_file=$(mktemp)
    tmp_err=$(mktemp)
    trap 'rm -f "$tmp_file" "$tmp_err" >/dev/null 2>&1' RETURN

    # Track start time for timeout differentiation
    local start_time elapsed_time curl_error __retry_count=0 __max_retries=3 __rc
    while true; do
        curl_exit=""
        start_time=$(date +%s)

        http_code=$(curl -s -w "%{http_code}" -o "$tmp_file" \
            --connect-timeout "$__AI_CONNECT_TIMEOUT" \
            --max-time "$__AI_RESPONSE_TIMEOUT" \
            -H "Content-Type: application/json" \
            -d "$body" \
            "${__AI_GEMINI_API}?key=${token}" 2>"$tmp_err") || curl_exit=$?

        elapsed_time=$(( $(date +%s) - start_time ))
        response=$(<"$tmp_file")
        curl_error=$(<"$tmp_err")

        __ai_debug "Gemini HTTP Code" "$http_code (elapsed: ${elapsed_time}s)"
        __ai_debug "Gemini Raw Response" "$(echo "$response" | jq -C . 2>/dev/null || echo "$response")"
        [[ -n "$curl_error" ]] && __ai_debug "Gemini Curl Stderr" "$curl_error"

        # Handle curl/HTTP errors (rc=2 means 429 rate-limited, retry once)
        __ai_handle_curl_error "Gemini" "${curl_exit:-}" "$http_code" "$elapsed_time" "$response" "${curl_error:-}"
        __rc=$?
        if [[ $__rc -eq 0 ]]; then break
        elif [[ $__rc -eq 2 && $__retry_count -lt $__max_retries ]]; then
            (( __retry_count++ ))
            __gash_warning "${__AI_RETRY_PROVIDER} API: Rate limited, waiting ${__AI_RETRY_DELAY:-5}s (retry ${__retry_count}/${__max_retries})..."
            sleep "${__AI_RETRY_DELAY:-5}"
            continue
        elif [[ $__rc -eq 2 ]]; then
            __gash_error "${__AI_RETRY_PROVIDER} API: Rate limit exceeded after ${__max_retries} retries"
            return 1
        else return 1; fi
    done

    # Extract response text (contains structured JSON)
    local text
    text=$(echo "$response" | jq -r '.candidates[0].content.parts[0].text // empty' 2>/dev/null)

    __ai_debug "Gemini Extracted JSON" "$(echo "$text" | jq -C . 2>/dev/null || echo "$text")"

    if [[ -z "$text" ]]; then
        __gash_error "Failed to parse Gemini response"
        __ai_debug "Gemini Parse Error" "Could not extract .candidates[0].content.parts[0].text from response"
        return 1
    fi

    printf '%s' "$text"
}

# =============================================================================
# SYSINFO API WRAPPERS
# =============================================================================

# Call Claude API for sysinfo analysis.
# Usage: __sysinfo_call_claude <token> <data>
# Returns: JSON analysis response
__sysinfo_call_claude() {
    local token="${1-}"
    local data="${2-}"

    local body
    body=$(jq -n \
        --arg model "$__AI_CLAUDE_MODEL" \
        --arg system "$__AI_SYSINFO_SYSTEM_PROMPT" \
        --arg data "$data" \
        --argjson max_tokens "$__AI_SYSINFO_MAX_TOKENS" \
        --argjson schema "$__AI_SYSINFO_RESPONSE_SCHEMA" \
        '{
            model: $model,
            max_tokens: $max_tokens,
            system: $system,
            messages: [{role: "user", content: ("Analyze this system enumeration:\n\n" + $data)}],
            output_config: {
                format: {
                    type: "json_schema",
                    schema: $schema
                }
            }
        }')

    __ai_debug "Sysinfo Claude Request" "$(echo "$body" | jq -C . 2>/dev/null || echo "$body")"

    local response http_code curl_exit
    local tmp_file tmp_err
    tmp_file=$(mktemp)
    tmp_err=$(mktemp)
    trap 'rm -f "$tmp_file" "$tmp_err" >/dev/null 2>&1' RETURN

    local start_time elapsed_time curl_error __retry_count=0 __max_retries=3 __rc
    while true; do
        curl_exit=""
        start_time=$(date +%s)

        http_code=$(curl -s -w "%{http_code}" -o "$tmp_file" \
            --connect-timeout "$__AI_CONNECT_TIMEOUT" \
            --max-time "$__AI_SYSINFO_RESPONSE_TIMEOUT" \
            -H "Content-Type: application/json" \
            -H "x-api-key: $token" \
            -H "anthropic-version: 2023-06-01" \
            -d "$body" \
            "$__AI_CLAUDE_API" 2>"$tmp_err") || curl_exit=$?

        elapsed_time=$(( $(date +%s) - start_time ))
        response=$(<"$tmp_file")
        curl_error=$(<"$tmp_err")

        __ai_debug "Sysinfo Claude HTTP" "$http_code (${elapsed_time}s)"
        __ai_debug "Sysinfo Claude Response" "$(echo "$response" | jq -C . 2>/dev/null || echo "$response")"

        __ai_handle_curl_error "Claude" "${curl_exit:-}" "$http_code" "$elapsed_time" "$response" "${curl_error:-}"
        __rc=$?
        if [[ $__rc -eq 0 ]]; then break
        elif [[ $__rc -eq 2 && $__retry_count -lt $__max_retries ]]; then
            (( __retry_count++ ))
            __gash_warning "${__AI_RETRY_PROVIDER} API: Rate limited, waiting ${__AI_RETRY_DELAY:-5}s (retry ${__retry_count}/${__max_retries})..."
            sleep "${__AI_RETRY_DELAY:-5}"
            continue
        elif [[ $__rc -eq 2 ]]; then
            __gash_error "${__AI_RETRY_PROVIDER} API: Rate limit exceeded after ${__max_retries} retries"
            return 1
        else return 1; fi
    done

    local text
    text=$(echo "$response" | jq -r '.content[0].text // empty' 2>/dev/null)

    if [[ -z "$text" ]]; then
        __gash_error "Failed to parse Claude sysinfo response"
        return 1
    fi

    printf '%s' "$text"
}

# Call Gemini API for sysinfo analysis.
# Usage: __sysinfo_call_gemini <token> <data>
# Returns: JSON analysis response
__sysinfo_call_gemini() {
    local token="${1-}"
    local data="${2-}"

    local body
    body=$(jq -n \
        --arg system "$__AI_SYSINFO_SYSTEM_PROMPT" \
        --arg data "$data" \
        --argjson max_tokens "$__AI_SYSINFO_MAX_TOKENS" \
        '{
            contents: [{
                parts: [{text: ($system + "\n\nAnalyze this system enumeration:\n\n" + $data)}]
            }],
            generationConfig: {
                maxOutputTokens: $max_tokens,
                responseMimeType: "application/json"
            }
        }')

    __ai_debug "Sysinfo Gemini Request" "$(echo "$body" | jq -C . 2>/dev/null || echo "$body")"

    local response http_code curl_exit
    local tmp_file tmp_err
    tmp_file=$(mktemp)
    tmp_err=$(mktemp)
    trap 'rm -f "$tmp_file" "$tmp_err" >/dev/null 2>&1' RETURN

    local start_time elapsed_time curl_error __retry_count=0 __max_retries=3 __rc
    while true; do
        curl_exit=""
        start_time=$(date +%s)

        http_code=$(curl -s -w "%{http_code}" -o "$tmp_file" \
            --connect-timeout "$__AI_CONNECT_TIMEOUT" \
            --max-time "$__AI_SYSINFO_RESPONSE_TIMEOUT" \
            -H "Content-Type: application/json" \
            -d "$body" \
            "${__AI_GEMINI_API}?key=${token}" 2>"$tmp_err") || curl_exit=$?

        elapsed_time=$(( $(date +%s) - start_time ))
        response=$(<"$tmp_file")
        curl_error=$(<"$tmp_err")

        __ai_debug "Sysinfo Gemini HTTP" "$http_code (${elapsed_time}s)"
        __ai_debug "Sysinfo Gemini Response" "$(echo "$response" | jq -C . 2>/dev/null || echo "$response")"

        __ai_handle_curl_error "Gemini" "${curl_exit:-}" "$http_code" "$elapsed_time" "$response" "${curl_error:-}"
        __rc=$?
        if [[ $__rc -eq 0 ]]; then break
        elif [[ $__rc -eq 2 && $__retry_count -lt $__max_retries ]]; then
            (( __retry_count++ ))
            __gash_warning "${__AI_RETRY_PROVIDER} API: Rate limited, waiting ${__AI_RETRY_DELAY:-5}s (retry ${__retry_count}/${__max_retries})..."
            sleep "${__AI_RETRY_DELAY:-5}"
            continue
        elif [[ $__rc -eq 2 ]]; then
            __gash_error "${__AI_RETRY_PROVIDER} API: Rate limit exceeded after ${__max_retries} retries"
            return 1
        else return 1; fi
    done

    local text
    text=$(echo "$response" | jq -r '.candidates[0].content.parts[0].text // empty' 2>/dev/null)

    if [[ -z "$text" ]]; then
        __gash_error "Failed to parse Gemini sysinfo response"
        return 1
    fi

    printf '%s' "$text"
}

# =============================================================================
# DRILLDOWN API WRAPPERS
# =============================================================================

# Call Claude API for drilldown analysis.
# Usage: __sysinfo_call_drilldown_claude <token> <section_title> <initial_findings> <deep_data>
__sysinfo_call_drilldown_claude() {
    local token="${1-}" section_title="${2-}" initial_findings="${3-}" deep_data="${4-}"

    local user_msg
    user_msg=$(printf 'Section: %s\n\nInitial findings:\n%s\n\nDetailed data:\n%s' "$section_title" "$initial_findings" "$deep_data")

    local body
    body=$(jq -n \
        --arg model "$__AI_CLAUDE_MODEL" \
        --arg system "$__AI_SYSINFO_DRILLDOWN_PROMPT" \
        --arg user "$user_msg" \
        --argjson max_tokens "$__AI_SYSINFO_MAX_TOKENS" \
        --argjson schema "$__AI_SYSINFO_DRILLDOWN_SCHEMA" \
        '{
            model: $model,
            max_tokens: $max_tokens,
            system: $system,
            messages: [{role: "user", content: $user}],
            output_config: {
                format: {
                    type: "json_schema",
                    schema: $schema
                }
            }
        }')

    __ai_debug "Drilldown Claude Request" "$(echo "$body" | jq -C . 2>/dev/null || echo "$body")"

    local response http_code curl_exit
    local tmp_file tmp_err
    tmp_file=$(mktemp)
    tmp_err=$(mktemp)
    trap 'rm -f "$tmp_file" "$tmp_err" >/dev/null 2>&1' RETURN

    local start_time elapsed_time curl_error __retry_count=0 __max_retries=3 __rc
    while true; do
        curl_exit=""
        start_time=$(date +%s)

        http_code=$(curl -s -w "%{http_code}" -o "$tmp_file" \
            --connect-timeout "$__AI_CONNECT_TIMEOUT" \
            --max-time "$__AI_SYSINFO_RESPONSE_TIMEOUT" \
            -H "Content-Type: application/json" \
            -H "x-api-key: $token" \
            -H "anthropic-version: 2023-06-01" \
            -d "$body" \
            "$__AI_CLAUDE_API" 2>"$tmp_err") || curl_exit=$?

        elapsed_time=$(( $(date +%s) - start_time ))
        response=$(<"$tmp_file")
        curl_error=$(<"$tmp_err")

        __ai_debug "Drilldown Claude HTTP" "$http_code (${elapsed_time}s)"
        __ai_debug "Drilldown Claude Response" "$(echo "$response" | jq -C . 2>/dev/null || echo "$response")"

        __ai_handle_curl_error "Claude" "${curl_exit:-}" "$http_code" "$elapsed_time" "$response" "${curl_error:-}"
        __rc=$?
        if [[ $__rc -eq 0 ]]; then break
        elif [[ $__rc -eq 2 && $__retry_count -lt $__max_retries ]]; then
            (( __retry_count++ ))
            __gash_warning "${__AI_RETRY_PROVIDER} API: Rate limited, waiting ${__AI_RETRY_DELAY:-5}s (retry ${__retry_count}/${__max_retries})..."
            sleep "${__AI_RETRY_DELAY:-5}"
            continue
        elif [[ $__rc -eq 2 ]]; then
            __gash_error "${__AI_RETRY_PROVIDER} API: Rate limit exceeded after ${__max_retries} retries"
            return 1
        else return 1; fi
    done

    local text
    text=$(echo "$response" | jq -r '.content[0].text // empty' 2>/dev/null)

    if [[ -z "$text" ]]; then
        __gash_error "Failed to parse Claude drilldown response"
        return 1
    fi

    printf '%s' "$text"
}

# Call Gemini API for drilldown analysis.
# Usage: __sysinfo_call_drilldown_gemini <token> <section_title> <initial_findings> <deep_data>
__sysinfo_call_drilldown_gemini() {
    local token="${1-}" section_title="${2-}" initial_findings="${3-}" deep_data="${4-}"

    local user_msg
    user_msg=$(printf 'Section: %s\n\nInitial findings:\n%s\n\nDetailed data:\n%s' "$section_title" "$initial_findings" "$deep_data")

    local body
    body=$(jq -n \
        --arg system "$__AI_SYSINFO_DRILLDOWN_PROMPT" \
        --arg user "$user_msg" \
        --argjson max_tokens "$__AI_SYSINFO_MAX_TOKENS" \
        '{
            contents: [{
                parts: [{text: ($system + "\n\n" + $user)}]
            }],
            generationConfig: {
                maxOutputTokens: $max_tokens,
                responseMimeType: "application/json"
            }
        }')

    __ai_debug "Drilldown Gemini Request" "$(echo "$body" | jq -C . 2>/dev/null || echo "$body")"

    local response http_code curl_exit
    local tmp_file tmp_err
    tmp_file=$(mktemp)
    tmp_err=$(mktemp)
    trap 'rm -f "$tmp_file" "$tmp_err" >/dev/null 2>&1' RETURN

    local start_time elapsed_time curl_error __retry_count=0 __max_retries=3 __rc
    while true; do
        curl_exit=""
        start_time=$(date +%s)

        http_code=$(curl -s -w "%{http_code}" -o "$tmp_file" \
            --connect-timeout "$__AI_CONNECT_TIMEOUT" \
            --max-time "$__AI_SYSINFO_RESPONSE_TIMEOUT" \
            -H "Content-Type: application/json" \
            -d "$body" \
            "${__AI_GEMINI_API}?key=${token}" 2>"$tmp_err") || curl_exit=$?

        elapsed_time=$(( $(date +%s) - start_time ))
        response=$(<"$tmp_file")
        curl_error=$(<"$tmp_err")

        __ai_debug "Drilldown Gemini HTTP" "$http_code (${elapsed_time}s)"
        __ai_debug "Drilldown Gemini Response" "$(echo "$response" | jq -C . 2>/dev/null || echo "$response")"

        __ai_handle_curl_error "Gemini" "${curl_exit:-}" "$http_code" "$elapsed_time" "$response" "${curl_error:-}"
        __rc=$?
        if [[ $__rc -eq 0 ]]; then break
        elif [[ $__rc -eq 2 && $__retry_count -lt $__max_retries ]]; then
            (( __retry_count++ ))
            __gash_warning "${__AI_RETRY_PROVIDER} API: Rate limited, waiting ${__AI_RETRY_DELAY:-5}s (retry ${__retry_count}/${__max_retries})..."
            sleep "${__AI_RETRY_DELAY:-5}"
            continue
        elif [[ $__rc -eq 2 ]]; then
            __gash_error "${__AI_RETRY_PROVIDER} API: Rate limit exceeded after ${__max_retries} retries"
            return 1
        else return 1; fi
    done

    local text
    text=$(echo "$response" | jq -r '.candidates[0].content.parts[0].text // empty' 2>/dev/null)

    if [[ -z "$text" ]]; then
        __gash_error "Failed to parse Gemini drilldown response"
        return 1
    fi

    printf '%s' "$text"
}

# Collect targeted context data based on entities detected in the user's question.
# Extracts systemd units, file paths, port numbers, and known service keywords,
# then runs read-only commands to gather relevant data for each.
# Usage: __sysinfo_collect_question_context <question>
# Returns: collected text on stdout (empty if no entities detected)
__sysinfo_collect_question_context() {
    local question="${1-}"
    [[ -z "$question" ]] && return 0

    local context=""
    local lower_q="${question,,}"
    declare -A seen

    # Pattern 1: Systemd units (word.service/timer/socket/mount/path/target/slice/scope)
    local unit_regex='[a-zA-Z0-9_@.-]+\.(service|timer|socket|mount|path|target|slice|scope)'
    local remaining="$question"
    while [[ "$remaining" =~ ($unit_regex) ]]; do
        local unit="${BASH_REMATCH[1]}"
        remaining="${remaining#*"$unit"}"
        [[ -n "${seen[$unit]-}" ]] && continue
        seen[$unit]=1

        context+="=== SYSTEMD UNIT: ${unit} ==="$'\n'
        context+="--- Unit file ---"$'\n'
        context+="$(systemctl cat "$unit" 2>/dev/null | head -60)"$'\n'
        context+="--- Status ---"$'\n'
        context+="$(systemctl status "$unit" --no-pager 2>/dev/null | head -30)"$'\n'
        context+="--- Journal (last 50 lines) ---"$'\n'
        context+="$(__sysinfo_sudo journalctl -u "$unit" -n 50 --no-pager 2>/dev/null | head -50)"$'\n'
        context+=$'\n'
    done

    # Pattern 2: File paths (/etc/..., /var/..., /usr/..., /opt/..., /srv/..., /home/..., /run/...)
    local path_regex='/(etc|var|usr|opt|srv|home|run)/[^ "'"'"'`|;&<>(){}]+'
    remaining="$question"
    while [[ "$remaining" =~ ($path_regex) ]]; do
        local fpath="${BASH_REMATCH[1]}"
        remaining="${remaining#*"$fpath"}"
        # Strip trailing punctuation (common in natural language questions)
        fpath="${fpath%%[?.!,:;)\"\']+([?.!,:;)\"\'])}"
        fpath="${fpath%[?.!,:;\)\"\']}"
        [[ -n "${seen[$fpath]-}" ]] && continue
        seen[$fpath]=1

        if [[ -f "$fpath" ]]; then
            context+="=== FILE: ${fpath} ==="$'\n'
            context+="$(head -200 "$fpath" 2>/dev/null)"$'\n'
            context+=$'\n'
        elif [[ -d "$fpath" ]]; then
            context+="=== DIRECTORY: ${fpath} ==="$'\n'
            context+="$(ls -la "$fpath" 2>/dev/null | head -50)"$'\n'
            context+=$'\n'
        fi
    done

    # Pattern 3: Port numbers (port 3306, port:3306, :3306)
    local port_regex='(port[: ]+|:)([0-9]{1,5})\b'
    remaining="$lower_q"
    while [[ "$remaining" =~ ($port_regex) ]]; do
        local port="${BASH_REMATCH[3]}"
        remaining="${remaining#*"${BASH_REMATCH[1]}"}"
        [[ -z "$port" || -n "${seen[port:$port]-}" ]] && continue
        seen[port:$port]=1

        context+="=== PORT: ${port} ==="$'\n'
        context+="$(ss -tlnp 2>/dev/null | grep -F ":${port}" | head -10)"$'\n'
        context+=$'\n'
    done

    # Pattern 4: Known service keywords → targeted collection
    if [[ "$lower_q" =~ apache|httpd ]] && [[ -z "${seen[svc:apache]-}" ]]; then
        seen[svc:apache]=1
        context+="=== SERVICE: Apache2 ==="$'\n'
        context+="--- Status ---"$'\n'
        context+="$(systemctl status apache2 --no-pager 2>/dev/null | head -20)"$'\n'
        if [[ -f /etc/apache2/apache2.conf ]]; then
            context+="--- /etc/apache2/apache2.conf (key lines) ---"$'\n'
            context+="$(grep -Ev '^(#|$)' /etc/apache2/apache2.conf 2>/dev/null | head -40)"$'\n'
        fi
        if [[ -d /etc/apache2/sites-enabled ]]; then
            context+="--- sites-enabled ---"$'\n'
            context+="$(ls -1 /etc/apache2/sites-enabled/ 2>/dev/null)"$'\n'
        fi
        if [[ -d /etc/apache2/mods-enabled ]]; then
            context+="--- mods-enabled ---"$'\n'
            context+="$(ls -1 /etc/apache2/mods-enabled/*.load 2>/dev/null | xargs -I{} basename {} .load)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ nginx ]] && [[ -z "${seen[svc:nginx]-}" ]]; then
        seen[svc:nginx]=1
        context+="=== SERVICE: Nginx ==="$'\n'
        context+="$(systemctl status nginx --no-pager 2>/dev/null | head -20)"$'\n'
        if [[ -f /etc/nginx/nginx.conf ]]; then
            context+="--- /etc/nginx/nginx.conf ---"$'\n'
            context+="$(grep -Ev '^(#|$)' /etc/nginx/nginx.conf 2>/dev/null | head -40)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ mysql|mariadb ]] && [[ -z "${seen[svc:mysql]-}" ]]; then
        seen[svc:mysql]=1
        context+="=== SERVICE: MariaDB/MySQL ==="$'\n'
        context+="$(systemctl status mariadb --no-pager 2>/dev/null | head -20)"$'\n'
        if [[ -d /etc/mysql ]]; then
            context+="--- /etc/mysql/ config files ---"$'\n'
            local cnf
            for cnf in /etc/mysql/mariadb.conf.d/*.cnf /etc/mysql/mysql.conf.d/*.cnf /etc/mysql/conf.d/*.cnf; do
                [[ -f "$cnf" ]] || continue
                context+="--- ${cnf} ---"$'\n'
                context+="$(grep -Ev '^(#|;|$)' "$cnf" 2>/dev/null | head -30)"$'\n'
            done
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ redis ]] && [[ -z "${seen[svc:redis]-}" ]]; then
        seen[svc:redis]=1
        context+="=== SERVICE: Redis ==="$'\n'
        context+="$(systemctl status redis-server --no-pager 2>/dev/null | head -20)"$'\n'
        if [[ -f /etc/redis/redis.conf ]]; then
            context+="--- /etc/redis/redis.conf (key lines) ---"$'\n'
            context+="$(grep -Ev '^(#|$)' /etc/redis/redis.conf 2>/dev/null | head -30)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ php|fpm ]] && [[ -z "${seen[svc:php]-}" ]]; then
        seen[svc:php]=1
        context+="=== SERVICE: PHP-FPM ==="$'\n'
        local phpver
        for phpver in /etc/php/*/fpm; do
            [[ -d "$phpver" ]] || continue
            local ver="${phpver%/fpm}"
            ver="${ver##*/}"
            context+="--- PHP ${ver} FPM ---"$'\n'
            context+="$(systemctl status "php${ver}-fpm" --no-pager 2>/dev/null | head -10)"$'\n'
            if [[ -d "${phpver}/pool.d" ]]; then
                local pool
                for pool in "${phpver}/pool.d"/*.conf; do
                    [[ -f "$pool" ]] || continue
                    context+="--- $(basename "$pool") ---"$'\n'
                    context+="$(grep -Ev '^(;|$)' "$pool" 2>/dev/null | head -20)"$'\n'
                done
            fi
        done
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ postfix|mail ]] && [[ -z "${seen[svc:postfix]-}" ]]; then
        seen[svc:postfix]=1
        context+="=== SERVICE: Postfix ==="$'\n'
        context+="$(systemctl status postfix --no-pager 2>/dev/null | head -20)"$'\n'
        if [[ -f /etc/postfix/main.cf ]]; then
            context+="--- /etc/postfix/main.cf (key lines) ---"$'\n'
            context+="$(grep -Ev '^(#|$)' /etc/postfix/main.cf 2>/dev/null | head -40)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ ssh|sshd ]] && [[ -z "${seen[svc:ssh]-}" ]]; then
        seen[svc:ssh]=1
        context+="=== SERVICE: SSH ==="$'\n'
        context+="$(systemctl status sshd --no-pager 2>/dev/null | head -20)"$'\n'
        if [[ -f /etc/ssh/sshd_config ]]; then
            context+="--- /etc/ssh/sshd_config (key lines) ---"$'\n'
            context+="$(__sysinfo_sudo grep -Ev '^(#|$)' /etc/ssh/sshd_config 2>/dev/null | head -30)"$'\n'
        fi
        if [[ -d /etc/ssh/sshd_config.d ]]; then
            context+="--- sshd_config.d/ ---"$'\n'
            context+="$(ls -1 /etc/ssh/sshd_config.d/ 2>/dev/null)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ fail2ban ]] && [[ -z "${seen[svc:fail2ban]-}" ]]; then
        seen[svc:fail2ban]=1
        context+="=== SERVICE: Fail2ban ==="$'\n'
        context+="$(systemctl status fail2ban --no-pager 2>/dev/null | head -20)"$'\n'
        if [[ -f /etc/fail2ban/jail.local ]]; then
            context+="--- /etc/fail2ban/jail.local ---"$'\n'
            context+="$(grep -Ev '^(#|;|$)' /etc/fail2ban/jail.local 2>/dev/null | head -40)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ docker ]] && [[ -z "${seen[svc:docker]-}" ]]; then
        seen[svc:docker]=1
        context+="=== SERVICE: Docker ==="$'\n'
        context+="$(systemctl status docker --no-pager 2>/dev/null | head -20)"$'\n'
        if [[ -f /etc/docker/daemon.json ]]; then
            context+="--- /etc/docker/daemon.json ---"$'\n'
            context+="$(cat /etc/docker/daemon.json 2>/dev/null)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ postgres ]] && [[ -z "${seen[svc:postgres]-}" ]]; then
        seen[svc:postgres]=1
        context+="=== SERVICE: PostgreSQL ==="$'\n'
        context+="$(systemctl status postgresql --no-pager 2>/dev/null | head -20)"$'\n'
        local pgconf
        pgconf=$(find /etc/postgresql -name "pg_hba.conf" 2>/dev/null | head -1)
        if [[ -n "$pgconf" ]]; then
            context+="--- ${pgconf} ---"$'\n'
            context+="$(grep -Ev '^(#|$)' "$pgconf" 2>/dev/null | head -20)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ cron ]] && [[ -z "${seen[svc:cron]-}" ]]; then
        seen[svc:cron]=1
        context+="=== SERVICE: Cron ==="$'\n'
        context+="$(crontab -l 2>/dev/null || echo '(no user crontab)')"$'\n'
        if [[ -d /etc/cron.d ]]; then
            context+="--- /etc/cron.d/ ---"$'\n'
            context+="$(ls -1 /etc/cron.d/ 2>/dev/null)"$'\n'
        fi
        context+=$'\n'
    fi

    # Pattern 5: Potential service names without .service suffix (e.g., "viper-backup", "wsl-pro")
    # Detect hyphenated/underscored words that could be systemd unit names
    local svc_name_regex='[a-zA-Z][a-zA-Z0-9]*[-_][a-zA-Z0-9][-a-zA-Z0-9_]*'
    remaining="$question"
    while [[ "$remaining" =~ ($svc_name_regex) ]]; do
        local svc_name="${BASH_REMATCH[1]}"
        remaining="${remaining#*"$svc_name"}"
        # Skip if already collected as a systemd unit or known service
        [[ -n "${seen[${svc_name}.service]-}" ]] && continue
        [[ -n "${seen[svc:${svc_name,,}]-}" ]] && continue

        # Try as a systemd service — only collect if the unit actually exists
        local unit_output
        unit_output=$(systemctl cat "${svc_name}.service" 2>/dev/null | head -60) || true
        if [[ -n "$unit_output" ]]; then
            seen[${svc_name}.service]=1
            context+="=== SYSTEMD UNIT: ${svc_name}.service ==="$'\n'
            context+="--- Unit file ---"$'\n'
            context+="$unit_output"$'\n'
            context+="--- Status ---"$'\n'
            context+="$(systemctl status "${svc_name}.service" --no-pager 2>/dev/null | head -30)"$'\n'
            context+="--- Journal (last 50 lines) ---"$'\n'
            context+="$(__sysinfo_sudo journalctl -u "${svc_name}.service" -n 50 --no-pager 2>/dev/null | head -50)"$'\n'
            context+=$'\n'
        fi
    done

    printf '%s' "$context"
}

# Call Claude API for free-text question about the server.
# Usage: __sysinfo_call_freetext_claude <token> <question> <initial_json> [context_data]
__sysinfo_call_freetext_claude() {
    local token="${1-}" question="${2-}" initial_json="${3-}" context_data="${4-}"

    local user_msg
    if [[ -n "$context_data" ]]; then
        user_msg=$(printf 'Initial server analysis:\n%s\n\nDetailed system data for this question:\n%s\n\nUser question: %s\n\nAnswer the question using all data provided above. Group your answer into relevant components.' "$initial_json" "$context_data" "$question")
    else
        user_msg=$(printf 'Initial server analysis:\n%s\n\nUser question: %s\n\nAnswer the question based on the server analysis above. Group your answer into relevant components.' "$initial_json" "$question")
    fi

    local body
    body=$(jq -n \
        --arg model "$__AI_CLAUDE_MODEL" \
        --arg system "$__AI_SYSINFO_DRILLDOWN_PROMPT" \
        --arg user "$user_msg" \
        --argjson max_tokens "$__AI_SYSINFO_MAX_TOKENS" \
        --argjson schema "$__AI_SYSINFO_DRILLDOWN_SCHEMA" \
        '{
            model: $model,
            max_tokens: $max_tokens,
            system: $system,
            messages: [{role: "user", content: $user}],
            output_config: {
                format: {
                    type: "json_schema",
                    schema: $schema
                }
            }
        }')

    __ai_debug "Freetext Claude Request" "$(echo "$body" | jq -C . 2>/dev/null || echo "$body")"

    local response http_code curl_exit
    local tmp_file tmp_err
    tmp_file=$(mktemp)
    tmp_err=$(mktemp)
    trap 'rm -f "$tmp_file" "$tmp_err" >/dev/null 2>&1' RETURN

    local start_time elapsed_time curl_error __retry_count=0 __max_retries=3 __rc
    while true; do
        curl_exit=""
        start_time=$(date +%s)

        http_code=$(curl -s -w "%{http_code}" -o "$tmp_file" \
            --connect-timeout "$__AI_CONNECT_TIMEOUT" \
            --max-time "$__AI_SYSINFO_RESPONSE_TIMEOUT" \
            -H "Content-Type: application/json" \
            -H "x-api-key: $token" \
            -H "anthropic-version: 2023-06-01" \
            -d "$body" \
            "$__AI_CLAUDE_API" 2>"$tmp_err") || curl_exit=$?

        elapsed_time=$(( $(date +%s) - start_time ))
        response=$(<"$tmp_file")
        curl_error=$(<"$tmp_err")

        __ai_debug "Freetext Claude HTTP" "$http_code (${elapsed_time}s)"
        __ai_debug "Freetext Claude Response" "$(echo "$response" | jq -C . 2>/dev/null || echo "$response")"

        __ai_handle_curl_error "Claude" "${curl_exit:-}" "$http_code" "$elapsed_time" "$response" "${curl_error:-}"
        __rc=$?
        if [[ $__rc -eq 0 ]]; then break
        elif [[ $__rc -eq 2 && $__retry_count -lt $__max_retries ]]; then
            (( __retry_count++ ))
            __gash_warning "${__AI_RETRY_PROVIDER} API: Rate limited, waiting ${__AI_RETRY_DELAY:-5}s (retry ${__retry_count}/${__max_retries})..."
            sleep "${__AI_RETRY_DELAY:-5}"
            continue
        elif [[ $__rc -eq 2 ]]; then
            __gash_error "${__AI_RETRY_PROVIDER} API: Rate limit exceeded after ${__max_retries} retries"
            return 1
        else return 1; fi
    done

    local text
    text=$(echo "$response" | jq -r '.content[0].text // empty' 2>/dev/null)

    if [[ -z "$text" ]]; then
        __gash_error "Failed to parse Claude freetext response"
        return 1
    fi

    printf '%s' "$text"
}

# Call Gemini API for free-text question about the server.
# Usage: __sysinfo_call_freetext_gemini <token> <question> <initial_json> [context_data]
__sysinfo_call_freetext_gemini() {
    local token="${1-}" question="${2-}" initial_json="${3-}" context_data="${4-}"

    local user_msg
    if [[ -n "$context_data" ]]; then
        user_msg=$(printf 'Initial server analysis:\n%s\n\nDetailed system data for this question:\n%s\n\nUser question: %s\n\nAnswer the question using all data provided above. Group your answer into relevant components.' "$initial_json" "$context_data" "$question")
    else
        user_msg=$(printf 'Initial server analysis:\n%s\n\nUser question: %s\n\nAnswer the question based on the server analysis above. Group your answer into relevant components.' "$initial_json" "$question")
    fi

    local body
    body=$(jq -n \
        --arg system "$__AI_SYSINFO_DRILLDOWN_PROMPT" \
        --arg user "$user_msg" \
        --argjson max_tokens "$__AI_SYSINFO_MAX_TOKENS" \
        '{
            contents: [{
                parts: [{text: ($system + "\n\n" + $user)}]
            }],
            generationConfig: {
                maxOutputTokens: $max_tokens,
                responseMimeType: "application/json"
            }
        }')

    __ai_debug "Freetext Gemini Request" "$(echo "$body" | jq -C . 2>/dev/null || echo "$body")"

    local response http_code curl_exit
    local tmp_file tmp_err
    tmp_file=$(mktemp)
    tmp_err=$(mktemp)
    trap 'rm -f "$tmp_file" "$tmp_err" >/dev/null 2>&1' RETURN

    local start_time elapsed_time curl_error __retry_count=0 __max_retries=3 __rc
    while true; do
        curl_exit=""
        start_time=$(date +%s)

        http_code=$(curl -s -w "%{http_code}" -o "$tmp_file" \
            --connect-timeout "$__AI_CONNECT_TIMEOUT" \
            --max-time "$__AI_SYSINFO_RESPONSE_TIMEOUT" \
            -H "Content-Type: application/json" \
            -d "$body" \
            "${__AI_GEMINI_API}?key=${token}" 2>"$tmp_err") || curl_exit=$?

        elapsed_time=$(( $(date +%s) - start_time ))
        response=$(<"$tmp_file")
        curl_error=$(<"$tmp_err")

        __ai_debug "Freetext Gemini HTTP" "$http_code (${elapsed_time}s)"
        __ai_debug "Freetext Gemini Response" "$(echo "$response" | jq -C . 2>/dev/null || echo "$response")"

        __ai_handle_curl_error "Gemini" "${curl_exit:-}" "$http_code" "$elapsed_time" "$response" "${curl_error:-}"
        __rc=$?
        if [[ $__rc -eq 0 ]]; then break
        elif [[ $__rc -eq 2 && $__retry_count -lt $__max_retries ]]; then
            (( __retry_count++ ))
            __gash_warning "${__AI_RETRY_PROVIDER} API: Rate limited, waiting ${__AI_RETRY_DELAY:-5}s (retry ${__retry_count}/${__max_retries})..."
            sleep "${__AI_RETRY_DELAY:-5}"
            continue
        elif [[ $__rc -eq 2 ]]; then
            __gash_error "${__AI_RETRY_PROVIDER} API: Rate limit exceeded after ${__max_retries} retries"
            return 1
        else return 1; fi
    done

    local text
    text=$(echo "$response" | jq -r '.candidates[0].content.parts[0].text // empty' 2>/dev/null)

    if [[ -z "$text" ]]; then
        __gash_error "Failed to parse Gemini freetext response"
        return 1
    fi

    printf '%s' "$text"
}

# Format and display AI sysinfo analysis response.
# Usage: __sysinfo_format_ai_response <json>
__sysinfo_format_ai_response() {
    local json="$1"
    local MUTED='\033[0;90m'
    local BOLD_RED='\033[1;31m'

    # Validate JSON
    if ! echo "$json" | jq empty 2>/dev/null; then
        __gash_error "Invalid JSON response from AI"
        __ai_debug "Invalid JSON" "$json"
        return 1
    fi

    local hostname platform
    hostname=$(echo "$json" | jq -r '.hostname // "unknown"')
    platform=$(echo "$json" | jq -r '.platform // "unknown"')

    echo
    echo -e "${__GASH_BOLD_WHITE}Server: ${hostname}${__GASH_COLOR_OFF}"
    echo -e "${MUTED}${platform}${__GASH_COLOR_OFF}"
    echo -e "${MUTED}$(printf '%.0s-' {1..60})${__GASH_COLOR_OFF}"

    # Count totals
    local critical warning info ok_count
    critical=$(echo "$json" | jq '[.sections[].findings[] | select(.severity=="critical")] | length')
    warning=$(echo "$json" | jq '[.sections[].findings[] | select(.severity=="warning")] | length')
    info=$(echo "$json" | jq '[.sections[].findings[] | select(.severity=="info")] | length')
    ok_count=$(echo "$json" | jq '[.sections[].findings[] | select(.severity=="ok")] | length')

    echo -e "${BOLD_RED}${critical} critical${__GASH_COLOR_OFF}  ${__GASH_BOLD_YELLOW}${warning} warnings${__GASH_COLOR_OFF}  ${__GASH_CYAN}${info} info${__GASH_COLOR_OFF}  ${__GASH_GREEN}${ok_count} ok${__GASH_COLOR_OFF}"
    echo

    # Iterate sections
    local section_count
    section_count=$(echo "$json" | jq '.sections | length')

    local i=0
    while [[ $i -lt $section_count ]]; do
        local title
        title=$(echo "$json" | jq -r ".sections[$i].title")

        echo -e "${__GASH_BOLD_WHITE}${title}${__GASH_COLOR_OFF}"

        local findings_count
        findings_count=$(echo "$json" | jq ".sections[$i].findings | length")

        local j=0
        while [[ $j -lt $findings_count ]]; do
            local sev text
            sev=$(echo "$json" | jq -r ".sections[$i].findings[$j].severity")
            text=$(echo "$json" | jq -r ".sections[$i].findings[$j].text")

            case "$sev" in
                critical) echo -e "  ${BOLD_RED}[!]${__GASH_COLOR_OFF} ${text}" ;;
                warning)  echo -e "  ${__GASH_BOLD_YELLOW}[!]${__GASH_COLOR_OFF} ${text}" ;;
                ok)       echo -e "  ${__GASH_GREEN}[+]${__GASH_COLOR_OFF} ${text}" ;;
                *)        echo -e "  ${__GASH_CYAN}[i]${__GASH_COLOR_OFF} ${text}" ;;
            esac

            j=$((j + 1))
        done
        echo

        i=$((i + 1))
    done
}

# Format and display drilldown analysis response (component-based).
# Usage: __sysinfo_format_drilldown_response <json>
__sysinfo_format_drilldown_response() {
    local json="$1"
    local MUTED='\033[0;90m'
    local BOLD_RED='\033[1;31m'

    # Validate JSON
    if ! echo "$json" | jq empty 2>/dev/null; then
        __gash_error "Invalid JSON response from AI"
        __ai_debug "Invalid JSON" "$json"
        return 1
    fi

    local title summary
    title=$(echo "$json" | jq -r '.title // "Analysis"')
    summary=$(echo "$json" | jq -r '.summary // ""')

    echo
    echo -e "${__GASH_BOLD_WHITE}${title}${__GASH_COLOR_OFF}"
    [[ -n "$summary" ]] && echo -e "${MUTED}${summary}${__GASH_COLOR_OFF}"
    echo -e "${MUTED}$(printf '%.0s-' {1..60})${__GASH_COLOR_OFF}"
    echo

    local comp_count
    comp_count=$(echo "$json" | jq '.components | length')

    local i=0
    while [[ $i -lt $comp_count ]]; do
        local name status
        name=$(echo "$json" | jq -r ".components[$i].name")
        status=$(echo "$json" | jq -r ".components[$i].status")

        # Status color
        local status_color
        case "$status" in
            running)   status_color="${__GASH_GREEN}" ;;
            installed) status_color="${__GASH_CYAN}" ;;
            stopped)   status_color="${__GASH_BOLD_YELLOW}" ;;
            eol|error) status_color="${BOLD_RED}" ;;
            *)         status_color="${MUTED}" ;;
        esac

        echo -e "${__GASH_BOLD_WHITE}${name}${__GASH_COLOR_OFF} (${status_color}${status}${__GASH_COLOR_OFF})"

        # Config highlights
        local highlights_count
        highlights_count=$(echo "$json" | jq ".components[$i].config_highlights | length")
        local h=0
        while [[ $h -lt $highlights_count ]]; do
            local highlight
            highlight=$(echo "$json" | jq -r ".components[$i].config_highlights[$h]")
            echo -e "  ${MUTED}${highlight}${__GASH_COLOR_OFF}"
            h=$((h + 1))
        done

        # Issues
        local issues_count
        issues_count=$(echo "$json" | jq ".components[$i].issues | length")
        local j=0
        while [[ $j -lt $issues_count ]]; do
            local sev text
            sev=$(echo "$json" | jq -r ".components[$i].issues[$j].severity")
            text=$(echo "$json" | jq -r ".components[$i].issues[$j].text")

            case "$sev" in
                critical) echo -e "  ${BOLD_RED}[!]${__GASH_COLOR_OFF} ${text}" ;;
                warning)  echo -e "  ${__GASH_BOLD_YELLOW}[!]${__GASH_COLOR_OFF} ${text}" ;;
                ok)       echo -e "  ${__GASH_GREEN}[+]${__GASH_COLOR_OFF} ${text}" ;;
                *)        echo -e "  ${__GASH_CYAN}[i]${__GASH_COLOR_OFF} ${text}" ;;
            esac

            j=$((j + 1))
        done
        echo

        i=$((i + 1))
    done
}

# Generate markdown for the initial AI sysinfo analysis.
# Usage: __sysinfo_md_initial <json> <provider>
# Returns: markdown string (no ANSI)
__sysinfo_md_initial() {
    local json="$1" provider="${2-unknown}"
    local md=""

    local hostname platform
    hostname=$(echo "$json" | jq -r '.hostname // "unknown"')
    platform=$(echo "$json" | jq -r '.platform // "unknown"')

    local critical warning info_count ok_count
    critical=$(echo "$json" | jq '[.sections[].findings[] | select(.severity=="critical")] | length')
    warning=$(echo "$json" | jq '[.sections[].findings[] | select(.severity=="warning")] | length')
    info_count=$(echo "$json" | jq '[.sections[].findings[] | select(.severity=="info")] | length')
    ok_count=$(echo "$json" | jq '[.sections[].findings[] | select(.severity=="ok")] | length')

    local provider_cap
    provider_cap="$(echo "$provider" | sed 's/.*/\u&/')"

    md+="# System Analysis Report"$'\n\n'
    md+="**Generated:** $(date '+%Y-%m-%d %H:%M:%S') | **Provider:** ${provider_cap}"$'\n\n'
    md+="## Server: ${hostname}"$'\n'
    md+="*${platform}*"$'\n\n'
    md+="**Summary:** ${critical} critical, ${warning} warnings, ${info_count} info, ${ok_count} ok"$'\n\n'

    local section_count
    section_count=$(echo "$json" | jq '.sections | length')

    local i=0
    while [[ $i -lt $section_count ]]; do
        local title
        title=$(echo "$json" | jq -r ".sections[$i].title")
        md+="### ${title}"$'\n'

        local findings_count
        findings_count=$(echo "$json" | jq ".sections[$i].findings | length")

        local j=0
        while [[ $j -lt $findings_count ]]; do
            local sev text icon
            sev=$(echo "$json" | jq -r ".sections[$i].findings[$j].severity")
            text=$(echo "$json" | jq -r ".sections[$i].findings[$j].text")
            case "$sev" in
                critical|warning) icon="[!]" ;;
                ok)               icon="[+]" ;;
                *)                icon="[i]" ;;
            esac
            md+="- **${icon}** ${text}"$'\n'
            j=$((j + 1))
        done
        md+=$'\n'

        i=$((i + 1))
    done

    printf '%s' "$md"
}

# Generate markdown for a drilldown response.
# Usage: __sysinfo_md_drilldown <json>
# Returns: markdown string (no ANSI)
__sysinfo_md_drilldown() {
    local json="$1"
    local md=""

    local title summary
    title=$(echo "$json" | jq -r '.title // "Analysis"')
    summary=$(echo "$json" | jq -r '.summary // ""')

    md+=$'\n'"---"$'\n\n'
    md+="## Drill-down: ${title}"$'\n'
    [[ -n "$summary" ]] && md+="*${summary}*"$'\n'
    md+=$'\n'

    local comp_count
    comp_count=$(echo "$json" | jq '.components | length')

    local i=0
    while [[ $i -lt $comp_count ]]; do
        local name status
        name=$(echo "$json" | jq -r ".components[$i].name")
        status=$(echo "$json" | jq -r ".components[$i].status")

        md+="### ${name} (\`${status}\`)"$'\n'

        local highlights_count
        highlights_count=$(echo "$json" | jq ".components[$i].config_highlights | length")
        local h=0
        while [[ $h -lt $highlights_count ]]; do
            local highlight
            highlight=$(echo "$json" | jq -r ".components[$i].config_highlights[$h]")
            md+="- ${highlight}"$'\n'
            h=$((h + 1))
        done

        local issues_count
        issues_count=$(echo "$json" | jq ".components[$i].issues | length")
        local j=0
        while [[ $j -lt $issues_count ]]; do
            local sev text icon
            sev=$(echo "$json" | jq -r ".components[$i].issues[$j].severity")
            text=$(echo "$json" | jq -r ".components[$i].issues[$j].text")
            case "$sev" in
                critical|warning) icon="[!]" ;;
                ok)               icon="[+]" ;;
                *)                icon="[i]" ;;
            esac
            md+="- **${icon}** ${text}"$'\n'
            j=$((j + 1))
        done
        md+=$'\n'

        i=$((i + 1))
    done

    printf '%s' "$md"
}

# Generate markdown for a free-text Q&A.
# Usage: __sysinfo_md_freetext <question> <json>
# Returns: markdown string (no ANSI)
__sysinfo_md_freetext() {
    local question="$1" json="$2"
    local md=""

    local summary
    summary=$(echo "$json" | jq -r '.summary // ""')

    md+=$'\n'"---"$'\n\n'
    md+="## Question: ${question}"$'\n'
    [[ -n "$summary" ]] && md+="*${summary}*"$'\n'
    md+=$'\n'

    local comp_count
    comp_count=$(echo "$json" | jq '.components | length')

    local i=0
    while [[ $i -lt $comp_count ]]; do
        local name status
        name=$(echo "$json" | jq -r ".components[$i].name")
        status=$(echo "$json" | jq -r ".components[$i].status")

        md+="### ${name} (\`${status}\`)"$'\n'

        local highlights_count
        highlights_count=$(echo "$json" | jq ".components[$i].config_highlights | length")
        local h=0
        while [[ $h -lt $highlights_count ]]; do
            local highlight
            highlight=$(echo "$json" | jq -r ".components[$i].config_highlights[$h]")
            md+="- ${highlight}"$'\n'
            h=$((h + 1))
        done

        local issues_count
        issues_count=$(echo "$json" | jq ".components[$i].issues | length")
        local j=0
        while [[ $j -lt $issues_count ]]; do
            local sev text icon
            sev=$(echo "$json" | jq -r ".components[$i].issues[$j].severity")
            text=$(echo "$json" | jq -r ".components[$i].issues[$j].text")
            case "$sev" in
                critical|warning) icon="[!]" ;;
                ok)               icon="[+]" ;;
                *)                icon="[i]" ;;
            esac
            md+="- **${icon}** ${text}"$'\n'
            j=$((j + 1))
        done
        md+=$'\n'

        i=$((i + 1))
    done

    printf '%s' "$md"
}

# =============================================================================
# PUBLIC FUNCTIONS
# =============================================================================

# Interactive AI chat
# Usage: ai_ask [provider]
# Example: ai_ask          # uses first available provider
# Example: ai_ask claude   # uses Claude
# Example: ai_ask gemini   # uses Gemini
ai_ask() {
    needs_help "ai_ask" "ai_ask [provider]" \
        "Interactive AI chat. Provider: claude or gemini (default: first available)" \
        "${1-}" && return

    __gash_no_history __ai_ask_impl "$@"
}

__ai_ask_impl() {
    local provider="${1-}"

    # Check dependencies
    __ai_require_curl || return 1
    __ai_require_jq || return 1

    # Determine provider
    if [[ -z "$provider" ]]; then
        provider=$(__gash_get_first_ai_provider) || {
            __gash_error "No AI provider configured in ~/.gash_env"
            __gash_info "Add: AI:claude=YOUR_API_KEY or AI:gemini=YOUR_API_KEY"
            return 1
        }
    fi

    # Validate provider
    if [[ ! "$provider" =~ ^(claude|gemini)$ ]]; then
        __gash_error "Unknown provider '$provider'. Use 'claude' or 'gemini'"
        return 1
    fi

    # Get token
    local token
    token=$(__gash_get_ai_token "$provider") || {
        __gash_error "No token configured for '$provider' in ~/.gash_env"
        __gash_info "Add: AI:$provider=YOUR_API_KEY"
        return 1
    }

    # Show prompt and read query
    local query
    printf "%s: " "$provider"
    read -r query

    if [[ -z "$query" ]]; then
        __gash_warning "Empty query, aborting"
        return 1
    fi

    # Show spinner with provider name
    local provider_display
    provider_display="$(echo "$provider" | sed 's/.*/\u&/')"  # Capitalize first letter
    __gash_spinner_start "${provider_display} is thinking..."

    # Call API
    local response
    case "$provider" in
        claude)
            response=$(__ai_call_claude "$token" "$query")
            ;;
        gemini)
            response=$(__ai_call_gemini "$token" "$query")
            ;;
    esac

    local rc=$?

    # Stop spinner (no status message)
    if [[ -n "${__GASH_SPINNER_PID-}" ]]; then
        kill "$__GASH_SPINNER_PID" 2>/dev/null || true
        wait "$__GASH_SPINNER_PID" 2>/dev/null || true
        __GASH_SPINNER_PID=""
        printf "\r\033[K"
        trap - EXIT
    fi

    if [[ $rc -ne 0 ]]; then
        return 1
    fi

    # Display formatted response
    __ai_format_response "$response"
}

# Non-interactive AI query
# Usage: ai_query [provider] "query"
# Example: ai_query "how to list files by size?"
# Example: ai_query claude "what is 2+2?"
ai_query() {
    needs_help "ai_query" "ai_query [provider] \"query\"" \
        "Non-interactive AI query. Provider is optional (uses first available)" \
        "${1-}" && return

    __gash_no_history __ai_query_impl "$@"
}

__ai_query_impl() {
    local provider=""
    local query=""
    local piped_content=""

    # FIRST: Check for pipe input (stdin not a terminal)
    if [[ ! -t 0 ]]; then
        # stdin is not a terminal = pipe input
        # Truncate to max 4KB (~1000 tokens) to avoid token waste
        piped_content=$(head -c 4096)
        local piped_size=${#piped_content}
        if [[ $piped_size -ge 4096 ]]; then
            piped_content="${piped_content}
[... truncated to 4KB]"
        fi
    fi

    # Parse arguments: ai_query [provider] "query"
    if [[ $# -eq 1 ]]; then
        # Only query provided
        query="$1"
    elif [[ $# -ge 2 ]]; then
        # Check if first arg is a provider
        if [[ "$1" =~ ^(claude|gemini)$ ]]; then
            provider="$1"
            shift
            query="$*"
        else
            # First arg is part of query
            query="$*"
        fi
    fi

    if [[ -z "$query" ]]; then
        __gash_error "No query provided"
        return 1
    fi

    # If pipe content exists, prepend it to query for troubleshoot mode
    if [[ -n "$piped_content" ]]; then
        query="[PIPE INPUT - TROUBLESHOOT MODE]
---
${piped_content}
---
User question: ${query}"
    fi

    # Check dependencies
    __ai_require_curl || return 1
    __ai_require_jq || return 1

    # Determine provider
    if [[ -z "$provider" ]]; then
        provider=$(__gash_get_first_ai_provider) || {
            __gash_error "No AI provider configured in ~/.gash_env"
            __gash_info "Add: AI:claude=YOUR_API_KEY or AI:gemini=YOUR_API_KEY"
            return 1
        }
    fi

    # Get token
    local token
    token=$(__gash_get_ai_token "$provider") || {
        __gash_error "No token configured for '$provider' in ~/.gash_env"
        return 1
    }

    # Show spinner with provider name
    local provider_display
    provider_display="$(echo "$provider" | sed 's/.*/\u&/')"  # Capitalize first letter
    __gash_spinner_start "${provider_display} is thinking..."

    # Call API
    local response
    case "$provider" in
        claude)
            response=$(__ai_call_claude "$token" "$query")
            ;;
        gemini)
            response=$(__ai_call_gemini "$token" "$query")
            ;;
    esac

    local rc=$?

    # Stop spinner (no status message)
    if [[ -n "${__GASH_SPINNER_PID-}" ]]; then
        kill "$__GASH_SPINNER_PID" 2>/dev/null || true
        wait "$__GASH_SPINNER_PID" 2>/dev/null || true
        __GASH_SPINNER_PID=""
        printf "\r\033[K"
        trap - EXIT
    fi

    if [[ $rc -ne 0 ]]; then
        return 1
    fi

    # Display formatted response
    __ai_format_response "$response"
}

# AI-powered system analysis.
# Usage: ai_sysinfo [provider] [--raw]
# Alias: sysinfo_ai
ai_sysinfo() {
    needs_help "ai_sysinfo" "ai_sysinfo [provider] [--raw]" \
        "AI-powered system analysis. Provider: claude or gemini. --raw dumps collected data without API call. Alias: sysinfo_ai" \
        "${1-}" && return

    __gash_no_history __ai_sysinfo_impl "$@"
}

__ai_sysinfo_impl() {
    # Ensure sudo cleanup on any exit path
    trap '__sysinfo_release_sudo' RETURN

    local provider=""
    local raw_mode=0

    # Parse args
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --raw) raw_mode=1; shift ;;
            claude|gemini) provider="$1"; shift ;;
            *)
                __gash_error "Unknown argument: $1"
                __gash_info "Usage: ai_sysinfo [claude|gemini] [--raw]"
                return 1
                ;;
        esac
    done

    # Check dependencies
    __ai_require_curl || return 1
    __ai_require_jq || return 1

    # Verify sysinfo module is loaded
    if ! declare -f __sysinfo_collect_identity >/dev/null 2>&1; then
        __gash_error "sysinfo module not loaded. Run: gash_doctor"
        return 1
    fi

    # Acquire sudo (single password prompt)
    __sysinfo_ensure_sudo

    # Collect system data in LLM mode
    __gash_spinner_start "Collecting system data..."

    __SYSINFO_MODE="llm"
    local sysinfo_data
    sysinfo_data=$(
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
    )
    __SYSINFO_MODE="verbose"

    # Stop spinner (no status message)
    if [[ -n "${__GASH_SPINNER_PID-}" ]]; then
        kill "$__GASH_SPINNER_PID" 2>/dev/null || true
        wait "$__GASH_SPINNER_PID" 2>/dev/null || true
        __GASH_SPINNER_PID=""
        printf "\r\033[K"
        trap - EXIT
    fi

    # Raw mode: dump data without API call
    if [[ "$raw_mode" -eq 1 ]]; then
        echo "$sysinfo_data"
        return 0
    fi

    # Determine provider
    if [[ -z "$provider" ]]; then
        provider=$(__gash_get_first_ai_provider) || {
            __gash_error "No AI provider configured in ~/.gash_env"
            __gash_info "Add: AI:claude=YOUR_API_KEY or AI:gemini=YOUR_API_KEY"
            return 1
        }
    fi

    # Validate provider
    if [[ ! "$provider" =~ ^(claude|gemini)$ ]]; then
        __gash_error "Unknown provider '$provider'. Use 'claude' or 'gemini'"
        return 1
    fi

    # Get token
    local token
    token=$(__gash_get_ai_token "$provider") || {
        __gash_error "No token configured for '$provider' in ~/.gash_env"
        return 1
    }

    # Show spinner for analysis
    local provider_display
    provider_display="$(echo "$provider" | sed 's/.*/\u&/')"
    __gash_spinner_start "${provider_display} is analyzing system..."

    # Call API
    local response
    case "$provider" in
        claude) response=$(__sysinfo_call_claude "$token" "$sysinfo_data") ;;
        gemini) response=$(__sysinfo_call_gemini "$token" "$sysinfo_data") ;;
    esac

    local rc=$?

    # Stop spinner
    if [[ -n "${__GASH_SPINNER_PID-}" ]]; then
        kill "$__GASH_SPINNER_PID" 2>/dev/null || true
        wait "$__GASH_SPINNER_PID" 2>/dev/null || true
        __GASH_SPINNER_PID=""
        printf "\r\033[K"
        trap - EXIT
    fi

    if [[ $rc -ne 0 ]]; then
        return 1
    fi

    # Display formatted response
    __sysinfo_format_ai_response "$response"

    # Interactive drill-down loop (only if stdin is a terminal)
    [[ ! -t 0 ]] && return 0

    local initial_json="$response"

    # Markdown accumulator for save-to-file
    local __sysinfo_md_buffer=""
    __sysinfo_md_buffer=$(__sysinfo_md_initial "$initial_json" "$provider")

    while true; do
        echo
        echo -e "${__GASH_BOLD_WHITE}Drill down for details:${__GASH_COLOR_OFF}"
        echo "  1) Services    2) Security    3) Network"
        echo "  4) Storage     5) Performance 6) Maintenance"
        echo "  ?) Ask a question              s) Save report  q) Quit"
        echo
        local choice
        read -rp "> " choice || break   # Ctrl+D exits

        local section_title="" deep_fn=""
        case "$choice" in
            q|Q|quit|exit) break ;;
            1) section_title="Services Detected";  deep_fn="__sysinfo_deep_services" ;;
            2) section_title="Security Posture";   deep_fn="__sysinfo_deep_security" ;;
            3) section_title="Network";            deep_fn="__sysinfo_deep_network" ;;
            4) section_title="Storage";            deep_fn="__sysinfo_deep_storage" ;;
            5) section_title="Performance Tuning"; deep_fn="__sysinfo_deep_performance" ;;
            6) section_title="Maintenance";        deep_fn="__sysinfo_deep_maintenance" ;;
            "?"|ask)
                echo
                local user_question
                read -rp "Question: " user_question || continue
                [[ -z "$user_question" ]] && { __gash_warning "Empty question."; continue; }

                # Phase 1: collect targeted context based on entities in the question
                __gash_spinner_start "Collecting context..."
                local question_context
                question_context=$(__sysinfo_collect_question_context "$user_question")

                if [[ -n "${__GASH_SPINNER_PID-}" ]]; then
                    kill "$__GASH_SPINNER_PID" 2>/dev/null || true
                    wait "$__GASH_SPINNER_PID" 2>/dev/null || true
                    __GASH_SPINNER_PID=""
                    printf "\r\033[K"
                    trap - EXIT
                fi

                # Phase 2: call AI with initial analysis + collected context
                __gash_spinner_start "${provider_display} is thinking..."

                local freetext_response
                case "$provider" in
                    claude) freetext_response=$(__sysinfo_call_freetext_claude "$token" "$user_question" "$initial_json" "$question_context") ;;
                    gemini) freetext_response=$(__sysinfo_call_freetext_gemini "$token" "$user_question" "$initial_json" "$question_context") ;;
                esac
                local ft_rc=$?

                if [[ -n "${__GASH_SPINNER_PID-}" ]]; then
                    kill "$__GASH_SPINNER_PID" 2>/dev/null || true
                    wait "$__GASH_SPINNER_PID" 2>/dev/null || true
                    __GASH_SPINNER_PID=""
                    printf "\r\033[K"
                    trap - EXIT
                fi

                if [[ $ft_rc -ne 0 ]]; then continue; fi

                __sysinfo_format_drilldown_response "$freetext_response"
                __sysinfo_md_buffer+=$(__sysinfo_md_freetext "$user_question" "$freetext_response")
                continue
                ;;
            s|S|save)
                local filename="gash-ai-sysinfo-$(date +%Y%m%d-%H%M%S).md"
                printf '%s' "$__sysinfo_md_buffer" > "$filename"
                __gash_success "Report saved to ${filename}"
                continue
                ;;
            *) __gash_warning "Invalid choice. Enter 1-6, ?, s, or q."; continue ;;
        esac

        # Extract initial findings for context
        local initial_findings
        initial_findings=$(echo "$initial_json" | jq -r \
            ".sections[] | select(.title==\"$section_title\") | .findings[] | \"[\(.severity)] \(.text)\"" 2>/dev/null)

        # Collect deep data
        __gash_spinner_start "Collecting detailed data..."
        local deep_data
        deep_data=$("$deep_fn" 2>/dev/null)

        # Stop spinner
        if [[ -n "${__GASH_SPINNER_PID-}" ]]; then
            kill "$__GASH_SPINNER_PID" 2>/dev/null || true
            wait "$__GASH_SPINNER_PID" 2>/dev/null || true
            __GASH_SPINNER_PID=""
            printf "\r\033[K"
            trap - EXIT
        fi

        # API call
        __gash_spinner_start "${provider_display} is analyzing ${section_title}..."

        local drill_response
        case "$provider" in
            claude) drill_response=$(__sysinfo_call_drilldown_claude "$token" "$section_title" "$initial_findings" "$deep_data") ;;
            gemini) drill_response=$(__sysinfo_call_drilldown_gemini "$token" "$section_title" "$initial_findings" "$deep_data") ;;
        esac
        local drill_rc=$?

        # Stop spinner
        if [[ -n "${__GASH_SPINNER_PID-}" ]]; then
            kill "$__GASH_SPINNER_PID" 2>/dev/null || true
            wait "$__GASH_SPINNER_PID" 2>/dev/null || true
            __GASH_SPINNER_PID=""
            printf "\r\033[K"
            trap - EXIT
        fi

        if [[ $drill_rc -ne 0 ]]; then
            continue
        fi

        __sysinfo_format_drilldown_response "$drill_response"
        __sysinfo_md_buffer+=$(__sysinfo_md_drilldown "$drill_response")
    done
}

# =============================================================================
# ALIAS
# =============================================================================

alias ask='ai_ask'
alias sysinfo_ai='ai_sysinfo'

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

__gash_register_help "ai_ask" \
    --aliases "ask" \
    --module "ai" \
    --short "Interactive AI chat from the terminal" \
    --see-also "ai_query ai_sysinfo gash_ai_list" \
    <<'HELP'
USAGE
  ai_ask [provider]

EXAMPLES
  # Start an interactive chat (uses first configured provider)
  ask

  # Use a specific provider
  ask claude
  ask gemini

  # Type your questions interactively:
  #   claude: how to find files larger than 100MB?
  #   Command: find . -type f -size +100M
  #   Explanation: Finds all files larger than 100MB.

  # Pipe a config file and then chat about it
  cat /etc/nginx/nginx.conf | ask
  #   claude: is this config secure?
  #   Issue: ...
  #   Suggestion: ...

RESPONSE TYPES
  The AI automatically detects your intent:
    "how to..."      -> Command + Explanation
    "what is..."     -> Explanation
    "write a..."     -> Code block
    (with piped data) -> Troubleshoot (Issue + Suggestion)

NOTES
  Type 'exit' or 'quit' or press Ctrl+C to end the session.
  Piped content is auto-truncated to 4KB to save tokens.
HELP

__gash_register_help "ai_query" \
    --module "ai" \
    --short "Non-interactive AI query with optional piped input" \
    --see-also "ai_ask ai_sysinfo gash_ai_list" \
    <<'HELP'
USAGE
  ai_query [provider] "query"
  command | ai_query "question about this output"

EXAMPLES
  # Ask a question (uses first configured provider)
  ai_query "how do I find files larger than 100MB?"

  # Use a specific provider
  ai_query claude "explain the difference between grep and rg"

  # Pipe a config file for analysis
  cat /etc/nginx/nginx.conf | ai_query "any security issues?"

  # Pipe error logs for troubleshooting
  tail -50 /var/log/apache2/error.log | ai_query "what is wrong?"

  # Pipe command output
  docker logs myapp 2>&1 | ai_query "why is this crashing?"

  # Review a script
  cat deploy.sh | ai_query "review this for bugs"

  # Pipe a diff for summary
  git diff | ai_query "summarize these changes"

MULTI-SOURCE PIPING
  # Combine multiple files for comparative analysis
  {
    echo "=== Apache Config ==="
    cat /etc/apache2/apache2.conf
    echo "=== Ports ==="
    cat /etc/apache2/ports.conf
  } | ai_query "is this Apache setup correct?"

  # Combine config and its test output
  {
    cat /etc/nginx/nginx.conf
    echo "---"
    nginx -t 2>&1
  } | ai_query "fix the nginx configuration errors"

  # Compare PHP-FPM pools
  diff /etc/php/8.1/fpm/pool.d/www.conf /etc/php/8.2/fpm/pool.d/www.conf \
    | ai_query "what changed between PHP 8.1 and 8.2 pool configs?"

  # Analyze multiple log sources
  {
    echo "=== System Log ==="
    tail -20 /var/log/syslog
    echo "=== App Log ==="
    tail -20 /var/log/myapp/error.log
  } | ai_query "correlate these errors"

NOTES
  Piped content is auto-truncated to 4KB to save tokens.
  When input is piped, the AI enters troubleshoot mode automatically.
HELP

__gash_register_help "ai_sysinfo" \
    --aliases "sysinfo_ai" \
    --module "ai" \
    --short "AI-powered system security and configuration analysis" \
    --see-also "sysinfo ai_query" \
    <<'HELP'
USAGE
  ai_sysinfo [provider] [--raw]

EXAMPLES
  # Run AI-powered system analysis
  ai_sysinfo

  # Use a specific provider
  ai_sysinfo claude
  ai_sysinfo gemini

  # Dump raw collected data (no API call, useful for debugging)
  ai_sysinfo --raw

INTERACTIVE DRILL-DOWN
  After the initial analysis, an interactive menu appears:
    1) Services    2) Security    3) Network
    4) Storage     5) Performance 6) Maintenance
    ?) Ask a question              s) Save report  q) Quit

  Options 1-6 run deep collectors that read full config files.
  Option ? lets you ask about specific services, files, or ports.
  Option s exports the full session as a Markdown report.

SMART QUESTIONS
  The ? option detects entities in your question:
    "analyze viper-backup.service" -> reads unit file + journal
    "what's in /etc/postfix/main.cf?" -> reads the file
    "what's on port 3306?" -> reads socket listeners
    "tell me about apache" -> reads service configs
HELP

fi  # end help registration guard
