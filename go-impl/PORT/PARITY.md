<!-- Gash 3.0 port: spec parity. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Spec parity (PRD 7)

Status of every 2.x spec test against the Bash reference and against the Go
port behind the generated shim. Updated by every Go-mode run
(`tests/run.sh --impl both`); at S1 no Go exists, so the Go column reads
`not run` and the plan column says how each test will be satisfied.

## Legend

| Column | Values |
|---|---|
| Class | `BLACKBOX` public surface only; `MOCK-DEPENDENT` public surface through a fake; `MIXED` public assertions plus an internal probe or a Bash seam; `INTERNAL` Bash internals or source text |
| Bash 2.1.0 | `pass`; `pass (needs X)` for the tests that skip without sqlite3 or gawk (CI installs both) |
| Go (rc.A) | `not run`, then `pass`, `fail`, `FD <LEDGER id>` (forced divergence), `n/a <Go test>` (Bash internals, replaced) |
| Plan | `run` unchanged against the shim; `run*` unchanged if Go executes the same CLI as 2.x (`testing.md` 2.4), else replaced; `split` public part runs, the seam or probe moves to a Go test; `shell` tests the Bash that stays Bash, valid in both modes; `static` source-level check, applied to the generated Bash; `unit` replaced by the named Go unit test |

## Totals at S1

| Class | Tests |
|---|---|
| BLACKBOX | 478 |
| MOCK-DEPENDENT | 151 |
| MIXED | 119 |
| INTERNAL | 347 |
| **all** | **1095** |

| Plan | Tests |
|---|---|
| `run` | 425 |
| `run*` | 140 |
| `split` | 119 |
| `shell` | 80 |
| `static` | 22 |
| `unit` | 309 |

Bash reference at S1: 807 passed with sqlite3 and gawk installed (800 passed,
7 skipped without them); the slow render spec 288/288. Go: not run.

Rewrites before Go mode (they change how a test reaches the code, never what
it expects): the `kill` fakes with a fixed PID (`functions_edge_spec.sh:87`,
`functions_more_spec.sh:226` and three render fixtures) become real victim
processes; the 52 render fixtures that fake external commands as Bash
functions use PATH stubs; `llm_spec.sh` stops using the repository's `lib/` as
a fixture tree (LEDGER `TEST-*` items).

## Tests

### ai_client_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 8 | secrets are redacted before sending | INTERNAL | pass | not run | `unit` | ai.Redact |
| 34 | redaction keeps ordinary words after password/token/secret | INTERNAL | pass | not run | `unit` | ai.Redact negative vectors (password/token/secret followed by prose) |
| 47 | the request body and API key never appear in curl argv | INTERNAL | pass | not run | `unit` | request built off-argv |
| 65 | consent: auto asks only for collected data, refuses without a terminal | INTERNAL | pass | not run | `unit` | ai.Confirm (GASH_AI_CONFIRM=0, 4 KB threshold, refuses without TTY) |
| 84 | ai_query sends nothing when consent is refused | MIXED | pass | not run | `split` |  |
| 102 | terminal escapes in a reply are stripped | INTERNAL | pass | not run | `unit` | ai.Sanitize |
| 119 | a 529 overload is retried after Retry-After | INTERNAL | pass | not run | `unit` | retry with Retry-After |
| 136 | a refusal stop_reason is reported, not parsed | INTERNAL | pass | not run | `unit` | refusal -> 'declined' message, not parsed |
| 149 | GASH_AI_MODEL_CLAUDE overrides the model sent | INTERNAL | pass | not run | `unit` | model override reaches request body |
| 165 | Claude 5 models get effort low, older ones no effort field | INTERNAL | pass | not run | `unit` | claude-haiku-5-5 gets output_config.effort=low + json_schema |

### ai_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | parses AI:claude=TOKEN from config | INTERNAL | pass | not run | `unit` | internal/ai, internal/config |
| 30 | parses AI:gemini=TOKEN from config | INTERNAL | pass | not run | `unit` | internal/ai, internal/config |
| 53 | rejects unknown AI provider | INTERNAL | pass | not run | `unit` | internal/ai, internal/config |
| 82 | parses multiple AI providers | INTERNAL | pass | not run | `unit` | internal/ai, internal/config |
| 111 | __gash_get_ai_token returns token for valid provider | INTERNAL | pass | not run | `unit` | config.Load AI:<provider>=TOKEN entries, unknown provider rejected, order kept |
| 134 | __gash_get_ai_token returns empty for unknown provider | INTERNAL | pass | not run | `unit` | config.Load AI:<provider>=TOKEN entries, unknown provider rejected, order kept |
| 162 | __gash_get_first_ai_provider returns first configured | INTERNAL | pass | not run | `unit` | config.Load AI:<provider>=TOKEN entries, unknown provider rejected, order kept |
| 186 | gash_ai_list shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 199 | __gash_json_escape escapes quotes correctly | INTERNAL | pass | not run | `unit` | moot in Go (encoding/json) |
| 211 | __gash_json_escape escapes newlines | INTERNAL | pass | not run | `unit` | moot in Go (encoding/json) |
| 225 | __gash_json_escape escapes backslashes | INTERNAL | pass | not run | `unit` | moot in Go (encoding/json) |
| 237 | __ai_gather_context returns valid JSON with 7 fields | INTERNAL | pass | not run | `unit` | ai.Context (7 fields incl. cwd) |
| 259 | __ai_gather_context includes current directory | INTERNAL | pass | not run | `unit` | ai.Context (7 fields incl. cwd) |
| 275 | __ai_require_jq fails when jq missing | INTERNAL | pass | not run | `unit` | dependency gate |
| 294 | __ai_require_curl fails when curl missing | INTERNAL | pass | not run | `unit` | dependency gate |
| 315 | __ai_call_claude builds request and returns structured JSON with type | INTERNAL | pass | not run | `unit` | request/response shape per provider ('type' field) |
| 335 | __ai_call_gemini builds request and returns structured JSON with type | INTERNAL | pass | not run | `unit` | request/response shape per provider ('type' field) |
| 355 | __ai_call_claude handles API error | INTERNAL | pass | not run | `unit` | HTTP error mapping |
| 379 | ai_ask shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 391 | ai_query shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 403 | ai_query fails without provider configured | MIXED | pass | not run | `split` |  |
| 428 | ai_query uses first available provider (type=command) | MIXED | pass | not run | `split` |  |
| 452 | ai_query uses specified provider (type=explanation) | MIXED | pass | not run | `split` |  |
| 476 | ai_query returns fallback for generic queries | MIXED | pass | not run | `split` |  |
| 504 | __ai_format_response type=command shows Command and Explanation | INTERNAL | pass | not run | `unit` | render of response types command/explanation/fallback/troubleshoot/code |
| 521 | __ai_format_response prints model text verbatim: no \n or \c expansion | INTERNAL | pass | not run | `unit` | render of response types command/explanation/fallback/troubleshoot/code |
| 546 | __ai_format_response type=explanation shows only Explanation | INTERNAL | pass | not run | `unit` | render of response types command/explanation/fallback/troubleshoot/code |
| 564 | __ai_format_response type=fallback shows direct text (no labels) | INTERNAL | pass | not run | `unit` | render of response types command/explanation/fallback/troubleshoot/code |
| 583 | __ai_format_response type=troubleshoot shows Issue and Suggestion | INTERNAL | pass | not run | `unit` | render of response types command/explanation/fallback/troubleshoot/code |
| 600 | __ai_format_response type=code shows description and code block | INTERNAL | pass | not run | `unit` | render of response types command/explanation/fallback/troubleshoot/code |
| 616 | __ai_format_response handles missing type (defaults to fallback) | INTERNAL | pass | not run | `unit` | render of response types command/explanation/fallback/troubleshoot/code |
| 633 | ask alias is defined | BLACKBOX | pass | not run | `run` |  |
| 652 | __ai_handle_curl_error returns 0 on success (HTTP 200, no curl error) | INTERNAL | pass | not run | `unit` | curl exit 28 / HTTP 401 mapping |
| 665 | __ai_handle_curl_error returns 1 on curl exit code 28 (timeout) | INTERNAL | pass | not run | `unit` | curl exit 28 / HTTP 401 mapping |
| 682 | __ai_handle_curl_error returns 1 on HTTP 401 | INTERNAL | pass | not run | `unit` | curl exit 28 / HTTP 401 mapping |

### aliases_refactor_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 5 | loads all alias files via _loader.sh | BLACKBOX | pass | not run | `shell` |  |
| 31 | colors.sh sets LS_COLORS | BLACKBOX | pass | not run | `shell` |  |
| 52 | navigation.sh defines ls and cd aliases | BLACKBOX | pass | not run | `shell` |  |
| 79 | safety.sh defines safe file operation aliases | BLACKBOX | pass | not run | `shell` |  |
| 110 | git.sh defines git aliases and functions when git is available | BLACKBOX | pass | not run | `shell` |  |
| 151 | gl --help shows usage information | BLACKBOX | pass | not run | `shell` |  |
| 178 | git log aliases use the Gash palette, keep git's plain text and honour NO_COLOR | MIXED | pass | not run | `split` |  |
| 211 | docker.sh defines docker aliases when docker is available | BLACKBOX | pass | not run | `shell` |  |
| 239 | general.sh defines the help function and quit alias | BLACKBOX | pass | not run | `shell` |  |
| 267 | backward compatible with monolithic aliases.sh | INTERNAL | pass | not run | `static` |  |
| 275 | gash.sh prefers _loader.sh over aliases.sh | BLACKBOX | pass | not run | `shell` |  |
| 301 | gap alias resolves to git add --patch (not git_apply_patch) | BLACKBOX | pass | not run | `shell` |  |
| 320 | gpatch alias resolves to git_apply_patch | BLACKBOX | pass | not run | `shell` |  |
| 339 | quit alias resolves to services_stop, which asks | BLACKBOX | pass | not run | `shell` |  |
| 358 | dcrm alias is defined when docker is available | MOCK-DEPENDENT | pass | not run | `shell` |  |

### aliases_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | defines core navigation and safety aliases | BLACKBOX | pass | not run | `shell` |  |
| 44 | does not inject alias definitions into composer aliases | BLACKBOX | pass | not run | `shell` |  |

### chrome_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 36 | rejects unknown options | BLACKBOX | pass | not run | `run` |  |
| 44 | refuses on a platform Chrome for Testing does not publish | MOCK-DEPENDENT | pass | not run | `run*` | fake: uname stub (MOCK_ARCH) [N if runtime.GOARCH] |
| 53 | --dry-run resolves the build pinned by the latest Playwright | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl/uname/unzip/ldd stubs in $tmp/bin [curl: N if net/http] |
| 65 | plans an upgrade that keeps the current version and prunes older ones | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl/uname/unzip/ldd stubs in $tmp/bin [curl: N if net/http] |
| 79 | does nothing when the pinned build is installed | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl/uname/unzip/ldd stubs in $tmp/bin [curl: N if net/http] |
| 91 | fails cleanly when the npm registry is unreachable | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl stub (fails) [N if net/http] |
| 101 | maps missing libraries to packages, without t64 where apt lacks it | INTERNAL | pass | not run | `unit` | chrome.LibToPkg (t64 suffix only where apt has it) |

### color_gating_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 34 | __gash_color_env_ok returns 1 when GASH_HEADLESS=1 | INTERNAL | pass | not run | `unit` | render.ColorEnabled (GASH_HEADLESS, NO_COLOR, GASH_NO_COLOR, TTY) |
| 47 | __gash_color_env_ok returns 1 when NO_COLOR is set | INTERNAL | pass | not run | `unit` | render.ColorEnabled (GASH_HEADLESS, NO_COLOR, GASH_NO_COLOR, TTY) |
| 57 | __gash_color_env_ok returns 1 when GASH_NO_COLOR is set | INTERNAL | pass | not run | `unit` | render.ColorEnabled (GASH_HEADLESS, NO_COLOR, GASH_NO_COLOR, TTY) |
| 67 | __gash_use_color returns 1 on non-TTY stdout | INTERNAL | pass | not run | `unit` | render.ColorEnabled (GASH_HEADLESS, NO_COLOR, GASH_NO_COLOR, TTY) |
| 77 | __gash_color_scope emits local-shadow block when colors disabled | INTERNAL | pass | not run | `unit` | render.ColorEnabled (GASH_HEADLESS, NO_COLOR, GASH_NO_COLOR, TTY) |
| 89 | __gash_color_scope emits nothing when colors enabled | INTERNAL | pass | not run | `unit` | render.ColorEnabled (GASH_HEADLESS, NO_COLOR, GASH_NO_COLOR, TTY) |
| 107 | GASH_HEADLESS=1 produces no ANSI in __gash_error | INTERNAL | pass | not run | `unit` | render status lines plain when gated |
| 118 | NO_COLOR=1 produces no ANSI in __gash_info | INTERNAL | pass | not run | `unit` | render status lines plain when gated |
| 129 | GASH_NO_COLOR=1 produces no ANSI in __gash_warning | INTERNAL | pass | not run | `unit` | render status lines plain when gated |
| 140 | GASH_HEADLESS=1 zeroes color vars at load time | INTERNAL | pass | not run | `unit` | render status lines plain when gated |
| 156 | pipe context (non-TTY stdout) produces no ANSI in __gash_info | INTERNAL | pass | not run | `unit` | render status lines plain when gated |
| 166 | pipe context produces no ANSI in __gash_error (stderr uniform rule) | INTERNAL | pass | not run | `unit` | render status lines plain when gated |
| 179 | files_largest --no-color suppresses ANSI on data rows | BLACKBOX | pass | not run | `run` |  |
| 191 | files_largest error on forbidden path emits no ANSI under pipe | BLACKBOX | pass | not run | `run` |  |
| 202 | disk_usage --no-color strips ANSI from awk output | MOCK-DEPENDENT | pass | not run | `run*` | fake: df [Y if exec; N if statfs] |
| 213 | sysinfo --llm implies no ANSI (LLM-compact mode) | MOCK-DEPENDENT | pass | not run | `run*` | fake: df [Y if exec; N if statfs]; journalctl [Y if exec]; ps [Y if exec; N if /proc]; ss [Y if exec; N if /proc]; systemctl [Y if exec; N if D-Bus] |
| 223 | gash_doctor --no-color emits no ANSI | BLACKBOX | pass | not run | `run` |  |
| 231 | gash_help --no-color emits no ANSI on function detail | BLACKBOX | pass | not run | `run` |  |
| 240 | gash_help --no-color on --list suppresses ANSI in module listing | BLACKBOX | pass | not run | `run` |  |
| 249 | gash_help --no-color --search flf emits no ANSI | BLACKBOX | pass | not run | `run` |  |
| 258 | dirs_list_empty --json error (forbidden) emits no ANSI | BLACKBOX | pass | not run | `run` |  |
| 271 | GASH_HEADLESS=1 end-to-end: files_largest emits no ANSI | BLACKBOX | pass | not run | `run` |  |
| 288 | GASH_HEADLESS=1 end-to-end: gash_doctor emits no ANSI | BLACKBOX | pass | not run | `run` |  |
| 304 | GASH_NO_COLOR=0 keeps colors on; NO_COLOR=0 still turns them off | INTERNAL | pass | not run | `unit` | GASH_NO_COLOR=0 keeps colour, NO_COLOR=0 disables (any value) |
| 316 | the --no-color scope tests the TTY in the caller: colors on a terminal, none in a pipe | MIXED | pass | not run | `split` |  |

### completion_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | bash_completion returns early if gash functions are missing | BLACKBOX | pass | not run | `shell` |  |
| 15 | completion returns empty for no-arg functions | BLACKBOX | pass | not run | `shell` |  |
| 40 | completion suggests git tags for git_add_tag | BLACKBOX | pass | not run | `shell` |  |
| 74 | completion suggests directories for files_largest | BLACKBOX | pass | not run | `shell` |  |
| 108 | completion suggests sections for gash command | BLACKBOX | pass | not run | `shell` |  |
| 132 | completion suggests files for archive_extract | BLACKBOX | pass | not run | `shell` |  |
| 163 | completion suggests AI providers for ai_ask | BLACKBOX | pass | not run | `shell` |  |
| 188 | completion is registered for functions and their aliases | BLACKBOX | pass | not run | `shell` |  |
| 214 | aliases complete like their function, and gash lists the real topics | BLACKBOX | pass | not run | `shell` |  |

