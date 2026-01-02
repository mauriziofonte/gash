#!/usr/bin/env bash

#########################################
#          Utility Functions            #
#########################################

# Print help message for a function if requested.
function needs_help() {
    local program="${1-}"
    local usage="${2-}"
    local help="${3-}"
    local user_input="${4-}"

    if [[ "$user_input" == "--help" || "$user_input" == "-h" ]]; then
        echo -e "\033[38;5;214m${program}\033[0m"
        echo -e "\033[1;97mUsage:\033[0m \033[1;96m${usage}\033[0m"
        echo -e "\033[1;97m${help}\033[0m"
        return 0
    fi

    return 1
}

function needs_confirm_prompt() {
        local prompt="${1-}"
        command printf '%b' "$prompt \e[1;37m(y/N):\033[0m "
    read -r REPLY < /dev/tty
    if [[ "$REPLY" =~ ^[Yy]$ ]]; then
        return 0
    fi
    return 1
}

function print_error() {
    local msg="${1-}"
    echo -e "\033[1;31mError:\033[0m \e[1;37m$msg\033[0m"
}

function __gash_tty_width() {
    # Best-effort terminal width. Avoids hard failures under strict mode.
    local w=""

    if [[ "${COLUMNS-}" =~ ^[0-9]+$ ]] && (( COLUMNS > 0 )); then
        printf '%s' "$COLUMNS"
        return 0
    fi

    if command -v tput >/dev/null 2>&1; then
        w="$(tput cols 2>/dev/null || true)"
        if [[ "$w" =~ ^[0-9]+$ ]] && (( w > 0 )); then
            printf '%s' "$w"
            return 0
        fi
    fi

    if command -v stty >/dev/null 2>&1; then
        w="$(stty size 2>/dev/null | awk '{print $2}' || true)"
        if [[ "$w" =~ ^[0-9]+$ ]] && (( w > 0 )); then
            printf '%s' "$w"
            return 0
        fi
    fi

    printf '80'
}

