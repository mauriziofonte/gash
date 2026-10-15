#!/usr/bin/env bash

# Gash Module: AI: ai_sysinfo (system analysis, drilldown, free-text questions).
# Sourced by lib/modules/ai.sh.

# =============================================================================
# SYSINFO API WRAPPERS
# =============================================================================

# Ask Claude for the overall system analysis.
# Usage: __sysinfo_call_claude <token> <data>
__sysinfo_call_claude() {
    __ai_request claude "${1-}" "$__AI_SYSINFO_SYSTEM_PROMPT" "Analyze this system enumeration:"$'\n\n'"${2-}" "$__AI_SYSINFO_RESPONSE_SCHEMA" "$__AI_SYSINFO_MAX_TOKENS" "$__AI_SYSINFO_RESPONSE_TIMEOUT"
}

# Ask Gemini for the overall system analysis.
# Usage: __sysinfo_call_gemini <token> <data>
__sysinfo_call_gemini() {
    __ai_request gemini "${1-}" "$__AI_SYSINFO_SYSTEM_PROMPT" "Analyze this system enumeration:"$'\n\n'"${2-}" "$__AI_SYSINFO_RESPONSE_SCHEMA" "$__AI_SYSINFO_MAX_TOKENS" "$__AI_SYSINFO_RESPONSE_TIMEOUT"
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

    __ai_request claude "$token" "$__AI_SYSINFO_DRILLDOWN_PROMPT" "$user_msg" "$__AI_SYSINFO_DRILLDOWN_SCHEMA" "$__AI_SYSINFO_MAX_TOKENS" "$__AI_SYSINFO_RESPONSE_TIMEOUT"
}

