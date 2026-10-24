# ai

Ask Claude or Gemini from the terminal.

_Generated from the help registry by `tools/gen-docs.sh`; do not edit by hand._

| Function | Alias | Description |
|---|---|---|
| [`ai_ask`](#ai-ask) | `ask` | Interactive AI chat from the terminal |
| [`ai_query`](#ai-query) |  | Non-interactive AI query with optional piped input |
| [`ai_sysinfo`](#ai-sysinfo) | `sysinfo_ai` | AI-powered system security and configuration analysis |

## ai_ask

Interactive AI chat from the terminal

Alias: `ask`

#### Usage

```text
ai_ask [provider]
```

#### Examples

```bash
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
```

#### Pipe support

```text
When stdin is piped, ai_ask reads the piped content first and
then prompts for a question via the terminal. Press Enter
without typing to auto-analyze the content (explains what it
does, identifies issues, suggests improvements).
```

#### Response types

```text
The AI automatically detects your intent:
  "how to..."      -> Command + Explanation
  "what is..."     -> Explanation
  "write a..."     -> Code block
  (with piped data) -> Troubleshoot (Issue + Suggestion)
```

#### Notes

```text
One question per call; Enter on an empty line aborts, exit/quit cancel.
ai_ask needs a terminal (not headless): from a script or an agent use
ai_query "question" instead.
Piped content is auto-truncated to 4KB to save tokens; piped input must
start within 2 seconds, or stdin is ignored with a warning.
-h/--help is help as the first word or right after the provider.
```

#### Privacy

```text
Secrets are redacted before sending: private keys, passwords in
URLs, KEY=value secrets, Authorization headers, SSH: passphrase
lines, password hashes ($6$...) and well-known token formats (sk-,
ghp_, AKIA, xox, AIza). Free text ("the password is ...") is not
detected. The API key stays inside the gash process, never on a
command line.
GASH_AI_CONFIRM=1 asks before every request (default: auto).
Models: GASH_AI_MODEL_CLAUDE, GASH_AI_MODEL_GEMINI in ~/.gash_env.
```

See also: `ai_query`, `ai_sysinfo`, `gash_ai_list`

## ai_query

Non-interactive AI query with optional piped input

#### Usage

```text
ai_query [provider] "query"
command | ai_query "question about this output"
```

#### Examples

```bash
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
```

#### Multi-source piping

```text
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
```

#### Notes

```text
Piped content is auto-truncated to 4KB to save tokens. Piped input must
start within 2 seconds: an open stdin that stays silent is ignored with
a warning (redirect </dev/null to skip the wait).
When input is piped, the AI enters troubleshoot mode automatically.
A provider alone is not a question: "ai_query claude" fails with
"No query provided". -h/--help is help as the first word or as the
only word after the provider; inside a longer question it is text.
```

#### Privacy

```text
Secrets are redacted before sending: private keys, passwords in
URLs, KEY=value secrets, Authorization headers, SSH: passphrase
lines, password hashes ($6$...) and well-known token formats (sk-,
ghp_, AKIA, xox, AIza). Free text ("the password is ...") is not
detected. The API key stays inside the gash process, never on a
command line.
GASH_AI_CONFIRM=1 asks before every request (default: auto).
Models: GASH_AI_MODEL_CLAUDE, GASH_AI_MODEL_GEMINI in ~/.gash_env.
```

See also: `ai_ask`, `ai_sysinfo`, `gash_ai_list`

## ai_sysinfo

AI-powered system security and configuration analysis

Alias: `sysinfo_ai`

#### Usage

```text
ai_sysinfo [provider] [--raw]
```

#### Examples

```bash
# Run AI-powered system analysis
ai_sysinfo

# Use a specific provider
ai_sysinfo claude
ai_sysinfo gemini

# Dump raw collected data (no API call, useful for debugging)
ai_sysinfo --raw
```

#### Interactive drill-down

```text
After the initial analysis, an interactive menu appears:
  1) Services    2) Security    3) Network
  4) Storage     5) Performance 6) Maintenance
  ?) Ask a question              s) Save report  q) Quit

Options 1-6 run deep collectors that read full config files.
Option ? lets you ask about specific services, files, or ports.
Option s exports the full session as a Markdown report.
```

#### Smart questions

```text
The ? option detects entities in your question:
  "analyze viper-backup.service" -> reads unit file + journal
  "what's in /etc/postfix/main.cf?" -> reads the file
  "what's on port 3306?" -> reads socket listeners
  "tell me about apache" -> reads service configs
Paths follow the llm_* reader rules: no .. components, symlinks
resolved, and protected paths (/proc, /sys, /dev, /boot, /root,
/etc/shadow, /etc/passwd, /etc/sudoers...) and secret files
(.env, *.key, id_rsa*, .ssh/...) are skipped, never read; directory
listings leave secret files out.
```

#### Privacy

```text
Before the first request, ai_sysinfo shows how much system data it
will send and asks for confirmation (GASH_AI_CONFIRM=auto). Each ?
question that collects data asks again before sending it. Without
a terminal it refuses; set GASH_AI_CONFIRM=0 in ~/.gash_env to allow
unattended runs. Secrets are redacted before sending either way.
Use --raw to see exactly what is collected.
```

See also: `sysinfo`, `ai_query`

