# AI assistants

Ask Claude or Gemini from the terminal, with the context of this system.

You put an API key for Claude (Anthropic) or Gemini (Google) in
`~/.gash_env`, then ask a question at the prompt, pipe a log or a config file
into a question, or have the model review the configuration of this machine.
The answer comes back formatted for the terminal: a command and its
explanation, an explanation, a piece of code, or an issue and a suggestion.
Gash removes the secrets it recognizes before anything leaves the machine,
and asks before it sends data it collected itself.

Platforms: Linux, macOS and WSL. The requests go to the providers' HTTPS
APIs directly, through the proxy that `HTTPS_PROXY` names when it is set; no
other program is needed. `ai_sysinfo` reads what
[`sysinfo`](../system/reference.md#sysinfo) reads.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [sudo](../requirements.md#tool-sudo) (feature), for `ai_sysinfo` (the privileged sections)
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).

## Add a provider

Add a line per provider to `~/.gash_env` (`gash_env_init` creates the file
from a template), with the API key from the provider's console:

```text
AI:claude=sk-ant-api03-...
AI:gemini=AIza...
```

The provider is `claude` or `gemini`. The first line is the default
provider; name the other one in a call to use it. Keep the file private
(`chmod 600 ~/.gash_env`). [`gash_ai_list`](reference.md#gash_ai_list) shows
the providers in effect, with the first 8 characters of each key:

<!-- example
printf 'AI:claude=sk-ant-api03-EXAMPLEKEY\nAI:gemini=AIzaSyEXAMPLEKEY\n' >.gash_env
chmod 600 .gash_env
-->
```console
$ gash_ai_list
Info: Available AI providers:

  claude     sk-ant-a...
  gemini     AIzaSyEX...
```

## Ask a question

[`ai_query`](reference.md#ai_query) sends one question and prints the
answer. It reads no keyboard input, so it also works in scripts:

```bash
ai_query "how do I find files larger than 100MB?"
ai_query gemini "explain the difference between grep -E and grep -P"
```

A first word `claude` or `gemini` picks the provider; every other word is
part of the question. The model answers in one of four shapes, and Gash
prints it that way: `Command:` and `Explanation:` for a "how do I" question,
`Explanation:` alone, a code block, or `Issue:` and `Suggestion:` for a
problem.

Pipe text into it to ask about that text; the model then looks for problems
in it:

```bash
tail -n 50 /var/log/nginx/error.log | ai_query "what is wrong?"
git diff | ai_query "summarize these changes"
{ cat /etc/nginx/nginx.conf; nginx -t 2>&1; } | ai_query "fix the configuration errors"
```

Gash reads the first 4 KB of piped input and marks the cut. Input must start
arriving within 2 seconds: a standard input that stays open and silent is
ignored with a warning (`</dev/null` skips the wait).

## Ask from the prompt

[`ai_ask`](reference.md#ai_ask) (alias `ask`) prompts you for one question,
answers it and returns:

```bash
ask             # the default provider
ask gemini
cat deploy.sh | ask
```

With piped input it says how much it received, then asks for the question
on the terminal; press Enter without typing to have the content analyzed
(what it does, what is wrong, what to improve). An empty question, `exit`,
`quit` or `q` cancels. `ai_ask` needs a terminal; in a script, use
`ai_query`.

## Have the AI review this machine

[`ai_sysinfo`](reference.md#ai_sysinfo) (alias `sysinfo_ai`) collects a
report of this system, the sections of `sysinfo` (identity, storage,
services, network, security, the web stack and more), sends it, and prints
the model's analysis:

```bash
ai_sysinfo --raw    # print what would be collected; nothing is sent
ai_sysinfo          # collect, ask, send, analyze
ai_sysinfo gemini
```

Some sections read files only root can read: on a terminal, `ai_sysinfo`
asks for your `sudo` password once, and without it collects less. Before
sending, it shows how much data it will send and asks you to confirm (see
[Consent](#consent-gash_ai_confirm)).

After the analysis a menu opens:

| Key | Does |
|---|---|
| `1` to `6` | Services, Security, Network, Storage, Performance, Maintenance: reads more for that area and asks the model about it |
| `?` | A question of yours. A service name, a file path or a port in it makes Gash read that unit, file or socket list and send it along, after asking again |
| `s` | Saves the whole session as Markdown in `gash-ai-sysinfo-YYYYMMDD-HHMMSS.md` in the current directory, mode 0600 |
| `q` | Quits (Ctrl-D too) |

The files a `?` question reads follow the rules of the `llm_*` readers:
secret files (`.env`, keys, `.ssh/...`) and protected paths (`/etc/shadow`,
`/root`, `/proc`, ...) are skipped, never read.

## What is sent, and what is redacted

A request carries:

- your question, and the piped input (at most 4 KB) when there is one;
- a small context object: the current directory's path, the shell, the
  distribution and its package manager, the git branch, the names of the
  first 8 entries of the current directory (hidden ones left out) and the
  exit status of the last command;
- for `ai_sysinfo`, the collected report, and in the menu the data each
  choice collects.

Before sending, Gash replaces with `[REDACTED]` what it recognizes as a
secret: private key blocks, passwords in URLs (`scheme://user:pass@`),
`KEY=value` and `key: value` pairs whose name says password, secret, token,
API key, access key, private key or credentials, `Authorization` headers,
the `SSH:` passphrase lines of `~/.gash_env`, password hashes (`$6$...`),
and the token formats of the common providers (`sk-`, `ghp_`, `AKIA`,
`xox`, `AIza`). A secret written as free text ("the password is hunter2")
is not recognized, so read what you pipe. The API key itself is sent only in
the request header; it never appears on a command line.

To see exactly what goes out, `ai_sysinfo --raw` prints the report, and
`GASH_AI_DEBUG=1 ai_query "..."` prints each request body and the raw
response on stderr.

## Consent: GASH_AI_CONFIRM

`GASH_AI_CONFIRM` in `~/.gash_env` decides when Gash asks before sending:

| Value | Asks before |
|---|---|
| `auto` (default) | data Gash collected itself (`ai_sysinfo`, the menu); what you type or pipe is sent without a question |
| `1` | every request |
| `0` | nothing |

Any other value (the check is case-sensitive: `off`, `no`, `AUTO`) asks
before every request and warns once. The question reads
`Send system data from this host, and drill-down details on request (12 KB, secrets redacted) to claude (claude-haiku-5-5)? [y/N]`;
only `y` or `yes` sends. Without a terminal there is no one to ask, so a
request that needs consent is refused: set `GASH_AI_CONFIRM=0` to allow
unattended runs, knowing that nothing is then asked.

## Choose the model

| Setting | Default |
|---|---|
| `GASH_AI_MODEL_CLAUDE` | `claude-haiku-5-5` |
| `GASH_AI_MODEL_GEMINI` | `gemini-2.5-flash` |

Set them in `~/.gash_env` (`GASH_AI_MODEL_GEMINI=gemini-2.5-pro`). A
model name is letters, digits, `.`, `_` and `-`; any other value is an error
and nothing is sent. Gash waits 5 seconds for a connection and 60 seconds
for an answer (120 for `ai_sysinfo`). A busy API (HTTP 429 or 5xx) is
retried up to three times, after the delay the API asks for.

## When something goes wrong

| Message | What it means and what to do |
|---|---|
| `Error: No AI provider configured in ~/.gash_env` | Add an `AI:claude=` or `AI:gemini=` line. |
| `Error: No token configured for 'gemini' in ~/.gash_env` | You named a provider that has no line. |
| `Warning: Line N of ~/.gash_env: unknown AI provider (supported: claude, gemini), skipping` | The name before `=` must be `claude` or `gemini`. |
| `Error: No query provided` | `ai_query claude` alone: the provider is not a question. |
| `Error: ai_ask needs a terminal: use ai_query "question" ...` | `ai_ask` ran from a script or in headless mode. |
| `Warning: stdin is open but sent nothing within 2s: ignored ...` | Nothing was piped although standard input was open; add `</dev/null` to skip the wait. |
| `Error: Not sending ... to claude: no terminal to confirm on. Set GASH_AI_CONFIRM=0 in ~/.gash_env to allow it` | The request needs consent and there is no terminal. |
| `Warning: GASH_AI_CONFIRM=off is not 0, 1 or auto: asking before every request` | Use one of the three values. |
| `Error: Invalid GASH_AI_MODEL_CLAUDE value (letters, digits, '.', '_' and '-' only)` | Fix the model name. |
| `Error: Claude API error (401): ...` | The API refused the request; the text after the colon is the provider's message (a wrong key, a model you have no access to, a billing problem). |
| `Warning: Claude API busy (HTTP 429), retrying in 5s (1/3)...` | The API is rate-limiting or overloaded; Gash retries, then gives up with `still busy`. |
| `Error: Claude API: Could not resolve host (check DNS/internet)`, `Failed to connect`, `Connection timed out` | No network path to the API; check the connection and `HTTPS_PROXY`. |
| `Error: Claude's answer was cut off (max_tokens ... reached)`, `Claude declined this request` | The model did not complete the answer; rephrase or narrow the question. |

## See also

- [reference.md](reference.md): every function of this area with all its options.
- [configuration](../configuration.md#ai-providers): the `AI:` lines, and [`GASH_AI_CONFIRM` and the model settings](../configuration.md#settings-reference).
- [security](../security.md): what the redaction and the consent cover, and what they do not.
- [System](../system/README.md): `sysinfo`, the report `ai_sysinfo` sends (`sysinfo --llm | ai_query "..."` works too).