# Call Gemini API for drilldown analysis.
# Usage: __sysinfo_call_drilldown_gemini <token> <section_title> <initial_findings> <deep_data>
__sysinfo_call_drilldown_gemini() {
    local token="${1-}" section_title="${2-}" initial_findings="${3-}" deep_data="${4-}"

    local user_msg
    user_msg=$(printf 'Section: %s\n\nInitial findings:\n%s\n\nDetailed data:\n%s' "$section_title" "$initial_findings" "$deep_data")

    __ai_request gemini "$token" "$__AI_SYSINFO_DRILLDOWN_PROMPT" "$user_msg" "$__AI_SYSINFO_DRILLDOWN_SCHEMA" "$__AI_SYSINFO_MAX_TOKENS" "$__AI_SYSINFO_RESPONSE_TIMEOUT"
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
        [[ -n "${seen["$unit"]-}" ]] && continue
        seen["$unit"]=1

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
        [[ -n "${seen["$fpath"]-}" ]] && continue
        seen["$fpath"]=1

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
    local port_regex='(port[: ]+|:)([0-9]{1,5})([^0-9]|$)'
    remaining="$lower_q"
    while [[ "$remaining" =~ ($port_regex) ]]; do
        local port="${BASH_REMATCH[3]}"
        remaining="${remaining#*"${BASH_REMATCH[1]}"}"
        [[ -z "$port" || -n "${seen["port:$port"]-}" ]] && continue
        seen["port:$port"]=1

        context+="=== PORT: ${port} ==="$'\n'
        context+="$(ss -tlnp 2>/dev/null | grep -F ":${port}" | head -10)"$'\n'
        context+=$'\n'
    done

    # Pattern 4: Known service keywords → targeted collection
    if [[ "$lower_q" =~ apache|httpd ]] && [[ -z "${seen["svc:apache"]-}" ]]; then
        seen["svc:apache"]=1
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
            context+="$(for __f in /etc/apache2/mods-enabled/*.load; do [[ -e "$__f" ]] && basename "$__f" .load; done)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ nginx ]] && [[ -z "${seen["svc:nginx"]-}" ]]; then
        seen["svc:nginx"]=1
        context+="=== SERVICE: Nginx ==="$'\n'
        context+="$(systemctl status nginx --no-pager 2>/dev/null | head -20)"$'\n'
        if [[ -f /etc/nginx/nginx.conf ]]; then
            context+="--- /etc/nginx/nginx.conf ---"$'\n'
            context+="$(grep -Ev '^(#|$)' /etc/nginx/nginx.conf 2>/dev/null | head -40)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ mysql|mariadb ]] && [[ -z "${seen["svc:mysql"]-}" ]]; then
        seen["svc:mysql"]=1
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

    if [[ "$lower_q" =~ redis ]] && [[ -z "${seen["svc:redis"]-}" ]]; then
        seen["svc:redis"]=1
        context+="=== SERVICE: Redis ==="$'\n'
        context+="$(systemctl status redis-server --no-pager 2>/dev/null | head -20)"$'\n'
        if [[ -f /etc/redis/redis.conf ]]; then
            context+="--- /etc/redis/redis.conf (key lines) ---"$'\n'
            context+="$(grep -Ev '^(#|$)' /etc/redis/redis.conf 2>/dev/null | head -30)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ php|fpm ]] && [[ -z "${seen["svc:php"]-}" ]]; then
        seen["svc:php"]=1
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

    if [[ "$lower_q" =~ postfix|mail ]] && [[ -z "${seen["svc:postfix"]-}" ]]; then
        seen["svc:postfix"]=1
        context+="=== SERVICE: Postfix ==="$'\n'
        context+="$(systemctl status postfix --no-pager 2>/dev/null | head -20)"$'\n'
        if [[ -f /etc/postfix/main.cf ]]; then
            context+="--- /etc/postfix/main.cf (key lines) ---"$'\n'
            context+="$(grep -Ev '^(#|$)' /etc/postfix/main.cf 2>/dev/null | head -40)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ ssh|sshd ]] && [[ -z "${seen["svc:ssh"]-}" ]]; then
        seen["svc:ssh"]=1
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

    if [[ "$lower_q" =~ fail2ban ]] && [[ -z "${seen["svc:fail2ban"]-}" ]]; then
        seen["svc:fail2ban"]=1
        context+="=== SERVICE: Fail2ban ==="$'\n'
        context+="$(systemctl status fail2ban --no-pager 2>/dev/null | head -20)"$'\n'
        if [[ -f /etc/fail2ban/jail.local ]]; then
            context+="--- /etc/fail2ban/jail.local ---"$'\n'
            context+="$(grep -Ev '^(#|;|$)' /etc/fail2ban/jail.local 2>/dev/null | head -40)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ docker ]] && [[ -z "${seen["svc:docker"]-}" ]]; then
        seen["svc:docker"]=1
        context+="=== SERVICE: Docker ==="$'\n'
        context+="$(systemctl status docker --no-pager 2>/dev/null | head -20)"$'\n'
        if [[ -f /etc/docker/daemon.json ]]; then
            context+="--- /etc/docker/daemon.json ---"$'\n'
            context+="$(cat /etc/docker/daemon.json 2>/dev/null)"$'\n'
        fi
        context+=$'\n'
    fi

    if [[ "$lower_q" =~ postgres ]] && [[ -z "${seen["svc:postgres"]-}" ]]; then
        seen["svc:postgres"]=1
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

    if [[ "$lower_q" =~ cron ]] && [[ -z "${seen["svc:cron"]-}" ]]; then
        seen["svc:cron"]=1
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
        [[ -n "${seen["${svc_name}.service"]-}" ]] && continue
        [[ -n "${seen["svc:${svc_name,,}"]-}" ]] && continue

        # Try as a systemd service — only collect if the unit actually exists
        local unit_output
        unit_output=$(systemctl cat "${svc_name}.service" 2>/dev/null | head -60) || true
        if [[ -n "$unit_output" ]]; then
            seen["${svc_name}.service"]=1
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

    __ai_request claude "$token" "$__AI_SYSINFO_DRILLDOWN_PROMPT" "$user_msg" "$__AI_SYSINFO_DRILLDOWN_SCHEMA" "$__AI_SYSINFO_MAX_TOKENS" "$__AI_SYSINFO_RESPONSE_TIMEOUT"
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

    __ai_request gemini "$token" "$__AI_SYSINFO_DRILLDOWN_PROMPT" "$user_msg" "$__AI_SYSINFO_DRILLDOWN_SCHEMA" "$__AI_SYSINFO_MAX_TOKENS" "$__AI_SYSINFO_RESPONSE_TIMEOUT"
}