function __gash_trim_ws() {
        local s="${1-}"
    # trim leading spaces/tabs
    s="${s#"${s%%[!$' \t']*}"}"
    # trim trailing spaces/tabs
    s="${s%"${s##*[!$' \t']}"}"
    printf '%s' "$s"
}

function __gash_expand_tilde_path() {
    local p="${1-}"
    if [[ "$p" == "~/"* ]]; then
        printf '%s' "$HOME/${p:2}"
        return 0
    fi
    if [[ "$p" == "~" ]]; then
        printf '%s' "$HOME"
        return 0
    fi
    printf '%s' "$p"
}

function __gash_os_expect_install_hint() {
    # Prints a best-effort install command for `expect` based on the OS.
    # No side effects.
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

function __gash_format_expect_output() {
    # Read from stdin and re-print with Gash-style formatting.
    local line
    while IFS= read -r line; do
        line="${line%$'\r'}"
        [[ -z "$line" ]] && continue

        # Filter common noise from expect
        if [[ "$line" == spawn\ ssh-add* ]]; then
            continue
        fi

        if [[ "$line" == Agent\ pid* ]]; then
            echo -e "\033[0;36mSSH:\033[0m \033[1;37m$line\033[0m"
        elif [[ "$line" == Identity\ added:* ]]; then
            echo -e "\033[0;36mSSH:\033[0m \033[1;32m$line\033[0m"
        elif [[ "$line" == The\ agent\ has\ no\ identities.* ]]; then
            echo -e "\033[0;36mSSH:\033[0m \033[1;33m$line\033[0m"
        elif [[ "$line" == Bad\ passphrase* ]]; then
            echo -e "\033[0;36mSSH:\033[0m \033[1;31m$line\033[0m"
        elif [[ "$line" == Could\ not\ open\ a\ connection\ to\ your\ authentication\ agent.* ]]; then
            echo -e "\033[0;36mSSH:\033[0m \033[1;31m$line\033[0m"
        else
            echo -e "\033[0;36mSSH:\033[0m \033[1;37m$line\033[0m"
        fi
    done
}

function __gash_read_ssh_credentials_file() {
    # Reads a credentials file and prints TAB-separated pairs: <keyfile>\t<password>
    # Returns 0 even if some lines are invalid; returns 1 only if the file is unreadable.
    local credentials_file="${1-}"

    if [[ ! -r "$credentials_file" ]]; then
        return 1
    fi

    local line
    while IFS= read -r line || [[ -n "$line" ]]; do
        line="${line%$'\r'}"
        # tolerate leading/trailing whitespace
        local trimmed
        trimmed="$(__gash_trim_ws "$line")"

        # allow comments / empty lines
        [[ -z "$trimmed" ]] && continue
        [[ "$trimmed" == \#* ]] && continue

        if [[ "$trimmed" != *:* ]]; then
            echo -e "__GASH_PARSE_ERROR__\tInvalid line (missing ':'): $trimmed"
            continue
        fi

        local key_part="${trimmed%%:*}"
        local pass_part="${trimmed#*:}"

        key_part="$(__gash_trim_ws "$key_part")"

        # Be tolerant of `:\tPASSWORD` formatting: strip leading spaces/tabs after ':'
        pass_part="${pass_part#"${pass_part%%[!$' \t']*}"}"

        if [[ -z "$key_part" || -z "$pass_part" ]]; then
            echo -e "__GASH_PARSE_ERROR__\tInvalid line (empty key or password): $trimmed"
            continue
        fi

        key_part="$(__gash_expand_tilde_path "$key_part")"

        if [[ ! -f "$key_part" ]]; then
            echo -e "__GASH_PARSE_ERROR__\tKey file not found: $key_part"
            continue
        fi

        printf '%s\t%s\n' "$key_part" "$pass_part"
    done < "$credentials_file"
}

gash_ssh_auto_unlock() {
    # Auto-add SSH keys listed in ~/.gash_ssh_credentials using `expect`.
    # This is designed to be safe when sourced at shell startup.
    # Run once per shell session
    if [[ -n "${GASH_SSH_AUTOUNLOCK_RAN:-}" ]]; then
        return 0
    fi
    export GASH_SSH_AUTOUNLOCK_RAN=1

    local credentials_file="${GASH_SSH_CREDENTIALS_FILE:-$HOME/.gash_ssh_credentials}"
    if [[ ! -f "$credentials_file" ]]; then
        return 0
    fi

    if ! command -v ssh-add >/dev/null 2>&1; then
        print_error "ssh-add is not available; cannot auto-unlock SSH keys."
        return 0
    fi

    # Detect ssh-agent not running early (avoid expect/ssh-add prompts)
    local ssh_add_list
    ssh_add_list="$(ssh-add -l 2>&1 || true)"
    if [[ "$ssh_add_list" == *"Could not open a connection to your authentication agent"* ]]; then
        print_error "ssh-agent is not running. Start it (e.g. eval \"\$(ssh-agent -s)\"), then retry."
        return 0
    fi

    if ! command -v expect >/dev/null 2>&1; then
        echo -e "\033[1;33mWarning:\033[0m \033[1;37m'expect' is not installed.\033[0m"
        echo -e "\033[1;37mInstall it to enable SSH key auto-unlock:\033[0m \033[1;36m$(__gash_os_expect_install_hint)\033[0m"
        return 0
    fi

    local parsed
    parsed="$(__gash_read_ssh_credentials_file "$credentials_file")"
    if [[ -z "$parsed" ]]; then
        echo -e "\033[1;33mWarning:\033[0m \033[1;37mNo valid SSH credentials found in $credentials_file\033[0m"
        return 0
    fi

    local -a args=()
    local had_errors=0
    local IFS=$'\n'
    local row
    for row in $parsed; do
        if [[ "$row" == __GASH_PARSE_ERROR__* ]]; then
            had_errors=1
            print_error "${row#*\t}"
            continue
        fi

        local keyfile="${row%%$'\t'*}"
        local password="${row#*$'\t'}"

        # Encode values to safely pass special characters through argv
        local key_b64 pass_b64
        key_b64="$(printf '%s' "$keyfile" | base64 | tr -d '\n')"
        pass_b64="$(printf '%s' "$password" | base64 | tr -d '\n')"
        args+=("$key_b64" "$pass_b64")
    done

    if [[ ${#args[@]} -eq 0 ]]; then
        return 0
    fi

    if [[ $had_errors -ne 0 ]]; then
        echo -e "\033[1;33mWarning:\033[0m \033[1;37mSome SSH credentials lines were skipped due to errors.\033[0m"
    fi

    local expect_script
    expect_script="$(mktemp -t gash-ssh-expect.XXXXXX)"

    cat > "$expect_script" <<'EOF'
#!/usr/bin/expect -f

proc b64decode {s} {
  return [binary decode base64 $s]
}

set timeout 20

if {([llength $argv] % 2) != 0} {
  puts "Invalid arguments (expected pairs of key/password)"
  exit 2
}

for {set i 0} {$i < [llength $argv]} {incr i 2} {
  set key_b64 [lindex $argv $i]
  set pass_b64 [lindex $argv [expr {$i+1}]]
  set keyfile [b64decode $key_b64]
  set pass [b64decode $pass_b64]

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

    local expect_output
    expect_output="$(expect -f "$expect_script" -- "${args[@]}" 2>&1)"
    rm -f "$expect_script" >/dev/null 2>&1

    echo "$expect_output" | __gash_format_expect_output

    return 0
}

# List the top 100 largest files in a directory.
largest_files() {
    needs_help "largest_files" "largest_files [PATH]" "Lists the top 100 largest files in PATH (or current directory if not specified), sorted by size." "${1-}" && return

    local dir="${1:-.}"

    # fail if the directory does not exist
    if [ ! -d "$dir" ]; then
        print_error "Directory '$dir' does not exist."
        return 1
    fi

    find "$dir" -type f -printf '%s\t%p\t%TY-%Tm-%Td\t%TH:%TM\n' 2>/dev/null | \
    sort -nr -k1,1 | \
    awk -F'\t' '{ printf "\033[1;33m%-12s\033[0m \033[0;36m%-50s\033[0m \033[1;37m%s %s\033[0m\n", $1/1024/1024 "MB", $2, $3, $4 }' | \
    head -n 100

    return 0
}

# List the top 100 largest directories in a directory.
largest_dirs() {
    needs_help "largest_dirs" "largest_dirs [PATH]" "Lists the top 100 largest directories in PATH (or current directory if not specified), sorted by size." "${1-}" && return

    local dir="${1:-.}"
    
    # fail if the directory does not exist
    if [ ! -d "$dir" ]; then
        print_error "Directory '$dir' does not exist."
        return 1
    fi

    find "$dir" -mindepth 1 -maxdepth 1 -type d -exec du -sm -- {} + 2>/dev/null | \
    sort -nr -k1,1 | \
    awk -F'\t' '{ printf "\033[1;33m%-12s\033[0m \033[0;36m%-50s\033[0m\n", $1 "MB", $2 }' | \
    head -n 100

    return 0
}

# Find directories exceeding a specified size and list their largest file modification time.
find_large_dirs() {
    # fail if we lack support for numfmt
    if ! command -v numfmt >/dev/null 2>&1; then
        print_error "This function requires the 'numfmt' utility, which is not available."
        return 1
    fi

    needs_help "find_large_dirs" "find_large_dirs [--size SIZE] [DIRECTORY]" "Finds directories larger than SIZE (default 20M) and lists their size and modification time of their largest file." "${1-}" && return

    local dir="."
    local size_threshold="20M"

    while [[ "$#" -gt 0 ]]; do
        case $1 in
            --size)
                if [[ -z "${2-}" ]]; then
                    print_error "Missing value for --size"
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

    # fail if the directory does not exist
    if [ ! -d "$dir" ]; then
        print_error "Directory '$dir' does not exist."
        return 1
    fi

    # Convert size threshold to KiB to match `du -sk`.
    # GNU coreutils `numfmt --from=auto` returns bytes.
    local size_bytes
    size_bytes="$(numfmt --from=auto "${size_threshold}" 2>/dev/null)" || true
    if [[ -z "$size_bytes" || ! "$size_bytes" =~ ^[0-9]+$ ]]; then
        print_error "Invalid size threshold: ${size_threshold}"
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

# Display disk usage for specific filesystem types.
disk_usage_fs() {
    needs_help "disk_usage_fs" "disk_usage_fs" "Displays disk usage for specific filesystem types, formatted for easy reading." "${1-}" && return

    df -hT | awk '
    BEGIN {printf "%-20s %-8s %-8s %-8s %-8s %-6s %-20s\n", "Filesystem", "Type", "Size", "Used", "Avail", "Use%", "Mountpoint"}
    $2 ~ /(ext[2-4]|xfs|btrfs|zfs|f2fs|fat|vfat|ntfs)/ {
        printf "\033[1;33m%-20s\033[0m \033[0;36m%-8s\033[0m \033[1;37m%-8s\033[0m \033[1;37m%-8s\033[0m \033[1;37m%-8s\033[0m \033[38;5;214m%-6s\033[0m %-20s\n", $1, $2, $3, $4, $5, $6, $7
    }'

    return 0
}

# Search command history with colored output, removing duplicates, and avoiding self-call
hgrep() {
    needs_help "hgrep" "hgrep PATTERN" "Searches the bash history for commands matching PATTERN." "${1-}" && return

    if [[ $# -eq 0 ]]; then
        print_error "Please specify a pattern. Usage: hgrep <pattern>"
        return 1
    fi

    # Extract the relevant parts (ignoring history line numbers), remove duplicates, and avoid self-call
    history | grep -i -- "$@" | grep -v "hgrep" | \
    awk '{ $1=""; seen[$0]++; if (seen[$0]==1) print $0 }' | \
    awk '{ 
        printf "\033[1;32m%-5s\033[0m \033[0;36m%-20s\033[0m \033[1;37m%s\033[0m\n", NR, $1" "$2, substr($0, index($0,$3)); 
    }'

    return 0
}

# Dump all revisions of a file in a GIT repo into multiple separate files
git_dump_revisions() {
    needs_help "git_dump_revisions" "git_dump_revisions FILENAME" "Dump all revisions of a file in a GIT repo into multiple separate files. Example: git_dump_revisions path/to/somefile.txt" "${1-}" && return

    local file="${1-}"

    if [[ -z "$file" ]]; then
        print_error "Please specify a filename. Usage: git_dump_revisions <filename>"
        return 1
    fi

    # Check if we're in a git repository
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        print_error "Not in a git repository."
        return 1
    fi

    # Check if the file exists in the repository
    if [ ! -f "$file" ]; then
        print_error "File '$file' does not exist in the repository."
        return 1
    fi
    local index=1

    for commit in $(git log --pretty=format:%h "$file"); do
        local padindex=$(printf %03d "$index")
        local out="$file.$padindex.$commit"
        local log="$out.logmsg"

        echo -e "\033[0;36mInfo:\033[0m \033[1;37mSaving version $index to file $out for commit $commit\033[0m"

        # Save commit log message in a separate log file
        echo "*******************************************************" > "$log"
        git log -1 --pretty=format:"%s%nAuthored by %an at %ai%n%n%b%n" "$commit" >> "$log"
        echo "*******************************************************" >> "$log"

        # Save the actual file content for the commit
        git show "$commit:./$file" > "$out"

        index=$((index + 1))
    done
}

# Function to create and apply a patch from a feature branch to the main branch
git_apply_feature_patch() {
    needs_help "git_apply_feature_patch" "git_apply_feature_patch MAIN_BRANCH FEATURE_BRANCH COMMIT_HASH" "Create and apply a patch from a feature branch to the main branch. Example: git_apply_feature_patch main old-feat 123456" "${1-}" && return

    # Ensure we're in a Git repository
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        print_error "Not in a Git repository."
        return 1
    fi

    # Define input arguments with descriptive names
    local main_branch="${1-}"     # Example: "main"
    local feature_branch="${2-}"  # Example: "old-feat"
    local commit_hash="${3-}"     # Example: the commit hash from the main branch before feature branch was created

    # Check if input arguments are provided
    if [ -z "$main_branch" ] || [ -z "$feature_branch" ] || [ -z "$commit_hash" ]; then
        print_error "Missing arguments."
        needs_help "git_apply_feature_patch" "git_apply_feature_patch MAIN_BRANCH FEATURE_BRANCH COMMIT_HASH" "Create and apply a patch from a feature branch to the main branch. Example: git_apply_feature_patch main old-feat 123456" "--help"
        return 1
    fi

    # Check if the main branch exists
    if ! git show-ref --verify --quiet "refs/heads/$main_branch"; then
        print_error "Main branch '$main_branch' does not exist."
        return 1
    fi

    # Check if the feature branch exists
    if ! git show-ref --verify --quiet "refs/heads/$feature_branch"; then
        print_error "Feature branch '$feature_branch' does not exist."
        return 1
    fi

    # Check if the commit hash is valid
    if ! git cat-file -e "$commit_hash" 2>/dev/null; then
        print_error "Commit hash '$commit_hash' is not valid."
        return 1
    fi

    # Start patching process
    echo -e "\033[0;36mInfo:\033[0m \033[1;37m1. Checking out to main branch '$main_branch'\033[0m..."
    git checkout "$main_branch" && git pull origin "$main_branch"

    if [ $? -ne 0 ]; then
        print_error "Failed to checkout or pull the main branch."
        return 1
    fi

    echo -e "\033[0;36mInfo:\033[0m \033[1;37m2. Checking out to feature branch '$feature_branch'\033[0m..."
    git checkout "$feature_branch"

    if [ $? -ne 0 ]; then
        print_error "Failed to checkout the feature branch."
        return 1
    fi

    echo -e "\033[0;36mInfo:\033[0m \033[1;37m3. Generating patch from '$feature_branch' since commit '$commit_hash'\033[0m..."
    patch_file="${feature_branch}.patch"
    git diff-index "$commit_hash" --binary > "$patch_file"

    if [ $? -ne 0 ]; then
        print_error "Failed to create patch file."
        return 1
    fi

    echo -e "\033[0;36mInfo:\033[0m \033[1;37m4. Applying patch to '$main_branch'\033[0m..."
    git checkout "$main_branch" && git apply --3way "$patch_file"

    if [ $? -ne 0 ]; then
        print_error "Failed to apply the patch."
        return 1
    fi

    echo -e "\033[1;32mOK:\033[0m \033[1;37mPatch applied successfully from '$feature_branch' to '$main_branch'.\033[0m"
}

# List all tags (local or remote)
gtags() {
    needs_help "gtags" "gtags" "Lists all local and remote tags." "${1-}" && return

    # Check if we're in a git repository
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        print_error "Not in a git repository."
        return 1
    fi

    if ! git remote get-url origin >/dev/null 2>&1; then
        print_error "Remote 'origin' is not configured."
        return 1
    fi

    echo -e "\033[1;34mLocal tags:\033[0m"
    git tag -l
    echo -e "\033[1;34mRemote tags:\033[0m"
    git ls-remote --tags origin | awk '{print $2}' | sed 's|refs/tags/||'

    return 0
}

# Create an annotated tag and push it to the remote
gadd_tag() {
    needs_help "gadd_tag" "gadd_tag <tag_name> \"<tag_message>\"" "Creates an annotated tag and pushes it to remote." "${1-}" && return

    # Check if we're in a git repository
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        print_error "Not in a git repository."
        return 1
    fi

    if ! git remote get-url origin >/dev/null 2>&1; then
        print_error "Remote 'origin' is not configured."
        return 1
    fi

    if [[ -z "${1-}" ]]; then
        print_error "Please specify a tag name."
        return 1
    fi

    local tag_name="${1-}"
    local tag_message="${2:-"Release $tag_name"}"

    # Check if the tag already exists locally or remotely
    if git rev-parse "$tag_name" >/dev/null 2>&1 || git ls-remote --tags origin | grep -q "refs/tags/$tag_name"; then
        print_error "Tag '$tag_name' already exists."
        return 1
    fi

    # Create the tag
    git tag -a "$tag_name" -m "$tag_message"
    echo -e "\033[1;32mTag '$tag_name' created.\033[0m"

    # Push the tag to remote
    git push origin "$tag_name"
    echo -e "\033[1;32mTag '$tag_name' pushed to remote.\033[0m"

    return 0
}

# Delete a tag locally and on the remote
gdel_tag() {
    needs_help "gdel_tag" "gdel_tag <tag_name>" "Deletes a tag both locally and on remote." "${1-}" && return

    # Check if we're in a git repository
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        print_error "Not in a git repository."
        return 1
    fi

    if ! git remote get-url origin >/dev/null 2>&1; then
        print_error "Remote 'origin' is not configured."
        return 1
    fi

    if [[ -z "${1-}" ]]; then
        print_error "Please specify a tag name to delete."
        echo -e "\033[1;32mUsage: gdel_tag <tag_name>\033[0m"
        return 1
    fi

    local tag_name="${1-}"

    # Check if the tag exists locally or remotely
    if ! git rev-parse "$tag_name" >/dev/null 2>&1 && ! git ls-remote --tags origin | grep -q "refs/tags/$tag_name"; then
        print_error "Tag '$tag_name' does not exist."
        return 1
    fi

    # Delete the local tag if it exists
    if git rev-parse "$tag_name" >/dev/null 2>&1; then
        git tag -d "$tag_name" && echo -e "\033[1;32mLocal tag '$tag_name' deleted.\033[0m"
    fi

    # Delete the tag on the remote if it exists
    if git ls-remote --tags origin | grep -q "refs/tags/$tag_name"; then
        git push origin --delete "$tag_name" && echo -e "\033[1;32mRemote tag '$tag_name' deleted.\033[0m"
    fi

    return 0
}

# Stop all running Docker containers
docker_stop_all() {
    # do we have docker installed?
    if ! command -v docker >/dev/null 2>&1; then
        print_error "Docker is not installed."
        return 1
    fi

    echo -e "\033[0;36mInfo:\033[0m \033[1;37mStopping Docker containers...\033[0m"
    docker stop $(docker ps -aq)
}

# Start all stopped Docker containers
docker_start_all() {
    # do we have docker installed?
    if ! command -v docker >/dev/null 2>&1; then
        print_error "Docker is not installed."
        return 1
    fi

    echo -e "\033[0;36mInfo:\033[0m \033[1;37mStarting Docker containers...\033[0m"
    docker start $(docker ps -aq)
}

# Remove all Docker containers, images, volumes, and networks and clean up the environment
docker_prune_all() {
    # do we have docker installed?
    if ! command -v docker >/dev/null 2>&1; then
        print_error "Docker is not installed."
        return 1
    fi

    if ! needs_confirm_prompt "\033[1;33mWarning:\033[0m \033[1;37mRemove all Docker containers, images, volumes, and networks?\033[0m"; then
        return 0
    fi

    echo -e "\033[0;36mInfo:\033[0m \033[1;37m1. Stopping Docker containers...\033[0m"
    docker stop $(docker ps -aq)

    echo -e "\033[0;36mInfo:\033[0m \033[1;37m2. Removing Docker containers...\033[0m"
    docker rm $(docker ps -aq)

    echo -e "\033[0;36mInfo:\033[0m \033[1;37m3. Removing Docker images...\033[0m"
    docker rmi $(docker images -q)

    echo -e "\033[0;36mInfo:\033[0m \033[1;37m4. Removing Docker volumes...\033[0m"
    docker volume rm $(docker volume ls -q)

    echo -e "\033[0;36mInfo:\033[0m \033[1;37m5. Removing Docker networks...\033[0m"
    docker network rm $(docker network ls -q | grep -v "bridge\|host\|none")

    echo -e "\033[0;36mInfo:\033[0m \033[1;37m6. Cleaning Docker environment...\033[0m"
    docker system prune --volumes -a --force
}

# Prints all available colors with ANSI escape codes.
function all_colors() {
    for x in 0 1 4 5 7 8; do
        for i in seq 30 37; do
            for a in seq 40 47; do
                echo -ne "\e[$x;$i;$a""m\\\e[$x;$i;$a""m\e[0;37;40m "
            done
            echo
        done
    done
    echo ""
}

# Add 'please' command to re-run the previous command with sudo
please() {
    if [ "$EUID" -ne 0 ]; then
        if [ "$#" -eq 0 ]; then
            sudo $(fc -ln -1)
        else
            sudo "$@"
        fi
    else
        if [ "$#" -eq 0 ]; then
            $(fc -ln -1)
        else
            "$@"
        fi
    fi
}

# Create a function to make directory and cd into it
mkcd() {
    needs_help "mkcd" "mkcd DIRECTORY" "Creates a new directory and changes into it." "${1-}" && return

    local dir="${1-}"
    if [[ -z "$dir" ]]; then
        print_error "Please specify a directory. Usage: mkcd <directory>"
        return 1
    fi

    mkdir -p "$dir" && cd "$dir"
}

# Function to extract various archive types (case-insensitive) with optional output directory.
extract() {
    needs_help "extract" "extract ARCHIVE_FILE [OUTPUT_DIR]" "Extracts the ARCHIVE_FILE in the current directory or the specified OUTPUT_DIR." "${1-}" && return

    local archive_file="${1-}"
    local output_dir="${2:-.}"

    if [[ -z "$archive_file" ]]; then
        print_error "Please specify an archive file. Usage: extract <archive_file> [output_dir]"
        return 1
    fi

    if [ ! -f "$archive_file" ]; then
        print_error "'$archive_file' does not exist."
        return 1
    fi

    if [ ! -d "$output_dir" ]; then
        mkdir -p "$output_dir"
        if [ $? -ne 0 ]; then
            print_error "Failed to create output directory '$output_dir'."
            return 1
        fi
    fi

    shopt -s nocasematch
    case "$archive_file" in
        *.tar.bz2)   tar xvjf "$archive_file" -C "$output_dir"    ;;
        *.tar.gz)    tar xvzf "$archive_file" -C "$output_dir"    ;;
        *.bz2)       bunzip2 -c "$archive_file" > "$output_dir/$(basename "$archive_file" .bz2)"    ;;
        *.rar)       unrar x "$archive_file" "$output_dir"        ;;
        *.gz)        gunzip -c "$archive_file" > "$output_dir/$(basename "$archive_file" .gz)"      ;;
        *.tar)       tar xvf "$archive_file" -C "$output_dir"     ;;
        *.tbz2)      tar xvjf "$archive_file" -C "$output_dir"    ;;
        *.tgz)       tar xvzf "$archive_file" -C "$output_dir"    ;;
        *.zip)       unzip "$archive_file" -d "$output_dir"       ;;
        *.z)         uncompress "$archive_file" -c > "$output_dir/$(basename "$archive_file" .z)"   ;;
        *.7z)        7z x "$archive_file" -o"$output_dir"         ;;
        *)           echo "Error: Cannot extract '$archive_file', unsupported file type." ;;
    esac
    shopt -u nocasematch
}

