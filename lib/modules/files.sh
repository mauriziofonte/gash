#!/usr/bin/env bash

# Gash Module: File Operations
# Functions for file and directory analysis, extraction, and backup.
#
# Dependencies: core/output.sh, core/validation.sh, core/utils.sh
#
# Public functions (LONG name + SHORT alias):
#   files_largest (flf)     - List top 100 largest files
#   dirs_largest (dld)      - List top 100 largest directories
#   dirs_find_large (dfl)   - Find directories exceeding size threshold
#   dirs_list_empty (dle)   - List empty directories
#   archive_extract (axe)   - Extract various archive types
#   file_backup (fbk)       - Create timestamped file backup

# -----------------------------------------------------------------------------
# Directory Analysis
# -----------------------------------------------------------------------------

# List the top 100 largest files in a directory.
# Usage: files_largest [PATH]
# Alias: flf
files_largest() {
    needs_help "files_largest" "files_largest [PATH]" \
        "Lists the top 100 largest files in PATH (or current directory if not specified), sorted by size. Alias: flf" \
        "${1-}" && return

    local dir="${1:-.}"

    __gash_require_dir "$dir" || return 1

    find "$dir" -type f -printf '%s\t%p\t%TY-%Tm-%Td\t%TH:%TM\n' 2>/dev/null | \
        sort -nr -k1,1 | \
        awk -F'\t' '{ printf "\033[1;33m%-12s\033[0m \033[0;36m%-50s\033[0m \033[1;37m%s %s\033[0m\n", $1/1024/1024 "MB", $2, $3, $4 }' | \
        head -n 100

    return 0
}

# List the top 100 largest directories in a directory.
# Usage: dirs_largest [PATH]
# Alias: dld
dirs_largest() {
    needs_help "dirs_largest" "dirs_largest [PATH]" \
        "Lists the top 100 largest directories in PATH (or current directory if not specified), sorted by size. Alias: dld" \
        "${1-}" && return

    local dir="${1:-.}"

    __gash_require_dir "$dir" || return 1

    find "$dir" -mindepth 1 -maxdepth 1 -type d -exec du -sm -- {} + 2>/dev/null | \
        sort -nr -k1,1 | \
        awk -F'\t' '{ printf "\033[1;33m%-12s\033[0m \033[0;36m%-50s\033[0m\n", $1 "MB", $2 }' | \
        head -n 100

    return 0
}

# Find directories exceeding a specified size and list their largest file modification time.
# Usage: dirs_find_large [--size SIZE] [DIRECTORY]
# Alias: dfl
dirs_find_large() {
    __gash_require_command "numfmt" "This function requires the 'numfmt' utility, which is not available." || return 1

    needs_help "dirs_find_large" "dirs_find_large [--size SIZE] [DIRECTORY]" \
        "Finds directories larger than SIZE (default 20M) and lists their size and modification time of their largest file. Alias: dfl" \
        "${1-}" && return

    local dir="."
    local size_threshold="20M"

    while [[ "$#" -gt 0 ]]; do
        case $1 in
            --size)
                if [[ -z "${2-}" ]]; then
                    __gash_error "Missing value for --size"
                    return 1
                fi
                size_threshold="$2"
                shift
                ;;
            *)
                dir="$1"
                ;;
        esac
        shift
    done

    __gash_require_dir "$dir" || return 1

    # Convert size threshold to KiB to match `du -sk`.
    # GNU coreutils `numfmt --from=auto` returns bytes.
    local size_bytes
    size_bytes="$(numfmt --from=auto "${size_threshold}" 2>/dev/null)" || true
    if [[ -z "$size_bytes" || ! "$size_bytes" =~ ^[0-9]+$ ]]; then
        __gash_error "Invalid size threshold: ${size_threshold}"
        return 1
    fi
    local size_kb=$(( (size_bytes + 1023) / 1024 ))

    find "$dir" -type d 2>/dev/null | while read -r d; do
        local total_kb
        total_kb="$(du -sk -- "$d" 2>/dev/null | cut -f1)"
        [[ -z "$total_kb" ]] && continue

        if [ "$total_kb" -ge "$size_kb" ]; then
            local largest_file_time
            largest_file_time="$(find "$d" -type f -printf '%s\t%TY-%Tm-%Td %TH:%TM:%TS\t%p\n' 2>/dev/null | sort -nr -k1,1 | head -n1 | cut -f2)"
            local human_size
            human_size="$(du -sh -- "$d" 2>/dev/null | cut -f1)"
            printf '%s\t%s\t%s\t%s\n' "$total_kb" "$human_size" "$d" "${largest_file_time:-N/A}"
        fi
    done | \
        sort -nr -k1,1 | \
        awk -F'\t' '{ printf "\033[1;33m%-12s\033[0m \033[0;36m%-80s\033[0m \033[1;37m%-20s\033[0m\n", $2, $3, $4 }'

    return 0
}