# The __gash_mark status of a finding's severity: critical -> error,
# warning, ok, anything else -> info.
# Usage: __sysinfo_severity_status SEVERITY
__sysinfo_severity_status() {
    case "${1-}" in
        critical) printf 'error' ;;
        warning) printf 'warning' ;;
        ok) printf 'ok' ;;
        *) printf 'info' ;;
    esac
}

# Print the findings at PATH (a jq path to an array of {severity, text})
# of JSON, one __gash_mark line each, indented by 2. Model text is printed
# as given.
# Usage: __sysinfo_print_findings JSON PATH
__sysinfo_print_findings() {
    local json="$1" path="$2" n j sev text
    n=$(jq "($path // []) | length" <<<"$json")
    for ((j = 0; j < n; j++)); do
        sev=$(jq -r "${path}[$j].severity" <<<"$json")
        text=$(jq -r "${path}[$j].text" <<<"$json")
        __gash_mark --indent 2 "$(__sysinfo_severity_status "$sev")" "$text"
    done
}

# Format and display AI sysinfo analysis response.
# Usage: __sysinfo_format_ai_response <json>
__sysinfo_format_ai_response() {
    local json="$1"

    # Validate JSON
    if ! jq empty <<<"$json" 2>/dev/null; then
        __gash_error "Invalid JSON response from AI"
        __ai_debug "Invalid JSON" "$json"
        return 1
    fi

    local hostname platform
    hostname=$(jq -r '.hostname // "unknown"' <<<"$json")
    platform=$(jq -r '.platform // "unknown"' <<<"$json")

    __gash_section "Server: ${hostname}"
    __gash_print muted "$platform"

    # Count totals
    local critical warning info ok_count
    critical=$(jq '[.sections[].findings[] | select(.severity=="critical")] | length' <<<"$json")
    warning=$(jq '[.sections[].findings[] | select(.severity=="warning")] | length' <<<"$json")
    info=$(jq '[.sections[].findings[] | select(.severity=="info")] | length' <<<"$json")
    ok_count=$(jq '[.sections[].findings[] | select(.severity=="ok")] | length' <<<"$json")

    local E="" W="" I="" S="" R=""
    if __gash_use_color; then
        E="$__GASH_COLOR_ERROR" W="$__GASH_COLOR_WARNING" I="$__GASH_COLOR_INFO" S="$__GASH_COLOR_SUCCESS" R="$__GASH_COLOR_OFF"
    fi
    printf '%s%s critical%s  %s%s warnings%s  %s%s info%s  %s%s ok%s\n' \
        "$E" "$critical" "$R" "$W" "$warning" "$R" "$I" "$info" "$R" "$S" "$ok_count" "$R"
    echo

    # Iterate sections
    local section_count
    section_count=$(jq '.sections | length' <<<"$json")

    local i=0
    while [[ $i -lt $section_count ]]; do
        __gash_heading "$(jq -r ".sections[$i].title" <<<"$json")"
        __sysinfo_print_findings "$json" ".sections[$i].findings"
        echo
        i=$((i + 1))
    done
}