### config_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | __gash_url_decode decodes simple %XX sequences | INTERNAL | pass | not run | `unit` | config.URLDecode/Encode (%XX, @, :, plain text) |
| 17 | __gash_url_decode decodes @ symbol (%40) | INTERNAL | pass | not run | `unit` | config.URLDecode/Encode (%XX, @, :, plain text) |
| 27 | __gash_url_decode decodes colon (%3A) | INTERNAL | pass | not run | `unit` | config.URLDecode/Encode (%XX, @, :, plain text) |
| 37 | __gash_url_decode handles multiple encoded chars | INTERNAL | pass | not run | `unit` | config.URLDecode/Encode (%XX, @, :, plain text) |
| 47 | __gash_url_decode leaves plain text unchanged | INTERNAL | pass | not run | `unit` | config.URLDecode/Encode (%XX, @, :, plain text) |
| 57 | __gash_url_encode encodes @ symbol | INTERNAL | pass | not run | `unit` | config.URLDecode/Encode (%XX, @, :, plain text) |
| 67 | __gash_url_encode leaves alphanumeric unchanged | INTERNAL | pass | not run | `unit` | config.URLDecode/Encode (%XX, @, :, plain text) |
| 79 | __gash_parse_db_url parses mysql URL | INTERNAL | pass | not run | `unit` | config.ParseDBURL (split on last @, default ports 3306/5432, encoded password stays encoded) |
| 100 | __gash_parse_db_url parses pgsql URL | INTERNAL | pass | not run | `unit` | config.ParseDBURL (split on last @, default ports 3306/5432, encoded password stays encoded) |
| 120 | __gash_parse_db_url parses mariadb URL | INTERNAL | pass | not run | `unit` | config.ParseDBURL (split on last @, default ports 3306/5432, encoded password stays encoded) |
| 140 | __gash_parse_db_url handles URL-encoded password | INTERNAL | pass | not run | `unit` | config.ParseDBURL (split on last @, default ports 3306/5432, encoded password stays encoded) |
| 160 | __gash_parse_db_url handles URL without database | INTERNAL | pass | not run | `unit` | config.ParseDBURL (split on last @, default ports 3306/5432, encoded password stays encoded) |
| 179 | __gash_parse_db_url defaults mysql port to 3306 | INTERNAL | pass | not run | `unit` | config.ParseDBURL (split on last @, default ports 3306/5432, encoded password stays encoded) |
| 193 | __gash_parse_db_url defaults pgsql port to 5432 | INTERNAL | pass | not run | `unit` | config.ParseDBURL (split on last @, default ports 3306/5432, encoded password stays encoded) |
| 209 | __gash_load_env handles missing file gracefully | INTERNAL | pass | not run | `unit` | config.Load (SSH/DB entries, comments, trim, missing key files skipped, invalid URL skipped) |
| 228 | __gash_load_env parses SSH entries | INTERNAL | pass | not run | `unit` | config.Load (SSH/DB entries, comments, trim, missing key files skipped, invalid URL skipped) |
| 258 | __gash_load_env parses DB entries | INTERNAL | pass | not run | `unit` | config.Load (SSH/DB entries, comments, trim, missing key files skipped, invalid URL skipped) |
| 287 | __gash_get_db_url returns error for missing connection | INTERNAL | pass | not run | `unit` | config.Load (SSH/DB entries, comments, trim, missing key files skipped, invalid URL skipped) |
| 315 | __gash_load_env skips comments and empty lines | INTERNAL | pass | not run | `unit` | config.Load (SSH/DB entries, comments, trim, missing key files skipped, invalid URL skipped) |
| 345 | __gash_load_env trims whitespace | INTERNAL | pass | not run | `unit` | config.Load (SSH/DB entries, comments, trim, missing key files skipped, invalid URL skipped) |
| 368 | __gash_reload_env clears and reloads cache | INTERNAL | pass | not run | `unit` | config.Load (SSH/DB entries, comments, trim, missing key files skipped, invalid URL skipped) |
| 405 | __gash_load_env skips SSH keys for non-existent files | INTERNAL | pass | not run | `unit` | config.Load (SSH/DB entries, comments, trim, missing key files skipped, invalid URL skipped) |
| 430 | __gash_load_env skips invalid DB URL formats | INTERNAL | pass | not run | `unit` | config.Load (SSH/DB entries, comments, trim, missing key files skipped, invalid URL skipped) |
| 465 | gash_db_list shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 476 | gash_db_test shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 487 | gash_env_init shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 498 | gash_env_init refuses to overwrite without --force | BLACKBOX | pass | not run | `run` |  |
| 523 | gash_env_init creates config with --force | BLACKBOX | pass | not run | `run` |  |
| 552 | __gash_parse_db_url handles @ in password (URL-encoded) | INTERNAL | pass | not run | `unit` | config.ParseDBURL (split on last @, default ports 3306/5432, encoded password stays encoded) |
| 574 | __gash_parse_db_url handles raw @ in password (split on last @) | INTERNAL | pass | not run | `unit` | config.ParseDBURL (split on last @, default ports 3306/5432, encoded password stays encoded) |

### core_output_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | defines color variables | INTERNAL | pass | not run | `unit` | palette constants (internal/render) |
| 19 | __gash_info prints cyan Info prefix | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 29 | __gash_error prints red Error prefix to stderr | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 39 | __gash_success prints green OK prefix | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 49 | __gash_warning prints yellow Warning prefix | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 59 | __gash_ssh prints SSH prefix with color variants | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 75 | __gash_step prints step indicator | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 85 | __gash_debug prints only when GASH_DEBUG=1 | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 101 | __gash_print outputs colored text without prefix | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 110 | all output functions handle empty messages | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 126 | __gash_color_of maps semantic names to the palette | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 137 | __gash_table: an all-empty column is a spacer, also under set -u | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 144 | __gash_table_into: lines into an array, styled by the caller's color decision | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 156 | __gash_table: --gap and --min reproduce a fixed-width layout | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 164 | __gash_table: --cell-color styles a column per row and hides the style column | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 177 | __gash_table: --bar draws VALUE/MAX, full part in the cell style, empty part muted | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 188 | __gash_table --bar: a cell that is not VALUE/MAX prints as text (a header row given as data) | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 196 | __gash_table: --max cuts the end, --max-left keeps it | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 203 | __gash_table: --fit N shrinks a middle column to the terminal width | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 212 | __gash_kv: padded bold keys, optional value style, separator and width | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 226 | __gash_heading: bold title on a color terminal, plain text otherwise | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 234 | __gash_tty_io: true only when stdin and stdout are terminals | INTERNAL | pass | not run | `unit` |  |
| 244 | __gash_heading --note: secondary text after the title, muted in color | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 253 | __gash_usage, __gash_note and __gash_debug keep their plain prefixes and get a glyph in color | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 266 | __gash_ask_render: QUESTION [ANSWERS] on stderr, answers bold in color, question verbatim | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 275 | __gash_read_line reads a line and prompts only on a terminal | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 282 | __gash_clear_screen prints nothing when stdout is not a terminal, with NO_COLOR or headless | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 294 | __gash_color_chart: 256 names, colored only on a color terminal; __gash_sgr_extra lists its swatches | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 305 | spinner and progress print no escape sequence when stdout is not a terminal | INTERNAL | pass | not run | `unit` |  |
| 318 | __gash_list: bulleted items, bullet colored on a color terminal | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 326 | __gash_mark: glyph in the status color on a terminal, word prefix otherwise | INTERNAL | pass | not run | `unit` | internal/render golden tests |
| 337 | __gash_hint: muted Hint line on stderr | INTERNAL | pass | not run | `unit` | internal/render golden tests |

### core_utils_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | needs_help prints usage on -h | INTERNAL | pass | not run | `unit` | help.Needs (-h/--help) |
| 21 | needs_help prints usage on --help | INTERNAL | pass | not run | `unit` | help.Needs (-h/--help) |
| 33 | needs_help returns 1 for non-help args | INTERNAL | pass | not run | `unit` | help.Needs (-h/--help) |
| 46 | needs_help returns 1 for empty arg | INTERNAL | pass | not run | `unit` | help.Needs (-h/--help) |
| 59 | __gash_tty_width returns a number | INTERNAL | pass | not run | `unit` | internal/render / internal/util |
| 69 | __gash_tty_width defaults to 80 when no terminal | INTERNAL | pass | not run | `unit` | internal/render / internal/util |
| 82 | __gash_trim_ws trims leading and trailing whitespace | INTERNAL | pass | not run | `unit` | internal/render / internal/util |
| 91 | __gash_trim_ws trims tabs | INTERNAL | pass | not run | `unit` | internal/render / internal/util |
| 100 | __gash_trim_ws handles empty string | INTERNAL | pass | not run | `unit` | internal/render / internal/util |
| 109 | __gash_trim_ws handles whitespace-only string | INTERNAL | pass | not run | `unit` | internal/render / internal/util |
| 118 | __gash_expand_tilde_path expands ~/path | INTERNAL | pass | not run | `unit` | internal/render / internal/util |
| 127 | __gash_expand_tilde_path expands bare ~ | INTERNAL | pass | not run | `unit` | internal/render / internal/util |
| 136 | __gash_expand_tilde_path leaves absolute paths unchanged | INTERNAL | pass | not run | `unit` | internal/render / internal/util |
| 145 | __gash_expand_tilde_path leaves relative paths unchanged | INTERNAL | pass | not run | `unit` | internal/render / internal/util |
| 154 | all_colors prints help on -h | BLACKBOX | pass | not run | `run` |  |
| 170 | needs_confirm_prompt and __gash_confirm_plan render the same way, with their own defaults | INTERNAL | pass | not run | `unit` |  |
| 200 | needs_help fallback uses the semantic palette only | INTERNAL | pass | not run | `unit` | help.Needs (-h/--help) |
| 209 | __gash_task: glyph and colored status on a color terminal, plain text otherwise | INTERNAL | pass | not run | `unit` | internal/render / internal/util |
| 219 | __gash_json_warning prints one JSON warning line on stderr | INTERNAL | pass | not run | `unit` | {"warning":"partial_results","skipped_entries":3,"hint":...} on stderr |

### core_validation_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | __gash_require_command succeeds for existing command | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 16 | __gash_require_command fails for missing command | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 31 | __gash_require_command uses custom error message | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 46 | __gash_require_file succeeds for existing file | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 58 | __gash_require_file fails for missing file | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 73 | __gash_require_dir succeeds for existing directory | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 85 | __gash_require_dir fails for missing directory | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 100 | __gash_require_git_repo fails outside git repo | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 119 | __gash_require_git_repo succeeds inside git repo | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 133 | __gash_require_git_remote fails when remote missing | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 153 | __gash_require_arg fails for empty value | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 169 | __gash_require_arg succeeds for non-empty value | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 178 | __gash_require_docker checks docker command | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 197 | __gash_require_binary succeeds for existing binary | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 206 | __gash_require_binary fails for missing binary | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 221 | __gash_require_binary ignores aliases and functions | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 239 | __gash_require_readable succeeds for readable file | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 252 | __gash_require_readable fails for missing file | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 267 | __gash_require_git_repo_with_remote fails outside git repo | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 286 | __gash_require_git_repo_with_remote fails when remote missing | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |
| 306 | __gash_require_git_repo_with_remote succeeds with repo and remote | INTERNAL | pass | not run | `unit` | require.* messages ('not installed'/'not in PATH', 'does not exist', 'not readable', 'git repository', 'origin ... not configured') |

