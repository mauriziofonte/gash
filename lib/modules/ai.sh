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

# Implementation lives in lib/modules/ai/, loaded in dependency order.
for __gash_ai_part in prompts client chat sysinfo; do
    # shellcheck source=/dev/null
    source "${BASH_SOURCE[0]%/*}/ai/${__gash_ai_part}.sh"
done
unset __gash_ai_part

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

  # Pipe a config file and ask about it
  cat /etc/nginx/nginx.conf | ask claude
  #   claude (or Enter for auto-analysis): is this config secure?
  #   Issue: ...
  #   Suggestion: ...

  # Pipe content and press Enter for auto-analysis
  cat my-script.sh | ask
  #   (auto-analyzes: explains, identifies issues, suggests improvements)

  # Pipe multiple files for combined analysis
  cat /usr/local/bin/backup.sh /etc/systemd/system/backup.service | ask claude
  #   claude (or Enter for auto-analysis): <your question>

PIPE SUPPORT
  When stdin is piped, ai_ask reads the piped content first and
  then prompts for a question via the terminal. Press Enter
  without typing to auto-analyze the content (explains what it
  does, identifies issues, suggests improvements).

RESPONSE TYPES
  The AI automatically detects your intent:
    "how to..."      -> Command + Explanation
    "what is..."     -> Explanation
    "write a..."     -> Code block
    (with piped data) -> Troubleshoot (Issue + Suggestion)

NOTES
  Type 'exit' or 'quit' or press Ctrl+C to end the session.
  Piped content is auto-truncated to 4KB to save tokens.

PRIVACY
  Secrets are redacted before sending: private keys, passwords in
  URLs, KEY=value secrets and well-known token formats (sk-, ghp_,
  AKIA, xox, AIza). The API key travels in a curl config read from
  a pipe, never on the command line.
  GASH_AI_CONFIRM=1 asks before every request (default: auto).
  Models: GASH_AI_MODEL_CLAUDE, GASH_AI_MODEL_GEMINI in ~/.gash_env.
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

PRIVACY
  Secrets are redacted before sending: private keys, passwords in
  URLs, KEY=value secrets and well-known token formats (sk-, ghp_,
  AKIA, xox, AIza). The API key travels in a curl config read from
  a pipe, never on the command line.
  GASH_AI_CONFIRM=1 asks before every request (default: auto).
  Models: GASH_AI_MODEL_CLAUDE, GASH_AI_MODEL_GEMINI in ~/.gash_env.
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

PRIVACY
  Before the first request, ai_sysinfo shows how much system data it
  will send and asks for confirmation (GASH_AI_CONFIRM=auto). Without
  a terminal it refuses; set GASH_AI_CONFIRM=0 in ~/.gash_env to allow
  unattended runs. Secrets are redacted before sending either way.
  Use --raw to see exactly what is collected.
HELP

fi # end help registration guard