# List all empty directories in the specified path.
# Usage: dirs_list_empty [PATH]
# Alias: dle
dirs_list_empty() {
    local dir="${1:-.}"

    __gash_require_dir "$dir" || return 1

    find "$dir" -type d -empty
}

# -----------------------------------------------------------------------------
# Archive Extraction
# -----------------------------------------------------------------------------

# Extract various archive types (case-insensitive) with optional output directory.
# Usage: archive_extract ARCHIVE_FILE [OUTPUT_DIR]
# Alias: axe
archive_extract() {
    needs_help "archive_extract" "archive_extract ARCHIVE_FILE [OUTPUT_DIR]" \
        "Extracts the ARCHIVE_FILE in the current directory or the specified OUTPUT_DIR. Alias: axe" \
        "${1-}" && return

    local archive_file="${1-}"
    local output_dir="${2:-.}"

    __gash_require_arg "$archive_file" "archive file" "archive_extract <archive_file> [output_dir]" || return 1
    __gash_require_file "$archive_file" || return 1

    if [ ! -d "$output_dir" ]; then
        mkdir -p "$output_dir" || {
            __gash_error "Failed to create output directory '$output_dir'."
            return 1
        }
    fi

    local rc=0
    shopt -s nocasematch
    case "$archive_file" in
        *.tar.bz2)   tar xvjf "$archive_file" -C "$output_dir" --no-same-owner --no-same-permissions || rc=1 ;;
        *.tar.gz)    tar xvzf "$archive_file" -C "$output_dir" --no-same-owner --no-same-permissions || rc=1 ;;
        *.bz2)       bunzip2 -c "$archive_file" > "$output_dir/$(basename "$archive_file" .bz2)" || rc=1 ;;
        *.rar)       unrar x "$archive_file" "$output_dir" || rc=1 ;;
        *.gz)        gunzip -c "$archive_file" > "$output_dir/$(basename "$archive_file" .gz)" || rc=1 ;;
        *.tar)       tar xvf "$archive_file" -C "$output_dir" --no-same-owner --no-same-permissions || rc=1 ;;
        *.tbz2)      tar xvjf "$archive_file" -C "$output_dir" --no-same-owner --no-same-permissions || rc=1 ;;
        *.tgz)       tar xvzf "$archive_file" -C "$output_dir" --no-same-owner --no-same-permissions || rc=1 ;;
        *.zip)       unzip "$archive_file" -d "$output_dir" || rc=1 ;;
        *.z)         uncompress "$archive_file" -c > "$output_dir/$(basename "$archive_file" .z)" || rc=1 ;;
        *.7z)        7z x "$archive_file" -o"$output_dir" || rc=1 ;;
        *)           __gash_error "Cannot extract '$archive_file', unsupported file type."; rc=1 ;;
    esac
    shopt -u nocasematch

    return $rc
}

# -----------------------------------------------------------------------------
# File Backup
# -----------------------------------------------------------------------------

# Create a backup of a file with a timestamp suffix.
# Usage: file_backup FILE
# Alias: fbk
file_backup() {
    local file="${1-}"

    __gash_require_arg "$file" "file" "file_backup <file>" || return 1
    __gash_require_file "$file" || return 1

    local backup="${file}_backup_$(date +%Y%m%d%H%M%S)"
    cp -v "$file" "$backup"

    if [ ! -f "$backup" ]; then
        __gash_error "Failed to create backup file."
        return 1
    fi
}