### docker_compose_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 11 | __gash_parse_compose_services extracts services correctly | INTERNAL | pass | not run | `unit` | compose.ParseServices (YAML subset, quoted images, build-only |
| 41 | __gash_parse_compose_services handles quoted images | INTERNAL | pass | not run | `unit` | compose.ParseServices (YAML subset, quoted images, build-only |
| 62 | __gash_parse_compose_services reads any indentation without Compose v2 | INTERNAL | pass | not run | `unit` | compose.ParseServices (YAML subset, quoted images, build-only |
| 78 | __gash_parse_compose_services prefers docker compose config --format json | INTERNAL | pass | not run | `unit` | compose.ParseServices (YAML subset, quoted images, build-only |
| 91 | __gash_parse_compose_services handles services without image (build) | INTERNAL | pass | not run | `unit` | compose.ParseServices (YAML subset, quoted images, build-only |
| 115 | __gash_resolve_env_vars expands variables from .env | INTERNAL | pass | not run | `unit` | compose.ResolveEnv (.env, ${VAR:-default}, quotes) |
| 130 | __gash_resolve_env_vars uses defaults when var not set | INTERNAL | pass | not run | `unit` | compose.ResolveEnv (.env, ${VAR:-default}, quotes) |
| 146 | __gash_resolve_env_vars handles quoted values in .env | INTERNAL | pass | not run | `unit` | compose.ResolveEnv (.env, ${VAR:-default}, quotes) |
| 167 | __gash_normalize_image handles official Docker Hub images | INTERNAL | pass | not run | `unit` | image.Normalize -> 'docker.io\ |
| 182 | __gash_normalize_image handles namespaced Docker Hub images | INTERNAL | pass | not run | `unit` | image.Normalize -> 'docker.io\ |
| 194 | __gash_normalize_image handles GHCR images | INTERNAL | pass | not run | `unit` | image.Normalize -> 'docker.io\ |
| 203 | __gash_normalize_image handles digest-pinned images | INTERNAL | pass | not run | `unit` | image.Normalize -> 'docker.io\ |
| 212 | __gash_normalize_image handles custom registries | INTERNAL | pass | not run | `unit` | image.Normalize -> 'docker.io\ |
| 228 | __gash_is_upgradeable returns true for mutable tags | INTERNAL | pass | not run | `unit` | image.IsUpgradeable (mutable/partial/pinned/suffix tags) |
| 240 | __gash_is_upgradeable returns true for partial versions | INTERNAL | pass | not run | `unit` | image.IsUpgradeable (mutable/partial/pinned/suffix tags) |
| 250 | __gash_is_upgradeable returns false for pinned versions | INTERNAL | pass | not run | `unit` | image.IsUpgradeable (mutable/partial/pinned/suffix tags) |
| 261 | __gash_is_upgradeable handles tags with suffixes | INTERNAL | pass | not run | `unit` | image.IsUpgradeable (mutable/partial/pinned/suffix tags) |
| 276 | __gash_compose_cmd detects docker compose v2 | INTERNAL | pass | not run | `unit` | compose CLI detection v2 vs v1 (exec) |
| 287 | __gash_compose_cmd falls back to docker-compose v1 | INTERNAL | pass | not run | `unit` | compose CLI detection v2 vs v1 (exec) |
| 315 | __gash_require_compose fails without docker | INTERNAL | pass | not run | `unit` | compose CLI detection v2 vs v1 (exec) |
| 335 | docker_compose_check shows help with -h | BLACKBOX | pass | not run | `run` |  |
| 346 | docker_compose_check errors on missing compose file | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl [N if net/http]; docker [Y if CLI; N if Engine API] |
| 363 | docker_compose_check --json returns valid JSON structure | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl [N if net/http]; docker [Y if CLI; N if Engine API] |
| 390 | docker_compose_check correctly identifies pinned vs upgradeable | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl [N if net/http]; docker [Y if CLI; N if Engine API] |
| 416 | docker_compose_check handles compose files with env vars | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl [N if net/http]; docker [Y if CLI; N if Engine API] |
| 447 | docker_compose_upgrade shows help with -h | BLACKBOX | pass | not run | `run` |  |
| 458 | docker_compose_upgrade errors on missing compose file | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker [Y if CLI; N if Engine API] |
| 477 | docker_compose_upgrade --dry-run shows planned actions | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker [Y if CLI; N if Engine API] |
| 506 | docker_compose_upgrade skips all pinned versions | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker [Y if CLI; N if Engine API] |
| 530 | docker_compose_upgrade --force includes pinned versions | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker [Y if CLI; N if Engine API] |
| 553 | docker_compose_upgrade executes commands in correct order | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker [Y if CLI; N if Engine API] |
| 591 | docker_compose_scan shows help with -h | BLACKBOX | pass | not run | `run` |  |
| 602 | docker_compose_scan finds compose files | BLACKBOX | pass | not run | `run` |  |
| 620 | docker_compose_scan respects depth limit | BLACKBOX | pass | not run | `run` |  |
| 637 | docker_compose_scan handles empty directory | BLACKBOX | pass | not run | `run` |  |
| 650 | docker_compose_scan finds all naming variants | BLACKBOX | pass | not run | `run` |  |
| 672 | docker compose aliases are defined | BLACKBOX | pass | not run | `run` |  |
| 686 | llm_docker_check outputs JSON | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl [N if net/http]; docker [Y if CLI; N if Engine API] |
| 713 | handles compose file with syntax edge cases | INTERNAL | pass | not run | `unit` | compose.ParseServices (YAML subset, quoted images, build-only |
| 746 | handles missing .env file gracefully | INTERNAL | pass | not run | `unit` | compose.ResolveEnv (.env, ${VAR:-default}, quotes) |
| 758 | docker_compose_check handles empty services gracefully | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl [N if net/http]; docker [Y if CLI; N if Engine API] |
| 785 | __gash_registry_curl returns 0 on HTTP 200 | INTERNAL | pass | not run | `unit` | registry client status mapping (200/401/429/timeout/500 -> error codes) |
| 799 | __gash_registry_curl returns 1 + unauthorized on HTTP 401 | INTERNAL | pass | not run | `unit` | registry client status mapping (200/401/429/timeout/500 -> error codes) |
| 817 | __gash_registry_curl returns 1 + rate_limited on HTTP 429 | INTERNAL | pass | not run | `unit` | registry client status mapping (200/401/429/timeout/500 -> error codes) |
| 834 | __gash_registry_curl returns 1 + timeout on curl exit 28 | INTERNAL | pass | not run | `unit` | registry client status mapping (200/401/429/timeout/500 -> error codes) |
| 851 | __gash_registry_curl returns 1 + server_error on HTTP 500 | INTERNAL | pass | not run | `unit` | registry client status mapping (200/401/429/timeout/500 -> error codes) |
| 872 | __gash_dockerhub_token returns token on success | INTERNAL | pass | not run | `unit` | token endpoints (auth.docker.io, ghcr.io, GITHUB_TOKEN) |
| 883 | __gash_dockerhub_token returns 1 on auth failure | INTERNAL | pass | not run | `unit` | token endpoints (auth.docker.io, ghcr.io, GITHUB_TOKEN) |
| 900 | __gash_ghcr_token returns token on success | INTERNAL | pass | not run | `unit` | token endpoints (auth.docker.io, ghcr.io, GITHUB_TOKEN) |
| 911 | __gash_ghcr_token uses GITHUB_TOKEN when available | INTERNAL | pass | not run | `unit` | token endpoints (auth.docker.io, ghcr.io, GITHUB_TOKEN) |
| 922 | __gash_ghcr_token returns 1 on auth failure | INTERNAL | pass | not run | `unit` | token endpoints (auth.docker.io, ghcr.io, GITHUB_TOKEN) |
| 943 | __gash_get_remote_digest succeeds for Docker Hub | INTERNAL | pass | not run | `unit` | manifest HEAD with manifest.list.v2+json and oci.image.index.v1+json Accept headers |
| 955 | __gash_get_remote_digest asks for multi-arch indexes on every registry | INTERNAL | pass | not run | `unit` | manifest HEAD with manifest.list.v2+json and oci.image.index.v1+json Accept headers |
| 971 | __gash_get_remote_digest succeeds for GHCR | INTERNAL | pass | not run | `unit` | manifest HEAD with manifest.list.v2+json and oci.image.index.v1+json Accept headers |
| 983 | __gash_get_remote_digest returns 1 on auth failure | INTERNAL | pass | not run | `unit` | manifest HEAD with manifest.list.v2+json and oci.image.index.v1+json Accept headers |
| 1000 | __gash_get_remote_digest returns 1 on rate limit | INTERNAL | pass | not run | `unit` | manifest HEAD with manifest.list.v2+json and oci.image.index.v1+json Accept headers |
| 1017 | __gash_get_remote_digest handles digest-pinned images | INTERNAL | pass | not run | `unit` | manifest HEAD with manifest.list.v2+json and oci.image.index.v1+json Accept headers |
| 1027 | __gash_get_remote_digest sets __gash_remote_digest variable | INTERNAL | pass | not run | `unit` | manifest HEAD with manifest.list.v2+json and oci.image.index.v1+json Accept headers |
| 1044 | docker_compose_check --json shows specific error type for rate limit | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl [N if net/http]; docker [Y if CLI; N if Engine API] |
| 1068 | docker_compose_check --json shows unauthorized for auth failure | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl [N if net/http]; docker [Y if CLI; N if Engine API] |
| 1094 | docker_compose_check --json escapes special chars in path | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl [N if net/http]; docker [Y if CLI; N if Engine API] |
| 1123 | __gash_normalize_image handles localhost registry | INTERNAL | pass | not run | `unit` | image.Normalize -> 'docker.io\ |

### docker_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 11 | docker_stop_all stops running containers | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin, MOCK_DOCKER_LOG) [Y if Go execs the docker CLI; N with the Engine API] |
| 32 | docker_stop_all is no-op when no running containers | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin, MOCK_DOCKER_LOG) [Y if Go execs the docker CLI; N with the Engine API] |
| 44 | docker_stop_all errors without docker | BLACKBOX | pass | not run | `run` |  |
| 63 | docker_start_all starts only stopped containers | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin, MOCK_DOCKER_LOG) [Y if Go execs the docker CLI; N with the Engine API] |
| 84 | docker_start_all is no-op when no stopped containers | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin, MOCK_DOCKER_LOG) [Y if Go execs the docker CLI; N with the Engine API] |
| 96 | docker_start_all errors without docker | BLACKBOX | pass | not run | `run` |  |
| 115 | docker_prune_all filters networks by name not ID | MIXED | pass | not run | `split` |  |
| 138 | docker_prune_all removes custom networks but preserves built-in | MIXED | pass | not run | `split` |  |
| 165 | docker_prune_all --dry-run counts and removes nothing; -y skips the question | MIXED | pass | not run | `split` |  |
| 187 | docker_prune_all guards empty container lists | MIXED | pass | not run | `split` |  |
| 214 | docker aliases are defined | BLACKBOX | pass | not run | `run` |  |

### docs_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 15 | docs/reference is generated from the current help registry | INTERNAL | pass | not run | `static` |  |
| 26 | every Gash function cited in the documentation exists | BLACKBOX | pass | not run | `run` |  |
| 46 | every GASH_ setting cited in the documentation is read by the code | INTERNAL | pass | not run | `static` |  |
| 56 | every page linked from the documentation exists | INTERNAL | pass | not run | `static` |  |

### envelope_schema_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 21 | size-list envelope: files_largest, dirs_largest, dirs_find_large | BLACKBOX | pass | not run | `run` |  |
| 39 | empty-dirs and tree-stats envelopes | BLACKBOX | pass | not run | `run` |  |
| 54 | llm_tree --stats: cumulative sizes and real child counts past the depth limit | BLACKBOX | pass | not run | `run` |  |
| 67 | llm_tree flags a listing cut at 500 entries | BLACKBOX | pass | not run | `run` |  |
| 75 | database envelope: every llm_db_* returns {data, rows, query_time} | BLACKBOX | pass | not run | `run` |  |
| 91 | git envelopes: llm_git_status keys and llm_git_log records | BLACKBOX | pass | not run | `run` |  |
| 106 | system and project envelopes: llm_project, llm_procs, llm_ports | MOCK-DEPENDENT | pass | not run | `run*` | fake: ss (mocks/bin) [Y if exec ss; N if /proc/net/tcp*]; ps [Y if exec] |

### error_contract_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 8 | __gash_json_error emits the documented shape and derives recoverable | INTERNAL | pass | not run | `unit` | errjson shape {error,message,action,recoverable,details} |
| 25 | llm_* and scanners agree: recoverable is false exactly for FATAL | BLACKBOX | pass | not run | `run` |  |
| 40 | a value flag at the end of the line errors instead of hanging | BLACKBOX | pass | not run | `run` |  |
| 57 | flags mean one thing: -n is a limit, --json is JSON, unknown flags fail | BLACKBOX | pass | not run | `run` |  |
| 75 | the DB functions take -n as the row limit too | BLACKBOX | pass | not run | `run` |  |
| 95 | llm_procs rejects unknown options with the unknown_option envelope | BLACKBOX | pass | not run | `run` |  |
| 108 | llm_git_status, llm_git_diff and llm_git_log agree outside a repository | BLACKBOX | pass | not run | `run` |  |
| 125 | llm_docker_check errors follow the contract and unknown digests are null | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl [N if net/http]; docker [Y if CLI; N if Engine API] |

### files_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 19 | parse_size: plain integer is bytes | INTERNAL | pass | not run | `unit` | fs.ParseSize (bytes, K/M/G/T IEC, 2GiB, invalid -> error) |
| 27 | parse_size: K/M/G/T IEC suffix | INTERNAL | pass | not run | `unit` | fs.ParseSize (bytes, K/M/G/T IEC, 2GiB, invalid -> error) |
| 37 | parse_size: invalid input returns 1 | INTERNAL | pass | not run | `unit` | fs.ParseSize (bytes, K/M/G/T IEC, 2GiB, invalid -> error) |
| 52 | human_size: zero bytes | INTERNAL | pass | not run | `unit` | fs.HumanSize ('0 B', '1.0 KiB', '1.5 MiB') |
| 59 | human_size: IEC scaling with one decimal | INTERNAL | pass | not run | `unit` | fs.HumanSize ('0 B', '1.0 KiB', '1.5 MiB') |
| 72 | files_largest --min-size filters out small files | BLACKBOX | pass | not run | `run` |  |
| 84 | files_largest --limit caps results | BLACKBOX | pass | not run | `run` |  |
| 95 | files_largest --json emits a valid envelope | BLACKBOX | pass | not run | `run` |  |
| 112 | files_largest --null emits NUL-delimited paths | BLACKBOX | pass | not run | `run` |  |
| 123 | files_largest respects default prune (node_modules) | BLACKBOX | pass | not run | `run` |  |
| 136 | files_largest --no-ignore disables pruning | BLACKBOX | pass | not run | `run` |  |
| 151 | dirs_largest --depth 2 reveals nested offenders | BLACKBOX | pass | not run | `run` |  |
| 162 | dirs_largest --json envelope is valid | BLACKBOX | pass | not run | `run` |  |
| 178 | dirs_find_large --with-mtime adds mtime column | BLACKBOX | pass | not run | `run` |  |
| 190 | dirs_find_large --json envelope is valid | BLACKBOX | pass | not run | `run` |  |
| 206 | dirs_list_empty --count outputs a number | BLACKBOX | pass | not run | `run` |  |
| 218 | dirs_list_empty --null emits NUL-delimited | BLACKBOX | pass | not run | `run` |  |
| 228 | dirs_list_empty --json envelope is valid | BLACKBOX | pass | not run | `run` |  |
| 243 | safe_path blocks / without --allow-root | BLACKBOX | pass | not run | `run` |  |
| 251 | safe_path blocks forbidden prefixes (/proc) | BLACKBOX | pass | not run | `run` |  |
| 259 | safe_path emits JSON error when --json is active | BLACKBOX | pass | not run | `run` |  |
| 268 | invalid size returns JSON error under --json | BLACKBOX | pass | not run | `run` |  |
| 281 | tree_stats prints totals and extension tops | BLACKBOX | pass | not run | `run` |  |
| 299 | tree_stats --json envelope is valid and has stats fields | BLACKBOX | pass | not run | `run` |  |
| 311 | tls alias is defined | BLACKBOX | pass | not run | `run` |  |
| 322 | llm_tree JSON default depth 1 stays flat (backward-compat) | BLACKBOX | pass | not run | `run` |  |
| 336 | llm_tree --depth 3 produces nested JSON | BLACKBOX | pass | not run | `run` |  |
| 353 | llm_tree --stats adds size and children_count | BLACKBOX | pass | not run | `run` |  |
| 370 | llm_tree rejects --depth 0 (invalid) | BLACKBOX | pass | not run | `run` |  |
| 378 | size scanners print nothing on stderr when SIGPIPE is ignored | BLACKBOX | pass | not run | `run` |  |
| 394 | extracts .tar.xz and .xz, and .tar.zst when zstd is installed | BLACKBOX | pass | not run | `run` |  |

### fs_render_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 8 | files_largest on a terminal ranks, draws share bars and sums up | MIXED | pass | not run | `split` |  |
| 31 | dirs_largest and dirs_find_large share the layout with their own titles | MIXED | pass | not run | `split` |  |
| 49 | piped output keeps the plain SIZE  PATH lines | BLACKBOX | pass | not run | `run` |  |
| 61 | tree_stats on a terminal shows grouped totals and share bars | MIXED | pass | not run | `split` |  |
| 76 | __gash_group_digits and __gash_bar | INTERNAL | pass | not run | `unit` | render.GroupDigits / render.Bar |

### functions_edge_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 11 | files_largest handles paths with spaces | BLACKBOX | pass | not run | `run` |  |
| 24 | dirs_largest handles spaces and lists only directories | BLACKBOX | pass | not run | `run` |  |
| 41 | dirs_largest lists subdirectories | BLACKBOX | pass | not run | `run` |  |
| 54 | history_grep filters history output | MOCK-DEPENDENT | pass | not run | `shell` |  |
| 73 | history_grep fails gracefully when missing arg under nounset | BLACKBOX | pass | not run | `run` |  |
| 87 | process_kill calls kill for matching processes | MOCK-DEPENDENT | pass | not run | `run*` | rewrite first: real victim process (kill hazard); fake: kill (function override) + pgrep/ps (mocks/bin) [kill: N, Go syscall.Kill would signal real PID 2222] |
| 107 | process_find fails gracefully when missing arg under nounset | BLACKBOX | pass | not run | `run` |  |
| 121 | process_kill fails without calling kill when missing arg | MOCK-DEPENDENT | pass | not run | `run*` | fake: kill (function override) [N] |
| 144 | sudo_last runs sudo for explicit command | MOCK-DEPENDENT | pass | not run | `shell` |  |
| 154 | mkdir_cd creates dir and cds | BLACKBOX | pass | not run | `shell` |  |
| 164 | mkdir_cd fails gracefully when missing arg under nounset | BLACKBOX | pass | not run | `run` |  |
| 178 | archive_extract fails gracefully when missing arg under nounset | BLACKBOX | pass | not run | `run` |  |
| 192 | file_backup fails gracefully when missing arg under nounset | BLACKBOX | pass | not run | `run` |  |
| 206 | port_kill fails gracefully when missing arg under nounset | BLACKBOX | pass | not run | `run` |  |
| 220 | docker_prune_all returns 1 when the user declines, and removes nothing | MIXED | pass | not run | `split` |  |
| 241 | git_dump_revisions writes per-commit files | BLACKBOX | pass | not run | `run` |  |
| 269 | git_dump_revisions fails gracefully when missing arg under nounset | BLACKBOX | pass | not run | `run` |  |
| 283 | git_apply_patch fails gracefully when missing args under nounset | BLACKBOX | pass | not run | `run` |  |
| 304 | git_apply_patch errors outside git repo | BLACKBOX | pass | not run | `run` |  |
| 316 | gash_uninstall returns error when not installed | BLACKBOX | pass | not run | `run` |  |

### functions_more_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 11 | disk_usage formats filtered filesystem types | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin; MOCK_DF_NO_TYPE) [Y if exec df; N if statfs] |
| 27 | disk_usage falls back to df -Pk without types (BSD/macOS) | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin; MOCK_DF_NO_TYPE) [Y if exec df; N if statfs] |
| 38 | disk_usage colors the bar by fill: green, yellow from 75%, red from 90% | MIXED | pass | not run | `split` |  |
| 50 | dirs_find_large works without numfmt (pure-bash IEC fallback) | MIXED | pass | not run | `split` |  |
| 74 | dirs_find_large lists a directory over threshold | BLACKBOX | pass | not run | `run` |  |
| 89 | dirs_find_large sorts by size and supports K/M suffix | BLACKBOX | pass | not run | `run` |  |
| 110 | dirs_list_empty lists empty dirs | BLACKBOX | pass | not run | `run` |  |
| 123 | process_find prints error when no match | BLACKBOX | pass | not run | `run` |  |
| 135 | services_stop -y with nothing running succeeds quietly | MIXED | pass | not run | `split` |  |
| 147 | services_stop lists, refuses without a terminal, stops with -y | MIXED | pass | not run | `split` |  |
| 164 | services_stop falls back to service without systemd | MIXED | pass | not run | `split` |  |
| 179 | docker_start_all errors when docker missing | BLACKBOX | pass | not run | `run` |  |
| 191 | docker_prune_all errors when docker missing (no prompt) | BLACKBOX | pass | not run | `run` |  |
| 203 | gash_inspiring_quote prints a quote when file present | BLACKBOX | pass | not run | `run` |  |
| 217 | gash_username returns a non-empty string | BLACKBOX | pass | not run | `run` |  |
| 226 | process_kill without a terminal refuses unless --force, whatever the number of matches | MOCK-DEPENDENT | pass | not run | `run*` | rewrite first: real victim process (kill hazard); fake: kill override + pgrep stub [kill: N] |
| 263 | gash_inspiring_quote skips source comments and shows them with --source | BLACKBOX | pass | not run | `run` |  |
| 283 | every quote has a source line, the right shape, and no duplicate | INTERNAL | pass | not run | `static` |  |

### functions_smoke_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 11 | gash_help displays grouped overview | BLACKBOX | pass | not run | `run` |  |
| 25 | needs_help prints usage on -h | INTERNAL | pass | not run | `unit` |  |
| 34 | __gash_trim_ws trims tabs/spaces | INTERNAL | pass | not run | `unit` |  |
| 43 | file_backup creates timestamped backup | BLACKBOX | pass | not run | `run` |  |
| 54 | archive_extract rejects missing file | BLACKBOX | pass | not run | `run` |  |
| 63 | files_largest errors on missing directory | BLACKBOX | pass | not run | `run` |  |
| 72 | files_largest lists files in directory | BLACKBOX | pass | not run | `run` |  |
| 85 | ip_public fails gracefully without curl/wget | BLACKBOX | pass | not run | `run` |  |
| 97 | port_kill reports when lsof missing | BLACKBOX | pass | not run | `run` |  |
| 109 | docker_stop_all errors when docker missing | BLACKBOX | pass | not run | `run` |  |
| 123 | wsl_restart shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 132 | wsl_shutdown shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 141 | wsl_explorer shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 150 | wsl_taskmanager shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 159 | wsl_restart fails outside WSL with mocked detection | MIXED | pass | not run | `split` |  |
| 179 | wsl_explorer fails outside WSL with mocked detection | MIXED | pass | not run | `split` |  |
| 198 | wr alias points to wsl_restart | BLACKBOX | pass | not run | `run` |  |
| 207 | wex alias points to wsl_explorer | BLACKBOX | pass | not run | `run` |  |
| 218 | port_kill targets listeners only, validates the port and asks first | MOCK-DEPENDENT | pass | not run | `run*` | fake: lsof stub in $tmp/bin [Y if exec]; real `sleep` victim |

### gash_card_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 8 | gash all lists every registered function | MIXED | pass | not run | `split` |  |
| 20 | the overview counts add up to the registry | MIXED | pass | not run | `split` |  |
| 28 | gash topics show aliases with their expansion, from the alias files | MIXED | pass | not run | `split` |  |
| 42 | gash accepts the old section names and refuses unknown topics | BLACKBOX | pass | not run | `run` |  |
| 54 | gash writes no escape bytes when piped, and the box lines line up | BLACKBOX | pass | not run | `run` |  |
| 66 | __gash_box keeps its border aligned with colored and over-wide lines | INTERNAL | pass | not run | `unit` | render.Box / render.Table --fit |
| 79 | __gash_table --fit truncates the last column and --color skips the header | INTERNAL | pass | not run | `unit` | render.Box / render.Table --fit |

### gash_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | gash_doctor shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 21 | gash_doctor runs without errors when GASH_DIR is set | BLACKBOX | pass | not run | `run` |  |
| 39 | gash_doctor detects missing modules gracefully | BLACKBOX | pass | not run | `run` |  |
| 54 | gash_doctor reports files that failed to load | MIXED | pass | not run | `split` |  |
| 69 | gdoctor alias resolves to gash_doctor | BLACKBOX | pass | not run | `run` |  |

### gash_upgrade_no_exit_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 11 | gash_upgrade returns instead of exiting on cd failure | BLACKBOX | pass | not run | `run` |  |

### git_repair_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 11 | errors outside a git repository | BLACKBOX | pass | not run | `run` |  |
| 23 | fails gracefully when origin is missing | BLACKBOX | pass | not run | `run` |  |
| 41 | repairs a repo with corrupted loose objects | BLACKBOX | pass | not run | `run` |  |
| 85 | repairs inside a linked worktree, and --dry-run changes nothing | BLACKBOX | pass | not run | `run` |  |
| 115 | says there is nothing to repair on a healthy repository | BLACKBOX | pass | not run | `run` |  |

### git_sync_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 23 | commits new, modified and deleted files and pushes | BLACKBOX | pass | not run | `run` |  |
| 38 | --dry-run shows the plan and changes nothing | BLACKBOX | pass | not run | `run` |  |
| 49 | --no-push commits only | BLACKBOX | pass | not run | `run` |  |
| 59 | pushes a new branch to origin and sets its upstream | BLACKBOX | pass | not run | `run` |  |
| 70 | pushes pending commits even with nothing to commit | BLACKBOX | pass | not run | `run` |  |
| 81 | refuses on a detached HEAD and during a merge | BLACKBOX | pass | not run | `run` |  |
| 95 | rejects unknown options and -m without a message | BLACKBOX | pass | not run | `run` |  |

### git_tags_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 11 | fails gracefully when origin is missing | BLACKBOX | pass | not run | `run` |  |
| 29 | creates and deletes tags against a local origin | BLACKBOX | pass | not run | `run` |  |
| 61 | tag checks are exact: v1.0 is not v1.0.1, nor a branch called v1.0 | BLACKBOX | pass | not run | `run` |  |

### headless_contract_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 47 | llm_grep with zero matches returns 0 under pipefail | BLACKBOX | pass | not run | `run` |  |
| 53 | llm_find with zero matches returns 0 under pipefail | BLACKBOX | pass | not run | `run` |  |
| 58 | llm_grep/llm_find reject non-integer --limit (C8 family) | BLACKBOX | pass | not run | `run` |  |
| 71 | llm_tree emits valid JSON on hostile sandbox | BLACKBOX | pass | not run | `run` |  |
| 77 | llm_project emits valid JSON | BLACKBOX | pass | not run | `run` |  |
| 83 | llm_env emits valid JSON (multi-line env value) | BLACKBOX | pass | not run | `run` |  |
| 89 | llm_procs emits valid JSON under it-style locale input | BLACKBOX | pass | not run | `run` |  |
| 94 | llm_ports emits valid JSON | MOCK-DEPENDENT | pass | not run | `run*` | fake: ss (mocks/bin) [Y if exec ss] |
| 99 | llm_git_status emits valid JSON with hostile filenames (C5) | BLACKBOX | pass | not run | `run` |  |
| 112 | llm_git_log emits valid JSON with quotes in subject (C6) | BLACKBOX | pass | not run | `run` |  |
| 125 | llm_db_query -r injection payload is rejected before the engine | BLACKBOX | pass | not run | `run` |  |
| 139 | llm_db_sample rejects non-integer --limit | BLACKBOX | pass | not run | `run` |  |
| 156 | files_largest survives unreadable dir and tiny --limit (C4) | BLACKBOX | pass | not run | `run` |  |
| 163 | dirs_largest emits valid JSON on hostile sandbox | BLACKBOX | pass | not run | `run` |  |
| 169 | dirs_find_large emits valid JSON and excludes the root (M15) | BLACKBOX | pass | not run | `run` |  |
| 178 | dirs_list_empty and tree_stats emit valid JSON on hostile sandbox | BLACKBOX | pass | not run | `run` |  |
| 185 | files_largest --null emits intact NUL records (A10) | BLACKBOX | pass | not run | `run` |  |
| 197 | files_largest honors PATH after -- separator (M17) | BLACKBOX | pass | not run | `run` |  |
| 207 | sysinfo identity/storage/all --llm survive pipefail (C3) | BLACKBOX | pass | not run | `run` |  |
| 214 | history_grep with zero matches returns 0 (M4) | BLACKBOX | pass | not run | `run` |  |
| 219 | __gash_url_decode survives errexit and keeps newlines (A6/M9) | INTERNAL | pass | not run | `unit` | config.URLDecode keeps newlines |
| 225 | __gash_url_encode is UTF-8 byte-wise (M6) | INTERNAL | pass | not run | `unit` | config.URLDecode keeps newlines |
| 235 | config warnings never contaminate captured DB URLs (C12) | INTERNAL | pass | not run | `unit` | warnings go to stderr, never into captured DB URL |
| 247 | needs_confirm_prompt refuses in headless mode, no ghost confirm (C13) | INTERNAL | pass | not run | `unit` | confirm refuses headless |

### headless_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 5 | loads core functions in headless mode | INTERNAL | pass | not run | `unit` | checks type __gash_info/__gash_error/__gash_success |
| 31 | loads LLM functions in headless mode | BLACKBOX | pass | not run | `run` |  |
| 59 | does not modify PS1/PS2 in headless mode | BLACKBOX | pass | not run | `run` |  |
| 88 | produces no output in headless mode | BLACKBOX | pass | not run | `run` |  |
| 108 | does not load aliases in headless mode | BLACKBOX | pass | not run | `run` |  |
| 135 | does not source user bash_local in headless mode | BLACKBOX | pass | not run | `run` |  |
| 160 | works in non-interactive shell | BLACKBOX | pass | not run | `run` |  |

### help_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | __gash_register_help stores help text | INTERNAL | pass | not run | `unit` | help.Register (text, short, aliases, module, see-also, empty name rejected) |
| 15 | __gash_register_help stores short description | INTERNAL | pass | not run | `unit` | help.Register (text, short, aliases, module, see-also, empty name rejected) |
| 23 | __gash_register_help stores aliases | INTERNAL | pass | not run | `unit` | help.Register (text, short, aliases, module, see-also, empty name rejected) |
| 31 | __gash_register_help stores module | INTERNAL | pass | not run | `unit` | help.Register (text, short, aliases, module, see-also, empty name rejected) |
| 39 | __gash_register_help stores see-also | INTERNAL | pass | not run | `unit` | help.Register (text, short, aliases, module, see-also, empty name rejected) |
| 47 | __gash_register_help rejects empty func name | INTERNAL | pass | not run | `unit` | help.Register (text, short, aliases, module, see-also, empty name rejected) |
| 59 | help registry is populated with >40 entries after full load | INTERNAL | pass | not run | `unit` | registry size > 40 (85 today) |
| 70 | gash_help shows help for known function | BLACKBOX | pass | not run | `run` |  |
| 82 | gash_help resolves alias to function | BLACKBOX | pass | not run | `run` |  |
| 93 | gash_help unknown function returns error | BLACKBOX | pass | not run | `run` |  |
| 107 | gash_help no args shows overview | BLACKBOX | pass | not run | `run` |  |
| 119 | gash_help --short returns one line | BLACKBOX | pass | not run | `run` |  |
| 131 | gash_help --bash delegates to builtin help | BLACKBOX | pass | not run | `shell` |  |
| 141 | gash_help -h shows self-help | BLACKBOX | pass | not run | `run` |  |
| 155 | gash_help --search finds by function name | BLACKBOX | pass | not run | `run` |  |
| 165 | gash_help --search finds by alias | BLACKBOX | pass | not run | `run` |  |
| 175 | gash_help --search no results shows message | BLACKBOX | pass | not run | `run` |  |
| 190 | gash_help --list shows grouped output | BLACKBOX | pass | not run | `run` |  |
| 203 | gash_help --list includes aliases in output | BLACKBOX | pass | not run | `run` |  |
| 216 | ai_query help includes multi-piping examples | BLACKBOX | pass | not run | `run` |  |
| 227 | gash_help --short resolves alias | BLACKBOX | pass | not run | `run` |  |

### hgrep_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 6 | hgrep deduplicates keeping last execution | BLACKBOX | pass | not run | `run` |  |
| 33 | hgrep respects --limit option | BLACKBOX | pass | not run | `run` |  |
| 57 | hgrep excludes self-references by default | BLACKBOX | pass | not run | `run` |  |
| 77 | hgrep includes self-references with -a flag | BLACKBOX | pass | not run | `run` |  |
| 96 | hgrep --reverse shows oldest first | BLACKBOX | pass | not run | `run` |  |
| 117 | hgrep --json outputs valid JSON | BLACKBOX | pass | not run | `run` |  |
| 141 | hgrep --count returns correct count | BLACKBOX | pass | not run | `run` |  |
| 163 | hgrep --regex enables extended regex | BLACKBOX | pass | not run | `run` |  |
| 185 | hgrep fails gracefully on missing pattern | BLACKBOX | pass | not run | `run` |  |
| 199 | hgrep shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 210 | hgrep rejects unknown options | BLACKBOX | pass | not run | `run` |  |
| 224 | hgrep returns empty JSON array when no matches | BLACKBOX | pass | not run | `run` |  |
| 241 | hgrep returns 0 for count when no matches | BLACKBOX | pass | not run | `run` |  |
| 258 | hgrep handles commands with special characters | BLACKBOX | pass | not run | `run` |  |
| 275 | hgrep JSON escapes special characters | BLACKBOX | pass | not run | `run` |  |

### hygiene_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 9 | interactive load leaks no unprefixed globals | BLACKBOX | pass | not run | `shell` |  |
| 31 | umask is left alone unless GASH_UMASK is set | BLACKBOX | pass | not run | `shell` |  |
| 47 | existing PROMPT_COMMAND string is kept and the hook is added once | BLACKBOX | pass | not run | `shell` |  |
| 61 | existing PROMPT_COMMAND array is kept (bash 5.1+) | BLACKBOX | pass | not run | `shell` |  |
| 81 | prompt hook preserves the exit status for later hooks | BLACKBOX | pass | not run | `shell` |  |
| 93 | loading leaves no RETURN/DEBUG trap and no shell options behind | BLACKBOX | pass | not run | `shell` |  |
| 108 | a set -euo pipefail caller survives sourcing in both modes | BLACKBOX | pass | not run | `shell` |  |
| 120 | headless mode prints nothing and leaves the prompt untouched | MIXED | pass | not run | `split` |  |
| 135 | non-Bash shells get a clear message instead of a parse error | BLACKBOX | pass | not run | `shell` |  |
| 144 | every public function answers --help without side effects | BLACKBOX | pass | not run | `run` |  |
| 171 | commands leave no RETURN trap and keep the caller EXIT trap | BLACKBOX | pass | not run | `run` |  |
| 185 | cleanup traps call command rm, so the user's rm alias never applies | INTERNAL | pass | not run | `static` |  |

### install_uninstall_upgrade_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 10 | install.sh installs into HOME and updates profile | BLACKBOX | pass | not run | `run` |  |
| 31 | gash_uninstall removes gash and cleans profile | MIXED | pass | not run | `split` |  |
| 66 | gash_upgrade updates from older tag to latest tag | BLACKBOX | pass | not run | `run` |  |
| 114 | gash_uninstall preserves other if/fi blocks in profile | MIXED | pass | not run | `split` |  |
| 151 | gash_uninstall removes new format with delimiters | MIXED | pass | not run | `split` |  |
| 186 | gash_uninstall removes custom GASH_DIR location | MIXED | pass | not run | `split` |  |
| 203 | gash_uninstall removes the settings file named by GASH_ENV_FILE | MIXED | pass | not run | `split` |  |
| 222 | gash_uninstall cleans all profile files | MIXED | pass | not run | `split` |  |
| 251 | gash_uninstall handles missing profile files gracefully | MIXED | pass | not run | `split` |  |
| 268 | gash_uninstall refuses a GASH_DIR that is not a Gash checkout | MIXED | pass | not run | `split` |  |
| 289 | install then uninstall leaves system clean | MIXED | pass | not run | `split` |  |
| 317 | gash_upgrade reports already up-to-date | BLACKBOX | pass | not run | `run` |  |
| 354 | install.sh uses new delimited format | BLACKBOX | pass | not run | `run` |  |
| 372 | install.sh refuses to update a checkout with local modifications | BLACKBOX | pass | not run | `run` |  |
| 387 | install.sh without a terminal and without --assume-yes fails instead of silently aborting | BLACKBOX | pass | not run | `run` |  |
| 402 | install.sh rejects unknown options | BLACKBOX | pass | not run | `run` |  |
| 410 | install.sh into a custom GASH_DIR writes that location into ~/.gashrc | BLACKBOX | pass | not run | `run` |  |
| 435 | install.sh installs a specific ref with GASH_INSTALL_REF | BLACKBOX | pass | not run | `run` |  |
| 462 | gash_upgrade picks the highest version, not the newest tag | BLACKBOX | pass | not run | `run` |  |

### llm_db_names_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 32 | -d must be a plain database name | MOCK-DEPENDENT | pass | not run | `run*` | fake: mysql/mariadb/psql argv stubs ($tmp/bin) [Y if exec clients; N with native drivers] |
| 48 | MySQL gets --database=NAME, PostgreSQL gets PGDATABASE (never -d) | MOCK-DEPENDENT | pass | not run | `run*` | fake: mysql/mariadb/psql argv stubs ($tmp/bin) [Y if exec clients; N with native drivers] |
| 65 | .target-database is read only when it holds a plain name | MOCK-DEPENDENT | pass | not run | `run*` | fake: mysql/mariadb/psql argv stubs ($tmp/bin) [Y if exec clients; N with native drivers] |
| 92 | MySQL TSV escapes decode left to right (an escaped backslash before 0) | INTERNAL | pass | not run | `unit` | db.MySQLTSVToJSON (escapes decoded left to right) |

### llm_db_security_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 9 | the SQL guard refuses client commands, comment tricks and second statements | INTERNAL | pass | not run | `unit` | sql.Guard verdict table (client commands, comment tricks, second statements |
| 37 | the SQL guard accepts ordinary read queries | INTERNAL | pass | not run | `unit` | sql.Guard verdict table (client commands, comment tricks, second statements |
| 54 | llm_db_query on SQLite caps rows, keeps comments harmless and returns empty envelopes | BLACKBOX | pass | not run | `run` |  |
| 73 | llm_db_query on SQLite never writes, even past the guard | MIXED | pass | not run | `split` |  |
| 89 | llm_db_query sends MySQL a read-only session with a row cap, SQL on stdin | MOCK-DEPENDENT | pass | not run | `run*` | fake: mysql stub [Y if exec; N native] |
| 127 | llm_db_query runs PostgreSQL read-only with a timeout and standard strings, SQL on stdin | MOCK-DEPENDENT | pass | not run | `run*` | fake: psql stub [Y if exec; N pgx] |
| 158 | llm_db_* reject unknown options and stray arguments | BLACKBOX | pass | not run | `run` |  |

### llm_exec_security_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 8 | llm_exec runs plain read-only command lines in a child shell | BLACKBOX | pass | not run | `run` |  |
| 30 | llm_exec refuses expansions, redirections, subshells and unknown commands | BLACKBOX | pass | not run | `run` |  |
| 55 | llm_exec refuses writing options of allowed commands | BLACKBOX | pass | not run | `run` |  |
| 75 | llm_exec checks what unquoted globs expand to | BLACKBOX | pass | not run | `run` |  |
| 91 | llm_exec runs sed and awk sandboxed | BLACKBOX | pass | not run | `run` |  |
| 107 | llm_exec ignores exported functions that shadow allowed commands | BLACKBOX | pass | not run | `run` |  |
| 117 | llm_exec settings come from ~/.gash_env, never from the environment | MIXED | pass | not run | `split` |  |
| 137 | llm_exec wraps the child in bwrap when GASH_LLM_SANDBOX=1 | MIXED | pass | not run | `split` |  |
| 161 | __llm_is_secret_file covers keys, credentials, secret dirs and symlinks | INTERNAL | pass | not run | `unit` | llm/guard secret-file and path tables |
| 179 | llm_grep and llm_find never read secret files | MIXED | pass | not run | `split` |  |
| 201 | __llm_validate_path blocks whole system trees, not look-alike names | INTERNAL | pass | not run | `unit` | llm/guard secret-file and path tables |
| 212 | llm_exec refuses forbidden system paths in arguments, globs and after cd | BLACKBOX | pass | not run | `run` |  |
| 248 | llm_exec reads ~/.gash_env once per call and prints each warning once | MIXED | pass | not run | `split` |  |
| 264 | the grep fallback (no rg on PATH) skips nested secret directories | BLACKBOX | pass | not run | `run` |  |

### llm_exec_walk_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 8 | grep -r, egrep, rg and diff -r leave secret files out | BLACKBOX | pass | not run | `run` |  |
| 40 | walks that follow symlinks are refused | BLACKBOX | pass | not run | `run` |  |
| 57 | walks cannot start at / or above a protected tree | BLACKBOX | pass | not run | `run` |  |
| 82 | git commands that print contents leave secret files out | BLACKBOX | pass | not run | `run` |  |
| 135 | git in llm_exec takes no optional locks: status does not write the index | BLACKBOX | pass | not run | `run` |  |
| 154 | the llm_exec child gets a short allowlisted environment | MIXED | pass | not run | `split` |  |
| 176 | yq load functions and split files, getent shadow are refused | BLACKBOX | pass | not run | `run` |  |

### llm_flags_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 9 | --help and -h work anywhere on the line | BLACKBOX | pass | not run | `run` |  |
| 25 | functions without options reject them; extra arguments are refused | BLACKBOX | pass | not run | `run` |  |
| 56 | llm_find PATTERN --contains honours PATTERN, with rg and with grep alike | MIXED | pass | not run | `split` |  |
| 84 | llm_procs prints one compact JSON array | BLACKBOX | pass | not run | `run` |  |
| 97 | llm_procs --name leaves out the calling shell, its parents and their subshells | BLACKBOX | pass | not run | `run` |  |
| 106 | llm_env hides secret names and masks token values | BLACKBOX | pass | not run | `run` |  |

### llm_portability_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 8 | timings work when EPOCHREALTIME uses a decimal comma (it_IT, de_DE) | MIXED | pass | not run | `split` |  |
| 25 | a slow-looking clock never makes llm_db_query run twice | MIXED | pass | not run | `split` |  |
| 48 | path guards resolve symlinks without realpath -m (BSD, older macOS) | INTERNAL | pass | not run | `unit` | guard symlink resolution (filepath.EvalSymlinks) |
| 79 | llm_env works where env has no -0 (BSD) | MOCK-DEPENDENT | pass | not run | `run*` | fake: env without -0 stub [moot: Go reads os.Environ] |
| 92 | llm_config converts YAML with either yq flavour and falls back to the raw text | MIXED | pass | not run | `split` |  |
| 120 | llm_config refuses files over --max-bytes (1 MiB by default) with a JSON error | BLACKBOX | pass | not run | `run` |  |

### llm_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 14 | __llm_validate_command blocks rm -rf / | INTERNAL | pass | not run | `unit` | llm/guard command denylist (rm -rf / with spaces/tabs, dd if=, fork bomb) |
| 30 | __llm_validate_command blocks dd if= | INTERNAL | pass | not run | `unit` | llm/guard command denylist (rm -rf / with spaces/tabs, dd if=, fork bomb) |
| 46 | __llm_validate_command blocks fork bomb | INTERNAL | pass | not run | `unit` | llm/guard command denylist (rm -rf / with spaces/tabs, dd if=, fork bomb) |
| 62 | __llm_validate_command allows safe commands | INTERNAL | pass | not run | `unit` | llm/guard command denylist (rm -rf / with spaces/tabs, dd if=, fork bomb) |
| 74 | __llm_validate_path blocks path traversal | INTERNAL | pass | not run | `unit` | llm/guard path traversal/forbidden |
| 90 | __llm_validate_path blocks forbidden paths | INTERNAL | pass | not run | `unit` | llm/guard path traversal/forbidden |
| 106 | __llm_validate_path allows safe paths | INTERNAL | pass | not run | `unit` | llm/guard path traversal/forbidden |
| 120 | __llm_is_secret_file detects .env files | INTERNAL | pass | not run | `unit` | llm/guard secret files (.env) |
| 132 | __llm_is_secret_file allows regular files | INTERNAL | pass | not run | `unit` | llm/guard secret files (.env) |
| 148 | llm_exec blocks dangerous commands | BLACKBOX | pass | not run | `run` |  |
| 164 | llm_exec executes safe commands | BLACKBOX | pass | not run | `run` |  |
| 175 | llm_exec shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 191 | llm_tree outputs JSON by default | BLACKBOX | pass | not run | `run` |  |
| 204 | llm_tree outputs text with --text | BLACKBOX | pass | not run | `run` |  |
| 216 | llm_tree shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 231 | llm_find finds files by pattern | BLACKBOX | pass | not run | `run` |  |
| 242 | llm_find shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 257 | llm_grep finds patterns in files | BLACKBOX | pass | not run | `run` |  |
| 268 | llm_grep shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 283 | llm_project detects project info | BLACKBOX | pass | not run | `run` |  |
| 301 | llm_project shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 316 | llm_config blocks .env files | BLACKBOX | pass | not run | `run` |  |
| 336 | llm_config reads JSON files | BLACKBOX | pass | not run | `run` |  |
| 356 | llm_git_status outputs JSON in git repo | BLACKBOX | pass | not run | `run` |  |
| 379 | llm_git_status shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 394 | llm_git_log outputs JSON array | BLACKBOX | pass | not run | `run` |  |
| 421 | llm_ports outputs JSON array | MOCK-DEPENDENT | pass | not run | `run*` | fake: ss (mocks/bin) [Y if exec ss; N if /proc] |
| 433 | llm_ports shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 448 | llm_env outputs JSON object | BLACKBOX | pass | not run | `run` |  |
| 460 | llm_env filters out secrets | BLACKBOX | pass | not run | `run` |  |
| 478 | llm_env shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 493 | llm_db_query blocks write operations | BLACKBOX | pass | not run | `run` |  |
| 510 | llm_db_query blocks DELETE | BLACKBOX | pass | not run | `run` |  |
| 527 | llm_db_query blocks DROP | BLACKBOX | pass | not run | `run` |  |
| 544 | llm_db_schema validates table name | BLACKBOX | pass | not run | `run` |  |
| 565 | __llm_validate_command blocks rm -rf / with extra spaces | INTERNAL | pass | not run | `unit` | llm/guard command denylist (rm -rf / with spaces/tabs, dd if=, fork bomb) |
| 581 | __llm_validate_command blocks rm -rf / with tabs | INTERNAL | pass | not run | `unit` | llm/guard command denylist (rm -rf / with spaces/tabs, dd if=, fork bomb) |
| 598 | __llm_validate_command blocks rm -rf / with leading/trailing spaces | INTERNAL | pass | not run | `unit` | llm/guard command denylist (rm -rf / with spaces/tabs, dd if=, fork bomb) |
| 618 | llm_db_query blocks semicolon injection | BLACKBOX | pass | not run | `run` |  |
| 635 | llm_db_query blocks UPDATE | BLACKBOX | pass | not run | `run` |  |
| 652 | llm_db_query blocks TRUNCATE | BLACKBOX | pass | not run | `run` |  |
| 673 | llm_deps detects PHP project dependencies | BLACKBOX | pass | not run | `run` |  |
| 689 | llm_deps detects Node.js project dependencies | BLACKBOX | pass | not run | `run` |  |
| 705 | llm_deps shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 720 | llm_git_diff shows diff stats | BLACKBOX | pass | not run | `run` |  |
| 742 | llm_git_diff shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 757 | llm_procs shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 772 | llm_db_tables shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 788 | llm_db_sample validates table name | BLACKBOX | pass | not run | `run` |  |
| 805 | llm_db_sample shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 821 | llm_db_explain shows help on -h | BLACKBOX | pass | not run | `run` |  |
| 833 | llm_db_explain blocks semicolon injection | BLACKBOX | pass | not run | `run` |  |
| 854 | __llm_clean_error_msg collapses whitespace and escapes quotes | INTERNAL | pass | not run | `unit` | db error cleaning (whitespace, quotes, MySQL password warning) |
| 865 | __llm_clean_error_msg with --mysql filters password warning | INTERNAL | pass | not run | `unit` | db error cleaning (whitespace, quotes, MySQL password warning) |
| 879 | __llm_clean_error_msg handles empty input | INTERNAL | pass | not run | `unit` | db error cleaning (whitespace, quotes, MySQL password warning) |
| 894 | __llm_resolve_db returns error for missing connection | INTERNAL | pass | not run | `unit` | db resolve -> no_db_config |

### llm_system_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | llm_ports parses ss output with a Netid column and honors --listen | MOCK-DEPENDENT | pass | not run | `run*` | fake: ss stub with Netid column [Y if exec ss; N if /proc] |
| 32 | llm_ports reports a missing ss and netstat as a CONTINUE error | BLACKBOX | pass | not run | `run` |  |
| 49 | llm_project reports has_tests=false for a Go module without tests | BLACKBOX | pass | not run | `run` |  |
| 62 | llm_git_status reports renames, odd names, conflicts and ahead/behind | BLACKBOX | pass | not run | `run` |  |
| 92 | llm_env honors --filter, redacts URL passwords and hides secret names | BLACKBOX | pass | not run | `run` |  |
| 103 | llm_deps reads requirements.txt and go.mod into the same JSON shape | BLACKBOX | pass | not run | `run` |  |
| 116 | llm_git_* never run programs named by the repository config | BLACKBOX | pass | not run | `run` |  |

### migrate_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 45 | detects a 1.x ~/.gashrc and nothing else | MIXED | pass | not run | `split` |  |
| 54 | outside a terminal Gash prints one reminder a day on stderr | BLACKBOX | pass | not run | `run` |  |
| 64 | --yes applies the recommended choices, backs up, and --rollback restores | MIXED | pass | not run | `split` |  |
| 89 | each screen choice lands in ~/.gash_env as the matching setting | MIXED | pass | not run | `split` |  |
| 106 | nothing is written on cancel, on later, or with a settings file to create | MIXED | pass | not run | `split` |  |
| 131 | gash migrate --status and option errors | BLACKBOX | pass | not run | `run` |  |
| 137 | __gash_choose reads a number, takes the default, and gives up on bad input | INTERNAL | pass | not run | `unit` | tui.Choose (numbers, default, bad input |
| 150 | __gash_choose follows arrow keys, j/k and digits on a terminal | INTERNAL | pass | not run | `unit` | tui.Choose (numbers, default, bad input |
| 160 | a missing SSH key is not a line to comment out | INTERNAL | pass | not run | `unit` | migrate env problems |
| 167 | the inventory helpers succeed when they find nothing (set -e callers) | INTERNAL | pass | not run | `unit` | migrate inventory helpers return 0 on empty |

### node_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 16 | errors when nvm is not loaded | BLACKBOX | pass | not run | `shell` |  |
| 25 | dry-run (default) lists duplicates without uninstalling | MOCK-DEPENDENT | pass | not run | `shell` |  |
| 49 | --force uninstalls only older duplicate patches, never current or latest | MOCK-DEPENDENT | pass | not run | `shell` |  |
| 81 | errors when NVM_DIR does not exist | BLACKBOX | pass | not run | `run` |  |
| 91 | reads .nvmrc (highest priority) | BLACKBOX | pass | not run | `run` |  |
| 108 | falls back to package.json engines.node | BLACKBOX | pass | not run | `run` |  |
| 124 | falls back to latest installed when nothing specified | BLACKBOX | pass | not run | `run` |  |
| 138 | warns and substitutes the closest version when the requested major is not installed | BLACKBOX | pass | not run | `run` |  |
| 156 | matches the exact JSON field names of the legacy detect-node-version.sh contract | BLACKBOX | pass | not run | `run` |  |

### node_upgrade_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 28 | rejects unknown options | BLACKBOX | pass | not run | `run` |  |
| 36 | errors when nvm is not loaded | BLACKBOX | pass | not run | `run` |  |
| 44 | --dry-run prints the plan per installed major and changes nothing | MOCK-DEPENDENT | pass | not run | `shell` |  |
| 62 | refuses without a terminal unless -y | MOCK-DEPENDENT | pass | not run | `shell` |  |
| 73 | -y installs with the old globals, switches, then removes old patches | MOCK-DEPENDENT | pass | not run | `shell` |  |
| 88 | keeps the patches of a major whose install failed | MOCK-DEPENDENT | pass | not run | `shell` |  |
| 100 | reports up to date when every major is on its newest patch | MOCK-DEPENDENT | pass | not run | `shell` |  |
| 112 | fails without touching anything when the remote index is unreadable | MOCK-DEPENDENT | pass | not run | `shell` |  |
| 125 | resolves lts/<name> through nvm alias files, also new codenames | INTERNAL | pass | not run | `unit` | node.ParseVersionToMajor via alias files |

### output_style_lint_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 147 | the scan sees every shell file of lib/ and gash.sh | INTERNAL | pass | not run | `static` |  |
| 158 | S1: no echo -e, -ne or -en | INTERNAL | pass | not run | `static` |  |
| 159 | S2: no printf %b outside lib/core/output.sh | INTERNAL | pass | not run | `static` |  |
| 160 | S3: no literal escape sequences | INTERNAL | pass | not run | `static` |  |
| 161 | S4: no hue palette names outside lib/core/output.sh | INTERNAL | pass | not run | `static` |  |
| 162 | S5: no __gash_print with a hue name | INTERNAL | pass | not run | `static` |  |
| 163 | S6: no private fallback palettes | INTERNAL | pass | not run | `static` |  |
| 164 | S7: no width-padded %Ns formats outside lib/core | INTERNAL | pass | not run | `static` |  |
| 165 | S8: no ASCII status markers in string literals | INTERNAL | pass | not run | `static` |  |
| 166 | S9: no hand-drawn rules | INTERNAL | pass | not run | `static` |  |
| 172 | S10: no echo or printf to stderr outside lib/core | INTERNAL | pass | not run | `static` |  |
| 173 | S11: no private TTY tests outside lib/core | INTERNAL | pass | not run | `static` |  |
| 174 | S12: no read -p prompts outside lib/core | INTERNAL | pass | not run | `static` |  |

### php_detect_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 11 | reads .target-php-version (highest priority) | BLACKBOX | pass | not run | `run` |  |
| 24 | .target-php-version walks up parent directories | BLACKBOX | pass | not run | `run` |  |
| 37 | falls back to composer.json require.php | BLACKBOX | pass | not run | `run` |  |
| 49 | falls back to 8.4 when nothing explicit is found | BLACKBOX | pass | not run | `run` |  |
| 60 | matches the exact JSON field names of the legacy detect-php-version.sh contract | BLACKBOX | pass | not run | `run` |  |
| 72 | emits JSON automatically under GASH_HEADLESS=1 even without --llm | BLACKBOX | pass | not run | `run` |  |
| 83 | prints human-readable text when interactive (no --llm, no headless) | BLACKBOX | pass | not run | `run` |  |

### prompt_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | loads gash.sh in an interactive shell and builds PS1 | INTERNAL | pass | not run | `shell` |  |
| 22 | defines __gash_in_git_repo function | INTERNAL | pass | not run | `shell` |  |
| 41 | uses __GASH_PS1_* color variables instead of generic names | INTERNAL | pass | not run | `static` |  |
| 51 | __gash_in_git_repo returns success in a git repo | INTERNAL | pass | not run | `shell` |  |
| 75 | __gash_in_git_repo returns failure outside git repo | INTERNAL | pass | not run | `shell` |  |
| 98 | __gash_in_git_repo respects GASH_GIT_EXCLUDE | INTERNAL | pass | not run | `shell` |  |
| 130 | uses optimized git commands with GIT_OPTIONAL_LOCKS=0 | INTERNAL | pass | not run | `static` |  |
| 138 | git segment shows branch, dirty marker and ahead count from one git status | INTERNAL | pass | not run | `shell` |  |
| 170 | .gash_env.template documents GASH_GIT_EXCLUDE | INTERNAL | pass | not run | `static` |  |
| 179 | __gash_in_git_repo returns failure when git root is HOME | INTERNAL | pass | not run | `shell` |  |
| 213 | __gash_in_git_repo works correctly with nested repos | INTERNAL | pass | not run | `shell` |  |
| 249 | the job counter shows running and stopped jobs, not finished ones | BLACKBOX | pass | not run | `shell` |  |
| 261 | NO_COLOR / GASH_NO_COLOR drop the prompt colors at the next prompt, same layout | BLACKBOX | pass | not run | `shell` |  |

### remote_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 11 | classifies DENY paths correctly | INTERNAL | pass | not run | `unit` | remote safety tables DENY/ASK/ALLOW + delete safety |
| 25 | demotes /etc/systemd, /etc/crontab, /etc/init.d to ASK (not DENY) | INTERNAL | pass | not run | `unit` | remote safety tables DENY/ASK/ALLOW + delete safety |
| 36 | classifies ALLOW paths correctly | INTERNAL | pass | not run | `unit` | remote safety tables DENY/ASK/ALLOW + delete safety |
| 48 | normalizes paths before classifying: no slash or dot tricks, ~/.ssh asks | INTERNAL | pass | not run | `unit` | remote safety tables DENY/ASK/ALLOW + delete safety |
| 69 | resolve_target accepts a raw user@host interactively | INTERNAL | pass | not run | `unit` | remote.ResolveTarget (raw user@host refused headless, alias accepted) |
| 79 | resolve_target refuses a raw user@host under GASH_HEADLESS=1 | INTERNAL | pass | not run | `unit` | remote.ResolveTarget (raw user@host refused headless, alias accepted) |
| 91 | resolve_target accepts a registered alias under GASH_HEADLESS=1 | INTERNAL | pass | not run | `unit` | remote.ResolveTarget (raw user@host refused headless, alias accepted) |
| 106 | remote_push refuses an unregistered alias under GASH_HEADLESS=1 | BLACKBOX | pass | not run | `run` |  |
| 118 | remote_push refuses a DENY path even with --force | BLACKBOX | pass | not run | `run` |  |
| 128 | remote_sync_push refuses an ASK path under GASH_HEADLESS=1 without --force | BLACKBOX | pass | not run | `run` |  |
| 142 | a .ssh component anywhere asks; /boot/.ssh stays denied | MIXED | pass | not run | `split` | remote safety) |
| 165 | rsync --delete never treats the remote home itself as safe, however it is written | INTERNAL | pass | not run | `unit` | remote safety tables DENY/ASK/ALLOW + delete safety |
| 183 | another user's home is spelled out before the safety tables | INTERNAL | pass | not run | `unit` | remote safety tables DENY/ASK/ALLOW + delete safety |

### settings_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 8 | settings are parsed with quotes stripped and read back | INTERNAL | pass | not run | `unit` | internal/config settings parser |
| 29 | a variable already set in the environment wins over the file | INTERNAL | pass | not run | `unit` | internal/config settings parser |
| 46 | unknown settings and malformed lines warn with a line number, never any content | INTERNAL | pass | not run | `unit` | warnings 'Line N of FILE:' with ~ for HOME, never content |
| 74 | an inline # comment after a value is not part of the value | INTERNAL | pass | not run | `unit` | internal/config settings parser |
| 106 | the permission check warns only when group or other bits are set | INTERNAL | pass | not run | `unit` | config.CheckPerms warns on 640/604/660/644/710, not 600/400/700/500 |
| 125 | headless mode applies the function settings of ~/.gash_env, quietly, environment first | MOCK-DEPENDENT | pass | not run | `run*` | fake: psql stub [Y if exec] |
| 155 | gash_env_init honours GASH_ENV_FILE | BLACKBOX | pass | not run | `run` |  |
| 165 | GASH_PROMPT=0 keeps the user prompt but still installs the hook | BLACKBOX | pass | not run | `shell` |  |
| 181 | GASH_HISTORY=0 leaves the history settings alone | BLACKBOX | pass | not run | `shell` |  |
| 199 | tool replacements are off by default and on with GASH_REPLACE_TOOLS=1 | BLACKBOX | pass | not run | `shell` |  |
| 219 | safety aliases can be turned off with GASH_SAFETY_ALIASES=0 | BLACKBOX | pass | not run | `shell` |  |
| 236 | every setting documented in .gash_env.template is accepted by the parser | INTERNAL | pass | not run | `unit` | every .gash_env.template setting in the known-settings table |
| 249 | php and composer are only redirected to phpX.Y with GASH_REPLACE_TOOLS=1 | BLACKBOX | pass | not run | `shell` |  |

### ssh_auto_unlock_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 10 | parses ~/.gash_env SSH entries tolerantly | INTERNAL | pass | not run | `unit` | config SSH entries |
| 44 | prints install hint when expect is missing | MIXED | pass | not run | `split` |  |
| 71 | shows error when ssh-agent cannot be started | MIXED | pass | not run | `split` |  |
| 100 | auto-starts ssh-agent when SSH keys are configured | MIXED | pass | not run | `split` |  |
| 129 | formats expect output using gash style | MIXED | pass | not run | `split` |  |
| 155 | skips keys the agent already holds | MIXED | pass | not run | `split` |  |
| 178 | reuses the agent on the fixed socket instead of starting another | INTERNAL | pass | not run | `unit` | agent socket XDG_RUNTIME_DIR/gash-ssh-agent.sock (python socket fixture) |

### startup_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 10 | GASH_TIMING=1 records each file; nothing is recorded by default | INTERNAL | pass | not run | `shell` |  |
| 28 | gash_doctor --timing lists the files and the whole-shell time | BLACKBOX | pass | not run | `run` |  |
| 44 | a new shell runs no probe program once the cache is warm | BLACKBOX | pass | not run | `shell` |  |
| 70 | the probe cache is ignored when it does not match or holds bad values | BLACKBOX | pass | not run | `shell` |  |
| 91 | an alias named like a program on PATH is not defined; wrappers and earlier aliases stay | BLACKBOX | pass | not run | `shell` |  |
| 118 | LESSOPEN and LESSCLOSE match what eval "$(lesspipe)" sets | BLACKBOX | pass | not run | `shell` |  |
| 131 | __gash_path_of finds what type -P finds, without a subshell | INTERNAL | pass | not run | `shell` |  |
| 150 | the hook builds the prompt; the PS1 set at load runs no git status | INTERNAL | pass | not run | `shell` |  |
| 169 | the hook goes first in a PROMPT_COMMAND array or string, once | INTERNAL | pass | not run | `shell` |  |
| 194 | completion is registered for public Gash functions and help aliases only | BLACKBOX | pass | not run | `shell` |  |
| 211 | an interactive shell with Gash starts in under 400 ms (median of 5) | BLACKBOX | pass | not run | `shell` |  |

### sysinfo_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 11 | __sysinfo_section outputs === TITLE === in LLM mode | INTERNAL | pass | not run | `unit` | sysinfo LLM-mode formatter ('=== T ===', 'K=V', '--- t', empty skipped) |
| 25 | __sysinfo_kv outputs key=value in LLM mode | INTERNAL | pass | not run | `unit` | sysinfo LLM-mode formatter ('=== T ===', 'K=V', '--- t', empty skipped) |
| 39 | __sysinfo_kv skips empty values | INTERNAL | pass | not run | `unit` | sysinfo LLM-mode formatter ('=== T ===', 'K=V', '--- t', empty skipped) |
| 53 | __sysinfo_item outputs plain text in LLM mode | INTERNAL | pass | not run | `unit` | sysinfo LLM-mode formatter ('=== T ===', 'K=V', '--- t', empty skipped) |
| 67 | __sysinfo_sub outputs --- title in LLM mode | INTERNAL | pass | not run | `unit` | sysinfo LLM-mode formatter ('=== T ===', 'K=V', '--- t', empty skipped) |
| 87 | __sysinfo_release_sudo clears keepalive PID | INTERNAL | pass | not run | `unit` | sudo keepalive (Bash process management |
| 104 | __sysinfo_sudo runs command without sudo when __SYSINFO_HAS_SUDO=0 | INTERNAL | pass | not run | `unit` | sudo keepalive (Bash process management |
| 124 | __sysinfo_collect_identity outputs IDENTITY header and Kernel | INTERNAL | pass | not run | `unit` | collectors (header only |
| 140 | __sysinfo_collect_storage outputs STORAGE header | INTERNAL | pass | not run | `unit` | collectors (header only |
| 155 | __sysinfo_collect_services outputs SERVICES header | INTERNAL | pass | not run | `unit` | collectors (header only |
| 170 | __sysinfo_collect_auth outputs AUTH header | INTERNAL | pass | not run | `unit` | collectors (header only |
| 185 | __sysinfo_collect_network outputs NETWORK header | INTERNAL | pass | not run | `unit` | collectors (header only |
| 200 | __sysinfo_collect_security outputs SECURITY header | INTERNAL | pass | not run | `unit` | collectors (header only |
| 215 | __sysinfo_collect_webstack outputs WEBSTACK header | INTERNAL | pass | not run | `unit` | collectors (header only |
| 230 | __sysinfo_collect_mail outputs MAIL header | INTERNAL | pass | not run | `unit` | collectors (header only |
| 245 | __sysinfo_collect_infra outputs INFRA header | INTERNAL | pass | not run | `unit` | collectors (header only |
| 260 | __sysinfo_collect_system outputs SYSTEM header | INTERNAL | pass | not run | `unit` | collectors (header only |
| 281 | sysinfo -h shows help | BLACKBOX | pass | not run | `run` |  |
| 293 | sysinfo identity --llm outputs only IDENTITY section | MIXED | pass | not run | `split` |  |
| 314 | sysinfo badname returns error | BLACKBOX | pass | not run | `run` |  |
| 331 | si alias is defined | BLACKBOX | pass | not run | `run` |  |
| 343 | sysinfo all --llm outputs all 10 section headers | MIXED | pass | not run | `split` |  |
| 369 | sysinfo opens with an overview: a card on screen, key=value for --llm | MIXED | pass | not run | `split` |  |
| 394 | sysinfo prints a sub-header only when something follows it | MIXED | pass | not run | `split` |  |
| 439 | ai_sysinfo -h shows help | BLACKBOX | pass | not run | `run` |  |
| 453 | ai_sysinfo --raw outputs LLM data without API call | MIXED | pass | not run | `split` |  |
| 476 | sysinfo_ai alias is defined | BLACKBOX | pass | not run | `run` |  |
| 496 | __sysinfo_call_claude returns structured JSON via mock | INTERNAL | pass | not run | `unit` | ai sysinfo requests (curl fake N if net/http) |
| 515 | __sysinfo_call_gemini returns structured JSON via mock | INTERNAL | pass | not run | `unit` | ai sysinfo requests (curl fake N if net/http) |
| 540 | __sysinfo_format_ai_response formats valid JSON with hostname and findings | INTERNAL | pass | not run | `unit` | report render ('Error:/Warning:/OK:/Info:' + counts, 'Invalid JSON') |
| 560 | __sysinfo_format_ai_response fails on invalid JSON | INTERNAL | pass | not run | `unit` | report render ('Error:/Warning:/OK:/Info:' + counts, 'Invalid JSON') |
| 578 | __sysinfo_format_ai_response shows severity icons correctly | INTERNAL | pass | not run | `unit` | report render ('Error:/Warning:/OK:/Info:' + counts, 'Invalid JSON') |
| 607 | __sysinfo_deep_services outputs header and runs without error | INTERNAL | pass | not run | `unit` | collectors (header only |
| 622 | __sysinfo_deep_security outputs header and runs without error | INTERNAL | pass | not run | `unit` | collectors (header only |
| 637 | __sysinfo_deep_network outputs header and interface data | INTERNAL | pass | not run | `unit` | collectors (header only |
| 654 | __sysinfo_deep_storage outputs header and disk data | INTERNAL | pass | not run | `unit` | collectors (header only |
| 671 | __sysinfo_deep_performance outputs header and CPU/memory data | INTERNAL | pass | not run | `unit` | collectors (header only |
| 689 | __sysinfo_deep_maintenance outputs header and cron/package data | INTERNAL | pass | not run | `unit` | collectors (header only |
| 712 | __sysinfo_call_drilldown_claude returns component-based JSON via mock | INTERNAL | pass | not run | `unit` | ai sysinfo requests (curl fake N if net/http) |
| 732 | __sysinfo_call_drilldown_gemini returns component-based JSON via mock | INTERNAL | pass | not run | `unit` | ai sysinfo requests (curl fake N if net/http) |
| 758 | __sysinfo_format_drilldown_response formats components with status and issues | INTERNAL | pass | not run | `unit` | report render ('Error:/Warning:/OK:/Info:' + counts, 'Invalid JSON') |
| 781 | __sysinfo_format_drilldown_response handles empty components array | INTERNAL | pass | not run | `unit` | report render ('Error:/Warning:/OK:/Info:' + counts, 'Invalid JSON') |
| 797 | __sysinfo_format_drilldown_response fails on invalid JSON | INTERNAL | pass | not run | `unit` | report render ('Error:/Warning:/OK:/Info:' + counts, 'Invalid JSON') |
| 821 | __sysinfo_call_freetext_claude returns component-based JSON via mock | INTERNAL | pass | not run | `unit` | ai sysinfo requests (curl fake N if net/http) |
| 839 | __sysinfo_call_freetext_gemini returns component-based JSON via mock | INTERNAL | pass | not run | `unit` | ai sysinfo requests (curl fake N if net/http) |
| 863 | __sysinfo_md_initial generates markdown with hostname and sections | INTERNAL | pass | not run | `unit` | markdown report ('# System Analysis Report', '## Server:', '## Drill-down:', '## Question:') |
| 886 | __sysinfo_md_drilldown generates markdown with components | INTERNAL | pass | not run | `unit` | markdown report ('# System Analysis Report', '## Server:', '## Drill-down:', '## Question:') |
| 906 | __sysinfo_md_freetext generates markdown with question header | INTERNAL | pass | not run | `unit` | markdown report ('# System Analysis Report', '## Server:', '## Drill-down:', '## Question:') |
| 932 | save handler writes markdown file with correct content | INTERNAL | pass | not run | `unit` | markdown report ('# System Analysis Report', '## Server:', '## Drill-down:', '## Question:') |
| 964 | __sysinfo_collect_question_context detects systemd unit in question | INTERNAL | pass | not run | `unit` | collectors (header only |
| 986 | __sysinfo_collect_question_context detects file path in question | INTERNAL | pass | not run | `unit` | collectors (header only |
| 1014 | __sysinfo_collect_question_context detects port number in question | INTERNAL | pass | not run | `unit` | collectors (header only |
| 1032 | __sysinfo_collect_question_context returns empty for generic question | INTERNAL | pass | not run | `unit` | collectors (header only |
| 1049 | __sysinfo_collect_question_context detects known service keyword (apache) | INTERNAL | pass | not run | `unit` | collectors (header only |
| 1068 | __sysinfo_collect_question_context detects service name without .service suffix | INTERNAL | pass | not run | `unit` | collectors (header only |
| 1089 | __sysinfo_call_freetext_claude passes context_data to API | INTERNAL | pass | not run | `unit` | ai sysinfo requests (curl fake N if net/http) |

### toolchains_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 11 | prepends existing dirs in the order given, once, and skips missing ones | BLACKBOX | pass | not run | `shell` |  |
| 24 | --append adds at the end and ~ is expanded | BLACKBOX | pass | not run | `shell` |  |
| 37 | GASH_PATH_ADD puts the listed toolchains first | BLACKBOX | pass | not run | `shell` |  |
| 49 | the default version of nvm resolves through aliases | INTERNAL | pass | not run | `shell` |  |
| 71 | GASH_NVM=lazy puts node on PATH and loads nvm.sh on the first nvm call | BLACKBOX | pass | not run | `shell` |  |
| 93 | GASH_NVM=on sources nvm.sh at once; off (default) does nothing | BLACKBOX | pass | not run | `shell` |  |
| 109 | GASH_NVM and GASH_PATH_ADD are known ~/.gash_env settings | INTERNAL | pass | not run | `unit` | settings table has GASH_NVM, GASH_PATH_ADD |
| 125 | __gash_human_size uses IEC units | INTERNAL | pass | not run | `unit` | render.HumanSize (abc -> '0 B') |
| 136 | __gash_confirm_plan: -y passes, no terminal refuses without waiting | INTERNAL | pass | not run | `unit` | tui.ConfirmPlan (-y passes, headless refuses with 'pass -y') |
| 146 | __gash_task reports done or failed and keeps the status | INTERNAL | pass | not run | `unit` | tui.Task 'Working... done' / 'Breaking... failed', status kept |
| 157 | __gash_download saves the file and removes it when curl fails | INTERNAL | pass | not run | `unit` | download '[5 B]', 'Fetched 5 B', removes file on failure, rc 22 |

### unload_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 5 | gash_unload restores prompt, options, and removes gash symbols | BLACKBOX | pass | not run | `shell` |  |

### utils_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 7 | __gash_json_escape escapes backslashes | INTERNAL | pass | not run | `unit` | moot (encoding/json) |
| 17 | __gash_json_escape escapes double quotes | INTERNAL | pass | not run | `unit` | moot (encoding/json) |
| 27 | __gash_json_escape escapes newlines and tabs | INTERNAL | pass | not run | `unit` | moot (encoding/json) |
| 39 | __gash_json_escape handles empty string | INTERNAL | pass | not run | `unit` | moot (encoding/json) |
| 51 | __gash_no_history preserves return code on success | INTERNAL | pass | not run | `shell` |  |
| 62 | __gash_no_history preserves return code on failure | INTERNAL | pass | not run | `shell` |  |
| 75 | __gash_no_history passes arguments to wrapped command | INTERNAL | pass | not run | `shell` |  |

### wsl_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 11 | rejects unknown options before anything else | BLACKBOX | pass | not run | `run` |  |
| 19 | refuses outside WSL without calling sudo | MOCK-DEPENDENT | pass | not run | `run*` | fake: sudo (shell function override) [N: Go would exec real sudo] |
| 29 | the memory snapshot has free memory, slab and per-zone blocks | INTERNAL | pass | not run | `unit` | wsl.MemSnapshot from /proc/buddyinfo (skips without it) |
| 40 | wsl_restart asks first and refuses without a terminal; explorer gets a Windows path | MIXED | pass | not run | `split` |  |

### output_style_render_spec.sh

| Line | Test | Class | Bash 2.1.0 | Go (rc.A) | Plan | Replacement or note |
|---|---|---|---|---|---|---|
| 554 | R1: every public human-facing function has a fixture | MIXED | pass | not run | `split` |  |
| 48 | gash:overview: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 49 | gash:topic: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 50 | gash:all: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 51 | gash:unknown-topic: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 52 | gash_doctor:ok: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 53 | gash_doctor:problems: R2-R6 | MIXED | pass | not run | `split` |  |
| 54 | gash_doctor:timing: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 55 | gash_help:overview: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 56 | gash_help:list: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 57 | gash_help:search: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 58 | gash_help:search-none: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 59 | gash_help:search-usage: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 60 | gash_help:function: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 61 | gash_help:alias: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 62 | gash_help:self: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 63 | gash_help:short: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 64 | gash_help:not-found: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 65 | gash_inspiring_quote:plain: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 66 | gash_inspiring_quote:source: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 67 | gash_inspiring_quote:bad-option: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 68 | gash_uninstall:not-installed: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 69 | gash_uninstall:not-gash: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 70 | gash_uninstall:cancel: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 71 | gash_uninstall:confirm: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 73 | gash_unload:no-snapshot: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 74 | gash_upgrade:not-a-checkout: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 75 | gash_upgrade:upgrade: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 76 | gash_upgrade:dirty: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 77 | gash_username:plain: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 78 | all_colors:chart: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 79 | path_add:plain: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 80 | gash_env_init:create: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 81 | gash_env_init:no-template: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 82 | gash_db_list:empty: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 83 | gash_db_list:list: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 84 | gash_db_test:missing: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 85 | gash_db_test:unknown-driver: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 86 | gash_ai_list:empty: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 87 | gash_ai_list:list: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 88 | gash_migrate:status: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 89 | gash_migrate:menu: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 90 | gash_migrate:plain-yes: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 91 | gash_migrate:rollback-none: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 94 | files_largest:list: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 95 | files_largest:no-human: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 96 | files_largest:no-color: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 97 | files_largest:long-path: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 98 | files_largest:empty: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 99 | files_largest:unreadable: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: find() shell fn [N: make it a PATH stub] |
| 100 | files_largest:forbidden: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 101 | files_largest:not-found: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 102 | files_largest:unknown-option: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 103 | files_largest:json-error: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 104 | dirs_largest:list: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 105 | dirs_largest:depth: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 106 | dirs_largest:empty: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 107 | dirs_largest:root-blocked: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 108 | dirs_find_large:list: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 109 | dirs_find_large:with-mtime: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 110 | dirs_find_large:empty: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 111 | dirs_find_large:invalid-size: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 112 | dirs_list_empty:list: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 113 | dirs_list_empty:count: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 114 | dirs_list_empty:none: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 115 | dirs_list_empty:not-a-directory: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 116 | tree_stats:stats: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 117 | tree_stats:top-1: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 118 | tree_stats:empty-dir: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 119 | tree_stats:no-color: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 120 | tree_stats:invalid-top: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 121 | archive_extract:tar: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 122 | archive_extract:missing-arg: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 123 | archive_extract:missing-file: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 124 | archive_extract:unsupported: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 125 | archive_extract:no-zstd: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 126 | file_backup:copy: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 127 | file_backup:missing-arg: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 128 | file_backup:missing-file: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 131 | chrome_headless_update:dry-run-new: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl() shell fn [N: make it a PATH stub]; ldd() shell fn [N: make it a PATH stub]; uname() shell fn [N: make it a PATH stub]; unzip() shell fn [N: make it a PATH stub] |
| 132 | chrome_headless_update:upgrade-declined: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl() shell fn [N: make it a PATH stub]; ldd() shell fn [N: make it a PATH stub]; old() shell fn [N: make it a PATH stub]; uname() shell fn [N: make it a PATH stub]; unzip() shell fn [N: make it a PATH stub] |
| 133 | chrome_headless_update:install: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl() shell fn [N: make it a PATH stub]; ldd() shell fn [N: make it a PATH stub]; old() shell fn [N: make it a PATH stub]; uname() shell fn [N: make it a PATH stub]; unzip() shell fn [N: make it a PATH stub] |
| 134 | chrome_headless_update:up-to-date: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl() shell fn [N: make it a PATH stub]; ldd() shell fn [N: make it a PATH stub]; uname() shell fn [N: make it a PATH stub]; unzip() shell fn [N: make it a PATH stub] |
| 135 | chrome_headless_update:missing-libs: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl() shell fn [N: make it a PATH stub]; ldd() shell fn [N: make it a PATH stub]; uname() shell fn [N: make it a PATH stub]; unzip() shell fn [N: make it a PATH stub] |
| 136 | chrome_headless_update:no-network: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl() shell fn [N: make it a PATH stub]; ldd() shell fn [N: make it a PATH stub]; uname() shell fn [N: make it a PATH stub]; unzip() shell fn [N: make it a PATH stub] |
| 137 | chrome_headless_update:unknown-option: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 145 | disk_usage:table: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin) [Y if exec; N if statfs] |
| 146 | disk_usage:no-color: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin) [Y if exec; N if statfs] |
| 147 | disk_usage:no-types: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin) [Y if exec; N if statfs] |
| 148 | disk_usage:virtual-only: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df() shell fn [N: make it a PATH stub] |
| 149 | disk_usage:df-failed: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df() shell fn [N: make it a PATH stub] |
| 150 | disk_usage:bad-option: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin) [Y if exec; N if statfs] |
| 153 | history_grep:matches: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: history() shell fn [N: shell side] |
| 154 | history_grep:no-match: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: history() shell fn [N: shell side] |
| 155 | history_grep:missing-arg: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 158 | hgrep:flat: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 159 | hgrep:group: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 160 | hgrep:group-no-color: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 161 | hgrep:regex-reverse-limit: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 162 | hgrep:no-highlight: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 163 | hgrep:count: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 164 | hgrep:json: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 165 | hgrep:empty: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 166 | hgrep:pick: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: fzf() shell fn [N: make it a PATH stub] |
| 167 | hgrep:missing-pattern: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 168 | hgrep:bad-option: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 169 | hgrep:bad-limit: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 170 | hgrep:no-histfile: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 173 | ip_public:ip: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: wget() shell fn [N: make it a PATH stub]; curl (mocks/bin) [N if net/http] |
| 174 | ip_public:no-color: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: wget() shell fn [N: make it a PATH stub]; curl (mocks/bin) [N if net/http] |
| 175 | ip_public:failed: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: wget() shell fn [N: make it a PATH stub]; curl (mocks/bin) [N if net/http] |
| 176 | ip_public:no-tool: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http] |
| 179 | mkdir_cd:create: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 180 | mkdir_cd:missing-arg: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 183 | process_find:matches: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: pgrep (mocks/bin) [Y if exec; N if /proc]; ps (mocks/bin) [Y if exec; N if /proc] |
| 184 | process_find:no-match: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: pgrep (mocks/bin) [Y if exec; N if /proc]; ps (mocks/bin) [Y if exec; N if /proc] |
| 185 | process_find:missing-arg: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: pgrep (mocks/bin) [Y if exec; N if /proc]; ps (mocks/bin) [Y if exec; N if /proc] |
| 188 | process_kill:declined: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: ps() shell fn [N: make it a PATH stub]; kill (mocks/bin) [N: syscall.Kill, real PID]; pgrep (mocks/bin) [Y if exec; N if /proc] |
| 189 | process_kill:yes: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: kill() shell fn [N: make it a PATH stub]; pgrep (mocks/bin) [Y if exec; N if /proc]; ps (mocks/bin) [Y if exec; N if /proc] |
| 190 | process_kill:survivor: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: kill() shell fn [N: make it a PATH stub]; sleep() shell fn [N: make it a PATH stub]; pgrep (mocks/bin) [Y if exec; N if /proc]; ps (mocks/bin) [Y if exec; N if /proc] |
| 191 | process_kill:no-terminal: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: kill (mocks/bin) [N: syscall.Kill, real PID]; pgrep (mocks/bin) [Y if exec; N if /proc]; ps (mocks/bin) [Y if exec; N if /proc] |
| 192 | process_kill:no-match: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: kill (mocks/bin) [N: syscall.Kill, real PID]; pgrep (mocks/bin) [Y if exec; N if /proc]; ps (mocks/bin) [Y if exec; N if /proc] |
| 193 | process_kill:bad-option: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: kill (mocks/bin) [N: syscall.Kill, real PID]; pgrep (mocks/bin) [Y if exec; N if /proc]; ps (mocks/bin) [Y if exec; N if /proc] |
| 194 | process_kill:missing-arg: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: kill (mocks/bin) [N: syscall.Kill, real PID]; pgrep (mocks/bin) [Y if exec; N if /proc]; ps (mocks/bin) [Y if exec; N if /proc] |
| 197 | port_kill:declined: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: lsof() shell fn [N: make it a PATH stub]; ps() shell fn [N: make it a PATH stub]; kill (mocks/bin) [N: syscall.Kill, real PID] |
| 198 | port_kill:yes: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: kill() shell fn [N: make it a PATH stub]; lsof() shell fn [N: make it a PATH stub]; ps() shell fn [N: make it a PATH stub] |
| 199 | port_kill:nothing: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: kill (mocks/bin) [N: syscall.Kill, real PID]; lsof (mocks/bin) [Y if exec; N if /proc] |
| 200 | port_kill:invalid: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: kill (mocks/bin) [N: syscall.Kill, real PID]; lsof (mocks/bin) [Y if exec; N if /proc] |
| 201 | port_kill:no-lsof: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: kill (mocks/bin) [N: syscall.Kill, real PID]; lsof (mocks/bin) [Y if exec; N if /proc] |
| 202 | port_kill:missing-arg: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: kill (mocks/bin) [N: syscall.Kill, real PID]; lsof (mocks/bin) [Y if exec; N if /proc] |
| 205 | services_stop:dry-run: R2-R6 | MIXED | pass | not run | `split` |  |
| 206 | services_stop:declined: R2-R6 | MIXED | pass | not run | `split` |  |
| 207 | services_stop:yes: R2-R6 | MIXED | pass | not run | `split` |  |
| 208 | services_stop:none: R2-R6 | MIXED | pass | not run | `split` |  |
| 209 | services_stop:bad-option: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: systemctl (mocks/bin) [Y if exec; N if D-Bus] |
| 212 | sudo_last:command: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: sudo (mocks/bin) [Y if exec] |
| 213 | sudo_last:declined: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: fc() shell fn [N: shell side]; sudo (mocks/bin) [Y if exec] |
| 216 | wsl_restart:declined: R2-R6 | MIXED | pass | not run | `split` |  |
| 217 | wsl_restart:yes: R2-R6 | MIXED | pass | not run | `split` |  |
| 218 | wsl_restart:not-wsl: R2-R6 | MIXED | pass | not run | `split` |  |
| 219 | wsl_shutdown:yes: R2-R6 | MIXED | pass | not run | `split` |  |
| 220 | wsl_shutdown:bad-option: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 221 | wsl_explorer:open: R2-R6 | MIXED | pass | not run | `split` |  |
| 222 | wsl_explorer:missing-path: R2-R6 | MIXED | pass | not run | `split` |  |
| 223 | wsl_explorer:not-wsl: R2-R6 | MIXED | pass | not run | `split` |  |
| 224 | wsl_taskmanager:open: R2-R6 | MIXED | pass | not run | `split` |  |
| 225 | wsl_taskmanager:not-wsl: R2-R6 | MIXED | pass | not run | `split` |  |
| 226 | wsl_compact:status: R2-R6 | MIXED | pass | not run | `split` |  |
| 227 | wsl_compact:compact: R2-R6 | MIXED | pass | not run | `split` |  |
| 228 | wsl_compact:sudo-failed: R2-R6 | MIXED | pass | not run | `split` |  |
| 229 | wsl_compact:bad-option: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 232 | docker_stop_all:running: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 233 | docker_stop_all:none: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 234 | docker_stop_all:no-docker: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 235 | docker_start_all:stopped: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 236 | docker_start_all:none: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 237 | docker_prune_all:dry-run: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 238 | docker_prune_all:declined: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 239 | docker_prune_all:yes: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 240 | docker_prune_all:bad-option: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 243 | docker_compose_check:updates: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http]; docker (mocks/bin) [Y if CLI; N if Engine API] |
| 244 | docker_compose_check:errors: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http]; docker (mocks/bin) [Y if CLI; N if Engine API] |
| 245 | docker_compose_check:timeout: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http]; docker (mocks/bin) [Y if CLI; N if Engine API] |
| 246 | docker_compose_check:no-color: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http]; docker (mocks/bin) [Y if CLI; N if Engine API] |
| 247 | docker_compose_check:json: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http]; docker (mocks/bin) [Y if CLI; N if Engine API] |
| 248 | docker_compose_check:no-file: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http]; docker (mocks/bin) [Y if CLI; N if Engine API] |
| 249 | docker_compose_check:no-services: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http]; docker (mocks/bin) [Y if CLI; N if Engine API] |
| 250 | docker_compose_check:bad-option: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http]; docker (mocks/bin) [Y if CLI; N if Engine API] |
| 251 | docker_compose_upgrade:dry-run: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 252 | docker_compose_upgrade:all-pinned: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 253 | docker_compose_upgrade:declined: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 254 | docker_compose_upgrade:yes: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 255 | docker_compose_upgrade:no-file: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 256 | docker_compose_upgrade:old-n: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: docker (mocks/bin) [Y if CLI; N if Engine API] |
| 257 | docker_compose_scan:found: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 258 | docker_compose_scan:none: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 259 | docker_compose_scan:json: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 260 | llm_docker_check:json: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http]; docker (mocks/bin) [Y if CLI; N if Engine API] |
| 267 | git_list_tags:tags: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 268 | git_list_tags:no_remote: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 269 | git_add_tag:created: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 270 | git_add_tag:exists: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 271 | git_add_tag:missing_name: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 272 | git_delete_tag:deleted: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 273 | git_delete_tag:unknown: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 274 | git_dump_revisions:dumped: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 275 | git_dump_revisions:missing_file: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 276 | git_dump_revisions:missing_arg: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 277 | git_apply_patch:applied: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 278 | git_apply_patch:missing_args: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 279 | git_apply_patch:dirty: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 280 | git_repair:clean: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 281 | git_repair:dry_run: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 282 | git_repair:declined: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 283 | git_repair:repaired: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 284 | git_repair:bad_option: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 285 | git_sync:dry_run: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 286 | git_sync:pushed: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 287 | git_sync:nothing: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 288 | git_sync:detached: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 291 | remote_hosts:list: R2-R6 | MIXED | pass | not run | `split` |  |
| 292 | remote_hosts:empty: R2-R6 | MIXED | pass | not run | `split` |  |
| 293 | remote_exec:ran: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: ssh() shell fn [N: make it a PATH stub] |
| 294 | remote_exec:unreachable: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: ssh() shell fn [N: make it a PATH stub] |
| 295 | remote_exec:unknown_alias: R2-R6 | MIXED | pass | not run | `split` |  |
| 296 | remote_exec:extra_args: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 297 | remote_push:copied: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: scp() shell fn [N: make it a PATH stub] |
| 298 | remote_push:sensitive_declined: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: scp() shell fn [N: make it a PATH stub] |
| 299 | remote_push:denied: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 300 | remote_pull:copied: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: scp() shell fn [N: make it a PATH stub] |
| 301 | remote_pull:denied: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 302 | remote_pull:missing_args: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 303 | remote_sync_push:synced: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: rsync() shell fn [N: make it a PATH stub] |
| 304 | remote_sync_push:delete_declined: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: rsync() shell fn [N: make it a PATH stub] |
| 305 | remote_sync_pull:synced: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: rsync() shell fn [N: make it a PATH stub] |
| 306 | remote_sync_pull:denied: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 309 | node_detect_version:detected: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 310 | node_detect_version:not_installed: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 311 | node_detect_version:json: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 312 | node_detect_version:no_nvm: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 313 | node_detect_version:bad_option: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 314 | node_upgrade:dry_run: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: nvm() shell fn [N: shell side] |
| 315 | node_upgrade:declined: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: nvm() shell fn [N: shell side] |
| 316 | node_upgrade:upgraded: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: nvm() shell fn [N: shell side] |
| 317 | node_upgrade:up_to_date: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: nvm() shell fn [N: shell side] |
| 318 | node_upgrade:offline: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: nvm() shell fn [N: shell side] |
| 319 | node_upgrade:no_nvm: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 320 | nvm_prune_versions:dry_run: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: nvm() shell fn [N: shell side] |
| 321 | nvm_prune_versions:force: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: nvm() shell fn [N: shell side] |
| 322 | nvm_prune_versions:none: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: nvm() shell fn [N: shell side] |
| 323 | nvm_prune_versions:no_nvm: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 326 | php_detect_version:detected: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 327 | php_detect_version:json: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 328 | php_detect_version:bad_option: R2-R6 | BLACKBOX | pass | not run | `run` |  |
| 331 | gash_ssh_auto_unlock:unlocked: R2-R6 | MIXED | pass | not run | `split` |  |
| 332 | gash_ssh_auto_unlock:no_expect: R2-R6 | MIXED | pass | not run | `split` |  |
| 333 | gash_ssh_auto_unlock:no_agent: R2-R6 | MIXED | pass | not run | `split` |  |
| 338 | ai_ask:command: R2-R6 | MIXED | pass | not run | `split` |  |
| 339 | ai_ask:explanation: R2-R6 | MIXED | pass | not run | `split` |  |
| 340 | ai_ask:code: R2-R6 | MIXED | pass | not run | `split` |  |
| 341 | ai_ask:fallback: R2-R6 | MIXED | pass | not run | `split` |  |
| 342 | ai_ask:piped: R2-R6 | MIXED | pass | not run | `split` |  |
| 343 | ai_ask:empty-query: R2-R6 | MIXED | pass | not run | `split` |  |
| 344 | ai_ask:no-key: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http] |
| 345 | ai_ask:no-token: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http] |
| 346 | ai_ask:bad-provider: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http] |
| 347 | ai_ask:http-error: R2-R6 | MIXED | pass | not run | `split` |  |
| 348 | ai_ask:confirm-no: R2-R6 | MIXED | pass | not run | `split` |  |
| 349 | ai_query:command: R2-R6 | MIXED | pass | not run | `split` |  |
| 350 | ai_query:explanation: R2-R6 | MIXED | pass | not run | `split` |  |
| 351 | ai_query:code: R2-R6 | MIXED | pass | not run | `split` |  |
| 352 | ai_query:fallback: R2-R6 | MIXED | pass | not run | `split` |  |
| 353 | ai_query:piped: R2-R6 | MIXED | pass | not run | `split` |  |
| 354 | ai_query:no-query: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http] |
| 355 | ai_query:no-key: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http] |
| 356 | ai_query:no-token: R2-R6 | MIXED | pass | not run | `split` |  |
| 357 | ai_query:http-error: R2-R6 | MIXED | pass | not run | `split` |  |
| 358 | ai_query:timeout: R2-R6 | MIXED | pass | not run | `split` |  |
| 359 | ai_query:refusal: R2-R6 | MIXED | pass | not run | `split` |  |
| 360 | ai_query:confirm-yes: R2-R6 | MIXED | pass | not run | `split` |  |
| 361 | ai_query:confirm-no: R2-R6 | MIXED | pass | not run | `split` |  |
| 362 | ai_query:debug: R2-R6 | MIXED | pass | not run | `split` |  |
| 363 | ai_sysinfo:raw: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http] |
| 364 | ai_sysinfo:bad-arg: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http] |
| 365 | ai_sysinfo:no-key: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: curl (mocks/bin) [N if net/http] |
| 366 | ai_sysinfo:confirm-no: R2-R6 | MIXED | pass | not run | `split` |  |
| 367 | ai_sysinfo:report-quit: R2-R6 | MIXED | pass | not run | `split` |  |
| 368 | ai_sysinfo:report-eof: R2-R6 | MIXED | pass | not run | `split` |  |
| 369 | ai_sysinfo:drilldown: R2-R6 | MIXED | pass | not run | `split` |  |
| 370 | ai_sysinfo:question: R2-R6 | MIXED | pass | not run | `split` |  |
| 371 | ai_sysinfo:empty-question: R2-R6 | MIXED | pass | not run | `split` |  |
| 372 | ai_sysinfo:save: R2-R6 | MIXED | pass | not run | `split` |  |
| 373 | ai_sysinfo:bad-choice: R2-R6 | MIXED | pass | not run | `split` |  |
| 374 | ai_sysinfo:no-color: R2-R6 | MIXED | pass | not run | `split` |  |
| 375 | ai_sysinfo:http-error: R2-R6 | MIXED | pass | not run | `split` |  |
| 376 | ai_sysinfo:drilldown-error: R2-R6 | MIXED | pass | not run | `split` |  |
| 377 | ai_sysinfo:invalid-reply: R2-R6 | MIXED | pass | not run | `split` |  |
| 378 | sysinfo:all: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin) [Y if exec; N if statfs]; journalctl (mocks/bin) [Y if exec]; ps (mocks/bin) [Y if exec; N if /proc]; ss (mocks/bin) [Y if exec; N if /proc]; systemctl (mocks/bin) [Y if exec; N if D-Bus] |
| 379 | sysinfo:overview: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin) [Y if exec; N if statfs]; journalctl (mocks/bin) [Y if exec]; ps (mocks/bin) [Y if exec; N if /proc]; ss (mocks/bin) [Y if exec; N if /proc]; systemctl (mocks/bin) [Y if exec; N if D-Bus] |
| 380 | sysinfo:overview-no-color: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin) [Y if exec; N if statfs]; journalctl (mocks/bin) [Y if exec]; ps (mocks/bin) [Y if exec; N if /proc]; ss (mocks/bin) [Y if exec; N if /proc]; systemctl (mocks/bin) [Y if exec; N if D-Bus] |
| 381 | sysinfo:identity: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin) [Y if exec; N if statfs]; journalctl (mocks/bin) [Y if exec]; ps (mocks/bin) [Y if exec; N if /proc]; ss (mocks/bin) [Y if exec; N if /proc]; systemctl (mocks/bin) [Y if exec; N if D-Bus] |
| 382 | sysinfo:services: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin) [Y if exec; N if statfs]; journalctl (mocks/bin) [Y if exec]; ps (mocks/bin) [Y if exec; N if /proc]; ss (mocks/bin) [Y if exec; N if /proc]; systemctl (mocks/bin) [Y if exec; N if D-Bus] |
| 383 | sysinfo:network: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin) [Y if exec; N if statfs]; journalctl (mocks/bin) [Y if exec]; ps (mocks/bin) [Y if exec; N if /proc]; ss (mocks/bin) [Y if exec; N if /proc]; systemctl (mocks/bin) [Y if exec; N if D-Bus] |
| 384 | sysinfo:system: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin) [Y if exec; N if statfs]; journalctl (mocks/bin) [Y if exec]; ps (mocks/bin) [Y if exec; N if /proc]; ss (mocks/bin) [Y if exec; N if /proc]; systemctl (mocks/bin) [Y if exec; N if D-Bus] |
| 385 | sysinfo:llm: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin) [Y if exec; N if statfs]; journalctl (mocks/bin) [Y if exec]; ps (mocks/bin) [Y if exec; N if /proc]; ss (mocks/bin) [Y if exec; N if /proc]; systemctl (mocks/bin) [Y if exec; N if D-Bus] |
| 386 | sysinfo:layout: R2-R6 | MIXED | pass | not run | `split` |  |
| 387 | sysinfo:bad-arg: R2-R6 | MOCK-DEPENDENT | pass | not run | `run*` | fake: df (mocks/bin) [Y if exec; N if statfs]; journalctl (mocks/bin) [Y if exec]; ps (mocks/bin) [Y if exec; N if /proc]; ss (mocks/bin) [Y if exec; N if /proc]; systemctl (mocks/bin) [Y if exec; N if D-Bus] |
| 580 | R7: headless loading prints nothing, on a terminal or not | BLACKBOX | pass | not run | `run` |  |