# Format and display drilldown analysis response (component-based).
# Usage: __sysinfo_format_drilldown_response <json>
__sysinfo_format_drilldown_response() {
    local json="$1"

    # Validate JSON
    if ! jq empty <<<"$json" 2>/dev/null; then
        __gash_error "Invalid JSON response from AI"
        __ai_debug "Invalid JSON" "$json"
        return 1
    fi

    local title summary
    title=$(jq -r '.title // "Analysis"' <<<"$json")
    summary=$(jq -r '.summary // ""' <<<"$json")

    __gash_section "$title"
    if [[ -n "$summary" ]]; then
        __gash_print muted "$summary"
        echo
    fi

    local B="" M="" R="" color=0
    if __gash_use_color; then
        B="$__GASH_BOLD" M="$__GASH_COLOR_MUTED" R="$__GASH_COLOR_OFF" color=1
    fi

    local comp_count
    comp_count=$(jq '.components | length' <<<"$json")

    local i=0 name status status_color highlight
    while [[ $i -lt $comp_count ]]; do
        name=$(jq -r ".components[$i].name" <<<"$json")
        status=$(jq -r ".components[$i].status" <<<"$json")

        # Status color
        status_color=""
        if ((color)); then
            case "$status" in
                running) status_color="$__GASH_COLOR_SUCCESS" ;;
                installed) status_color="$__GASH_COLOR_INFO" ;;
                stopped) status_color="$__GASH_COLOR_WARNING" ;;
                eol | error) status_color="$__GASH_COLOR_ERROR" ;;
                *) status_color="$M" ;;
            esac
        fi
        printf '%s%s%s (%s%s%s)\n' "$B" "$name" "$R" "$status_color" "$status" "$R"

        # Config highlights
        while IFS= read -r highlight; do
            printf '  %s%s%s\n' "$M" "$highlight" "$R"
        done < <(jq -r ".components[$i].config_highlights[]?" <<<"$json")

        # Issues
        __sysinfo_print_findings "$json" ".components[$i].issues"
        echo

        i=$((i + 1))
    done
}

# The Markdown label of a finding's severity in a saved report.
# Usage: __sysinfo_md_label SEVERITY
__sysinfo_md_label() {
    case "${1-}" in
        critical) printf 'Critical:' ;;
        warning) printf 'Warning:' ;;
        ok) printf 'OK:' ;;
        *) printf 'Info:' ;;
    esac
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
    provider_cap="${provider^}"

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
            icon="$(__sysinfo_md_label "$sev")"
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
            icon="$(__sysinfo_md_label "$sev")"
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
            icon="$(__sysinfo_md_label "$sev")"
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

# AI-powered system analysis.
# Usage: ai_sysinfo [provider] [--raw] [--no-color]
# Alias: sysinfo_ai
ai_sysinfo() {
    needs_help "ai_sysinfo" "ai_sysinfo [provider] [--raw] [--no-color]" \
        "AI-powered system analysis. Provider: claude or gemini. --raw dumps collected data without API call. --no-color disables ANSI. Alias: sysinfo_ai" \
        "${1-}" && return

    __gash_no_history __ai_sysinfo_impl "$@"
}

__ai_sysinfo_impl() {
    # Ensure sudo cleanup on any exit path
    trap '__sysinfo_release_sudo; trap - RETURN' RETURN

    local provider=""
    local raw_mode=0
    local no_color=0

    # Parse args
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --raw)
                raw_mode=1
                shift
                ;;
            --no-color)
                no_color=1
                shift
                ;;
            claude | gemini)
                provider="$1"
                shift
                ;;
            *)
                __gash_error "Unknown argument: $1"
                __gash_usage "ai_sysinfo [claude|gemini] [--raw] [--no-color]"
                return 1
                ;;
        esac
    done

    # --raw implies plain text (no ANSI in LLM-consumed data dump)
    [[ "$raw_mode" -eq 1 ]] && no_color=1

    # Local color scope (propagates to the interactive report formatter below)
    __gash_want_colors "$no_color" || eval "$__GASH_COLOR_OFF_LOCALS"

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
    __gash_spinner_stop none

    # Raw mode: dump data without API call
    if [[ "$raw_mode" -eq 1 ]]; then
        printf '%s\n' "$sysinfo_data"
        return 0
    fi

    # Determine provider
    if [[ -z "$provider" ]]; then
        __gash_load_env
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
    __gash_load_env
    token=$(__gash_get_ai_token "$provider") || {
        __gash_error "No token configured for '$provider' in ~/.gash_env"
        return 1
    }

    # One confirmation covers the report and its drill-downs.
    __ai_confirm_send "$provider" data "system data from this host, and drill-down details on request" \
        "${#sysinfo_data}" || return 1

    # Show spinner for analysis
    local provider_display
    provider_display="${provider^}"
    __gash_spinner_start "${provider_display} is analyzing system..."

    # Call API, outside a command substitution so that its diagnostics see
    # the terminal; replies come back in __AI_REPLY.
    local __AI_REPLY_TO_VAR=1 __AI_REPLY="" rc=0
    case "$provider" in
        claude) __sysinfo_call_claude "$token" "$sysinfo_data" || rc=$? ;;
        gemini) __sysinfo_call_gemini "$token" "$sysinfo_data" || rc=$? ;;
    esac

    # Stop spinner
    __gash_spinner_stop none

    if [[ $rc -ne 0 ]]; then
        return 1
    fi
    local response="$__AI_REPLY"

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
        __gash_heading "Drill down for details:"
        __gash_table --no-header --indent 2 <<'MENU'
