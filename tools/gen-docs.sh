#!/usr/bin/env bash
#
# Generate the function reference (docs/reference/*.md) from the help
# registry and the alias files, so the published reference can never drift
# from the code. A test fails when the committed pages are out of date.
#
# Usage: tools/gen-docs.sh [OUTPUT_DIR]   (default: docs/reference)

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="${1:-$root/docs/reference}"
mkdir -p "$out"

# A neutral environment: no user files, no colors, stable sort order.
home="$(mktemp -d)"
trap 'command rm -rf "$home"' EXIT
export HOME="$home" GASH_HEADLESS=1 LC_ALL=C GASH_DIR="$root"
unset GASH_ENV_FILE
# shellcheck source=/dev/null
source "$root/gash.sh"

# Help body to Markdown: each UPPERCASE heading becomes a level-4 heading
# and its indented block a fenced block (bash for examples).
help_to_md() {
    local body="$1" line section="" block="" lang
    flush() {
        [[ -z "$section" ]] && return 0
        while [[ "$block" == *$'\n\n' ]]; do block="${block%$'\n'}"; done
        [[ -z "${block//[[:space:]]/}" ]] && return 0
        lang=text
        [[ "$section" == EXAMPLES* ]] && lang=bash
        local rest="${section:1}"
        printf '#### %s\n\n```%s\n%s```\n\n' "${section:0:1}${rest,,}" "$lang" "$block"
    }
    while IFS= read -r line || [[ -n "$line" ]]; do
        if [[ "$line" =~ ^[A-Z][A-Z\ /-]+$ ]]; then
            flush
            section="$line" block=""
            continue
        fi
        [[ -z "$block" && -z "${line//[[:space:]]/}" ]] && continue
        block+="${line#  }"$'\n'
    done <<<"$body"
    flush
    unset -f flush
}

# One page per topic of the reference card.
index="# Function reference"$'\n\n'
index+="Generated from the help registry by \`tools/gen-docs.sh\`; do not edit by hand."$'\n'
index+="In a shell, \`gash <topic>\` shows the same card and \`gash_help <name>\` any entry."$'\n\n'
index+="| Topic | Functions | Aliases | What |"$'\n'"|---|---:|---:|---|"$'\n'

for entry in "${__GASH_CARD_TOPICS[@]}"; do
    IFS='|' read -r topic modules alias_files desc <<<"$entry"
    fns="$(__gash_card_functions "$modules")"
    aliases=""
    for f in $alias_files; do aliases+="$(__gash_card_aliases "$f")"$'\n'; done
    aliases="$(grep -v '^$' <<<"$aliases" || true)"
    nf=0 na=0
    [[ -n "$fns" ]] && nf=$(grep -c . <<<"$fns")
    [[ -n "$aliases" ]] && na=$(grep -c . <<<"$aliases")
    index+="| [$topic]($topic.md) | $nf | $na | $desc |"$'\n'

    {
        printf '# %s\n\n%s.\n\n' "$topic" "$desc"
        printf '_Generated from the help registry by `tools/gen-docs.sh`; do not edit by hand._\n\n'
        if [[ -n "$fns" ]]; then
            printf '| Function | Alias | Description |\n|---|---|---|\n'
            while IFS= read -r fn; do
                al="${__GASH_HELP_ALIASES[$fn]-}"
                [[ -n "$al" ]] && al="\`${al// /\`, \`}\`"
                printf '| [`%s`](#%s) | %s | %s |\n' "$fn" "${fn//_/-}" "$al" "${__GASH_HELP_SHORT[$fn]-}"
            done <<<"$fns"
            printf '\n'
            while IFS= read -r fn; do
                printf '## %s\n\n' "$fn"
                [[ -n "${__GASH_HELP_SHORT[$fn]-}" ]] && printf '%s\n\n' "${__GASH_HELP_SHORT[$fn]}"
                [[ -n "${__GASH_HELP_ALIASES[$fn]-}" ]] && printf 'Alias: `%s`\n\n' "${__GASH_HELP_ALIASES[$fn]// /\`, \`}"
                help_to_md "${__GASH_HELP_REGISTRY[$fn]}"
                if [[ -n "${__GASH_HELP_SEE_ALSO[$fn]-}" ]]; then
                    see=""
                    for s in ${__GASH_HELP_SEE_ALSO[$fn]}; do see+="\`$s\`, "; done
                    printf 'See also: %s\n\n' "${see%, }"
                fi
            done <<<"$fns"
        fi
        if [[ -n "$aliases" ]]; then
            printf '## Aliases\n\n| Alias | Runs |\n|---|---|\n'
            while IFS=$'\t' read -r name value; do
                value="${value//|/\\|}"
                printf '| `%s` | `%s` |\n' "$name" "$value"
            done <<<"$aliases"
            printf '\n'
        fi
    } >"$out/$topic.md"
done

printf '%s' "$index" >"$out/README.md"