# Create a backup of a file with a timestamp suffix
backup_file() {
    local file="${1-}"

    if [[ -z "$file" ]]; then
        print_error "Please specify a file. Usage: backup_file <file>"
        return 1
    fi

    if [ ! -f "$file" ]; then
        print_error "File '$file' does not exist."
        return 1
    fi

    local backup="${file}_backup_$(date +%Y%m%d%H%M%S)"
    cp -v "$file" "$backup"

    if [ ! -f "$backup" ]; then
        print_error "Failed to create backup file."
        return 1
    fi
}

# List all empty directories in the specified path (default: current directory)
list_empty_dirs() {
    local dir="${1:-.}"

    if [ ! -d "$dir" ]; then
        print_error "Directory '$dir' does not exist."
        return 1
    fi

    find "$dir" -type d -empty
}

# Get your public IP address
myip() {
    # check for wget / curl and use one of them to fetch the IP from https://ipinfo.io/ip
    local ip
    if command -v wget >/dev/null 2>&1; then
        ip=$(wget -qO- https://ipinfo.io/ip)
        echo -e "\033[1;37mPublic IP:\033[0m \033[0;36m$ip\033[0m"
    elif command -v curl >/dev/null 2>&1; then
        ip=$(curl -s https://ipinfo.io/ip)
        echo -e "\033[1;37mPublic IP:\033[0m \033[0;36m$ip\033[0m"
    else
        print_error "This function requires either 'wget' or 'curl' to be installed."
        return 1
    fi
}

# Search for a process by name
psgrep() {
    needs_help "psgrep" "psgrep PROCESS_NAME" "Search for a process by name." "${1-}" && return

    local process_name="${1-}"
    if [[ -z "$process_name" ]]; then
        print_error "Please specify a process name. Usage: psgrep <process_name>"
        return 1
    fi

    local result
    result=$(ps aux | grep -i -- "$process_name" | grep -v grep)

    if [ -n "$result" ]; then
        echo -e "\033[0;36mInfo:\033[0m \033[1;37mProcesses matching $process_name:\033[0m"
        echo "$result" | awk '{ printf "   \033[1;33m%-8s\033[0m \033[0;36m%-12s\033[0m %-4s \033[1;37m%-40s\033[0m\n", $2, $1, $3, $11 }'
    else
        print_error "No process found with name '$process_name'."
    fi
}

# Kill all processes by name
pskill() {
    needs_help "pskill" "pskill PROCESS_NAME" "Kill all processes by name." "${1-}" && return

    local process_name="${1-}"
    if [[ -z "$process_name" ]]; then
        print_error "Please specify a process name. Usage: pskill <process_name>"
        return 1
    fi

    local pids
    pids=$(ps aux | grep -i -- "$process_name" | grep -v grep | awk '{print $2}')
    
    if [ -n "$pids" ]; then
        for pid in $pids; do
            kill -9 "$pid"
            echo -e "\033[0;36mInfo:\033[0m \033[1;37mProcess with PID $pid killed.\033[0m"
        done
    else
        print_error "No process found with name '$process_name'."
    fi
}

# Kill all processes by port
portkill() {
    local port="${1-}"

    if [[ -z "$port" ]]; then
        print_error "Please specify a port. Usage: portkill <port>"
        return 1
    fi

    if ! command -v lsof >/dev/null 2>&1; then
        print_error "This function requires 'lsof', which is not available."
        return 1
    fi

    local pids
    pids=$(lsof -t -i:"$port" 2>/dev/null || true)
    
    if [ -n "$pids" ]; then
        for pid in $pids; do
            kill -9 "$pid"
            echo -e "\033[0;36mInfo:\033[0m \033[1;37mProcess on port $port with PID $pid killed.\033[0m\n"
        done
    else
        print_error "No process found on port $port."
    fi
}

# Stop well-known services like Apache, Nginx, MySQL, MariaDB, Pgsql, redis, memcached, etc.
stop_services() {
    local services=("apache2" "nginx" "mysql" "mariadb" "postgresql" "mongodb" "redis" "memcached" "docker")
    local force_flag="${1-}"
    
    if [[ "$force_flag" != "--force" ]]; then
        if ! needs_confirm_prompt "\033[1;33mWarning:\033[0m \033[1;37mStop all well-known services?\033[0m"; then
            return 0
        fi
    fi

    for service in "${services[@]}"; do
        if systemctl is-active --quiet "$service"; then
            echo -e "\033[0;36mInfo:\033[0m \033[1;37mStopping $service service...\033[0m"
            sudo systemctl stop "$service"
        fi
    done
}

# Function to uninstall Gash and clean up configurations
gash_uninstall() {
    # If we don't have a ~/.gash directory, simply exit
    if [ ! -d "$HOME/.gash" ]; then
        print_error "Gash is not installed on this system."
        return 1
    fi

    echo -e "\033[1;33mWarning:\033[0m \033[1;37mThis will remove Gash and its configuration from this account.\033[0m"
    if ! needs_confirm_prompt "Continue?"; then
        echo -e "\033[1;33mNote:\033[0m \033[1;37mUninstall cancelled.\033[0m"
        return 0
    fi

    # Define profile files to check
    profile_files=( "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.profile" )

    # Remove Gash sourcing lines from profiles
    for profile_file in "${profile_files[@]}"; do
        if [ -f "$profile_file" ]; then
            if command grep -qc 'source.*[./]gashrc' "$profile_file"; then
                echo -e "\033[0;36mInfo:\033[0m \033[1;37mRemoving Gash block from: $profile_file\033[0m"
                # Remove the block between '# Load Gash Bash' and 'fi'
                sed -i.bak '/# Load Gash Bash/,/^fi$/d' "$profile_file"
            else
                echo -e "\033[0;36mInfo:\033[0m \033[1;37mNo Gash block found in: $profile_file; skipping.\033[0m"
            fi
        fi
    done

    # Remove the ~/.gashrc file if it exists
    if [ -f "$HOME/.gashrc" ]; then
        echo -e "\033[0;36mInfo:\033[0m \033[1;37mRemoving ~/.gashrc...\033[0m"
        rm -f "$HOME/.gashrc" >/dev/null 2>&1

        # Failsafe: if the file still exists, try with sudo
        if [ -f "$HOME/.gashrc" ]; then
            echo -e "\033[1;33mWarning:\033[0m \033[1;37mFailed to remove ~/.gashrc; trying with sudo...\033[0m"
            sudo rm -f "$HOME/.gashrc" >/dev/null 2>&1

            if [ -f "$HOME/.gashrc" ]; then
                print_error "Failed to remove ~/.gashrc; please remove it manually."
            fi
        fi
    else
        echo -e "\033[0;36mInfo:\033[0m \033[1;37m~/.gashrc not found; skipping.\033[0m"
    fi

    # Remove the Gash directory
    local GASH_DIR
    GASH_DIR="${GASH_DIR:-$HOME/.gash}"

    if [ -d "$GASH_DIR" ]; then
        echo -e "\033[0;36mInfo:\033[0m \033[1;37mRemoving Gash at ~/.gash...\033[0m"
        rm -rf "$GASH_DIR" > /dev/null 2>&1

        # Failsafe: if the directory still exists, try with sudo
        if [ -d "$GASH_DIR" ]; then
            echo -e "\033[1;33mWarning:\033[0m \033[1;37mFailed to remove ~/.gash; trying with sudo...\033[0m"
            sudo rm -rf "$GASH_DIR" >/dev/null 2>&1

            if [ -d "$GASH_DIR" ]; then
                print_error "Failed to remove ~/.gash; please remove it manually."
            fi
        fi
    else
        echo -e "\033[0;36mInfo:\033[0m \033[1;37m~/.gash not found; skipping.\033[0m"
    fi

    echo -e "\033[1;32mOK:\033[0m \033[1;37mGash uninstalled.\033[0m"
    echo -e "\033[0;36mInfo:\033[0m \033[1;37mRestart your terminal to apply changes.\033[0m"

    # Clean up backup files created by sed (if any)
    for profile_file in "${profile_files[@]}"; do
        if [ -f "${profile_file}.bak" ]; then
            rm -f "${profile_file}.bak"
        fi
    done
}

gash_upgrade() {
    local install_dir="${GASH_DIR:-$HOME/.gash}"

    if [ ! -d "$install_dir" ]; then
        print_error "Failed to change directory to $install_dir"
        return 1
    fi

    # Check if Gash is installed
    if [ ! -d "$install_dir/.git" ]; then
        print_error "Gash is not installed. Please refer to https://github.com/mauriziofonte/gash."
        return 1
    fi

    # Get the current directory
    local CURRENT_DIR
    CURRENT_DIR=$(pwd)

    echo -e "\033[0;36mInfo:\033[0m \033[1;37mUpgrading Gash in $install_dir...\033[0m"
    cd "$install_dir" || { print_error "Failed to change directory to $install_dir"; return 1; }

    # Fetch latest tags
    if ! command git fetch --tags origin; then
        FETCH_ERROR="Failed to fetch updates. Please check your network connection and try manually with 'git fetch'."
        cd "$CURRENT_DIR" || { print_error "Failed to change directory to $CURRENT_DIR"; return 1; }
        print_error "$FETCH_ERROR"
        return 1
    fi

    # Get the latest tag
    local LATEST_TAG
    LATEST_TAG="$(git for-each-ref --sort=-creatordate --format='%(refname:short)' refs/tags | head -n1)"
    if [ -z "$LATEST_TAG" ]; then
        echo -e "\033[1;33mWarning:\033[0m \033[1;37mNo tags found. Using the latest commit on the default branch.\033[0m"
        cd "$CURRENT_DIR" || { print_error "Failed to change directory to $CURRENT_DIR"; return 1; }
        return 0
    fi

    # Check current version
    local CURRENT_TAG
    CURRENT_TAG="$(git describe --tags --abbrev=0 2>/dev/null)"
    if [ "$CURRENT_TAG" = "$LATEST_TAG" ]; then
        local RELEASE_DATE
        RELEASE_DATE="$(git log -1 --format=%ai "$CURRENT_TAG")"
        echo -e "\033[1;32mOK:\033[0m \033[1;37mGash is already up-to-date ($CURRENT_TAG, released on $RELEASE_DATE).\033[0m"
        cd "$CURRENT_DIR" || { print_error "Failed to change directory to $CURRENT_DIR"; return 1; }
        return 0
    fi

    # Checkout the latest release tag
    if ! git checkout "$LATEST_TAG" >/dev/null 2>&1; then
        print_error "Failed to checkout tag $LATEST_TAG."
        cd "$CURRENT_DIR" || { print_error "Failed to change directory to $CURRENT_DIR"; return 1; }
        return 1
    fi

    if ! git reset --hard "$LATEST_TAG" >/dev/null 2>&1; then
        print_error "Failed to reset to $LATEST_TAG."
        cd "$CURRENT_DIR" || { print_error "Failed to change directory to $CURRENT_DIR"; return 1; }
        return 1
    fi

    echo -e "\033[1;32mOK:\033[0m \033[1;37mUpgraded Gash to version $LATEST_TAG.\033[0m"

    # Clean up git history to save space
    echo -e "\033[0;36mInfo:\033[0m \033[1;37mCleaning up Git repository...\033[0m"
    if ! git reflog expire --expire=now --all; then
        cd "$CURRENT_DIR" || { print_error "Failed to change directory to $CURRENT_DIR"; return 1; }
        print_error "Your version of git is out of date. Please update it!"
    fi
    if ! git gc --auto --aggressive --prune=now; then
        cd "$CURRENT_DIR" || { print_error "Failed to change directory to $CURRENT_DIR"; return 1; }
        print_error "Your version of git is out of date. Please update it!"
    fi

    cd "$CURRENT_DIR" || { print_error "Failed to change directory to $CURRENT_DIR"; return 1; }
    echo -e "\033[1;32mOK:\033[0m \033[1;37mGash upgrade completed.\033[0m"
}

gash_inspiring_quote() {
    local tty_width
    tty_width="$(__gash_tty_width)"

    local GASH_DIR
    GASH_DIR="${GASH_DIR:-$HOME/.gash}"
    local QUOTES_FILE="$GASH_DIR/quotes/list.txt"

    # Check if the quote file exists and is readable
    if [[ ! -r "$QUOTES_FILE" ]]; then
        return 1
    fi

    # Read all quotes into an array without changing IFS
    local -a quotes
    mapfile -t quotes < "$QUOTES_FILE"

    if [[ ${#quotes[@]} -eq 0 ]]; then
        return 1
    fi

    # Generate a random index
    local IDX=$(( RANDOM % ${#quotes[@]} ))

    local quote="${quotes[$IDX]}"

    # Wrap only the plain quote text (avoid breaking ANSI escape sequences).
    local visible_prefix="Quote: "
    local prefix_len=${#visible_prefix}
    local wrap_width=$(( tty_width - prefix_len ))
    if (( wrap_width < 20 )); then
        wrap_width=80
    fi

    local first=1
    while IFS= read -r qline; do
        if (( first )); then
            printf '%b%s\n' "\033[0;36mQuote:\033[0m " "$qline"
            first=0
        else
            printf '%s%s\n' "       " "$qline"
        fi
    done < <(printf '%s\n' "$quote" | fold -s -w "$wrap_width")
}

gash_username() {
    local user_name user_id
    # Check if "whoami" command exists and is executable
    if command -v whoami >/dev/null 2>&1; then
        user_name=$(whoami)
    
    # Fallback to "id -un" if available
    elif command -v id >/dev/null 2>&1 && id -un >/dev/null 2>&1; then
        user_name=$(id -un)
    
    # Fallback to "id -u" and grep /etc/passwd if previous methods are unavailable
    elif command -v id >/dev/null 2>&1; then
        user_id=$(id -u)
        user_name=$(grep "^.*:.*:$user_id:" /etc/passwd | cut -d':' -f1)
        
        # If no username was found in /etc/passwd, default to UNKNOWN_USERNAME
        if [ -z "$user_name" ]; then
            user_name="UNKNOWN_USERNAME"
        fi
    else
        # If all methods fail, set to UNKNOWN_USERNAME
        user_name="UNKNOWN_USERNAME"
    fi

    echo "$user_name"
}

gash_unload() {
    needs_help "gash_unload" "gash_unload" "Restores the shell state saved before Gash was loaded (best-effort)." "${1-}" && return

    if [[ -z "${__GASH_SNAPSHOT_TAKEN-}" ]]; then
        print_error "Gash snapshot not found. Source gash.sh first."
        return 1
    fi

    # Restore prompt-related variables
    if [[ "${__GASH_ORIG_PROMPT_COMMAND_SET-0}" -eq 1 ]]; then
        PROMPT_COMMAND="${__GASH_ORIG_PROMPT_COMMAND-}"
    else
        unset PROMPT_COMMAND
    fi

    if [[ "${__GASH_ORIG_PS1_SET-0}" -eq 1 ]]; then
        PS1="${__GASH_ORIG_PS1-}"
    else
        unset PS1
    fi

    if [[ "${__GASH_ORIG_PS2_SET-0}" -eq 1 ]]; then
        PS2="${__GASH_ORIG_PS2-}"
    else
        unset PS2
    fi

    # Restore history-related settings
    if [[ "${__GASH_ORIG_HISTCONTROL_SET-0}" -eq 1 ]]; then
        HISTCONTROL="${__GASH_ORIG_HISTCONTROL-}"
    else
        unset HISTCONTROL
    fi

    if [[ "${__GASH_ORIG_HISTTIMEFORMAT_SET-0}" -eq 1 ]]; then
        HISTTIMEFORMAT="${__GASH_ORIG_HISTTIMEFORMAT-}"
    else
        unset HISTTIMEFORMAT
    fi

    if [[ "${__GASH_ORIG_HISTSIZE_SET-0}" -eq 1 ]]; then
        HISTSIZE="${__GASH_ORIG_HISTSIZE-}"
    else
        unset HISTSIZE
    fi

    if [[ "${__GASH_ORIG_HISTFILESIZE_SET-0}" -eq 1 ]]; then
        HISTFILESIZE="${__GASH_ORIG_HISTFILESIZE-}"
    else
        unset HISTFILESIZE
    fi

    if [[ "${__GASH_ORIG_SHOPT_HISTAPPEND-1}" -eq 0 ]]; then
        shopt -s histappend
    else
        shopt -u histappend
    fi

    if [[ "${__GASH_ORIG_SHOPT_CHECKWINSIZE-1}" -eq 0 ]]; then
        shopt -s checkwinsize
    else
        shopt -u checkwinsize
    fi

    if [[ -n "${__GASH_ORIG_UMASK-}" ]]; then
        umask "${__GASH_ORIG_UMASK}"
    fi

    # Remove aliases that were introduced by Gash.
    if [[ -n "${__GASH_ADDED_ALIASES-}" ]]; then
        while IFS= read -r __gash_name; do
            [[ -z "${__gash_name-}" ]] && continue
            unalias "${__gash_name}" >/dev/null 2>&1 || true
        done <<< "${__GASH_ADDED_ALIASES}"
    fi

    # Remove functions that were introduced by Gash.
    # Avoid removing ourselves until the end.
    if [[ -n "${__GASH_ADDED_FUNCS-}" ]]; then
        while IFS= read -r __gash_name; do
            [[ -z "${__gash_name-}" ]] && continue
            [[ "${__gash_name}" == "gash_unload" ]] && continue
            unset -f "${__gash_name}" >/dev/null 2>&1 || true
        done <<< "${__GASH_ADDED_FUNCS}"
    fi

    # Clear internal snapshot variables (best-effort). Keep user-configurable vars intact.
    unset __GASH_SNAPSHOT_TAKEN __GASH_PRE_FUNCS __GASH_PRE_ALIASES __GASH_ADDED_FUNCS __GASH_ADDED_ALIASES \
        __GASH_ORIG_PS1_SET __GASH_ORIG_PS1 __GASH_ORIG_PS2_SET __GASH_ORIG_PS2 __GASH_ORIG_PROMPT_COMMAND_SET __GASH_ORIG_PROMPT_COMMAND \
        __GASH_ORIG_HISTCONTROL_SET __GASH_ORIG_HISTCONTROL __GASH_ORIG_HISTTIMEFORMAT_SET __GASH_ORIG_HISTTIMEFORMAT \
        __GASH_ORIG_HISTSIZE_SET __GASH_ORIG_HISTSIZE __GASH_ORIG_HISTFILESIZE_SET __GASH_ORIG_HISTFILESIZE \
        __GASH_ORIG_SHOPT_HISTAPPEND __GASH_ORIG_SHOPT_CHECKWINSIZE __GASH_ORIG_UMASK

    unset __gash_name

    # Finally, remove gash_unload itself.
    unset -f gash_unload >/dev/null 2>&1 || true

    return 0
}

# Define custom help function
function gash_help() {
    # Display the built-in Bash help
    builtin help "$@"

    # If no specific help topic is requested, show Gash-specific help
    if [[ -z "${1-}" ]]; then
        echo
        echo -e "\e[1;37m===\033[0m \033[0;36mG\033[0;33ma\033[38;5;214ms\033[0;32mh \033[1;37mGash, Another SHell!\033[0m - \e[1;37mCustom Commands ===\033[0m"
        
        # List Gash-defined functions and their descriptions
        echo -e " > \e[0;33mlargest_files\033[0m \e[0;36m[PATH]\033[0m - \e[1;37mLists the top 100 largest files in PATH (or current directory if not specified), sorted by size.\033[0m"
        echo -e " > \e[0;33mlargest_dirs\033[0m \e[0;36m[PATH]\033[0m - \e[1;37mLists the top 100 largest directories in PATH (or current directory if not specified), sorted by size.\033[0m"
        echo -e " > \e[0;33mfind_large_dirs\033[0m \e[0;36m[--size SIZE] [DIRECTORY]\033[0m - \e[1;37mFinds directories larger than SIZE (default 20M) and lists their size and modification time of their largest file.\033[0m"
        echo -e " > \e[0;33mdisk_usage_fs\033[0m - \e[1;37mDisplays disk usage for specific filesystem types, formatted for easy reading.\033[0m"
        echo -e " > \e[0;33mhgrep\033[0m \e[0;36mPATTERN\033[0m - \e[1;37mSearches the bash history for commands matching PATTERN.\033[0m"
        echo -e " > \e[0;33mgit_dump_revisions\033[0m \e[0;36mFILENAME\033[0m - \e[1;37mDump all revisions of a file in a GIT repo into multiple separate files.\033[0m"
        echo -e " > \e[0;33mgit_apply_feature_patch\033[0m \e[0;36mMAIN_BRANCH FEATURE_BRANCH COMMIT_HASH\033[0m - \e[1;37mCreate and apply a patch from a feature branch to the main branch.\033[0m"
        echo -e " > \e[0;33mgtags\033[0m - \e[1;37mLists all local and remote tags.\033[0m"
        echo -e " > \e[0;33mgadd_tag\033[0m \e[0;36m<tag_name> \"<tag_message>\"\033[0m - \e[1;37mCreates an annotated tag and pushes it to remote.\033[0m"
        echo -e " > \e[0;33mgdel_tag\033[0m \e[0;36m<tag_name>\033[0m - \e[1;37mDeletes a tag both locally and on remote.\033[0m"
        echo -e " > \e[0;33mdocker_prune_all\033[0m - \e[1;37mRemove all Docker containers, images, volumes, and networks.\033[0m"
        echo -e " > \e[0;33mplease\033[0m - \e[1;37mRe-runs the previous command with sudo.\033[0m"
        echo -e " > \e[0;33mmkcd\033[0m \e[0;36mDIRECTORY\033[0m - \e[1;37mCreates a new directory and changes into it.\033[0m"
        echo -e " > \e[0;33mextract\033[0m \e[0;36mARCHIVE_FILE [OUTPUT_DIR]\033[0m - \e[1;37mExtracts the ARCHIVE_FILE in the current directory or the specified OUTPUT_DIR.\033[0m"
        echo -e " > \e[0;33mbackup_file\033[0m \e[0;36mFILE\033[0m - \e[1;37mCreates a backup of a file with a timestamp suffix.\033[0m"
        echo -e " > \e[0;33mlist_empty_dirs\033[0m \e[0;36m[DIRECTORY]\033[0m - \e[1;37mList all empty directories in the specified path (default: current directory).\033[0m"
        echo -e " > \e[0;33mmyip\033[0m - \e[1;37mGet your public IP address.\033[0m"
        echo -e " > \e[0;33mpsgrep\033[0m \e[0;36mPROCESS_NAME\033[0m - \e[1;37mSearch for a process by name.\033[0m"
        echo -e " > \e[0;33mpskill\033[0m \e[0;36mPROCESS_NAME\033[0m - \e[1;37mKill all processes by name.\033[0m"
        echo -e " > \e[0;33mportkill\033[0m \e[0;36mPORT\033[0m - \e[1;37mKill all processes by port.\033[0m"
        echo -e " > \e[0;33mstop_services\033[0m \e[0;36m[--force]\033[0m - \e[1;37mStop well-known services like Apache, Nginx, MySQL, MariaDB, Pgsql, redis, memcached, etc.\033[0m"
        echo -e " > \e[0;33mgash_inspiring_quote\033[0m - \e[1;37mDisplay an inspiring quote, to enlighten your day.\033[0m"
        echo -e " > \e[0;33mgash_username\033[0m - \e[1;37mGet the current username.\033[0m"
        echo -e " > \e[0;33mgash_upgrade\033[0m - \e[1;37mUpgrade Gash to the latest version.\033[0m"
        echo -e " > \e[0;33mgash_uninstall\033[0m - \e[1;37mUninstall Gash and clean up configurations.\033[0m"

        echo -e " > \e[0;33m..\033[0m - \e[1;37mChange to the parent directory.\033[0m"
        echo -e " > \e[0;33m...\033[0m - \e[1;37mChange to the parent's parent directory.\033[0m"
        echo -e " > \e[0;33mcd..\033[0m - \e[1;37mChange to the parent directory.\033[0m"
        echo -e " > \e[0;33m...\033[0m - \e[1;37mChange to the parent's parent directory.\033[0m"
        echo -e " > \e[0;33m....\033[0m - \e[1;37mChange to the parent's parent's parent directory.\033[0m"
        echo -e " > \e[0;33m.....\033[0m - \e[1;37mChange to the parent's parent's parent directory.\033[0m"
        echo -e " > \e[0;33m.4\033[0m - \e[1;37mChange to the parent's parent's parent directory.\033[0m"
        echo -e " > \e[0;33m.5\033[0m - \e[1;37mChange to the parent's parent's parent's parent directory.\033[0m"
        echo -e " > \e[0;33mports\033[0m - \e[1;37mDisplay listening ports.\033[0m"
        if grep -qi "microsoft" /proc/version && [ -n "$WSLENV" ]; then
            echo -e " > \e[0;33mwslrestart\033[0m - \e[1;37mRestart WSL.\033[0m"
            echo -e " > \e[0;33mwslshutdown\033[0m - \e[1;37mShutdown WSL.\033[0m"
            echo -e " > \e[0;33mexplorer\033[0m - \e[1;37mOpen the current directory in Windows Explorer.\033[0m"
            echo -e " > \e[0;33mtaskmanager\033[0m - \e[1;37mOpen Task Manager.\033[0m"
        fi
        if command -v git >/dev/null 2>&1; then
            echo -e " > \e[0;33mgl or glog\033[0m - \e[1;37mDisplay a pretty git log.\033[0m"
            echo -e " > \e[0;33mgst or gstatus\033[0m - \e[1;37mDisplay the git status.\033[0m"
            echo -e " > \e[0;33mga or gadd\033[0m - \e[1;37mAdd files to the git index.\033[0m"
            echo -e " > \e[0;33mgc or gcommit\033[0m - \e[1;37mCommit changes to the git repository.\033[0m"
            echo -e " > \e[0;33mgp or gpush\033[0m - \e[1;37mPush changes to the git repository.\033[0m"
            echo -e " > \e[0;33mgco or gcheckout\033[0m - \e[1;37mSwitch branches or restore working tree files.\033[0m"
            echo -e " > \e[0;33mgb or gbranch\033[0m - \e[1;37mList, create, or delete branches.\033[0m"
            echo -e " > \e[0;33mgd or gdiff\033[0m - \e[1;37mShow changes between commits, commit and working tree, etc.\033[0m"
            echo -e " > \e[0;33mgt or gtags\033[0m - \e[1;37mCreate, list, delete or verify a tag object signed with GPG.\033[0m"
            echo -e " > \e[0;33mgta or gadd_tag\033[0m - \e[1;37mCreate an annotated tag and push it to the remote.\033[0m"
            echo -e " > \e[0;33mgtd or gdel_tag\033[0m - \e[1;37mDelete a tag both locally and on remote.\033[0m"
        fi
        if command -v docker >/dev/null 2>&1; then
            echo -e " > \e[0;33mdcls\033[0m - \e[1;37mList all Docker containers.\033[0m"
            echo -e " > \e[0;33mdclsr\033[0m - \e[1;37mList running Docker containers.\033[0m"
            echo -e " > \e[0;33mdils\033[0m - \e[1;37mList all Docker images.\033[0m"
            echo -e " > \e[0;33mdcrm\033[0m - \e[1;37mRemove all Docker containers.\033[0m"
            echo -e " > \e[0;33mdirm\033[0m - \e[1;37mRemove all Docker images.\033[0m"
            echo -e " > \e[0;33mdstop\033[0m - \e[1;37mStop a Docker container.\033[0m"
            echo -e " > \e[0;33mdstopall\033[0m - \e[1;37mStop all Docker containers.\033[0m"
            echo -e " > \e[0;33mdocker_stop_all\033[0m - \e[1;37mStop all Docker containers.\033[0m"
            echo -e " > \e[0;33mdstart\033[0m - \e[1;37mStart a Docker container.\033[0m"
            echo -e " > \e[0;33mdstartall\033[0m - \e[1;37mStart all Docker containers.\033[0m"
            echo -e " > \e[0;33mdocker_start_all\033[0m - \e[1;37mStart all Docker containers.\033[0m"
            echo -e " > \e[0;33mdexec\033[0m - \e[1;37mExecute a command in a running Docker container.\033[0m"
            echo -e " > \e[0;33mdrm\033[0m - \e[1;37mRemove a Docker container.\033[0m"
            echo -e " > \e[0;33mdrmi\033[0m - \e[1;37mRemove a Docker image.\033[0m"
            echo -e " > \e[0;33mdlogs\033[0m - \e[1;37mShow logs of a Docker container.\033[0m"
            echo -e " > \e[0;33mdinspect\033[0m - \e[1;37mInspect a Docker object.\033[0m"
            echo -e " > \e[0;33mdnetls\033[0m - \e[1;37mList all Docker networks.\033[0m"
            echo -e " > \e[0;33mdpruneall\033[0m - \e[1;37mRemove all Docker containers, images, volumes, and networks.\033[0m"
            echo -e " > \e[0;33mdocker_prune_all\033[0m - \e[1;37mRemove all Docker containers, images, volumes, and networks.\033[0m"
        fi
    fi
}