1) Services	2) Security	3) Network
4) Storage	5) Performance	6) Maintenance
?) Ask a question	s) Save report	q) Quit
MENU
        echo
        local choice
        __gash_read_line choice "> " || break # Ctrl+D exits

        local section_title="" deep_fn=""
        case "$choice" in
            q | Q | quit | exit) break ;;
            1)
                section_title="Services Detected"
                deep_fn="__sysinfo_deep_services"
                ;;
            2)
                section_title="Security Posture"
                deep_fn="__sysinfo_deep_security"
                ;;
            3)
                section_title="Network"
                deep_fn="__sysinfo_deep_network"
                ;;
            4)
                section_title="Storage"
                deep_fn="__sysinfo_deep_storage"
                ;;
            5)
                section_title="Performance Tuning"
                deep_fn="__sysinfo_deep_performance"
                ;;
            6)
                section_title="Maintenance"
                deep_fn="__sysinfo_deep_maintenance"
                ;;
            "?" | ask)
                echo
                local user_question
                __gash_read_line user_question "Question: " || continue
                [[ -z "$user_question" ]] && {
                    __gash_warning "Empty question."
                    continue
                }

                # Phase 1: collect targeted context based on entities in the question
                __gash_spinner_start "Collecting context..."
                local question_context
                question_context=$(__sysinfo_collect_question_context "$user_question")

                __gash_spinner_stop none

                # Phase 2: call AI with initial analysis + collected context
                __gash_spinner_start "${provider_display} is thinking..."

                local freetext_response ft_rc=0
                __AI_REPLY=""
                case "$provider" in
                    claude) __sysinfo_call_freetext_claude "$token" "$user_question" "$initial_json" "$question_context" || ft_rc=$? ;;
                    gemini) __sysinfo_call_freetext_gemini "$token" "$user_question" "$initial_json" "$question_context" || ft_rc=$? ;;
                esac

                __gash_spinner_stop none

                if [[ $ft_rc -ne 0 ]]; then continue; fi
                freetext_response="$__AI_REPLY"

                __sysinfo_format_drilldown_response "$freetext_response"
                __sysinfo_md_buffer+=$(__sysinfo_md_freetext "$user_question" "$freetext_response")
                continue
                ;;
            s | S | save)
                local filename
                filename="gash-ai-sysinfo-$(date +%Y%m%d-%H%M%S).md"
                printf '%s' "$__sysinfo_md_buffer" >"$filename"
                __gash_success "Report saved to ${filename}"
                continue
                ;;
            *)
                __gash_warning "Invalid choice. Enter 1-6, ?, s, or q."
                continue
                ;;
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
        __gash_spinner_stop none

        # API call
        __gash_spinner_start "${provider_display} is analyzing ${section_title}..."

        local drill_response drill_rc=0
        __AI_REPLY=""
        case "$provider" in
            claude) __sysinfo_call_drilldown_claude "$token" "$section_title" "$initial_findings" "$deep_data" || drill_rc=$? ;;
            gemini) __sysinfo_call_drilldown_gemini "$token" "$section_title" "$initial_findings" "$deep_data" || drill_rc=$? ;;
        esac

        # Stop spinner
        __gash_spinner_stop none

        if [[ $drill_rc -ne 0 ]]; then
            continue
        fi
        drill_response="$__AI_REPLY"

        __sysinfo_format_drilldown_response "$drill_response"
        __sysinfo_md_buffer+=$(__sysinfo_md_drilldown "$drill_response")
    done
}