# -----------------------------------------------------------------------------
# Short Aliases
# -----------------------------------------------------------------------------
alias flf='files_largest'
alias dld='dirs_largest'
alias dfl='dirs_find_large'
alias dle='dirs_list_empty'
alias axe='archive_extract'
alias fbk='file_backup'

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

__gash_register_help "files_largest" \
    --aliases "flf" \
    --module "files" \
    --short "List top 100 largest files in a directory" \
    --see-also "dirs_largest dirs_find_large" \
    <<'HELP'
USAGE
  files_largest [PATH]

EXAMPLES
  # Find the biggest files in the current directory
  files_largest

  # Audit /var/log for space hogs
  files_largest /var/log

  # Show only the top 20
  files_largest /home | head -20
HELP

__gash_register_help "dirs_largest" \
    --aliases "dld" \
    --module "files" \
    --short "List top 100 largest directories" \
    --see-also "files_largest dirs_find_large disk_usage" \
    <<'HELP'
USAGE
  dirs_largest [PATH]

EXAMPLES
  # Find the biggest directories here
  dirs_largest

  # Find which user's home is eating disk space
  dirs_largest /home

  # Identify bloated node_modules or vendor dirs
  dirs_largest ~/projects
HELP

__gash_register_help "dirs_find_large" \
    --aliases "dfl" \
    --module "files" \
    --short "Find directories exceeding a size threshold" \
    --see-also "dirs_largest files_largest disk_usage" \
    <<'HELP'
USAGE
  dirs_find_large [--size SIZE] [DIRECTORY]

EXAMPLES
  # Find directories larger than 20MB (default) in current path
  dirs_find_large

  # Find directories larger than 1GB
  dirs_find_large --size 1G

  # Audit /var for directories over 500MB
  dirs_find_large --size 500M /var

NOTES
  SIZE format: 20M, 1G, 500K, etc.
  Output shows: size, path, and modification time of the largest file.
HELP

__gash_register_help "dirs_list_empty" \
    --aliases "dle" \
    --module "files" \
    --short "List all empty directories" \
    --see-also "files_largest" \
    <<'HELP'
USAGE
  dirs_list_empty [PATH]

EXAMPLES
  # Find empty directories in current path
  dirs_list_empty

  # Find and remove empty directories
  dirs_list_empty /var/www | xargs rmdir

  # Check a deployment directory for missing content
  dirs_list_empty /opt/app/uploads
HELP

__gash_register_help "archive_extract" \
    --aliases "axe" \
    --module "files" \
    --short "Extract archives (tar, zip, gz, bz2, rar, 7z)" \
    --see-also "file_backup" \
    <<'HELP'
USAGE
  archive_extract ARCHIVE_FILE [OUTPUT_DIR]

EXAMPLES
  # Extract a tar.gz in the current directory
  axe backup.tar.gz

  # Extract a zip to a specific folder
  axe release.zip /opt/app

  # Extract a 7z archive
  axe data.7z /tmp/extracted

SUPPORTED FORMATS
  .tar.gz, .tar.bz2, .tar, .gz, .bz2,
  .tgz, .tbz2, .zip, .rar, .7z, .z
  (case-insensitive)
HELP

__gash_register_help "file_backup" \
    --aliases "fbk" \
    --module "files" \
    --short "Create a timestamped backup of a file" \
    --see-also "archive_extract" \
    <<'HELP'
USAGE
  file_backup FILE

EXAMPLES
  # Backup a config before editing
  fbk /etc/nginx/nginx.conf
  # Creates: /etc/nginx/nginx.conf_backup_20240115143022

  # Backup-then-edit workflow
  fbk /etc/apache2/apache2.conf && vim /etc/apache2/apache2.conf

  # Backup a script before refactoring
  fbk my-deploy-script.sh
HELP

fi  # end help registration guard
